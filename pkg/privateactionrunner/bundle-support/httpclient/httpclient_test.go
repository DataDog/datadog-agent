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
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
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

func TestIsSameOriginRedirect(t *testing.T) {
	tests := []struct {
		name     string
		first    string
		target   string
		expected bool
	}{
		{name: "same origin", first: "https://jenkins.example.com/job/a", target: "https://jenkins.example.com/job/a/", expected: true},
		{name: "host case differs", first: "https://Jenkins.Example.com/a", target: "https://jenkins.example.com/b", expected: true},
		{name: "explicit default port", first: "https://jenkins.example.com/a", target: "https://jenkins.example.com:443/b", expected: true},
		{name: "same explicit port", first: "http://10.0.0.5:8080/a", target: "http://10.0.0.5:8080/b", expected: true},
		{name: "http to https upgrade on default ports", first: "http://jenkins.example.com/a", target: "https://jenkins.example.com/a", expected: true},
		{name: "other host", first: "https://jenkins.example.com/a", target: "https://other.example.com/a", expected: false},
		{name: "subdomain", first: "https://example.com/a", target: "https://jenkins.example.com/a", expected: false},
		{name: "loopback", first: "https://jenkins.example.com/a", target: "http://127.0.0.1:8126/info", expected: false},
		{name: "other port", first: "http://127.0.0.1:8080/a", target: "http://127.0.0.1:5001/a", expected: false},
		{name: "https to http", first: "https://jenkins.example.com/a", target: "http://jenkins.example.com/a", expected: false},
		{name: "upgrade to non default port", first: "http://jenkins.example.com/a", target: "https://jenkins.example.com:8443/a", expected: false},
		{name: "upgrade from non default port", first: "http://jenkins.example.com:8080/a", target: "https://jenkins.example.com/a", expected: false},
		{name: "other scheme", first: "https://jenkins.example.com/a", target: "ftp://jenkins.example.com/a", expected: false},
		{name: "ipv6 other port", first: "http://[::1]:8080/a", target: "http://[::1]:5001/a", expected: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			first, err := url.Parse(tt.first)
			require.NoError(t, err)
			target, err := url.Parse(tt.target)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, isSameOriginRedirect(first, target))
		})
	}
}

// newRedirectServers returns an origin server and a second server on another port.
// The origin redirects /away to the second server, /moved to /final on itself and
// /loop to itself.
func newRedirectServers(t *testing.T) (origin *httptest.Server, otherHits *atomic.Int32) {
	otherHits = &atomic.Int32{}
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		otherHits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(other.Close)

	origin = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/away":
			http.Redirect(w, r, other.URL+"/info", http.StatusFound)
		case "/moved":
			http.Redirect(w, r, "/final", http.StatusFound)
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusFound)
		case "/final":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(origin.Close)
	return origin, otherHits
}

func doGet(t *testing.T, client HTTPClient, rawURL string) (int, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

func TestNewRunnerHttpClient_SameOriginRedirectsOnly(t *testing.T) {
	origin, otherHits := newRedirectServers(t)
	client, err := NewRunnerHttpClient(&RunnerHttpClientConfig{SameOriginRedirectsOnly: true})
	require.NoError(t, err)

	_, err = doGet(t, client, origin.URL+"/away")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "different origin")
	assert.Zero(t, otherHits.Load(), "redirect target on another origin must not be requested")

	status, err := doGet(t, client, origin.URL+"/moved")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)

	_, err = doGet(t, client, origin.URL+"/loop")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stopped after 10 redirects")
}

func TestNewRunnerHttpClient_SameOriginRedirectsOnlyHonorsMaxRedirect(t *testing.T) {
	origin, _ := newRedirectServers(t)
	client, err := NewRunnerHttpClient(&RunnerHttpClientConfig{SameOriginRedirectsOnly: true, MaxRedirect: 2})
	require.NoError(t, err)

	status, err := doGet(t, client, origin.URL+"/moved")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)

	_, err = doGet(t, client, origin.URL+"/loop")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stopped after 2 redirects")
}

