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
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	flarebuilder "github.com/DataDog/datadog-agent/comp/core/flare/builder"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
)

// memPersistence stores state in memory, replacing disk I/O in unit tests.
type memPersistence struct {
	mu    sync.Mutex
	state *PersistedState
}

func (m *memPersistence) load() (*PersistedState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state, nil
}

func (m *memPersistence) save(state *PersistedState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state = state
	return nil
}

// mockHostname implements hostnameinterface.Component in tests.
type mockHostname struct{ name string }

func (m *mockHostname) Get(_ context.Context) (string, error) { return m.name, nil }
func (m *mockHostname) GetSafe(_ context.Context) string      { return m.name }
func (m *mockHostname) GetWithProvider(_ context.Context) (hostnameinterface.Data, error) {
	return hostnameinterface.Data{Hostname: m.name, Provider: "mock"}, nil
}

var _ hostnameinterface.Component = (*mockHostname)(nil)

// mockFlareBuilder captures AddFile calls; stubs the rest of the interface.
type mockFlareBuilder struct {
	mu    sync.Mutex
	files map[string][]byte
}

func newMockFlareBuilder() *mockFlareBuilder {
	return &mockFlareBuilder{files: make(map[string][]byte)}
}
func (f *mockFlareBuilder) get(name string) ([]byte, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, ok := f.files[name]
	return d, ok
}
func (f *mockFlareBuilder) AddFile(name string, content []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files[name] = content
	return nil
}
func (f *mockFlareBuilder) AddFileWithoutScrubbing(name string, content []byte) error {
	return f.AddFile(name, content)
}
func (f *mockFlareBuilder) AddFileFromFunc(name string, cb func() ([]byte, error)) error {
	b, err := cb()
	if err != nil {
		return err
	}
	return f.AddFile(name, b)
}
func (f *mockFlareBuilder) IsLocal() bool                                    { return false }
func (f *mockFlareBuilder) Logf(_ string, _ ...interface{}) error            { return nil }
func (f *mockFlareBuilder) CopyFile(_ string) error                          { return nil }
func (f *mockFlareBuilder) CopyFileTo(_, _ string) error                     { return nil }
func (f *mockFlareBuilder) CopyDirTo(_, _ string, _ func(string) bool) error { return nil }
func (f *mockFlareBuilder) CopyDirToWithoutScrubbing(_, _ string, _ func(string) bool) error {
	return nil
}
func (f *mockFlareBuilder) PrepareFilePath(_ string) (string, error) { return "", nil }
func (f *mockFlareBuilder) RegisterFilePerm(_ string)                {}
func (f *mockFlareBuilder) RegisterDirPerm(_ string)                 {}
func (f *mockFlareBuilder) GetFlareArgs() flarebuilder.FlareArgs     { return flarebuilder.FlareArgs{} }
func (f *mockFlareBuilder) Save() (string, error)                    { return "", nil }

var _ flarebuilder.FlareBuilder = (*mockFlareBuilder)(nil)

func newTestStore(t *testing.T) *healthPlatformImpl {
	t.Helper()
	tel := telemetrymock.New(t)
	return &healthPlatformImpl{
		log:              logmock.New(t),
		telemetry:        tel,
		hostnameProvider: &mockHostname{name: "test-host"},
		agentFlavor:      "agent",
		issues:           make(map[string]*healthplatformpayload.Issue),
		issuesByType:     make(map[string]map[string]struct{}),
		persistedIssues:  make(map[string]*PersistedIssue),
		persistence:      &memPersistence{},
		metrics: telemetryMetrics{
			issuesCounter: tel.NewCounter(
				"health_platform", "issues_detected", []string{"issue_type"}, ""),
		},
	}
}

func TestReportIssueNil(t *testing.T) {
	h := newTestStore(t)
	err := h.ReportIssue(nil)
	assert.ErrorContains(t, err, "nil")
}

func TestReportIssueEmptyID(t *testing.T) {
	h := newTestStore(t)
	err := h.ReportIssue(&healthplatformpayload.Issue{Id: "", IssueName: "some-type"})
	assert.ErrorContains(t, err, "issue id")
}

func TestReportIssueEmptyName(t *testing.T) {
	h := newTestStore(t)
	err := h.ReportIssue(&healthplatformpayload.Issue{Id: "id-1", IssueName: ""})
	assert.ErrorContains(t, err, "issue name")
}

