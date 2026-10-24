package approval

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/state"
)

const defaultHost = "local"

type ApproveRequest struct {
	Host      string
	Container string
	Stack     string
}

type Approved struct {
	Host      string `json:"host"`
	Container string `json:"container"`
	Digest    string `json:"digest"`
}

type Released struct {
	Host         string   `json:"host"`
	Container    string   `json:"container,omitempty"`
	Stack        string   `json:"stack,omitempty"`
	Transactions []string `json:"transactions"`
}

type Failure struct {
	Error string `json:"error"`
}

type ContainerSource interface {
	Containers(ctx context.Context) ([]domain.Container, error)
}

type Store interface {
	Current() state.Document
	Update(change func(*state.Document) error) error
}

type Clock interface {
	Now() time.Time
}

type Service struct {
	store  Store
	hosts  map[string]ContainerSource
	clock  Clock
	logger *slog.Logger
}

func NewService(store Store, hosts map[string]ContainerSource, clock Clock, logger *slog.Logger) *Service {
	return &Service{store: store, hosts: hosts, clock: clock, logger: logger}
}

func (s *Service) Approve(ctx context.Context, request ApproveRequest) (int, any, error) {
	if (request.Container == "") == (request.Stack == "") {
		return http.StatusBadRequest, Failure{Error: "name exactly one of container and stack"}, nil
	}
	host := request.Host
	if host == "" && len(s.hosts) > 1 {
		return http.StatusBadRequest, Failure{Error: "host is required when several hosts are configured"}, nil
	}
	if host == "" {
		host = defaultHost
	}
	source, known := s.hosts[host]
	if !known {
		return http.StatusNotFound, Failure{Error: "unknown host"}, nil
	}
	if request.Stack != "" {
		return s.releaseStack(host, request.Stack)
	}
	containers, err := source.Containers(ctx)
	if err != nil {
		return 0, nil, fmt.Errorf("list containers on host %s: %w", host, err)
	}
	index := slices.IndexFunc(containers, func(c domain.Container) bool { return c.Name == request.Container })
	if index < 0 {
		return http.StatusNotFound, Failure{Error: "unknown container"}, nil
	}
	container := containers[index]
	covering := s.freezes(func(freeze state.Freeze) bool { return freeze.Covers(host, container) })
	if len(covering) > 0 {
		return s.release(covering, Released{Host: host, Container: container.Name})
	}
	digest, held := s.heldDigest(host, container)
	if !held {
		return http.StatusNotFound, Failure{Error: "no update is held for approval"}, nil
	}
	if err := s.store.Update(func(document *state.Document) error {
		document.Approvals = append(slices.DeleteFunc(document.Approvals, func(record state.Approval) bool {
			return record.Host == host && record.Container == container.ID
		}), state.Approval{Host: host, Container: container.ID, Digest: digest, Time: s.clock.Now().UTC()})
		return nil
	}); err != nil {
		return 0, nil, fmt.Errorf("store approval for %s: %w", container.Name, err)
	}
	s.logger.Info("update approved", "host", host, "container", container.Name, "digest", digest)
	return http.StatusAccepted, Approved{Host: host, Container: container.Name, Digest: digest}, nil
}

func (s *Service) heldDigest(host string, container domain.Container) (string, bool) {
	for _, result := range s.store.Current().Results {
		if result.Host == host && result.ContainerID == container.ID && result.State == domain.StateHeld &&
			result.CandidateDigest != "" && (result.Reason == "approval_required" || result.Reason == "previous_attempt_failed") {
			return result.CandidateDigest, true
		}
	}
	return "", false
}

func (s *Service) freezes(match func(state.Freeze) bool) []state.Freeze {
	return slices.DeleteFunc(s.store.Current().ActiveFreezes(), func(freeze state.Freeze) bool { return !match(freeze) })
}

func (s *Service) releaseStack(host, stack string) (int, any, error) {
	covering := s.freezes(func(freeze state.Freeze) bool { return freeze.HasStack(host, stack) })
	if len(covering) == 0 {
		return http.StatusNotFound, Failure{Error: "no freeze for this stack"}, nil
	}
	return s.release(covering, Released{Host: host, Stack: stack})
}

func (s *Service) release(freezes []state.Freeze, body Released) (int, any, error) {
	now := s.clock.Now().UTC()
	body.Transactions = []string{}
	if err := s.store.Update(func(document *state.Document) error {
		for _, freeze := range freezes {
			document.Releases = append(document.Releases, state.Release{Transaction: freeze.Transaction, Keys: slices.Clone(freeze.Keys), Time: now})
		}
		return nil
	}); err != nil {
		return 0, nil, fmt.Errorf("store release: %w", err)
	}
	for _, freeze := range freezes {
		if !slices.Contains(body.Transactions, freeze.Transaction) {
			body.Transactions = append(body.Transactions, freeze.Transaction)
		}
	}
	s.logger.Info("freeze released", "host", body.Host, "container", body.Container, "stack", body.Stack, "transactions", body.Transactions)
	return http.StatusOK, body, nil
}
