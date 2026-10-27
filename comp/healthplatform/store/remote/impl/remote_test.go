// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package remoteimpl

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/version"
)

type remoteTestIdentity struct {
	deploymentID string
	clusterID    string
}

func (i *remoteTestIdentity) DeploymentIDWithContext(context.Context) string { return i.deploymentID }
func (i *remoteTestIdentity) ClusterIDWithContext(context.Context) string    { return i.clusterID }

func newTestRemoteIssueLoader(t *testing.T, resourceID, agentType string, handler http.HandlerFunc) *remoteIssueLoader {
	t.Helper()

	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	reqs := Requires{
		Config: config.NewMockWithOverrides(t, map[string]interface{}{"api_key": " api-key\n"}),
		Log:    logmock.New(t),
	}
	loader := newRemoteIssueLoader(reqs, flavor.DefaultAgent, &remoteTestIdentity{deploymentID: resourceID})
	loader.agentType = agentType
	loader.baseURL = server.URL
	loader.httpClient.Transport = server.Client().Transport
	return loader
}

func snapshotResponse(t *testing.T, resourceID string, issues []remoteIssue) []byte {
	t.Helper()
	data, err := json.Marshal(remoteIssuesResponse{Data: &remoteIssueResource{
		ID:   resourceID,
		Type: remoteIssuesResourceType,
		Attributes: remoteIssueAttributes{
			Issues: &issues,
		},
	}})
	assert.NoError(t, err)
	return data
}

func TestNewRemoteIssueLoaderUsesAPISite(t *testing.T) {
	reqs := Requires{
		Config: config.NewMockWithOverrides(t, map[string]interface{}{
			"site":   "datadoghq.eu",
			"dd_url": "https://metrics.example.com",
		}),
		Log: logmock.New(t),
	}

	loader := newRemoteIssueLoader(reqs, flavor.DefaultAgent, &remoteTestIdentity{deploymentID: "daemonset-uid"})
	assert.Equal(t, "https://api.datadoghq.eu.", loader.baseURL)
}

func TestRemoteIssueLoaderLoad(t *testing.T) {
	const (
		resourceID   = "daemonset/uid with space"
		agentIssueID = "invalid-config:daemonset-uid"
	)

	type capturedRequest struct {
		method     string
		requestURI string
		headers    http.Header
	}
	requestCh := make(chan capturedRequest, 1)
	loader := newTestRemoteIssueLoader(t, resourceID, remoteIssuesNodeAgentType, func(w http.ResponseWriter, r *http.Request) {
		requestCh <- capturedRequest{
			method:     r.Method,
			requestURI: r.RequestURI,
			headers:    r.Header.Clone(),
		}
		_, _ = w.Write(snapshotResponse(t, resourceID, []remoteIssue{
			{IssueID: agentIssueID, IssueName: "Invalid Agent Configuration", IssueType: "invalid_agent_configuration", Category: "configuration"},
			{IssueID: "another-id", IssueName: "Another Issue", IssueType: "another_issue", Category: "availability"},
		}))
	})

	issues, err := loader.Load(context.Background())
	require.NoError(t, err)
	require.Len(t, issues, 2)
	assert.Equal(t, agentIssueID, issues[0].IssueID)
	assert.Equal(t, "configuration", issues[0].Category)

	request := <-requestCh
	assert.Equal(t, http.MethodGet, request.method)
	assert.Equal(t, "/api/v2/agenthealth/hosts/daemonset%2Fuid%20with%20space/issues?agent_type=node", request.requestURI)
	assert.Equal(t, jsonAPIContentType, request.headers.Get("Accept"))
	assert.Equal(t, "api-key", request.headers.Get("DD-API-KEY"))
	assert.Empty(t, request.headers.Get("DD-APPLICATION-KEY"))
	assert.Equal(t, version.AgentVersion, request.headers.Get("DD-Agent-Version"))
	assert.Equal(t, "datadog-agent/"+version.AgentVersion, request.headers.Get("User-Agent"))
}

