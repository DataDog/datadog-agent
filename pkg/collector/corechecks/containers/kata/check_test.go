// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && test

package kata

import (
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
)

const fixtureMetrics = `# HELP kata_hypervisor_fds open file descriptors for hypervisor
# TYPE kata_hypervisor_fds gauge
kata_hypervisor_fds{sandbox_id="abc123"} 42
# HELP kata_shim_io_wait_duration_secs io wait duration
# TYPE kata_shim_io_wait_duration_secs counter
kata_shim_io_wait_duration_secs{sandbox_id="abc123"} 1.5
# HELP kata_go_goroutines goroutine count
# TYPE kata_go_goroutines gauge
kata_go_goroutines{version="1.21",sandbox_id="abc123"} 10
`

func TestDiscoverSandboxes(t *testing.T) {
	tmpDir := t.TempDir()

	// Create two sandbox dirs with shim sockets
	sandbox1 := filepath.Join(tmpDir, "sandbox-aaa")
	sandbox2 := filepath.Join(tmpDir, "sandbox-bbb")
	require.NoError(t, os.MkdirAll(sandbox1, 0755))
	require.NoError(t, os.MkdirAll(sandbox2, 0755))

	sock1 := filepath.Join(sandbox1, shimSocket)
	sock2 := filepath.Join(sandbox2, shimSocket)
	require.NoError(t, os.WriteFile(sock1, []byte{}, 0600))
	require.NoError(t, os.WriteFile(sock2, []byte{}, 0600))

	// Create a dir without a socket (should be ignored)
	noSocket := filepath.Join(tmpDir, "no-socket-dir")
	require.NoError(t, os.MkdirAll(noSocket, 0755))

	c := &KataCheck{
		instance: &KataConfig{
			SandboxStoragePaths: []string{tmpDir},
		},
	}

	sandboxes := c.discoverSandboxes()

	assert.Len(t, sandboxes, 2)
	assert.Equal(t, sock1, sandboxes["sandbox-aaa"])
	assert.Equal(t, sock2, sandboxes["sandbox-bbb"])
	assert.NotContains(t, sandboxes, "no-socket-dir")
}

func TestDiscoverSandboxes_NonExistentPath(t *testing.T) {
	c := &KataCheck{
		instance: &KataConfig{
			SandboxStoragePaths: []string{"/nonexistent/path/that/does/not/exist"},
		},
	}

	sandboxes := c.discoverSandboxes()
	assert.Empty(t, sandboxes)
}

func TestFormatMetricName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"kata_hypervisor_fds", "kata.hypervisor.fds"},
		{"kata_shim_io_wait_duration_secs", "kata.shim.io.wait.duration.secs"},
		{"some_metric", "kata.some.metric"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, formatMetricName(tt.input))
		})
	}
}

func TestScrapeSandbox_MetricCollection(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "shim-monitor.sock")

	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	defer listener.Close()

	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain; version=0.0.4")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(fixtureMetrics))
		}),
	}
	go server.Serve(listener) //nolint:errcheck
	defer server.Close()

	ms := mocksender.NewMockSender("kata_containers")
	ms.SetupAcceptAll()

	c := &KataCheck{
		instance: &KataConfig{
			RenameLabels: map[string]string{"version": "go_version"},
		},
	}

	baseTags := []string{"sandbox_id:abc123"}
	c.scrapeSandbox(ms, "abc123", socketPath, baseTags)

	// Gauge for kata_hypervisor_fds
	ms.AssertMetricTaggedWith(t, "Gauge", "kata.hypervisor.fds", []string{"sandbox_id:abc123"})

	// Rate for kata_shim_io_wait_duration_secs (COUNTER)
	ms.AssertMetricTaggedWith(t, "Rate", "kata.shim.io.wait.duration.secs", []string{"sandbox_id:abc123"})

	// Label rename: version → go_version
	ms.AssertMetricTaggedWith(t, "Gauge", "kata.go.goroutines", []string{"sandbox_id:abc123", "go_version:1.21"})

	// OK service check
	ms.AssertServiceCheck(t, "kata.openmetrics.health", servicecheck.ServiceCheckOK, "", []string{"sandbox_id:abc123"}, "")
}

func TestScrapeSandbox_ConnectionFailure(t *testing.T) {
	ms := mocksender.NewMockSender("kata_containers")
	ms.SetupAcceptAll()

	c := &KataCheck{
		instance: &KataConfig{},
	}

	// Non-existent socket path → connection failure
	c.scrapeSandbox(ms, "deadbeef", "/nonexistent/shim-monitor.sock", []string{"sandbox_id:deadbeef"})

	ms.Mock.AssertCalled(t, "ServiceCheck", "kata.openmetrics.health", servicecheck.ServiceCheckCritical, "", mocksender.MatchTagsContains([]string{"sandbox_id:deadbeef"}), mock.AnythingOfType("string"))
}

