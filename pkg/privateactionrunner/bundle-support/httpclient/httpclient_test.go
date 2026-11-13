// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package httpclient

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testCert struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
}

func newTestCert(t *testing.T, cn string, dnsNames []string, notBefore, notAfter time.Time, parent *testCert) *testCert {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	return newTestCertWithKey(t, key, cn, dnsNames, notBefore, notAfter, parent)
}

func newTestCertWithKey(t *testing.T, key *ecdsa.PrivateKey, cn string, dnsNames []string, notBefore, notAfter time.Time, parent *testCert) *testCert {
	t.Helper()
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: cn},
		DNSNames:     dnsNames,
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if len(dnsNames) == 0 {
		tmpl.IsCA = true
		tmpl.BasicConstraintsValid = true
		tmpl.KeyUsage |= x509.KeyUsageCertSign
	}
	signerCert, signerKey := tmpl, key
	if parent != nil {
		signerCert, signerKey = parent.cert, parent.key
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, signerCert, &key.PublicKey, signerKey)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return &testCert{cert: cert, key: key}
}

func TestVerifyAllowingExpiredCertificate(t *testing.T) {
	now := time.Now()
	past, longAgo, future := now.Add(-24*time.Hour), now.Add(-48*time.Hour), now.Add(24*time.Hour)
	const host = "example.test"

	ca := newTestCert(t, "test-ca", nil, longAgo, now.Add(365*24*time.Hour), nil)
	expiredCA := newTestCert(t, "expired-ca", nil, longAgo.Add(-time.Hour), past, nil)
	untrustedCA := newTestCert(t, "untrusted-ca", nil, longAgo, future, nil)
	roots := x509.NewCertPool()
	roots.AddCert(ca.cert)
	roots.AddCert(expiredCA.cert)

	intermediate := newTestCert(t, "intermediate", nil, longAgo, future, ca)
	expiredIntermediate := newTestCert(t, "expired-intermediate", nil, longAgo, past, ca)
	futureIntermediate := newTestCert(t, "future-intermediate", nil, future, future.Add(time.Hour), ca)
	invalidSignatureLeaf := newTestCert(t, host, []string{host}, longAgo, past, ca)
	invalidSignatureLeaf.cert.Signature[0] ^= 1

	// Same subject and key: one issued by the trusted CA and expired since the leaf did, one by an untrusted CA.
	crossKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	crossTrusted := newTestCertWithKey(t, crossKey, "cross-intermediate", nil, longAgo, past.Add(time.Hour), ca)
	crossUntrusted := newTestCertWithKey(t, crossKey, "cross-intermediate", nil, longAgo, future, untrustedCA)
	// Renewed with the same key, but only valid after the leaf expired.
	renewedKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	originalIntermediate := newTestCertWithKey(t, renewedKey, "renewed-intermediate", nil, longAgo, past.Add(time.Minute), ca)
	renewedIntermediate := newTestCertWithKey(t, renewedKey, "renewed-intermediate", nil, past.Add(time.Hour), future, ca)

	tests := []struct {
		name       string
		chain      []*x509.Certificate
		serverName string
		wantErr    string
	}{
		{
			name:       "valid leaf",
			chain:      []*x509.Certificate{newTestCert(t, host, []string{host}, past, future, ca).cert},
			serverName: host,
		},
		{
			name:       "expired leaf from trusted CA",
			chain:      []*x509.Certificate{newTestCert(t, host, []string{host}, longAgo, past, ca).cert},
			serverName: host,
		},
		{
			name: "expired leaf and intermediate valid at leaf expiry",
			chain: []*x509.Certificate{
				newTestCert(t, host, []string{host}, longAgo, past.Add(-time.Hour), expiredIntermediate).cert,
				expiredIntermediate.cert,
			},
			serverName: host,
		},
		{
			name: "expired leaf outliving its intermediate",
			chain: []*x509.Certificate{
				newTestCert(t, host, []string{host}, longAgo, past.Add(time.Hour), expiredIntermediate).cert,
				expiredIntermediate.cert,
			},
			serverName: host,
			wantErr:    "expired",
		},
		{
			name: "expired leaf with cross-signed intermediates",
			chain: []*x509.Certificate{
				newTestCert(t, host, []string{host}, longAgo, past, crossTrusted).cert,
				crossUntrusted.cert,
				crossTrusted.cert,
			},
			serverName: host,
		},
		{
			name: "expired leaf with intermediate renewed after leaf expiry",
			chain: []*x509.Certificate{
				newTestCert(t, host, []string{host}, longAgo, past, originalIntermediate).cert,
				renewedIntermediate.cert,
			},
			serverName: host,
			wantErr:    "not yet valid",
		},
		{
			name: "expired leaf with valid intermediate",
			chain: []*x509.Certificate{
				newTestCert(t, host, []string{host}, longAgo, past, intermediate).cert,
				intermediate.cert,
			},
			serverName: host,
		},
		{
			name: "expired leaf with valid intermediate and stale extra certificate",
			chain: []*x509.Certificate{
				newTestCert(t, host, []string{host}, longAgo, past, intermediate).cert,
				intermediate.cert,
				newTestCert(t, "stale-cross-sign", nil, longAgo.Add(-10*24*time.Hour), longAgo.Add(-5*24*time.Hour), ca).cert,
			},
			serverName: host,
		},
		{
			name: "expired leaf with extra certificates expiring after it",
			chain: []*x509.Certificate{
				newTestCert(t, host, []string{host}, longAgo, past, intermediate).cert,
				intermediate.cert,
				newTestCert(t, "extra-1", nil, longAgo, past.Add(1*time.Hour), ca).cert,
				newTestCert(t, "extra-2", nil, longAgo, past.Add(2*time.Hour), ca).cert,
				newTestCert(t, "extra-3", nil, longAgo, past.Add(3*time.Hour), ca).cert,
				newTestCert(t, "extra-4", nil, longAgo, past.Add(4*time.Hour), ca).cert,
			},
			serverName: host,
		},
		{
			name:       "expired leaf outliving its omitted root",
			chain:      []*x509.Certificate{newTestCert(t, host, []string{host}, longAgo, past.Add(time.Hour), expiredCA).cert},
			serverName: host,
			wantErr:    "expired",
		},
		{
			name: "valid leaf with expired intermediate",
			chain: []*x509.Certificate{
				newTestCert(t, host, []string{host}, longAgo, future, expiredIntermediate).cert,
				expiredIntermediate.cert,
			},
			serverName: host,
			wantErr:    "expired",
		},
		{
			name:       "expired leaf from untrusted CA",
			chain:      []*x509.Certificate{newTestCert(t, host, []string{host}, longAgo, past, untrustedCA).cert},
			serverName: host,
			wantErr:    "unknown authority",
		},
		{
			name: "expired leaf with untrusted peer root",
			chain: []*x509.Certificate{
				newTestCert(t, host, []string{host}, longAgo, past, untrustedCA).cert,
				untrustedCA.cert,
			},
			serverName: host,
			wantErr:    "unknown authority",
		},
		{
			name:       "valid leaf from untrusted CA",
			chain:      []*x509.Certificate{newTestCert(t, host, []string{host}, past, future, untrustedCA).cert},
			serverName: host,
			wantErr:    "unknown authority",
		},
		{
			name:       "expired self-signed leaf",
			chain:      []*x509.Certificate{newTestCert(t, host, []string{host}, longAgo, past, nil).cert},
			serverName: host,
			wantErr:    "unknown authority",
		},
		{
			name:       "expired leaf for another host",
			chain:      []*x509.Certificate{newTestCert(t, "other.test", []string{"other.test"}, longAgo, past, ca).cert},
			serverName: host,
			wantErr:    "valid for other.test, not example.test",
		},
		{
			name:       "valid leaf for another host",
			chain:      []*x509.Certificate{newTestCert(t, "other.test", []string{"other.test"}, past, future, ca).cert},
			serverName: host,
			wantErr:    "valid for other.test, not example.test",
		},
		{
			name:       "not yet valid leaf",
			chain:      []*x509.Certificate{newTestCert(t, host, []string{host}, future, future.Add(time.Hour), ca).cert},
			serverName: host,
			wantErr:    "not yet valid",
		},
		{
			name: "expired leaf with not yet valid intermediate",
			chain: []*x509.Certificate{
				newTestCert(t, host, []string{host}, longAgo, past, futureIntermediate).cert,
				futureIntermediate.cert,
			},
			serverName: host,
			wantErr:    "before",
		},
		{
			name:       "expired leaf with invalid signature",
			chain:      []*x509.Certificate{invalidSignatureLeaf.cert},
			serverName: host,
			wantErr:    "unknown authority",
		},
		{
			name:       "leaf from expired CA",
			chain:      []*x509.Certificate{newTestCert(t, host, []string{host}, past.Add(-time.Hour), past.Add(-time.Minute), expiredCA).cert},
			serverName: host,
		},
		{
			name:       "missing server name",
			chain:      []*x509.Certificate{newTestCert(t, host, []string{host}, longAgo, past, ca).cert},
			serverName: "",
			wantErr:    "requires a DNS hostname",
		},
		{
			name:       "no certificates",
			serverName: host,
			wantErr:    "no certificates",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verify := verifyAllowingExpiredCertificate(roots, func() time.Time { return now })
			err := verify(tls.ConnectionState{PeerCertificates: tt.chain, ServerName: tt.serverName})
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

func TestNewRunnerHttpClientAllowExpiredCertificateStillVerifiesChain(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	urls := []string{server.URL, strings.Replace(server.URL, "127.0.0.1", "localhost", 1)}

	for _, url := range urls {
		for _, allowExpired := range []bool{false, true} {
			client, err := NewRunnerHttpClient(&RunnerHttpClientConfig{
				Transport: &RunnerHttpTransportConfig{AllowExpiredCertificate: allowExpired},
			})
			require.NoError(t, err)
			resp, err := client.Get(url)
			if resp != nil {
				_ = resp.Body.Close()
			}
			assert.ErrorContains(t, err, "certificate", "url=%s allowExpired=%v", url, allowExpired)
		}
	}
}
