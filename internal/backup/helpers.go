package backup

import (
	"context"
	"slices"
)

func CleanupHelpers(ctx context.Context, engine Engine, allowedTransactions []string) error {
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
		if spec.Config != nil && spec.Config.Labels != nil {
			txnID = spec.Config.Labels["parachute.transaction"]
			if spec.Config.Labels["parachute.restore-verify"] != "" {
				_ = engine.Remove(ctx, id, false, true)
				continue
			}
		}
		if txnID == "" || !slices.Contains(allowedTransactions, txnID) {
			continue
		}
		_ = engine.Remove(ctx, id, false, true)
	}
	return nil
}
