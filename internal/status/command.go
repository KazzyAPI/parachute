package status

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/KazzyAPI/parachute/internal/state"
)

type StateReader interface {
	Read() (state.Document, bool, error)
}

type Clock interface {
	Now() time.Time
}

type Command struct {
	builder *Builder
	state   StateReader
	clock   Clock
	out     io.Writer
}

func NewCommand(builder *Builder, stateReader StateReader, clock Clock, out io.Writer) *Command {
	return &Command{builder: builder, state: stateReader, clock: clock, out: out}
}

func (c *Command) Document(ctx context.Context) (Document, error) {
	saved, _, err := c.state.Read()
	if err != nil {
		return Document{}, err
	}
	return c.builder.Build(ctx, saved, c.clock.Now())
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
	table := tabwriter.NewWriter(c.out, 0, 4, 2, ' ', 0)
	fmt.Fprintf(table, "armed: %t\n\n", document.Armed)
	fmt.Fprintln(table, "HOST\tSTACK\tSERVICE\tCONTAINER\tIMAGE\tUPDATE\tHOLD")
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
				fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", host.Name, stack.Name, service.Name, service.Container, service.Image, update, hold)
			}
		}
	}
	if err := table.Flush(); err != nil {
		return fmt.Errorf("write status: %w", err)
	}
	return nil
}
