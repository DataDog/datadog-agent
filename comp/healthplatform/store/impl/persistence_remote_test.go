// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package storeimpl

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	storedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/version"
)

type remoteTestIdentity struct {
	deploymentID string
	clusterID    string
}

func (i *remoteTestIdentity) DeploymentID() string { return i.deploymentID }
func (i *remoteTestIdentity) ClusterID() string    { return i.clusterID }

func newTestRemoteIssueLoader(t *testing.T, resourceID, agentType string, handler http.HandlerFunc) *remoteIssueLoader {
	t.Helper()

	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"api_key": " api-key\n",
	})
	loader := newRemoteIssueLoader(cfg, agentType, func() string { return resourceID })
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
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"site":   "datadoghq.eu",
		"dd_url": "https://metrics.example.com",
	})

	loader := newRemoteIssueLoader(cfg, remoteIssuesNodeAgentType, func() string { return "daemonset-uid" })
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
		_, _ = w.Write(snapshotResponse(t, resourceID,
			[]remoteIssue{
				{IssueID: agentIssueID, IssueName: "Invalid Agent Configuration", IssueType: "invalid_agent_configuration", Category: "configuration"},
				{IssueID: "another-id", IssueName: "Another Issue", IssueType: "another_issue", Category: "availability"},
			}))
	})

	snapshot, err := loader.load(context.Background())
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	assert.Len(t, snapshot, 2)
	assert.Contains(t, snapshot, agentIssueID)
	assert.Contains(t, snapshot, "another-id")
	assert.Equal(t, "configuration", snapshot[agentIssueID].Category)

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

	snapshot, err := loader.load(context.Background())
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	assert.Empty(t, snapshot)
}

func TestLoadFromRemoteRestoresReconciliationState(t *testing.T) {
	const issueID = "invalid-config:daemonset-uid"
	loader := newTestRemoteIssueLoader(t, "daemonset-uid", remoteIssuesNodeAgentType, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(snapshotResponse(t, "daemonset-uid", []remoteIssue{
			{IssueID: issueID, IssueName: "Invalid Agent Configuration", IssueType: "invalid_agent_configuration", Category: "configuration"},
		}))
	})

	h := newTestStore(t)
	h.persistence = &noopPersistence{}
	h.remoteLoader = loader
	resolvedCh := make(chan *healthplatformpayload.Issue, 1)
	h.RegisterIssuesObserver(storedef.IssuesObserver{ResolvedCh: resolvedCh})

	require.NoError(t, h.start(context.Background()))
	assert.Equal(t, []string{issueID}, h.GetActiveIssueIDsByIssueName("Invalid Agent Configuration"))

	persisted := h.persistedIssues[issueID]
	require.NotNil(t, persisted)
	assert.Equal(t, "Invalid Agent Configuration", persisted.IssueType)
	assert.Equal(t, "invalid_agent_configuration", persisted.ProtoIssueType)
	assert.Equal(t, "configuration", persisted.Category)
	assert.Equal(t, IssueStateActive, persisted.State)
	assert.NotEmpty(t, persisted.FirstSeen)
	assert.NotEmpty(t, persisted.LastSeen)

	h.ResolveIssue(issueID)
	require.Len(t, resolvedCh, 1)
	resolved := <-resolvedCh
	assert.Equal(t, issueID, resolved.Id)
	assert.Equal(t, "Invalid Agent Configuration", resolved.IssueName)
	assert.Equal(t, "invalid_agent_configuration", resolved.IssueType)
	assert.Equal(t, "configuration", resolved.Category)
	require.NotNil(t, resolved.PersistedIssue)
	assert.Equal(t, IssueStateResolved, resolved.PersistedIssue.State)
}

