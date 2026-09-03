// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package certreloader

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
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

// testCA is a self-signed CA able to sign revocation lists.
type testCA struct {
	key  crypto.Signer
	cert *x509.Certificate
}

func newTestCA(t *testing.T, cn string) *testCA {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)

	return &testCA{key: key, cert: cert}
}

// crlDER creates a revocation list covering the given serial numbers.
func (ca *testCA) crlDER(t *testing.T, nextUpdate time.Time, serials ...int64) []byte {
	t.Helper()

	entries := make([]x509.RevocationListEntry, 0, len(serials))
	for _, serial := range serials {
		entries = append(entries, x509.RevocationListEntry{
			SerialNumber:   big.NewInt(serial),
			RevocationTime: time.Now().Add(-time.Minute),
		})
	}

	der, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{
		Number:                    big.NewInt(1),
		ThisUpdate:                time.Now().Add(-time.Hour),
		NextUpdate:                nextUpdate,
		RevokedCertificateEntries: entries,
	}, ca.cert, ca.key)
	require.NoError(t, err)
	return der
}

func writeFile(t *testing.T, dir, name string, contents []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, contents, 0644))
	return path
}

func TestLoadCRLs_PEM(t *testing.T) {
	dir := t.TempDir()
	ca := newTestCA(t, "crl-pem-ca")
	der := ca.crlDER(t, time.Now().Add(time.Hour), 42)
	path := writeFile(t, dir, "crl.pem", pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: der}))

	crls, err := LoadCRLs(path)
	require.NoError(t, err)
	require.Len(t, crls, 1)
	require.Len(t, crls[0].RevokedCertificateEntries, 1)
	assert.Equal(t, int64(42), crls[0].RevokedCertificateEntries[0].SerialNumber.Int64())
}

func TestLoadCRLs_DER(t *testing.T) {
	dir := t.TempDir()
	ca := newTestCA(t, "crl-der-ca")
	path := writeFile(t, dir, "crl.der", ca.crlDER(t, time.Now().Add(time.Hour), 7))

	crls, err := LoadCRLs(path)
	require.NoError(t, err)
	require.Len(t, crls, 1)
	assert.Equal(t, int64(7), crls[0].RevokedCertificateEntries[0].SerialNumber.Int64())
}

func TestLoadCRLs_ConcatenatedPEMFromMultipleIssuers(t *testing.T) {
	dir := t.TempDir()
	first := newTestCA(t, "crl-ca-one")
	second := newTestCA(t, "crl-ca-two")

	var contents []byte
	contents = append(contents, pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: first.crlDER(t, time.Now().Add(time.Hour), 1)})...)
	contents = append(contents, pem.EncodeToMemory(&pem.Block{Type: "CRL", Bytes: second.crlDER(t, time.Now().Add(time.Hour), 2)})...)
	path := writeFile(t, dir, "crls.pem", contents)

	crls, err := LoadCRLs(path)
	require.NoError(t, err)
	require.Len(t, crls, 2)
	assert.Contains(t, crls[0].Issuer.String(), "crl-ca-one")
	assert.Contains(t, crls[1].Issuer.String(), "crl-ca-two")
}

func TestLoadCRLs_IgnoresUnrelatedPEMBlocks(t *testing.T) {
	dir := t.TempDir()
	ca := newTestCA(t, "crl-mixed-ca")

	var contents []byte
	contents = append(contents, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.cert.Raw})...)
	contents = append(contents, pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: ca.crlDER(t, time.Now().Add(time.Hour), 3)})...)
	path := writeFile(t, dir, "bundle.pem", contents)

	crls, err := LoadCRLs(path)
	require.NoError(t, err)
	require.Len(t, crls, 1)
}

