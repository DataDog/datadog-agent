// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package notableevents

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestSecureDarwinBookmarkStoreAtomicSaveLoadAndMode(t *testing.T) {
	base := realTempDir(t)
	store := newTestDarwinBookmarkStore(base, uint32(os.Getuid()))
	state := newDarwinBookmarkState()
	event := validPersistedDarwinEvent("event")
	state.Pending[event.ID] = event
	directory := validPersistedDirectoryState()
	directory.Files["baseline.ips"] = reportFileState{Fingerprint: "fingerprint", BaselineOnly: true}
	state.Directories[hashString("directory")] = directory

	require.NoError(t, store.Save(state))

	path := filepath.Join(base, "notable-events", darwinBookmarkFilename)
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	loaded, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, state, loaded)
}

func TestSecureDarwinBookmarkStorePreservesCustomNumbers(t *testing.T) {
	const exactInteger = "9007199254740993"
	base := realTempDir(t)
	store := newTestDarwinBookmarkStore(base, uint32(os.Getuid()))
	state := newDarwinBookmarkState()
	event := validPersistedDarwinEvent("exact-number")
	event.Custom["value"] = json.Number(exactInteger)
	state.Pending[event.ID] = event

	require.NoError(t, store.Save(state))
	loaded, err := store.Load()
	require.NoError(t, err)

	number, ok := loaded.Pending[event.ID].Custom["value"].(json.Number)
	require.True(t, ok)
	assert.Equal(t, json.Number(exactInteger), number)

	data, err := os.ReadFile(filepath.Join(base, "notable-events", darwinBookmarkFilename))
	require.NoError(t, err)
	assert.Contains(t, string(data), `"value":`+exactInteger)
}

func TestSecureDarwinBookmarkStoreAcceptsLiveParsedInteger(t *testing.T) {
	report, err := parseMacOSCrashReport([]byte(sampleIPSReport(
		"IntegerApp",
		"com.example.integer",
		"/Applications/IntegerApp",
		"INCIDENT-INTEGER",
	)))
	require.NoError(t, err)
	event := report.event("incident:INCIDENT-INTEGER", "system")
	reportPayload := event.Custom["macos_diagnostic_report"].(map[string]interface{})["report"].(map[string]interface{})
	termination := reportPayload["termination"].(map[string]interface{})
	require.IsType(t, int64(0), termination["code"], "live parser should produce an int64")

	base := realTempDir(t)
	store := newTestDarwinBookmarkStore(base, uint32(os.Getuid()))
	state := newDarwinBookmarkState()
	state.Pending[event.ID] = event
	require.NoError(t, store.Save(state))

	loaded, err := store.Load()
	require.NoError(t, err)
	loadedReport := loaded.Pending[event.ID].Custom["macos_diagnostic_report"].(map[string]interface{})["report"].(map[string]interface{})
	loadedTermination := loadedReport["termination"].(map[string]interface{})
	assert.Equal(t, json.Number("11"), loadedTermination["code"])
}

