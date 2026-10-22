package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

type requirement struct {
	id        string
	file      string
	accepted  bool
	normative bool
}

type milestone struct {
	name string
	ids  []string
}

type Tracer struct {
	root      string
	out       io.Writer
	heading   *regexp.Regexp
	normative *regexp.Regexp
	testName  *regexp.Regexp
	id        *regexp.Regexp
	milestone *regexp.Regexp
}

func NewTracer(root string, out io.Writer) *Tracer {
	return &Tracer{
		root:      root,
		out:       out,
		heading:   regexp.MustCompile(`^### ([A-Z]{3}-\d{3}) `),
		normative: regexp.MustCompile(`\b(MUST|SHOULD)\b`),
		testName:  regexp.MustCompile(`func Test([A-Z]{3})[_-](\d{3})_`),
		id:        regexp.MustCompile(`[A-Z]{3}-\d{3}`),
		milestone: regexp.MustCompile(`^M\d+\b`),
	}
}

func (t *Tracer) requirements() ([]requirement, error) {
	files, err := filepath.Glob(filepath.Join(t.root, "docs", "specs", "[0-9][0-9]-*.md"))
	if err != nil {
		return nil, err
	}
	var requirements []requirement
	for _, file := range files {
		contents, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		accepted := strings.Contains(string(contents), "**Status:** Accepted")
		var current *requirement
		for _, line := range strings.Split(string(contents), "\n") {
			if match := t.heading.FindStringSubmatch(line); match != nil {
				requirements = append(requirements, requirement{id: match[1], file: filepath.Base(file), accepted: accepted})
				current = &requirements[len(requirements)-1]
				continue
			}
			if strings.HasPrefix(line, "#") {
				current = nil
			}
			if current != nil && !strings.HasPrefix(line, "- **Given**") && t.normative.MatchString(line) {
				current.normative = true
			}
		}
	}
	return requirements, nil
}

func (t *Tracer) tested() (map[string]int, error) {
	files, err := filepath.Glob(filepath.Join(t.root, "e2e", "*_test.go"))
	if err != nil {
		return nil, err
	}
	tests := map[string]int{}
	for _, file := range files {
		contents, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		for _, match := range t.testName.FindAllStringSubmatch(string(contents), -1) {
			tests[match[1]+"-"+match[2]]++
		}
	}
	return tests, nil
}

func (t *Tracer) milestones() ([]milestone, error) {
	contents, err := os.ReadFile(filepath.Join(t.root, "docs", "specs", "README.md"))
	if err != nil {
		return nil, err
	}
	var milestones []milestone
	for _, line := range strings.Split(string(contents), "\n") {
		cells := strings.Split(line, "|")
		if len(cells) < 3 || !t.milestone.MatchString(strings.TrimSpace(cells[1])) {
			continue
		}
		milestones = append(milestones, milestone{name: strings.TrimSpace(cells[1]), ids: t.id.FindAllString(cells[2], -1)})
	}
	return milestones, nil
}

type milestoneCoverage struct {
	name    string
	total   int
	covered int
	missing []string
}

type coverage struct {
	milestones      []milestoneCoverage
	later           int
	acceptedMissing []string
}

func (c coverage) ok() bool {
	for _, stone := range c.milestones {
		if len(stone.missing) > 0 {
			return false
		}
	}
	return len(c.acceptedMissing) == 0
}

func (t *Tracer) Run() (bool, error) {
	requirements, err := t.requirements()
	if err != nil {
		return false, err
	}
	tests, err := t.tested()
	if err != nil {
		return false, err
	}
	milestones, err := t.milestones()
	if err != nil {
		return false, err
	}
	result := t.measure(requirements, tests, milestones)
	return result.ok(), t.write(result)
}

func (t *Tracer) measure(requirements []requirement, tests map[string]int, milestones []milestone) coverage {
	known := map[string]requirement{}
	for _, req := range requirements {
		known[req.id] = req
	}
	var result coverage
	planned := map[string]bool{}
	for _, stone := range milestones {
		stoneCoverage := milestoneCoverage{name: stone.name, total: len(stone.ids)}
		for _, id := range stone.ids {
			planned[id] = true
			req, exists := known[id]
			switch {
			case !exists:
				stoneCoverage.missing = append(stoneCoverage.missing, id+" (not in any spec)")
			case tests[id] > 0:
				stoneCoverage.covered++
			case req.normative:
				stoneCoverage.missing = append(stoneCoverage.missing, id)
			}
		}
		result.milestones = append(result.milestones, stoneCoverage)
	}
	for _, req := range requirements {
		if !planned[req.id] {
			result.later++
		}
		if req.accepted && req.normative && tests[req.id] == 0 {
			result.acceptedMissing = append(result.acceptedMissing, req.id)
		}
	}
	slices.Sort(result.acceptedMissing)
	return result
}

func (t *Tracer) write(result coverage) error {
	var report strings.Builder
	for _, stone := range result.milestones {
		fmt.Fprintf(&report, "%s: %d requirements, %d tested\n", stone.name, stone.total, stone.covered)
		if len(stone.missing) > 0 {
			fmt.Fprintf(&report, "  untested: %s\n", strings.Join(stone.missing, ", "))
		}
	}
	fmt.Fprintf(&report, "later milestones: %d requirements\n", result.later)
	if len(result.acceptedMissing) > 0 {
		fmt.Fprintf(&report, "accepted MUST/SHOULD without a test: %s\n", strings.Join(result.acceptedMissing, ", "))
	}
	if _, err := io.WriteString(t.out, report.String()); err != nil {
		return fmt.Errorf("write trace report: %w", err)
	}
	return nil
}

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	ok, err := NewTracer(root, os.Stdout).Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "trace:", err)
		os.Exit(2)
	}
	if !ok {
		os.Exit(1)
	}
}
