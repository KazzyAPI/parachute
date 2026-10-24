package verify

import "github.com/KazzyAPI/parachute/internal/txn"

const (
	ReasonFailed  txn.Reason = "verify_failed"
	ReasonTimeout txn.Reason = "verify_timeout"
)
