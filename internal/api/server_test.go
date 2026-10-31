package api

import "testing"

func TestFirstParameterTreatsEmptyNameAsParameter(t *testing.T) {
	for _, query := range []string{"=x", "&=1", "a"} {
		if _, found := firstParameter(query); !found {
			t.Errorf("%q: expected a parameter", query)
		}
	}
	if _, found := firstParameter("&&"); found {
		t.Error("empty pairs are not parameters")
	}
}
