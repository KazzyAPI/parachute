package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/KazzyAPI/parachute/internal/detect"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/pipeline"
)

const localHost = "local"

func (c *Container) hosts(ctx context.Context) ([]pipeline.Host, func() error, error) {
	docker, err := c.verifiedDocker(ctx)
	if err != nil {
		return nil, nil, err
	}
	source := engine.NewRetrying(docker, c.deps.Clock, engine.RetryPolicy{
		FirstDelay: time.Second, MaxDelay: 30 * time.Second, Window: 5 * time.Minute, AttemptTimeout: time.Minute,
	}, c.logger)
	host := pipeline.Host{Name: localHost, Endpoint: c.cfg.Engine.Host, Source: source, Detector: detect.NewDetector(source, c.registryClient())}
	return []pipeline.Host{host}, docker.Close, nil
}

func (c *Container) verifiedDocker(ctx context.Context) (*engine.Docker, error) {
	endpoint := engine.Endpoint{
		Host: c.cfg.Engine.Host, APIVersion: c.cfg.Engine.APIVersion, TLSVerify: c.cfg.Engine.TLSVerify,
		CertPath: c.cfg.Engine.CertPath, UserAgent: c.userAgent(),
	}
	if _, err := endpoint.Scheme(); err != nil {
		return nil, configurationError(err)
	}
	if endpoint.Plaintext() {
		c.logger.Warn("Docker host uses tcp:// without TLS verification; anyone who can reach this port controls the host", "host", endpoint.Host)
	}
	docker, err := engine.NewDocker(endpoint)
	if err != nil {
		return nil, unreachableError(err)
	}
	if err := docker.Verify(ctx); err != nil {
		reachErr := fmt.Errorf("no managed host is reachable: %s: %w", endpoint.Host, err)
		return nil, unreachableError(errors.Join(reachErr, docker.Close()))
	}
	return docker, nil
}
