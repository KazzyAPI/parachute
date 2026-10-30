package graph

import (
	"context"
	"errors"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/recreate"
	"github.com/KazzyAPI/parachute/internal/txn"
)

var errDeferred = errors.New("hook deferred")

func preUpdateHookFailure() error {
	return recreate.Failure{State: string(domain.StateFailed), Reason: "pre_update_hook_failed", Detail: "pre-update hook failed"}
}

func (e *Executor) runPreUpdateHooks(ctx context.Context, group Group, edges []Edge, ceiling *txn.Ceiling) error {
	names := MemberNames(group.Members)
	for _, name := range StopOrder(names, edges) {
		member := memberOf(group, name)
		if member == nil || (!member.Update && !member.Recreate) {
			continue
		}
		outcome := e.hooks.Run(ctx, txn.HookPreUpdate, member.Container)
		if outcome.Deferred {
			return errDeferred
		}
		if outcome.Failed() {
			if ceiling != nil && ceiling.Expired() {
				return timeoutFailure()
			}
			return preUpdateHookFailure()
		}
		if ceiling != nil && ceiling.Expired() {
			return timeoutFailure()
		}
	}
	return nil
}

func (e *Executor) runPostUpdateHook(ctx context.Context, container domain.Container) {
	e.hooks.Run(ctx, txn.HookPostUpdate, container)
}

func (e *Executor) runPostChecks(ctx context.Context, candidates []*pipeline.Candidate) {
	for _, candidate := range candidates {
		e.hooks.Run(ctx, txn.HookPostCheck, candidate.Container)
	}
}

func transactionCandidates(group Group, candidates []*pipeline.Candidate) []*pipeline.Candidate {
	var members []*pipeline.Candidate
	for _, member := range group.Members {
		for _, candidate := range candidates {
			if candidate.Container.Name == member.Container.Name {
				members = append(members, candidate)
				break
			}
		}
	}
	return members
}

func (e *Executor) abortDeferred(ctx context.Context, id string, affected []*pipeline.Candidate) {
	for _, candidate := range affected {
		candidate.Result = candidate.Result.Conclude(domain.StateHeld, "hook_deferred", "")
		candidate.Result.Transaction = id
	}
	_ = e.journal.Finish(ctx, id, txn.Outcome{State: txn.StateAborted, Result: domain.StateHeld, Reason: "hook_deferred"})
	e.runPostChecks(ctx, affected)
}

func (e *Executor) abortPreUpdateHook(ctx context.Context, id string, affected []*pipeline.Candidate, err error) {
	e.abort(ctx, id, affected, err)
}
