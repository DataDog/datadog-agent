// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/utils"
)

// ProcessStoreEntry contains debug information about a process tracked by the
// module.
type ProcessStoreEntry struct {
	PID         int32  `json:"pid"`
	RuntimeID   string `json:"runtime_id"`
	Service     string `json:"service"`
	Version     string `json:"version"`
	Environment string `json:"environment"`
	Executable  string `json:"executable"`
}

// processStoreDebugInfo returns a snapshot of all tracked processes.
func (ps *processStore) debugInfo() []ProcessStoreEntry {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	entries := make([]ProcessStoreEntry, 0, len(ps.processes))
	for _, p := range ps.processes {
		entries = append(entries, ProcessStoreEntry{
			PID:         p.PID,
			RuntimeID:   p.runtimeID,
			Service:     p.service,
			Version:     p.version,
			Environment: p.environment,
			Executable:  p.executable.Path,
		})
	}
	return entries
}

// ProbeDiagnostics contains the diagnostic status for a single probe,
// transposed so consumers can look up a probe and see all its statuses.
type ProbeDiagnostics struct {
	ProbeID    string   `json:"probe_id"`
	Version    int      `json:"version"`
	RuntimeIDs []string `json:"runtime_ids"`
	Statuses   []string `json:"statuses"`
}

// DiagnosticsDebugInfo is keyed by probe ID+version for easy consumption.
// Each entry shows which statuses have been reported for that probe.
type DiagnosticsDebugInfo struct {
	Probes []ProbeDiagnostics `json:"probes"`
}

// diagnosticTracker.collectInto adds items from the tracker into the
// accumulator map, tagging each with the given status name.
func (dt *diagnosticTracker) collectInto(acc map[probeVersionKey]*ProbeDiagnostics, status string) {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	dt.mu.btree.Ascend(func(item diagnosticItem) bool {
		key := probeVersionKey{probeID: item.key.probeID, version: item.version}
		entry, ok := acc[key]
		if !ok {
			entry = &ProbeDiagnostics{
				ProbeID: item.key.probeID,
				Version: item.version,
			}
			acc[key] = entry
		}
		// Add runtime ID if not already present for this probe.
		found := false
		for _, rid := range entry.RuntimeIDs {
			if rid == item.key.runtimeID {
				found = true
				break
			}
		}
		if !found {
			entry.RuntimeIDs = append(entry.RuntimeIDs, item.key.runtimeID)
		}
		// Add status if not already present.
		for _, s := range entry.Statuses {
			if s == status {
				return true
			}
		}
		entry.Statuses = append(entry.Statuses, status)
		return true
	})
}

type probeVersionKey struct {
	probeID string
	version int
}

func (dm *diagnosticsManager) debugInfo() DiagnosticsDebugInfo {
	acc := make(map[probeVersionKey]*ProbeDiagnostics)
	dm.received.collectInto(acc, "received")
	dm.installed.collectInto(acc, "installed")
	dm.emitted.collectInto(acc, "emitting")
	dm.errors.collectInto(acc, "error")

	probes := make([]ProbeDiagnostics, 0, len(acc))
	for _, entry := range acc {
		probes = append(probes, *entry)
	}
	return DiagnosticsDebugInfo{Probes: probes}
}

// SymDBDebugInfo contains the state of the SymDB upload manager.
type SymDBDebugInfo struct {
	Enabled            bool     `json:"enabled"`
	QueueSize          int      `json:"queue_size"`
	TrackedProcesses   []string `json:"tracked_processes"`
	CurrentUpload      string   `json:"current_upload,omitempty"`
	HasPersistentCache bool     `json:"has_persistent_cache"`
}

func (m *symdbManager) debugInfo() SymDBDebugInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	tracked := make([]string, 0, len(m.mu.trackedProcesses))
	for k := range m.mu.trackedProcesses {
		tracked = append(tracked, k.String())
	}

	var currentUpload string
	if m.mu.currentUpload != nil {
		currentUpload = m.mu.currentUpload.procID.String()
	}

	return SymDBDebugInfo{
		Enabled:            m.enabled,
		QueueSize:          len(m.mu.queuedUploads),
		TrackedProcesses:   tracked,
		CurrentUpload:      currentUpload,
		HasPersistentCache: m.persistentCache != nil,
	}
}

