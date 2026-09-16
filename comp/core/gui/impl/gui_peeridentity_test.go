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
	"os"
	"testing"
	"time"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test_intentToken_peerIdentity exercises peer-identity binding through real loopback TCP connections (unlike httptest.NewRequest's synthetic RemoteAddr), so mint/redeem go through the real, platform-specific lookupLoopbackPeerIdentity; restricted to platforms that implement it, since the peeridentity_noop.go fallback (e.g. AIX) always returns an empty identity by design.
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
		if os.Getuid() == 0 {
			// elevatedMintIdentity's resolution is platform-specific (see its per-OS implementations); cross-check against it directly rather than assuming a single fixed outcome for root.
			assert.Equal(t, mintTimeIdentity(rootIdentity), record.identity, "root's mint-time identity should match this platform's elevatedMintIdentity")
		} else {
			require.NotEmpty(t, record.identity, "a real loopback connection from this same process should resolve to a real OS identity")
		}

		resp, err := noRedirectClient.Get(ts.URL + "/auth?intent=" + token)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusFound, resp.StatusCode)
	})

	t.Run("mismatched OS identity: redeem is rejected", func(t *testing.T) {
		// The identity is forced directly so this test is independent of the test process's own OS identity (see mintTimeIdentity).
		token, record := mintToken(t)
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
		// The identity is forced directly for the same reason as above.
		token, record := mintToken(t)
		record.identity = "some-bound-identity"
		g.intentMu.Lock()
		g.intentTokens[token] = record
		g.intentMu.Unlock()

		// Bypass the real listener: a synthetic, non-loopback request simulates redeem-time resolution failing despite a real identity bound at mint time.
		req := httptest.NewRequest(http.MethodGet, "/auth?intent="+token, nil)
		rr := httptest.NewRecorder()
		g.getAccessToken(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})
}
