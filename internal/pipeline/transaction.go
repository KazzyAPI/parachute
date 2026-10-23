package pipeline

import (
	"context"
)

type TransactionStage struct {
	enabled bool
}

func NewTransactionStage(enabled bool) *TransactionStage {
	return &TransactionStage{enabled: enabled}
}

func (s *TransactionStage) Run(ctx context.Context, cycle *Cycle) error {
	if !cycle.Armed || !s.enabled {
		return nil
	}
	return nil
}
