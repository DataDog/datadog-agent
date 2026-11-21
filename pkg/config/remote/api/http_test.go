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
	"sync"
	"testing"
	"time"

	"github.com/coreos/go-semver/semver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

func TestUpdateWorkloadToken(t *testing.T) {
	agentConfig := mock.New(t)
	agentConfig.SetInTest("skip_ssl_validation", true)
	agentConfig.SetInTest("remote_configuration.no_tls_validation", true)
	agentConfig.SetInTest("remote_configuration.no_tls", true)

	var mu sync.Mutex
	var receivedHeaders []http.Header
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedHeaders = append(receivedHeaders, r.Header.Clone())
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	baseURL, err := url.Parse(ts.URL)
	require.NoError(t, err)

	client, err := NewHTTPClient(Auth{APIKey: "managed-intake-key"}, agentConfig, baseURL)
	require.NoError(t, err)

	ctx := t.Context()

	// Initially absent while DD-Api-Key is present.
	require.Empty(t, client.Headers().Get("DD-Workload-Token"))
	_, err = client.FetchOrgStatus(ctx)
	require.NoError(t, err)

	// Setting workload token attaches DD-Workload-Token across RC endpoints while keeping DD-Api-Key intact.
	client.UpdateWorkloadToken("wt-1")
	require.Equal(t, "wt-1", client.Headers().Get("DD-Workload-Token"))
	_, err = client.FetchOrgStatus(ctx)
	require.NoError(t, err)
	_, err = client.FetchOrgData(ctx)
	require.NoError(t, err)
	_, err = client.Fetch(ctx, &pbgo.LatestConfigsRequest{})
	require.NoError(t, err)

	// Updating API key preserves DD-Workload-Token.
	client.UpdateAPIKey("rotated-intake-key")
	_, err = client.FetchOrgStatus(ctx)
	require.NoError(t, err)

	// Clearing workload token removes the header completely while keeping DD-Api-Key intact.
	client.UpdateWorkloadToken("")
	require.Empty(t, client.Headers().Get("DD-Workload-Token"))
	_, err = client.FetchOrgStatus(ctx)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, receivedHeaders, 6)

	assert.Empty(t, receivedHeaders[0].Get("DD-Workload-Token"))
	assert.Equal(t, "managed-intake-key", receivedHeaders[0].Get("DD-Api-Key"))

	for i := 1; i <= 3; i++ {
		assert.Equal(t, "wt-1", receivedHeaders[i].Get("DD-Workload-Token"))
		assert.Equal(t, "managed-intake-key", receivedHeaders[i].Get("DD-Api-Key"))
	}

	assert.Equal(t, "wt-1", receivedHeaders[4].Get("DD-Workload-Token"))
	assert.Equal(t, "rotated-intake-key", receivedHeaders[4].Get("DD-Api-Key"))

	_, present := receivedHeaders[5][http.CanonicalHeaderKey("DD-Workload-Token")]
	assert.False(t, present, "cleared workload token header should be removed")
	assert.Equal(t, "rotated-intake-key", receivedHeaders[5].Get("DD-Api-Key"))
}

func TestUpdateWorkloadTokenConcurrent(t *testing.T) {
	agentConfig := mock.New(t)
	agentConfig.SetInTest("skip_ssl_validation", true)
	agentConfig.SetInTest("remote_configuration.no_tls_validation", true)
	agentConfig.SetInTest("remote_configuration.no_tls", true)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	baseURL, err := url.Parse(ts.URL)
	require.NoError(t, err)

	client, err := NewHTTPClient(Auth{APIKey: "intake-key"}, agentConfig, baseURL)
	require.NoError(t, err)

	ctx := t.Context()
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 25; j++ {
				if (id+j)%2 == 0 {
					client.UpdateWorkloadToken("wt-token")
				} else {
					client.UpdateWorkloadToken("")
				}
				_ = client.Headers()
			}
		}(i)
	}
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				_, _ = client.FetchOrgStatus(ctx)
			}
		}()
	}
	wg.Wait()
}
