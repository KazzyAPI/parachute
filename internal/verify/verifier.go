package verify

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/txn"
	dockerspec "github.com/moby/docker-image-spec/specs-go/v1"
)

const (
	checkHealth = "health"
	checkStable = "stable"
	checkCmd    = "parachute.verify.cmd"
	checkURL    = "parachute.verify.url"
	retryEvery  = 5 * time.Second
	pollEvery   = time.Second
	execLimit   = 30 * time.Second
	maxOutput   = 1000
)

type Verifier struct {
	inspector Inspector
	commander Commander
	prober    URLProber
	labels    LabelReader
	settings  Settings
	clock     Clock
}

func New(inspector Inspector, commander Commander, prober URLProber, labels LabelReader, settings Settings, clock Clock) *Verifier {
	return &Verifier{
		inspector: inspector,
		commander: commander,
		prober:    prober,
		labels:    labels,
		settings:  settings,
		clock:     clock,
	}
}

func (v *Verifier) Verify(ctx context.Context, plan txn.Plan, containerID string) (txn.Verdict, error) {
	spec, err := v.inspector.Inspect(ctx, containerID)
	if err != nil {
		return txn.Verdict{}, err
	}
	labels := labelMap(spec)
	runtime, err := v.inspector.Runtime(ctx, containerID)
	if err != nil {
		return txn.Verdict{}, err
	}
	if runtime.Status == "created" {
		return txn.Verdict{Passed: true}, nil
	}
	timeout := v.labels.Duration(containerID, labels, "verify.timeout", v.settings.Timeout)
	stable := v.labels.Duration(containerID, labels, "verify.stable", v.settings.Stable)
	health := mergedHealth(spec)
	deadline := EffectiveDeadline(timeout, health)
	deadlineAt := v.deadlineStart(runtime).Add(deadline)
	if err := v.baseCheck(ctx, containerID, runtime, health, stable, deadlineAt); err != nil {
		return failVerdict(err), nil
	}
	if cmd, ok := v.labels.String(containerID, labels, "verify.cmd"); ok {
		if verdict := v.retryCheck(ctx, containerID, deadlineAt, checkCmd, func(ctx context.Context) (bool, string) {
			result, err := v.commander.Exec(ctx, containerID, []string{"sh", "-c", cmd}, nil, execLimit, maxOutput)
			if err != nil {
				return false, err.Error()
			}
			return result.ExitCode == 0, string(result.Output)
		}); !verdict.Passed {
			return verdict, nil
		}
	}
	if url, ok := v.labels.String(containerID, labels, "verify.url"); ok {
		if v.prober == nil {
			return txn.Verdict{Passed: false, Reason: ReasonFailed, Check: checkURL, Output: "url probe unavailable"}, nil
		}
		if verdict := v.retryCheck(ctx, containerID, deadlineAt, checkURL, func(ctx context.Context) (bool, string) {
			status, _, err := v.prober.Probe(ctx, plan.ID, containerID, url)
			if err != nil {
				return false, err.Error()
			}
			if status >= 200 && status < 400 {
				return true, fmt.Sprintf("%d", status)
			}
			return false, fmt.Sprintf("%d", status)
		}); !verdict.Passed {
			return verdict, nil
		}
	}
	return txn.Verdict{Passed: true}, nil
}

func (v *Verifier) deadlineStart(runtime engine.Runtime) time.Time {
	if runtime.StartedAt.IsZero() {
		return v.clock.Now()
	}
	return runtime.StartedAt
}

func (v *Verifier) baseCheck(ctx context.Context, containerID string, initial engine.Runtime, health *dockerspec.HealthcheckConfig, stable time.Duration, deadlineAt time.Time) error {
	if health != nil && len(health.Test) > 0 && health.Test[0] != "NONE" {
		return v.healthBase(ctx, containerID, deadlineAt)
	}
	return v.stableBase(ctx, containerID, initial, stable, deadlineAt)
}

