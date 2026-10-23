package app

import (
	"github.com/KazzyAPI/parachute/internal/approval"
	"github.com/KazzyAPI/parachute/internal/config"
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/policy"
	"github.com/KazzyAPI/parachute/internal/state"
)

func (c *Container) policyStage(store *state.Store) pipeline.Stage {
	settings, location := c.cfg.Policy, c.cfg.Schedule.Location
	for _, relaxation := range c.cfg.Relaxations {
		c.logger.Warn("safety default relaxed", "setting", relaxation.Setting, "value", relaxation.Value)
	}
	databases := policy.NewDatabases()
	evaluator := policy.NewEvaluator(
		policy.NewLabels(config.NewSyntax(), location),
		policy.NewApprovals(store, c.logger),
		policy.NewRelaxations(databases, c.logger),
		policy.NewMonitorRule(c.labels, c.cfg.Selection.MonitorOnly, c.cfg.Selection.LabelTakePrecedence),
		policy.NewSelfRule(),
		policy.NewApprovalRule(databases, settings.ProtectDatabases),
		policy.NewDelayRule(store, settings.DelayDays),
		policy.NewWindowRule(settings.Window, settings.WindowDuration, location),
	)
	return pipeline.NewPolicyStage(evaluator, c.deps.Clock)
}

func (c *Container) Approver(store *state.Store, hosts []pipeline.Host) *approval.Service {
	sources := map[string]approval.ContainerSource{}
	for _, host := range hosts {
		sources[host.Name] = host.Source
	}
	return approval.NewService(store, sources, c.deps.Clock, c.logger)
}