func TestRemoteIssueLoaderLoadClusterAgent(t *testing.T) {
	requestURI := make(chan string, 1)
	loader := newTestRemoteIssueLoader(t, "cluster-uuid", remoteIssuesClusterAgentType, func(w http.ResponseWriter, r *http.Request) {
		requestURI <- r.RequestURI
		_, _ = w.Write(snapshotResponse(t, "cluster-uuid", []remoteIssue{}))
	})

	snapshot, err := loader.load(context.Background())
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	assert.Equal(t, "/api/v2/agenthealth/hosts/cluster-uuid/issues?agent_type=cluster", <-requestURI)
}

func TestRemoteIssueLoaderLoadRequiresAPIKey(t *testing.T) {
	loader := &remoteIssueLoader{
		config:     config.NewMock(t),
		resourceID: func() string { return "daemonset-uid" },
	}

	snapshot, err := loader.load(context.Background())
	assert.Nil(t, snapshot)
	assert.ErrorContains(t, err, "API key is required")
}

func TestRemoteIssueLoaderLoadRequiresResourceID(t *testing.T) {
	loader := newTestRemoteIssueLoader(t, " ", remoteIssuesNodeAgentType, func(http.ResponseWriter, *http.Request) {
		t.Fatal("request should not be sent")
	})

	snapshot, err := loader.load(context.Background())
	assert.Nil(t, snapshot)
	assert.ErrorContains(t, err, "resource ID is required")
}

func TestRemoteIssueLoaderLoadRequiresHTTPS(t *testing.T) {
	loader := newTestRemoteIssueLoader(t, "daemonset-uid", remoteIssuesNodeAgentType, func(http.ResponseWriter, *http.Request) {
		t.Fatal("request should not be sent")
	})
	loader.baseURL = "http://api.example.com"

	snapshot, err := loader.load(context.Background())
	assert.Nil(t, snapshot)
	assert.ErrorContains(t, err, "must use HTTPS")
}

func TestRemoteIssueLoaderLoadResponseErrors(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantError  string
	}{
		{
			name:       "non-OK response",
			statusCode: http.StatusUnauthorized,
			body:       `{"errors":[]}`,
			wantError:  "status 401",
		},
		{
			name:       "malformed response",
			statusCode: http.StatusOK,
			body:       `{"data":`,
			wantError:  "decode remote issue response",
		},
		{
			name:       "oversized response",
			statusCode: http.StatusOK,
			body:       strings.Repeat("x", remoteIssuesMaxResponse+1),
			wantError:  "response exceeds",
		},
		{
			name:       "missing data",
			statusCode: http.StatusOK,
			body:       `{"data":null}`,
			wantError:  "must contain data",
		},
		{
			name:       "wrong resource type",
			statusCode: http.StatusOK,
			body:       `{"data":{"id":"daemonset-uid","type":"other","attributes":{"issues":[]}}}`,
			wantError:  "unexpected type",
		},
		{
			name:       "wrong resource ID",
			statusCode: http.StatusOK,
			body:       `{"data":{"id":"other-uid","type":"agent_health_issues","attributes":{"issues":[]}}}`,
			wantError:  "expected \"daemonset-uid\"",
		},
		{
			name:       "missing issues",
			statusCode: http.StatusOK,
			body:       `{"data":{"id":"daemonset-uid","type":"agent_health_issues","attributes":{}}}`,
			wantError:  "no issues",
		},
		{
			name:       "empty issue ID",
			statusCode: http.StatusOK,
			body:       `{"data":{"id":"daemonset-uid","type":"agent_health_issues","attributes":{"issues":[{"issue_id":"","issue_name":"First Issue","issue_type":"first_issue","category":"configuration"}]}}}`,
			wantError:  "empty issue ID",
		},
		{
			name:       "empty issue name",
			statusCode: http.StatusOK,
			body:       `{"data":{"id":"daemonset-uid","type":"agent_health_issues","attributes":{"issues":[{"issue_id":"issue-1","issue_name":"","issue_type":"first_issue","category":"configuration"}]}}}`,
			wantError:  "no issue_name",
		},
		{
			name:       "empty issue type",
			statusCode: http.StatusOK,
			body:       `{"data":{"id":"daemonset-uid","type":"agent_health_issues","attributes":{"issues":[{"issue_id":"issue-1","issue_name":"First Issue","issue_type":"","category":"configuration"}]}}}`,
			wantError:  "no issue_type",
		},
		{
			name:       "empty category",
			statusCode: http.StatusOK,
			body:       `{"data":{"id":"daemonset-uid","type":"agent_health_issues","attributes":{"issues":[{"issue_id":"issue-1","issue_name":"First Issue","issue_type":"first_issue","category":""}]}}}`,
			wantError:  "no category",
		},
		{
			name:       "duplicate issue ID",
			statusCode: http.StatusOK,
			body:       `{"data":{"id":"daemonset-uid","type":"agent_health_issues","attributes":{"issues":[{"issue_id":"issue-1","issue_name":"First Issue","issue_type":"first_issue","category":"configuration"},{"issue_id":"issue-1","issue_name":"First Issue","issue_type":"first_issue","category":"configuration"}]}}}`,
			wantError:  "duplicate issue ID",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loader := newTestRemoteIssueLoader(t, "daemonset-uid", remoteIssuesNodeAgentType, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.statusCode)
				_, _ = w.Write([]byte(test.body))
			})

			snapshot, err := loader.load(context.Background())
			assert.Nil(t, snapshot)
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

	snapshot, err := loader.load(ctx)
	assert.Nil(t, snapshot)
	require.ErrorIs(t, err, context.Canceled)
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

	snapshot, err := loader.load(context.Background())
	assert.Nil(t, snapshot)
	assert.ErrorContains(t, err, "status 302")
	assert.Equal(t, int32(1), requestCount.Load())
}

