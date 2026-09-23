// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package guiimpl

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	sysprobeconfigmock "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
)

func TestGUIAddress(t *testing.T) {
	for _, tt := range []struct {
		name string
		host string
		want string
	}{
		{name: "default", want: "127.0.0.1:0"},
		{name: "localhost", host: "localhost", want: "127.0.0.1:0"},
		{name: "IPv4", host: "127.0.0.1", want: "127.0.0.1:0"},
		{name: "IPv6", host: "::1", want: "[::1]:0"},
		{name: "non-loopback", host: "192.0.2.1"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tokenPath := filepath.Join(t.TempDir(), "auth_token")
			require.NoError(t, os.WriteFile(tokenPath, []byte("test-auth-token"), 0600))
			cfg := config.NewMockWithOverrides(t, map[string]interface{}{
				"GUI_port":             0,
				"auth_token_file_path": tokenPath,
			})
			if tt.host != "" {
				cfg.SetInTest("GUI_host", tt.host)
			}
			lc := compdef.NewTestLifecycle(t)
			provides := NewComponent(Requires{
				Config:         cfg,
				Log:            logmock.New(t),
				Lc:             lc,
				Ipc:            ipcmock.New(t),
				SysprobeConfig: sysprobeconfigmock.NewMock(t),
			})
			component, ok := provides.Comp.Get()
			if tt.want == "" {
				require.False(t, ok)
				lc.AssertHooksNumber(0)
				return
			}
			require.True(t, ok)
			g := component.(*gui)
			require.Equal(t, tt.want, g.address)

			// Exercise the default address through the component's lifecycle.
			// Explicit IPv6 configuration is checked above without requiring IPv6 support.
			if tt.want == "127.0.0.1:0" {
				t.Cleanup(func() { require.NoError(t, lc.Stop(context.Background())) })
				require.NoError(t, lc.Start(context.Background()))
				require.NotNil(t, g.listener)
				require.Equal(t, "127.0.0.1", g.listener.Addr().(*net.TCPAddr).IP.String())
				client := &http.Client{Timeout: 5 * time.Second}
				resp, err := client.Get("http://" + g.listener.Addr().String() + "/")
				require.NoError(t, err)
				defer resp.Body.Close()
				require.Equal(t, http.StatusOK, resp.StatusCode)
			}
		})
	}
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
