package harness

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

type FakeHub struct {
	env      *Env
	CAFile   string
	Port     int
	lock     sync.Mutex
	tokens   []string
	tlsConns chan net.Conn
}

func (e *Env) FakeDockerHub() *FakeHub {
	e.T.Helper()
	hub := &FakeHub{env: e, tlsConns: make(chan net.Conn)}
	certificate, caPEM, _, _ := hub.certificates()
	hub.CAFile = e.WriteFile("hub/ca.pem", string(caPEM), 0o644)
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		e.T.Fatal(err)
	}
	hub.Port = listener.Addr().(*net.TCPAddr).Port
	proxy := &http.Server{Handler: http.HandlerFunc(hub.connect), ReadHeaderTimeout: 10 * time.Second}
	registry := &http.Server{Handler: http.HandlerFunc(hub.serve), ReadHeaderTimeout: 10 * time.Second,
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}}
	tunnel := &channelListener{conns: hub.tlsConns, done: make(chan struct{}), address: listener.Addr()}
	go func() { _ = proxy.Serve(listener) }()
	go func() { _ = registry.ServeTLS(tunnel, "", "") }()
	e.T.Cleanup(func() {
		_ = proxy.Close()
		_ = registry.Close()
	})
	return hub
}

func (h *FakeHub) ProxyURL() string {
	return "http://host.docker.internal:" + strconv.Itoa(h.Port)
}

func (h *FakeHub) TokenAuthorizations() []string {
	h.lock.Lock()
	defer h.lock.Unlock()
	return append([]string{}, h.tokens...)
}

func (h *FakeHub) connect(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodConnect {
		http.Error(response, "CONNECT only", http.StatusMethodNotAllowed)
		return
	}
	hijacker, ok := response.(http.Hijacker)
	if !ok {
		http.Error(response, "no hijack", http.StatusInternalServerError)
		return
	}
	conn, buffered, err := hijacker.Hijack()
	if err != nil {
		return
	}
	if _, err := conn.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n")); err != nil {
		_ = conn.Close()
		return
	}
	h.tlsConns <- &bufferedConn{Conn: conn, reader: buffered.Reader}
}

func (h *FakeHub) serve(response http.ResponseWriter, request *http.Request) {
	if request.Host == "auth.docker.io" {
		h.lock.Lock()
		h.tokens = append(h.tokens, request.Header.Get("Authorization"))
		h.lock.Unlock()
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"token":"fake-token"}`))
		return
	}
	if request.Header.Get("Authorization") != "Bearer fake-token" {
		response.Header().Set("WWW-Authenticate", `Bearer realm="https://auth.docker.io/token",service="registry.docker.io",scope="repository:library/fixture:pull"`)
		http.Error(response, "unauthorized", http.StatusUnauthorized)
		return
	}
	response.Header().Set("Docker-Content-Digest", "sha256:"+fakeDigest)
	response.WriteHeader(http.StatusOK)
}

const fakeDigest = "1111111111111111111111111111111111111111111111111111111111111111"

func (h *FakeHub) certificates() (tls.Certificate, []byte, []byte, []byte) {
	h.env.T.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		h.env.T.Fatal(err)
	}
	caTemplate := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "e2e fake hub CA"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		h.env.T.Fatal(err)
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		h.env.T.Fatal(err)
	}
	leafTemplate := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "registry-1.docker.io"},
		DNSNames: []string{"registry-1.docker.io", "auth.docker.io", "index.docker.io"}, NotBefore: time.Now().Add(-time.Hour),
		NotAfter: time.Now().Add(time.Hour), ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, KeyUsage: x509.KeyUsageDigitalSignature}
	caCertificate, err := x509.ParseCertificate(caDER)
	if err != nil {
		h.env.T.Fatal(err)
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, caCertificate, &leafKey.PublicKey, caKey)
	if err != nil {
		h.env.T.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(leafKey)
	if err != nil {
		h.env.T.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{leafDER}, PrivateKey: leafKey},
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
}

func (e *Env) UnrelatedCertificates() string {
	e.T.Helper()
	_, caPEM, certPEM, keyPEM := (&FakeHub{env: e}).certificates()
	e.WriteFile("certs/ca.pem", string(caPEM), 0o644)
	e.WriteFile("certs/cert.pem", string(certPEM), 0o644)
	return filepath.Dir(e.WriteFile("certs/key.pem", string(keyPEM), 0o600))
}

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(buffer []byte) (int, error) { return c.reader.Read(buffer) }

type channelListener struct {
	conns   chan net.Conn
	done    chan struct{}
	once    sync.Once
	address net.Addr
}

func (l *channelListener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.conns:
		return conn, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *channelListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}

func (l *channelListener) Addr() net.Addr { return l.address }
