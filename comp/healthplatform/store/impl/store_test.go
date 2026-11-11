// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package storeimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/core/config"
	flarebuilder "github.com/DataDog/datadog-agent/comp/core/flare/builder"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issueregistry/utils/selfident"
	storedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
)

// memPersistence stores state in memory, replacing disk I/O in unit tests.
type memPersistence struct {
	mu        sync.Mutex
	state     *PersistedState
	loadErr   error
	loadCalls int
}

func (m *memPersistence) load(context.Context) (*PersistedState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loadCalls++
	return m.state, m.loadErr
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
	h := newUnstartedTestStore(t)
	startTestStore(t, h)
	return h
}

func startTestStore(t *testing.T, h *healthPlatformImpl) {
	t.Helper()
	require.NoError(t, h.start(context.Background()))
	t.Cleanup(func() { stopTestStore(t, h) })
}

func stopTestStore(t *testing.T, h *healthPlatformImpl) {
	t.Helper()
	select {
	case <-h.doneCh:
	default:
		require.NoError(t, h.stop(context.Background()))
	}
}

func (h *healthPlatformImpl) sync() {
	done := make(chan struct{})
	h.enqueue(ingestCmd{kind: cmdSync, done: done})
	<-done
}

func newUnstartedTestStore(t *testing.T) *healthPlatformImpl {
	t.Helper()
	tel := telemetrymock.New(t)
	return &healthPlatformImpl{
		log:              logmock.New(t),
		telemetry:        tel,
		hostnameProvider: &mockHostname{name: "test-host"},
		agentFlavor:      "agent",
		selfIdent:        selfident.New(nil),
		issues:           make(map[string]*storedIssue),
		issuesByName:     make(map[string][]string),
		persistedIssues:  make(map[string]*PersistedIssue),
		persistence:      &memPersistence{},
		ingestWake:       make(chan struct{}, 1),
		stopCh:           make(chan struct{}),
		doneCh:           make(chan struct{}),
		metrics: telemetryMetrics{
			issuesDropped:  tel.NewCounter("health_platform", "issues_dropped", []string{"operation"}, ""),
			ingestQueueLen: tel.NewGauge("health_platform", "ingest_queue_length", nil, ""),
			issuesCounter: tel.NewCounter(
				"health_platform", "issues_detected", []string{"issue_type", "severity"}, ""),
		},
	}
}

func TestNewComponentPersistence(t *testing.T) {
	tests := []struct {
		name                      string
		kubernetes, local, remote bool
		wantDisk, wantRemote      bool
	}{
		{name: "non-Kubernetes uses disk", wantDisk: true},
		{name: "non-Kubernetes ignores flags", local: true, remote: true, wantDisk: true},
		{name: "Kubernetes persistence disabled", kubernetes: true},
		{name: "Kubernetes local persistence", kubernetes: true, local: true, wantDisk: true},
		{name: "Kubernetes remote persistence", kubernetes: true, remote: true, wantRemote: true},
		{name: "remote takes precedence", kubernetes: true, local: true, remote: true, wantRemote: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("KUBERNETES_SERVICE_PORT", "")
			t.Setenv("KUBERNETES", "")
			if test.kubernetes {
				t.Setenv("KUBERNETES", "true")
			}
			cfg := config.NewMockWithOverrides(t, map[string]interface{}{
				"health_platform.enabled":                        true,
				"health_platform.persist_on_kubernetes":          test.local,
				"health_platform.persist_remotely_on_kubernetes": test.remote,
				"api_key":  "api-key",
				"run_path": t.TempDir(),
			})
			provided, err := NewComponent(Requires{
				Lifecycle:    compdef.NewTestLifecycle(t),
				Config:       cfg,
				Log:          logmock.New(t),
				Telemetry:    telemetrymock.New(t),
				RemoteIssues: &fakeRemoteIssueLoader{},
			})
			require.NoError(t, err)
			h := provided.Comp.(*healthPlatformImpl)
			if test.wantRemote {
				assert.IsType(t, &remotePersistence{}, h.persistence)
			} else if test.wantDisk {
				assert.IsType(t, &diskPersistence{}, h.persistence)
			} else {
				assert.IsType(t, &noopPersistence{}, h.persistence)
			}
		})
	}
}

