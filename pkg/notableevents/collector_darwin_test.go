// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package notableevents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

type fakeDarwinBookmarkStore struct {
	mu        sync.Mutex
	state     *darwinBookmarkState
	loadErr   error
	saveErr   error
	loadCalls int
	saveCalls int
	onSave    func(*darwinBookmarkState)
}

func TestCloneEventPreservesCustomNumbers(t *testing.T) {
	const exactInteger = "9007199254740993"
	event := Event{
		Custom: map[string]interface{}{
			"nested": []interface{}{
				map[string]interface{}{"value": json.Number(exactInteger)},
			},
		},
	}

	cloned := cloneEvent(event)
	number, ok := cloned.Custom["nested"].([]interface{})[0].(map[string]interface{})["value"].(json.Number)
	require.True(t, ok)
	assert.Equal(t, json.Number(exactInteger), number)

	encoded, err := json.Marshal(cloned)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"value":`+exactInteger)
}

func TestDarwinScanIncidentReconciliationInvariants(t *testing.T) {
	now := time.Date(2026, time.July, 22, 12, 0, 0, 0, time.UTC)
	deliverable := func(event Event, sourceKey, runtimeDirKey, stateDirKey, name string) darwinScanDeliverable {
		return darwinScanDeliverable{
			Event:     event,
			SourceKey: sourceKey,
			ContributingDirKeys: map[string]struct{}{
				runtimeDirKey: {},
			},
			ContributingReports: map[string]darwinReportSource{
				sourceKey: {DirectoryStateKey: stateDirKey, Name: name},
			},
		}
	}

	t.Run("deliverable overrides baseline", func(t *testing.T) {
		event := validPersistedDarwinEvent("deliverable-baseline")
		incidents := newDarwinScanIncidents()
		incidents.baselineIDs[event.ID] = struct{}{}
		incidents.deliverables[event.ID] = deliverable(event, "b/report", "runtime-b", "state-b", "report.ips")
		state := newDarwinBookmarkState()

		assert.True(t, incidents.reconcile(state, make(map[string]bool), now))
		assert.Equal(t, event, state.Pending[event.ID])
		assert.NotContains(t, state.Acknowledged, event.ID)
		require.NoError(t, validateDarwinBookmarkState(state))
	})

	t.Run("pre-pending remains pending", func(t *testing.T) {
		event := validPersistedDarwinEvent("pre-pending")
		state := newDarwinBookmarkState()
		state.Pending[event.ID] = event
		incidents := newDarwinScanIncidents()
		incidents.baselineIDs[event.ID] = struct{}{}
		incidents.deliverables[event.ID] = deliverable(event, "a/report", "runtime-a", "state-a", "report.ips")

		assert.False(t, incidents.reconcile(state, make(map[string]bool), now))
		assert.Equal(t, event, state.Pending[event.ID])
		assert.NotContains(t, state.Acknowledged, event.ID)
		require.NoError(t, validateDarwinBookmarkState(state))
	})

	t.Run("pre-acknowledged remains authoritative", func(t *testing.T) {
		event := validPersistedDarwinEvent("pre-acknowledged")
		state := newDarwinBookmarkState()
		state.Acknowledged[event.ID] = now.Add(-time.Hour).Unix()
		incidents := newDarwinScanIncidents()
		incidents.deliverables[event.ID] = deliverable(event, "a/report", "runtime-a", "state-a", "report.ips")

		assert.False(t, incidents.reconcile(state, make(map[string]bool), now))
		assert.NotContains(t, state.Pending, event.ID)
		assert.Contains(t, state.Acknowledged, event.ID)
		require.NoError(t, validateDarwinBookmarkState(state))
	})

	t.Run("capacity overflow retries every contributor", func(t *testing.T) {
		state := newDarwinBookmarkState()
		for index := 0; index < maxDarwinPendingEvents; index++ {
			event := validPersistedDarwinEvent(fmt.Sprintf("existing-%03d", index))
			state.Pending[event.ID] = event
		}
		event := validPersistedDarwinEvent("overflow")
		stateA := hashString("state-a")
		stateB := hashString("state-b")
		state.Directories[stateA] = &directoryBookmarkState{
			Initialized: true,
			LastSeen:    now.Unix(),
			Files:       map[string]reportFileState{"a.ips": {EventID: event.ID}},
		}
		state.Directories[stateB] = &directoryBookmarkState{
			Initialized: true,
			LastSeen:    now.Unix(),
			Files:       map[string]reportFileState{"b.ips": {EventID: event.ID}},
		}
		incidents := newDarwinScanIncidents()
		incidents.baselineIDs[event.ID] = struct{}{}
		incidents.addDirectoryResult(directoryScanResult{Deliverables: map[string]darwinScanDeliverable{
			event.ID: deliverable(event, "b/report", "runtime-b", stateB, "b.ips"),
		}})
		incidents.addDirectoryResult(directoryScanResult{Deliverables: map[string]darwinScanDeliverable{
			event.ID: deliverable(event, "a/report", "runtime-a", stateA, "a.ips"),
		}})
		retries := make(map[string]bool)

		assert.True(t, incidents.reconcile(state, retries, now))
		assert.NotContains(t, state.Pending, event.ID)
		assert.NotContains(t, state.Acknowledged, event.ID)
		assert.Equal(t, map[string]bool{"runtime-a": true, "runtime-b": true}, retries)
		assert.Empty(t, state.Directories[stateA].Files)
		assert.Empty(t, state.Directories[stateB].Files)
		require.NoError(t, validateDarwinBookmarkState(state))
	})

	t.Run("duplicate selects smallest source", func(t *testing.T) {
		larger := validPersistedDarwinEvent("duplicate")
		larger.Title = "larger source"
		smaller := cloneEvent(larger)
		smaller.Title = "smaller source"
		incidents := newDarwinScanIncidents()
		incidents.addDirectoryResult(directoryScanResult{Deliverables: map[string]darwinScanDeliverable{
			larger.ID: deliverable(larger, "z/report", "runtime-z", "state-z", "z.ips"),
		}})
		incidents.addDirectoryResult(directoryScanResult{Deliverables: map[string]darwinScanDeliverable{
			smaller.ID: deliverable(smaller, "a/report", "runtime-a", "state-a", "a.ips"),
		}})
		state := newDarwinBookmarkState()

		assert.True(t, incidents.reconcile(state, make(map[string]bool), now))
		assert.Equal(t, "smaller source", state.Pending[smaller.ID].Title)
		require.NoError(t, validateDarwinBookmarkState(state))
	})

	t.Run("empty intents do not mutate state", func(t *testing.T) {
		state := newDarwinBookmarkState()
		assert.False(t, newDarwinScanIncidents().reconcile(state, make(map[string]bool), now))
		require.NoError(t, validateDarwinBookmarkState(state))
	})
}

func TestDarwinCollectorNestedJSONArraysAreDeepCopied(t *testing.T) {
	event := validPersistedDarwinEvent("nested-array-copy")
	event.Custom["nested"] = []interface{}{
		[]interface{}{
			map[string]interface{}{"value": "original"},
		},
	}

	collector := newTestCollector(t, realTempDir(t), &fakeDarwinBookmarkStore{})
	collector.state.Pending[event.ID] = event
	pending := collector.Pending()
	require.Len(t, pending, 1)
	nestedArrayMap(t, pending[0])["value"] = "mutated-through-pending"
	assert.Equal(t, "original", nestedArrayMap(t, collector.state.Pending[event.ID])["value"])

	store := &fakeDarwinBookmarkStore{saveErr: errors.New("disk full")}
	store.onSave = func(saved *darwinBookmarkState) {
		nestedArrayMap(t, saved.Pending[event.ID])["value"] = "mutated-by-store"
	}
	stagedCollector := newTestCollector(t, realTempDir(t), store)
	candidate := newDarwinBookmarkState()
	candidate.Pending[event.ID] = event
	stagedCollector.persistCandidateLocked(candidate, true, nil, nil, stagedCollector.generation)
	require.NotNil(t, stagedCollector.stagedScan)
	assert.Equal(t, "original", nestedArrayMap(t, stagedCollector.stagedScan.candidate.Pending[event.ID])["value"])

	nestedArrayMap(t, candidate.Pending[event.ID])["value"] = "mutated-after-stage"
	assert.Equal(t, "original", nestedArrayMap(t, stagedCollector.stagedScan.candidate.Pending[event.ID])["value"])
}

func nestedArrayMap(t *testing.T, event Event) map[string]interface{} {
	t.Helper()
	outer, ok := event.Custom["nested"].([]interface{})
	require.True(t, ok)
	require.Len(t, outer, 1)
	inner, ok := outer[0].([]interface{})
	require.True(t, ok)
	require.Len(t, inner, 1)
	value, ok := inner[0].(map[string]interface{})
	require.True(t, ok)
	return value
}

// Load returns a deep copy of the fake persisted bookmark state.
func (s *fakeDarwinBookmarkStore) Load() (*darwinBookmarkState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadCalls++
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	if s.state == nil {
		return newDarwinBookmarkState(), nil
	}
	return cloneDarwinBookmarkState(s.state), nil
}

// Save records a deep copy of collector state or the configured failure.
func (s *fakeDarwinBookmarkStore) Save(state *darwinBookmarkState) error {
	s.mu.Lock()
	s.saveCalls++
	onSave := s.onSave
	s.mu.Unlock()

	if onSave != nil {
		onSave(state)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.saveErr != nil {
		return s.saveErr
	}
	s.state = cloneDarwinBookmarkState(state)
	return nil
}

func (s *fakeDarwinBookmarkStore) setOnSave(onSave func(*darwinBookmarkState)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onSave = onSave
}

func (s *fakeDarwinBookmarkStore) saveCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveCalls
}

// TestDarwinCollectorFirstBaselineSuppressesHistory verifies initial discovery does not replay old crashes.
func TestDarwinCollectorFirstBaselineSuppressesHistory(t *testing.T) {
	reportDir := realTempDir(t)
	writeIPSFile(t, reportDir, "OldApp_1.ips", "OldApp", "/Applications/OldApp", "INCIDENT-OLD")
	collector := newTestCollector(t, reportDir, &fakeDarwinBookmarkStore{})

	collector.scanOnce(context.Background())
	assert.Empty(t, collector.Pending())

	writeIPSFile(t, reportDir, "NewApp_1.ips", "NewApp", "/Applications/NewApp", "INCIDENT-NEW")
	collector.scanOnce(context.Background())

	pending := collector.Pending()
	require.Len(t, pending, 1)
	assert.Equal(t, "Application crash: NewApp", pending[0].Title)
	assert.Equal(t, eventID("incident:INCIDENT-NEW"), pending[0].ID)
}

func TestDarwinCollectorStableMalformedBaselineProvenance(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "empty report"},
		{name: "metadata without newline", content: `{"bug_type":"309","incident_id":"INCIDENT-PARTIAL"}`},
		{
			name:    "truncated body",
			content: `{"bug_type":"309","incident_id":"INCIDENT-PARTIAL"}` + "\n" + `{"bug_type":"309"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reportDir := realTempDir(t)
			path := filepath.Join(reportDir, "PartialApp_1.ips")
			require.NoError(t, os.WriteFile(path, []byte(test.content), 0o600))
			store := &fakeDarwinBookmarkStore{}
			collector := newTestCollector(t, reportDir, store)

			collector.scanOnce(context.Background())
			dirState := collector.state.Directories[hashString(filepath.Clean(reportDir))]
			require.NotNil(t, dirState)
			assert.True(t, dirState.Initialized)
			assert.NotContains(t, collector.retryDirs, directoryRuntimeKey(reportDir))
			require.True(t, dirState.Files["PartialApp_1.ips"].BaselineOnly)
			assert.Empty(t, collector.Pending())

			restarted := newTestCollector(t, reportDir, store)
			writeIPSFile(t, reportDir, filepath.Base(path), "PartialApp", "/Applications/PartialApp", "INCIDENT-PARTIAL")
			restarted.scanOnce(context.Background())
			dirState = restarted.state.Directories[hashString(filepath.Clean(reportDir))]
			assert.False(t, dirState.Files["PartialApp_1.ips"].BaselineOnly)
			assert.Empty(t, restarted.Pending(), "completed pre-existing malformed report must remain baseline history")
			assert.Contains(t, restarted.state.Acknowledged, eventID("incident:INCIDENT-PARTIAL"))

			writeIPSFile(t, reportDir, "AfterBaseline.ips", "AfterBaseline", "/Applications/AfterBaseline", "INCIDENT-AFTER")
			restarted.scanOnce(context.Background())
			require.Len(t, restarted.Pending(), 1)
			assert.Equal(t, eventID("incident:INCIDENT-AFTER"), restarted.Pending()[0].ID)
		})
	}
}

