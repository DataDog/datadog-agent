// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package storeimpl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	saveCalls int
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
	m.saveCalls++
	return nil
}

func (m *memPersistence) saved() (*PersistedState, int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state, m.saveCalls
}

// blockingPersistence holds every save until release is closed, standing in
// for a slow or hung disk.
type blockingPersistence struct {
	memPersistence
	entered chan struct{}
	release chan struct{}
}

func (b *blockingPersistence) save(state *PersistedState) error {
	select {
	case b.entered <- struct{}{}:
	default:
	}
	<-b.release
	return b.memPersistence.save(state)
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

// newUnstartedTestStore returns a store whose worker isn't running yet:
// mutations queue up until it starts.
func newUnstartedTestStore(t *testing.T) *healthPlatformImpl {
	t.Helper()
	return newHealthPlatform(logmock.New(t), telemetrymock.New(t), &mockHostname{name: "test-host"},
		"agent", selfident.New(nil), &memPersistence{})
}

// startTestStore runs h's start hook and stops h when the test ends.
func startTestStore(t *testing.T, h *healthPlatformImpl) {
	t.Helper()
	require.NoError(t, h.start(context.Background()))
	t.Cleanup(func() { assert.NoError(t, h.stop(context.Background())) })
}

// newTestStore returns a started store.
func newTestStore(t *testing.T) *healthPlatformImpl {
	t.Helper()
	h := newUnstartedTestStore(t)
	startTestStore(t, h)
	return h
}

// flush waits until the worker has applied every mutation queued before it.
func flush(t *testing.T, h *healthPlatformImpl) {
	t.Helper()
	done := make(chan struct{})
	select {
	case h.ops <- func() { close(done) }:
	default:
		t.Fatal("ops queue full, cannot flush")
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("worker did not apply queued mutations")
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
	flush(t, h)

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
	flush(t, h)

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
	flush(t, h)

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
	flush(t, h)

	issue := h.GetIssue("id-2")
	require.NotNil(t, issue)
	assert.Empty(t, issue.IssueType)
}

func TestReportIssueStateTransition(t *testing.T) {
	h := newTestStore(t)
	issue := &healthplatformpayload.Issue{Id: "t:id", IssueName: "t"}

	require.NoError(t, h.ReportIssue(issue))
	flush(t, h)
	persisted := h.persistedIssues["t:id"]
	require.NotNil(t, persisted)
	assert.Equal(t, IssueStateActive, persisted.State)
	firstSeen := persisted.FirstSeen

	require.NoError(t, h.ReportIssue(issue))
	flush(t, h)
	persisted = h.persistedIssues["t:id"]
	assert.Equal(t, IssueStateActive, persisted.State)
	assert.Equal(t, firstSeen, persisted.FirstSeen, "FirstSeen must not change on re-report")
	assert.GreaterOrEqual(t, persisted.LastSeen, firstSeen)
}

func TestResolveIssueRemovesFromActive(t *testing.T) {
	h := newTestStore(t)
	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "t:id", IssueName: "t"}))

	ch := make(chan *healthplatformpayload.Issue, 1)
	h.RegisterIssuesObserver(storedef.IssuesObserver{ResolvedCh: ch})

	h.ResolveIssue("t:id")
	flush(t, h)

	// Issue must be removed from the active set; resolved snapshot written to ResolvedCh.
	assert.Nil(t, h.GetIssue("t:id"), "issue must be removed from active set after ResolveIssue")
	got := receiveResolved(t, ch)
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

	ch := make(chan *healthplatformpayload.Issue, 1)
	h.RegisterIssuesObserver(storedef.IssuesObserver{ResolvedCh: ch})

	h.ResolveIssue("t:id")
	flush(t, h)

	got := receiveResolved(t, ch)
	assert.Equal(t, "custom_type", got.IssueType)
	assert.Equal(t, "configuration", got.Category)
}

func TestResolveAllIssuesPreservesIssueTypeAndCategory(t *testing.T) {
	h := newTestStore(t)
	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{
		Id: "t:1", IssueName: "t", IssueType: "custom_type", Category: "configuration",
	}))

	ch := make(chan *healthplatformpayload.Issue, 1)
	h.RegisterIssuesObserver(storedef.IssuesObserver{ResolvedCh: ch})

	h.ResolveAllIssues()
	flush(t, h)

	got := receiveResolved(t, ch)
	assert.Equal(t, "custom_type", got.IssueType)
	assert.Equal(t, "configuration", got.Category)
}

