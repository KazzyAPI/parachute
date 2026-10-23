package api

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"reflect"
	"strings"
	"testing"
)

type recordedComparison struct {
	presented, configured []byte
}

func TestTokenComparisonUsesDigestsAndConstantTime(t *testing.T) {
	admin := strings.Repeat("a", 63) + "Z"
	statusToken := "read-only-token"
	tokens := NewTokens(admin, statusToken)
	if reflect.ValueOf(tokens.compare).Pointer() != reflect.ValueOf(subtle.ConstantTimeCompare).Pointer() {
		t.Fatal("tokens are not compared with subtle.ConstantTimeCompare")
	}
	var calls []recordedComparison
	tokens.compare = func(x, y []byte) int {
		calls = append(calls, recordedComparison{presented: bytes.Clone(x), configured: bytes.Clone(y)})
		return subtle.ConstantTimeCompare(x, y)
	}
	adminDigest, statusDigest := sha256.Sum256([]byte(admin)), sha256.Sum256([]byte(statusToken))
	for _, tc := range []struct {
		presented string
		want      Match
	}{
		{admin, Match{Admin: true}},
		{statusToken, Match{Status: true}},
		{admin[:63] + "Y", Match{}},
		{statusToken[:len(statusToken)-1] + "X", Match{}},
		{admin[:10], Match{}},
		{admin + "a", Match{}},
		{statusToken + "x", Match{}},
		{strings.Repeat("-", 64), Match{}},
		{"", Match{}},
	} {
		calls = nil
		if got := tokens.Match(tc.presented); got != tc.want {
			t.Errorf("Match(%q) = %+v, want %+v", tc.presented, got, tc.want)
		}
		presentedDigest := sha256.Sum256([]byte(tc.presented))
		if len(calls) != 2 {
			t.Fatalf("Match(%q) made %d comparisons, want 2", tc.presented, len(calls))
		}
		for index, configured := range [][sha256.Size]byte{adminDigest, statusDigest} {
			if !bytes.Equal(calls[index].presented, presentedDigest[:]) || !bytes.Equal(calls[index].configured, configured[:]) {
				t.Errorf("Match(%q) comparison %d did not compare SHA-256 digests", tc.presented, index)
			}
		}
	}
}

func TestEmptyStatusTokenMatchesNothing(t *testing.T) {
	if got := NewTokens("admin-token", "").Match(""); got != (Match{}) {
		t.Errorf("empty presented token matched %+v", got)
	}
}
