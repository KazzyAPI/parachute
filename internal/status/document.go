package status

import "time"

const Format = "parachute.status/v1"

type Document struct {
	Format    string    `json:"format"`
	Generated time.Time `json:"generated"`
	Armed     bool      `json:"armed"`
	LastCycle *Cycle    `json:"last_cycle"`
	Hosts     []Host    `json:"hosts"`
}

type Cycle struct {
	ID      string    `json:"id"`
	Started time.Time `json:"started"`
	Ended   time.Time `json:"ended"`
}

type Host struct {
	Name      string  `json:"name"`
	Reachable bool    `json:"reachable"`
	Stacks    []Stack `json:"stacks"`
}

type Stack struct {
	Name                      string    `json:"name"`
	Compose                   bool      `json:"compose"`
	LastBackup                *struct{} `json:"last_backup"`
	LastRestoreVerification   *struct{} `json:"last_restore_verification"`
	LastGoodRestoreAgeSeconds *int      `json:"last_good_restore_age_seconds"`
	Frozen                    []Freeze  `json:"frozen"`
	Services                  []Service `json:"services"`
}

type Freeze struct {
	Transaction string   `json:"transaction"`
	Snapshot    string   `json:"snapshot"`
	Containers  []string `json:"containers"`
}

type Hold struct {
	Reason string     `json:"reason"`
	Until  *time.Time `json:"until"`
}

type Service struct {
	Name            string       `json:"name"`
	Container       string       `json:"container"`
	Image           string       `json:"image"`
	ImageID         string       `json:"image_id"`
	UpdateAvailable bool         `json:"update_available"`
	CandidateDigest *string      `json:"candidate_digest"`
	Hold            *Hold        `json:"hold"`
	Drift           *string      `json:"drift"`
	LastTransaction *Transaction `json:"last_transaction"`
}

type Transaction struct {
	ID     string    `json:"id"`
	State  string    `json:"state"`
	Result *string   `json:"result"`
	Reason *string   `json:"reason"`
	Time   time.Time `json:"time"`
}