func TestResolveIssueUnknownIDIsNoop(t *testing.T) {
	h := newTestStore(t)
	h.ResolveIssue("nonexistent") // must not panic or error
	flush(t, h)
}

func TestResolveAllIssues(t *testing.T) {
	h := newTestStore(t)
	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "t:1", IssueName: "t"}))
	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "t:2", IssueName: "t"}))

	h.ResolveAllIssues()
	flush(t, h)

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
	flush(t, h)

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
	flush(t, h)

	count, issues := h.GetAllIssues()
	assert.Equal(t, 2, count)
	assert.Contains(t, issues, "db-error:prod-1")
	assert.Contains(t, issues, "db-error:prod-2")
}

func TestMultiTypeSameSource(t *testing.T) {
	h := newTestStore(t)

	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "a:id", IssueName: "type-a", Source: "mysrc"}))
	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "b:id", IssueName: "type-b", Source: "mysrc"}))
	flush(t, h)

	count, _ := h.GetAllIssues()
	assert.Equal(t, 2, count)
}

func TestPersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "issues.json")
	logger := logmock.New(t)

	h1 := newUnstartedTestStore(t)
	h1.persistence = newDiskPersistence(path, logger)
	require.NoError(t, h1.start(context.Background()))
	require.NoError(t, h1.ReportIssue(&healthplatformpayload.Issue{
		Id: "t:id", IssueName: "t", IssueType: "custom_type", Category: "configuration", Title: "Test Issue", Source: "test-src",
	}))
	// stop applies the queued report and saves it.
	require.NoError(t, h1.stop(context.Background()))
	firstSeen := h1.persistedIssues["t:id"].FirstSeen

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
	flush(t, h2)
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
	require.NoError(t, h1.start(context.Background()))
	require.NoError(t, h1.ReportIssue(&healthplatformpayload.Issue{
		Id: "t:id", IssueName: "t", IssueType: "custom_type", Category: "configuration",
	}))
	h1.ResolveIssue("t:id")
	require.NoError(t, h1.stop(context.Background()))

	h2 := newUnstartedTestStore(t)
	h2.persistence = newDiskPersistence(path, logger)
	ch := make(chan *healthplatformpayload.Issue, 1)
	h2.RegisterIssuesObserver(storedef.IssuesObserver{ResolvedCh: ch})
	require.NoError(t, h2.loadPersistedState(context.Background()))

	got := receiveResolved(t, ch)
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
	flush(t, h)

	// Back-date the resolved-at timestamp so it's older than the TTL.
	h.issuesMux.Lock()
	h.persistedIssues["t:id"].State = IssueStateResolved
	h.persistedIssues["t:id"].ResolvedAt = time.Now().Add(-25 * time.Hour).Format(time.RFC3339)
	h.issuesMux.Unlock()

	mem := h.persistence.(*memPersistence)
	require.NoError(t, h.saveToDisk())

	require.NotNil(t, mem.state)
	assert.NotContains(t, mem.state.Issues, "t:id",
		"resolved issue older than TTL must be pruned on save")
}

func TestGetIssuesHTTPEndpoint(t *testing.T) {
	h := newTestStore(t)
	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "t:1", IssueName: "t"}))
	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "t:2", IssueName: "t"}))
	flush(t, h)

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
	flush(t, h)

	fb := newMockFlareBuilder()
	require.NoError(t, h.fillFlare(context.Background(), fb))

	data, written := fb.get("health-platform-issues.json")
	require.True(t, written, "fillFlare must write a file when the store has issues")
	assert.Contains(t, string(data), "t:id")
}

func TestTelemetryCounterIncrements(t *testing.T) {
	h := newTestStore(t)

	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "t:id", IssueName: "t"}))
	flush(t, h)

	assert.Equal(t, 1.0, h.metrics.issuesCounter.WithValues("t", "ISSUE_SEVERITY_UNSPECIFIED").Get())
}

func TestGetActiveIssueIDsByIssueName(t *testing.T) {
	h := newTestStore(t)
	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "t:1", IssueName: "t"}))
	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "t:2", IssueName: "t"}))
	flush(t, h)

	ids := h.GetActiveIssueIDsByIssueName("t")
	assert.ElementsMatch(t, []string{"t:1", "t:2"}, ids)

	h.ResolveIssue("t:1")
	flush(t, h)
	ids = h.GetActiveIssueIDsByIssueName("t")
	assert.ElementsMatch(t, []string{"t:2"}, ids)
}