func TestSecureDarwinBookmarkStoreRejectsSemanticCorruption(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*darwinBookmarkState)
	}{
		{
			name: "too many pending events",
			mutate: func(state *darwinBookmarkState) {
				for index := 0; index <= maxDarwinPendingEvents; index++ {
					event := validPersistedDarwinEvent(fmt.Sprintf("pending-%d", index))
					state.Pending[event.ID] = event
				}
			},
		},
		{
			name: "pending key mismatch",
			mutate: func(state *darwinBookmarkState) {
				event := validPersistedDarwinEvent("pending")
				state.Pending[eventID("different")] = event
			},
		},
		{
			name: "invalid pending id prefix",
			mutate: func(state *darwinBookmarkState) {
				event := validPersistedDarwinEvent("pending")
				event.ID = "event"
				state.Pending[event.ID] = event
			},
		},
		{
			name: "oversized event string",
			mutate: func(state *darwinBookmarkState) {
				event := validPersistedDarwinEvent("pending")
				event.Title = strings.Repeat("x", maxDarwinEventStringBytes+1)
				state.Pending[event.ID] = event
			},
		},
		{
			name: "custom payload too deep",
			mutate: func(state *darwinBookmarkState) {
				event := validPersistedDarwinEvent("pending")
				var value interface{} = "leaf"
				for range maxDarwinCustomDepth + 1 {
					value = map[string]interface{}{"child": value}
				}
				event.Custom = map[string]interface{}{"root": value}
				state.Pending[event.ID] = event
			},
		},
		{
			name: "invalid directory key",
			mutate: func(state *darwinBookmarkState) {
				state.Directories["private-path"] = validPersistedDirectoryState()
			},
		},
		{
			name: "too many directories",
			mutate: func(state *darwinBookmarkState) {
				for index := 0; index <= maxDarwinDirectories; index++ {
					state.Directories[hashString(fmt.Sprintf("directory-%d", index))] = validPersistedDirectoryState()
				}
			},
		},
		{
			name: "too many files in directory",
			mutate: func(state *darwinBookmarkState) {
				directory := validPersistedDirectoryState()
				for index := 0; index <= maxDarwinFilesPerDirectory; index++ {
					directory.Files[fmt.Sprintf("%03d.ips", index)] = reportFileState{Fingerprint: "fingerprint"}
				}
				state.Directories[hashString("directory")] = directory
			},
		},
		{
			name: "invalid report filename",
			mutate: func(state *darwinBookmarkState) {
				directory := validPersistedDirectoryState()
				directory.Files["../private.ips"] = reportFileState{Fingerprint: "fingerprint"}
				state.Directories[hashString("directory")] = directory
			},
		},
		{
			name: "invalid fingerprint",
			mutate: func(state *darwinBookmarkState) {
				directory := validPersistedDirectoryState()
				directory.Files["report.ips"] = reportFileState{Fingerprint: strings.Repeat("x", maxDarwinFingerprintBytes+1)}
				state.Directories[hashString("directory")] = directory
			},
		},
		{
			name: "invalid file event id",
			mutate: func(state *darwinBookmarkState) {
				directory := validPersistedDirectoryState()
				directory.Files["report.ips"] = reportFileState{Fingerprint: "fingerprint", EventID: "event"}
				state.Directories[hashString("directory")] = directory
			},
		},
		{
			name: "baseline-only file with event id",
			mutate: func(state *darwinBookmarkState) {
				directory := validPersistedDirectoryState()
				directory.Files["report.ips"] = reportFileState{
					Fingerprint:  "fingerprint",
					EventID:      eventID("baseline-only"),
					BaselineOnly: true,
				}
				state.Directories[hashString("directory")] = directory
			},
		},
		{
			name: "invalid acknowledged timestamp",
			mutate: func(state *darwinBookmarkState) {
				state.Acknowledged[eventID("ack")] = 0
			},
		},
		{
			name: "too many acknowledgements",
			mutate: func(state *darwinBookmarkState) {
				for index := 0; index <= defaultDarwinMaxAcknowledged; index++ {
					state.Acknowledged[eventID(fmt.Sprintf("ack-%d", index))] = 1
				}
			},
		},
		{
			name: "pending and acknowledged conflict",
			mutate: func(state *darwinBookmarkState) {
				event := validPersistedDarwinEvent("conflict")
				state.Pending[event.ID] = event
				state.Acknowledged[event.ID] = 1
			},
		},
		{
			name: "too many total file records",
			mutate: func(state *darwinBookmarkState) {
				remaining := maxDarwinTotalFiles + 1
				for directoryIndex := 0; remaining > 0; directoryIndex++ {
					directory := validPersistedDirectoryState()
					count := min(remaining, maxDarwinFilesPerDirectory)
					for fileIndex := 0; fileIndex < count; fileIndex++ {
						directory.Files[fmt.Sprintf("%03d.ips", fileIndex)] = reportFileState{Fingerprint: "fingerprint"}
					}
					state.Directories[hashString(fmt.Sprintf("directory-%d", directoryIndex))] = directory
					remaining -= count
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			base := realTempDir(t)
			state := newDarwinBookmarkState()
			test.mutate(state)
			writeRawDarwinBookmark(t, base, state)

			store := newTestDarwinBookmarkStore(base, uint32(os.Getuid()))
			_, err := store.Load()
			require.Error(t, err)
			assert.ErrorIs(t, err, errDarwinBookmarkCorrupt)
		})
	}
}