func TestStartFailsOpenWhenPersistenceLoadFails(t *testing.T) {
	h := newUnstartedTestStore(t)
	persistence := &memPersistence{loadErr: assert.AnError}
	h.persistence = persistence

	startTestStore(t, h)
	assert.Equal(t, 1, persistence.loadCalls)
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
		IssueType: "check_failure",
		Title:     "Check 'mysql' Failed",
		Source:    "mysql",
		Severity:  healthplatformpayload.IssueSeverity_ISSUE_SEVERITY_MEDIUM,
		Tags:      []string{"env:prod"},
	})
	require.NoError(t, err)
	h.sync()

	issue := h.GetIssue("check-failure:mysql:abc")
	require.NotNil(t, issue)
	assert.Equal(t, "check-failure:mysql:abc", issue.Id)
	assert.Equal(t, "mysql", issue.Source)
	assert.Equal(t, healthplatformpayload.IssueSeverity_ISSUE_SEVERITY_MEDIUM, issue.Severity)
	assert.Contains(t, issue.Tags, "env:prod")
	assert.NotEmpty(t, issue.DetectedAt)
	assert.NotNil(t, issue.PersistedIssue)
	assert.Equal(t, "check_failure", issue.IssueType)
}

func TestReportIssueMinimalProto(t *testing.T) {
	h := newTestStore(t)

	err := h.ReportIssue(&healthplatformpayload.Issue{
		Id:        "custom:id-1",
		IssueName: "custom-type",
		Source:    "my-component",
	})
	require.NoError(t, err)
	h.sync()

	issue := h.GetIssue("custom:id-1")
	require.NotNil(t, issue)
	assert.Equal(t, "custom:id-1", issue.Id)
	assert.Equal(t, "my-component", issue.Source)
	assert.Equal(t, "custom-type", issue.IssueName)
}

// TestReportIssuePreservesIssueType guards that the store never overwrites or
// derives IssueType — the caller (module BuildIssue or a direct reporter) owns
// that value entirely, same as every other proto field.
func TestReportIssuePreservesIssueType(t *testing.T) {
	h := newTestStore(t)
	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{
		Id:        "id-1",
		IssueName: "Some Issue Name",
		IssueType: "custom_caller_chosen_type",
	}))
	h.sync()

	issue := h.GetIssue("id-1")
	require.NotNil(t, issue)
	assert.Equal(t, "custom_caller_chosen_type", issue.IssueType)
}

// TestReportIssueAllowsEmptyIssueType guards that an unset IssueType is not
// backfilled by the store — the backend, not the agent, owns any default
// derivation from IssueName.
func TestReportIssueAllowsEmptyIssueType(t *testing.T) {
	h := newTestStore(t)
	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{
		Id:        "id-2",
		IssueName: "Some Issue Name",
	}))
	h.sync()

	issue := h.GetIssue("id-2")
	require.NotNil(t, issue)
	assert.Empty(t, issue.IssueType)
}

func TestReportIssueStateTransition(t *testing.T) {
	h := newTestStore(t)
	issue := &healthplatformpayload.Issue{Id: "t:id", IssueName: "t"}

	require.NoError(t, h.ReportIssue(issue))
	h.sync()
	persisted := h.persistedIssues["t:id"]
	require.NotNil(t, persisted)
	assert.Equal(t, IssueStateActive, persisted.State)
	firstSeen := persisted.FirstSeen

	require.NoError(t, h.ReportIssue(issue))
	h.sync()
	persisted = h.persistedIssues["t:id"]
	assert.Equal(t, IssueStateActive, persisted.State)
	assert.Equal(t, firstSeen, persisted.FirstSeen, "FirstSeen must not change on re-report")
	assert.GreaterOrEqual(t, persisted.LastSeen, firstSeen)
}