func TestDarwinCollectorPermanentMalformedReportDoesNotBlockBaseline(t *testing.T) {
	reportDir := realTempDir(t)
	content := `{"bug_type":"309","incident_id":"INCIDENT-MALFORMED"}` + "\n" +
		`{"bug_type":"309","nested":]}`
	require.NoError(t, os.WriteFile(filepath.Join(reportDir, "Malformed.ips"), []byte(content), 0o600))
	collector := newTestCollector(t, reportDir, &fakeDarwinBookmarkStore{})

	collector.scanOnce(context.Background())

	dirState := collector.state.Directories[hashString(filepath.Clean(reportDir))]
	require.NotNil(t, dirState)
	assert.True(t, dirState.Initialized)
	assert.Contains(t, dirState.Files, "Malformed.ips")
	assert.NotContains(t, collector.retryDirs, directoryRuntimeKey(reportDir))
	assert.Empty(t, collector.Pending())
}

func TestDarwinCollectorPostBaselineMalformedReportRemainsDeliveryEligible(t *testing.T) {
	reportDir := realTempDir(t)
	path := filepath.Join(reportDir, "Later.ips")
	collector := newTestCollector(t, reportDir, &fakeDarwinBookmarkStore{})
	collector.scanOnce(context.Background())

	require.NoError(t, os.WriteFile(path, nil, 0o600))
	collector.scanOnce(context.Background())
	dirState := collector.state.Directories[hashString(filepath.Clean(reportDir))]
	require.NotNil(t, dirState)
	assert.False(t, dirState.Files["Later.ips"].BaselineOnly)

	writeIPSFile(t, reportDir, filepath.Base(path), "Later", "/Applications/Later", "INCIDENT-LATER")
	collector.scanOnce(context.Background())

	require.Len(t, collector.Pending(), 1)
	assert.Equal(t, eventID("incident:INCIDENT-LATER"), collector.Pending()[0].ID)
	assert.NotContains(t, collector.state.Acknowledged, eventID("incident:INCIDENT-LATER"))
}

func TestDarwinCollectorDeliverableCopyOverridesBaselineOnlyCopy(t *testing.T) {
	baselineDir := realTempDir(t)
	deliveryDir := realTempDir(t)
	baselinePath := filepath.Join(baselineDir, "BaselineCopy.ips")
	require.NoError(t, os.WriteFile(baselinePath, nil, 0o600))
	collector, err := newDarwinCollectorWithDeps(
		func() []reportDirectory {
			return []reportDirectory{{path: baselineDir, scope: "system"}, {path: deliveryDir, scope: "user"}}
		},
		time.Hour,
		time.Hour,
		&fakeDarwinBookmarkStore{},
		nil,
	)
	require.NoError(t, err)
	collector.scanOnce(context.Background())

	writeIPSFile(t, baselineDir, filepath.Base(baselinePath), "App", "/Applications/App", "INCIDENT-COPY")
	writeIPSFile(t, deliveryDir, "DeliveryCopy.ips", "App", "/Applications/App", "INCIDENT-COPY")
	collector.scanOnce(context.Background())

	id := eventID("incident:INCIDENT-COPY")
	assert.Contains(t, collector.state.Pending, id)
	assert.NotContains(t, collector.state.Acknowledged, id)
	require.NoError(t, validateDarwinBookmarkState(collector.state))
}

func TestIsTransientDiagnosticReportOpenError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		transient bool
	}{
		{name: "nil", err: nil},
		{name: "ENOENT", err: unix.ENOENT, transient: true},
		{name: "wrapped EINTR", err: fmt.Errorf("open crash report: %w", unix.EINTR), transient: true},
		{name: "EIO", err: unix.EIO, transient: true},
		{name: "EAGAIN", err: unix.EAGAIN, transient: true},
		{name: "EBUSY", err: unix.EBUSY, transient: true},
		{name: "ESTALE", err: unix.ESTALE, transient: true},
		{name: "ETIMEDOUT", err: unix.ETIMEDOUT, transient: true},
		{name: "EMFILE", err: unix.EMFILE, transient: true},
		{name: "ENFILE", err: unix.ENFILE, transient: true},
		{name: "ENOMEM", err: unix.ENOMEM, transient: true},
		{name: "EACCES", err: unix.EACCES},
		{name: "EPERM", err: unix.EPERM},
		{name: "ELOOP", err: unix.ELOOP},
		{name: "policy", err: &diagnosticReportPolicyError{message: "rejected by policy"}},
		{name: "unknown", err: errors.New("unknown open failure")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.transient, isTransientDiagnosticReportOpenError(test.err))
		})
	}
}

func TestDiagnosticReportOpenErrorsControlBaselineCompletion(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		initialized bool
		shouldRetry bool
	}{
		{name: "transient open error defers baseline", err: fmt.Errorf("open crash report: %w", unix.EIO), shouldRetry: true},
		{name: "permission error completes baseline", err: fmt.Errorf("open crash report: %w", unix.EACCES), initialized: true},
		{name: "policy error completes baseline", err: &diagnosticReportPolicyError{message: "not a regular file"}, initialized: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dirState := &directoryBookmarkState{Saturated: true}
			result := directoryScanResult{}

			recordDiagnosticReportOpenError(&result, test.err)
			completeDiagnosticReportBaseline(dirState, &result, true)

			assert.Equal(t, test.shouldRetry, result.ShouldRetry)
			assert.Equal(t, test.initialized, dirState.Initialized)
			assert.Equal(t, !test.initialized, dirState.Saturated)
			assert.Equal(t, test.initialized, result.StateChanged)
		})
	}
}

