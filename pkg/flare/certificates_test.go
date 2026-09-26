// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateTestCertificate returns the PEM encoding of a self-signed certificate.
func generateTestCertificate(t *testing.T) []byte {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "flare-test-cert"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	require.NoError(t, err)

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func TestCountPEMCerts(t *testing.T) {
	certPEM := generateTestCertificate(t)
	loaded := make(map[[sha256.Size]byte]struct{})

	// valid certificate
	assert.Equal(t, 1, countPEMCerts(certPEM, loaded))

	// same certificate twice: deduplicated like a CertPool
	assert.Equal(t, 1, countPEMCerts(append(certPEM, certPEM...), loaded))
	assert.Len(t, loaded, 1)

	// non-PEM content
	loaded = make(map[[sha256.Size]byte]struct{})
	assert.Equal(t, 0, countPEMCerts([]byte("not a certificate"), loaded))

	// PEM block of the wrong type
	loaded = make(map[[sha256.Size]byte]struct{})
	other := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("junk")})
	assert.Equal(t, 0, countPEMCerts(other, loaded))
}

func TestWriteCertDirListing(t *testing.T) {
	// Directory listing behavior mirrors Go's Unix root loader and uses
	// symlinks; restrict it to the platforms the provider supports.
	if runtime.GOOS != "linux" && runtime.GOOS != "aix" {
		t.Skipf("skipping on %s", runtime.GOOS)
	}

	dir := t.TempDir()
	certPEM := generateTestCertificate(t)
	// a distinct certificate only present in a nested directory: it must be
	// listed but not counted in the total (Go only reads immediate entries)
	nestedCertPEM := generateTestCertificate(t)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "ca.pem"), certPEM, 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "garbage.txt"), []byte("junk"), 0644))
	require.NoError(t, os.Symlink("ca.pem", filepath.Join(dir, "abc123.0")))                     // same-directory symlink
	require.NoError(t, os.Symlink(filepath.Join(dir, "ca.pem"), filepath.Join(dir, "absolute"))) // symlink with a "/" in target
	require.NoError(t, os.Symlink("/does/not/exist", filepath.Join(dir, "dangling")))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "extra"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "extra", "ca2.pem"), nestedCertPEM, 0644))

	b := new(bytes.Buffer)
	loaded := make(map[[sha256.Size]byte]struct{})
	stats := &certDirStats{}
	writeCertDirListing(b, dir, 0, stats, loaded)

	out := b.String()
	assert.Contains(t, out, dir+": 6 entries")
	assert.Contains(t, out, "ca.pem (1 certificates)")
	assert.Contains(t, out, "garbage.txt (0 certificates)")
	assert.Contains(t, out, "abc123.0 -> ca.pem (skipped: same-directory symlink)")
	assert.Contains(t, out, "absolute -> "+filepath.Join(dir, "ca.pem")+" (1 certificates)")
	assert.Contains(t, out, "dangling -> /does/not/exist (dangling)")
	assert.Contains(t, out, "extra/")
	// the nested certificate is listed but not counted in the total
	assert.Contains(t, out, "ca2.pem (1 certificates)")

	// 3 file entries (ca.pem, garbage.txt, extra/ca2.pem) and 2 directories
	assert.Equal(t, 3, stats.files)
	assert.Equal(t, 2, stats.dirs)
	// unique certificates counted: ca.pem and its absolute symlink target;
	// the distinct nested certificate must not be included
	assert.Len(t, loaded, 1)
}

func TestGetCertificateSourcesReport(t *testing.T) {
	if _, ok := certFilesByOS[runtime.GOOS]; !ok {
		return
	}

	report, err := getCertificateSourcesReport()
	require.NoError(t, err)
	assert.Contains(t, string(report), certFileEnv+": ")
	assert.Contains(t, string(report), certDirEnv+": ")
	assert.Contains(t, string(report), "certificates (used)")
	assert.Contains(t, string(report), "total: ")
}
