package harness

import (
	"encoding/json"
	"io"
	"net"
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
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		e.T.Fatal(err)
	}
	webhook.server = httptest.NewUnstartedServer(http.HandlerFunc(webhook.record))
	webhook.server.Listener = listener
	webhook.server.Start()
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

func (w *Webhook) ContainerURL() string {
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(w.server.URL, "http://"))
	return "generic://host.docker.internal:" + port + "/hook?disabletls=yes&template=json"
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

func (w *Webhook) WaitForContainer(timeout time.Duration, count int, container string) []Notification {
	w.env.T.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(w.Received()) >= count {
			return w.Received()
		}
		time.Sleep(200 * time.Millisecond)
	}
	logs, _ := w.env.DockerMayFail("logs", container)
	inspect, _ := w.env.DockerMayFail("inspect", "--format",
		"status={{.State.Status}} exit={{.State.ExitCode}} error={{.State.Error}} image={{.Image}} config_image={{.Config.Image}} labels={{json .Config.Labels}}", container)
	w.env.T.Fatalf("timed out after %s waiting for webhook notifications from %s\ninspect: %s\nlogs:\n%s", timeout, container, inspect, logs)
	return nil
}

type HangingWebhook struct {
	env      *Env
	server   *httptest.Server
	lock     sync.Mutex
	received []Notification
	entered  chan struct{}
	release  chan struct{}
}

func (e *Env) HangingWebhook() *HangingWebhook {
	webhook := &HangingWebhook{env: e, entered: make(chan struct{}, 32), release: make(chan struct{}, 1)}
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		e.T.Fatal(err)
	}
	webhook.server = httptest.NewUnstartedServer(http.HandlerFunc(webhook.serve))
	webhook.server.Listener = listener
	webhook.server.Start()
	e.T.Cleanup(webhook.server.Close)
	return webhook
}

func (w *HangingWebhook) serve(response http.ResponseWriter, request *http.Request) {
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
	select {
	case w.entered <- struct{}{}:
	default:
	}
	select {
	case <-w.release:
		response.WriteHeader(http.StatusNoContent)
	case <-request.Context().Done():
	}
}

func (w *HangingWebhook) URL() string {
	return "generic://" + strings.TrimPrefix(w.server.URL, "http://") + "/hook?disabletls=yes&template=json"
}

func (w *HangingWebhook) ContainerURL() string {
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(w.server.URL, "http://"))
	return "generic://host.docker.internal:" + port + "/hook?disabletls=yes&template=json"
}

func (w *HangingWebhook) WaitForRequest(timeout time.Duration) {
	w.env.T.Helper()
	select {
	case <-w.entered:
		return
	case <-time.After(timeout):
		w.env.T.Fatalf("timed out after %s waiting for webhook request", timeout)
	}
}

func (w *HangingWebhook) Release() {
	select {
	case w.release <- struct{}{}:
	default:
	}
}

func (w *HangingWebhook) Received() []Notification {
	w.lock.Lock()
	defer w.lock.Unlock()
	return append([]Notification{}, w.received...)
}

func (w *HangingWebhook) Matching(text string) []Notification {
	var matching []Notification
	for _, notification := range w.Received() {
		if strings.Contains(notification.Message, text) {
			matching = append(matching, notification)
		}
	}
	return matching
}