func TestRemoteIssueLoaderLoadEmpty(t *testing.T) {
	loader := newTestRemoteIssueLoader(t, "daemonset-uid", remoteIssuesNodeAgentType, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(snapshotResponse(t, "daemonset-uid", []remoteIssue{}))
	})

	issues, err := loader.Load(context.Background())
	require.NoError(t, err)
	assert.Empty(t, issues)
}

func TestRemoteIssueLoaderLoadClusterAgent(t *testing.T) {
	requestURI := make(chan string, 1)
	loader := newTestRemoteIssueLoader(t, "cluster-uuid", remoteIssuesClusterAgentType, func(w http.ResponseWriter, r *http.Request) {
		requestURI <- r.RequestURI
		_, _ = w.Write(snapshotResponse(t, "cluster-uuid", []remoteIssue{}))
	})
	loader.agentFlavor = flavor.ClusterAgent
	loader.resourceID = (&remoteTestIdentity{clusterID: "cluster-uuid"}).ClusterIDWithContext

	issues, err := loader.Load(context.Background())
	require.NoError(t, err)
	assert.Empty(t, issues)
	assert.Equal(t, "/api/v2/agenthealth/hosts/cluster-uuid/issues?agent_type=cluster", <-requestURI)
}

func TestRemoteIssueLoaderLoadRequiresResourceID(t *testing.T) {
	loader := newTestRemoteIssueLoader(t, " ", remoteIssuesNodeAgentType, func(http.ResponseWriter, *http.Request) {
		t.Fatal("request should not be sent")
	})

	issues, err := loader.Load(context.Background())
	assert.Nil(t, issues)
	assert.ErrorContains(t, err, "resource ID is required")
}

func TestRemoteIssueLoaderIdentityDeadline(t *testing.T) {
	for _, startupTimeout := range []time.Duration{time.Minute, time.Second} {
		t.Run(startupTimeout.String(), func(t *testing.T) {
			reqs := Requires{
				Config: config.NewMockWithOverrides(t, map[string]interface{}{"api_key": "api-key"}),
				Log:    logmock.New(t),
			}
			synctest.Test(t, func(t *testing.T) {
				calls := 0
				identity := &remoteTestIdentity{deploymentID: "unused"}
				loader := newRemoteIssueLoader(reqs, flavor.DefaultAgent, identity)
				loader.resourceID = func(ctx context.Context) string {
					calls++
					<-ctx.Done()
					return ""
				}
				ctx, cancel := context.WithTimeout(context.Background(), startupTimeout)
				defer cancel()

				start := time.Now()
				issues, err := loader.Load(ctx)
				assert.Nil(t, issues)
				require.ErrorIs(t, err, context.DeadlineExceeded)
				assert.Equal(t, min(10*time.Second, startupTimeout), time.Since(start))
				assert.Equal(t, 1, calls)
			})
		})
	}
}

func TestRemoteIssueLoaderLoadRequiresHTTPS(t *testing.T) {
	loader := newTestRemoteIssueLoader(t, "daemonset-uid", remoteIssuesNodeAgentType, func(http.ResponseWriter, *http.Request) {
		t.Fatal("request should not be sent")
	})
	loader.baseURL = "http://api.example.com"

	issues, err := loader.Load(context.Background())
	assert.Nil(t, issues)
	assert.ErrorContains(t, err, "must use HTTPS")
}

func TestRemoteIssueLoaderLoadResponseErrors(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantError  string
	}{
		{name: "non-OK response", statusCode: http.StatusUnauthorized, body: `{"errors":[]}`, wantError: "status 401"},
		{name: "malformed response", statusCode: http.StatusOK, body: `{"data":`, wantError: "decode remote issue response"},
		{name: "oversized response", statusCode: http.StatusOK, body: strings.Repeat("x", remoteIssuesMaxResponse+1), wantError: "response exceeds"},
		{name: "missing data", statusCode: http.StatusOK, body: `{"data":null}`, wantError: "must contain data"},
		{name: "wrong resource type", statusCode: http.StatusOK, body: `{"data":{"id":"daemonset-uid","type":"other","attributes":{"issues":[]}}}`, wantError: "unexpected type"},
		{name: "wrong resource ID", statusCode: http.StatusOK, body: `{"data":{"id":"other-uid","type":"agent_health_issues","attributes":{"issues":[]}}}`, wantError: "expected \"daemonset-uid\""},
		{name: "missing issues", statusCode: http.StatusOK, body: `{"data":{"id":"daemonset-uid","type":"agent_health_issues","attributes":{}}}`, wantError: "no issues"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loader := newTestRemoteIssueLoader(t, "daemonset-uid", remoteIssuesNodeAgentType, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.statusCode)
				_, _ = w.Write([]byte(test.body))
			})

			issues, err := loader.Load(context.Background())
			assert.Nil(t, issues)
			assert.ErrorContains(t, err, test.wantError)
		})
	}
}

