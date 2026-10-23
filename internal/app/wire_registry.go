package app

import (
	"net/http"
	"time"

	"github.com/KazzyAPI/parachute/internal/config"
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/redact"
	"github.com/KazzyAPI/parachute/internal/registry"
)

func (c *Container) userAgent() string { return "parachute/" + c.deps.Version }

func (c *Container) registryClient() *registry.Client {
	httpClient := &http.Client{Timeout: 30 * time.Second, CheckRedirect: registry.CheckRedirect}
	authenticator := registry.NewAuthenticator(httpClient, c.userAgent())
	headLog := registry.NewHeadFailureLog(c.cfg.Detection.WarnOnHeadFailure, c.logger)
	return registry.NewClient(httpClient, authenticator, c.userAgent(), headLog)
}

func (c *Container) detectStage() pipeline.Stage {
	return pipeline.NewDetectStage(
		registry.NewCredentialStore(c.cfg.Detection.DockerConfigDir, c.deps.System), c.labels,
		pipeline.SwitchSettings{Global: c.cfg.Selection.NoPull, LabelWins: c.cfg.Selection.LabelTakePrecedence}, c.logger)
}

func registrySecrets(cfg config.Config, files registry.FileReader) redact.Source {
	return registry.NewCredentialStore(cfg.Detection.DockerConfigDir, files)
}
