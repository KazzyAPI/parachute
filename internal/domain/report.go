package domain

import "time"

type HostReport struct {
	Name      string
	Endpoint  string
	Reachable bool
	Results   []Result
}

type Report struct {
	CycleID string
	Started time.Time
	Ended   time.Time
	Armed   bool
	Hosts   []HostReport
}

type Counts struct {
	Scanned     int
	Updated     int
	Failed      int
	Fresh       int
	Skipped     int
	Held        int
	WouldUpdate int
}

func (r Report) Results() []Result {
	var all []Result
	for _, host := range r.Hosts {
		all = append(all, host.Results...)
	}
	return all
}

func (r Report) Counts() Counts {
	var counts Counts
	for _, result := range r.Results() {
		counts.Scanned++
		switch result.State {
		case StateUpdated:
			counts.Updated++
		case StateFailed, StateRolledBack, StateRollbackFailed:
			counts.Failed++
		case StateFresh:
			counts.Fresh++
		case StateSkipped:
			counts.Skipped++
		case StateHeld:
			counts.Held++
		case StateWouldUpdate:
			counts.WouldUpdate++
		}
	}
	return counts
}

func (r Report) AllHostsReachable() bool {
	for _, host := range r.Hosts {
		if !host.Reachable {
			return false
		}
	}
	return true
}