func TestSecureDarwinBookmarkStoreSaveRejectsPendingAcknowledgedConflict(t *testing.T) {
	base := realTempDir(t)
	store := newTestDarwinBookmarkStore(base, uint32(os.Getuid()))
	valid := newDarwinBookmarkState()
	require.NoError(t, store.Save(valid))
	path := filepath.Join(base, "notable-events", darwinBookmarkFilename)
	before, err := os.ReadFile(path)
	require.NoError(t, err)

	invalid := newDarwinBookmarkState()
	event := validPersistedDarwinEvent("save-conflict")
	invalid.Pending[event.ID] = event
	invalid.Acknowledged[event.ID] = 1

	err = store.Save(invalid)
	require.Error(t, err)
	assert.ErrorIs(t, err, errDarwinBookmarkCorrupt)
	after, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, before, after)
	matches, globErr := filepath.Glob(filepath.Join(base, "notable-events", ".bookmark-*.tmp"))
	require.NoError(t, globErr)
	assert.Empty(t, matches)
}

func TestDarwinBookmarkMaximumBoundedStateFitsStoreLimit(t *testing.T) {
	state := newDarwinBookmarkState()
	for directoryIndex := 0; directoryIndex < maxDarwinTotalFiles/maxDarwinFilesPerDirectory; directoryIndex++ {
		directory := validPersistedDirectoryState()
		for fileIndex := 0; fileIndex < maxDarwinFilesPerDirectory; fileIndex++ {
			prefix := fmt.Sprintf("%03d-", fileIndex)
			name := prefix + strings.Repeat("n", maxReportBasenameSize-len(prefix)-len(".ips")) + ".ips"
			id := eventID(fmt.Sprintf("file-%d-%d", directoryIndex, fileIndex))
			directory.Files[name] = reportFileState{
				Fingerprint: strings.Repeat("f", maxDarwinFingerprintBytes),
				EventID:     id,
			}
			state.Acknowledged[id] = 1
		}
		state.Directories[hashString(fmt.Sprintf("directory-%d", directoryIndex))] = directory
	}

	template := maximumWireDarwinEvent(t)
	for index := 0; index < maxDarwinPendingEvents; index++ {
		event := template
		event.ID = eventID(fmt.Sprintf("pending-%d", index))
		state.Pending[event.ID] = event
	}

	require.NoError(t, validateDarwinBookmarkState(state))
	data, err := json.Marshal(state)
	require.NoError(t, err)
	assert.Less(t, len(data), maxDarwinBookmarkSize)
}

func TestSecureDarwinBookmarkStoreRejectsSymlinkedDirectory(t *testing.T) {
	base := realTempDir(t)
	target := realTempDir(t)
	require.NoError(t, os.Symlink(target, filepath.Join(base, "notable-events")))

	store := newTestDarwinBookmarkStore(base, uint32(os.Getuid()))
	_, err := store.Load()
	require.Error(t, err)
}

func TestSecureDarwinBookmarkStoreRejectsSymlinkedFile(t *testing.T) {
	base := realTempDir(t)
	stateDirectory := filepath.Join(base, "notable-events")
	require.NoError(t, os.Mkdir(stateDirectory, 0o700))
	target := filepath.Join(base, "target")
	require.NoError(t, os.WriteFile(target, []byte(`{"version":1,"directories":{}}`), 0o600))
	require.NoError(t, os.Symlink(target, filepath.Join(stateDirectory, darwinBookmarkFilename)))

	store := newTestDarwinBookmarkStore(base, uint32(os.Getuid()))
	_, err := store.Load()
	require.Error(t, err)
	require.Error(t, store.Save(newDarwinBookmarkState()))
}

func TestSecureDarwinBookmarkStoreRejectsUnsafeDirectoryModeAndOwner(t *testing.T) {
	t.Run("mode", func(t *testing.T) {
		base := realTempDir(t)
		stateDirectory := filepath.Join(base, "notable-events")
		require.NoError(t, os.Mkdir(stateDirectory, 0o700))
		require.NoError(t, os.Chmod(stateDirectory, 0o770))

		store := newTestDarwinBookmarkStore(base, uint32(os.Getuid()))
		_, err := store.Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsafe mode")
	})

	t.Run("owner", func(t *testing.T) {
		base := realTempDir(t)
		store := newTestDarwinBookmarkStore(base, uint32(os.Getuid()+1))
		_, err := store.Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "owned by uid")
	})
}

func TestSecureDarwinBookmarkStoreRejectsUnsafeFileMode(t *testing.T) {
	base := realTempDir(t)
	stateDirectory := filepath.Join(base, "notable-events")
	require.NoError(t, os.Mkdir(stateDirectory, 0o700))
	require.NoError(t, os.WriteFile(
		filepath.Join(stateDirectory, darwinBookmarkFilename),
		[]byte(`{"version":1,"directories":{}}`),
		0o640,
	))

	store := newTestDarwinBookmarkStore(base, uint32(os.Getuid()))
	_, err := store.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected 0600")
	require.Error(t, store.Save(newDarwinBookmarkState()))
}

