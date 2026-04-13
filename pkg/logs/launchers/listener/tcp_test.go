// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package listener

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// use a randomly assigned port
var tcpTestPort = 0

func TestTCPShouldReceivesMessages(t *testing.T) {
	pp := mock.NewMockProvider()
	msgChan := pp.NextPipelineChan()
	listener, err := NewTCPListener(pp, sources.NewLogSource("", &config.LogsConfig{Port: tcpTestPort}), 9000)
	require.NoError(t, err)
	listener.Start()
	conn, err := net.Dial("tcp", listener.listener.Addr().String())
	assert.Nil(t, err)
	defer conn.Close()
	var msg *message.Message

	fmt.Fprint(conn, "hello world\n")
	msg = <-msgChan
	assert.Equal(t, "hello world", string(msg.GetContent()))
	assert.Equal(t, 1, len(listener.tailers))

	listener.Stop()
}

func TestTCPTLSShouldReceiveMessages(t *testing.T) {
	certFile, keyFile := generateTestCert(t)

	pp := mock.NewMockProvider()
	msgChan := pp.NextPipelineChan()

	src := sources.NewLogSource("", &config.LogsConfig{
		Port: tcpTestPort,
		TLS: &config.TLSListenerConfig{
			CertFile: certFile,
			KeyFile:  keyFile,
		},
	})

	listener, err := NewTCPListener(pp, src, 9000)
	require.NoError(t, err)
	listener.Start()
	require.NotNil(t, listener.listener)
	require.NotNil(t, listener.tlsCredentials)

	conn, err := tls.Dial("tcp", listener.listener.Addr().String(), &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec
	})
	require.NoError(t, err)
	defer conn.Close()

	fmt.Fprint(conn, "hello tls\n")
	msg := <-msgChan
	assert.Equal(t, "hello tls", string(msg.GetContent()))
	assert.Equal(t, 1, len(listener.tailers))

	listener.Stop()
}

func TestTCPTLSRejectsPlaintextConnection(t *testing.T) {
	certFile, keyFile := generateTestCert(t)

	pp := mock.NewMockProvider()

	src := sources.NewLogSource("", &config.LogsConfig{
		Port: tcpTestPort,
		TLS: &config.TLSListenerConfig{
			CertFile: certFile,
			KeyFile:  keyFile,
		},
	})

	listener, err := NewTCPListener(pp, src, 9000)
	require.NoError(t, err)
	listener.Start()
	require.NotNil(t, listener.listener)

	conn, err := net.Dial("tcp", listener.listener.Addr().String())
	require.NoError(t, err)

	fmt.Fprint(conn, "hello plaintext\n")
	conn.Close()

	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, 0, len(pp.NextPipelineChan()))

	listener.Stop()
}

// generateTestCert creates a self-signed ECDSA certificate and writes the
// PEM-encoded cert and key to temporary files. Returns the file paths.
func generateTestCert(t *testing.T) (certPath, keyPath string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	dir := t.TempDir()
	certPath = filepath.Join(dir, "cert.pem")
	keyPath = filepath.Join(dir, "key.pem")

	certOut, err := os.Create(certPath)
	require.NoError(t, err)
	require.NoError(t, pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}))
	certOut.Close()

	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	keyOut, err := os.Create(keyPath)
	require.NoError(t, err)
	require.NoError(t, pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
	keyOut.Close()

	return certPath, keyPath
}

// testPKI holds a CA and its PEM file path, used for mTLS tests.
type testPKI struct {
	caKey    *ecdsa.PrivateKey
	caCert   *x509.Certificate
	caPath   string
	dir      string
	serialNo int64
}

// generateTestPKI creates a CA certificate, writes it to a temp file, and
// returns the PKI context for issuing signed server/client certificates.
func generateTestPKI(t *testing.T) *testPKI {
	t.Helper()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	require.NoError(t, err)
	caCert, err := x509.ParseCertificate(caDER)
	require.NoError(t, err)

	dir := t.TempDir()
	caPath := filepath.Join(dir, "ca.pem")
	f, err := os.Create(caPath)
	require.NoError(t, err)
	require.NoError(t, pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: caDER}))
	f.Close()

	return &testPKI{caKey: caKey, caCert: caCert, caPath: caPath, dir: dir, serialNo: 10}
}

