package app

import (
	"github.com/KazzyAPI/parachute/internal/approval"
	"github.com/KazzyAPI/parachute/internal/config"
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/policy"
	"github.com/KazzyAPI/parachute/internal/state"
)

func (c *Container) policyStage(store *state.Store) (pipeline.Stage, error) {
	settings, location := c.cfg.Policy, c.cfg.Schedule.Location
	window, err := policy.NewWindowRule(settings.Window, settings.WindowDuration, location)
	if err != nil {
		return nil, configurationError(err)
	}
	databases := policy.NewDatabases()
	evaluator := policy.NewEvaluator(
		policy.NewLabels(c.labels, config.NewSyntax(), location),
		policy.NewApprovals(store, c.logger),
		policy.NewRelaxations(databases, c.logger),
		policy.NewMonitorRule(c.labels, c.cfg.Selection.MonitorOnly, c.cfg.Selection.LabelTakePrecedence),
		policy.NewSelfRule(),
		policy.NewApprovalRule(databases, settings.ProtectDatabases),
		policy.NewDelayRule(store, settings.DelayDays),
		window,
	)
	return pipeline.NewPolicyStage(evaluator, policy.NewRecords(store, c.logger), c.deps.Clock), nil
}

func (c *Container) Approver(store *state.Store, hosts []pipeline.Host) *approval.Service {
	sources := map[string]approval.ContainerSource{}
	for _, host := range hosts {
		sources[host.Name] = host.Source
	}
	return approval.NewService(store, sources, c.deps.Clock, c.logger)
}