func TestSecureDarwinBookmarkStoreEnforcesReadSizeCap(t *testing.T) {
	base := realTempDir(t)
	stateDirectory := filepath.Join(base, "notable-events")
	require.NoError(t, os.Mkdir(stateDirectory, 0o700))
	file, err := os.OpenFile(
		filepath.Join(stateDirectory, darwinBookmarkFilename),
		os.O_CREATE|os.O_WRONLY,
		0o600,
	)
	require.NoError(t, err)
	require.NoError(t, file.Truncate(maxDarwinBookmarkSize+1))
	require.NoError(t, file.Close())

	store := newTestDarwinBookmarkStore(base, uint32(os.Getuid()))
	_, err = store.Load()
	require.Error(t, err)
	assert.ErrorIs(t, err, errDarwinBookmarkCorrupt)
}

func TestSecureDarwinBookmarkStoreRejectsOtherSchemaVersions(t *testing.T) {
	base := realTempDir(t)
	stateDirectory := filepath.Join(base, "notable-events")
	require.NoError(t, os.Mkdir(stateDirectory, 0o700))
	require.NoError(t, os.WriteFile(
		filepath.Join(stateDirectory, darwinBookmarkFilename),
		[]byte(`{"version":2,"directories":{}}`),
		0o600,
	))

	store := newTestDarwinBookmarkStore(base, uint32(os.Getuid()))
	_, err := store.Load()
	require.Error(t, err)
	assert.ErrorIs(t, err, errDarwinBookmarkCorrupt)
}

func TestSecureDarwinBookmarkStoreSynchronizesExistingManagedParentsOnce(t *testing.T) {
	base := realTempDir(t)
	require.NoError(t, os.MkdirAll(filepath.Join(base, "agent", "notable-events"), 0o700))
	store := &secureDarwinBookmarkStore{
		baseDirectory:         base,
		directories:           []string{"agent", "notable-events"},
		managedDirectoryStart: 0,
		expectedUID:           uint32(os.Getuid()),
		fsync:                 unix.Fsync,
	}
	syncCalls := 0
	syncedParents := make([]string, 0, 2)
	store.fsync = func(fd int) error {
		syncCalls++
		syncedParents = append(syncedParents, darwinFDIdentity(t, fd))
		return unix.Fsync(fd)
	}

	_, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, 2, syncCalls, "first initialization must sync every managed parent")
	assert.Equal(t, []string{
		darwinPathIdentity(t, base),
		darwinPathIdentity(t, filepath.Join(base, "agent")),
	}, syncedParents)

	_, err = store.Load()
	require.NoError(t, err)
	assert.Equal(t, 2, syncCalls, "an unchanged durable tree must not be synchronized repeatedly")
}

func TestSecureDarwinBookmarkStoreRetriesAmbiguousParentSynchronization(t *testing.T) {
	base := realTempDir(t)
	store := &secureDarwinBookmarkStore{
		baseDirectory:         base,
		directories:           []string{"agent", "notable-events"},
		managedDirectoryStart: 0,
		expectedUID:           uint32(os.Getuid()),
		fsync:                 unix.Fsync,
	}
	syncCalls := 0
	syncedParents := make([]string, 0, 4)
	store.fsync = func(fd int) error {
		syncCalls++
		syncedParents = append(syncedParents, darwinFDIdentity(t, fd))
		if syncCalls == 1 {
			return errors.New("injected parent sync failure")
		}
		return unix.Fsync(fd)
	}

	_, err := store.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sync parent of state directory agent")

	_, err = store.Load()
	require.NoError(t, err)
	assert.Equal(t, 3, syncCalls, "retry must resync every parent, including the existing first component")
	assert.Equal(t, []string{
		darwinPathIdentity(t, base),
		darwinPathIdentity(t, base),
		darwinPathIdentity(t, filepath.Join(base, "agent")),
	}, syncedParents)

	_, err = store.Load()
	require.NoError(t, err)
	assert.Equal(t, 3, syncCalls)

	require.NoError(t, os.Remove(filepath.Join(base, "agent", "notable-events")))
	_, err = store.Load()
	require.NoError(t, err)
	assert.Equal(t, 4, syncCalls, "recreating a managed component must sync its parent")
	assert.Equal(t, darwinPathIdentity(t, filepath.Join(base, "agent")), syncedParents[3])
}

