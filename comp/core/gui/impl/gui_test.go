// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package guiimpl

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_guiHTTPFlow(t *testing.T) {
	g := &gui{
		auth:         newAuthenticator("test-auth-token", time.Hour),
		intentTokens: map[string]time.Time{},
		address:      "localhost:5002",
	}

	publicRouter := http.NewServeMux()
	publicRouter.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	publicRouter.HandleFunc("GET /auth", g.getAccessToken)

	agentMux := http.NewServeMux()
	agentMux.HandleFunc("POST /ping", func(w http.ResponseWriter, _ *http.Request) {
		ping(w, time.Now().Unix())
	})
	securedMux := http.NewServeMux()
	securedMux.Handle("/agent/", http.StripPrefix("/agent", agentMux))
	publicRouter.Handle("/", g.authMiddleware(securedMux))

	server := httptest.NewServer(publicRouter)
	defer server.Close()
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	client := &http.Client{Jar: jar}

	t.Run("unauthenticated ping is rejected", func(t *testing.T) {
		resp, err := client.Post(server.URL+"/agent/ping", "", nil)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	var intentToken string
	t.Run("intent token sets cookie and redirects to index", func(t *testing.T) {
		rr := httptest.NewRecorder()
		g.getIntentToken(rr, httptest.NewRequest(http.MethodGet, "/gui/intent", nil))
		require.Equal(t, http.StatusOK, rr.Code)
		intentToken = rr.Body.String()
		require.NotEmpty(t, intentToken)

		resp, err := client.Get(server.URL + "/auth?intent=" + intentToken)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "/", resp.Request.URL.Path)
		cookies := jar.Cookies(resp.Request.URL)
		require.Len(t, cookies, 1)
		assert.Equal(t, "accessToken", cookies[0].Name)
		assert.NotEmpty(t, cookies[0].Value)
	})

	t.Run("authenticated ping returns a number", func(t *testing.T) {
		resp, err := client.Post(server.URL+"/agent/ping", "", nil)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Regexp(t, `^[0-9]+$`, string(body))
	})

	t.Run("intent token cannot be reused", func(t *testing.T) {
		resp, err := client.Get(server.URL + "/auth?intent=" + intentToken)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("cross-origin ping is rejected", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodPost, server.URL+"/agent/ping", nil)
		require.NoError(t, err)
		req.Header.Set("Origin", "http://evil.example")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, "invalid origin\n", string(body))
	})
}

func Test_getAccessToken_intentTokenExpiry(t *testing.T) {
	g := &gui{
		auth:         newAuthenticator("test-auth-token", time.Hour),
		intentTokens: make(map[string]time.Time),
	}

	t.Run("valid unexpired token grants access", func(t *testing.T) {
		g.intentMu.Lock()
		g.intentTokens["valid"] = time.Now().Add(time.Minute)
		g.intentMu.Unlock()

		req := httptest.NewRequest(http.MethodGet, "/auth?intent=valid", nil)
		rr := httptest.NewRecorder()
		g.getAccessToken(rr, req)

		assert.Equal(t, http.StatusFound, rr.Code)
	})

	t.Run("expired token is rejected and consumed", func(t *testing.T) {
		g.intentMu.Lock()
		g.intentTokens["expired"] = time.Now().Add(-time.Second)
		g.intentMu.Unlock()

		req := httptest.NewRequest(http.MethodGet, "/auth?intent=expired", nil)
		rr := httptest.NewRecorder()
		g.getAccessToken(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)

		g.intentMu.Lock()
		_, stillPresent := g.intentTokens["expired"]
		g.intentMu.Unlock()
		assert.False(t, stillPresent, "an expired token must still be consumed on use, closing any reuse race")
	})

	t.Run("token cannot be redeemed twice", func(t *testing.T) {
		g.intentMu.Lock()
		g.intentTokens["single-use"] = time.Now().Add(time.Minute)
		g.intentMu.Unlock()

		g.getAccessToken(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/auth?intent=single-use", nil))

		rr := httptest.NewRecorder()
		g.getAccessToken(rr, httptest.NewRequest(http.MethodGet, "/auth?intent=single-use", nil))
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})
}

func Test_getIntentToken_setsExpiryAndPurgesStale(t *testing.T) {
	g := &gui{
		intentTokens: make(map[string]time.Time),
	}
	g.intentTokens["stale"] = time.Now().Add(-time.Minute)

	rr := httptest.NewRecorder()
	g.getIntentToken(rr, httptest.NewRequest(http.MethodGet, "/gui/intent", nil))

	require.Equal(t, http.StatusOK, rr.Code)
	token := rr.Body.String()
	require.NotEmpty(t, token)

	g.intentMu.Lock()
	defer g.intentMu.Unlock()

	_, staleStillPresent := g.intentTokens["stale"]
	assert.False(t, staleStillPresent, "expired intent tokens should be purged whenever a new one is issued")

	expiresAt, ok := g.intentTokens[token]
	require.True(t, ok)
	assert.WithinDuration(t, time.Now().Add(intentTokenTTL), expiresAt, 2*time.Second)
}
