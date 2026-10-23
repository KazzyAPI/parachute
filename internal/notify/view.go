package notify

import (
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
)

type ServiceView struct {
	Name            string
	Container       string
	Image           string
	CandidateDigest string
	State           string
	Reason          string
	HoldUntil       string
	TransactionID   string
	SnapshotID      string
}

type StackView struct {
	Name     string
	Compose  bool
	Services []ServiceView
}

type HostView struct {
	Name      string
	Reachable bool
	Stacks    []StackView
}

type ReportView struct {
	Title       string
	Host        string
	CycleID     string
	Armed       bool
	Hosts       []HostView
	Scanned     int
	Updated     int
	Failed      int
	Fresh       int
	Skipped     int
	Held        int
	WouldUpdate int
}

type ViewBuilder struct {
	minimum  domain.Level
	hostname string
}

func NewViewBuilder(minimum domain.Level, hostname string) *ViewBuilder {
	return &ViewBuilder{minimum: minimum, hostname: hostname}
}

func (b *ViewBuilder) Build(report domain.Report, title string, hidden func(domain.Result) bool) ReportView {
	counts := report.Counts()
	view := ReportView{
		Title: title, Host: b.hostname, CycleID: report.CycleID, Armed: report.Armed,
		Scanned: counts.Scanned, Updated: counts.Updated, Failed: counts.Failed, Fresh: counts.Fresh,
		Skipped: counts.Skipped, Held: counts.Held, WouldUpdate: counts.WouldUpdate,
	}
	for _, host := range report.Hosts {
		view.Hosts = append(view.Hosts, HostView{Name: host.Name, Reachable: host.Reachable, Stacks: b.stacks(host.Results, hidden)})
	}
	return view
}

func (b *ViewBuilder) Shows(level domain.Level) bool { return level >= b.minimum }

func (b *ViewBuilder) stacks(results []domain.Result, hidden func(domain.Result) bool) []StackView {
	var stacks []StackView
	for _, result := range results {
		if !b.Shows(result.State.Level()) || hidden(result) {
			continue
		}
		if len(stacks) == 0 || stacks[len(stacks)-1].Name != result.Stack {
			stacks = append(stacks, StackView{Name: result.Stack, Compose: result.Compose})
		}
		last := &stacks[len(stacks)-1]
		last.Services = append(last.Services, ServiceView{
			Name: result.Service, Container: result.Container, Image: result.Image, CandidateDigest: result.CandidateDigest,
			State: string(result.State), Reason: result.Reason, TransactionID: result.Transaction, SnapshotID: result.Snapshot,
			HoldUntil: holdUntil(result),
		})
	}
	return stacks
}

func holdUntil(result domain.Result) string {
	if result.HoldUntil == nil {
		return ""
	}
	return result.HoldUntil.UTC().Format(time.RFC3339)
}
