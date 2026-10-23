package porcelain

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/KazzyAPI/parachute/internal/domain"
)

type entry struct {
	V               int     `json:"v"`
	Type            string  `json:"type"`
	Cycle           string  `json:"cycle"`
	Host            string  `json:"host"`
	Stack           *string `json:"stack"`
	Service         *string `json:"service"`
	Container       *string `json:"container"`
	Image           *string `json:"image"`
	ImageID         *string `json:"image_id"`
	CandidateDigest *string `json:"candidate_digest"`
	State           string  `json:"state"`
	Reason          *string `json:"reason"`
	Transaction     *string `json:"txn"`
	Snapshot        *string `json:"snapshot"`
}

type summary struct {
	V           int    `json:"v"`
	Type        string `json:"type"`
	Cycle       string `json:"cycle"`
	Armed       bool   `json:"armed"`
	Scanned     int    `json:"scanned"`
	Updated     int    `json:"updated"`
	Failed      int    `json:"failed"`
	Fresh       int    `json:"fresh"`
	Skipped     int    `json:"skipped"`
	Held        int    `json:"held"`
	WouldUpdate int    `json:"would_update"`
}

type Writer struct {
	out io.Writer
}

func NewWriter(out io.Writer) *Writer {
	return &Writer{out: out}
}

func (w *Writer) Publish(_ context.Context, report domain.Report) error {
	encoder := json.NewEncoder(w.out)
	encoder.SetEscapeHTML(false)
	for _, result := range report.Results() {
		line := entry{
			V: 1, Type: "entry", Cycle: report.CycleID, Host: result.Host, State: string(result.State),
			Stack: nullable(result.Stack), Service: nullable(result.Service), Container: nullable(result.Container),
			Image: nullable(result.Image), ImageID: nullable(result.ImageID), CandidateDigest: nullable(result.CandidateDigest),
			Reason: nullable(result.Reason), Transaction: nullable(result.Transaction), Snapshot: nullable(result.Snapshot),
		}
		if err := encoder.Encode(line); err != nil {
			return fmt.Errorf("write porcelain entry: %w", err)
		}
	}
	counts := report.Counts()
	line := summary{
		V: 1, Type: "summary", Cycle: report.CycleID, Armed: report.Armed, Scanned: counts.Scanned, Updated: counts.Updated,
		Failed: counts.Failed, Fresh: counts.Fresh, Skipped: counts.Skipped, Held: counts.Held, WouldUpdate: counts.WouldUpdate,
	}
	if err := encoder.Encode(line); err != nil {
		return fmt.Errorf("write porcelain summary: %w", err)
	}
	return nil
}

func nullable(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