// TestDarwinCollectorPendingIsRepeatableAndDeepCopied verifies snapshots neither consume nor expose internal events.
func TestDarwinCollectorPendingIsRepeatableAndDeepCopied(t *testing.T) {
	reportDir := realTempDir(t)
	collector := newTestCollector(t, reportDir, &fakeDarwinBookmarkStore{})
	collector.scanOnce(context.Background())
	writeIPSFile(t, reportDir, "App.ips", "App", "/Applications/App", "INCIDENT-PENDING")
	collector.scanOnce(context.Background())

	first := collector.Pending()
	second := collector.Pending()
	require.Equal(t, first, second)
	require.Len(t, first, 1)
	first[0].Title = "mutated"
	first[0].Custom["mutated"] = true

	third := collector.Pending()
	assert.Equal(t, "Application crash: App", third[0].Title)
	assert.NotContains(t, third[0].Custom, "mutated")
}

// TestDarwinCollectorAckIsIdempotent verifies repeated acknowledgements remain safe.
func TestDarwinCollectorAckIsIdempotent(t *testing.T) {
	store := &fakeDarwinBookmarkStore{}
	reportDir := realTempDir(t)
	collector := newTestCollector(t, reportDir, store)
	collector.scanOnce(context.Background())
	writeIPSFile(t, reportDir, "App.ips", "App", "/Applications/App", "INCIDENT-ACK")
	collector.scanOnce(context.Background())
	id := collector.Pending()[0].ID

	require.NoError(t, collector.Ack([]string{id, id, "unknown"}))
	assert.Empty(t, collector.Pending())
	savesAfterAck := store.saveCalls
	require.NoError(t, collector.Ack([]string{id, "unknown"}))
	assert.Equal(t, savesAfterAck, store.saveCalls)
	assert.Contains(t, store.state.Acknowledged, id)

	collector.scanOnce(context.Background())
	assert.Empty(t, collector.Pending())
}

func TestDarwinCollectorPendingAndAckRemainResponsiveDuringScan(t *testing.T) {
	store := &fakeDarwinBookmarkStore{}
	reportDir := realTempDir(t)
	collector := newTestCollector(t, reportDir, store)
	event := validPersistedDarwinEvent("concurrent-scan")
	collector.state.Pending[event.ID] = event

	scanStarted := make(chan struct{})
	releaseScan := make(chan struct{})
	collector.scanDirectory = func(_ context.Context, dir reportDirectory, candidate *darwinBookmarkState) (directoryScanResult, error) {
		close(scanStarted)
		<-releaseScan
		candidate.Directories[hashString(filepath.Clean(dir.path))] = validPersistedDirectoryState()
		return directoryScanResult{StateChanged: true}, nil
	}

	scanDone := make(chan struct{})
	go func() {
		collector.scanOnce(context.Background())
		close(scanDone)
	}()
	<-scanStarted

	pendingResult := make(chan []Event)
	go func() {
		pendingResult <- collector.Pending()
	}()
	require.Len(t, <-pendingResult, 1)

	ackResult := make(chan error)
	go func() {
		ackResult <- collector.Ack([]string{event.ID})
	}()
	require.NoError(t, <-ackResult)

	close(releaseScan)
	<-scanDone

	assert.Empty(t, collector.Pending())
	assert.Contains(t, collector.state.Acknowledged, event.ID)
	assert.Contains(t, collector.retryDirs, directoryRuntimeKey(reportDir), "stale scan work must be retried")
}

func TestDarwinCollectorBlockedScanSaveDoesNotBlockPendingOrAck(t *testing.T) {
	store := &fakeDarwinBookmarkStore{saveErr: errors.New("disk full")}
	reportDir := realTempDir(t)
	collector := newTestCollector(t, reportDir, store)
	event := validPersistedDarwinEvent("blocked-scan-save")
	collector.state.Pending[event.ID] = event
	collector.scanDirectory = func(_ context.Context, dir reportDirectory, candidate *darwinBookmarkState) (directoryScanResult, error) {
		candidate.Directories[hashString(filepath.Clean(dir.path))] = validPersistedDirectoryState()
		return directoryScanResult{StateChanged: true}, nil
	}

	saveStarted := make(chan struct{})
	releaseSave := make(chan struct{})
	store.setOnSave(func(*darwinBookmarkState) {
		close(saveStarted)
		<-releaseSave
	})

	scanDone := make(chan struct{})
	go func() {
		collector.scanOnce(context.Background())
		close(scanDone)
	}()
	<-saveStarted

	pendingResult := make(chan []Event)
	go func() {
		pendingResult <- collector.Pending()
	}()
	require.Len(t, <-pendingResult, 1)

	ackResult := make(chan error)
	go func() {
		ackResult <- collector.Ack([]string{event.ID})
	}()
	require.Error(t, <-ackResult)

	close(releaseSave)
	<-scanDone
	require.NotNil(t, collector.stagedScan)
	require.Error(t, collector.Ack([]string{event.ID}), "staged state must replace the reservation without an ACK window")
}

func TestDarwinCollectorBlockedAckSaveSerializesOtherCommits(t *testing.T) {
	store := &fakeDarwinBookmarkStore{}
	reportDir := realTempDir(t)
	collector := newTestCollector(t, reportDir, store)
	event := validPersistedDarwinEvent("blocked-ack-save")
	collector.state.Pending[event.ID] = event
	collector.scanDirectory = func(_ context.Context, dir reportDirectory, candidate *darwinBookmarkState) (directoryScanResult, error) {
		candidate.Directories[hashString(filepath.Clean(dir.path))] = validPersistedDirectoryState()
		return directoryScanResult{StateChanged: true}, nil
	}

	saveStarted := make(chan struct{})
	releaseSave := make(chan struct{})
	store.setOnSave(func(*darwinBookmarkState) {
		close(saveStarted)
		<-releaseSave
	})

	ackDone := make(chan error)
	go func() {
		ackDone <- collector.Ack([]string{event.ID})
	}()
	<-saveStarted

	pendingResult := make(chan []Event)
	go func() {
		pendingResult <- collector.Pending()
	}()
	require.Len(t, <-pendingResult, 1, "unpublished ACK state must not be visible")

	secondAck := make(chan error)
	go func() {
		secondAck <- collector.Ack([]string{event.ID})
	}()
	require.Error(t, <-secondAck)

	scanDone := make(chan struct{})
	go func() {
		collector.scanOnce(context.Background())
		close(scanDone)
	}()
	<-scanDone
	assert.Equal(t, 1, store.saveCallCount(), "scan must not enter storage while ACK owns the reservation")

	close(releaseSave)
	require.NoError(t, <-ackDone)
	assert.Empty(t, collector.Pending())
	assert.Contains(t, collector.retryDirs, directoryRuntimeKey(reportDir))
}

// TestDarwinCollectorAckSaveFailureLeavesPending verifies persistence failure preserves retryable events.
func TestDarwinCollectorAckSaveFailureLeavesPending(t *testing.T) {
	store := &fakeDarwinBookmarkStore{}
	reportDir := realTempDir(t)
	collector := newTestCollector(t, reportDir, store)
	collector.scanOnce(context.Background())
	writeIPSFile(t, reportDir, "App.ips", "App", "/Applications/App", "INCIDENT-ACK-FAIL")
	collector.scanOnce(context.Background())
	id := collector.Pending()[0].ID

	store.saveErr = errors.New("disk full")
	require.Error(t, collector.Ack([]string{id}))
	require.Len(t, collector.Pending(), 1)
	assert.Contains(t, store.state.Pending, id)
	assert.NotContains(t, collector.state.Acknowledged, id)

	store.saveErr = nil
	require.NoError(t, collector.Ack([]string{id}))
	assert.Empty(t, collector.Pending())
}

// TestDarwinCollectorAckKeepsAcknowledgementsBounded verifies ACK cannot
// persist state that secure loading would reject.
func TestDarwinCollectorAckKeepsAcknowledgementsBounded(t *testing.T) {
	store := &fakeDarwinBookmarkStore{}
	reportDir := realTempDir(t)
	collector := newTestCollector(t, reportDir, store)
	collector.scanOnce(context.Background())
	writeIPSFile(t, reportDir, "App.ips", "App", "/Applications/App", "INCIDENT-ACK-BOUND")
	collector.scanOnce(context.Background())
	id := collector.Pending()[0].ID
	for index := 0; index < collector.maxAcknowledged; index++ {
		collector.state.Acknowledged[eventID(fmt.Sprintf("old-%d", index))] = 1
	}

	require.NoError(t, collector.Ack([]string{id}))

	assert.Len(t, collector.state.Acknowledged, collector.maxAcknowledged)
	assert.Contains(t, collector.state.Acknowledged, id)
	assert.NotContains(t, collector.state.Pending, id)
}

