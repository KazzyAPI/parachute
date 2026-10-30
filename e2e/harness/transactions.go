package harness

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

var transactionID = regexp.MustCompile(`txn-[0-9]{8}T[0-9]{6}Z-[0-9a-f]{6}`)

func (e *Env) TransactionIDs() []string {
	contents, err := os.ReadFile(filepath.Join(e.StateDir, "state.json"))
	if err != nil {
		return nil
	}
	found := transactionID.FindAllString(string(contents), -1)
	slices.Sort(found)
	return slices.Compact(found)
}

func (e *Env) TransactionContainers(helpersOnly bool) []string {
	e.T.Helper()
	var queries [][]string
	if !helpersOnly {
		queries = append(queries, []string{"--filter", "label=parachute.transaction", "--filter", "label=parachute-e2e=" + e.ID})
	}
	for _, id := range e.TransactionIDs() {
		query := []string{"--filter", "label=parachute.transaction=" + id}
		if helpersOnly {
			query = append(query, "--filter", "label=parachute.helper=true")
		}
		queries = append(queries, query)
	}
	var found []string
	for _, query := range queries {
		found = append(found, strings.Fields(e.Docker(append([]string{"ps", "-aq"}, query...)...))...)
	}
	slices.Sort(found)
	return slices.Compact(found)
}
