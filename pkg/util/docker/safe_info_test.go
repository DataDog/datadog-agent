// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/swarm"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dockerHostFromTestServer rewrites a httptest server URL ("http://127.0.0.1:PORT")
// to the form expected by moby's WithHost option ("tcp://127.0.0.1:PORT").
func dockerHostFromTestServer(serverURL string) string {
	return "tcp://" + strings.TrimPrefix(serverURL, "http://")
}

func newTestDockerClient(t *testing.T, serverURL string) *client.Client {
	t.Helper()
	cli, err := client.New(client.WithHost(dockerHostFromTestServer(serverURL)))
	require.NoError(t, err)
	return cli
}

// fakeDockerDaemon is a minimal stand-in for the moby daemon that answers
// /_ping (used by the client for version negotiation) and any /info path
// (possibly version-prefixed, e.g. /v1.54/info) with the supplied body.
func fakeDockerDaemon(t *testing.T, infoBody string, infoStatus int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/_ping":
			w.Header().Set("API-Version", "1.54")
			w.Header().Set("OSType", "linux")
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/info"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(infoStatus)
			if infoStatus == http.StatusOK {
				fmt.Fprint(w, infoBody)
			}
		default:
			t.Errorf("unexpected request path %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestSafeInfo_HappyPath(t *testing.T) {
	body := `{
		"Name": "test-daemon",
		"ServerVersion": "29.0.1",
		"Swarm": {"LocalNodeState": "active", "ControlAvailable": true}
	}`
	server := fakeDockerDaemon(t, body, http.StatusOK)
	defer server.Close()

	cli := newTestDockerClient(t, server.URL)
	info, err := safeInfo(t.Context(), cli)
	require.NoError(t, err)
	assert.Equal(t, "test-daemon", info.Name)
	assert.Equal(t, "29.0.1", info.ServerVersion)
	assert.Equal(t, swarm.LocalNodeStateActive, info.Swarm.LocalNodeState)
	assert.True(t, info.Swarm.ControlAvailable)
}

func TestSafeInfo_FallbackOnInvalidPrefix(t *testing.T) {
	// Reproduces the failure mode reported in incident #54830: the daemon emits
	// "invalid Prefix" (netip.Prefix.String() of an invalid prefix) for the
	// Base field of a DefaultAddressPools entry, which trips moby v29's strict
	// netip.Prefix decoding and fails the entire /info JSON decode.
	body := `{
		"Name": "broken-daemon",
		"ServerVersion": "28.5.0",
		"Swarm": {"LocalNodeState": "inactive", "ControlAvailable": false},
		"DefaultAddressPools": [
			{"Base": "invalid Prefix", "Size": 0}
		]
	}`
	server := fakeDockerDaemon(t, body, http.StatusOK)
	defer server.Close()

	cli := newTestDockerClient(t, server.URL)
	info, err := safeInfo(t.Context(), cli)
	require.NoError(t, err)
	assert.Equal(t, "broken-daemon", info.Name)
	assert.Equal(t, "28.5.0", info.ServerVersion)
	assert.Equal(t, swarm.LocalNodeStateInactive, info.Swarm.LocalNodeState)
	// The problematic field is intentionally dropped by the tolerant decoder.
	assert.Empty(t, info.DefaultAddressPools)
}

func TestSafeInfo_PropagatesNonDecodeErrors(t *testing.T) {
	// The fallback only kicks in for JSON-decode failures of /info. A daemon
	// returning a non-2xx status must propagate the original SDK error without
	// engaging the fallback.
	server := fakeDockerDaemon(t, "", http.StatusInternalServerError)
	defer server.Close()

	cli := newTestDockerClient(t, server.URL)
	_, err := safeInfo(t.Context(), cli)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "tolerant /info fallback")
}