func TestReportIssueStoresProto(t *testing.T) {
	h := newTestStore(t)

	err := h.ReportIssue(&healthplatformpayload.Issue{
		Id:        "check-failure:mysql:abc",
		IssueName: "check-failure",
		Title:     "Check 'mysql' Failed",
		Source:    "mysql",
		Severity:  "medium",
		Tags:      []string{"env:prod"},
	})
	require.NoError(t, err)

	issue := h.GetIssue("check-failure:mysql:abc")
	require.NotNil(t, issue)
	assert.Equal(t, "check-failure:mysql:abc", issue.Id)
	assert.Equal(t, "mysql", issue.Source)
	assert.Equal(t, "medium", issue.Severity)
	assert.Contains(t, issue.Tags, "env:prod")
	assert.NotEmpty(t, issue.DetectedAt)
	assert.NotNil(t, issue.PersistedIssue)
}

func TestReportIssueMinimalProto(t *testing.T) {
	h := newTestStore(t)

	err := h.ReportIssue(&healthplatformpayload.Issue{
		Id:        "custom:id-1",
		IssueName: "custom-type",
		Source:    "my-component",
	})
	require.NoError(t, err)

	issue := h.GetIssue("custom:id-1")
	require.NotNil(t, issue)
	assert.Equal(t, "custom:id-1", issue.Id)
	assert.Equal(t, "my-component", issue.Source)
	assert.Equal(t, "custom-type", issue.IssueName)
}

func TestReportIssueStateTransition(t *testing.T) {
	h := newTestStore(t)
	issue := &healthplatformpayload.Issue{Id: "t:id", IssueName: "t"}

	require.NoError(t, h.ReportIssue(issue))
	persisted := h.persistedIssues["t:id"]
	require.NotNil(t, persisted)
	assert.Equal(t, IssueStateNew, persisted.State)
	firstSeen := persisted.FirstSeen

	require.NoError(t, h.ReportIssue(issue))
	persisted = h.persistedIssues["t:id"]
	assert.Equal(t, IssueStateOngoing, persisted.State)
	assert.Equal(t, firstSeen, persisted.FirstSeen, "FirstSeen must not change on re-report")
	assert.GreaterOrEqual(t, persisted.LastSeen, firstSeen)
}

func TestResolveIssueRemovesFromActive(t *testing.T) {
	h := newTestStore(t)
	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "t:id", IssueName: "t"}))

	h.ResolveIssue("t:id")

	assert.Nil(t, h.GetIssue("t:id"))
	require.NotNil(t, h.persistedIssues["t:id"])
	assert.Equal(t, IssueStateResolved, h.persistedIssues["t:id"].State)
	assert.NotEmpty(t, h.persistedIssues["t:id"].ResolvedAt)
}

func TestResolveIssueUnknownIDIsNoop(t *testing.T) {
	h := newTestStore(t)
	h.ResolveIssue("nonexistent") // must not panic or error
}

func TestResolveAllIssues(t *testing.T) {
	h := newTestStore(t)
	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "t:1", IssueName: "t"}))
	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "t:2", IssueName: "t"}))

	h.ResolveAllIssues()

	count, issues := h.GetAllIssues()
	assert.Equal(t, 0, count)
	assert.Empty(t, issues)
	for _, p := range h.persistedIssues {
		assert.Equal(t, IssueStateResolved, p.State)
	}
}

func TestGetIssueNilForUnknown(t *testing.T) {
	h := newTestStore(t)
	assert.Nil(t, h.GetIssue("does-not-exist"))
}

func TestGetAllIssuesDeepCopy(t *testing.T) {
	h := newTestStore(t)
	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "t:id", IssueName: "t", Source: "orig"}))

	_, issues := h.GetAllIssues()
	got := issues["t:id"]
	require.NotNil(t, got)

	// Mutating the returned value must not affect the in-store issue.
	originalSource := h.issues["t:id"].Source
	got.Source = "hacked"
	assert.Equal(t, originalSource, h.issues["t:id"].Source)
}

func TestMultiInstanceSameType(t *testing.T) {
	h := newTestStore(t)

	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "db-error:prod-1", IssueName: "db-error"}))
	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "db-error:prod-2", IssueName: "db-error"}))

	count, issues := h.GetAllIssues()
	assert.Equal(t, 2, count)
	assert.Contains(t, issues, "db-error:prod-1")
	assert.Contains(t, issues, "db-error:prod-2")
}

func TestMultiTypeSameSource(t *testing.T) {
	h := newTestStore(t)

	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "a:id", IssueName: "type-a", Source: "mysrc"}))
	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "b:id", IssueName: "type-b", Source: "mysrc"}))

	count, _ := h.GetAllIssues()
	assert.Equal(t, 2, count)
}

func TestPersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "issues.json")
	logger := logmock.New(t)

	h1 := newTestStore(t)
	h1.persistence = newDiskPersistence(path, logger)
	require.NoError(t, h1.ReportIssue(&healthplatformpayload.Issue{
		Id: "t:id", IssueName: "t", Title: "Test Issue", Source: "test-src",
	}))

	h2 := newTestStore(t)
	h2.persistence = newDiskPersistence(path, logger)
	require.NoError(t, h2.loadFromDisk())

	issue := h2.GetIssue("t:id")
	require.NotNil(t, issue, "issue must survive persistence round-trip")
	assert.Equal(t, "t:id", issue.Id)
	assert.Equal(t, "Test Issue", issue.Title)
}

func TestPersistenceVersionMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "issues.json")

	stale := map[string]interface{}{
		"version":    1,
		"updated_at": time.Now().Format(time.RFC3339),
		"issues": map[string]interface{}{
			"t:id": map[string]interface{}{
				"issue_type": "t",
				"state":      "new",
				"first_seen": time.Now().Format(time.RFC3339),
				"last_seen":  time.Now().Format(time.RFC3339),
			},
		},
	}
	data, err := json.Marshal(stale)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0644))

	h := newTestStore(t)
	h.persistence = newDiskPersistence(path, logmock.New(t))
	require.NoError(t, h.loadFromDisk())

	// Stale version: store must start fresh.
	assert.Nil(t, h.GetIssue("t:id"))
	count, _ := h.GetAllIssues()
	assert.Equal(t, 0, count)
}

func TestResolvedTTLPruning(t *testing.T) {
	h := newTestStore(t)
	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "t:id", IssueName: "t"}))

	// Back-date the resolved-at timestamp so it's older than the TTL.
	h.persistedIssues["t:id"].State = IssueStateResolved
	h.persistedIssues["t:id"].ResolvedAt = time.Now().Add(-25 * time.Hour).Format(time.RFC3339)

	mem := &memPersistence{}
	h.persistence = mem
	require.NoError(t, h.saveToDisk())

	require.NotNil(t, mem.state)
	assert.NotContains(t, mem.state.Issues, "t:id",
		"resolved issue older than TTL must be pruned on save")
}

func TestGetIssuesHTTPEndpoint(t *testing.T) {
	h := newTestStore(t)
	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "t:1", IssueName: "t"}))
	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "t:2", IssueName: "t"}))

	req := httptest.NewRequest(http.MethodGet, "/health-platform/issues", nil)
	rec := httptest.NewRecorder()
	h.getIssuesHandler(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var body struct {
		Count  int                                     `json:"count"`
		Issues map[string]*healthplatformpayload.Issue `json:"issues"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
	assert.Equal(t, 2, body.Count)
	assert.Contains(t, body.Issues, "t:1")
	assert.Contains(t, body.Issues, "t:2")
}

func TestFillFlareSkipsWhenEmpty(t *testing.T) {
	h := newTestStore(t)
	fb := newMockFlareBuilder()
	require.NoError(t, h.fillFlare(context.Background(), fb))
	_, written := fb.get("health-platform-issues.json")
	assert.False(t, written, "fillFlare must not write a file when the store is empty")
}

func TestFillFlareWritesWhenNonEmpty(t *testing.T) {
	h := newTestStore(t)
	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "t:id", IssueName: "t"}))

	fb := newMockFlareBuilder()
	require.NoError(t, h.fillFlare(context.Background(), fb))

	data, written := fb.get("health-platform-issues.json")
	require.True(t, written, "fillFlare must write a file when the store has issues")
	assert.Contains(t, string(data), "t:id")
}

func TestTelemetryCounterIncrements(t *testing.T) {
	tel := telemetrymock.New(t)
	counter := tel.NewCounter("health_platform", "issues_detected", []string{"issue_type"}, "")
	h := &healthPlatformImpl{
		log:              logmock.New(t),
		telemetry:        tel,
		hostnameProvider: &mockHostname{name: "test-host"},
		agentFlavor:      "agent",
		issues:           make(map[string]*healthplatformpayload.Issue),
		issuesByType:     make(map[string]map[string]struct{}),
		persistedIssues:  make(map[string]*PersistedIssue),
		persistence:      &memPersistence{},
		metrics:          telemetryMetrics{issuesCounter: counter},
	}

	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "t:id", IssueName: "t"}))

	assert.Equal(t, 1.0, counter.WithValues("t").Get())
}

func TestGetActiveIssueIDsByIssueType(t *testing.T) {
	h := newTestStore(t)
	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "t:1", IssueName: "t"}))
	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "t:2", IssueName: "t"}))

	ids := h.GetActiveIssueIDsByIssueType("t")
	assert.ElementsMatch(t, []string{"t:1", "t:2"}, ids)

	h.ResolveIssue("t:1")
	ids = h.GetActiveIssueIDsByIssueType("t")
	assert.ElementsMatch(t, []string{"t:2"}, ids)
}