func TestSecureDarwinBookmarkStoreCleansTempOnFileSyncFailure(t *testing.T) {
	base := realTempDir(t)
	store := newTestDarwinBookmarkStore(base, uint32(os.Getuid()))
	_, err := store.Load()
	require.NoError(t, err)
	store.fsync = func(int) error { return errors.New("injected sync failure") }

	require.Error(t, store.Save(newDarwinBookmarkState()))
	matches, err := filepath.Glob(filepath.Join(base, "notable-events", ".bookmark-*.tmp"))
	require.NoError(t, err)
	assert.Empty(t, matches)
	_, err = os.Stat(filepath.Join(base, "notable-events", darwinBookmarkFilename))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestSecureDarwinBookmarkStoreReportsDirectorySyncFailure(t *testing.T) {
	base := realTempDir(t)
	store := newTestDarwinBookmarkStore(base, uint32(os.Getuid()))
	_, err := store.Load()
	require.NoError(t, err)
	syncCalls := 0
	store.fsync = func(fd int) error {
		syncCalls++
		if syncCalls == 2 {
			return errors.New("injected directory sync failure")
		}
		return unix.Fsync(fd)
	}
	state := newDarwinBookmarkState()
	event := validPersistedDarwinEvent("event")
	state.Pending[event.ID] = event

	err = store.Save(state)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sync bookmark directory")
	loaded, loadErr := store.Load()
	require.NoError(t, loadErr)
	assert.Equal(t, state, loaded)
}

func validPersistedDarwinEvent(identity string) Event {
	id := eventID(identity)
	return Event{
		ID:        id,
		Timestamp: time.Date(2026, time.July, 22, 12, 0, 0, 0, time.UTC),
		EventType: "Application crash",
		Title:     "Application crash: Test",
		Message:   "An application crashed unexpectedly",
		Custom: map[string]interface{}{
			"macos_diagnostic_report": map[string]interface{}{
				"scope": "system",
			},
		},
	}
}

func darwinFDIdentity(t *testing.T, fd int) string {
	t.Helper()
	var stat unix.Stat_t
	require.NoError(t, unix.Fstat(fd, &stat))
	return fmt.Sprintf("%d:%d", stat.Dev, stat.Ino)
}

func darwinPathIdentity(t *testing.T, path string) string {
	t.Helper()
	var stat unix.Stat_t
	require.NoError(t, unix.Stat(path, &stat))
	return fmt.Sprintf("%d:%d", stat.Dev, stat.Ino)
}

func maximumWireDarwinEvent(t *testing.T) Event {
	t.Helper()
	event := validPersistedDarwinEvent("maximum")
	event.EventType = strings.Repeat("e", maxDarwinEventStringBytes)
	event.Title = strings.Repeat("t", maxDarwinEventStringBytes)
	event.Message = strings.Repeat("m", maxDarwinEventStringBytes)
	custom := make(map[string]interface{})
	for index := 0; index < maxDarwinCustomItems; index++ {
		keyPrefix := fmt.Sprintf("%03d-", index)
		key := keyPrefix + strings.Repeat("k", maxDarwinEventStringBytes-len(keyPrefix))
		custom[key] = strings.Repeat("v", maxDarwinEventStringBytes)
		event.Custom = custom
		if !eventFitsWireLimit(event) {
			delete(custom, key)
			break
		}
	}
	data, err := json.Marshal(event)
	require.NoError(t, err)
	require.Greater(t, len(data), maxDarwinEventWireSize-512)
	return event
}

func validPersistedDirectoryState() *directoryBookmarkState {
	return &directoryBookmarkState{
		Initialized: true,
		Files:       make(map[string]reportFileState),
		LastSeen:    1,
	}
}

func writeRawDarwinBookmark(t *testing.T, base string, state *darwinBookmarkState) {
	t.Helper()
	stateDirectory := filepath.Join(base, "notable-events")
	require.NoError(t, os.Mkdir(stateDirectory, 0o700))
	data, err := json.Marshal(state)
	require.NoError(t, err)
	require.LessOrEqual(t, len(data), maxDarwinBookmarkSize)
	require.NoError(t, os.WriteFile(filepath.Join(stateDirectory, darwinBookmarkFilename), data, 0o600))
}
