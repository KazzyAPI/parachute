package txn

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

func (s State) Open() bool {
	switch s {
	case StateSnapshot, StateUpdate, StateVerify, StateHandover, StateRollingBack:
		return true
	default:
		return false
	}
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
	if from == to {
		return true
	}
	switch from {
	case StateSnapshot:
		return to == StateUpdate || to == StateHandover || to == StateAborted
	case StateUpdate:
		return to == StateVerify || to == StateRollingBack
	case StateVerify:
		return to == StateCommitted || to == StateRollingBack
	case StateHandover:
		return to == StateCommitted || to == StateRollingBack || to == StateRollbackFail
	case StateRollingBack:
		return to == StateRolledBack || to == StateRollbackFail
	default:
		return false
	}
}
