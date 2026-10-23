package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/KazzyAPI/parachute/internal/config"
	"github.com/KazzyAPI/parachute/internal/detect"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/pipeline"
)

const (
	localHost        = "local"
	startupTimeout   = 10 * time.Second
	insecureHostText = "Docker host uses tcp:// without TLS verification; anyone who can reach this port controls the host"
)

type connectedHost struct {
	spec     config.HostSpec
	docker   *engine.Docker
	engineID string
	err      error
}

func (c *Container) hostSpecs() []config.HostSpec {
	if len(c.cfg.Hosts) > 0 {
		return c.cfg.Hosts
	}
	return []config.HostSpec{{Name: localHost, Endpoint: c.cfg.Engine.Host}}
}

func (c *Container) endpoint(spec config.HostSpec) engine.Endpoint {
	certPath := c.cfg.Engine.CertPath
	if info, err := c.deps.System.Stat(filepath.Join(certPath, spec.Name)); err == nil && info.IsDir() {
		certPath = filepath.Join(certPath, spec.Name)
	}
	return engine.Endpoint{
		Host: spec.Endpoint, APIVersion: c.cfg.Engine.APIVersion, TLSVerify: c.cfg.Engine.TLSVerify,
		CertPath: certPath, UserAgent: c.userAgent(),
	}
}

func (c *Container) hosts(ctx context.Context) ([]pipeline.Host, func() error, error) {
	if len(c.cfg.Hosts) > 0 {
		c.logger.Warn("--hosts is set: DOCKER_HOST and --host are ignored")
	}
	specs := c.hostSpecs()
	if err := c.checkEndpoints(specs); err != nil {
		return nil, nil, configurationError(err)
	}
	connected := make([]connectedHost, 0, len(specs))
	closeAll := func() error {
		var errs []error
		for _, host := range connected {
			if host.docker != nil {
				errs = append(errs, host.docker.Close())
			}
		}
		return errors.Join(errs...)
	}
	for _, spec := range specs {
		host, err := c.connect(ctx, spec)
		if err != nil {
			return nil, nil, errors.Join(unreachableError(err), closeAll())
		}
		connected = append(connected, host)
	}
	if err := checkReachable(connected); err != nil {
		return nil, nil, errors.Join(err, closeAll())
	}
	return c.pipelineHosts(connected), closeAll, nil
}

func (c *Container) checkEndpoints(specs []config.HostSpec) error {
	for _, spec := range specs {
		endpoint := c.endpoint(spec)
		if _, err := endpoint.Scheme(); err != nil {
			return err
		}
		if !endpoint.Plaintext() {
			continue
		}
		if !c.cfg.Engine.AllowInsecureTCP {
			return fmt.Errorf("host %s at %s is an insecure TCP host: enable TLS verification (--tlsverify) or set --allow-insecure-tcp", spec.Name, spec.Endpoint)
		}
		c.logger.Warn(insecureHostText, "host", spec.Name, "endpoint", spec.Endpoint)
	}
	return nil
}

func (c *Container) connect(ctx context.Context, spec config.HostSpec) (connectedHost, error) {
	docker, err := engine.NewDocker(c.endpoint(spec))
	if err != nil {
		return connectedHost{}, err
	}
	host := connectedHost{spec: spec, docker: docker}
	attempt, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	if host.err = docker.Verify(attempt); host.err == nil {
		host.engineID, host.err = docker.EngineID(attempt)
	}
	if host.err != nil {
		c.logger.Error("host unreachable", "host", spec.Name, "endpoint", spec.Endpoint, "error", host.err.Error())
	}
	return host, nil
}

func checkReachable(connected []connectedHost) error {
	names := map[string]string{}
	var failures []string
	for _, host := range connected {
		if host.err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", host.spec.Endpoint, host.err))
			continue
		}
		if other, seen := names[host.engineID]; seen {
			return configurationError(fmt.Errorf("hosts %s and %s report the same engine ID %s: one engine must be managed under one name", other, host.spec.Name, host.engineID))
		}
		names[host.engineID] = host.spec.Name
	}
	if len(names) == 0 {
		return unreachableError(fmt.Errorf("no managed host is reachable: %s", strings.Join(failures, "; ")))
	}
	return nil
}

func (c *Container) pipelineHosts(connected []connectedHost) []pipeline.Host {
	hosts := make([]pipeline.Host, 0, len(connected))
	for _, host := range connected {
		source := engine.NewRetrying(host.docker, c.deps.Clock, engine.RetryPolicy{
			FirstDelay: time.Second, MaxDelay: 30 * time.Second, Window: 5 * time.Minute, AttemptTimeout: time.Minute, ProbeTimeout: startupTimeout,
		}, c.logger.With("host", host.spec.Name))
		if host.err != nil {
			source.MarkUnreachable()
		}
		hosts = append(hosts, pipeline.Host{
			Name: host.spec.Name, Endpoint: host.spec.Endpoint, Source: source, Detector: detect.NewDetector(source, c.registryClient()),
		})
	}
	return hosts
}

type insecureHostWarning struct {
	hosts  []config.HostSpec
	logger *slog.Logger
}

func (c *Container) insecureHostWarning() *insecureHostWarning {
	warning := &insecureHostWarning{logger: c.logger}
	for _, spec := range c.hostSpecs() {
		if c.endpoint(spec).Plaintext() {
			warning.hosts = append(warning.hosts, spec)
		}
	}
	return warning
}

func (w *insecureHostWarning) Run(_ context.Context, cycle *pipeline.Cycle) error {
	for _, spec := range w.hosts {
		w.logger.Warn(insecureHostText, "cycle", cycle.ID, "host", spec.Name, "endpoint", spec.Endpoint)
	}
	return nil
}

type reachability interface {
	Reachable() bool
}

func firstReachable(hosts []pipeline.Host) pipeline.Host {
	for _, host := range hosts {
		if probe, ok := host.Source.(reachability); ok && probe.Reachable() {
			return host
		}
	}
	return hosts[0]
}