// TestDarwinCollectorPersistsPendingBeforePublishing verifies a scan candidate
// is durable before it becomes visible through Pending or the live baseline.
func TestDarwinCollectorPersistsPendingBeforePublishing(t *testing.T) {
	store := &fakeDarwinBookmarkStore{}
	reportDir := realTempDir(t)
	collector := newTestCollector(t, reportDir, store)
	collector.scanOnce(context.Background())
	writeIPSFile(t, reportDir, "App.ips", "App", "/Applications/App", "INCIDENT-SAVE-FIRST")

	id := eventID("incident:INCIDENT-SAVE-FIRST")
	store.onSave = func(candidate *darwinBookmarkState) {
		assert.Contains(t, candidate.Pending, id)
		assert.NotContains(t, collector.state.Pending, id)
		liveDirectory := collector.state.Directories[hashString(filepath.Clean(reportDir))]
		assert.NotContains(t, liveDirectory.Files, "App.ips")
	}
	collector.scanOnce(context.Background())

	assert.Contains(t, collector.state.Pending, id)
}

// TestDarwinCollectorPersistsBaselineBeforePublishing verifies initial file
// suppression is durable before the initialized directory becomes live.
func TestDarwinCollectorPersistsBaselineBeforePublishing(t *testing.T) {
	store := &fakeDarwinBookmarkStore{}
	reportDir := realTempDir(t)
	writeIPSFile(t, reportDir, "Old.ips", "Old", "/Applications/Old", "INCIDENT-BASELINE")
	collector := newTestCollector(t, reportDir, store)
	dirKey := hashString(filepath.Clean(reportDir))

	store.onSave = func(candidate *darwinBookmarkState) {
		require.NotNil(t, candidate.Directories[dirKey])
		assert.True(t, candidate.Directories[dirKey].Initialized)
		assert.Contains(t, candidate.Directories[dirKey].Files, "Old.ips")
		assert.NotContains(t, collector.state.Directories, dirKey)
	}
	collector.scanOnce(context.Background())

	assert.True(t, collector.state.Directories[dirKey].Initialized)
}

// TestDarwinCollectorPendingSaveFailurePreservesLiveState verifies failed
// persistence publishes neither a pending event nor its file bookkeeping.
func TestDarwinCollectorPendingSaveFailurePreservesLiveState(t *testing.T) {
	store := &fakeDarwinBookmarkStore{}
	reportDir := realTempDir(t)
	collector := newTestCollector(t, reportDir, store)
	collector.scanOnce(context.Background())
	liveBefore := cloneDarwinBookmarkState(collector.state)
	writeIPSFile(t, reportDir, "App.ips", "App", "/Applications/App", "INCIDENT-FAIL")

	store.saveErr = errors.New("disk full")
	collector.scanOnce(context.Background())

	assert.Equal(t, liveBefore, collector.state)
	assert.Empty(t, collector.Pending())
	assert.Contains(t, collector.retryDirs, directoryRuntimeKey(reportDir))
}

// TestDarwinCollectorFailedFirstBaselineIsNotVisible verifies an unpersisted
// initial baseline cannot suppress reports in live memory.
func TestDarwinCollectorFailedFirstBaselineIsNotVisible(t *testing.T) {
	store := &fakeDarwinBookmarkStore{saveErr: errors.New("disk full")}
	reportDir := realTempDir(t)
	writeIPSFile(t, reportDir, "Old.ips", "Old", "/Applications/Old", "INCIDENT-OLD")
	collector := newTestCollector(t, reportDir, store)

	collector.scanOnce(context.Background())

	assert.Empty(t, collector.state.Directories)
	assert.Empty(t, collector.state.Acknowledged)
	assert.Empty(t, collector.Pending())
	assert.Contains(t, collector.retryDirs, directoryRuntimeKey(reportDir))
}

func TestDarwinCollectorStagesDirectorySyncAmbiguity(t *testing.T) {
	base := realTempDir(t)
	reportDir := realTempDir(t)
	writeIPSFile(t, reportDir, "Old.ips", "Old", "/Applications/Old", "INCIDENT-DIRSYNC")
	store := newTestDarwinBookmarkStore(base, uint32(os.Getuid()))
	_, err := store.Load()
	require.NoError(t, err)
	syncCalls := 0
	store.fsync = func(_ int) error {
		syncCalls++
		if syncCalls == 2 {
			return errors.New("injected directory sync failure")
		}
		return nil
	}
	collector := newTestCollector(t, reportDir, store)

	collector.scanOnce(context.Background())

	require.NotNil(t, collector.stagedScan)
	assert.Empty(t, collector.state.Directories)
	assert.Contains(t, collector.retryDirs, directoryRuntimeKey(reportDir))
	loaded, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, collector.stagedScan.candidate, loaded, "rename may have committed despite Save returning an error")

	store.fsync = func(int) error { return nil }
	collector.retryMaintenance(context.Background())
	assert.Nil(t, collector.stagedScan)
	assert.Equal(t, loaded, collector.state)
	assert.Empty(t, collector.Pending())
}

func TestDarwinCollectorStagedScanIsImmutable(t *testing.T) {
	store := &fakeDarwinBookmarkStore{saveErr: errors.New("disk full")}
	reportDir := realTempDir(t)
	collector := newTestCollector(t, reportDir, store)
	dir := reportDirectory{path: reportDir, scope: "system"}
	collector.knownDirs[directoryRuntimeKey(reportDir)] = dir
	candidate := newDarwinBookmarkState()
	event := validPersistedDarwinEvent("staged-immutable")
	candidate.Pending[event.ID] = event
	dirs := []reportDirectory{dir}
	retryResults := map[string]bool{directoryRuntimeKey(reportDir): false}
	store.onSave = func(saved *darwinBookmarkState) {
		saved.Pending[event.ID].Custom["mutated-by-store"] = true
	}

	collector.persistCandidateLocked(candidate, true, dirs, retryResults, collector.generation)
	require.NotNil(t, collector.stagedScan)
	stagedBefore := newStagedDarwinScan(collector.stagedScan.candidate)
	runtimeBefore := newStagedDarwinScanRuntime(
		collector.stagedRuntime.dirs,
		collector.stagedRuntime.retryResults,
	)

	candidate.Pending[event.ID].Custom["mutated-after-stage"] = true
	dirs[0].path = "/mutated"
	retryResults[directoryRuntimeKey(reportDir)] = true
	_, recovered := collector.persistStagedScanLocked()

	assert.False(t, recovered)
	assert.Equal(t, stagedBefore, collector.stagedScan)
	assert.Equal(t, runtimeBefore, collector.stagedRuntime)
	assert.NotContains(t, collector.stagedScan.candidate.Pending[event.ID].Custom, "mutated-by-store")
	assert.NotContains(t, collector.stagedScan.candidate.Pending[event.ID].Custom, "mutated-after-stage")
}

func TestDarwinCollectorRecoversStageBeforeRescanningNewCrash(t *testing.T) {
	store := &fakeDarwinBookmarkStore{saveErr: errors.New("disk full")}
	reportDir := realTempDir(t)
	writeIPSFile(t, reportDir, "Old.ips", "Old", "/Applications/Old", "INCIDENT-STAGED-OLD")
	collector := newTestCollector(t, reportDir, store)

	collector.scanOnce(context.Background())
	require.NotNil(t, collector.stagedScan)
	writeIPSFile(t, reportDir, "New.ips", "New", "/Applications/New", "INCIDENT-STAGED-NEW")
	store.saveErr = nil

	collector.retryMaintenance(context.Background())

	assert.Nil(t, collector.stagedScan)
	pending := collector.Pending()
	require.Len(t, pending, 1)
	assert.Equal(t, eventID("incident:INCIDENT-STAGED-NEW"), pending[0].ID)
	assert.NotContains(t, collector.state.Pending, eventID("incident:INCIDENT-STAGED-OLD"))
}

func TestDarwinCollectorAckBlockedWhileScanStaged(t *testing.T) {
	store := &fakeDarwinBookmarkStore{}
	reportDir := realTempDir(t)
	collector := newTestCollector(t, reportDir, store)
	collector.scanOnce(context.Background())
	writeIPSFile(t, reportDir, "First.ips", "First", "/Applications/First", "INCIDENT-STAGED-ACK-FIRST")
	collector.scanOnce(context.Background())
	firstID := eventID("incident:INCIDENT-STAGED-ACK-FIRST")
	require.Contains(t, collector.state.Pending, firstID)

	writeIPSFile(t, reportDir, "Second.ips", "Second", "/Applications/Second", "INCIDENT-STAGED-ACK-SECOND")
	store.saveErr = errors.New("disk full")
	collector.scanOnce(context.Background())
	require.NotNil(t, collector.stagedScan)
	stateBeforeAck := cloneDarwinBookmarkState(collector.state)
	stageBeforeAck := newStagedDarwinScan(collector.stagedScan.candidate)
	runtimeBeforeAck := newStagedDarwinScanRuntime(
		collector.stagedRuntime.dirs,
		collector.stagedRuntime.retryResults,
	)
	savesBeforeAck := store.saveCalls

	require.Error(t, collector.Ack([]string{firstID}))
	require.NoError(t, collector.Ack([]string{"unknown"}))
	assert.Equal(t, savesBeforeAck, store.saveCalls)
	assert.Equal(t, stateBeforeAck, collector.state)
	assert.Equal(t, stageBeforeAck, collector.stagedScan)
	assert.Equal(t, runtimeBeforeAck, collector.stagedRuntime)

	store.saveErr = nil
	collector.retryMaintenance(context.Background())
	require.NoError(t, collector.Ack([]string{firstID}))
	assert.NotContains(t, collector.state.Pending, firstID)
	assert.Contains(t, collector.state.Acknowledged, firstID)
}

