package hosts

import (
	"fmt"

	"github.com/KazzyAPI/parachute/internal/domain"
)

type StackKey struct {
	Host  string
	Stack string
}

func StackKeyOf(result domain.Result) StackKey {
	return StackKey{Host: result.Host, Stack: result.Stack}
}

func ValidateTransactions(results []domain.Result) error {
	byStack := map[StackKey]string{}
	byTransaction := map[string]StackKey{}
	for _, result := range results {
		if result.Transaction == "" {
			continue
		}
		key := StackKeyOf(result)
		if existing, ok := byStack[key]; ok && existing != result.Transaction {
			return fmt.Errorf("host %s stack %s has conflicting transaction IDs", key.Host, key.Stack)
		}
		byStack[key] = result.Transaction
		if other, ok := byTransaction[result.Transaction]; ok {
			if other != key {
				return fmt.Errorf("transaction %s spans hosts or stacks", result.Transaction)
			}
			continue
		}
		byTransaction[result.Transaction] = key
	}
	return nil
}
