package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/KazzyAPI/parachute/internal/api"
	"github.com/KazzyAPI/parachute/internal/metrics"
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/schedule"
	"github.com/KazzyAPI/parachute/internal/state"
	"github.com/KazzyAPI/parachute/internal/status"
)

type Server interface {
	Serve(ctx context.Context) error
}

type ServerParts struct {
	Gate    *schedule.Gate
	Runner  CycleRunner
	Store   *state.Store
	Status  status.Source
	Hosts   []pipeline.Host
	Metrics *metrics.Collector
}

type idleServer struct{}

func (idleServer) Serve(context.Context) error { return nil }

type requestsOnly struct{}

func (requestsOnly) Run(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

type pendingApprover struct{}

func (pendingApprover) Approve(context.Context, api.ApproveRequest) (int, any, error) {
	return http.StatusNotFound, map[string]string{"error": "no held update or freeze for this container or stack"}, nil
}

func (c *Container) apiTriggered() bool {
	if !c.cfg.API.Update || c.cfg.API.PeriodicPolls {
		return false
	}
	if c.cfg.Schedule.Explicit {
		setting := "--interval"
		if c.cfg.Schedule.Cron != "" {
			setting = "--schedule"
		}
		c.logger.Warn(setting+" is ignored: with --http-api-update cycles run only on API requests unless --http-api-periodic-polls is on", "setting", setting)
	}
	return true
}

func (c *Container) server(parts ServerParts) (Server, error) {
	settings := c.cfg.API
	trusted, err := api.ParseTrustedProxies(settings.TrustedProxies)
	if err != nil {
		return nil, configurationError(err)
	}
	if !settings.Update && !settings.Metrics && !settings.Status {
		return idleServer{}, nil
	}
	if settings.Token == "" {
		return nil, configurationError(errors.New("--http-api-token is required when an HTTP API endpoint is enabled"))
	}
	tokenKey := make([]byte, sha256.Size)
	if _, err := io.ReadFull(c.deps.Random, tokenKey); err != nil {
		return nil, fmt.Errorf("generate the HTTP API token key: %w", err)
	}
	hosts := make([]string, 0, len(parts.Hosts))
	for _, host := range parts.Hosts {
		hosts = append(hosts, host.Name)
	}
	return api.NewServer(api.Parts{
		Settings: api.Settings{
			Port: settings.Port, Update: settings.Update, Metrics: settings.Metrics, Status: settings.Status,
			Token: settings.Token, StatusToken: settings.StatusToken, TrustedProxies: trusted, Hosts: hosts,
		},
		TokenKey: tokenKey, Gate: parts.Gate, Runner: parts.Runner, State: parts.Store, Status: parts.Status, HTML: status.NewHTML(),
		Metrics: parts.Metrics, Approver: pendingApprover{}, Clock: c.deps.Clock, Redactor: c.redactor, Logger: c.logger,
	}), nil
}
