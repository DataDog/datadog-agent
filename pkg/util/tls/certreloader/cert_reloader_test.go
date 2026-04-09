// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package certreloader

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTestCert(t *testing.T, dir, cn string) (certFile, keyFile string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)

	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	require.NoError(t, os.WriteFile(certFile, certPEM, 0644))

	keyBytes, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	require.NoError(t, os.WriteFile(keyFile, keyPEM, 0600))

	return certFile, keyFile
}

func TestNew_LoadsCertificate(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := writeTestCert(t, dir, "test-cert")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := New(ctx, certFile, keyFile, RealClock())

	cert, err := r.GetCertificate(nil)
	require.NoError(t, err)
	require.NotNil(t, cert)
	require.NotNil(t, cert.Leaf)
	assert.Equal(t, "test-cert", cert.Leaf.Subject.CommonName)
}

func TestNew_ErrorOnMissingFile(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := New(ctx, "/nonexistent/cert.pem", "/nonexistent/key.pem", RealClock())

	cert, err := r.GetCertificate(nil)
	assert.Nil(t, cert)
	assert.Error(t, err)
}

func TestGetClientCertificate(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := writeTestCert(t, dir, "client-cert")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := New(ctx, certFile, keyFile, RealClock())

	cert, err := r.GetClientCertificate(nil)
	require.NoError(t, err)
	require.NotNil(t, cert)
	require.NotNil(t, cert.Leaf)
	assert.Equal(t, "client-cert", cert.Leaf.Subject.CommonName)
}

func TestRetryLoadX509KeyPair_Success(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := writeTestCert(t, dir, "retry-test")

	cert, err := retryLoadX509KeyPair(3, time.Millisecond, certFile, keyFile)
	require.NoError(t, err)
	require.NotNil(t, cert)
}

func TestRetryLoadX509KeyPair_NonRetryableError(t *testing.T) {
	cert, err := retryLoadX509KeyPair(3, time.Millisecond, "/nonexistent", "/nonexistent")
	assert.Nil(t, cert)
	assert.Error(t, err)
}

func TestShouldReload_AfterCacheTimeout(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := writeTestCert(t, dir, "reload-test")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := New(ctx, certFile, keyFile, RealClock())

	assert.False(t, r.shouldReload(), "should not reload immediately after creation")

	r.mu.Lock()
	r.lastUpdate = time.Now().Add(-cacheTimeout - time.Minute)
	r.mu.Unlock()

	assert.True(t, r.shouldReload(), "should reload after cache timeout")
}

func TestShouldReload_AfterErrorCacheTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := New(ctx, "/nonexistent", "/nonexistent", RealClock())
	assert.False(t, r.shouldReload(), "should not reload immediately after error")

	r.mu.Lock()
	r.lastUpdate = time.Now().Add(-errorCacheTimeout - time.Minute)
	r.mu.Unlock()

	assert.True(t, r.shouldReload(), "should reload after error cache timeout")
}

func TestCancelStopsBackground(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := writeTestCert(t, dir, "cancel-test")

	ctx, cancel := context.WithCancel(context.Background())
	r := New(ctx, certFile, keyFile, RealClock())
	cancel()

	time.Sleep(50 * time.Millisecond)
	cert, err := r.GetCertificate(nil)
	require.NoError(t, err)
	assert.NotNil(t, cert, "certificate should still be available after cancel")
}

func TestReloadPicksUpNewCert(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := writeTestCert(t, dir, "original-cert")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := New(ctx, certFile, keyFile, RealClock())

	cert1, err := r.GetCertificate(nil)
	require.NoError(t, err)
	assert.Equal(t, "original-cert", cert1.Leaf.Subject.CommonName)

	writeTestCert(t, dir, "rotated-cert")
	r.reloadCertificate()

	cert2, err := r.GetCertificate(nil)
	require.NoError(t, err)
	assert.Equal(t, "rotated-cert", cert2.Leaf.Subject.CommonName)
}

func TestShouldReload_ExpiredCert(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := writeTestCert(t, dir, "expiry-test")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := New(ctx, certFile, keyFile, RealClock())

	r.mu.Lock()
	r.certificate.Leaf.NotAfter = time.Now().Add(-time.Hour)
	r.mu.Unlock()

	assert.True(t, r.shouldReload(), "should reload when certificate is expired")
}

func TestGetCertificate_ConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := writeTestCert(t, dir, "concurrent-test")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := New(ctx, certFile, keyFile, RealClock())

	done := make(chan struct{})
	for range 10 {
		go func() {
			for i := range 100 {
				if i%10 == 0 {
					r.reloadCertificate()
				} else {
					_, _ = r.GetCertificate(nil)
				}
			}
			done <- struct{}{}
		}()
	}
	for range 10 {
		<-done
	}

	cert, err := r.GetCertificate(&tls.ClientHelloInfo{})
	require.NoError(t, err)
	assert.NotNil(t, cert)
}