func TestDarwinCollectorStageRecoveryDoesNotRestoreRemovedDirectory(t *testing.T) {
	firstDir := realTempDir(t)
	removedDir := realTempDir(t)
	store := &fakeDarwinBookmarkStore{saveErr: errors.New("disk full")}
	discovered := []reportDirectory{
		{path: firstDir, scope: "system"},
		{path: removedDir, scope: "user"},
	}
	collector, err := newDarwinCollectorWithDeps(
		func() []reportDirectory { return append([]reportDirectory(nil), discovered...) },
		time.Hour,
		time.Hour,
		store,
		nil,
	)
	require.NoError(t, err)

	collector.scanOnce(context.Background())
	require.NotNil(t, collector.stagedScan)
	assert.Contains(t, collector.retryDirs, directoryRuntimeKey(removedDir))

	discovered = discovered[:1]
	store.saveErr = nil
	collector.scanOnce(context.Background())

	assert.Nil(t, collector.stagedScan)
	assert.NotContains(t, collector.knownDirs, directoryRuntimeKey(removedDir))
	assert.NotContains(t, collector.retryDirs, directoryRuntimeKey(removedDir))
}

// TestDarwinCollectorPendingBatchIsOrderedAndBounded verifies polling has a
// deterministic fixed upper bound independent of map iteration.
func TestDarwinCollectorPendingBatchIsOrderedAndBounded(t *testing.T) {
	collector := newTestCollector(t, realTempDir(t), &fakeDarwinBookmarkStore{})
	for index := maxDarwinPendingBatch + 5; index >= 0; index-- {
		id := fmt.Sprintf("event-%03d", index)
		collector.state.Pending[id] = Event{ID: id}
	}

	pending := collector.Pending()
	require.Len(t, pending, maxDarwinPendingBatch)
	for index, event := range pending {
		assert.Equal(t, fmt.Sprintf("event-%03d", index), event.ID)
	}
}

// TestDarwinCollectorQueueFullRetriesAfterAck verifies capacity pressure never
// accounts for or drops a new report and collection resumes after ACK.
func TestDarwinCollectorQueueFullRetriesAfterAck(t *testing.T) {
	store := &fakeDarwinBookmarkStore{}
	reportDir := realTempDir(t)
	collector := newTestCollector(t, reportDir, store)
	collector.scanOnce(context.Background())
	for index := 0; index < maxDarwinPendingEvents; index++ {
		id := fmt.Sprintf("queued-%03d", index)
		collector.state.Pending[id] = Event{ID: id}
	}
	writeIPSFile(t, reportDir, "Later.ips", "Later", "/Applications/Later", "INCIDENT-LATER")

	collector.scanOnce(context.Background())

	laterID := eventID("incident:INCIDENT-LATER")
	assert.NotContains(t, collector.state.Pending, laterID)
	assert.NotContains(t, collector.state.Directories[hashString(filepath.Clean(reportDir))].Files, "Later.ips")
	assert.Contains(t, collector.retryDirs, directoryRuntimeKey(reportDir))

	require.NoError(t, collector.Ack([]string{"queued-000"}))
	collector.retryMaintenance(context.Background())

	assert.Contains(t, collector.state.Pending, laterID)
	assert.Contains(t, collector.state.Directories[hashString(filepath.Clean(reportDir))].Files, "Later.ips")
	assert.NotContains(t, collector.retryDirs, directoryRuntimeKey(reportDir))
}

// TestDarwinCollectorSaturatedDirectoryDoesNotBlockOtherDirectories verifies
// hostile report counts are isolated and the persisted candidate remains bounded.
func TestDarwinCollectorSaturatedDirectoryDoesNotBlockOtherDirectories(t *testing.T) {
	hostileDir := realTempDir(t)
	healthyDir := realTempDir(t)
	store := &fakeDarwinBookmarkStore{}
	collector, err := newDarwinCollectorWithDeps(
		func() []reportDirectory {
			return []reportDirectory{
				{path: hostileDir, scope: "user"},
				{path: healthyDir, scope: "system"},
			}
		},
		time.Hour,
		time.Hour,
		store,
		nil,
	)
	require.NoError(t, err)
	collector.scanOnce(context.Background())

	for index := 0; index <= maxDarwinFilesPerDirectory; index++ {
		name := fmt.Sprintf("%03d-%s.ips", index, strings.Repeat("x", maxReportBasenameSize-len("000-.ips")))
		require.NoError(t, os.WriteFile(filepath.Join(hostileDir, name), []byte(`{"bug_type":"288"}`+"\n"), 0o600))
	}
	writeIPSFile(t, healthyDir, "Healthy.ips", "Healthy", "/Applications/Healthy", "INCIDENT-HEALTHY")

	collector.scanOnce(context.Background())

	hostileState := collector.state.Directories[hashString(filepath.Clean(hostileDir))]
	require.NotNil(t, hostileState)
	assert.True(t, hostileState.Saturated)
	assert.LessOrEqual(t, len(hostileState.Files), maxDarwinFilesPerDirectory)
	pending := collector.Pending()
	require.Len(t, pending, 1)
	assert.Equal(t, eventID("incident:INCIDENT-HEALTHY"), pending[0].ID)
	data, err := json.Marshal(store.state)
	require.NoError(t, err)
	assert.Less(t, len(data), maxDarwinBookmarkSize)
}

// TestDarwinCollectorSaturationRecoveryRebaselines verifies reports omitted
// during saturation are suppressed before the directory resumes collection.
func TestDarwinCollectorSaturationRecoveryRebaselines(t *testing.T) {
	reportDir := realTempDir(t)
	store := &fakeDarwinBookmarkStore{}
	collector := newTestCollector(t, reportDir, store)
	paths := make([]string, 0, maxDarwinFilesPerDirectory+1)
	for index := 0; index <= maxDarwinFilesPerDirectory; index++ {
		name := fmt.Sprintf("Old-%03d.ips", index)
		writeIPSFile(t, reportDir, name, "Old", "/Applications/Old", fmt.Sprintf("INCIDENT-OLD-%03d", index))
		paths = append(paths, filepath.Join(reportDir, name))
	}

	collector.scanOnce(context.Background())
	dirState := collector.state.Directories[hashString(filepath.Clean(reportDir))]
	require.NotNil(t, dirState)
	assert.True(t, dirState.Saturated)
	assert.False(t, dirState.Initialized)
	assert.Empty(t, collector.Pending())

	for _, path := range paths[2:] {
		require.NoError(t, os.Remove(path))
	}
	collector.scanOnce(context.Background())
	dirState = collector.state.Directories[hashString(filepath.Clean(reportDir))]
	assert.False(t, dirState.Saturated)
	assert.True(t, dirState.Initialized)
	assert.Empty(t, collector.Pending(), "reports present during saturation must become baseline history")

	writeIPSFile(t, reportDir, "New.ips", "New", "/Applications/New", "INCIDENT-NEW-AFTER-SATURATION")
	collector.scanOnce(context.Background())
	require.Len(t, collector.Pending(), 1)
	assert.Equal(t, eventID("incident:INCIDENT-NEW-AFTER-SATURATION"), collector.Pending()[0].ID)
}

// TestDarwinCollectorPendingSurvivesRestart verifies undelivered events are restored from persisted state.
func TestDarwinCollectorPendingSurvivesRestart(t *testing.T) {
	store := &fakeDarwinBookmarkStore{}
	reportDir := realTempDir(t)
	first := newTestCollector(t, reportDir, store)
	first.scanOnce(context.Background())
	writeIPSFile(t, reportDir, "App.ips", "App", "/Applications/App", "INCIDENT-RESTART")
	first.scanOnce(context.Background())
	expected := first.Pending()
	require.Len(t, expected, 1)

	restarted := newTestCollector(t, reportDir, store)
	assert.Equal(t, expected, restarted.Pending(), "persisted pending events must be available before the first rescan")
	restarted.scanOnce(context.Background())
	assert.Equal(t, expected, restarted.Pending())
}

// TestDarwinCollectorReconstructsMissingPendingPayload verifies rescanning repairs incomplete pending state.
func TestDarwinCollectorReconstructsMissingPendingPayload(t *testing.T) {
	store := &fakeDarwinBookmarkStore{}
	reportDir := realTempDir(t)
	first := newTestCollector(t, reportDir, store)
	first.scanOnce(context.Background())
	writeIPSFile(t, reportDir, "App.ips", "App", "/Applications/App", "INCIDENT-RECONSTRUCT")
	first.scanOnce(context.Background())
	id := first.Pending()[0].ID

	// Simulate a bookmark written by an intermediate implementation that
	// retained the file event ID but not the pending wire payload.
	delete(store.state.Pending, id)
	restarted := newTestCollector(t, reportDir, store)
	assert.Empty(t, restarted.Pending())
	restarted.scanOnce(context.Background())
	require.Len(t, restarted.Pending(), 1)
	assert.Equal(t, id, restarted.Pending()[0].ID)
}