// receiveResolved waits for the next tombstone an observer receives.
func receiveResolved(t *testing.T, ch <-chan *healthplatformpayload.Issue) *healthplatformpayload.Issue {
	t.Helper()
	select {
	case got := <-ch:
		return got
	case <-time.After(5 * time.Second):
		t.Fatal("observer did not receive a resolved issue")
		return nil
	}
}

func TestIssueLifecycleTelemetry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newTestStore(t)
		first := &healthplatformpayload.Issue{Id: "test:1", IssueName: "Test Issue", IssueType: "test_issue"}
		second := &healthplatformpayload.Issue{Id: "test:2", IssueName: "Test Issue", IssueType: "test_issue"}
		other := &healthplatformpayload.Issue{Id: "other:1", IssueName: "Other Issue", IssueType: "other_issue"}
		gauge := h.metrics.activeIssues.WithValues("test_issue")
		resolved := h.metrics.issuesResolved.WithValues("test_issue")

		require.NoError(t, h.ReportIssue(first))
		flush(t, h)
		assert.Equal(t, 1.0, gauge.Get())
		require.NoError(t, h.ReportIssue(first))
		flush(t, h)
		assert.Equal(t, 1.0, gauge.Get())
		require.NoError(t, h.ReportIssue(second))
		require.NoError(t, h.ReportIssue(other))
		flush(t, h)
		assert.Equal(t, 2.0, gauge.Get())
		assert.Equal(t, 1.0, h.metrics.activeIssues.WithValues("other_issue").Get())

		time.Sleep(time.Hour)
		h.ResolveIssue(first.Id)
		flush(t, h)
		assert.Equal(t, 1.0, gauge.Get())
		assert.Equal(t, 1.0, resolved.Get())

		h.ResolveIssue(first.Id)
		h.ResolveIssue("unknown")
		flush(t, h)
		assert.Equal(t, 1.0, resolved.Get())

		h.ResolveAllIssues()
		flush(t, h)
		assert.Zero(t, gauge.Get())
		assert.Zero(t, h.metrics.activeIssues.WithValues("other_issue").Get())
		assert.Equal(t, 2.0, resolved.Get())
		assert.Equal(t, 1.0, h.metrics.issuesResolved.WithValues("other_issue").Get())
		h.ResolveAllIssues()
		flush(t, h)
		assert.Equal(t, 2.0, resolved.Get())

		require.NoError(t, h.ReportIssue(first))
		flush(t, h)
		assert.Equal(t, 1.0, gauge.Get())
		h.ResolveIssue(first.Id)
		flush(t, h)
		assert.Equal(t, 3.0, resolved.Get())
	})
}

func TestRestoreActiveIssuesTelemetry(t *testing.T) {
	h := newTestStore(t)
	require.NoError(t, h.restorePersistedState(&PersistedState{
		Version: persistedStateVersion,
		Issues: map[string]*PersistedIssue{
			"test:1":     {IssueType: "Test Issue", ProtoIssueType: "test_issue", State: IssueStateActive},
			"test:2":     {IssueType: "Test Issue", ProtoIssueType: "test_issue", State: IssueStateActive},
			"other:1":    {IssueType: "Other Issue", ProtoIssueType: "other_issue", State: IssueStateActive},
			"resolved:1": {IssueType: "Resolved Issue", ProtoIssueType: "resolved_issue", State: IssueStateResolved},
		},
	}))
	assert.Equal(t, 2.0, h.metrics.activeIssues.WithValues("test_issue").Get())
	assert.Equal(t, 1.0, h.metrics.activeIssues.WithValues("other_issue").Get())
	assert.Zero(t, h.metrics.activeIssues.WithValues("resolved_issue").Get())
	assert.Zero(t, h.metrics.issuesResolved.WithValues("resolved_issue").Get())

	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "test:1", IssueName: "Test Issue", IssueType: "test_issue"}))
	flush(t, h)
	assert.Equal(t, 2.0, h.metrics.activeIssues.WithValues("test_issue").Get())
	h.ResolveIssue("test:2")
	flush(t, h)
	assert.Equal(t, 1.0, h.metrics.activeIssues.WithValues("test_issue").Get())
	h.ResolveAllIssues()
	flush(t, h)
	assert.Zero(t, h.metrics.activeIssues.WithValues("test_issue").Get())
	assert.Zero(t, h.metrics.activeIssues.WithValues("other_issue").Get())
}