func TestResolveIssueRemovesFromActive(t *testing.T) {
	h := newTestStore(t)
	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "t:id", IssueName: "t"}))
	h.sync()

	ch := make(chan *healthplatformpayload.Issue, 1)
	h.RegisterIssuesObserver(storedef.IssuesObserver{ResolvedCh: ch})

	h.ResolveIssue("t:id")
	h.sync()

	// Issue must be removed from the active set; resolved snapshot written to ResolvedCh.
	assert.Nil(t, h.GetIssue("t:id"), "issue must be removed from active set after ResolveIssue")
	require.Len(t, ch, 1, "resolved issue must be written to ResolvedCh")
	got := <-ch
	require.NotNil(t, got.PersistedIssue)
	assert.Equal(t, IssueStateResolved, got.PersistedIssue.State)

	require.NotNil(t, h.persistedIssues["t:id"])
	assert.Equal(t, IssueStateResolved, h.persistedIssues["t:id"].State)
	assert.NotEmpty(t, h.persistedIssues["t:id"].ResolvedAt)
}

func TestResolveIssuePreservesIssueTypeAndCategory(t *testing.T) {
	h := newTestStore(t)
	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{
		Id: "t:id", IssueName: "t", IssueType: "custom_type", Category: "configuration",
	}))
	h.sync()

	ch := make(chan *healthplatformpayload.Issue, 1)
	h.RegisterIssuesObserver(storedef.IssuesObserver{ResolvedCh: ch})

	h.ResolveIssue("t:id")
	h.sync()

	require.Len(t, ch, 1)
	got := <-ch
	assert.Equal(t, "custom_type", got.IssueType)
	assert.Equal(t, "configuration", got.Category)
}

func TestResolveAllIssuesPreservesIssueTypeAndCategory(t *testing.T) {
	h := newTestStore(t)
	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{
		Id: "t:1", IssueName: "t", IssueType: "custom_type", Category: "configuration",
	}))
	h.sync()

	ch := make(chan *healthplatformpayload.Issue, 1)
	h.RegisterIssuesObserver(storedef.IssuesObserver{ResolvedCh: ch})

	h.ResolveAllIssues()
	h.sync()

	require.Len(t, ch, 1)
	got := <-ch
	assert.Equal(t, "custom_type", got.IssueType)
	assert.Equal(t, "configuration", got.Category)
}

func TestResolveIssueUnknownIDIsNoop(t *testing.T) {
	h := newTestStore(t)
	h.ResolveIssue("nonexistent") // must not panic or error
	h.sync()
}

func TestResolveAllIssues(t *testing.T) {
	h := newTestStore(t)
	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "t:1", IssueName: "t"}))
	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "t:2", IssueName: "t"}))
	h.sync()

	h.ResolveAllIssues()
	h.sync()

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
	h.sync()

	_, issues := h.GetAllIssues()
	got := issues["t:id"]
	require.NotNil(t, got)

	// Mutating the returned value must not affect the in-store issue.
	originalSource := h.issues["t:id"].issue.Source
	got.Source = "hacked"
	assert.Equal(t, originalSource, h.issues["t:id"].issue.Source)
}

func TestMultiInstanceSameType(t *testing.T) {
	h := newTestStore(t)

	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "db-error:prod-1", IssueName: "db-error"}))
	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "db-error:prod-2", IssueName: "db-error"}))
	h.sync()

	count, issues := h.GetAllIssues()
	assert.Equal(t, 2, count)
	assert.Contains(t, issues, "db-error:prod-1")
	assert.Contains(t, issues, "db-error:prod-2")
}