// TestDarwinCollectorReportsCrashCreatedWhileStopped verifies reconciliation catches downtime events.
func TestDarwinCollectorReportsCrashCreatedWhileStopped(t *testing.T) {
	store := &fakeDarwinBookmarkStore{}
	reportDir := realTempDir(t)
	first := newTestCollector(t, reportDir, store)
	first.scanOnce(context.Background())

	writeIPSFile(t, reportDir, "Downtime.ips", "Downtime", "/Applications/Downtime", "INCIDENT-DOWNTIME")
	restarted := newTestCollector(t, reportDir, store)
	restarted.scanOnce(context.Background())
	require.Len(t, restarted.Pending(), 1)
	assert.Equal(t, eventID("incident:INCIDENT-DOWNTIME"), restarted.Pending()[0].ID)
}

// TestDarwinCollectorDeduplicatesIncidentAcrossDirectories verifies one incident emits only one event.
func TestDarwinCollectorDeduplicatesIncidentAcrossDirectories(t *testing.T) {
	systemDir := realTempDir(t)
	userDir := realTempDir(t)
	store := &fakeDarwinBookmarkStore{}
	collector, err := newDarwinCollectorWithDeps(
		func() []reportDirectory {
			return []reportDirectory{{path: systemDir, scope: "system"}, {path: userDir, scope: "user"}}
		},
		time.Hour,
		time.Hour,
		store,
		nil,
	)
	require.NoError(t, err)
	collector.scanOnce(context.Background())

	writeIPSFile(t, systemDir, "SystemCopy.ips", "App", "/Applications/App", "INCIDENT-COPY")
	writeIPSFile(t, userDir, "UserCopy.ips", "App", "/Applications/App", "INCIDENT-COPY")
	collector.scanOnce(context.Background())
	require.Len(t, collector.Pending(), 1)
}

func TestDarwinCollectorDirectoryPermutationProducesSameIncidentState(t *testing.T) {
	initializedDir := realTempDir(t)
	baselineDir := realTempDir(t)
	fixedNow := time.Date(2026, time.July, 22, 12, 0, 0, 0, time.UTC)
	initialized := reportDirectory{path: initializedDir, scope: "system"}
	baseline := reportDirectory{path: baselineDir, scope: "user"}
	firstOrder := []reportDirectory{initialized}
	secondOrder := []reportDirectory{initialized}
	firstStore := &fakeDarwinBookmarkStore{}
	secondStore := &fakeDarwinBookmarkStore{}
	first, err := newDarwinCollectorWithDeps(
		func() []reportDirectory { return append([]reportDirectory(nil), firstOrder...) },
		time.Hour,
		time.Hour,
		firstStore,
		nil,
	)
	require.NoError(t, err)
	second, err := newDarwinCollectorWithDeps(
		func() []reportDirectory { return append([]reportDirectory(nil), secondOrder...) },
		time.Hour,
		time.Hour,
		secondStore,
		nil,
	)
	require.NoError(t, err)
	first.now = func() time.Time { return fixedNow }
	second.now = func() time.Time { return fixedNow }
	first.scanOnce(context.Background())
	second.scanOnce(context.Background())

	writeIPSFile(t, initializedDir, "InitializedCopy.ips", "Initialized", "/Applications/Initialized", "INCIDENT-PERMUTATION")
	writeIPSFile(t, baselineDir, "BaselineCopy.ips", "Baseline", "/Applications/Baseline", "INCIDENT-PERMUTATION")
	firstOrder = []reportDirectory{initialized, baseline}
	secondOrder = []reportDirectory{baseline, initialized}
	first.scanOnce(context.Background())
	second.scanOnce(context.Background())

	id := eventID("incident:INCIDENT-PERMUTATION")
	require.Equal(t, first.state, second.state)
	assert.Contains(t, first.state.Pending, id)
	assert.NotContains(t, first.state.Acknowledged, id)
	assert.Equal(t, first.retryDirs, second.retryDirs)
	require.NoError(t, validateDarwinBookmarkState(first.state))

	restarted, restartErr := newDarwinCollectorWithDeps(
		func() []reportDirectory { return []reportDirectory{baseline, initialized} },
		time.Hour,
		time.Hour,
		firstStore,
		nil,
	)
	require.NoError(t, restartErr)
	restarted.now = func() time.Time { return fixedNow }
	restarted.scanOnce(context.Background())
	assert.Contains(t, restarted.state.Pending, id)
	assert.NotContains(t, restarted.state.Acknowledged, id)
	require.NoError(t, validateDarwinBookmarkState(restarted.state))
}

func TestDarwinCollectorBaselineOnlyDuplicateRemainsAcknowledgedAcrossPermutations(t *testing.T) {
	firstDir := realTempDir(t)
	secondDir := realTempDir(t)
	writeIPSFile(t, firstDir, "First.ips", "First", "/Applications/First", "INCIDENT-BASELINE-PERMUTATION")
	writeIPSFile(t, secondDir, "Second.ips", "Second", "/Applications/Second", "INCIDENT-BASELINE-PERMUTATION")
	dirs := []reportDirectory{{path: firstDir, scope: "system"}, {path: secondDir, scope: "user"}}
	orders := [][]reportDirectory{dirs, {dirs[1], dirs[0]}}
	fixedNow := time.Date(2026, time.July, 22, 12, 0, 0, 0, time.UTC)
	states := make([]*darwinBookmarkState, 0, len(orders))

	for _, order := range orders {
		store := &fakeDarwinBookmarkStore{}
		collector, err := newDarwinCollectorWithDeps(
			func() []reportDirectory { return append([]reportDirectory(nil), order...) },
			time.Hour,
			time.Hour,
			store,
			nil,
		)
		require.NoError(t, err)
		collector.now = func() time.Time { return fixedNow }
		collector.scanOnce(context.Background())
		states = append(states, cloneDarwinBookmarkState(collector.state))
	}

	id := eventID("incident:INCIDENT-BASELINE-PERMUTATION")
	require.Equal(t, states[0], states[1])
	assert.Empty(t, states[0].Pending)
	assert.Contains(t, states[0].Acknowledged, id)
	require.NoError(t, validateDarwinBookmarkState(states[0]))
}

func TestReadBoundedDirectoryEntriesReturnsStableNameOrder(t *testing.T) {
	reportDir := realTempDir(t)
	for _, name := range []string{"Zulu.ips", "Alpha.ips", "Middle.ips"} {
		require.NoError(t, os.WriteFile(filepath.Join(reportDir, name), []byte("{}"), 0o600))
	}
	directory, err := os.Open(reportDir)
	require.NoError(t, err)
	defer directory.Close()

	entries, complete, err := readBoundedDirectoryEntries(directory)
	require.NoError(t, err)
	require.True(t, complete)
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	assert.Equal(t, []string{"Alpha.ips", "Middle.ips", "Zulu.ips"}, names)
}

func TestDarwinCollectorCanceledPartialScanDefersIncidentDisposition(t *testing.T) {
	dirs := []reportDirectory{{path: "/tmp/notableevents-b", scope: "user"}, {path: "/tmp/notableevents-a", scope: "system"}}
	store := &fakeDarwinBookmarkStore{}
	collector, err := newDarwinCollectorWithDeps(
		func() []reportDirectory { return append([]reportDirectory(nil), dirs...) },
		time.Hour,
		time.Hour,
		store,
		nil,
	)
	require.NoError(t, err)
	collector.knownDirs = map[string]reportDirectory{
		directoryRuntimeKey(dirs[0].path): dirs[0],
		directoryRuntimeKey(dirs[1].path): dirs[1],
	}
	event := validPersistedDarwinEvent("canceled-duplicate")
	secondStarted := make(chan struct{})
	collector.scanDirectory = func(ctx context.Context, dir reportDirectory, _ *darwinBookmarkState) (directoryScanResult, error) {
		if directoryRuntimeKey(dir.path) == directoryRuntimeKey(dirs[0].path) {
			close(secondStarted)
			<-ctx.Done()
			return directoryScanResult{}, context.Canceled
		}
		sourceKey := directoryRuntimeKey(dir.path) + "\x00report.ips"
		fromFirstSource := cloneEvent(event)
		fromFirstSource.Title = "smallest source"
		return directoryScanResult{Deliverables: map[string]darwinScanDeliverable{
			event.ID: {
				Event:     fromFirstSource,
				SourceKey: sourceKey,
				ContributingDirKeys: map[string]struct{}{
					directoryRuntimeKey(dir.path): {},
				},
			},
		}}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		collector.scanDirectoriesLocked(ctx, []reportDirectory{dirs[0], dirs[1]})
		close(done)
	}()
	<-secondStarted
	cancel()
	<-done

	assert.Empty(t, collector.state.Pending)
	assert.Empty(t, collector.state.Acknowledged)
	assert.Contains(t, collector.retryDirs, directoryRuntimeKey(dirs[0].path))
	assert.Contains(t, collector.retryDirs, directoryRuntimeKey(dirs[1].path))
	require.NoError(t, validateDarwinBookmarkState(collector.state))

	collector.scanDirectory = func(_ context.Context, dir reportDirectory, _ *darwinBookmarkState) (directoryScanResult, error) {
		fromSource := cloneEvent(event)
		fromSource.Title = directoryRuntimeKey(dir.path)
		sourceKey := directoryRuntimeKey(dir.path) + "\x00report.ips"
		return directoryScanResult{Deliverables: map[string]darwinScanDeliverable{
			event.ID: {
				Event:     fromSource,
				SourceKey: sourceKey,
				ContributingDirKeys: map[string]struct{}{
					directoryRuntimeKey(dir.path): {},
				},
			},
		}}, nil
	}
	collector.scanDirectoriesLocked(context.Background(), []reportDirectory{dirs[0], dirs[1]})

	require.Contains(t, collector.state.Pending, event.ID)
	assert.Equal(t, directoryRuntimeKey(dirs[1].path), collector.state.Pending[event.ID].Title)
	assert.NotContains(t, collector.state.Acknowledged, event.ID)
	assert.Empty(t, collector.retryDirs)
	require.NoError(t, validateDarwinBookmarkState(collector.state))
}

