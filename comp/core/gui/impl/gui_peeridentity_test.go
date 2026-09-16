// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || darwin || windows

package guiimpl

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test_intentToken_peerIdentity exercises the peer-identity binding through
// real loopback TCP connections (as opposed to httptest.NewRequest's
// synthetic, non-loopback RemoteAddr), so that mint and redeem go through
// the real, platform-specific lookupLoopbackPeerIdentity implementation.
// Restricted to platforms that actually implement peer identity resolution:
// on the peeridentity_other.go fallback (e.g. AIX), lookupLoopbackPeerIdentity
// always returns an empty identity by design, which would make every
// assertion below fail even though that fallback behavior is correct.
func Test_intentToken_peerIdentity(t *testing.T) {
	g := &gui{
		auth:         newAuthenticator("test-auth-token", time.Hour),
		intentTokens: make(map[string]intentTokenRecord),
		logger:       logmock.New(t),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /gui/intent", g.getIntentToken)
	mux.HandleFunc("GET /auth", g.getAccessToken)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	g.listener = ts.Listener

	noRedirectClient := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}

	mintToken := func(t *testing.T) (string, intentTokenRecord) {
		t.Helper()
		resp, err := http.Get(ts.URL + "/gui/intent")
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		token := string(body)
		require.NotEmpty(t, token)

		g.intentMu.Lock()
		record := g.intentTokens[token]
		g.intentMu.Unlock()
		return token, record
	}

	t.Run("same OS identity: mint then redeem succeeds", func(t *testing.T) {
		token, record := mintToken(t)
		require.NotEmpty(t, record.identity, "a real loopback connection from this same process should resolve to a real OS identity")

		resp, err := noRedirectClient.Get(ts.URL + "/auth?intent=" + token)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusFound, resp.StatusCode)
	})

	t.Run("mismatched OS identity: redeem is rejected", func(t *testing.T) {
		token, record := mintToken(t)
		require.NotEmpty(t, record.identity)

		record.identity = "not-the-real-identity"
		g.intentMu.Lock()
		g.intentTokens[token] = record
		g.intentMu.Unlock()

		resp, err := noRedirectClient.Get(ts.URL + "/auth?intent=" + token)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("bound identity but unresolvable redeem connection fails closed", func(t *testing.T) {
		token, record := mintToken(t)
		require.NotEmpty(t, record.identity)

		// Bypass the real listener: a synthetic, non-loopback request
		// simulates an environment where redeem-time resolution can't
		// succeed even though a real identity was bound at mint time.
		req := httptest.NewRequest(http.MethodGet, "/auth?intent="+token, nil)
		rr := httptest.NewRecorder()
		g.getAccessToken(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})
}
