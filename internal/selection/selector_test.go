package selection

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/KazzyAPI/parachute/internal/config"
	"github.com/KazzyAPI/parachute/internal/domain"
)

func TestWarnUnmatchedNamesOnlyForNamesOnNoHost(t *testing.T) {
	var out bytes.Buffer
	selector := NewSelector(config.Selection{Names: []string{"web", "ghost"}}, nil, slog.New(slog.NewTextHandler(&out, nil)))
	selector.WarnUnmatchedNames([]domain.Container{{Name: "db"}, {Name: "web"}})
	if strings.Contains(out.String(), "name=web") || strings.Count(out.String(), "name=ghost") != 1 {
		t.Fatalf("log %q", out.String())
	}
}
