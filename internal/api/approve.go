package api

import (
	"context"
	"net/http"
	"net/url"
	"slices"
)

const defaultHost = "local"

type ApproveRequest struct {
	Host      string
	Container string
	Stack     string
}

type Approver interface {
	Approve(ctx context.Context, request ApproveRequest) (status int, body any, err error)
}

func (s *Server) approve(w http.ResponseWriter, r *http.Request) {
	request, problem := s.approveRequest(r.URL.RawQuery)
	if problem != "" {
		s.respond.json(w, http.StatusBadRequest, errorBody{Error: problem})
		return
	}
	if !slices.Contains(s.parts.Settings.Hosts, request.Host) {
		s.respond.json(w, http.StatusNotFound, errorBody{Error: "unknown host: " + request.Host})
		return
	}
	code, body, err := s.parts.Approver.Approve(r.Context(), request)
	if err != nil {
		s.respond.json(w, http.StatusInternalServerError, errorBody{Error: err.Error()})
		return
	}
	s.respond.json(w, code, body)
}

func (s *Server) approveRequest(rawQuery string) (ApproveRequest, string) {
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return ApproveRequest{}, "invalid query"
	}
	for name, values := range query {
		if !slices.Contains([]string{"container", "stack", "host"}, name) {
			return ApproveRequest{}, "unknown parameter: " + name
		}
		if len(values) != 1 || values[0] == "" {
			return ApproveRequest{}, "parameter " + name + " must be given once with a value"
		}
	}
	request := ApproveRequest{Host: query.Get("host"), Container: query.Get("container"), Stack: query.Get("stack")}
	if (request.Container == "") == (request.Stack == "") {
		return ApproveRequest{}, "name exactly one of container and stack"
	}
	if request.Host == "" {
		if len(s.parts.Settings.Hosts) > 1 {
			return ApproveRequest{}, "host is required when more than one host is configured"
		}
		request.Host = defaultHost
	}
	return request, ""
}