func TestLoadCRLs_RejectsFileWithNoCRL(t *testing.T) {
	dir := t.TempDir()
	ca := newTestCA(t, "cert-only-ca")
	path := writeFile(t, dir, "cert.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.cert.Raw}))

	_, err := LoadCRLs(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no valid PEM or DER revocation list")
}

func TestLoadCRLs_RejectsMissingFile(t *testing.T) {
	_, err := LoadCRLs("/nonexistent/crl.pem")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read CRL file")
}

func TestCRLReloader_LoadsAtConstruction(t *testing.T) {
	dir := t.TempDir()
	ca := newTestCA(t, "reloader-ca")
	path := writeFile(t, dir, "crl.pem", pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: ca.crlDER(t, time.Now().Add(time.Hour), 5)}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := NewCRLReloader(ctx, path, RealClock())
	crls, err := r.GetCRLs()
	require.NoError(t, err)
	require.Len(t, crls, 1)
}

func TestCRLReloader_InitialLoadFailureReturnsError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := NewCRLReloader(ctx, "/nonexistent/crl.pem", RealClock())
	crls, err := r.GetCRLs()
	assert.Nil(t, crls)
	assert.Error(t, err)
}

func TestCRLReloader_ReloadPicksUpNewRevocations(t *testing.T) {
	dir := t.TempDir()
	ca := newTestCA(t, "rotating-ca")
	path := writeFile(t, dir, "crl.pem", pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: ca.crlDER(t, time.Now().Add(time.Hour), 1)}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := NewCRLReloader(ctx, path, RealClock())
	crls, err := r.GetCRLs()
	require.NoError(t, err)
	require.Len(t, crls[0].RevokedCertificateEntries, 1)

	writeFile(t, dir, "crl.pem", pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: ca.crlDER(t, time.Now().Add(time.Hour), 1, 2)}))
	r.reloadCRLs()

	crls, err = r.GetCRLs()
	require.NoError(t, err)
	assert.Len(t, crls[0].RevokedCertificateEntries, 2, "reload should pick up newly revoked serials")
}

// A revocation list that can no longer be read must stay in effect: discarding
// it would silently re-admit every certificate it revoked.
func TestCRLReloader_ReloadFailurePreservesLastKnownGoodCRLs(t *testing.T) {
	dir := t.TempDir()
	ca := newTestCA(t, "preserve-ca")
	path := writeFile(t, dir, "crl.pem", pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: ca.crlDER(t, time.Now().Add(time.Hour), 9)}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := NewCRLReloader(ctx, path, RealClock())
	require.NoError(t, os.Remove(path))
	r.reloadCRLs()

	crls, err := r.GetCRLs()
	require.NoError(t, err, "should not return an error while last known good CRLs are available")
	require.Len(t, crls, 1)
	assert.Equal(t, int64(9), crls[0].RevokedCertificateEntries[0].SerialNumber.Int64())
}

func TestCRLReloader_ShouldReloadUsesErrorCadenceAfterFailure(t *testing.T) {
	dir := t.TempDir()
	ca := newTestCA(t, "cadence-ca")
	path := writeFile(t, dir, "crl.pem", pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: ca.crlDER(t, time.Now().Add(time.Hour), 1)}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := NewCRLReloader(ctx, path, RealClock())
	assert.False(t, r.shouldReload(), "should not reload immediately after construction")

	require.NoError(t, os.Remove(path))
	r.reloadCRLs()

	r.mu.Lock()
	r.lastUpdate = time.Now().Add(-errorCacheTimeout - time.Minute)
	r.mu.Unlock()

	assert.True(t, r.shouldReload(), "should retry on the error cadence after a failed reload")
}

func TestCRLReloader_ShouldReloadOnFileChange(t *testing.T) {
	dir := t.TempDir()
	ca := newTestCA(t, "mtime-ca")
	path := writeFile(t, dir, "crl.pem", pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: ca.crlDER(t, time.Now().Add(time.Hour), 1)}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := NewCRLReloader(ctx, path, RealClock())

	r.mu.Lock()
	r.lastUpdate = time.Now().Add(-cacheTimeout - time.Minute)
	r.mu.Unlock()
	assert.False(t, r.shouldReload(), "should not reload when the file is unchanged")

	future := time.Now().Add(time.Hour)
	require.NoError(t, os.Chtimes(path, future, future))
	assert.True(t, r.shouldReload(), "should reload once the file changes")
}

// An expired list is still honored; only a warning distinguishes it.
func TestCRLReloader_ExpiredCRLIsStillLoaded(t *testing.T) {
	dir := t.TempDir()
	ca := newTestCA(t, "expired-ca")
	path := writeFile(t, dir, "crl.pem", pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: ca.crlDER(t, time.Now().Add(-time.Minute), 4)}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := NewCRLReloader(ctx, path, RealClock())
	crls, err := r.GetCRLs()
	require.NoError(t, err)
	require.Len(t, crls, 1)
	assert.Equal(t, int64(4), crls[0].RevokedCertificateEntries[0].SerialNumber.Int64())
}
