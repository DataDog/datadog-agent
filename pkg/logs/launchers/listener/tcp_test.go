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
	listener := NewTCPListener(pp, sources.NewLogSource("", &config.LogsConfig{Port: tcpTestPort}), 9000)
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

	listener := NewTCPListener(pp, src, 9000)
	listener.Start()
	require.NotNil(t, listener.listener)
	require.NotNil(t, listener.tlsConfig)

	conn, err := tls.Dial("tcp", listener.listener.Addr().String(), &tls.Config{
		InsecureSkipVerify: true,
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

	listener := NewTCPListener(pp, src, 9000)
	listener.Start()
	require.NotNil(t, listener.listener)

	conn, err := net.Dial("tcp", listener.listener.Addr().String())
	require.NoError(t, err)

	fmt.Fprint(conn, "hello plaintext\n")
	conn.Close()

	// The tailer should not produce any messages for plaintext on a TLS listener.
	// Give a short window for any spurious messages then verify none arrived.
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

func TestTCPDoesNotTruncateMessagesThatAreBiggerThanTheReadBufferSize(t *testing.T) {
	pp := mock.NewMockProvider()
	msgChan := pp.NextPipelineChan()
	listener := NewTCPListener(pp, sources.NewLogSource("", &config.LogsConfig{Port: tcpTestPort}), 100)
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