func TestActiveGaugeTracksProtoTypeChanges(t *testing.T) {
	h := newTestStore(t)
	require.NoError(t, h.restorePersistedState(&PersistedState{
		Version: persistedStateVersion,
		Issues: map[string]*PersistedIssue{
			"test:1":         {IssueType: "Test Issue", State: IssueStateActive},
			"other:1":        {IssueType: "Other Issue", State: IssueStateActive},
			"missing-name:1": {State: IssueStateActive},
		},
	}))
	unknown := h.metrics.activeIssues.WithValues("unknown")
	assert.Equal(t, 2.0, unknown.Get())

	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "test:1", IssueName: "Test Issue", IssueType: "test_issue"}))
	flush(t, h)
	assert.Equal(t, 1.0, unknown.Get())
	assert.Equal(t, 1.0, h.metrics.activeIssues.WithValues("test_issue").Get())
	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "test:1", IssueName: "Changed Issue", IssueType: "changed_issue"}))
	flush(t, h)
	assert.Zero(t, h.metrics.activeIssues.WithValues("test_issue").Get())
	assert.Equal(t, 1.0, h.metrics.activeIssues.WithValues("changed_issue").Get())

	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "unknown:1", IssueName: "Unknown Issue"}))
	flush(t, h)
	assert.Equal(t, 2.0, unknown.Get())
	h.ResolveIssue("other:1")
	flush(t, h)
	assert.Equal(t, 1.0, unknown.Get())
	h.ResolveAllIssues()
	flush(t, h)
	assert.Zero(t, unknown.Get())
	assert.Zero(t, h.metrics.activeIssues.WithValues("changed_issue").Get())
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
	flush(t, h)

	got := receiveResolved(t, obs.ResolvedCh)
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

// TestMutationsDoNotWaitForSlowPersistence verifies that a hung save blocks
// neither callers nor the worker: reports and resolutions keep applying while
// the persister is stuck.
func TestMutationsDoNotWaitForSlowPersistence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		persistence := &blockingPersistence{entered: make(chan struct{}, 1), release: make(chan struct{})}
		h := newUnstartedTestStore(t)
		h.persistence = persistence
		startTestStore(t, h)
		// Registered after startTestStore, so it runs first and lets stop's final save finish.
		t.Cleanup(func() { close(persistence.release) })

		require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "t:0", IssueName: "t"}))
		time.Sleep(persistInterval) // advance virtual time to trigger the first save
		synctest.Wait()             // the worker applies t:0 and the persister blocks in save()

		// Verify callers don't block while the persister is stuck.
		for i := 1; i <= 100; i++ {
			assert.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: fmt.Sprintf("t:%d", i), IssueName: "t"}))
		}
		h.ResolveIssue("t:0")
		h.ResolveAllIssues()
		synctest.Wait() // the worker applies mutations while the persister is still blocked

		count, _ := h.GetAllIssues()
		assert.Zero(t, count, "the worker kept applying mutations while the save was stuck")
	})
}

// TestBurstOfReportsIsPersistedOnce verifies that persistence is driven by the
// timer rather than by each mutation: a burst costs no writes until the
// interval elapses, and stop saves everything still pending exactly once.
func TestBurstOfReportsIsPersistedOnce(t *testing.T) {
	h := newUnstartedTestStore(t)
	h.persistInterval = time.Hour
	mem := h.persistence.(*memPersistence)
	require.NoError(t, h.start(context.Background()))

	for i := 0; i < 100; i++ {
		require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: fmt.Sprintf("t:%d", i%10), IssueName: "t"}))
	}
	h.ResolveIssue("t:0")
	flush(t, h)
	_, saves := mem.saved()
	assert.Zero(t, saves, "nothing is written before the persist interval elapses")

	require.NoError(t, h.stop(context.Background()))
	state, saves := mem.saved()
	assert.Equal(t, 1, saves, "stop saves pending changes once")
	require.NotNil(t, state)
	assert.Len(t, state.Issues, 10)
	assert.Equal(t, IssueStateResolved, state.Issues["t:0"].State)
}

