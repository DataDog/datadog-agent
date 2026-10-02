// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package yara

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	eventtestutil "github.com/DataDog/datadog-agent/pkg/eventmonitor/testutil"
)

// TestExecScannerEventMonitor runs the whole scanner behind a real event monitor: it needs root
// and eBPF, and is skipped otherwise.
func TestExecScannerEventMonitor(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("the event monitor needs root")
	}

	// the rules and binaries are created by root, as in production
	rulesDir := t.TempDir()
	writeRuleFiles(t, rulesDir, map[string]string{"marker.yar": testRule})

	recorder := &recordingReporter{}
	eventtestutil.StartEventMonitor(t, func(t testing.TB, evm *eventmonitor.EventMonitor) {
		p, err := NewPipeline(testPipelineConfig(rulesDir), evm.StatsdClient, PipelineOpts{
			Compiler:      StandInCompiler,
			ContainerPIDs: containerPIDsFromEventMonitor(evm),
			ExtraReporter: recorder,
		})
		require.NoError(t, err)
		require.NoError(t, p.Consumer().register(evm))
	})

	dir := t.TempDir()
	// an interpreter exec: the script file is scanned
	script := writeTempFile(t, dir, "evil.sh", []byte("#!/bin/sh\n# EVIL_MARKER\nexit 0\n"))
	for i := 0; i < 5; i++ {
		require.NoError(t, exec.Command(script).Run())
	}

	// a binary: a copy of /bin/true, with the marker appended (ELF ignores trailing bytes)
	trueBin, err := os.ReadFile("/bin/true")
	require.NoError(t, err)
	binary := writeTempFile(t, dir, "evil-true", append(trueBin, []byte("EVIL_MARKER")...))
	for i := 0; i < 5; i++ {
		require.NoError(t, exec.Command(binary).Run())
	}

	matchedPaths := func() map[string]int {
		res := make(map[string]int)
		for _, r := range recorder.all() {
			if len(r.matches) > 0 {
				res[filepath.Base(r.file.Path)]++
			}
		}
		return res
	}
	require.Eventually(t, func() bool {
		m := matchedPaths()
		return m["evil.sh"] >= 1 && m["evil-true"] >= 1
	}, 20*time.Second, 50*time.Millisecond)
	// each content is scanned once, however many times it runs
	time.Sleep(500 * time.Millisecond)
	assert.Equal(t, map[string]int{"evil.sh": 1, "evil-true": 1}, matchedPaths())
}
