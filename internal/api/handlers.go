package api

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/KazzyAPI/parachute/internal/schedule"
	"github.com/KazzyAPI/parachute/internal/status"
)

type updateBody struct {
	Cycle       string `json:"cycle"`
	Scanned     int    `json:"scanned"`
	Updated     int    `json:"updated"`
	Failed      int    `json:"failed"`
	Held        int    `json:"held"`
	WouldUpdate int    `json:"would_update"`
}

type busyBody struct {
	Running string `json:"running"`
}

func (s *Server) update(ctx context.Context) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if name, found := firstParameter(r.URL.RawQuery); found {
			s.respond.json(w, http.StatusBadRequest, errorBody{Error: "unknown parameter: " + name})
			return
		}
		report, err := s.parts.Gate.Run(ctx, s.parts.Runner.Run)
		var busy *schedule.BusyError
		switch {
		case errors.As(err, &busy):
			s.respond.json(w, http.StatusConflict, busyBody{Running: busy.Running})
			return
		case err != nil:
			s.respond.json(w, http.StatusInternalServerError, errorBody{Error: err.Error()})
			return
		}
		counts := report.Counts()
		s.respond.json(w, http.StatusOK, updateBody{
			Cycle: report.CycleID, Scanned: counts.Scanned, Updated: counts.Updated, Failed: counts.Failed,
			Held: counts.Held, WouldUpdate: counts.WouldUpdate,
		})
	}
}

func firstParameter(rawQuery string) (string, bool) {
	for pair := range strings.SplitSeq(rawQuery, "&") {
		if pair == "" {
			continue
		}
		name, _, _ := strings.Cut(pair, "=")
		if unescaped, err := url.QueryUnescape(name); err == nil {
			name = unescaped
		}
		return name, true
	}
	return "", false
}

func (s *Server) metrics(w http.ResponseWriter, _ *http.Request) {
	var text strings.Builder
	if err := s.parts.Metrics.Write(&text); err != nil {
		s.respond.json(w, http.StatusInternalServerError, errorBody{Error: err.Error()})
		return
	}
	s.respond.write(w, http.StatusOK, "text/plain; version=0.0.4; charset=utf-8", text.String())
}

func (s *Server) document(r *http.Request) (status.Document, error) {
	return s.parts.Status.Build(r.Context(), s.parts.State.Current(), s.parts.Clock.Now())
}

func (s *Server) statusJSON(w http.ResponseWriter, r *http.Request) {
	document, err := s.document(r)
	if err != nil {
		s.respond.json(w, http.StatusServiceUnavailable, errorBody{Error: err.Error()})
		return
	}
	s.respond.json(w, http.StatusOK, document)
}

func (s *Server) statusHTML(w http.ResponseWriter, r *http.Request) {
	document, err := s.document(r)
	if err != nil {
		s.respond.json(w, http.StatusServiceUnavailable, errorBody{Error: err.Error()})
		return
	}
	var page bytes.Buffer
	if err := s.parts.HTML.Write(&page, document); err != nil {
		s.respond.json(w, http.StatusInternalServerError, errorBody{Error: err.Error()})
		return
	}
	s.respond.write(w, http.StatusOK, "text/html; charset=utf-8", page.String())
}