// TestStopTimeoutKeepsFinalSaveAfterDrain verifies that when stop's context
// expires before the worker finishes draining, the final save still waits for
// the drain, so it includes every mutation accepted before stop.
func TestStopTimeoutKeepsFinalSaveAfterDrain(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newUnstartedTestStore(t)
		h.persistInterval = time.Hour
		mem := h.persistence.(*memPersistence)
		// Start only the persister: the worker stands in for one still draining.
		go h.runPersister()
		require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "t:0", IssueName: "t"}))
		h.ResolveIssue("t:0")

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		require.ErrorIs(t, h.stop(ctx), context.Canceled)
		synctest.Wait()
		select {
		case <-h.persisterDone:
			t.Fatal("the final save must wait for the worker to drain")
		default:
		}

		go h.runWorker()
		select {
		case <-h.persisterDone:
		case <-time.After(5 * time.Second):
			t.Fatal("the persister did not stop after the drain")
		}
		state, saves := mem.saved()
		assert.Equal(t, 1, saves)
		require.NotNil(t, state)
		assert.Equal(t, IssueStateResolved, state.Issues["t:0"].State, "the final save must include the drained resolution")
	})
}

// TestPersistsOnTimerOnlyWhenChanged verifies that the persister saves on its
// interval once state changed, and skips ticks where nothing changed.
func TestPersistsOnTimerOnlyWhenChanged(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newTestStore(t)
		mem := h.persistence.(*memPersistence)

		require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "t:id", IssueName: "t"}))
		synctest.Wait()
		_, saves := mem.saved()
		assert.Zero(t, saves)

		time.Sleep(persistInterval)
		synctest.Wait()
		state, saves := mem.saved()
		assert.Equal(t, 1, saves)
		require.NotNil(t, state)
		assert.Equal(t, IssueStateActive, state.Issues["t:id"].State)

		time.Sleep(10 * persistInterval)
		synctest.Wait()
		_, saves = mem.saved()
		assert.Equal(t, 1, saves, "no save while nothing changed")

		h.ResolveIssue("t:id")
		time.Sleep(persistInterval)
		synctest.Wait()
		state, saves = mem.saved()
		assert.Equal(t, 2, saves)
		assert.Equal(t, IssueStateResolved, state.Issues["t:id"].State)
	})
}

// TestFailedSaveIsRetried verifies that a failed save is retried on the next
// tick even when nothing else changes.
func TestFailedSaveIsRetried(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newUnstartedTestStore(t)
		persistence := &failingPersistence{failures: 1}
		h.persistence = persistence
		startTestStore(t, h)

		require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "t:id", IssueName: "t"}))
		time.Sleep(persistInterval)
		synctest.Wait()
		_, saves := persistence.saved()
		assert.Zero(t, saves, "first save failed")

		time.Sleep(persistInterval)
		synctest.Wait()
		state, saves := persistence.saved()
		assert.Equal(t, 1, saves, "retried on the next tick")
		require.NotNil(t, state)
		assert.Contains(t, state.Issues, "t:id")
	})
}

// failingPersistence fails its first failures saves, then stores in memory.
type failingPersistence struct {
	memPersistence
	failures int
}

func (f *failingPersistence) save(state *PersistedState) error {
	f.mu.Lock()
	if f.failures > 0 {
		f.failures--
		f.mu.Unlock()
		return assert.AnError
	}
	f.mu.Unlock()
	return f.memPersistence.save(state)
}

// TestMutationsDropAndCountWhenQueueFull verifies that a full queue makes
// mutations drop rather than wait, without surfacing an error to the caller.
func TestMutationsDropAndCountWhenQueueFull(t *testing.T) {
	h := newUnstartedTestStore(t) // no worker, so nothing drains the queue
	for i := 0; i < opsQueueSize; i++ {
		require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: fmt.Sprintf("t:%d", i), IssueName: "t"}))
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		assert.Error(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "t:overflow", IssueName: "t"}),
			"a full queue must return an error so callers can retry")
		assert.ErrorIs(t, h.ResolveIssue("t:0"), storedef.ErrQueueFull,
			"a dropped resolution must return an error so callers can retry")
		h.ResolveAllIssues()
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("callers blocked on a full queue")
	}

	dropped := h.metrics.droppedOperations
	assert.Equal(t, 1.0, dropped.WithValues("report_issue").Get())
	assert.Equal(t, 1.0, dropped.WithValues("resolve_issue").Get())
	assert.Equal(t, 1.0, dropped.WithValues("resolve_all_issues").Get())
}

