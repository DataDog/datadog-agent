// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package storeimpl

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	storedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/version"
)

type remoteTestHostname struct{ name string }

func (h *remoteTestHostname) Get(_ context.Context) (string, error) { return h.name, nil }
func (h *remoteTestHostname) GetSafe(_ context.Context) string      { return h.name }
func (h *remoteTestHostname) GetWithProvider(_ context.Context) (hostnameinterface.Data, error) {
	return hostnameinterface.Data{Hostname: h.name, Provider: "test"}, nil
}

func newTestRemotePersistence(t *testing.T, hostname string, handler http.HandlerFunc) *remotePersistence {
	t.Helper()

	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"api_key": " api-key\n",
		"app_key": " app-key ",
	})
	persistence := newRemotePersistence(cfg, &remoteTestHostname{name: hostname})
	persistence.baseURL = server.URL
	persistence.httpClient.Transport = server.Client().Transport
	return persistence
}

func TestNewRemotePersistenceUsesAPISite(t *testing.T) {
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"site":   "datadoghq.eu",
		"dd_url": "https://metrics.example.com",
	})

	persistence := newRemotePersistence(cfg, &remoteTestHostname{name: "node-one"})
	assert.Equal(t, "https://api.datadoghq.eu.", persistence.baseURL)
}

func TestRemotePersistenceLoad(t *testing.T) {
	type capturedRequest struct {
		method     string
		requestURI string
		headers    http.Header
	}
	requestCh := make(chan capturedRequest, 1)
	persistence := newTestRemotePersistence(t, "node/one with space", func(w http.ResponseWriter, r *http.Request) {
		requestCh <- capturedRequest{
			method:     r.Method,
			requestURI: r.RequestURI,
			headers:    r.Header.Clone(),
		}
		_, _ = w.Write([]byte(`{
			"data": [
				{
					"id": "detected-id",
					"type": "agent_health_issue",
					"attributes": {
						"issue_name": "Detected Issue",
						"detected_at": "2026-08-30T10:00:00Z"
					}
				},
				{
					"id": "fallback-id",
					"type": "agent_health_issue",
					"attributes": {
						"issue_name": "Fallback Issue"
					}
				}
			]
		}`))
	})

	state, err := persistence.load(context.Background())
	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, persistedStateVersion, state.Version)
	assert.Len(t, state.Issues, 2)

	detected := state.Issues["detected-id"]
	require.NotNil(t, detected)
	assert.Equal(t, "detected-id", detected.IssueID)
	assert.Equal(t, "Detected Issue", detected.IssueType)
	assert.Empty(t, detected.ProtoIssueType)
	assert.Equal(t, IssueStateActive, detected.State)
	assert.Equal(t, "2026-08-30T10:00:00Z", detected.FirstSeen)
	assert.Equal(t, "2026-08-30T10:00:00Z", detected.LastSeen)

	fallback := state.Issues["fallback-id"]
	require.NotNil(t, fallback)
	assert.Equal(t, "Fallback Issue", fallback.IssueType)
	assert.Empty(t, fallback.ProtoIssueType)
	assert.Equal(t, state.UpdatedAt, fallback.FirstSeen)
	assert.Equal(t, state.UpdatedAt, fallback.LastSeen)

	request := <-requestCh
	assert.Equal(t, http.MethodGet, request.method)
	assert.Equal(t, "/api/v2/agenthealth/hosts/node%2Fone%20with%20space/issues", request.requestURI)
	assert.Equal(t, jsonAPIContentType, request.headers.Get("Accept"))
	assert.Equal(t, "api-key", request.headers.Get("DD-API-KEY"))
	assert.Equal(t, "app-key", request.headers.Get("DD-APPLICATION-KEY"))
	assert.Equal(t, version.AgentVersion, request.headers.Get("DD-Agent-Version"))
	assert.Equal(t, "datadog-agent/"+version.AgentVersion, request.headers.Get("User-Agent"))
}

func TestRemotePersistenceLoadEmpty(t *testing.T) {
	persistence := newTestRemotePersistence(t, "node-one", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	})

	state, err := persistence.load(context.Background())
	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, persistedStateVersion, state.Version)
	assert.Empty(t, state.Issues)
}