func TestNewRunnerHttpClient_FollowsCrossOriginRedirectsByDefault(t *testing.T) {
	origin, otherHits := newRedirectServers(t)
	client, err := NewRunnerHttpClient(&RunnerHttpClientConfig{})
	require.NoError(t, err)

	status, err := doGet(t, client, origin.URL+"/away")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, int32(1), otherHits.Load())
}

func TestNewDefaultProvider_WithSameOriginRedirectsOnly(t *testing.T) {
	origin, otherHits := newRedirectServers(t)
	clientConfig := &RunnerHttpClientConfig{}
	provider := NewDefaultProvider(&config.Config{}, WithURLAllowlistDisabled(), WithSameOriginRedirectsOnly())

	defaultClient, err := provider.NewDefaultClient()
	require.NoError(t, err)
	customClient, err := provider.NewClient(clientConfig)
	require.NoError(t, err)
	assert.False(t, clientConfig.SameOriginRedirectsOnly, "caller config must not be modified")

	for name, client := range map[string]HTTPClient{"default client": defaultClient, "custom client": customClient} {
		t.Run(name, func(t *testing.T) {
			_, err := doGet(t, client, origin.URL+"/away")
			require.Error(t, err)

			status, err := doGet(t, client, origin.URL+"/moved")
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, status)
		})
	}
	assert.Zero(t, otherHits.Load())
}

func TestNewDefaultProvider_WithDomainAllowlist(t *testing.T) {
	origin, otherHits := newRedirectServers(t)
	originHost, err := url.Parse(origin.URL)
	require.NoError(t, err)

	t.Run("unset allows every domain", func(t *testing.T) {
		client, err := NewDefaultProvider(&config.Config{}, WithURLAllowlistDisabled(), WithDomainAllowlist()).NewDefaultClient()
		require.NoError(t, err)
		status, err := doGet(t, client, origin.URL+"/final")
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, status)
	})

	t.Run("set refuses unlisted domain before sending", func(t *testing.T) {
		var hits atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			hits.Add(1)
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(server.Close)

		provider := NewDefaultProvider(&config.Config{DomainAllowlist: []string{"jenkins.example.com"}}, WithURLAllowlistDisabled(), WithDomainAllowlist())
		defaultClient, err := provider.NewDefaultClient()
		require.NoError(t, err)
		customClient, err := provider.NewClient(&RunnerHttpClientConfig{})
		require.NoError(t, err)
		for name, client := range map[string]HTTPClient{"default client": defaultClient, "custom client": customClient} {
			t.Run(name, func(t *testing.T) {
				_, err := doGet(t, client, server.URL)
				require.Error(t, err)
				assert.Contains(t, err.Error(), "private_action_runner.domain_allowlist")
			})
		}
		assert.Zero(t, hits.Load())
	})

	t.Run("set refuses redirect to unlisted domain", func(t *testing.T) {
		var targetHits atomic.Int32
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			targetHits.Add(1)
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(target.Close)
		// Same loopback listener, but named by a hostname outside the list.
		renamedTarget := strings.Replace(target.URL, originHost.Hostname(), "localhost", 1)
		redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, renamedTarget, http.StatusFound)
		}))
		t.Cleanup(redirector.Close)

		// Same-origin redirects stay off so only the domain allowlist can refuse the hop.
		provider := NewDefaultProvider(&config.Config{DomainAllowlist: []string{originHost.Hostname()}}, WithURLAllowlistDisabled(), WithDomainAllowlist())
		client, err := provider.NewDefaultClient()
		require.NoError(t, err)

		status, err := doGet(t, client, origin.URL+"/moved")
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, status)

		_, err = doGet(t, client, redirector.URL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `domain "localhost" is not allowed`)
		assert.Zero(t, targetHits.Load())
	})
	assert.Zero(t, otherHits.Load())
}