func TestMultiTypeSameSource(t *testing.T) {
	h := newTestStore(t)

	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "a:id", IssueName: "type-a", Source: "mysrc"}))
	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "b:id", IssueName: "type-b", Source: "mysrc"}))
	h.sync()

	count, _ := h.GetAllIssues()
	assert.Equal(t, 2, count)
}

func TestPersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "issues.json")
	logger := logmock.New(t)

	h1 := newUnstartedTestStore(t)
	h1.persistence = newDiskPersistence(path, logger)
	startTestStore(t, h1)
	require.NoError(t, h1.ReportIssue(&healthplatformpayload.Issue{
		Id: "t:id", IssueName: "t", IssueType: "custom_type", Category: "configuration", Title: "Test Issue", Source: "test-src",
	}))
	h1.sync()
	firstSeen := h1.persistedIssues["t:id"].FirstSeen

	stopTestStore(t, h1)

	h2 := newUnstartedTestStore(t)
	h2.persistence = newDiskPersistence(path, logger)
	startTestStore(t, h2)

	// Detailed proto payload is not persisted — it is repopulated when the check re-runs.
	// What must survive is the lifecycle state so that storeIssue can correctly
	// resume firstSeen and state on the next ReportIssue call.
	persisted := h2.persistedIssues["t:id"]
	require.NotNil(t, persisted, "lifecycle state must survive persistence round-trip")
	assert.Equal(t, "t:id", persisted.IssueID)
	assert.Equal(t, "t", persisted.IssueType)
	assert.Equal(t, "custom_type", persisted.ProtoIssueType, "proto IssueType must survive persistence round-trip")
	assert.Equal(t, "configuration", persisted.Category)
	assert.Equal(t, firstSeen, persisted.FirstSeen)
	assert.Equal(t, IssueStateActive, persisted.State)

	// Re-reporting the same issue picks up the persisted firstSeen.
	require.NoError(t, h2.ReportIssue(&healthplatformpayload.Issue{
		Id: "t:id", IssueName: "t", Category: "permissions", Title: "Test Issue", Source: "test-src",
	}))
	h2.sync()
	assert.Equal(t, firstSeen, h2.persistedIssues["t:id"].FirstSeen, "firstSeen must be preserved across restart")
	assert.Equal(t, IssueStateActive, h2.persistedIssues["t:id"].State)
	assert.Equal(t, "permissions", h2.persistedIssues["t:id"].Category, "category must reflect the latest report")
}

func TestLoadPersistedStatePreservesIssueTypeAndCategoryOnResolvedTombstone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "issues.json")
	logger := logmock.New(t)

	h1 := newUnstartedTestStore(t)
	h1.persistence = newDiskPersistence(path, logger)
	startTestStore(t, h1)
	require.NoError(t, h1.ReportIssue(&healthplatformpayload.Issue{
		Id: "t:id", IssueName: "t", IssueType: "custom_type", Category: "configuration",
	}))
	h1.sync()
	h1.ResolveIssue("t:id")
	h1.sync()

	stopTestStore(t, h1)

	h2 := newUnstartedTestStore(t)
	h2.persistence = newDiskPersistence(path, logger)
	ch := make(chan *healthplatformpayload.Issue, 1)
	h2.RegisterIssuesObserver(storedef.IssuesObserver{ResolvedCh: ch})
	startTestStore(t, h2)

	require.Len(t, ch, 1)
	got := <-ch
	assert.Equal(t, "custom_type", got.IssueType)
	assert.Equal(t, "configuration", got.Category)
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
				"state":      "active",
				"first_seen": time.Now().Format(time.RFC3339),
				"last_seen":  time.Now().Format(time.RFC3339),
			},
		},
	}
	data, err := json.Marshal(stale)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0644))

	h := newUnstartedTestStore(t)
	h.persistence = newDiskPersistence(path, logmock.New(t))
	require.NoError(t, h.loadPersistedState(context.Background()))

	// Stale version: store must start fresh.
	assert.Nil(t, h.GetIssue("t:id"))
	count, _ := h.GetAllIssues()
	assert.Equal(t, 0, count)
}