// issueServerCert creates a server certificate signed by the CA and writes it
// to disk. Returns the cert and key file paths.
func (p *testPKI) issueServerCert(t *testing.T) (certPath, keyPath string) {
	t.Helper()
	return p.issueCert(t, "server", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth})
}

// issueClientCert creates a client certificate signed by the CA and writes it
// to disk. Returns the cert and key file paths.
func (p *testPKI) issueClientCert(t *testing.T) (certPath, keyPath string) {
	t.Helper()
	return p.issueCert(t, "client", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth})
}

func (p *testPKI) issueCert(t *testing.T, prefix string, extKeyUsage []x509.ExtKeyUsage) (certPath, keyPath string) {
	t.Helper()
	p.serialNo++

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(p.serialNo),
		Subject:      pkix.Name{CommonName: prefix},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  extKeyUsage,
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, p.caCert, &key.PublicKey, p.caKey)
	require.NoError(t, err)

	certPath = filepath.Join(p.dir, prefix+"-cert.pem")
	keyPath = filepath.Join(p.dir, prefix+"-key.pem")

	certOut, err := os.Create(certPath)
	require.NoError(t, err)
	require.NoError(t, pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}))
	certOut.Close()

	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	keyOut, err := os.Create(keyPath)
	require.NoError(t, err)
	require.NoError(t, pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
	keyOut.Close()

	return certPath, keyPath
}

func TestTCPTLSRefusesToStartOnBadCert(t *testing.T) {
	pp := mock.NewMockProvider()

	src := sources.NewLogSource("", &config.LogsConfig{
		Port: tcpTestPort,
		TLS: &config.TLSListenerConfig{
			CertFile: "/nonexistent/cert.pem",
			KeyFile:  "/nonexistent/key.pem",
		},
	})

	_, err := NewTCPListener(pp, src, 9000)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load TLS credentials")
}

func TestTCPBindHost(t *testing.T) {
	pp := mock.NewMockProvider()
	msgChan := pp.NextPipelineChan()
	listener, err := NewTCPListener(pp, sources.NewLogSource("", &config.LogsConfig{
		Port:     tcpTestPort,
		BindHost: "127.0.0.1",
	}), 9000)
	require.NoError(t, err)
	listener.Start()
	require.NotNil(t, listener.listener)

	addr := listener.listener.Addr().String()
	assert.True(t, strings.HasPrefix(addr, "127.0.0.1:"), "expected 127.0.0.1 bind, got %s", addr)

	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer conn.Close()

	fmt.Fprint(conn, "bound msg\n")
	msg := <-msgChan
	assert.Equal(t, "bound msg", string(msg.GetContent()))

	listener.Stop()
}

func TestTCPMaxConnections(t *testing.T) {
	pp := mock.NewMockProvider()
	listener, err := NewTCPListener(pp, sources.NewLogSource("", &config.LogsConfig{
		Port:           tcpTestPort,
		MaxConnections: 2,
	}), 9000)
	require.NoError(t, err)
	listener.Start()
	require.NotNil(t, listener.listener)

	conn1, err := net.Dial("tcp", listener.listener.Addr().String())
	require.NoError(t, err)
	defer conn1.Close()
	fmt.Fprint(conn1, "msg1\n")
	<-pp.NextPipelineChan()

	conn2, err := net.Dial("tcp", listener.listener.Addr().String())
	require.NoError(t, err)
	defer conn2.Close()
	fmt.Fprint(conn2, "msg2\n")
	<-pp.NextPipelineChan()

	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 2, len(listener.tailers))

	conn3, err := net.Dial("tcp", listener.listener.Addr().String())
	require.NoError(t, err)
	defer conn3.Close()

	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, 2, len(listener.tailers), "third connection should have been rejected")

	listener.Stop()
}

func TestTCPDefaultIdleTimeoutForTLS(t *testing.T) {
	certFile, keyFile := generateTestCert(t)

	pp := mock.NewMockProvider()
	src := sources.NewLogSource("", &config.LogsConfig{
		Port: tcpTestPort,
		TLS: &config.TLSListenerConfig{
			CertFile: certFile,
			KeyFile:  keyFile,
		},
	})

	listener, err := NewTCPListener(pp, src, 9000)
	require.NoError(t, err)
	assert.Equal(t, defaultTLSIdleTimeout, listener.idleTimeout)

	listener.Stop()
}

