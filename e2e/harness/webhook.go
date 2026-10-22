package harness

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"
)

type Notification struct {
	Title    string
	HasTitle bool
	Message  string
	Received time.Time
}

type Webhook struct {
	env      *Env
	server   *httptest.Server
	lock     sync.Mutex
	received []Notification
}

func (e *Env) Webhook() *Webhook {
	webhook := &Webhook{env: e}
	webhook.server = httptest.NewServer(http.HandlerFunc(webhook.record))
	e.T.Cleanup(webhook.server.Close)
	return webhook
}

func (w *Webhook) record(response http.ResponseWriter, request *http.Request) {
	body, err := io.ReadAll(request.Body)
	if err != nil {
		http.Error(response, err.Error(), http.StatusBadRequest)
		return
	}
	notification := Notification{Message: string(body), Received: time.Now()}
	var payload map[string]string
	if json.Unmarshal(body, &payload) == nil {
		notification.Message = payload["message"]
		notification.Title, notification.HasTitle = payload["title"]
	}
	w.lock.Lock()
	w.received = append(w.received, notification)
	w.lock.Unlock()
	response.WriteHeader(http.StatusNoContent)
}

func (w *Webhook) URL() string {
	return "generic://" + strings.TrimPrefix(w.server.URL, "http://") + "/hook?disabletls=yes&template=json"
}

func (w *Webhook) Received() []Notification {
	w.lock.Lock()
	defer w.lock.Unlock()
	return append([]Notification{}, w.received...)
}

func (w *Webhook) Matching(text string) []Notification {
	var matching []Notification
	for _, notification := range w.Received() {
		if strings.Contains(notification.Message, text) {
			matching = append(matching, notification)
		}
	}
	return matching
}

func (w *Webhook) Reports() []Notification {
	var reports []Notification
	for _, notification := range w.Received() {
		if !strings.HasPrefix(notification.Message, "Parachute started") {
			reports = append(reports, notification)
		}
	}
	return reports
}

func (w *Webhook) WaitFor(timeout time.Duration, count int) []Notification {
	w.env.T.Helper()
	w.env.Eventually(timeout, "webhook notifications", func() bool { return len(w.Received()) >= count })
	return w.Received()
}
