package app

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/KazzyAPI/parachute/internal/engine"
)

type urlProber struct {
	docker      *engine.Docker
	helperImage string
}

func (p *urlProber) Probe(ctx context.Context, transactionID, containerID, url string) (int, string, error) {
	if p.helperImage == "" {
		return 0, "", fmt.Errorf("helper-image is not set")
	}
	id, err := p.docker.RunHelper(ctx, engine.HelperSpec{
		Image:       p.helperImage,
		Command:     []string{"sleep", "300"},
		NetworkMode: "container:" + containerID,
		Labels:      map[string]string{"parachute.helper": "true", "parachute.transaction": transactionID},
	})
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = p.docker.Remove(context.WithoutCancel(ctx), id, false, true) }()
	result, err := p.docker.Exec(ctx, id, []string{"sh", "-c", "wget -S -O /dev/null " + shellQuote(url) + " 2>&1"}, nil, 30*time.Second, 1000)
	if err != nil {
		return 0, "", err
	}
	output := strings.TrimSpace(string(result.Output))
	status, err := probeStatus(result.ExitCode, output)
	return status, output, err
}

func probeStatus(exitCode int, output string) (int, error) {
	status := parseHTTPStatus(output)
	switch {
	case status != 0:
		return status, nil
	case exitCode == 0:
		return http.StatusOK, nil
	}
	return 0, fmt.Errorf("probe failed: %s", output)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

var httpStatusLine = regexp.MustCompile(`HTTP/\S+\s+(\d{3})`)

func parseHTTPStatus(output string) int {
	matches := httpStatusLine.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return 0
	}
	code, err := strconv.Atoi(matches[len(matches)-1][1])
	if err != nil {
		return 0
	}
	return code
}
