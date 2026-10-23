package status

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/KazzyAPI/parachute/internal/state"
)

type Clock interface {
	Now() time.Time
}

type Command struct {
	source Source
	clock  Clock
	out    io.Writer
}

func NewCommand(source Source, clock Clock, out io.Writer) *Command {
	return &Command{source: source, clock: clock, out: out}
}

func (c *Command) Document(ctx context.Context, saved state.Document) (Document, error) {
	return c.source.Build(ctx, saved, c.clock.Now())
}

func (c *Command) WriteJSON(document Document) error {
	encoder := json.NewEncoder(c.out)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(document); err != nil {
		return fmt.Errorf("write status: %w", err)
	}
	return nil
}

func (c *Command) WriteTable(document Document) error {
	var rows strings.Builder
	fmt.Fprintf(&rows, "armed: %t\n\nHOST\tSTACK\tSERVICE\tCONTAINER\tIMAGE\tUPDATE\tHOLD\n", document.Armed)
	for _, host := range document.Hosts {
		for _, stack := range host.Stacks {
			for _, service := range stack.Services {
				update, hold := "no", "-"
				if service.UpdateAvailable && service.CandidateDigest != nil {
					update = *service.CandidateDigest
				}
				if service.Hold != nil {
					hold = service.Hold.Reason
				}
				fmt.Fprintf(&rows, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", host.Name, stack.Name, service.Name, service.Container, service.Image, update, hold)
			}
		}
	}
	table := tabwriter.NewWriter(c.out, 0, 4, 2, ' ', 0)
	if _, err := io.WriteString(table, rows.String()); err != nil {
		return fmt.Errorf("write status: %w", err)
	}
	if err := table.Flush(); err != nil {
		return fmt.Errorf("write status: %w", err)
	}
	return nil
}
