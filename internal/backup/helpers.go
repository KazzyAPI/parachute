package backup

import (
	"context"
	"slices"
)

func CleanupHelpers(ctx context.Context, engine Engine, allowedTransactions, allowedRestoreVerify []string) error {
	ids, err := engine.Helpers(ctx, map[string][]string{"label": {"parachute.helper=true"}})
	if err != nil {
		return err
	}
	for _, id := range ids {
		spec, err := engine.Inspect(ctx, id)
		if err != nil {
			continue
		}
		txnID := ""
		runID := ""
		if spec.Config != nil && spec.Config.Labels != nil {
			txnID = spec.Config.Labels["parachute.transaction"]
			runID = spec.Config.Labels["parachute.restore-verify"]
		}
		switch {
		case runID != "" && slices.Contains(allowedRestoreVerify, runID):
			_ = engine.Remove(ctx, id, false, true)
		case txnID != "" && slices.Contains(allowedTransactions, txnID):
			_ = engine.Remove(ctx, id, false, true)
		}
	}
	return nil
}
