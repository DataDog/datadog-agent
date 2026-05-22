// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package lite

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubRescueIntake registers a fake intake server that captures the POSTed
// HealthReport. Returns (server, &captured). Tests pass srv.URL+rescueIntakePath
// to rescueWithURL to route POSTs at the test server.
func stubRescueIntake(t *testing.T) (*httptest.Server, *[]*healthplatform.HealthReport) {
	t.Helper()

	mu := sync.Mutex{}
	var captured []*healthplatform.HealthReport

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/v2/agenthealth", r.URL.Path)
		assert.NotEmpty(t, r.Header.Get("DD-API-KEY"))

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		report := &healthplatform.HealthReport{}
		require.NoError(t, json.Unmarshal(body, report))

		mu.Lock()
		captured = append(captured, report)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv, &captured
}

// schemaIssue is a test helper that builds a schema-validation issue the
// same way rescueWithURL does.
func schemaIssue(t *testing.T, path string, errs []string) *healthplatform.Issue {
	t.Helper()
	return BuildInvalidConfigIssue(IssueInfo{
		Kind:       ErrorKindSchemaValidation,
		ConfigPath: path,
		Errors:     strings.Join(errs, "\n"),
		ErrorCount: len(errs),
	})
}

func TestBuildRescue_YAMLParseHasHighSeverity(t *testing.T) {
	cfg := LiteConfig{
		APIKey:       ConfigField{Value: "k", Source: SourceFileRegex},
		Site:         ConfigField{Value: "datadoghq.com", Source: SourceDefault},
		YAMLParseErr: errors.New("yaml: line 12: did not find expected ',' or ']'"),
	}
	issue := buildRescueIssue(cfg, nil)
	require.NotNil(t, issue)
	assert.Equal(t, "high", issue.Severity)
	assert.Equal(t, IssueID, issue.Id)
	assert.Contains(t, issue.Title, "not valid YAML")
	kind, ok := issue.Extra.Fields["error_kind"]
	require.True(t, ok)
	assert.Equal(t, string(ErrorKindYAMLParse), kind.GetStringValue())
}

func TestBuildRescue_SchemaErrorsHaveMediumSeverity(t *testing.T) {
	// schema.ValidateCoreConfig returns an infra error in the test build (the
	// compressed schema isn't embedded), so we drive the helper directly.
	errs := []string{
		"/agent_ipc/port: expected integer, got string",
		"/tags: expected array, got string",
	}
	issue := schemaIssue(t, "/etc/datadog-agent/datadog.yaml", errs)
	assert.Equal(t, "medium", issue.Severity)
	assert.Equal(t, IssueID, issue.Id)
	assert.Contains(t, issue.Title, "2 schema violations")

	count := int(issue.Extra.Fields["error_count"].GetNumberValue())
	assert.Equal(t, 2, count)
	errorsJoined := issue.Extra.Fields["errors"].GetStringValue()
	assert.Contains(t, errorsJoined, "agent_ipc/port")
	assert.Contains(t, errorsJoined, "tags")
}

func TestBuildRescue_SchemaAllErrorsEmitted(t *testing.T) {
	// Every schema error is embedded; no truncation cap.
	errs := make([]string, 30)
	for i := range errs {
		errs[i] = "violation"
	}
	issue := schemaIssue(t, "", errs)
	count := int(issue.Extra.Fields["error_count"].GetNumberValue())
	assert.Equal(t, 30, count)
	visible := issue.Extra.Fields["errors"].GetStringValue()
	assert.Equal(t, 30, strings.Count(visible, "violation"))
}

func TestBuildRescue_StartupFailureWhenLiteConfigIsClean(t *testing.T) {
	cfg := LiteConfig{
		APIKey:       ConfigField{Value: "k", Source: SourceFileYAMLFull},
		Site:         ConfigField{Value: "datadoghq.com", Source: SourceFileYAMLFull},
		ParsedConfig: map[string]any{},
	}
	issue := buildRescueIssue(cfg, errors.New("port :8126 already in use"))
	require.NotNil(t, issue)
	assert.Equal(t, "high", issue.Severity)
	assert.Equal(t, "Datadog Agent failed to start", issue.Title)
	assert.Equal(t, string(ErrorKindStartupFailure),
		issue.Extra.Fields["error_kind"].GetStringValue())
	assert.Contains(t, issue.Extra.Fields["error_message"].GetStringValue(), "port :8126")
}

func TestBuildRescue_NothingToReportReturnsNil(t *testing.T) {
	cfg := LiteConfig{
		APIKey:       ConfigField{Value: "k", Source: SourceFileYAMLFull},
		Site:         ConfigField{Value: "datadoghq.com", Source: SourceFileYAMLFull},
		ParsedConfig: map[string]any{},
	}
	assert.Nil(t, buildRescueIssue(cfg, nil),
		"clean config + no startup error must produce nil — Rescue MUST NOT post a useless issue")
}

