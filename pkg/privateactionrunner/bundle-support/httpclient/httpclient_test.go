// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package httpclient

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
)

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