func (v *Verifier) healthBase(ctx context.Context, containerID string, deadlineAt time.Time) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if v.clock.Now().After(deadlineAt) {
			return timeoutError(checkHealth, "")
		}
		runtime, err := v.inspector.Runtime(ctx, containerID)
		if err != nil {
			return err
		}
		if !runtime.Running && runtime.Status != "running" {
			output := ""
			if runtime.Health != nil {
				output = runtime.Health.Log
			}
			return failedError(checkHealth, output)
		}
		if runtime.Health != nil {
			switch runtime.Health.Status {
			case "healthy":
				return nil
			case "unhealthy":
				return failedError(checkHealth, runtime.Health.Log)
			}
		}
		if err := v.clock.Sleep(ctx, pollEvery); err != nil {
			return err
		}
	}
}

func (v *Verifier) stableBase(ctx context.Context, containerID string, initial engine.Runtime, stable time.Duration, deadlineAt time.Time) error {
	baseline := initial.RestartCount
	stableSince := time.Time{}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if v.clock.Now().After(deadlineAt) {
			return timeoutError(checkStable, "")
		}
		runtime, err := v.inspector.Runtime(ctx, containerID)
		if err != nil {
			return err
		}
		if !runtime.Running || runtime.RestartCount != baseline {
			return failedError(checkStable, "")
		}
		if stableSince.IsZero() {
			stableSince = v.clock.Now()
		}
		if v.clock.Now().Sub(stableSince) >= stable {
			return nil
		}
		if err := v.clock.Sleep(ctx, pollEvery); err != nil {
			return err
		}
	}
}

func (v *Verifier) retryCheck(ctx context.Context, containerID string, deadlineAt time.Time, check string, attempt func(context.Context) (bool, string)) txn.Verdict {
	var lastOutput string
	attempted := false
	for {
		if err := ctx.Err(); err != nil {
			return txn.Verdict{Passed: false, Reason: ReasonFailed, Check: check, Output: err.Error()}
		}
		if v.clock.Now().After(deadlineAt) {
			reason := ReasonTimeout
			if attempted {
				reason = ReasonFailed
			}
			return txn.Verdict{Passed: false, Reason: reason, Check: check, Output: truncate(lastOutput)}
		}
		ok, output := attempt(ctx)
		attempted = true
		lastOutput = output
		if ok {
			return txn.Verdict{Passed: true}
		}
		if err := v.clock.Sleep(ctx, retryEvery); err != nil {
			return txn.Verdict{Passed: false, Reason: ReasonFailed, Check: check, Output: err.Error()}
		}
	}
}

func labelMap(spec engine.Spec) map[string]string {
	if spec.Config != nil && spec.Config.Labels != nil {
		return spec.Config.Labels
	}
	return map[string]string{}
}

func mergedHealth(spec engine.Spec) *dockerspec.HealthcheckConfig {
	if spec.Config != nil && spec.Config.Healthcheck != nil {
		return spec.Config.Healthcheck
	}
	if spec.ImageConfig != nil && spec.ImageConfig.Healthcheck != nil {
		return spec.ImageConfig.Healthcheck
	}
	return nil
}

type checkError struct {
	timeout bool
	check   string
	output  string
}

func (e checkError) Error() string { return e.check }

func timeoutError(check, output string) error {
	return checkError{timeout: true, check: check, output: output}
}

func failedError(check, output string) error {
	return checkError{timeout: false, check: check, output: output}
}

func failVerdict(err error) txn.Verdict {
	if err == nil {
		return txn.Verdict{Passed: true}
	}
	var check checkError
	if errors.As(err, &check) {
	} else {
		return txn.Verdict{Passed: false, Reason: ReasonFailed, Check: checkHealth, Output: err.Error()}
	}
	reason := ReasonFailed
	if check.timeout {
		reason = ReasonTimeout
	}
	return txn.Verdict{Passed: false, Reason: reason, Check: check.check, Output: truncate(check.output)}
}

func truncate(text string) string {
	if len(text) <= maxOutput {
		return text
	}
	return text[:maxOutput]
}
