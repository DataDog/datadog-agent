// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	defaultEnvoyName = "/bin/envoy"
)

func TestGetEnvoyPath(t *testing.T) {
	_, pid := createFakeProcess(t, defaultEnvoyName)
	monitor := newIstioTestMonitor(t)

	t.Run("an actual envoy process", func(t *testing.T) {
		path := monitor.getEnvoyPath(uint32(pid))
		assert.True(t, strings.HasSuffix(path, defaultEnvoyName))
	})
	t.Run("something else", func(t *testing.T) {
		path := monitor.getEnvoyPath(uint32(2))
		assert.Empty(t, "", path)
	})
}

func TestGetEnvoyPathWithConfig(t *testing.T) {
	cfg := config.New()
	cfg.EnableIstioMonitoring = true
	cfg.EnvoyPath = "/test/envoy"
	monitor := newIstioTestMonitorWithCFG(t, cfg)

	_, pid := createFakeProcess(t, cfg.EnvoyPath)

	path := monitor.getEnvoyPath(uint32(pid))
	assert.True(t, strings.HasSuffix(path, cfg.EnvoyPath))
}

func TestIstioSync(t *testing.T) {
	t.Run("calling sync multiple times", func(t *testing.T) {
		procRoot1, _ := createFakeProcess(t, filepath.Join("test1", defaultEnvoyName))
		procRoot2, _ := createFakeProcess(t, filepath.Join("test2", defaultEnvoyName))
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

		pathID1, err := utils.NewPathIdentifier(procRoot1)
		require.NoError(t, err)

		pathID2, err := utils.NewPathIdentifier(procRoot2)
		require.NoError(t, err)

		// Each PathID should have triggered a callback exactly once
		assert.Equal(t, 2, registerRecorder.TotalCalls())
		assert.Equal(t, 1, registerRecorder.CallsForPathID(pathID1))
		assert.Equal(t, 1, registerRecorder.CallsForPathID(pathID2))
	})
}

// createFakeProcess creates a fake process in a temporary location.
// returns the full path of the temporary process and the PID of the fake process.
func createFakeProcess(t *testing.T, processName string) (procRoot string, pid int) {
	fakePath := filepath.Join(t.TempDir(), processName)
	require.NoError(t, exec.Command("mkdir", "-p", filepath.Dir(fakePath)).Run())

	// we are using the `yes` command as a fake envoy binary.
	require.NoError(t, exec.Command("cp", "/usr/bin/yes", fakePath).Run())

	cmd := exec.Command(fakePath)
	require.NoError(t, cmd.Start())

	// Schedule process termination after the test
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
	})

	return fakePath, cmd.Process.Pid
}

func newIstioTestMonitor(t *testing.T) *istioMonitor {
	cfg := config.New()
	cfg.EnableIstioMonitoring = true

	return newIstioTestMonitorWithCFG(t, cfg)
}

func newIstioTestMonitorWithCFG(t *testing.T, cfg *config.Config) *istioMonitor {
	monitor := newIstioMonitor(cfg, nil)
	require.NotNil(t, monitor)
	return monitor
}