// ConfigDebugInfo contains the effective configuration of the dynamic
// instrumentation module, excluding testing knobs.
type ConfigDebugInfo struct {
	LogUploaderURL         string            `json:"log_uploader_url"`
	DiagsUploaderURL       string            `json:"diags_uploader_url"`
	SymDBUploadEnabled     bool              `json:"symdb_upload_enabled"`
	SymDBUploaderURL       string            `json:"symdb_uploader_url"`
	ProbeTombstoneFilePath string            `json:"probe_tombstone_file_path"`
	SymDBCacheDir          string            `json:"symdb_cache_dir"`
	DiskCacheEnabled       bool              `json:"disk_cache_enabled"`
	CircuitBreaker         map[string]string `json:"circuit_breaker"`
	DiscoveredTypesLimit   int               `json:"discovered_types_limit"`
	RecompilationRateLimit float64           `json:"recompilation_rate_limit"`
	RecompilationRateBurst int               `json:"recompilation_rate_burst"`
}

func configDebugInfo(c *Config) ConfigDebugInfo {
	return ConfigDebugInfo{
		LogUploaderURL:         c.LogUploaderURL,
		DiagsUploaderURL:       c.DiagsUploaderURL,
		SymDBUploadEnabled:     c.SymDBUploadEnabled,
		SymDBUploaderURL:       c.SymDBUploaderURL,
		ProbeTombstoneFilePath: c.ProbeTombstoneFilePath,
		SymDBCacheDir:          c.SymDBCacheDir,
		DiskCacheEnabled:       c.DiskCacheEnabled,
		CircuitBreaker: map[string]string{
			"interval":             c.ActuatorConfig.CircuitBreakerConfig.Interval.String(),
			"per_probe_cpu_limit":  fmt.Sprintf("%f", c.ActuatorConfig.CircuitBreakerConfig.PerProbeCPULimit),
			"all_probes_cpu_limit": fmt.Sprintf("%f", c.ActuatorConfig.CircuitBreakerConfig.AllProbesCPULimit),
			"interrupt_overhead":   c.ActuatorConfig.CircuitBreakerConfig.InterruptOverhead.String(),
		},
		DiscoveredTypesLimit:   c.ActuatorConfig.DiscoveredTypesLimit,
		RecompilationRateLimit: c.ActuatorConfig.RecompilationRateLimit,
		RecompilationRateBurst: c.ActuatorConfig.RecompilationRateBurst,
	}
}

// registerDebugEndpoints registers the debug HTTP endpoints on the module router.
func (m *Module) registerDebugEndpoints(router *module.Router) {
	router.HandleFunc(
		"/debug/stats",
		utils.WithConcurrencyLimit(
			utils.DefaultMaxConcurrentRequests,
			func(w http.ResponseWriter, req *http.Request) {
				stats := m.GetStats()
				utils.WriteAsJSON(req, w, stats, utils.PrettyPrint)
			},
		),
	)

	router.HandleFunc(
		"/debug/state",
		utils.WithConcurrencyLimit(
			utils.DefaultMaxConcurrentRequests,
			func(w http.ResponseWriter, req *http.Request) {
				type stateInfo struct {
					Processes []ProcessStoreEntry `json:"processes"`
					Actuator  *actuator.DebugInfo `json:"actuator"`
				}
				info := stateInfo{
					Processes: m.store.debugInfo(),
				}
				if m.shutdown.realDependencies.actuator != nil {
					info.Actuator = m.shutdown.realDependencies.actuator.DebugInfo()
				}
				utils.WriteAsJSON(req, w, info, utils.PrettyPrint)
			},
		),
	)

	router.HandleFunc(
		"/debug/diagnostics",
		utils.WithConcurrencyLimit(
			utils.DefaultMaxConcurrentRequests,
			func(w http.ResponseWriter, req *http.Request) {
				info := m.diagnostics.debugInfo()
				utils.WriteAsJSON(req, w, info, utils.PrettyPrint)
			},
		),
	)

	router.HandleFunc(
		"/debug/config",
		utils.WithConcurrencyLimit(
			utils.DefaultMaxConcurrentRequests,
			func(w http.ResponseWriter, req *http.Request) {
				if m.config == nil {
					utils.WriteAsJSON(req, w, json.RawMessage(`null`), utils.CompactOutput)
					return
				}
				info := configDebugInfo(m.config)
				utils.WriteAsJSON(req, w, info, utils.PrettyPrint)
			},
		),
	)

	router.HandleFunc(
		"/debug/symdb",
		utils.WithConcurrencyLimit(
			utils.DefaultMaxConcurrentRequests,
			func(w http.ResponseWriter, req *http.Request) {
				if sm, ok := m.symdb.(*symdbManager); ok {
					info := sm.debugInfo()
					utils.WriteAsJSON(req, w, info, utils.PrettyPrint)
				} else {
					utils.WriteAsJSON(req, w, json.RawMessage(`{"enabled":false}`), utils.CompactOutput)
				}
			},
		),
	)
}
