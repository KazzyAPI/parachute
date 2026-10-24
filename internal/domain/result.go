package domain

import "time"

type State string

const (
	StateFresh          State = "fresh"
	StateUpdated        State = "updated"
	StateWouldUpdate    State = "would_update"
	StateHeld           State = "held"
	StateSkipped        State = "skipped"
	StateFailed         State = "failed"
	StateRolledBack     State = "rolled_back"
	StateRollbackFailed State = "rollback_failed"
)

func (s State) Level() Level {
	switch s {
	case StateFresh, StateSkipped:
		return LevelDebug
	case StateFailed, StateRolledBack:
		return LevelWarn
	case StateRollbackFailed:
		return LevelError
	}
	return LevelInfo
}

type Result struct {
	Host            string     `json:"host"`
	Stack           string     `json:"stack"`
	Compose         bool       `json:"compose"`
	Service         string     `json:"service"`
	Container       string     `json:"container"`
	Image           string     `json:"image"`
	ImageID         string     `json:"image_id"`
	CandidateDigest string     `json:"candidate_digest"`
	State           State      `json:"state"`
	Reason          string     `json:"reason"`
	Transaction     string     `json:"txn"`
	Snapshot        string     `json:"snapshot"`
	HoldUntil       *time.Time `json:"hold_until,omitempty"`
	Drift           *string    `json:"drift,omitempty"`
	ContainerID     string     `json:"container_id,omitempty"`
	Detail          string     `json:"-"`
	TargetImage     string     `json:"-"`
}

func NewResult(host string, container Container) Result {
	return Result{
		Host:        host,
		Stack:       container.Stack(),
		Compose:     container.IsCompose(),
		Service:     container.Service(),
		Container:   container.Name,
		Image:       container.ImageRef,
		ImageID:     container.ImageID,
		ContainerID: container.ID,
	}
}

func (r Result) Conclude(state State, reason, detail string) Result {
	r.State, r.Reason, r.Detail = state, reason, detail
	return r
}

func (r Result) Available(candidate string) Result {
	r.State, r.CandidateDigest = StateWouldUpdate, candidate
	return r
}

func (r Result) UpdateAvailable() bool {
	return r.CandidateDigest != "" && r.State != StateFresh
}
