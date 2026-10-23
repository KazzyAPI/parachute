package api

import (
	"bytes"
	"crypto/hmac"
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
	key := []byte("per-process-key")
	tokens := newTokenSet(key, admin, statusToken)
	if reflect.ValueOf(tokens.compare).Pointer() != reflect.ValueOf(subtle.ConstantTimeCompare).Pointer() {
		t.Fatal("tokens are not compared with subtle.ConstantTimeCompare")
	}
	var calls []recordedComparison
	tokens.compare = func(x, y []byte) int {
		calls = append(calls, recordedComparison{presented: bytes.Clone(x), configured: bytes.Clone(y)})
		return subtle.ConstantTimeCompare(x, y)
	}
	digest := func(token string) []byte {
		mac := hmac.New(sha256.New, key)
		mac.Write([]byte(token))
		return mac.Sum(nil)
	}
	adminDigest, statusDigest := digest(admin), digest(statusToken)
	for _, tc := range []struct {
		presented string
		want      tokenMatch
	}{
		{admin, tokenMatch{Admin: true}},
		{statusToken, tokenMatch{Status: true}},
		{admin[:63] + "Y", tokenMatch{}},
		{statusToken[:len(statusToken)-1] + "X", tokenMatch{}},
		{admin[:10], tokenMatch{}},
		{admin + "a", tokenMatch{}},
		{statusToken + "x", tokenMatch{}},
		{strings.Repeat("-", 64), tokenMatch{}},
		{"", tokenMatch{}},
	} {
		calls = nil
		if got := tokens.match(tc.presented); got != tc.want {
			t.Errorf("Match(%q) = %+v, want %+v", tc.presented, got, tc.want)
		}
		presentedDigest := digest(tc.presented)
		if len(calls) != 2 {
			t.Fatalf("Match(%q) made %d comparisons, want 2", tc.presented, len(calls))
		}
		for index, configured := range [][]byte{adminDigest, statusDigest} {
			if !bytes.Equal(calls[index].presented, presentedDigest) || !bytes.Equal(calls[index].configured, configured) {
				t.Errorf("Match(%q) comparison %d did not compare HMAC-SHA256 digests", tc.presented, index)
			}
		}
	}
}

func TestEmptyStatusTokenMatchesNothing(t *testing.T) {
	if got := newTokenSet([]byte("key"), "admin-token", "").match(""); got != (tokenMatch{}) {
		t.Errorf("empty presented token matched %+v", got)
	}
}