func TestDarwinCollectorCancellationPreservesUnconsumedBaselineProvenance(t *testing.T) {
	dirs := []reportDirectory{{path: "/tmp/notableevents-baseline-a", scope: "system"}, {path: "/tmp/notableevents-baseline-b", scope: "user"}}
	store := &fakeDarwinBookmarkStore{}
	collector, err := newDarwinCollectorWithDeps(
		func() []reportDirectory { return append([]reportDirectory(nil), dirs...) },
		time.Hour,
		time.Hour,
		store,
		nil,
	)
	require.NoError(t, err)
	collector.knownDirs = map[string]reportDirectory{
		directoryRuntimeKey(dirs[0].path): dirs[0],
		directoryRuntimeKey(dirs[1].path): dirs[1],
	}
	stateKey := hashString(filepath.Clean(dirs[0].path))
	collector.state.Directories[stateKey] = &directoryBookmarkState{
		Initialized: true,
		LastSeen:    time.Now().Unix(),
		Files: map[string]reportFileState{
			"Baseline.ips": {Fingerprint: "old", BaselineOnly: true},
		},
	}
	event := validPersistedDarwinEvent("baseline-cancellation")
	completedState := reportFileState{Fingerprint: "new", EventID: event.ID}
	secondStarted := make(chan struct{})
	collector.scanDirectory = func(ctx context.Context, dir reportDirectory, _ *darwinBookmarkState) (directoryScanResult, error) {
		if directoryRuntimeKey(dir.path) == directoryRuntimeKey(dirs[1].path) {
			close(secondStarted)
			<-ctx.Done()
			return directoryScanResult{}, context.Canceled
		}
		return directoryScanResult{
			DirectoryKey: stateKey,
			BaselineIncidentIDs: map[string]struct{}{
				event.ID: {},
			},
			BaselineCompletions: []darwinBaselineReportCompletion{{
				Name:  "Baseline.ips",
				State: completedState,
			}},
		}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		collector.scanDirectoriesLocked(ctx, dirs)
		close(done)
	}()
	<-secondStarted
	cancel()
	<-done

	fileState := collector.state.Directories[stateKey].Files["Baseline.ips"]
	assert.True(t, fileState.BaselineOnly)
	assert.Empty(t, fileState.EventID)
	assert.NotContains(t, collector.state.Acknowledged, event.ID)

	collector.scanDirectory = func(_ context.Context, dir reportDirectory, _ *darwinBookmarkState) (directoryScanResult, error) {
		if directoryRuntimeKey(dir.path) != directoryRuntimeKey(dirs[0].path) {
			return directoryScanResult{}, nil
		}
		return directoryScanResult{
			DirectoryKey: stateKey,
			BaselineIncidentIDs: map[string]struct{}{
				event.ID: {},
			},
			BaselineCompletions: []darwinBaselineReportCompletion{{
				Name:  "Baseline.ips",
				State: completedState,
			}},
		}, nil
	}
	collector.scanDirectoriesLocked(context.Background(), dirs)

	fileState = collector.state.Directories[stateKey].Files["Baseline.ips"]
	assert.False(t, fileState.BaselineOnly)
	assert.Equal(t, event.ID, fileState.EventID)
	assert.Contains(t, collector.state.Acknowledged, event.ID)
	require.NoError(t, validateDarwinBookmarkState(collector.state))
}

func TestDarwinCollectorOverflowPreservesUnconsumedBaselineProvenance(t *testing.T) {
	dirs := []reportDirectory{{path: "/tmp/notableevents-overflow-a", scope: "system"}, {path: "/tmp/notableevents-overflow-b", scope: "user"}}
	collector, err := newDarwinCollectorWithDeps(
		func() []reportDirectory { return append([]reportDirectory(nil), dirs...) },
		time.Hour,
		time.Hour,
		&fakeDarwinBookmarkStore{},
		nil,
	)
	require.NoError(t, err)
	collector.knownDirs = map[string]reportDirectory{
		directoryRuntimeKey(dirs[0].path): dirs[0],
		directoryRuntimeKey(dirs[1].path): dirs[1],
	}
	for index := 0; index < maxDarwinPendingEvents; index++ {
		pending := validPersistedDarwinEvent(fmt.Sprintf("overflow-existing-%03d", index))
		collector.state.Pending[pending.ID] = pending
	}
	stateKey := hashString(filepath.Clean(dirs[0].path))
	collector.state.Directories[stateKey] = &directoryBookmarkState{
		Initialized: true,
		LastSeen:    time.Now().Unix(),
		Files: map[string]reportFileState{
			"Baseline.ips": {Fingerprint: "old", BaselineOnly: true},
		},
	}
	event := validPersistedDarwinEvent("baseline-overflow")
	collector.scanDirectory = func(_ context.Context, dir reportDirectory, _ *darwinBookmarkState) (directoryScanResult, error) {
		if directoryRuntimeKey(dir.path) == directoryRuntimeKey(dirs[0].path) {
			return directoryScanResult{
				DirectoryKey: stateKey,
				BaselineIncidentIDs: map[string]struct{}{
					event.ID: {},
				},
				BaselineCompletions: []darwinBaselineReportCompletion{{
					Name:  "Baseline.ips",
					State: reportFileState{Fingerprint: "new", EventID: event.ID},
				}},
			}, nil
		}
		sourceKey := directoryRuntimeKey(dir.path) + "\x00Delivery.ips"
		return directoryScanResult{Deliverables: map[string]darwinScanDeliverable{
			event.ID: {
				Event:     event,
				SourceKey: sourceKey,
				ContributingDirKeys: map[string]struct{}{
					directoryRuntimeKey(dir.path): {},
				},
			},
		}}, nil
	}

	collector.scanDirectoriesLocked(context.Background(), dirs)

	fileState := collector.state.Directories[stateKey].Files["Baseline.ips"]
	assert.True(t, fileState.BaselineOnly)
	assert.Empty(t, fileState.EventID)
	assert.NotContains(t, collector.state.Pending, event.ID)
	assert.NotContains(t, collector.state.Acknowledged, event.ID)
	assert.Contains(t, collector.retryDirs, directoryRuntimeKey(dirs[1].path))
	require.NoError(t, validateDarwinBookmarkState(collector.state))
}

func TestDarwinCollectorNewDirectoryBaselineKeepsCrossDirectoryIncidentPending(t *testing.T) {
	systemDir := realTempDir(t)
	userDir := realTempDir(t)
	store := &fakeDarwinBookmarkStore{}
	discovered := []reportDirectory{{path: systemDir, scope: "system"}}
	collector, err := newDarwinCollectorWithDeps(
		func() []reportDirectory { return append([]reportDirectory(nil), discovered...) },
		time.Hour,
		time.Hour,
		store,
		nil,
	)
	require.NoError(t, err)
	collector.scanOnce(context.Background())

	writeIPSFile(t, systemDir, "SystemCopy.ips", "App", "/Applications/App", "INCIDENT-NEW-DIR-COPY")
	collector.scanOnce(context.Background())
	id := eventID("incident:INCIDENT-NEW-DIR-COPY")
	require.Contains(t, collector.state.Pending, id)

	writeIPSFile(t, userDir, "UserCopy.ips", "App", "/Applications/App", "INCIDENT-NEW-DIR-COPY")
	discovered = append(discovered, reportDirectory{path: userDir, scope: "user"})
	collector.scanOnce(context.Background())

	assert.Contains(t, collector.state.Pending, id)
	assert.NotContains(t, collector.state.Acknowledged, id)
	require.NoError(t, validateDarwinBookmarkState(collector.state))

	restarted, restartErr := newDarwinCollectorWithDeps(
		func() []reportDirectory { return append([]reportDirectory(nil), discovered...) },
		time.Hour,
		time.Hour,
		store,
		nil,
	)
	require.NoError(t, restartErr)
	restarted.scanOnce(context.Background())
	require.Len(t, restarted.Pending(), 1)
	assert.Equal(t, id, restarted.Pending()[0].ID)
}