// TestMutationsApplyInCallOrder verifies that the worker applies reports and
// resolutions for the same issue in the order they were made.
func TestMutationsApplyInCallOrder(t *testing.T) {
	h := newTestStore(t)

	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "a", IssueName: "t"}))
	h.ResolveIssue("a")
	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "a", IssueName: "t"}))
	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "b", IssueName: "t"}))
	h.ResolveIssue("b")
	flush(t, h)

	assert.NotNil(t, h.GetIssue("a"), "re-reported after its resolution")
	assert.Nil(t, h.GetIssue("b"), "resolved after its report")
}

// TestReportIssueCopiesCallerIssue verifies that the queued issue is detached
// from the caller's, so a reporter reusing its *Issue can't change what's stored.
func TestReportIssueCopiesCallerIssue(t *testing.T) {
	h := newTestStore(t)
	issue := &healthplatformpayload.Issue{Id: "t:id", IssueName: "t", Title: "original"}

	require.NoError(t, h.ReportIssue(issue))
	issue.Title = "reused"
	flush(t, h)

	assert.Equal(t, "original", h.GetIssue("t:id").GetTitle())
	assert.Empty(t, issue.DetectedAt, "the store must not write to the caller's issue")
}

// TestReportsQueuedBeforeStartApplyAfterRestore verifies that the worker only
// starts once persisted state is restored, so an early report updates the
// restored record instead of being overwritten by it.
func TestReportsQueuedBeforeStartApplyAfterRestore(t *testing.T) {
	old := time.Now().Add(-time.Hour).Format(time.RFC3339)
	h := newUnstartedTestStore(t)
	h.persistence = &memPersistence{state: &PersistedState{
		Version: persistedStateVersion,
		Issues: map[string]*PersistedIssue{
			"t:id": {IssueType: "t", State: IssueStateActive, FirstSeen: old, LastSeen: old},
		},
	}}

	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "t:id", IssueName: "t"}))
	startTestStore(t, h)
	flush(t, h)

	persisted := h.persistedIssues["t:id"]
	require.NotNil(t, persisted)
	assert.Equal(t, old, persisted.FirstSeen, "FirstSeen comes from the restored record")
	assert.NotEqual(t, old, persisted.LastSeen, "the report applied on top of the restored record")
	assert.NotNil(t, h.GetIssue("t:id"))
}

// TestReportsRacingStopAreAllAppliedAndSaved verifies that every report
// ReportIssue accepts while stop runs concurrently is applied and included in
// the final save: closing admission and sending must be one step relative to
// stop, or a report could land after the worker drained and be lost. Reporters
// keep reporting until after stop returns, and stop starts only once reports
// are flowing, so reports overlap the closing transition; the scenario repeats
// to widen that window.
func TestReportsRacingStopAreAllAppliedAndSaved(t *testing.T) {
	for range 20 {
		h := newUnstartedTestStore(t)
		h.persistInterval = time.Hour // only the final save on stop
		mem := h.persistence.(*memPersistence)
		require.NoError(t, h.start(context.Background()))

		var (
			mu         sync.Mutex
			accepted   []string
			unexpected []error
			flowing    atomic.Int32
			wg         sync.WaitGroup
		)
		stopped := make(chan struct{})
		for r := range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; ; i++ {
					select {
					case <-stopped:
						return
					default:
					}
					id := fmt.Sprintf("t:%d:%d", r, i)
					err := h.ReportIssue(&healthplatformpayload.Issue{Id: id, IssueName: "t"})
					mu.Lock()
					switch {
					case err == nil:
						accepted = append(accepted, id)
						flowing.Add(1)
					case !errors.Is(err, storedef.ErrQueueFull):
						unexpected = append(unexpected, err)
					}
					mu.Unlock()
				}
			}()
		}
		require.Eventually(t, func() bool { return flowing.Load() >= 50 }, 5*time.Second, time.Millisecond)
		require.NoError(t, h.stop(context.Background()))
		close(stopped)
		wg.Wait()

		mu.Lock()
		require.Empty(t, unexpected)
		state, _ := mem.saved()
		require.NotNil(t, state, "reports were accepted, so stop must save")
		for _, id := range accepted {
			require.NotNil(t, h.GetIssue(id), "accepted report %s must be applied", id)
			require.Contains(t, state.Issues, id, "accepted report %s must be in the final save", id)
		}
		mu.Unlock()
	}
}