func TestRemoteIssueLoaderLoadCanceledContext(t *testing.T) {
	loader := newTestRemoteIssueLoader(t, "daemonset-uid", remoteIssuesNodeAgentType, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(snapshotResponse(t, "daemonset-uid", []remoteIssue{}))
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	issues, err := loader.Load(ctx)
	assert.Nil(t, issues)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRemoteIssueLoaderLoadDoesNotFollowRedirects(t *testing.T) {
	var requestCount atomic.Int32
	loader := newTestRemoteIssueLoader(t, "daemonset-uid", remoteIssuesNodeAgentType, func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		if r.URL.Path == "/redirect-target" {
			_, _ = w.Write(snapshotResponse(t, "daemonset-uid", []remoteIssue{}))
			return
		}
		http.Redirect(w, r, "/redirect-target", http.StatusFound)
	})

	issues, err := loader.Load(context.Background())
	assert.Nil(t, issues)
	assert.ErrorContains(t, err, "status 302")
	assert.Equal(t, int32(1), requestCount.Load())
}

func TestRemoteIssueLoaderDisabledModes(t *testing.T) {
	tests := []struct {
		name        string
		agentFlavor string
		overrides   map[string]interface{}
	}{
		{name: "unsupported flavor", agentFlavor: flavor.ProcessAgent},
		{name: "missing API key", agentFlavor: flavor.DefaultAgent, overrides: map[string]interface{}{"api_key": ""}},
		{name: "FIPS mode", agentFlavor: flavor.DefaultAgent, overrides: map[string]interface{}{"fips.enabled": true}},
		{name: "TLS verification disabled", agentFlavor: flavor.DefaultAgent, overrides: map[string]interface{}{"skip_ssl_validation": true}},
		{
			name: "Cluster Check Runner", agentFlavor: flavor.DefaultAgent,
			overrides: map[string]interface{}{
				"clc_runner_enabled": true,
				"config_providers":   []map[string]interface{}{{"name": "clusterchecks"}},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := config.NewMockWithOverrides(t, map[string]interface{}{"api_key": "api-key"})
			for key, value := range test.overrides {
				cfg.SetInTest(key, value)
			}
			loader := newRemoteIssueLoader(Requires{Config: cfg, Log: logmock.New(t)}, test.agentFlavor,
				&remoteTestIdentity{deploymentID: "daemonset-uid", clusterID: "cluster-uuid"})

			issues, err := loader.Load(context.Background())
			require.NoError(t, err)
			assert.Nil(t, issues)
		})
	}
}

func TestNewRemoteIssueLoaderSelectsIdentity(t *testing.T) {
	reqs := Requires{
		Config: config.NewMockWithOverrides(t, map[string]interface{}{"api_key": "api-key"}),
		Log:    logmock.New(t),
	}
	identity := &remoteTestIdentity{deploymentID: "daemonset-uid", clusterID: "cluster-uuid"}

	node := newRemoteIssueLoader(reqs, flavor.DefaultAgent, identity)
	assert.Equal(t, remoteIssuesNodeAgentType, node.agentType)
	assert.Equal(t, "daemonset-uid", node.resourceID(context.Background()))

	cluster := newRemoteIssueLoader(reqs, flavor.ClusterAgent, identity)
	assert.Equal(t, remoteIssuesClusterAgentType, cluster.agentType)
	assert.Equal(t, "cluster-uuid", cluster.resourceID(context.Background()))
}
