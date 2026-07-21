// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/process"
	"github.com/DataDog/datadog-agent/pkg/dyninst/uploader"
)

func TestProcessStoreDebugInfoEmpty(t *testing.T) {
	ps := newProcessStore()
	entries := ps.debugInfo()
	assert.Empty(t, entries)
}

func TestProcessStoreDebugInfo(t *testing.T) {
	ps := newProcessStore()
	ps.ensureExists(&process.Config{
		Info: process.Info{
			ProcessID:   process.ID{PID: 42},
			Service:     "svc-a",
			Version:     "1.0",
			Environment: "prod",
			Executable:  process.Executable{Path: "/usr/bin/myapp"},
		},
		RuntimeID: "runtime-1",
	})
	ps.ensureExists(&process.Config{
		Info: process.Info{
			ProcessID:  process.ID{PID: 99},
			Service:    "svc-b",
			Executable: process.Executable{Path: "/usr/bin/other"},
		},
		RuntimeID: "runtime-2",
	})

	entries := ps.debugInfo()
	assert.Len(t, entries, 2)

	byPID := map[int32]ProcessStoreEntry{}
	for _, e := range entries {
		byPID[e.PID] = e
	}

	e1 := byPID[42]
	assert.Equal(t, "runtime-1", e1.RuntimeID)
	assert.Equal(t, "svc-a", e1.Service)
	assert.Equal(t, "1.0", e1.Version)
	assert.Equal(t, "prod", e1.Environment)
	assert.Equal(t, "/usr/bin/myapp", e1.Executable)

	e2 := byPID[99]
	assert.Equal(t, "runtime-2", e2.RuntimeID)
	assert.Equal(t, "svc-b", e2.Service)
}

func TestDiagnosticsDebugInfoEmpty(t *testing.T) {
	dm := newDiagnosticsManager(noopDiagUploader{})
	info := dm.debugInfo()
	assert.Empty(t, info.Probes)
}

func TestDiagnosticsDebugInfoTransposed(t *testing.T) {
	dm := newDiagnosticsManager(noopDiagUploader{})

	// Mark some diagnostics.
	dm.received.mark("rt-1", "probe-a", 1)
	dm.installed.mark("rt-1", "probe-a", 1)
	dm.emitted.mark("rt-1", "probe-a", 1)
	dm.received.mark("rt-2", "probe-a", 1)

	// A different probe with an error.
	dm.received.mark("rt-1", "probe-b", 2)
	dm.errors.mark("rt-1", "probe-b", 2)

	info := dm.debugInfo()

	// Should have 2 probes.
	require.Len(t, info.Probes, 2)

	byProbe := map[string]ProbeDiagnostics{}
	for _, p := range info.Probes {
		byProbe[p.ProbeID] = p
	}

	probeA := byProbe["probe-a"]
	assert.Equal(t, "probe-a", probeA.ProbeID)
	assert.Equal(t, 1, probeA.Version)
	assert.Contains(t, probeA.Statuses, "received")
	assert.Contains(t, probeA.Statuses, "installed")
	assert.Contains(t, probeA.Statuses, "emitting")
	// Should have both runtime IDs.
	assert.Contains(t, probeA.RuntimeIDs, "rt-1")
	assert.Contains(t, probeA.RuntimeIDs, "rt-2")

	probeB := byProbe["probe-b"]
	assert.Equal(t, "probe-b", probeB.ProbeID)
	assert.Equal(t, 2, probeB.Version)
	assert.Contains(t, probeB.Statuses, "received")
	assert.Contains(t, probeB.Statuses, "error")
	assert.Equal(t, []string{"rt-1"}, probeB.RuntimeIDs)
}

func TestSymDBDebugInfoDisabled(t *testing.T) {
	// A zero-value symdbManager (nil uploadURL) is disabled.
	sm := &symdbManager{}
	sm.mu.trackedProcesses = make(map[processKey]struct{})
	info := sm.debugInfo()
	assert.False(t, info.Enabled)
	assert.Empty(t, info.TrackedProcesses)
	assert.Empty(t, info.CurrentUpload)
}

func TestConfigDebugInfo(t *testing.T) {
	cfg := &Config{
		LogUploaderURL:         "http://localhost:8126/debugger/v2/input",
		DiagsUploaderURL:       "http://localhost:8126/debugger/v1/diagnostics",
		SymDBUploadEnabled:     true,
		SymDBUploaderURL:       "http://localhost:8126/symdb/v1/input",
		ProbeTombstoneFilePath: "/tmp/tombstone.json",
		SymDBCacheDir:          "/tmp/cache",
		DiskCacheEnabled:       true,
		ActuatorConfig: actuator.Config{
			DiscoveredTypesLimit:   512,
			RecompilationRateLimit: 0.5,
			RecompilationRateBurst: 3,
		},
	}

	info := configDebugInfo(cfg)

	assert.Equal(t, "http://localhost:8126/debugger/v2/input", info.LogUploaderURL)
	assert.True(t, info.SymDBUploadEnabled)
	assert.True(t, info.DiskCacheEnabled)
	assert.Equal(t, 512, info.DiscoveredTypesLimit)
	assert.Equal(t, 0.5, info.RecompilationRateLimit)
	assert.Equal(t, 3, info.RecompilationRateBurst)
	assert.NotEmpty(t, info.CircuitBreaker)
}

// noopDiagUploader is a no-op DiagnosticsUploader for testing.
type noopDiagUploader struct{}

func (noopDiagUploader) Enqueue(_ *uploader.DiagnosticMessage) error { return nil }