func TestResolvedTTLPruning(t *testing.T) {
	h := newTestStore(t)
	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "t:id", IssueName: "t"}))
	h.sync()

	stopTestStore(t, h)

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
	h.sync()

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
	h.sync()

	fb := newMockFlareBuilder()
	require.NoError(t, h.fillFlare(context.Background(), fb))

	data, written := fb.get("health-platform-issues.json")
	require.True(t, written, "fillFlare must write a file when the store has issues")
	assert.Contains(t, string(data), "t:id")
}

func TestTelemetryCounterIncrements(t *testing.T) {
	h := newTestStore(t)
	counter := h.metrics.issuesCounter

	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "t:id", IssueName: "t"}))
	h.sync()

	assert.Equal(t, 1.0, counter.WithValues("t", "ISSUE_SEVERITY_UNSPECIFIED").Get())
}

func TestGetActiveIssueIDsByIssueName(t *testing.T) {
	h := newTestStore(t)
	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "t:1", IssueName: "t"}))
	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "t:2", IssueName: "t"}))
	h.sync()

	ids := h.GetActiveIssueIDsByIssueName("t")
	assert.ElementsMatch(t, []string{"t:1", "t:2"}, ids)

	h.ResolveIssue("t:1")
	h.sync()
	ids = h.GetActiveIssueIDsByIssueName("t")
	assert.ElementsMatch(t, []string{"t:2"}, ids)
}

func newTestObserver(resolvedSz int) storedef.IssuesObserver {
	return storedef.IssuesObserver{
		ResolvedCh: make(chan *healthplatformpayload.Issue, resolvedSz),
	}
}

// TestIssuesObserverResolvedNotification verifies that ResolvedCh receives a tombstone on ResolveIssue.
func TestIssuesObserverResolvedNotification(t *testing.T) {
	h := newTestStore(t)
	obs := newTestObserver(4)
	h.RegisterIssuesObserver(obs)

	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "i:1", IssueName: "t"}))
	h.ResolveIssue("i:1")
	h.sync()

	require.Len(t, obs.ResolvedCh, 1)
	got := <-obs.ResolvedCh
	assert.Equal(t, IssueStateResolved, got.PersistedIssue.GetState())
}

// TestResourceIdentityHost verifies the default, non-Kubernetes fallback:
// resource_type/resource_id describe the plain host agent process by hostname.
func TestResourceIdentityHost(t *testing.T) {
	h := newTestStore(t)

	resourceType, resourceID := h.ResourceIdentity("test-host")
	assert.Equal(t, "host", resourceType)
	assert.Equal(t, "test-host", resourceID)
}

func (h *healthPlatformImpl) queueLen() int {
	h.ingestMu.Lock()
	defer h.ingestMu.Unlock()
	return len(h.ingestQueue)
}

func TestIngestOverflow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newUnstartedTestStore(t)
		for i := 0; i < ingestChanBuf+100; i++ {
			require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: strconv.Itoa(i), IssueName: "test"}))
		}
		assert.Equal(t, ingestChanBuf, h.queueLen())
		assert.Equal(t, 100.0, h.metrics.issuesDropped.WithValues("report").Get())
		assert.Equal(t, float64(ingestChanBuf), h.metrics.ingestQueueLen.WithValues().Get())
		startTestStore(t, h)
		synctest.Wait()
		h.sync()
		count, _ := h.GetAllIssues()
		assert.Equal(t, ingestChanBuf, count)
		for i := 0; i < 100; i++ {
			assert.Nil(t, h.GetIssue(strconv.Itoa(i)))
		}
		for i := 100; i < ingestChanBuf+100; i++ {
			assert.NotNil(t, h.GetIssue(strconv.Itoa(i)))
		}
		assert.Equal(t, float64(ingestChanBuf), h.metrics.issuesCounter.WithValues("test", "ISSUE_SEVERITY_UNSPECIFIED").Get())
	})
}

