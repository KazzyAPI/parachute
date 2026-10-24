package txn

import "fmt"

type State string

const (
	StateSnapshot     State = "snapshot"
	StateUpdate       State = "update"
	StateVerify       State = "verify"
	StateHandover     State = "handover"
	StateRollingBack  State = "rolling_back"
	StateAborted      State = "aborted"
	StateCommitted    State = "committed"
	StateRolledBack   State = "rolled_back"
	StateRollbackFail State = "rollback_failed"
)

func (s State) next() []State {
	switch s {
	case StateSnapshot:
		return []State{StateUpdate, StateAborted, StateHandover}
	case StateUpdate:
		return []State{StateVerify, StateRollingBack}
	case StateVerify:
		return []State{StateCommitted, StateRollingBack}
	case StateHandover:
		return []State{StateCommitted, StateRollingBack, StateRollbackFail}
	case StateRollingBack:
		return []State{StateRolledBack, StateRollbackFail}
	default:
		return nil
	}
}

func (s State) Valid() bool {
	return s.Open() || s.Terminal()
}

func (s State) Open() bool {
	return len(s.next()) > 0
}

func (s State) Terminal() bool {
	switch s {
	case StateAborted, StateCommitted, StateRolledBack, StateRollbackFail:
		return true
	default:
		return false
	}
}

func CanAdvance(from, to State) bool {
	for _, next := range from.next() {
		if next == to {
			return true
		}
	}
	return false
}

func CheckAdvance(from, to State) error {
	if !CanAdvance(from, to) {
		return fmt.Errorf("illegal transaction transition %s -> %s", from, to)
	}
	return nil
}
