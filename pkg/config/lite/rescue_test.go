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
// HealthReport. The site arg in Rescue() is irrelevant; we point the prefix
// at the test server directly.
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

	// Route all rescue POSTs to the test server, regardless of the site
	// resolved by Extract.
	originalBuilder := buildIntakeURL
	buildIntakeURL = func(string) string { return srv.URL + rescueIntakePath }
	t.Cleanup(func() { buildIntakeURL = originalBuilder })

	return srv, &captured
}

// ─────────────────────────────────────────────────────────────────────────────
// buildRescueIssue dispatching
// ─────────────────────────────────────────────────────────────────────────────

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
	// schema.ValidateCoreConfig returns an infra error in the test build
	// because the compressed schema isn't embedded — but our handler still
	// dispatches on cfg.ParsedConfig + len(errs)>0. To exercise the medium-
	// severity path here we drive the rescue helper directly.
	errs := []string{
		"/agent_ipc/port: expected integer, got string",
		"/tags: expected array, got string",
	}
	issue := rescueSchemaIssue("/etc/datadog-agent/datadog.yaml", errs)
	assert.Equal(t, "medium", issue.Severity)
	assert.Equal(t, IssueID, issue.Id)
	assert.Contains(t, issue.Title, "2 schema violation(s)")

	count := int(issue.Extra.Fields["error_count"].GetNumberValue())
	assert.Equal(t, 2, count)
	errorsJoined := issue.Extra.Fields["errors"].GetStringValue()
	assert.Contains(t, errorsJoined, "agent_ipc/port")
	assert.Contains(t, errorsJoined, "tags")
}

func TestBuildRescue_SchemaTruncation(t *testing.T) {
	// 30 made-up errors → only top-20 in payload, error_count = 30,
	// truncated = true.
	errs := make([]string, 30)
	for i := range errs {
		errs[i] = "violation"
	}
	issue := rescueSchemaIssue("", errs)
	count := int(issue.Extra.Fields["error_count"].GetNumberValue())
	assert.Equal(t, 30, count)
	assert.True(t, issue.Extra.Fields["truncated"].GetBoolValue())
	visible := issue.Extra.Fields["errors"].GetStringValue()
	assert.Equal(t, MaxSchemaErrorsInPayload, strings.Count(visible, "violation"))
}

func TestBuildRescue_StartupFailureWhenConfigIsClean(t *testing.T) {
	cfg := LiteConfig{
		APIKey:       ConfigField{Value: "k", Source: SourceFileYAMLFull},
		Site:         ConfigField{Value: "datadoghq.com", Source: SourceFileYAMLFull},
		ParsedConfig: map[string]any{},
	}
	// ParsedConfig is empty so schema.ValidateCoreConfig returns no errors
	// (or infra error, which we treat as no errors). startupErr is what
	// should win in this case.
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

// ─────────────────────────────────────────────────────────────────────────────
// End-to-end via httptest.Server
// ─────────────────────────────────────────────────────────────────────────────

func TestRescue_PostsYAMLParseIssue(t *testing.T) {
	_, captured := stubRescueIntake(t)

	dir := withYAML(t, "{ this is not yaml\napi_key: rescued\nsite: dd.eu\n")

	err := Rescue(context.Background(), "", dir, nil)
	require.NoError(t, err)
	require.Len(t, *captured, 1, "Rescue must POST exactly one HealthReport")

	report := (*captured)[0]
	assert.Equal(t, rescueEventType, report.GetEventType())
	assert.Equal(t, "agent", report.GetService())
	require.Len(t, report.GetIssues(), 1)
	issue := report.GetIssues()["datadog.yaml"]
	require.NotNil(t, issue)
	assert.Equal(t, "high", issue.Severity)
	assert.Equal(t, IssueID, issue.Id)
}

func TestRescue_NoAPIKeyReturnsError(t *testing.T) {
	_, captured := stubRescueIntake(t)

	dir := withYAML(t, "{ broken\nsite: dd.eu\n")
	// No api_key anywhere → resolver leaves APIKey=SourceNone → Rescue errors.

	err := Rescue(context.Background(), "", dir, nil)
	require.Error(t, err)
	assert.Empty(t, *captured, "no POST should have been attempted without api_key")
}

func TestRescue_RespectsTimeout(t *testing.T) {
	// Server replies eventually, but slowly enough that Rescue's 3-second
	// budget expires first. We sleep with a short cancel-aware wait so the
	// handler doesn't keep running after the test ends.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(10 * time.Second):
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)

	originalBuilder := buildIntakeURL
	buildIntakeURL = func(string) string { return srv.URL + rescueIntakePath }
	t.Cleanup(func() { buildIntakeURL = originalBuilder })

	dir := withYAML(t, "{ broken\napi_key: rescued\nsite: dd.eu\n")

	start := time.Now()
	err := Rescue(context.Background(), "", dir, nil)
	elapsed := time.Since(start)

	require.Error(t, err, "slow server must trip the 3s timeout and surface as error")
	require.Less(t, elapsed, 8*time.Second, "Rescue must not block past its own timeout")
}

// ─────────────────────────────────────────────────────────────────────────────
// DefaultConfigPath
// ─────────────────────────────────────────────────────────────────────────────

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
