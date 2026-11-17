// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"testing"
	"time"

	"github.com/coreos/go-semver/semver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

func TestUserAgent(t *testing.T) {
	assert := assert.New(t)
	agentConfig := mock.New(t)

	// TLS test uses bogus certs
	agentConfig.SetInTest("skip_ssl_validation", true)                    // Transport
	agentConfig.SetInTest("remote_configuration.no_tls_validation", true) // RC check
	agentConfig.SetInTest("remote_configuration.no_tls", true)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	userAgentCh := make(chan string, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		userAgentCh <- r.UserAgent()
	}))
	defer ts.Close()

	url, err := url.Parse(ts.URL)
	assert.NoError(err)

	client, err := NewHTTPClient(Auth{}, agentConfig, url)
	assert.NoError(err)

	_, _ = client.FetchOrgData(ctx)

	select {
	case ua := <-userAgentCh:
		// Regex explained:
		//   * ^datadog-agent\/ == must start with "datadog-agent/"
		//   * (.+) == agent version: either "unknown" or a semver string
		//   * \(go...\)$ == ends with the Go toolchain version in parens, which is
		//     either a stable release (go1.26.4) or a release candidate (go1.27rc1).
		uaRegex := regexp.MustCompile(`^datadog-agent\/(.+) \(go\d+\.\d+(?:\.\d+|rc\d+)\)$`)
		parts := uaRegex.FindStringSubmatch(ua)
		assert.Len(parts, 2) // Original string + the extracted group.

		// The extracted string must match either "unknown" or be valid semver.
		switch parts[1] {
		case "unknown":
		default:
			_, err = semver.NewVersion(parts[1])
			assert.NoError(err)
		}

	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for user agent string")
	}
}

func TestWithHTTPClientUsesCallerTLSConfiguration(t *testing.T) {
	want := &pbgo.OrgDataResponse{Uuid: "test-org"}
	body, err := proto.Marshal(want)
	require.NoError(t, err)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, []string{"collector-secret"}, r.Header.Values("DD-API-KEY"))
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	endpoint, err := url.Parse(server.URL)
	require.NoError(t, err)
	supplied := server.Client()
	t.Cleanup(supplied.CloseIdleConnections)
	client, err := NewHTTPClient(Auth{APIKey: "collector-secret"}, mock.New(t), endpoint, WithHTTPClient(supplied))
	require.NoError(t, err)
	got, err := client.FetchOrgData(t.Context())
	require.NoError(t, err)
	require.True(t, proto.Equal(want, got))
}

func TestWithHTTPClientRejectsNil(t *testing.T) {
	endpoint, err := url.Parse("https://config.datadoghq.com")
	require.NoError(t, err)
	_, err = NewHTTPClient(Auth{}, mock.New(t), endpoint, WithHTTPClient(nil))
	require.ErrorContains(t, err, "HTTP client must not be nil")
}
