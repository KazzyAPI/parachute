package harness

import (
	"bufio"
	"crypto/tls"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

type EngineProxy struct {
	env      *Env
	port     int
	lock     sync.Mutex
	listener net.Listener
	conns    map[net.Conn]bool
}

func (e *Env) EngineProxy() *EngineProxy {
	e.T.Helper()
	proxy := &EngineProxy{env: e, port: e.FreePort(), conns: map[net.Conn]bool{}}
	proxy.Resume()
	e.T.Cleanup(proxy.Pause)
	return proxy
}

func (p *EngineProxy) Endpoint() string { return "tcp://127.0.0.1:" + strconv.Itoa(p.port) }

func (p *EngineProxy) Resume() {
	p.env.T.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(p.port))
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
