// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetEnvoyPath(t *testing.T) {
	_ = createFakeProcFS(t)
	monitor := newIstioTestMonitor(t)

	t.Run("an actual envoy process", func(t *testing.T) {
		path := monitor.getEnvoyPath(uint32(1))
		assert.Equal(t, "/usr/local/bin/envoy", path)
	})
	t.Run("something else", func(t *testing.T) {
		path := monitor.getEnvoyPath(uint32(2))
		assert.Empty(t, "", path)
	})
}

func TestIstioSync(t *testing.T) {
	t.Run("calling sync for the first time", func(t *testing.T) {
		procRoot := createFakeProcFS(t)
		monitor := newIstioTestMonitor(t)
		registerRecorder := new(utils.CallbackRecorder)

		// Setup test callbacks
		monitor.registerCB = registerRecorder.Callback()
		monitor.unregisterCB = utils.IgnoreCB

		// Calling sync should detect the two envoy processes
		monitor.sync()

		pathID1, err := utils.NewPathIdentifier(filepath.Join(procRoot, "1/root/usr/local/bin/envoy"))
		require.NoError(t, err)

		pathID2, err := utils.NewPathIdentifier(filepath.Join(procRoot, "3/root/usr/local/bin/envoy"))
		require.NoError(t, err)

		assert.Equal(t, 2, registerRecorder.TotalCalls())
		assert.Equal(t, 1, registerRecorder.CallsForPathID(pathID1))
		assert.Equal(t, 1, registerRecorder.CallsForPathID(pathID2))
	})

	t.Run("calling sync multiple times", func(t *testing.T) {
		procRoot := createFakeProcFS(t)
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

		pathID1, err := utils.NewPathIdentifier(filepath.Join(procRoot, "1/root/usr/local/bin/envoy"))
		require.NoError(t, err)

		pathID2, err := utils.NewPathIdentifier(filepath.Join(procRoot, "3/root/usr/local/bin/envoy"))
		require.NoError(t, err)

		// Each PathID should have triggered a callback exactly once
		assert.Equal(t, 2, registerRecorder.TotalCalls())
		assert.Equal(t, 1, registerRecorder.CallsForPathID(pathID1))
		assert.Equal(t, 1, registerRecorder.CallsForPathID(pathID2))
	})

	t.Run("detecting a dangling process", func(t *testing.T) {
		procRoot := createFakeProcFS(t)
		monitor := newIstioTestMonitor(t)
		registerRecorder := new(utils.CallbackRecorder)
		unregisterRecorder := new(utils.CallbackRecorder)

		// Setup test callbacks
		monitor.registerCB = registerRecorder.Callback()
		monitor.unregisterCB = unregisterRecorder.Callback()

		monitor.sync()

		// The first call to sync() will start tracing PIDs 1 and 3, but not PID 2
		assert.Contains(t, monitor.registry.GetRegisteredProcesses(), uint32(1))
		assert.NotContains(t, monitor.registry.GetRegisteredProcesses(), uint32(2))
		assert.Contains(t, monitor.registry.GetRegisteredProcesses(), uint32(3))

		// At this point we should have received:
		// * 2 register calls
		// * 0 unregister calls
		assert.Equal(t, 2, registerRecorder.TotalCalls())
		assert.Equal(t, 0, unregisterRecorder.TotalCalls())

		// Now we emulate a process termination for PID 3 by removing it from the fake
		// procFS tree
		require.NoError(t, os.RemoveAll(filepath.Join(procRoot, "3")))

		// Once we call sync() again, PID 3 termination should be detected
		// and the unregister callback should be executed
		monitor.sync()
		assert.Equal(t, 1, unregisterRecorder.TotalCalls())
		assert.NotContains(t, monitor.registry.GetRegisteredProcesses(), uint32(3))
	})
}

// This creates a bare-bones procFS with a structure that looks like
// the following:
//
// proc/
// ├── 1
// │   ├── cmdline
// │   └── root
// │       └── usr
// │           └── local
// │               └── bin
// │                   └── envoy
// ...
//
// This ProcFS contains 3 PIDs:
//
// PID 1 -> Envoy process
// PID 2 -> Bash process
// PID 3 -> Envoy process
func createFakeProcFS(t *testing.T) (procRoot string) {
	procRoot = t.TempDir()

	// Inject fake ProcFS path
	previousFn := kernel.ProcFSRoot
	kernel.ProcFSRoot = func() string { return procRoot }
	t.Cleanup(func() {
		kernel.ProcFSRoot = previousFn
	})

	// Taken from a real istio-proxy container
	const envoyCmdline = "/usr/local/bin/envoy" +
		"-cetc/istio/proxy/envoy-rev.json" +
		"--drain-time-s45" +
		"--drain-strategyimmediate" +
		"--local-address-ip-versionv4" +
		"--file-flush-interval-msec1000" +
		"--disable-hot-restart" +
		"--allow-unknown-static-fields" +
		"--log-format"

	// PID 1
	createFile(t,
		filepath.Join(procRoot, "1", "cmdline"),
		envoyCmdline,
	)
	createFile(t,
		filepath.Join(procRoot, "1", "root/usr/local/bin/envoy"),
		"",
	)

	// PID 2
	createFile(t,
		filepath.Join(procRoot, "2", "cmdline"),
		"/bin/bash",
	)

	// PID 3
	createFile(t,
		filepath.Join(procRoot, "3", "cmdline"),
		envoyCmdline,
	)
	createFile(t,
		filepath.Join(procRoot, "3", "root/usr/local/bin/envoy"),
		"",
	)

	return
}

func createFile(t *testing.T, path, data string) {
	dir := filepath.Dir(path)
	require.NoError(t, os.MkdirAll(dir, 0775))
	require.NoError(t, os.WriteFile(path, []byte(data), 0775))
}

func newIstioTestMonitor(t *testing.T) *istioMonitor {
	cfg := config.New()
	cfg.EnableIstioMonitoring = true

	monitor := newIstioMonitor(cfg, nil)
	require.NotNil(t, monitor)

	return monitor
}
