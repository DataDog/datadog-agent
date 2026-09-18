// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package lite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/require"
)

func TestRescueReport(t *testing.T) {
	cleanEnv(t)
	type received struct {
		body      []byte
		path, key string
	}
	reports := make(chan received, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		reports <- received{body, r.URL.Path, r.Header.Get("DD-API-KEY")}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	p := Params{ConfigPath: configFile(t, fmt.Sprintf("api_key: secret-sentinel\ndd_url: %s\nhostname: rescue-host\nlogs_config: [broken\n", server.URL))}
	var previousID string
	for range 2 {
		require.NoError(t, Rescue(context.Background(), p, errors.New("listen: address already in use; api_key: secret-sentinel")))
		select {
		case got := <-reports:
			require.Equal(t, "/api/v2/agenthealth", got.path)
			require.Equal(t, "secret-sentinel", got.key)
			require.NotContains(t, string(got.body), "secret-sentinel")
			require.NotContains(t, string(got.body), "logs_config")
			var report healthplatform.HealthReport
			require.NoError(t, json.Unmarshal(got.body, &report))
			require.Equal(t, "agent-health-issues", report.EventType)
			require.Equal(t, "agent", report.Service)
			require.Equal(t, "rescue-host", report.Host.Hostname)
			require.Len(t, report.Issues, 1)
			for id, issue := range report.Issues {
				require.Equal(t, id, issue.Id)
				if previousID != "" {
					require.Equal(t, previousID, id)
				}
				previousID = id
				require.Equal(t, "Agent Startup Failure", issue.IssueName)
				require.Equal(t, "agent_startup_failure", issue.IssueType)
				require.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH, issue.Severity)
				require.Equal(t, "agent", issue.Source)
				require.Equal(t, "agent", issue.Location)
				require.NotEmpty(t, issue.Category)
				require.Contains(t, issue.Description, "address already in use")
				require.NotEmpty(t, issue.Remediation.Summary)
				require.NotEmpty(t, issue.Remediation.Steps)
				require.NotEmpty(t, issue.Tags)
				require.NotNil(t, issue.Extra)
				require.NotEmpty(t, issue.DetectedAt)
			}
		default:
			t.Fatal("startup failure was not delivered")
		}
	}
}

func TestRescueDoesNotSend(t *testing.T) {
	for _, tc := range []struct {
		name, raw, env string
		startupErr     error
		canceled       bool
	}{
		{"nil error", "api_key: dummy\n", "", nil, false},
		{"disabled in YAML", "api_key: dummy\nhealth_platform:\n  enabled: false\n", "", errors.New("failed"), false},
		{"disabled in env", "api_key: dummy\n", "false", errors.New("failed"), false},
		{"missing key", "api_kye: candidate\n", "", errors.New("failed"), false},
		{"encrypted key without backend", "api_key: ENC[key]\n", "", errors.New("failed"), false},
		{"invalid TLS setting", "api_key: dummy\nmin_tls_version: nonsense\n", "", errors.New("failed"), false},
		{"invalid bool", "api_key: dummy\nhealth_platform:\n  enabled: nonsense\n", "", errors.New("failed"), false},
		{"invalid proxy", "api_key: dummy\nproxy:\n  http: [broken]\n", "", errors.New("failed"), false},
		{"invalid duration", "api_key: dummy\ntls_handshake_timeout: [broken]\n", "", errors.New("failed"), false},
		{"invalid proxy bypass list", "api_key: dummy\nproxy:\n  no_proxy: {bad: value}\n", "", errors.New("failed"), false},
		{"invalid backend arguments", "api_key: dummy\nsecret_backend_arguments: {bad: value}\n", "", errors.New("failed"), false},
		{"invalid backend map", "api_key: dummy\nsecret_backend_config: [bad]\n", "", errors.New("failed"), false},
		{"invalid multi backend map", "api_key: dummy\nmulti_secret_backends: [bad]\n", "", errors.New("failed"), false},
		{"invalid backend timeout", "api_key: dummy\nsecret_backend_timeout: bad\n", "", errors.New("failed"), false},
		{"null API key", "api_key: null\n", "", errors.New("failed"), false},
		{"numeric API key", "api_key: 123\n", "", errors.New("failed"), false},
		{"canceled", "api_key: dummy\n", "", errors.New("failed"), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cleanEnv(t)
			t.Setenv("DD_HEALTH_PLATFORM_ENABLED", tc.env)
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
			defer server.Close()
			p := Params{ConfigPath: configFile(t, tc.raw+"dd_url: "+server.URL+"\n")}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.canceled {
				cancel()
			}
			_ = Rescue(ctx, p, tc.startupErr)
			require.Zero(t, calls.Load())
		})
	}
}

func TestStartupIdentityIsPerInstance(t *testing.T) {
	ids := map[string]bool{}
	for _, instance := range [][2]string{{"host-a", "/etc/datadog.yaml"}, {"host-b", "/etc/datadog.yaml"}, {"host-a", "/other/datadog.yaml"}} {
		for id := range startupReport(instance[0], instance[1], "failure").Issues {
			require.False(t, ids[id], "distinct instances must not collapse")
			ids[id] = true
		}
	}
}

func TestRescueHonorsProxyAndDeadline(t *testing.T) {
	cleanEnv(t)
	received := make(chan string, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		received <- r.URL.Host
		_, _ = io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
	}))
	defer proxy.Close()
	p := Params{ConfigPath: configFile(t, "api_key: dummy\ndd_url: http://intake.example\nproxy:\n  http: "+proxy.URL+"\n")}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := Rescue(ctx, p, errors.New("failed"))
	require.Error(t, err)
	select {
	case host := <-received:
		require.Equal(t, "intake.example", host)
	default:
		t.Fatal("configured proxy not used")
	}
}