func TestDarwinCollectorSaturationRebaselineKeepsCrossDirectoryIncidentPending(t *testing.T) {
	systemDir := realTempDir(t)
	saturatedDir := realTempDir(t)
	store := &fakeDarwinBookmarkStore{}
	collector, err := newDarwinCollectorWithDeps(
		func() []reportDirectory {
			return []reportDirectory{{path: systemDir, scope: "system"}, {path: saturatedDir, scope: "user"}}
		},
		time.Hour,
		time.Hour,
		store,
		nil,
	)
	require.NoError(t, err)
	collector.scanOnce(context.Background())

	saturatedPaths := make([]string, 0, maxDarwinFilesPerDirectory+1)
	for index := 0; index <= maxDarwinFilesPerDirectory; index++ {
		name := fmt.Sprintf("Saturated-%03d.ips", index)
		writeIPSFile(t, saturatedDir, name, "Old", "/Applications/Old", fmt.Sprintf("INCIDENT-SATURATED-%03d", index))
		saturatedPaths = append(saturatedPaths, filepath.Join(saturatedDir, name))
	}
	collector.scanOnce(context.Background())
	saturatedKey := hashString(filepath.Clean(saturatedDir))
	require.True(t, collector.state.Directories[saturatedKey].Saturated)

	writeIPSFile(t, systemDir, "SystemCopy.ips", "App", "/Applications/App", "INCIDENT-SATURATION-COPY")
	collector.scanOnce(context.Background())
	id := eventID("incident:INCIDENT-SATURATION-COPY")
	require.Contains(t, collector.state.Pending, id)

	for _, path := range saturatedPaths {
		require.NoError(t, os.Remove(path))
	}
	writeIPSFile(t, saturatedDir, "UserCopy.ips", "App", "/Applications/App", "INCIDENT-SATURATION-COPY")
	collector.scanOnce(context.Background())

	assert.False(t, collector.state.Directories[saturatedKey].Saturated)
	assert.Contains(t, collector.state.Pending, id)
	assert.NotContains(t, collector.state.Acknowledged, id)
	assert.NotContains(t, collector.retryDirs, directoryRuntimeKey(saturatedDir))
	require.NoError(t, validateDarwinBookmarkState(collector.state))
}

// TestDarwinCollectorPrunesFilesButRetainsPending verifies file bookkeeping cleanup preserves delivery state.
func TestDarwinCollectorPrunesFilesButRetainsPending(t *testing.T) {
	reportDir := realTempDir(t)
	path := filepath.Join(reportDir, "App.ips")
	collector := newTestCollector(t, reportDir, &fakeDarwinBookmarkStore{})
	collector.scanOnce(context.Background())
	writeIPSFile(t, reportDir, filepath.Base(path), "App", "/Applications/App", "INCIDENT-PRUNE")
	collector.scanOnce(context.Background())
	id := collector.Pending()[0].ID

	require.NoError(t, os.Remove(path))
	collector.scanOnce(context.Background())
	assert.Contains(t, collector.state.Pending, id)
	dirState := collector.state.Directories[hashString(filepath.Clean(reportDir))]
	assert.Empty(t, dirState.Files)
}

// TestDarwinCollectorRecoversCorruptBookmarkByBaselining verifies corrupt cache recovery suppresses historical events.
func TestDarwinCollectorRecoversCorruptBookmarkByBaselining(t *testing.T) {
	store := &fakeDarwinBookmarkStore{loadErr: fmtCorrupt(errors.New("invalid JSON"))}
	reportDir := realTempDir(t)
	writeIPSFile(t, reportDir, "Old.ips", "Old", "/Applications/Old", "INCIDENT-CORRUPT")

	collector := newTestCollector(t, reportDir, store)
	collector.scanOnce(context.Background())
	assert.Empty(t, collector.Pending())
	require.NotNil(t, store.state)
	assert.Equal(t, darwinBookmarkSchemaVersion, store.state.Version)
}

// TestDarwinCollectorLoadFailureDoesNotSilentlyBaseline verifies operational cache failures remain visible.
func TestDarwinCollectorLoadFailureDoesNotSilentlyBaseline(t *testing.T) {
	store := &fakeDarwinBookmarkStore{loadErr: errors.New("permission denied")}
	_, err := newDarwinCollectorWithDeps(func() []reportDirectory {
		return []reportDirectory{{path: realTempDir(t), scope: "system"}}
	}, time.Hour, time.Hour, store, nil)
	require.Error(t, err)
	assert.Equal(t, 0, store.saveCalls)
}

// TestDarwinCollectorStartAndCloseAreIdempotent verifies repeated lifecycle calls are safe.
func TestDarwinCollectorStartAndCloseAreIdempotent(t *testing.T) {
	collector := newTestCollector(t, realTempDir(t), &fakeDarwinBookmarkStore{})
	require.NoError(t, collector.Start())
	require.NoError(t, collector.Start())
	require.NoError(t, collector.Close())
	require.NoError(t, collector.Close())
	require.Error(t, collector.Start())
}

func TestDarwinCollectorCloseCancelsBlockedScan(t *testing.T) {
	collector := newTestCollector(t, realTempDir(t), &fakeDarwinBookmarkStore{})
	scanStarted := make(chan struct{})
	collector.scanDirectory = func(ctx context.Context, _ reportDirectory, _ *darwinBookmarkState) (directoryScanResult, error) {
		close(scanStarted)
		<-ctx.Done()
		return directoryScanResult{}, ctx.Err()
	}

	require.NoError(t, collector.Start())
	<-scanStarted

	closeDone := make(chan error)
	go func() {
		closeDone <- collector.Close()
	}()
	require.NoError(t, <-closeDone)
}

func TestDarwinCollectorCloseWaitsForAckAndConcurrentCallers(t *testing.T) {
	store := &fakeDarwinBookmarkStore{}
	collector := newTestCollector(t, realTempDir(t), store)
	event := validPersistedDarwinEvent("close-waits-for-ack")
	collector.state.Pending[event.ID] = event

	saveStarted := make(chan struct{})
	releaseSave := make(chan struct{})
	store.setOnSave(func(*darwinBookmarkState) {
		close(saveStarted)
		<-releaseSave
	})

	ackDone := make(chan error)
	go func() {
		ackDone <- collector.Ack([]string{event.ID})
	}()
	<-saveStarted

	firstCloseDone := make(chan error, 1)
	go func() {
		firstCloseDone <- collector.Close()
	}()
	waitForDarwinCollectorClosed(collector)
	select {
	case err := <-firstCloseDone:
		require.Failf(t, "Close returned during ACK persistence", "error: %v", err)
	default:
	}

	secondCloseDone := make(chan error, 1)
	go func() {
		secondCloseDone <- collector.Close()
	}()

	close(releaseSave)
	require.NoError(t, <-ackDone)
	require.NoError(t, <-firstCloseDone)
	require.NoError(t, <-secondCloseDone)
	require.Error(t, collector.Ack([]string{event.ID}), "closed collectors must reject new ACK work")
}

// TestDarwinCollectorPrunesExpiredUnreferencedAcknowledgements verifies retention removes only safe stale entries.
func TestDarwinCollectorPrunesExpiredUnreferencedAcknowledgements(t *testing.T) {
	now := time.Unix(10_000, 0)
	collector := newTestCollector(t, realTempDir(t), &fakeDarwinBookmarkStore{})
	collector.now = func() time.Time { return now }
	collector.identityRetention = time.Hour
	collector.state.Acknowledged["expired"] = now.Add(-2 * time.Hour).Unix()
	collector.state.Acknowledged["recent"] = now.Add(-time.Minute).Unix()

	assert.True(t, pruneAcknowledged(collector.state, now, collector.identityRetention, collector.maxAcknowledged))
	assert.NotContains(t, collector.state.Acknowledged, "expired")
	assert.Contains(t, collector.state.Acknowledged, "recent")
}

// newTestCollector creates a deterministic collector for one temporary report directory.
func newTestCollector(t *testing.T, reportDir string, store darwinBookmarkStore) *Collector {
	t.Helper()
	collector, err := newDarwinCollectorWithDeps(
		func() []reportDirectory {
			return []reportDirectory{{path: reportDir, scope: "system"}}
		},
		time.Hour,
		time.Hour,
		store,
		nil,
	)
	require.NoError(t, err)
	return collector
}

func waitForDarwinCollectorClosed(collector *Collector) {
	for {
		collector.stateMu.Lock()
		closed := collector.closed
		collector.stateMu.Unlock()
		if closed {
			return
		}
		runtime.Gosched()
	}
}

// fmtCorrupt marks a fixture error as recoverable bookmark corruption.
func fmtCorrupt(err error) error {
	return errors.Join(errDarwinBookmarkCorrupt, err)
}

// realTempDir resolves a temporary directory to satisfy no-symlink directory validation.
func realTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return dir
}

// writeIPSFile writes a representative crash report fixture into a test directory.
func writeIPSFile(t *testing.T, dir, name, appName, procPath, incidentID string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, name),
		[]byte(sampleIPSReport(appName, "com.example."+appName, procPath, incidentID)),
		0o600,
	))
}