func TestNewRemoteIssueLoaderIfEnabled(t *testing.T) {
	tests := []struct {
		name           string
		agentFlavor    string
		remoteEnabled  bool
		overrides      map[string]interface{}
		wantAgentType  string
		wantResourceID string
	}{
		{name: "without long-running marker", agentFlavor: flavor.DefaultAgent},
		{name: "node Agent", agentFlavor: flavor.DefaultAgent, remoteEnabled: true, wantAgentType: "node", wantResourceID: "daemonset-uid"},
		{name: "Cluster Agent", agentFlavor: flavor.ClusterAgent, remoteEnabled: true, wantAgentType: "cluster", wantResourceID: "cluster-uuid"},
		{name: "missing API key", agentFlavor: flavor.DefaultAgent, remoteEnabled: true, overrides: map[string]interface{}{"api_key": ""}},
		{name: "FIPS proxy", agentFlavor: flavor.DefaultAgent, remoteEnabled: true, overrides: map[string]interface{}{"fips.enabled": true}},
		{name: "TLS verification disabled", agentFlavor: flavor.DefaultAgent, remoteEnabled: true, overrides: map[string]interface{}{"skip_ssl_validation": true}},
		{
			name: "Cluster Check Runner", agentFlavor: flavor.DefaultAgent, remoteEnabled: true,
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
			reqs := Requires{
				Config: cfg,
				Log:    logmock.New(t),
			}
			if test.remoteEnabled {
				reqs.RemoteRestoration = &storedef.RemoteRestorationParams{Enabled: true}
			}

			identity := &remoteTestIdentity{deploymentID: "daemonset-uid", clusterID: "cluster-uuid"}
			loader := newRemoteIssueLoaderIfEnabled(reqs, test.agentFlavor, identity)
			if test.wantAgentType != "" {
				require.NotNil(t, loader)
				assert.Equal(t, test.wantAgentType, loader.agentType)
				assert.Equal(t, test.wantResourceID, loader.resourceID())
			} else {
				assert.Nil(t, loader)
			}
		})
	}
}
