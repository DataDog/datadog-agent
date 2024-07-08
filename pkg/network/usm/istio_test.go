// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetEnvoyPath(t *testing.T) {
	_, pid := createFakeProc(t, "/bin")
	monitor := newIstioTestMonitor(t)

	t.Run("an actual envoy process", func(t *testing.T) {
		path := monitor.getEnvoyPath(uint32(pid))
		assert.True(t, strings.HasSuffix(path, "/bin/envoy"))
	})
	t.Run("something else", func(t *testing.T) {
		path := monitor.getEnvoyPath(uint32(2))
		assert.Empty(t, "", path)
	})
}

func TestGetEnvoyPathWithConfig(t *testing.T) {
	t.Setenv("DD_SERVICE_MONITORING_CONFIG_TLS_ISTIO_ENVOY_PATH", "/test/envoy")

	_, pid := createFakeProc(t, "/test")
	monitor := newIstioTestMonitor(t)

	t.Run("an actual envoy process", func(t *testing.T) {
		path := monitor.getEnvoyPath(uint32(pid))
		assert.True(t, strings.HasSuffix(path, "/test/envoy"))
	})
}

func TestIstioSync(t *testing.T) {
	t.Run("calling sync for the first time", func(t *testing.T) {
		procRoot1, _ := createFakeProc(t, filepath.Join("test1", "bin"))
		procRoot2, _ := createFakeProc(t, filepath.Join("test2", "bin"))
		monitor := newIstioTestMonitor(t)
		registerRecorder := new(utils.CallbackRecorder)

		// Setup test callbacks
		monitor.registerCB = registerRecorder.Callback()
		monitor.unregisterCB = utils.IgnoreCB

		// Calling sync should detect the two envoy processes
		monitor.sync()

		pathID1, err := utils.NewPathIdentifier(filepath.Join(procRoot1, "test1", "bin", "envoy"))
		require.NoError(t, err)

		pathID2, err := utils.NewPathIdentifier(filepath.Join(procRoot2, "test2", "bin", "envoy"))
		require.NoError(t, err)

		assert.Equal(t, 2, registerRecorder.TotalCalls())
		assert.Equal(t, 1, registerRecorder.CallsForPathID(pathID1))
		assert.Equal(t, 1, registerRecorder.CallsForPathID(pathID2))
	})

	t.Run("calling sync multiple times", func(t *testing.T) {
		procRoot1, _ := createFakeProc(t, filepath.Join("test1", "bin"))
		procRoot2, _ := createFakeProc(t, filepath.Join("test2", "bin"))
		monitor := newIstioTestMonitor(t)
		registerRecorder := new(utils.CallbackRecorder)

		// Setup test callbacks
		monitor.registerCB = registerRecorder.Callback()
		monitor.unregisterCB = utils.IgnoreCB

		// Calling sync multiple times shouldn't matter.
		// Once all envoy process are registered, calling it again shouldn't
		// trigger additional callback executions
		monitor.sync()
		monitor.sync()
		monitor.sync()

		pathID1, err := utils.NewPathIdentifier(filepath.Join(procRoot1, "test1", "bin", "envoy"))
		require.NoError(t, err)

		pathID2, err := utils.NewPathIdentifier(filepath.Join(procRoot2, "test2", "bin", "envoy"))
		require.NoError(t, err)

		// Each PathID should have triggered a callback exactly once
		assert.Equal(t, 2, registerRecorder.TotalCalls())
		assert.Equal(t, 1, registerRecorder.CallsForPathID(pathID1))
		assert.Equal(t, 1, registerRecorder.CallsForPathID(pathID2))
	})
}

// createFakeProc creates a fake envoy process in the given temporary directory.
// returns the temporary directory and the PID of the fake envoy process.
func createFakeProc(t *testing.T, path string) (procRoot string, pid int) {
	procRoot = t.TempDir()
	binDir := filepath.Join(procRoot, path)
	err := os.MkdirAll(binDir, os.ModePerm)
	require.NoError(t, err)
	fakeEnvoyPath := filepath.Join(binDir, "envoy")

	// we are using the `yes` command as a fake envoy binary.
	require.NoError(t, exec.Command("cp", "/usr/bin/yes", fakeEnvoyPath).Run())

	cmd := exec.Command(fakeEnvoyPath)
	require.NoError(t, cmd.Start())

	// Schedule process termination after the test
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
	})

	return procRoot, cmd.Process.Pid
}

func newIstioTestMonitor(t *testing.T) *istioMonitor {
	cfg := config.New()
	cfg.EnableIstioMonitoring = true

	monitor := newIstioMonitor(cfg, nil)
	require.NotNil(t, monitor)

	return monitor
}