func TestRemotePersistenceLoadRequiresCredentials(t *testing.T) {
	tests := []struct {
		name   string
		apiKey string
		appKey string
	}{
		{name: "missing API key", appKey: "app-key"},
		{name: "missing application key", apiKey: "api-key"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			persistence := &remotePersistence{
				config: config.NewMockWithOverrides(t, map[string]interface{}{
					"api_key": test.apiKey,
					"app_key": test.appKey,
				}),
				hostname: &remoteTestHostname{name: "node-one"},
			}

			state, err := persistence.load(context.Background())
			assert.Nil(t, state)
			assert.ErrorContains(t, err, "API key and application key are required")
		})
	}
}

func TestRemotePersistenceLoadRequiresHTTPS(t *testing.T) {
	persistence := newTestRemotePersistence(t, "node-one", func(http.ResponseWriter, *http.Request) {
		t.Fatal("request should not be sent")
	})
	persistence.baseURL = "http://api.example.com"

	state, err := persistence.load(context.Background())
	assert.Nil(t, state)
	assert.ErrorContains(t, err, "must use HTTPS")
}

func TestRemotePersistenceLoadResponseErrors(t *testing.T) {
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
			name:       "wrong resource type",
			statusCode: http.StatusOK,
			body:       `{"data":[{"id":"issue-id","type":"other","attributes":{"issue_name":"Issue"}}]}`,
			wantError:  "unexpected type",
		},
		{
			name:       "missing resource ID",
			statusCode: http.StatusOK,
			body:       `{"data":[{"id":"","type":"agent_health_issue","attributes":{"issue_name":"Issue"}}]}`,
			wantError:  "has no ID",
		},
		{
			name:       "missing issue name",
			statusCode: http.StatusOK,
			body:       `{"data":[{"id":"issue-id","type":"agent_health_issue","attributes":{}}]}`,
			wantError:  "has no issue name",
		},
		{
			name:       "null collection",
			statusCode: http.StatusOK,
			body:       `{"data":null}`,
			wantError:  "must contain a data array",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			persistence := newTestRemotePersistence(t, "node-one", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.statusCode)
				_, _ = w.Write([]byte(test.body))
			})

			state, err := persistence.load(context.Background())
			assert.Nil(t, state)
			assert.ErrorContains(t, err, test.wantError)
		})
	}
}