func TestIngestOverflowPreservesResolutions(t *testing.T) {
	for _, resolveAll := range []bool{false, true} {
		t.Run(fmt.Sprintf("resolve_all=%t", resolveAll), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := newUnstartedTestStore(t)
				p := &controlledPersistence{}
				h.persistence = p
				for i := 0; i < ingestChanBuf; i++ {
					require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: strconv.Itoa(i), IssueName: "test"}))
				}
				id := strconv.Itoa(ingestChanBuf - 1)
				if resolveAll {
					h.ResolveAllIssues()
				} else {
					h.ResolveIssue(id)
				}
				assert.Equal(t, ingestChanBuf, h.queueLen())
				assert.Zero(t, h.metrics.issuesDropped.WithValues("resolve").Get())
				assert.Zero(t, h.metrics.issuesDropped.WithValues("resolve_all").Get())
				assert.Equal(t, 1.0, h.metrics.issuesDropped.WithValues("report").Get())
				startTestStore(t, h)
				synctest.Wait()
				h.sync()
				assert.Nil(t, h.GetIssue(id))
				assert.Nil(t, h.GetIssue("0"))
				stopTestStore(t, h)
				state := p.snapshot()
				require.NotNil(t, state)
				require.Contains(t, state.Issues, id)
				assert.Equal(t, IssueStateResolved, state.Issues[id].State)
				assert.NotContains(t, state.Issues, "0")
			})
		})
	}
}

func TestIngestOverflowWithoutReportsPreservesBarriers(t *testing.T) {
	h := newUnstartedTestStore(t)
	for i := 0; i < ingestChanBuf; i++ {
		h.ResolveIssue(strconv.Itoa(i))
	}
	h.ResolveAllIssues()
	done := make(chan struct{})
	h.enqueue(ingestCmd{kind: cmdSync, done: done})
	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "dropped", IssueName: "test"}))
	assert.Equal(t, ingestChanBuf+2, h.queueLen())
	assert.Zero(t, h.metrics.issuesDropped.WithValues("resolve").Get())
	assert.Zero(t, h.metrics.issuesDropped.WithValues("resolve_all").Get())
	assert.Zero(t, h.metrics.issuesDropped.WithValues("sync").Get())
	assert.Equal(t, 1.0, h.metrics.issuesDropped.WithValues("report").Get())
	startTestStore(t, h)
	<-done
	h.sync()
	assert.Nil(t, h.GetIssue("dropped"))
}

func TestIngestVisibilityAndQueueMetric(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newUnstartedTestStore(t)
		require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "id", IssueName: "test"}))
		assert.Nil(t, h.GetIssue("id"))
		startTestStore(t, h)
		h.sync()
		synctest.Wait()
		require.NotNil(t, h.GetIssue("id"))
		assert.Equal(t, 1.0, h.metrics.issuesCounter.WithValues("test", "ISSUE_SEVERITY_UNSPECIFIED").Get())
		assert.Zero(t, h.metrics.ingestQueueLen.WithValues().Get())
	})
}

func TestIngestOrdering(t *testing.T) {
	for _, resolveAll := range []bool{false, true} {
		t.Run(fmt.Sprintf("resolve_all=%t", resolveAll), func(t *testing.T) {
			h := newUnstartedTestStore(t)
			obs := newTestObserver(1)
			h.RegisterIssuesObserver(obs)
			resolve := func() {
				if resolveAll {
					h.ResolveAllIssues()
				} else {
					h.ResolveIssue("id")
				}
			}
			require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "id", IssueName: "test", Title: "before"}))
			resolve()
			startTestStore(t, h)
			h.sync()
			assert.Nil(t, h.GetIssue("id"))
			require.Len(t, obs.ResolvedCh, 1)
			assert.Equal(t, IssueStateResolved, (<-obs.ResolvedCh).PersistedIssue.State)

			require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "id", IssueName: "test", Title: "before"}))
			resolve()
			require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "id", IssueName: "test", Title: "after"}))
			h.sync()
			issue := h.GetIssue("id")
			require.NotNil(t, issue)
			assert.Equal(t, "after", issue.Title)
			assert.Equal(t, IssueStateActive, issue.PersistedIssue.State)
			assert.Empty(t, issue.PersistedIssue.GetResolvedAt())
			require.Len(t, obs.ResolvedCh, 1)
			assert.Equal(t, IssueStateResolved, (<-obs.ResolvedCh).PersistedIssue.State)
		})
	}
}

