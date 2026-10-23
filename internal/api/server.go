package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/state"
	"github.com/KazzyAPI/parachute/internal/status"
)

const (
	headerLimit        = 8 << 10
	netHTTPHeaderSlack = 4096
	headerTimeout      = 10 * time.Second
	shutdownTimeout    = 10 * time.Second
	idleTimeout        = 60 * time.Second
)

type Gate interface {
	Run(ctx context.Context, run func(ctx context.Context, id string) domain.Report) (domain.Report, error)
}

type CycleRunner interface {
	Run(ctx context.Context, id string) domain.Report
}

type StateReader interface {
	Current() state.Document
}

type StatusSource interface {
	Build(ctx context.Context, saved state.Document, generated time.Time) (status.Document, error)
}

type HTMLWriter interface {
	Write(out io.Writer, document status.Document) error
}

type MetricsWriter interface {
	Write(out io.Writer) error
}

type Clock interface {
	Now() time.Time
}

type Redactor interface {
	Redact(text string) string
}

type Settings struct {
	Port           int
	Update         bool
	Metrics        bool
	Status         bool
	Token          string
	StatusToken    string
	TrustedProxies []netip.Prefix
	Hosts          []string
}

type Parts struct {
	Settings Settings
	TokenKey []byte
	Gate     Gate
	Runner   CycleRunner
	State    StateReader
	Status   StatusSource
	HTML     HTMLWriter
	Metrics  MetricsWriter
	Approver Approver
	Clock    Clock
	Redactor Redactor
	Logger   *slog.Logger
}

type Server struct {
	parts    Parts
	respond  responder
	tokens   *tokenSet
	throttle *failureThrottle
	sources  sourceResolver
}

func NewServer(parts Parts) *Server {
	return &Server{
		parts:    parts,
		respond:  responder{redactor: parts.Redactor},
		tokens:   newTokenSet(parts.TokenKey, parts.Settings.Token, parts.Settings.StatusToken),
		throttle: newFailureThrottle(parts.Clock, parts.Logger),
		sources:  sourceResolver{trusted: parts.Settings.TrustedProxies},
	}
}

func (s *Server) Serve(ctx context.Context) error {
	listener, err := net.Listen("tcp", ":"+strconv.Itoa(s.parts.Settings.Port))
	if err != nil {
		return fmt.Errorf("listen for the HTTP API: %w", err)
	}
	server := &http.Server{
		Handler: s.handler(ctx), ReadHeaderTimeout: headerTimeout, IdleTimeout: idleTimeout,
		MaxHeaderBytes: headerLimit - netHTTPHeaderSlack, ErrorLog: slog.NewLogLogger(s.parts.Logger.Handler(), slog.LevelDebug),
	}
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()
	select {
	case err := <-served:
		return fmt.Errorf("serve the HTTP API: %w", err)
	case <-ctx.Done():
	}
	stopping, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	shutdownErr := server.Shutdown(stopping)
	if shutdownErr != nil {
		shutdownErr = errors.Join(shutdownErr, server.Close())
	}
	if err := <-served; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve the HTTP API: %w", err)
	}
	if shutdownErr != nil {
		return fmt.Errorf("stop the HTTP API: %w", shutdownErr)
	}
	return nil
}

func (s *Server) handler(ctx context.Context) http.Handler {
	routes := http.NewServeMux()
	routes.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		s.respond.json(w, http.StatusNotFound, errorBody{Error: "not found"})
	})
	settings := s.parts.Settings
	if settings.Update {
		routes.HandleFunc("/v1/update", s.allow(s.update(ctx), http.MethodGet, http.MethodPost))
		routes.HandleFunc("/v1/approve", s.allow(s.approve, http.MethodPost))
	}
	if settings.Metrics {
		routes.HandleFunc("/v1/metrics", s.allow(s.metrics, http.MethodGet))
	}
	if settings.Status {
		routes.HandleFunc("/v1/status", s.allow(s.statusJSON, http.MethodGet))
		routes.HandleFunc("/status", s.allow(s.statusHTML, http.MethodGet))
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.withinBodyLimit(w, r) {
			s.respond.json(w, http.StatusRequestEntityTooLarge, errorBody{Error: "request body too large"})
			return
		}
		source := s.sources.of(r)
		if retryAfter, throttled := s.throttle.reserve(source); throttled {
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			s.respond.json(w, http.StatusTooManyRequests, errorBody{Error: "too many failed authentications"})
			return
		}
		statusEndpoint := isStatusEndpoint(r)
		allowed := s.tokens.allows(r, statusEndpoint)
		s.throttle.release(source, !allowed)
		if !allowed {
			w.Header().Add("WWW-Authenticate", `Bearer realm="parachute"`)
			if statusEndpoint {
				w.Header().Add("WWW-Authenticate", `Basic realm="parachute"`)
			}
			s.respond.json(w, http.StatusUnauthorized, errorBody{Error: "unauthorized"})
			return
		}
		routes.ServeHTTP(w, r)
	})
}

func isStatusEndpoint(r *http.Request) bool {
	return r.Method == http.MethodGet && (r.URL.Path == "/status" || r.URL.Path == "/v1/status")
}

func (s *Server) allow(handler http.HandlerFunc, methods ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !slices.Contains(methods, r.Method) {
			w.Header().Set("Allow", strings.Join(methods, ", "))
			s.respond.json(w, http.StatusMethodNotAllowed, errorBody{Error: "method not allowed"})
			return
		}
		handler(w, r)
	}
}
