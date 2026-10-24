package hosts

import (
	"testing"

	"github.com/KazzyAPI/parachute/internal/domain"
)

func TestValidateTransactionsRejectsOneIDAcrossHosts(t *testing.T) {
	results := []domain.Result{
		{Host: "a", Stack: "shop", Transaction: "txn-20260101T000000Z-aaaaaa"},
		{Host: "b", Stack: "shop", Transaction: "txn-20260101T000000Z-aaaaaa"},
	}
	if err := ValidateTransactions(results); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateTransactionsAllowsDistinctHosts(t *testing.T) {
	results := []domain.Result{
		{Host: "a", Stack: "shop", Transaction: "txn-20260101T000000Z-aaaaaa"},
		{Host: "b", Stack: "shop", Transaction: "txn-20260101T000001Z-bbbbbb"},
	}
	if err := ValidateTransactions(results); err != nil {
		t.Fatal(err)
	}
}