func TestIngestClonesCallerIssue(t *testing.T) {
	h := newUnstartedTestStore(t)
	issue := &healthplatformpayload.Issue{
		Id: "id", IssueName: "test", Title: "original", Tags: []string{"original"},
		Extra:       &structpb.Struct{Fields: map[string]*structpb.Value{"key": structpb.NewStringValue("original")}},
		Remediation: &healthplatformpayload.Remediation{Summary: "original"},
	}
	require.NoError(t, h.ReportIssue(issue))
	issue.Title = "changed"
	issue.Tags[0] = "changed"
	issue.Extra.Fields["key"] = structpb.NewStringValue("changed")
	issue.Remediation.Summary = "changed"
	startTestStore(t, h)
	h.sync()
	got := h.GetIssue("id")
	require.NotNil(t, got)
	assert.Equal(t, "original", got.Title)
	assert.Equal(t, []string{"original"}, got.Tags)
	assert.Equal(t, "original", got.Extra.Fields["key"].GetStringValue())
	assert.Equal(t, "original", got.Remediation.Summary)
	assert.Empty(t, issue.DetectedAt)
	assert.Nil(t, issue.PersistedIssue)
}

type controlledPersistence struct {
	memPersistence
	entered chan struct{}
	release chan struct{}
	saves   atomic.Int32
	err     error
}

func (p *controlledPersistence) save(state *PersistedState) error {
	p.saves.Add(1)
	if p.entered != nil {
		p.entered <- struct{}{}
		<-p.release
	}
	if p.err != nil {
		return p.err
	}
	return p.memPersistence.save(state)
}

func (p *controlledPersistence) snapshot() *PersistedState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

func TestIngestDoesNotBlockOnPersistence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newUnstartedTestStore(t)
		p := &controlledPersistence{entered: make(chan struct{}, 1), release: make(chan struct{})}
		h.persistence = p
		startTestStore(t, h)
		t.Cleanup(func() { close(p.release) })
		require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "first", IssueName: "test"}))
		h.sync()
		time.Sleep(flushInterval)
		<-p.entered
		require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "second", IssueName: "test"}))
		h.ResolveIssue("first")
		h.ResolveAllIssues()
		require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "third", IssueName: "test"}))
		assert.Equal(t, 4, h.queueLen())
		assert.Nil(t, h.GetIssue("second"))
		p.release <- struct{}{}
		h.sync()
		assert.Nil(t, h.GetIssue("first"))
		assert.Nil(t, h.GetIssue("second"))
		require.NotNil(t, h.GetIssue("third"))
	})
}

func TestIngestCoalescesPersistence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newUnstartedTestStore(t)
		p := &controlledPersistence{}
		h.persistence = p
		startTestStore(t, h)
		for i := 0; i < 100; i++ {
			require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: strconv.Itoa(i), IssueName: "test"}))
		}
		h.sync()
		synctest.Wait()
		assert.Zero(t, p.saves.Load())
		time.Sleep(flushInterval)
		synctest.Wait()
		assert.Equal(t, int32(1), p.saves.Load())
		state := p.snapshot()
		require.NotNil(t, state)
		assert.Len(t, state.Issues, 100)
		time.Sleep(2 * flushInterval)
		synctest.Wait()
		assert.Equal(t, int32(1), p.saves.Load())
		h.ResolveIssue("missing")
		h.sync()
		time.Sleep(flushInterval)
		synctest.Wait()
		assert.Equal(t, int32(1), p.saves.Load())
		stopTestStore(t, h)
		assert.Equal(t, int32(1), p.saves.Load())
	})
}

