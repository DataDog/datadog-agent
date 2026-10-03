// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package guiimpl

import (
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/gui/bootstrap"
)

// authRequest builds the POST the bootstrap page makes to /auth.
func authRequest(id, secret string) *http.Request {
	form := url.Values{"id": {id}, "secret": {secret}}
	req := httptest.NewRequest(http.MethodPost, "/auth", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func (g *gui) putIntentToken(id, secret string, expiresAt time.Time) {
	g.intentMu.Lock()
	defer g.intentMu.Unlock()
	g.intentTokens[id] = intentEntry{
		secretHash: sha256.Sum256([]byte(secret)),
		expiresAt:  expiresAt,
	}
}

func (g *gui) hasIntentToken(id string) bool {
	g.intentMu.Lock()
	defer g.intentMu.Unlock()
	_, ok := g.intentTokens[id]
	return ok
}

func Test_getAccessToken(t *testing.T) {
	newGUI := func() *gui {
		return &gui{
			auth:         newAuthenticator("test-auth-token", time.Hour),
			intentTokens: make(map[string]intentEntry),
		}
	}

	t.Run("valid unexpired token grants access", func(t *testing.T) {
		g := newGUI()
		g.putIntentToken("valid", "s3cret", time.Now().Add(time.Minute))

		rr := httptest.NewRecorder()
		g.getAccessToken(rr, authRequest("valid", "s3cret"))

		require.Equal(t, http.StatusSeeOther, rr.Code)

		cookies := rr.Result().Cookies()
		require.Len(t, cookies, 1)
		assert.Equal(t, "accessToken", cookies[0].Name)
		assert.NoError(t, g.auth.ValidateToken(cookies[0].Value))
	})

	t.Run("expired token is rejected and consumed", func(t *testing.T) {
		g := newGUI()
		g.putIntentToken("expired", "s3cret", time.Now().Add(-time.Second))

		rr := httptest.NewRecorder()
		g.getAccessToken(rr, authRequest("expired", "s3cret"))

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.False(t, g.hasIntentToken("expired"), "an expired token must still be consumed on use, closing any reuse race")
	})

	t.Run("token cannot be redeemed twice", func(t *testing.T) {
		g := newGUI()
		g.putIntentToken("single-use", "s3cret", time.Now().Add(time.Minute))

		g.getAccessToken(httptest.NewRecorder(), authRequest("single-use", "s3cret"))

		rr := httptest.NewRecorder()
		g.getAccessToken(rr, authRequest("single-use", "s3cret"))
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("unknown id is rejected", func(t *testing.T) {
		g := newGUI()

		rr := httptest.NewRecorder()
		g.getAccessToken(rr, authRequest("nope", "s3cret"))
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("wrong secret is rejected and consumes the token", func(t *testing.T) {
		g := newGUI()
		g.putIntentToken("guessed", "s3cret", time.Now().Add(time.Minute))

		rr := httptest.NewRecorder()
		g.getAccessToken(rr, authRequest("guessed", "wrong"))

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.Empty(t, rr.Result().Cookies())
		assert.False(t, g.hasIntentToken("guessed"), "a token presented with a wrong secret must not stay redeemable")
	})

	t.Run("missing fields are rejected", func(t *testing.T) {
		g := newGUI()

		for _, tc := range []struct{ id, secret string }{
			{"", "s3cret"},
			{"valid", ""},
			{"", ""},
		} {
			rr := httptest.NewRecorder()
			g.getAccessToken(rr, authRequest(tc.id, tc.secret))
			assert.Equal(t, http.StatusUnauthorized, rr.Code)
		}
	})

	// The whole point of VULN-92705: a token in the URL would be readable in
	// the argv of the OS URL-opener, so /auth must not honour one.
	t.Run("a token in the query string is ignored", func(t *testing.T) {
		g := newGUI()
		g.putIntentToken("in-url", "s3cret", time.Now().Add(time.Minute))

		req := httptest.NewRequest(http.MethodPost, "/auth?id=in-url&secret=s3cret&intent=in-url", nil)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		rr := httptest.NewRecorder()
		g.getAccessToken(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.Empty(t, rr.Result().Cookies())
		assert.True(t, g.hasIntentToken("in-url"), "a query-string token must not even be looked up")
	})

	// The bootstrap page is a file:// document, so it posts with "Origin: null".
	t.Run("an opaque origin is accepted", func(t *testing.T) {
		g := newGUI()
		g.putIntentToken("from-file", "s3cret", time.Now().Add(time.Minute))

		req := authRequest("from-file", "s3cret")
		req.Header.Set("Origin", "null")

		rr := httptest.NewRecorder()
		g.getAccessToken(rr, req)

		assert.Equal(t, http.StatusSeeOther, rr.Code)
	})
}

func Test_rejectLegacyAuth(t *testing.T) {
	rr := httptest.NewRecorder()
	rejectLegacyAuth(rr, httptest.NewRequest(http.MethodGet, "/auth?intent=whatever", nil))

	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	assert.Equal(t, http.MethodPost, rr.Header().Get("Allow"))
	assert.Empty(t, rr.Result().Cookies())
}

func Test_getIntentToken_setsExpiryAndPurgesStale(t *testing.T) {
	g := &gui{
		intentTokens: make(map[string]intentEntry),
	}
	g.putIntentToken("stale", "s3cret", time.Now().Add(-time.Minute))

	rr := httptest.NewRecorder()
	g.getIntentToken(rr, httptest.NewRequest(http.MethodGet, "/gui/intent", nil))

	require.Equal(t, http.StatusOK, rr.Code)

	var token bootstrap.Token
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &token))
	require.NotEmpty(t, token.ID)
	require.NotEmpty(t, token.Secret)
	assert.NotEqual(t, token.ID, token.Secret)

	assert.False(t, g.hasIntentToken("stale"), "expired intent tokens should be purged whenever a new one is issued")

	g.intentMu.Lock()
	defer g.intentMu.Unlock()

	entry, ok := g.intentTokens[token.ID]
	require.True(t, ok)
	assert.WithinDuration(t, time.Now().Add(intentTokenTTL), entry.expiresAt, 2*time.Second)
	assert.Equal(t, sha256.Sum256([]byte(token.Secret)), entry.secretHash)
	assert.NotContains(t, rr.Body.String(), string(entry.secretHash[:]), "the stored secret must be a hash, not the secret itself")
}

func Test_getIntentToken_issuesDistinctTokens(t *testing.T) {
	g := &gui{intentTokens: make(map[string]intentEntry)}

	seen := make(map[string]struct{})
	for i := 0; i < 10; i++ {
		rr := httptest.NewRecorder()
		g.getIntentToken(rr, httptest.NewRequest(http.MethodGet, "/gui/intent", nil))
		require.Equal(t, http.StatusOK, rr.Code)

		var token bootstrap.Token
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &token))

		_, dup := seen[token.ID]
		require.False(t, dup, "intent token ids must not repeat")
		seen[token.ID] = struct{}{}
	}
}
