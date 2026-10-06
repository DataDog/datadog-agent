// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package guiimpl

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
