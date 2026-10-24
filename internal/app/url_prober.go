package app

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/moby/moby/api/types/container"

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
	name := "parachute-url-" + transactionID + "-" + containerID[:12]
	spec := engine.Spec{
		Name: name,
		Config: &container.Config{
			Image: p.helperImage,
			Cmd:   []string{"sleep", "300"},
			Labels: map[string]string{
				"parachute.helper":      "true",
				"parachute.transaction": transactionID,
			},
		},
		HostConfig: &container.HostConfig{
			NetworkMode:    container.NetworkMode("container:" + containerID),
			ReadonlyRootfs: true,
			CapDrop:        []string{"ALL"},
			SecurityOpt:    []string{"no-new-privileges"},
			RestartPolicy:  container.RestartPolicy{Name: "no"},
			Tmpfs:          map[string]string{"/tmp": "rw"},
		},
	}
	id, err := p.docker.Create(ctx, spec)
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = p.docker.Remove(ctx, id, false, true) }()
	if err := p.docker.Start(ctx, id); err != nil {
		return 0, "", err
	}
	result, err := p.docker.Exec(ctx, id, []string{"sh", "-c", "wget -q -S -O /dev/null " + shellQuote(url) + " 2>&1"}, nil, 30*time.Second, 1000)
	if err != nil {
		return 0, "", err
	}
	status := parseHTTPStatus(string(result.Output))
	if status == 0 && result.ExitCode != 0 {
		return 0, strings.TrimSpace(string(result.Output)), fmt.Errorf("probe failed")
	}
	return status, strings.TrimSpace(string(result.Output)), nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

var httpStatusLine = regexp.MustCompile(`HTTP/\S+\s+(\d{3})`)

func parseHTTPStatus(output string) int {
	match := httpStatusLine.FindStringSubmatch(output)
	if len(match) < 2 {
		return 0
	}
	code, err := strconv.Atoi(match[1])
	if err != nil {
		return 0
	}
	return code
}