func TestIngestDrainServicesPersistenceTicks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newUnstartedTestStore(t)
		p := &controlledPersistence{}
		h.persistence = p
		require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "first", IssueName: "test"}))
		h.drain(false, nil)
		require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "second", IssueName: "test"}))
		require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "third", IssueName: "test"}))
		ticker := time.NewTicker(flushInterval)
		defer ticker.Stop()
		time.Sleep(flushInterval)
		h.drain(true, ticker.C)
		assert.Equal(t, int32(1), p.saves.Load())
		state := p.snapshot()
		require.NotNil(t, state)
		assert.Contains(t, state.Issues, "first")
		assert.NotContains(t, state.Issues, "second")
		assert.NotContains(t, state.Issues, "third")
		count, _ := h.GetAllIssues()
		assert.Equal(t, 3, count)
	})
}

func TestIngestShutdownDrainsAndFlushes(t *testing.T) {
	h := newUnstartedTestStore(t)
	p := &controlledPersistence{}
	h.persistence = p
	for i := 0; i < 100; i++ {
		require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: strconv.Itoa(i), IssueName: "test"}))
	}
	h.ResolveIssue("0")
	startTestStore(t, h)
	stopTestStore(t, h)
	assert.Zero(t, h.queueLen())
	state := p.snapshot()
	require.NotNil(t, state)
	assert.Len(t, state.Issues, 100)
	assert.Equal(t, IssueStateResolved, state.Issues["0"].State)
	assert.Nil(t, h.GetIssue("0"))
	count, _ := h.GetAllIssues()
	assert.Equal(t, 99, count)
	for i := 0; i < ingestChanBuf+1; i++ {
		require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "late", IssueName: "test"}))
	}
	assert.Equal(t, 1.0, h.metrics.issuesDropped.WithValues("report").Get())
	assert.Nil(t, h.GetIssue("late"))
}

func TestIngestShutdownRespectsContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newUnstartedTestStore(t)
		p := &controlledPersistence{entered: make(chan struct{}, 1), release: make(chan struct{})}
		h.persistence = p
		require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "id", IssueName: "test"}))
		require.NoError(t, h.start(context.Background()))
		t.Cleanup(func() {
			close(p.release)
			select {
			case <-h.stopCh:
			default:
				require.NoError(t, h.stop(context.Background()))
			}
			<-h.doneCh
		})
		<-p.entered

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		stopped := make(chan error, 1)
		go func() { stopped <- h.stop(ctx) }()
		select {
		case err := <-stopped:
			require.NoError(t, err)
		case <-time.After(time.Second):
			t.Fatal("stop did not return within one second")
		}
		assert.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
		select {
		case <-h.doneCh:
			t.Fatal("ingest worker exited while persistence was blocked")
		default:
		}
	})
}

func TestIngestPersistenceFailureDoesNotBlockFurtherCommands(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newUnstartedTestStore(t)
		p := &controlledPersistence{err: assert.AnError}
		h.persistence = p
		startTestStore(t, h)
		require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "id", IssueName: "test"}))
		h.sync()
		time.Sleep(flushInterval)
		synctest.Wait()
		assert.Equal(t, int32(1), p.saves.Load())
		time.Sleep(flushInterval)
		synctest.Wait()
		assert.Equal(t, int32(2), p.saves.Load())
		h.ResolveIssue("id")
		h.sync()
		assert.Nil(t, h.GetIssue("id"))
		stopTestStore(t, h)
		assert.Equal(t, int32(3), p.saves.Load())
	})
}