func TestRescue_PostsYAMLParseIssue(t *testing.T) {
	srv, captured := stubRescueIntake(t)

	dir := withYAML(t, "{ this is not yaml\napi_key: rescued\nsite: dd.eu\n")
	cfg := Extract(context.Background(), "", dir)

	err := rescueWithURL(context.Background(), cfg, srv.URL+rescueIntakePath, nil)
	require.NoError(t, err)
	require.Len(t, *captured, 1, "rescueWithURL must POST exactly one HealthReport")

	report := (*captured)[0]
	assert.Equal(t, rescueEventType, report.GetEventType())
	assert.Equal(t, "agent", report.GetService())
	require.Len(t, report.GetIssues(), 1)
	issue := report.GetIssues()[IssueID]
	require.NotNil(t, issue)
	assert.Equal(t, "high", issue.Severity)
	assert.Equal(t, IssueID, issue.Id)
}

func TestRescue_NoAPIKeyReturnsError(t *testing.T) {
	srv, captured := stubRescueIntake(t)

	dir := withYAML(t, "{ broken\nsite: dd.eu\n")
	cfg := Extract(context.Background(), "", dir)

	err := rescueWithURL(context.Background(), cfg, srv.URL+rescueIntakePath, nil)
	require.Error(t, err)
	assert.Empty(t, *captured, "no POST should have been attempted without api_key")
}

// TestRescue_RetriesCandidatesUntilSuccess covers the fuzzy-collision case:
// when both `app_key` and a typo'd `api_kye` are distance 1 from "api_key",
// the rescue path POSTs with the primary first; if it 401s it should retry
// with the next candidate until one succeeds.
func TestRescue_RetriesCandidatesUntilSuccess(t *testing.T) {
	const goodKey = "real_api_key_value"

	// Server: 401 unless the request bears goodKey.
	captured := []string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("DD-API-KEY")
		captured = append(captured, got)
		if got == goodKey {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	cfg := LiteConfig{
		APIKey: ConfigField{Value: "wrong_app_key", Source: SourceFileFuzzy, MatchedKey: "app_key"},
		APIKeyCandidates: []ConfigField{
			{Value: goodKey, Source: SourceFileFuzzy, MatchedKey: "api_kye"},
		},
		Site:         ConfigField{Value: "dd.eu", Source: SourceFileYAMLFull},
		YAMLParseErr: errors.New("forced parse failure"),
	}

	err := rescueWithURL(context.Background(), cfg, srv.URL+rescueIntakePath, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"wrong_app_key", goodKey}, captured,
		"rescue must try the primary then walk candidates until one returns 2xx")
}

func TestRescue_RespectsTimeout(t *testing.T) {
	// Server replies eventually, but slowly enough that the rescueHTTPTimeout
	// budget expires first. r.Context().Done() lets the handler stop when the
	// test ends.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(10 * time.Second):
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)

	dir := withYAML(t, "{ broken\napi_key: rescued\nsite: dd.eu\n")
	cfg := Extract(context.Background(), "", dir)

	ctx, cancel := context.WithTimeout(context.Background(), rescueHTTPTimeout)
	defer cancel()

	start := time.Now()
	err := rescueWithURL(ctx, cfg, srv.URL+rescueIntakePath, nil)
	elapsed := time.Since(start)

	require.Error(t, err, "slow server must trip the 3s timeout and surface as error")
	require.Less(t, elapsed, rescueHTTPTimeout+2*time.Second, "rescueWithURL must not block past its own timeout")
}

func TestDefaultConfigPath_EnvOverride(t *testing.T) {
	t.Setenv("DD_CONFIG", "/custom/path")
	assert.Equal(t, "/custom/path", DefaultConfigPath())
}

func TestDefaultConfigPath_PerPlatform(t *testing.T) {
	assert.Equal(t, "/etc/datadog-agent", defaultConfigPathForGOOS("linux"))
	assert.Equal(t, "/opt/datadog-agent/etc", defaultConfigPathForGOOS("darwin"))
	assert.Equal(t, `C:\ProgramData\Datadog`, defaultConfigPathForGOOS("windows"))
	assert.Equal(t, "/etc/datadog-agent", defaultConfigPathForGOOS("freebsd"),
		"unknown platforms fall back to the linux default")
}

// TestIntakeURL locks in the production URL format so the Rescue() wrapper,
// which composes intakeURL + rescueWithURL with no other logic, doesn't need
// its own end-to-end test.
func TestIntakeURL(t *testing.T) {
	cases := []struct {
		name, site, want string
	}{
		{"empty defaults to datadoghq.com", "", "https://agenthealth-intake.datadoghq.com/api/v2/agenthealth"},
		{"eu site", "datadoghq.eu", "https://agenthealth-intake.datadoghq.eu/api/v2/agenthealth"},
		{"trailing slash trimmed", "datadoghq.com/", "https://agenthealth-intake.datadoghq.com/api/v2/agenthealth"},
		{"attacker host rejected", "attacker.com/evil", "https://agenthealth-intake.datadoghq.com/api/v2/agenthealth"},
		{"embedded scheme rejected", "evil@host.com", "https://agenthealth-intake.datadoghq.com/api/v2/agenthealth"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, IntakeURL(c.site))
		})
	}
}
