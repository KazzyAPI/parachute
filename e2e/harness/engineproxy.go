package harness

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type EngineProxy struct {
	env      *Env
	address  string
	accepted atomic.Int64
	lock     sync.Mutex
	listener net.Listener
	conns    map[net.Conn]bool
}

func (e *Env) EngineProxy() *EngineProxy {
	e.T.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		e.T.Fatal(err)
	}
	proxy := &EngineProxy{env: e, address: listener.Addr().String(), listener: listener, conns: map[net.Conn]bool{}}
	go proxy.accept(listener)
	e.T.Cleanup(proxy.Pause)
	return proxy
}

func (p *EngineProxy) Endpoint() string { return "tcp://" + p.address }

func (p *EngineProxy) Connections() int64 { return p.accepted.Load() }

func (p *EngineProxy) Resume() {
	p.env.T.Helper()
	listener, err := net.Listen("tcp", p.address)
	if err != nil {
		p.env.T.Fatal(err)
	}
	p.lock.Lock()
	p.listener = listener
	p.lock.Unlock()
	go p.accept(listener)
}

func (p *EngineProxy) Pause() {
	p.lock.Lock()
	defer p.lock.Unlock()
	if p.listener != nil {
		_ = p.listener.Close()
		p.listener = nil
	}
	for conn := range p.conns {
		_ = conn.Close()
	}
	p.conns = map[net.Conn]bool{}
}

func (p *EngineProxy) accept(listener net.Listener) {
	for {
		inbound, err := listener.Accept()
		if err != nil {
			return
		}
		p.accepted.Add(1)
		outbound, err := net.Dial("unix", strings.TrimPrefix(suite.dockerHost, "unix://"))
		if err != nil {
			_ = inbound.Close()
			continue
		}
		p.lock.Lock()
		p.conns[inbound], p.conns[outbound] = true, true
		p.lock.Unlock()
		go func() {
			_, _ = io.Copy(outbound, inbound)
			_ = outbound.Close()
		}()
		go func() {
			_, _ = io.Copy(inbound, outbound)
			_ = inbound.Close()
		}()
	}
}

type FakeTLSDaemon struct {
	Endpoint string
	plain    atomic.Int64
}

func (e *Env) FakeTLSDaemon() *FakeTLSDaemon {
	e.T.Helper()
	certificate, _, _, _ := e.certificates()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		e.T.Fatal(err)
	}
	daemon := &FakeTLSDaemon{Endpoint: "tcp://" + listener.Addr().String()}
	e.T.Cleanup(func() { _ = listener.Close() })
	go daemon.serve(listener, certificate)
	return daemon
}

func (d *FakeTLSDaemon) PlainRequests() int64 { return d.plain.Load() }

func (d *FakeTLSDaemon) serve(listener net.Listener, certificate tls.Certificate) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go d.handle(conn, certificate)
	}
}

func (d *FakeTLSDaemon) handle(conn net.Conn, certificate tls.Certificate) {
	reader := bufio.NewReader(conn)
	if first, err := reader.Peek(1); err == nil && first[0] != 0x16 {
		d.plain.Add(1)
	} else if err == nil {
		server := tls.Server(&bufferedConn{Conn: conn, reader: reader}, &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12})
		_ = server.Handshake()
	}
	_ = conn.Close()
}

type HTTPEngineProxy struct {
	Endpoint string
	engineID string
	forward  *httputil.ReverseProxy
	lock     sync.Mutex
	failing  []string
	requests []string
}

func (e *Env) HTTPEngineProxy() *HTTPEngineProxy {
	e.T.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		e.T.Fatal(err)
	}
	proxy := newHTTPEngineProxy("tcp://"+listener.Addr().String(), "")
	e.serveHTTP(listener, proxy)
	return proxy
}

func (e *Env) DistinctEngineProxy() *HTTPEngineProxy {
	e.T.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		e.T.Fatal(err)
	}
	proxy := newHTTPEngineProxy("tcp://"+listener.Addr().String(), "e2e-engine-"+randomID())
	e.serveHTTP(listener, proxy)
	return proxy
}

func newHTTPEngineProxy(endpoint, engineID string) *HTTPEngineProxy {
	socket := strings.TrimPrefix(suite.dockerHost, "unix://")
	proxy := &HTTPEngineProxy{Endpoint: endpoint, engineID: engineID}
	proxy.forward = &httputil.ReverseProxy{
		Rewrite: func(request *httputil.ProxyRequest) {
			request.Out.URL.Scheme, request.Out.URL.Host = "http", "docker"
		},
		Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socket)
		}},
		ModifyResponse: proxy.rewriteEngineID,
	}
	return proxy
}

func (e *Env) serveHTTP(listener net.Listener, handler http.Handler) {
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = server.Serve(listener) }()
	e.T.Cleanup(func() { _ = server.Close() })
}

func (p *HTTPEngineProxy) Fail(pathPart string) {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.failing = append(p.failing, pathPart)
}

func (p *HTTPEngineProxy) Requests() []string {
	p.lock.Lock()
	defer p.lock.Unlock()
	return append([]string{}, p.requests...)
}

func (p *HTTPEngineProxy) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	p.lock.Lock()
	p.requests = append(p.requests, request.Method+" "+request.URL.Path)
	failing := slices.ContainsFunc(p.failing, func(part string) bool { return strings.Contains(request.URL.Path, part) })
	p.lock.Unlock()
	if !failing {
		p.forward.ServeHTTP(response, request)
		return
	}
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusInternalServerError)
	_, _ = io.WriteString(response, `{"message":"injected engine failure"}`)
}

func (p *HTTPEngineProxy) rewriteEngineID(response *http.Response) error {
	if p.engineID == "" || !strings.HasSuffix(response.Request.URL.Path, "/info") {
		return nil
	}
	var info map[string]any
	if err := json.NewDecoder(response.Body).Decode(&info); err != nil {
		return err
	}
	_ = response.Body.Close()
	info["ID"] = p.engineID
	body, err := json.Marshal(info)
	if err != nil {
		return err
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	response.ContentLength = int64(len(body))
	response.Header.Set("Content-Length", strconv.Itoa(len(body)))
	return nil
}
