package api

import (
	"encoding/json"
	"io"
	"net/http"
	"time"
)

const (
	bodyLimit   = 1 << 10
	bodyTimeout = 10 * time.Second
)

type errorBody struct {
	Error string `json:"error"`
}

type responder struct {
	redactor Redactor
}

func (p responder) json(w http.ResponseWriter, code int, body any) {
	encoded, err := json.Marshal(body)
	if err != nil {
		code, encoded = http.StatusInternalServerError, []byte(`{"error":"cannot encode the response"}`)
	}
	p.write(w, code, "application/json", string(encoded)+"\n")
}

func (p responder) write(w http.ResponseWriter, code int, contentType, body string) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(code)
	_, _ = io.WriteString(w, p.redactor.Redact(body))
}

func (s *Server) withinBodyLimit(w http.ResponseWriter, r *http.Request) bool {
	if r.ContentLength > bodyLimit {
		return false
	}
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(s.parts.Clock.Now().Add(bodyTimeout))
	read, _ := io.Copy(io.Discard, io.LimitReader(r.Body, bodyLimit+1))
	_ = controller.SetReadDeadline(time.Time{})
	return read <= bodyLimit
}
