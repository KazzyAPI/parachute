package metrics

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/KazzyAPI/parachute/internal/domain"
)

type hostGauges struct {
	scanned, updated, failed, held, up int
}

type Collector struct {
	lock              sync.Mutex
	hosts             []string
	gauges            map[string]hostGauges
	rollbacks         map[string]int
	rollbackFailures  map[string]int
	scansTotal        int
	scansSkipped      int
	labelValueEscaper *strings.Replacer
}

func NewCollector() *Collector {
	return &Collector{
		gauges: map[string]hostGauges{}, rollbacks: map[string]int{}, rollbackFailures: map[string]int{},
		labelValueEscaper: strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`),
	}
}

func (c *Collector) CycleStarted() {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.scansTotal++
}

func (c *Collector) CycleSkipped() {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.scansSkipped++
}

func (c *Collector) Publish(_ context.Context, report domain.Report) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	for _, host := range report.Hosts {
		if _, known := c.gauges[host.Name]; !known {
			c.hosts = append(c.hosts, host.Name)
		}
		gauges := hostGauges{}
		if host.Reachable {
			gauges.up = 1
		}
		for _, result := range host.Results {
			gauges.scanned++
			switch result.State {
			case domain.StateUpdated:
				gauges.updated++
			case domain.StateFailed:
				gauges.failed++
			case domain.StateRolledBack:
				gauges.failed++
				c.rollbacks[host.Name]++
			case domain.StateRollbackFailed:
				gauges.failed++
				c.rollbackFailures[host.Name]++
			case domain.StateHeld:
				gauges.held++
			}
		}
		c.gauges[host.Name] = gauges
	}
	return nil
}

func (c *Collector) Write(out io.Writer) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	var text strings.Builder
	c.perHost(&text, "parachute_containers_scanned", "gauge", "Candidates checked in the last completed cycle.", func(g hostGauges, _ string) int { return g.scanned })
	c.perHost(&text, "parachute_containers_updated", "gauge", "Results updated in the last completed cycle.", func(g hostGauges, _ string) int { return g.updated })
	c.perHost(&text, "parachute_containers_failed", "gauge", "Results failed, rolled_back, or rollback_failed in the last completed cycle.", func(g hostGauges, _ string) int { return g.failed })
	c.perHost(&text, "parachute_containers_held", "gauge", "Results held in the last completed cycle.", func(g hostGauges, _ string) int { return g.held })
	c.single(&text, "parachute_scans_total", "Cycles started.", c.scansTotal)
	c.single(&text, "parachute_scans_skipped", "Cycle starts skipped because another run was in progress.", c.scansSkipped)
	c.perHost(&text, "parachute_rollbacks_total", "counter", "Transactions ended rolled_back.", func(_ hostGauges, host string) int { return c.rollbacks[host] })
	c.perHost(&text, "parachute_rollback_failures_total", "counter", "Transactions ended rollback_failed.", func(_ hostGauges, host string) int { return c.rollbackFailures[host] })
	c.perHost(&text, "parachute_host_up", "gauge", "1 when the host was reachable in the last cycle, else 0.", func(g hostGauges, _ string) int { return g.up })
	header(&text, "parachute_restore_verification_last_success_timestamp_seconds", "gauge", "Unix time of the last passed restore verification.")
	if _, err := io.WriteString(out, text.String()); err != nil {
		return fmt.Errorf("write metrics: %w", err)
	}
	return nil
}

func (c *Collector) perHost(text *strings.Builder, name, kind, help string, value func(hostGauges, string) int) {
	header(text, name, kind, help)
	for _, host := range c.hosts {
		fmt.Fprintf(text, "%s{host=\"%s\"} %d\n", name, c.labelValueEscaper.Replace(host), value(c.gauges[host], host))
	}
}

func (c *Collector) single(text *strings.Builder, name, help string, value int) {
	header(text, name, "counter", help)
	fmt.Fprintf(text, "%s %d\n", name, value)
}

func header(text *strings.Builder, name, kind, help string) {
	fmt.Fprintf(text, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, kind)
}