// TestMutationsAfterStopAreDroppedAndCounted verifies that once stop began,
// mutations are rejected without blocking and counted as dropped.
func TestMutationsAfterStopAreDroppedAndCounted(t *testing.T) {
	h := newUnstartedTestStore(t)
	require.NoError(t, h.start(context.Background()))
	require.NoError(t, h.stop(context.Background()))

	require.ErrorIs(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "t:1", IssueName: "t"}), storedef.ErrQueueFull)
	require.ErrorIs(t, h.ResolveIssue("t:1"), storedef.ErrQueueFull)
	h.ResolveAllIssues()

	dropped := h.metrics.droppedOperations
	assert.Equal(t, 1.0, dropped.WithValues("report_issue").Get())
	assert.Equal(t, 1.0, dropped.WithValues("resolve_issue").Get())
	assert.Equal(t, 1.0, dropped.WithValues("resolve_all_issues").Get())
	assert.Nil(t, h.GetIssue("t:1"))
}

// TestReadsRaceWithWorkerAndPersister exercises every read path while the
// worker applies mutations and the persister saves, so the race detector
// (dda inv test --race) catches any read that skips the store's locks.
func TestReadsRaceWithWorkerAndPersister(t *testing.T) {
	h := newUnstartedTestStore(t)
	h.persistInterval = time.Millisecond
	startTestStore(t, h)

	const rounds = 200
	var wg sync.WaitGroup
	for w := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range rounds {
				id := fmt.Sprintf("t:%d:%d", w, i%10)
				_ = h.ReportIssue(&healthplatformpayload.Issue{Id: id, IssueName: "t", IssueType: "t_type"})
				if i%3 == 0 {
					h.ResolveIssue(id)
				}
				if i%50 == 0 {
					h.ResolveAllIssues()
				}
			}
		}()
	}
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range rounds {
				h.GetAllIssues()
				h.GetIssue(fmt.Sprintf("t:0:%d", i%10))
				h.GetActiveIssueIDsByIssueName("t")
				h.getIssuesHandler(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health-platform/issues", nil))
				assert.NoError(t, h.fillFlare(context.Background(), newMockFlareBuilder()))
			}
		}()
	}
	// No flush: the writers can leave the queue full, and stop in the
	// cleanup drains it.
	wg.Wait()
}

// TestChangeAppliedDuringSaveIsSavedNextTick verifies that a change applied
// while a save is in progress is not lost: the next tick saves it.
func TestChangeAppliedDuringSaveIsSavedNextTick(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		persistence := &blockingPersistence{entered: make(chan struct{}, 1), release: make(chan struct{})}
		var releaseOnce sync.Once
		release := func() { releaseOnce.Do(func() { close(persistence.release) }) }
		h := newUnstartedTestStore(t)
		h.persistence = persistence
		require.NoError(t, h.start(context.Background()))
		// On any failure, unblock the persister and stop the store so the
		// bubble exits with the real assertion message.
		defer func() {
			release()
			_ = h.stop(context.Background())
		}()

		require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "t:before", IssueName: "t"}))
		time.Sleep(persistInterval)
		synctest.Wait()
		select {
		case <-persistence.entered: // the persister snapshotted t:before and blocks in save
		default:
			t.Fatal("the save never started")
		}

		require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "t:during", IssueName: "t"}))
		flush(t, h)
		release()
		synctest.Wait()
		state, _ := persistence.saved()
		require.NotNil(t, state)
		require.Contains(t, state.Issues, "t:before")
		require.NotContains(t, state.Issues, "t:during", "the in-progress save snapshotted before the change")

		time.Sleep(persistInterval)
		synctest.Wait()
		state, _ = persistence.saved()
		assert.Contains(t, state.Issues, "t:during", "the next tick must save the change applied during the save")
	})
}

// TestWorkerSurvivesPanickingChange verifies that a panic while applying one
// change is recovered and the worker keeps applying the next ones.
func TestWorkerSurvivesPanickingChange(t *testing.T) {
	h := newTestStore(t)
	require.True(t, h.enqueue("test_panic", func() { panic("boom") }))
	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "t:after", IssueName: "t"}))
	flush(t, h)
	assert.NotNil(t, h.GetIssue("t:after"), "the worker must keep running after a panic")
}
