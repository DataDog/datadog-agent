// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateCACert returns a self-signed certificate and key, mimicking the
// agent IPC certificate (CA:TRUE, used as both server and client cert).
// commonName distinguishes subjects between certificates.
func generateCACert(t *testing.T, commonName string) (certPEM, keyPEM []byte, rawSubject []byte) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber:          randomSerial(t),
		Subject:               pkix.Name{Organization: []string{"Datadog, Inc."}, CommonName: commonName},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	certPEM = pemEncode(t, "CERTIFICATE", der)

	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	keyPEM = pemEncode(t, "EC PRIVATE KEY", keyDER)

	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return certPEM, keyPEM, cert.RawSubject
}

func randomSerial(t *testing.T) *big.Int {
	t.Helper()
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)
	return serial
}

func pemEncode(t *testing.T, blockType string, der []byte) []byte {
	t.Helper()
	return pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der})
}

// newIPCCertServer starts a TLS server requiring and verifying a client
// certificate signed by the given CA, like the agent IPC API servers.
func newIPCCertServer(t *testing.T, certPEM, keyPEM, clientCA []byte, handler http.HandlerFunc) *httptest.Server {
	t.Helper()

	serverCert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)
	pool := x509.NewCertPool()
	require.True(t, pool.AppendCertsFromPEM(clientCA))

	ts := httptest.NewUnstartedServer(handler)
	ts.TLS = &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientCAs:    pool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
	}
	ts.StartTLS()
	t.Cleanup(ts.Close)
	return ts
}

// newCertClient returns an HTTP client presenting certificates through the
// provider, like the host HTTP transport does. Keep-alives are disabled so
// each request opens a fresh TLS handshake.
func newCertClient(provider *ipcCertProvider) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DisableKeepAlives: true,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify:   true, //nolint:gosec // test server certificate is self-signed
				GetClientCertificate: provider.getCertificate,
			},
		},
	}
}

func TestIPCCertProviderPresentsCertToIPCServer(t *testing.T) {
	certPEM, keyPEM, _ := generateCACert(t, "ipc")
	file := string(certPEM) + "\n" + string(keyPEM) + "\n"

	fetches := atomic.Int32{}
	provider := newIPCCertProvider(func() (string, error) {
		fetches.Add(1)
		return file, nil
	})

	server := newIPCCertServer(t, certPEM, keyPEM, certPEM, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	client := newCertClient(provider)
	resp, err := client.Get(server.URL)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, int32(1), fetches.Load())

	// Second handshake: cached certificate, no extra fetch
	resp, err = client.Get(server.URL)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, int32(1), fetches.Load())
}

func TestIPCCertProviderNoCertForOtherCA(t *testing.T) {
	serverCAPEM, serverCAKey, _ := generateCACert(t, "server")
	ipcCertPEM, ipcKeyPEM, _ := generateCACert(t, "ipc")
	ipcFile := string(ipcCertPEM) + "\n" + string(ipcKeyPEM) + "\n"

	fetches := atomic.Int32{}
	provider := newIPCCertProvider(func() (string, error) {
		fetches.Add(1)
		return ipcFile, nil
	})

	// Server verifying against its own CA, like the cluster-agent trust domain
	server := newIPCCertServer(t, serverCAPEM, serverCAKey, serverCAPEM, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	client := newCertClient(provider)
	_, err := client.Get(server.URL) //nolint:bodyclose // handshake fails, no body
	require.ErrorContains(t, err, "certificate required")

	// The fetch was throttled to one attempt, not one per handshake
	_, err = client.Get(server.URL) //nolint:bodyclose // handshake fails, no body
	require.Error(t, err)
	assert.Equal(t, int32(1), fetches.Load())
}

func TestIPCCertProviderRefetchesStaleCert(t *testing.T) {
	// Agent reinstall: the new certificate has the same subject but another key,
	// so AcceptableCAs alone cannot detect the change.
	oldCertPEM, oldKeyPEM, _ := generateCACert(t, "ipc")
	newCertPEM, newKeyPEM, _ := generateCACert(t, "ipc")
	oldFile := string(oldCertPEM) + "\n" + string(oldKeyPEM) + "\n"
	newFile := string(newCertPEM) + "\n" + string(newKeyPEM) + "\n"

	current := &oldFile
	provider := newIPCCertProvider(func() (string, error) { return *current, nil })
	provider.refetchInterval = 0 // allow immediate refetch

	server := newIPCCertServer(t, newCertPEM, newKeyPEM, newCertPEM, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	client := newCertClient(provider)
	_, err := client.Get(server.URL) //nolint:bodyclose // handshake fails
	require.Error(t, err)            // cached cert doesn't match the new CA

	// Agent "reinstalled": the cert file now holds the new certificate
	current = &newFile
	resp, err := client.Get(server.URL)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestIPCCertProviderFetchFailureSendsNoCert(t *testing.T) {
	_, _, subject := generateCACert(t, "ipc")

	provider := newIPCCertProvider(func() (string, error) {
		return "", assert.AnError
	})

	// No certificate available: send an empty certificate instead of failing.
	// An empty non-nil Certificate means "no client certificate".
	cert, err := provider.getCertificate(&tls.CertificateRequestInfo{AcceptableCAs: [][]byte{subject}})
	require.NoError(t, err)
	require.NotNil(t, cert)
	assert.Empty(t, cert.Certificate)
}