func TestTCPTLSHandshakeTimeout(t *testing.T) {
	certFile, keyFile := generateTestCert(t)

	pp := mock.NewMockProvider()
	src := sources.NewLogSource("", &config.LogsConfig{
		Port: tcpTestPort,
		TLS: &config.TLSListenerConfig{
			CertFile: certFile,
			KeyFile:  keyFile,
		},
	})

	listener, err := NewTCPListener(pp, src, 9000)
	require.NoError(t, err)
	listener.Start()
	require.NotNil(t, listener.listener)

	conn, err := net.Dial("tcp", listener.listener.Addr().String())
	require.NoError(t, err)
	defer conn.Close()

	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, 0, len(listener.tailers), "raw TCP conn without TLS handshake should not create a tailer")

	listener.Stop()
}

func TestTCPDoesNotTruncateMessagesThatAreBiggerThanTheReadBufferSize(t *testing.T) {
	pp := mock.NewMockProvider()
	msgChan := pp.NextPipelineChan()
	listener, err := NewTCPListener(pp, sources.NewLogSource("", &config.LogsConfig{Port: tcpTestPort}), 100)
	require.NoError(t, err)
	listener.Start()

	conn, err := net.Dial("tcp", listener.listener.Addr().String())
	assert.Nil(t, err)

	var msg *message.Message
	fmt.Fprint(conn, strings.Repeat("a", 80)+"\n")
	msg = <-msgChan
	assert.Equal(t, strings.Repeat("a", 80), string(msg.GetContent()))

	fmt.Fprint(conn, strings.Repeat("a", 200)+"\n")
	msg = <-msgChan
	assert.Equal(t, strings.Repeat("a", 200), string(msg.GetContent()))

	fmt.Fprint(conn, strings.Repeat("a", 70)+"\n")
	msg = <-msgChan
	assert.Equal(t, strings.Repeat("a", 70), string(msg.GetContent()))

	listener.Stop()
}

func TestTCPTLSNegotiatesCipher(t *testing.T) {
	certFile, keyFile := generateTestCert(t)

	pp := mock.NewMockProvider()
	msgChan := pp.NextPipelineChan()

	src := sources.NewLogSource("", &config.LogsConfig{
		Port: tcpTestPort,
		TLS: &config.TLSListenerConfig{
			CertFile: certFile,
			KeyFile:  keyFile,
		},
	})

	listener, err := NewTCPListener(pp, src, 9000)
	require.NoError(t, err)
	listener.Start()
	require.NotNil(t, listener.listener)

	conn, err := tls.Dial("tcp", listener.listener.Addr().String(), &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec
		MaxVersion:         tls.VersionTLS12,
	})
	require.NoError(t, err)
	defer conn.Close()

	state := conn.ConnectionState()
	assert.Equal(t, uint16(tls.VersionTLS12), state.Version)
	assert.NotEqual(t, uint16(0), state.CipherSuite, "a cipher suite should have been negotiated")

	fmt.Fprint(conn, "tls12 msg\n")
	msg := <-msgChan
	assert.Equal(t, "tls12 msg", string(msg.GetContent()))

	listener.Stop()
}

func TestTCPPlaintextHasNoTLSTags(t *testing.T) {
	pp := mock.NewMockProvider()
	msgChan := pp.NextPipelineChan()
	listener, err := NewTCPListener(pp, sources.NewLogSource("", &config.LogsConfig{Port: tcpTestPort}), 9000)
	require.NoError(t, err)
	listener.Start()

	conn, err := net.Dial("tcp", listener.listener.Addr().String())
	require.NoError(t, err)
	defer conn.Close()

	fmt.Fprint(conn, "plaintext msg\n")
	msg := <-msgChan
	assert.Equal(t, "plaintext msg", string(msg.GetContent()))
	for _, tag := range msg.Origin.Tags(nil) {
		assert.False(t, strings.HasPrefix(tag, "tls_"), "plaintext message should not have TLS tags, got %q", tag)
	}

	listener.Stop()
}

