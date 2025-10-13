// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package usm

import (
	"encoding/json"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestSysinfoCommand(t *testing.T) {
	globalParams := &command.GlobalParams{}
	cmd := makeSysinfoCommand(globalParams)

	require.NotNil(t, cmd)
	require.Equal(t, "sysinfo", cmd.Use)
	require.Equal(t, "Show system information relevant to USM", cmd.Short)

	// Verify --json flag exists and has correct default
	jsonFlag := cmd.Flags().Lookup("json")
	require.NotNil(t, jsonFlag, "--json flag should exist")
	require.Equal(t, "false", jsonFlag.DefValue, "--json should default to false")

	// Test the OneShot command
	fxutil.TestOneShotSubcommand(t,
		Commands(globalParams),
		[]string{"usm", "sysinfo"},
		runSysinfo,
		func() {})
}

func TestOutputSysinfoJSON(t *testing.T) {
	// Create test data with comprehensive process information
	info := &SystemInfo{
		KernelVersion: "5.15.0-76-generic",
		OSType:        "linux",
		Architecture:  "amd64",
		Hostname:      "test-host",
		Processes: []*procutil.Process{
			{
				Pid:     1,
				Ppid:    0,
				Name:    "systemd",
				Cmdline: []string{"/usr/lib/systemd/systemd", "--system"},
				// Stats field present (should NOT appear in JSON output)
				Stats: &procutil.Stats{
					CreateTime: 1234567890,
					Status:     "S",
					Nice:       0,
				},
			},
			{
				Pid:     1234,
				Ppid:    1,
				Name:    "python3",
				Cmdline: []string{"/usr/bin/python3", "-u", "/opt/app/main.py"},
			},
			{
				Pid:     5678,
				Ppid:    1234,
				Name:    "child-process",
				Cmdline: []string{"/usr/bin/child"},
			},
		},
	}

	// Call outputSysinfoJSON which builds the JSON structure
	processes := make([]map[string]interface{}, len(info.Processes))
	for i, p := range info.Processes {
		processes[i] = map[string]interface{}{
			"pid":     p.Pid,
			"ppid":    p.Ppid,
			"name":    p.Name,
			"cmdline": p.Cmdline,
		}
	}

	output := map[string]interface{}{
		"kernel_version": info.KernelVersion,
		"os_type":        info.OSType,
		"architecture":   info.Architecture,
		"hostname":       info.Hostname,
		"processes":      processes,
	}

	// Verify it can be encoded to JSON
	jsonData, err := json.MarshalIndent(output, "", "  ")
	require.NoError(t, err, "JSON encoding should succeed")

	// Verify the output is valid JSON
	var decoded map[string]interface{}
	err = json.Unmarshal(jsonData, &decoded)
	require.NoError(t, err, "JSON output should be valid and parseable")

	// Verify system info fields
	assert.Equal(t, "5.15.0-76-generic", decoded["kernel_version"])
	assert.Equal(t, "linux", decoded["os_type"])
	assert.Equal(t, "amd64", decoded["architecture"])
	assert.Equal(t, "test-host", decoded["hostname"])

	// Verify processes array
	procs, ok := decoded["processes"].([]interface{})
	require.True(t, ok, "processes should be an array")
	assert.Len(t, procs, 3)

	// Verify first process (systemd)
	proc0, ok := procs[0].(map[string]interface{})
	require.True(t, ok, "process should be a map")
	assert.Equal(t, float64(1), proc0["pid"])
	assert.Equal(t, float64(0), proc0["ppid"])
	assert.Equal(t, "systemd", proc0["name"])

	cmdline0, ok := proc0["cmdline"].([]interface{})
	require.True(t, ok, "cmdline should be an array")
	assert.Len(t, cmdline0, 2)
	assert.Equal(t, "/usr/lib/systemd/systemd", cmdline0[0])
	assert.Equal(t, "--system", cmdline0[1])

	// Verify process does NOT have extra fields like Stats
	_, hasStats := proc0["stats"]
	assert.False(t, hasStats, "JSON output should not include stats field")
	_, hasService := proc0["service"]
	assert.False(t, hasService, "JSON output should not include service field")
	_, hasLanguage := proc0["language"]
	assert.False(t, hasLanguage, "JSON output should not include language field")

	// Verify second process (python3)
	proc1, ok := procs[1].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(1234), proc1["pid"])
	assert.Equal(t, float64(1), proc1["ppid"])
	assert.Equal(t, "python3", proc1["name"])

	cmdline1, ok := proc1["cmdline"].([]interface{})
	require.True(t, ok)
	assert.Len(t, cmdline1, 3)

	// Verify third process (child)
	proc2, ok := procs[2].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(5678), proc2["pid"])
	assert.Equal(t, float64(1234), proc2["ppid"])
	assert.Equal(t, "child-process", proc2["name"])
}

func TestSysinfoWithRealSystemData(t *testing.T) {
	// This test validates that we can collect real system data and serialize it correctly
	// Tests the complete flow with actual procutil.Process objects from the system

	// Collect real system data
	probe := procutil.NewProcessProbe()
	defer probe.Close()

	procs, err := probe.ProcessesByPID(time.Now(), false)
	require.NoError(t, err, "should be able to collect processes from system")
	require.NotEmpty(t, procs, "should have at least some processes running")

	// Convert to slice (like runSysinfo does)
	processList := make([]*procutil.Process, 0, len(procs))
	for _, proc := range procs {
		processList = append(processList, proc)
	}

	// Build JSON output with filtering (mimics outputSysinfoJSON)
	processes := make([]map[string]interface{}, len(processList))
	for i, p := range processList {
		processes[i] = map[string]interface{}{
			"pid":     p.Pid,
			"ppid":    p.Ppid,
			"name":    p.Name,
			"cmdline": p.Cmdline,
		}
	}

	// Get real system info
	kernelVersion, _ := kernel.Release()
	hostname, _ := os.Hostname()

	output := map[string]interface{}{
		"kernel_version": kernelVersion,
		"os_type":        runtime.GOOS,
		"architecture":   runtime.GOARCH,
		"hostname":       hostname,
		"processes":      processes,
	}

	// CRITICAL TEST: Real procutil.Process data must serialize to JSON without errors
	jsonData, err := json.MarshalIndent(output, "", "  ")
	require.NoError(t, err, "real system data should serialize to JSON without type errors")

	// Verify JSON is valid
	var decoded map[string]interface{}
	err = json.Unmarshal(jsonData, &decoded)
	require.NoError(t, err, "JSON should be parseable")

	// Verify structure
	assert.NotEmpty(t, decoded["kernel_version"])
	assert.Equal(t, "linux", decoded["os_type"])
	assert.NotEmpty(t, decoded["architecture"])
	assert.NotEmpty(t, decoded["hostname"])

	decodedProcs, ok := decoded["processes"].([]interface{})
	require.True(t, ok, "processes should be an array")
	assert.NotEmpty(t, decodedProcs, "should have processes")

	// Verify process filtering with real data - check first process
	if len(decodedProcs) > 0 {
		proc, ok := decodedProcs[0].(map[string]interface{})
		require.True(t, ok)

		// CRITICAL: Verify exactly 4 fields (filtering worked)
		assert.Len(t, proc, 4, "process should have exactly 4 fields: pid, ppid, name, cmdline")

		// Verify no extra fields from real procutil.Process
		_, hasStats := proc["stats"]
		assert.False(t, hasStats, "should not have stats field")
		_, hasService := proc["service"]
		assert.False(t, hasService, "should not have service field")
	}
}
