package harness

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"path/filepath"
	"time"
)

type certificateAuthority struct {
	env         *Env
	certificate *x509.Certificate
	key         *ecdsa.PrivateKey
	pem         []byte
}

func (e *Env) TLSEngineProxy(certDir string) *HTTPEngineProxy {
	e.T.Helper()
	authority := e.newAuthority()
	serverCert, _, _ := authority.issue(x509.ExtKeyUsageServerAuth)
	_, clientPEM, clientKeyPEM := authority.issue(x509.ExtKeyUsageClientAuth)
	e.WriteFile(filepath.Join(certDir, "ca.pem"), string(authority.pem), 0o644)
	e.WriteFile(filepath.Join(certDir, "cert.pem"), string(clientPEM), 0o644)
	e.WriteFile(filepath.Join(certDir, "key.pem"), string(clientKeyPEM), 0o600)
	pool := x509.NewCertPool()
	pool.AddCert(authority.certificate)
	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{serverCert}, ClientCAs: pool, ClientAuth: tls.RequireAndVerifyClientCert, MinVersion: tls.VersionTLS12,
	})
	if err != nil {
		e.T.Fatal(err)
	}
	proxy := newHTTPEngineProxy("tcp://"+listener.Addr().String(), "e2e-engine-"+randomID())
	e.serveHTTP(listener, proxy)
	return proxy
}

func (e *Env) CertDir(name string) string { return filepath.Join(e.Dir, name) }

func (e *Env) newAuthority() *certificateAuthority {
	e.T.Helper()
	key := e.generateKey()
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "e2e engine CA " + randomID()},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		e.T.Fatal(err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		e.T.Fatal(err)
	}
	return &certificateAuthority{env: e, certificate: certificate, key: key, pem: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}
}

func (a *certificateAuthority) issue(usage x509.ExtKeyUsage) (tls.Certificate, []byte, []byte) {
	a.env.T.Helper()
	key := a.env.generateKey()
	template := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: "127.0.0.1"},
		IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		ExtKeyUsage: []x509.ExtKeyUsage{usage}, KeyUsage: x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, template, a.certificate, &key.PublicKey, a.key)
	if err != nil {
		a.env.T.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		a.env.T.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key},
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
}

func (e *Env) generateKey() *ecdsa.PrivateKey {
	e.T.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		e.T.Fatal(err)
	}
	return key
}