func TestRunningShimCount(t *testing.T) {
	tmpDir := t.TempDir()

	for _, name := range []string{"sb1", "sb2", "sb3"} {
		dir := filepath.Join(tmpDir, name)
		require.NoError(t, os.MkdirAll(dir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, shimSocket), []byte{}, 0600))
	}

	c := &KataCheck{
		instance: &KataConfig{
			SandboxStoragePaths: []string{tmpDir},
			RenameLabels:        map[string]string{},
		},
	}

	sandboxes := c.discoverSandboxes()
	assert.Len(t, sandboxes, 3)
}

func TestBuildSampleTags_ExcludeLabels(t *testing.T) {
	c := &KataCheck{
		instance: &KataConfig{
			RenameLabels:  map[string]string{},
			ExcludeLabels: []string{"job"},
		},
		excludeSet: map[string]struct{}{"job": {}},
	}

	metric := map[string]string{
		"sandbox_id": "aaa",
		"job":        "kata",
		"env":        "prod",
	}

	baseTags := []string{"sandbox_id:sandbox-001"}
	tags := c.buildSampleTags(baseTags, metric)

	assert.Contains(t, tags, "sandbox_id:sandbox-001")
	assert.Contains(t, tags, "env:prod")
	for _, tag := range tags {
		assert.NotContains(t, tag, "job:")
	}
}

func TestProcessContainerEvents_MultiContainerPodAndUnset(t *testing.T) {
	c := &KataCheck{
		instance:            &KataConfig{},
		sandboxContainerIDs: make(map[string]map[string]struct{}),
		containerSandboxID:  make(map[string]string),
	}

	sandboxID := "pod-sandbox-abc"

	// Simulate SET events for two containers in the same pod sandbox.
	setEvent := func(containerID string) workloadmeta.Event {
		return workloadmeta.Event{
			Type: workloadmeta.EventTypeSet,
			Entity: &workloadmeta.Container{
				EntityID:  workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: containerID},
				SandboxID: sandboxID,
			},
		}
	}
	// Simulate UNSET events: SandboxID is empty, only EntityID is set.
	unsetEvent := func(containerID string) workloadmeta.Event {
		return workloadmeta.Event{
			Type: workloadmeta.EventTypeUnset,
			Entity: &workloadmeta.Container{
				EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: containerID},
			},
		}
	}

	bundle1 := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{setEvent("ctr-1"), setEvent("ctr-2")},
		Ch:     make(chan struct{}),
	}
	c.processContainerEvents(bundle1)

	// Both containers should be tracked under the same sandbox.
	c.mu.RLock()
	assert.Len(t, c.sandboxContainerIDs[sandboxID], 2)
	assert.Equal(t, sandboxID, c.containerSandboxID["ctr-1"])
	assert.Equal(t, sandboxID, c.containerSandboxID["ctr-2"])
	c.mu.RUnlock()

	// UNSET first container: sandbox entry should still exist (one container left).
	bundle2 := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{unsetEvent("ctr-1")},
		Ch:     make(chan struct{}),
	}
	c.processContainerEvents(bundle2)

	c.mu.RLock()
	assert.Len(t, c.sandboxContainerIDs[sandboxID], 1)
	assert.NotContains(t, c.containerSandboxID, "ctr-1")
	c.mu.RUnlock()

	// UNSET second container: sandbox entry should be fully removed.
	bundle3 := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{unsetEvent("ctr-2")},
		Ch:     make(chan struct{}),
	}
	c.processContainerEvents(bundle3)

	c.mu.RLock()
	assert.NotContains(t, c.sandboxContainerIDs, sandboxID)
	assert.NotContains(t, c.containerSandboxID, "ctr-2")
	c.mu.RUnlock()
}

func TestAssertServiceCheck_MessageContains(t *testing.T) {
	ms := mocksender.NewMockSender("kata_containers")
	ms.SetupAcceptAll()

	c := &KataCheck{
		instance: &KataConfig{},
	}

	c.scrapeSandbox(ms, "deadbeef", "/nonexistent/shim-monitor.sock", []string{"sandbox_id:deadbeef"})

	// Verify the service check message mentions the sandbox ID
	calls := ms.Calls
	found := false
	for _, call := range calls {
		if call.Method == "ServiceCheck" {
			msg, ok := call.Arguments[4].(string)
			if ok && len(msg) > 0 {
				found = true
				assert.Contains(t, msg, "deadbeef")
			}
		}
	}
	assert.True(t, found, "expected a ServiceCheck call with a non-empty message")
}