func TestRemotePersistenceLoadDeduplicatesIssueIDs(t *testing.T) {
	persistence := newTestRemotePersistence(t, "node-one", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[
			{"id":"same-id","type":"agent_health_issue","attributes":{"issue_name":"First"}},
			{"id":"same-id","type":"agent_health_issue","attributes":{"issue_name":"Second"}}
		]}`))
	})

	state, err := persistence.load(context.Background())
	require.NoError(t, err)
	require.Len(t, state.Issues, 1)
	assert.Equal(t, "First", state.Issues["same-id"].IssueType)
}

func TestRemotePersistenceLoadCanceledContext(t *testing.T) {
	persistence := newTestRemotePersistence(t, "node-one", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	state, err := persistence.load(ctx)
	assert.Nil(t, state)
	require.ErrorIs(t, err, context.Canceled)
}

func TestRemotePersistenceLoadDoesNotFollowRedirects(t *testing.T) {
	var requestCount atomic.Int32
	persistence := newTestRemotePersistence(t, "node-one", func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		if r.URL.Path == "/redirect-target" {
			_, _ = w.Write([]byte(`{"data":[]}`))
			return
		}
		http.Redirect(w, r, "/redirect-target", http.StatusFound)
	})

	state, err := persistence.load(context.Background())
	assert.Nil(t, state)
	assert.ErrorContains(t, err, "status 302")
	assert.Equal(t, int32(1), requestCount.Load())
}

func TestRemotePersistenceSaveIsNoop(t *testing.T) {
	persistence := &remotePersistence{}
	require.NoError(t, persistence.save(&PersistedState{}))
}

func TestNewIssuesPersistence(t *testing.T) {
	tests := []struct {
		name                string
		isKubernetes        bool
		agentFlavor         string
		persistOnKubernetes bool
		remoteEnabled       bool
		apiKey              string
		appKey              string
		fipsEnabled         bool
		skipSSLValidation   bool
		clcRunner           bool
		want                string
	}{
		{
			name:        "non-Kubernetes uses disk",
			agentFlavor: flavor.DefaultAgent,
			want:        "disk",
		},
		{
			name:                "Kubernetes durable run path uses disk",
			isKubernetes:        true,
			agentFlavor:         flavor.DefaultAgent,
			persistOnKubernetes: true,
			remoteEnabled:       true,
			apiKey:              "api-key",
			appKey:              "app-key",
			want:                "disk",
		},
		{
			name:         "Kubernetes without long-running marker uses noop",
			isKubernetes: true,
			agentFlavor:  flavor.DefaultAgent,
			apiKey:       "api-key",
			appKey:       "app-key",
			want:         "noop",
		},
		{
			name:          "Kubernetes node Agent with credentials uses remote",
			isKubernetes:  true,
			agentFlavor:   flavor.DefaultAgent,
			remoteEnabled: true,
			apiKey:        "api-key",
			appKey:        "app-key",
			want:          "remote",
		},
		{
			name:          "Kubernetes node Agent missing application key uses noop",
			isKubernetes:  true,
			agentFlavor:   flavor.DefaultAgent,
			remoteEnabled: true,
			apiKey:        "api-key",
			want:          "noop",
		},
		{
			name:          "Kubernetes node Agent using the FIPS proxy uses noop",
			isKubernetes:  true,
			agentFlavor:   flavor.DefaultAgent,
			remoteEnabled: true,
			apiKey:        "api-key",
			appKey:        "app-key",
			fipsEnabled:   true,
			want:          "noop",
		},
		{
			name:              "Kubernetes node Agent without TLS verification uses noop",
			isKubernetes:      true,
			agentFlavor:       flavor.DefaultAgent,
			remoteEnabled:     true,
			apiKey:            "api-key",
			appKey:            "app-key",
			skipSSLValidation: true,
			want:              "noop",
		},
		{
			name:          "Kubernetes Cluster Agent uses noop",
			isKubernetes:  true,
			agentFlavor:   flavor.ClusterAgent,
			remoteEnabled: true,
			apiKey:        "api-key",
			appKey:        "app-key",
			want:          "noop",
		},
		{
			name:          "Kubernetes Cluster Check Runner uses noop",
			isKubernetes:  true,
			agentFlavor:   flavor.DefaultAgent,
			remoteEnabled: true,
			apiKey:        "api-key",
			appKey:        "app-key",
			clcRunner:     true,
			want:          "noop",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runPath := t.TempDir()
			overrides := map[string]interface{}{
				"api_key":                               test.apiKey,
				"app_key":                               test.appKey,
				"fips.enabled":                          test.fipsEnabled,
				"skip_ssl_validation":                   test.skipSSLValidation,
				"run_path":                              runPath,
				"health_platform.persist_on_kubernetes": test.persistOnKubernetes,
			}
			if test.clcRunner {
				overrides["clc_runner_enabled"] = true
				overrides["config_providers"] = []map[string]interface{}{{"name": "clusterchecks"}}
			}
			cfg := config.NewMockWithOverrides(t, overrides)
			reqs := Requires{
				Config:   cfg,
				Log:      logmock.New(t),
				Hostname: &remoteTestHostname{name: "node-one"},
			}
			if test.remoteEnabled {
				reqs.RemotePersistence = &storedef.RemotePersistenceParams{Enabled: true}
			}

			persistence := newIssuesPersistence(reqs, test.agentFlavor, test.isKubernetes)
			switch test.want {
			case "disk":
				disk, ok := persistence.(*diskPersistence)
				require.True(t, ok)
				assert.Equal(t, filepath.Join(runPath, "health-platform", "issues.json"), disk.path)
			case "remote":
				assert.IsType(t, &remotePersistence{}, persistence)
			case "noop":
				assert.IsType(t, &noopPersistence{}, persistence)
			}
		})
	}
}