func TestTCPMTLSRequiredAcceptsValidClient(t *testing.T) {
	pki := generateTestPKI(t)
	serverCert, serverKey := pki.issueServerCert(t)
	clientCert, clientKey := pki.issueClientCert(t)

	pp := mock.NewMockProvider()
	msgChan := pp.NextPipelineChan()

	src := sources.NewLogSource("", &config.LogsConfig{
		Port: tcpTestPort,
		TLS: &config.TLSListenerConfig{
			CertFile:   serverCert,
			KeyFile:    serverKey,
			CAFile:     pki.caPath,
			ClientAuth: "required",
		},
	})

	listener, err := NewTCPListener(pp, src, 9000)
	require.NoError(t, err)
	listener.Start()
	require.NotNil(t, listener.listener)

	clientTLSCert, err := tls.LoadX509KeyPair(clientCert, clientKey)
	require.NoError(t, err)

	conn, err := tls.Dial("tcp", listener.listener.Addr().String(), &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec
		Certificates:       []tls.Certificate{clientTLSCert},
	})
	require.NoError(t, err)
	defer conn.Close()

	fmt.Fprint(conn, "mtls ok\n")
	msg := <-msgChan
	assert.Equal(t, "mtls ok", string(msg.GetContent()))

	listener.Stop()
}

func TestTCPMTLSRequiredRejectsNoClientCert(t *testing.T) {
	pki := generateTestPKI(t)
	serverCert, serverKey := pki.issueServerCert(t)

	pp := mock.NewMockProvider()

	src := sources.NewLogSource("", &config.LogsConfig{
		Port: tcpTestPort,
		TLS: &config.TLSListenerConfig{
			CertFile:   serverCert,
			KeyFile:    serverKey,
			CAFile:     pki.caPath,
			ClientAuth: "required",
		},
	})

	listener, err := NewTCPListener(pp, src, 9000)
	require.NoError(t, err)
	listener.Start()
	require.NotNil(t, listener.listener)

	conn, dialErr := tls.Dial("tcp", listener.listener.Addr().String(), &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec
	})

	if dialErr != nil {
		// TLS 1.2: handshake fails synchronously
		assert.Contains(t, dialErr.Error(), "certificate")
	} else {
		// TLS 1.3: handshake completes but server kills the connection on
		// the post-handshake client auth check. The rejection surfaces as
		// an error on the first I/O after the handshake.
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(2 * time.Second)) //nolint:errcheck
		_, writeErr := fmt.Fprint(conn, "should fail\n")
		if writeErr == nil {
			buf := make([]byte, 1)
			_, readErr := conn.Read(buf)
			require.Error(t, readErr, "server should terminate the connection")
		}
	}

	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, 0, len(listener.tailers), "no tailer should exist for a rejected mTLS connection")

	listener.Stop()
}

func TestTCPMTLSOptionalAcceptsClientCert(t *testing.T) {
	pki := generateTestPKI(t)
	serverCert, serverKey := pki.issueServerCert(t)
	clientCert, clientKey := pki.issueClientCert(t)

	pp := mock.NewMockProvider()
	msgChan := pp.NextPipelineChan()

	src := sources.NewLogSource("", &config.LogsConfig{
		Port: tcpTestPort,
		TLS: &config.TLSListenerConfig{
			CertFile:   serverCert,
			KeyFile:    serverKey,
			CAFile:     pki.caPath,
			ClientAuth: "optional",
		},
	})

	listener, err := NewTCPListener(pp, src, 9000)
	require.NoError(t, err)
	listener.Start()
	require.NotNil(t, listener.listener)

	clientTLSCert, err := tls.LoadX509KeyPair(clientCert, clientKey)
	require.NoError(t, err)

	conn, err := tls.Dial("tcp", listener.listener.Addr().String(), &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec
		Certificates:       []tls.Certificate{clientTLSCert},
	})
	require.NoError(t, err)
	defer conn.Close()

	fmt.Fprint(conn, "optional with cert\n")
	msg := <-msgChan
	assert.Equal(t, "optional with cert", string(msg.GetContent()))

	listener.Stop()
}

func TestTCPMTLSOptionalAcceptsNoClientCert(t *testing.T) {
	pki := generateTestPKI(t)
	serverCert, serverKey := pki.issueServerCert(t)

	pp := mock.NewMockProvider()
	msgChan := pp.NextPipelineChan()

	src := sources.NewLogSource("", &config.LogsConfig{
		Port: tcpTestPort,
		TLS: &config.TLSListenerConfig{
			CertFile:   serverCert,
			KeyFile:    serverKey,
			CAFile:     pki.caPath,
			ClientAuth: "optional",
		},
	})

	listener, err := NewTCPListener(pp, src, 9000)
	require.NoError(t, err)
	listener.Start()
	require.NotNil(t, listener.listener)

	conn, err := tls.Dial("tcp", listener.listener.Addr().String(), &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec
	})
	require.NoError(t, err)
	defer conn.Close()

	fmt.Fprint(conn, "optional no cert\n")
	msg := <-msgChan
	assert.Equal(t, "optional no cert", string(msg.GetContent()))

	listener.Stop()
}

func TestTCPNoIPFilterAcceptsAll(t *testing.T) {
	pp := mock.NewMockProvider()
	msgChan := pp.NextPipelineChan()
	listener, err := NewTCPListener(pp, sources.NewLogSource("", &config.LogsConfig{Port: tcpTestPort}), 9000)
	require.NoError(t, err)
	listener.Start()
	defer listener.Stop()

	conn, err := net.Dial("tcp", listener.listener.Addr().String())
	require.NoError(t, err)
	defer conn.Close()

	fmt.Fprint(conn, "hello\n")
	msg := <-msgChan
	assert.Equal(t, "hello", string(msg.GetContent()))
}

func TestTCPAllowedIPsAcceptsMatchingConnection(t *testing.T) {
	pp := mock.NewMockProvider()
	msgChan := pp.NextPipelineChan()
	source := sources.NewLogSource("", &config.LogsConfig{
		Port:       tcpTestPort,
		AllowedIPs: config.StringSliceField{"127.0.0.0/8", "::1"},
	})
	listener, err := NewTCPListener(pp, source, 9000)
	require.NoError(t, err)
	listener.Start()
	defer listener.Stop()

	conn, err := net.Dial("tcp", listener.listener.Addr().String())
	require.NoError(t, err)
	defer conn.Close()

	fmt.Fprint(conn, "allowed\n")
	msg := <-msgChan
	assert.Equal(t, "allowed", string(msg.GetContent()))
}

func TestTCPDeniedIPsRejectsMatchingConnection(t *testing.T) {
	pp := mock.NewMockProvider()
	source := sources.NewLogSource("", &config.LogsConfig{
		Port:      tcpTestPort,
		DeniedIPs: config.StringSliceField{"127.0.0.0/8", "::1"},
	})
	listener, err := NewTCPListener(pp, source, 9000)
	require.NoError(t, err)
	listener.Start()
	defer listener.Stop()

	conn, err := net.Dial("tcp", listener.listener.Addr().String())
	require.NoError(t, err)

	fmt.Fprint(conn, "denied\n")
	conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	buf := make([]byte, 1)
	_, err = conn.Read(buf)
	assert.Error(t, err, "connection from denied IP should be closed by server")
}

func TestTCPDeniedTakesPrecedenceOverAllow(t *testing.T) {
	pp := mock.NewMockProvider()
	source := sources.NewLogSource("", &config.LogsConfig{
		Port:       tcpTestPort,
		AllowedIPs: config.StringSliceField{"127.0.0.0/8", "::1"},
		DeniedIPs:  config.StringSliceField{"127.0.0.1", "::1"},
	})
	listener, err := NewTCPListener(pp, source, 9000)
	require.NoError(t, err)
	listener.Start()
	defer listener.Stop()

	conn, err := net.Dial("tcp", listener.listener.Addr().String())
	require.NoError(t, err)

	fmt.Fprint(conn, "should be denied\n")
	conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	buf := make([]byte, 1)
	_, err = conn.Read(buf)
	assert.Error(t, err, "connection from denied IP should be closed even if it matches allow list")
}

func TestTCPAllowedIPsRejectsNonMatchingConnection(t *testing.T) {
	pp := mock.NewMockProvider()
	source := sources.NewLogSource("", &config.LogsConfig{
		Port:       tcpTestPort,
		AllowedIPs: config.StringSliceField{"192.168.1.0/24"},
	})
	listener, err := NewTCPListener(pp, source, 9000)
	require.NoError(t, err)
	listener.Start()
	defer listener.Stop()

	conn, err := net.Dial("tcp", listener.listener.Addr().String())
	require.NoError(t, err)

	fmt.Fprint(conn, "should be rejected\n")
	conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	buf := make([]byte, 1)
	_, err = conn.Read(buf)
	assert.Error(t, err, "connection from non-allowed IP should be rejected")
}
