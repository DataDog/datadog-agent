// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package file

// Tests for the process_log symlink-rejection fix (logs-tail-symlink-escape).
//
// The process_log provider discovers log file paths from /proc/<pid>/fd/<n>.
// Linux resolves all symlinks in the path at open time, so the string stored in
// proc(5) is already canonical — no directory component is a symlink.  Any
// symlink that appears later (on the final file or a directory component) was
// planted after discovery and indicates an attacker-controlled swap.  The tailer
// must reject such paths.

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
	"github.com/DataDog/datadog-agent/pkg/logs/util/opener"
)

// makeProcessLogTailer creates a Tailer whose source has ProcessLog set to the
// provided value.  The tailer is not started; call t.Start/Stop explicitly.
func makeProcessLogTailer(t *testing.T, path string, processLog bool) *Tailer {
	t.Helper()
	configmock.New(t)

	cfg := &config.LogsConfig{
		Type:       config.FileType,
		Path:       path,
		ProcessLog: processLog,
	}

	logSource := sources.NewLogSource("test", cfg)
	replSource := sources.NewReplaceableSource(logSource)
	f := NewFile(path, logSource, false)

	info := status.NewInfoRegistry()
	opts := &TailerOptions{
		OutputChan:      make(chan *message.Message, 100),
		File:            f,
		SleepDuration:   10 * time.Millisecond,
		Decoder:         decoder.NewDecoderFromSource(replSource, info),
		Info:            info,
		CapacityMonitor: metrics.NewNoopPipelineMonitor("").GetCapacityMonitor("", ""),
		Registry:        auditor.NewMockRegistry(),
		FileOpener:      opener.NewFileOpener(),
	}

	return NewTailer(opts)
}

// TestTailerProcessLogSymlinkPolicyWiring verifies that NewTailer derives the
// correct symlink policy from the source config's ProcessLog field.
func TestTailerProcessLogSymlinkPolicyWiring(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	require.NoError(t, os.WriteFile(path, []byte("hello\n"), 0644))

	t.Run("ProcessLog=true uses RejectSymlinks", func(t *testing.T) {
		tailer := makeProcessLogTailer(t, path, true)
		assert.Equal(t, opener.RejectSymlinks, tailer.symlinkPolicy)
	})

	t.Run("ProcessLog=false uses FollowSymlinks", func(t *testing.T) {
		tailer := makeProcessLogTailer(t, path, false)
		assert.Equal(t, opener.FollowSymlinks, tailer.symlinkPolicy)
	})
}

// TestTailerProcessLogRejectsSymlinkSwap is the keystone security test.
// It builds a tailer from a ProcessLog=true source on a real file, confirms the
// initial open succeeds, swaps the file for a symlink, and confirms the
// subsequent open (via Start on a new tailer) fails — while a non-ProcessLog
// tailer on the same symlinked path succeeds.
func TestTailerProcessLogRejectsSymlinkSwap(t *testing.T) {
	dir := t.TempDir()

	// Canonical file that process_log would discover
	logFile := filepath.Join(dir, "app.log")
	require.NoError(t, os.WriteFile(logFile, []byte("line1\n"), 0644))

	// Sensitive file the attacker wants to exfiltrate
	sensitiveFile := filepath.Join(dir, "secret.dat")
	require.NoError(t, os.WriteFile(sensitiveFile, []byte("SECRET"), 0644))

	t.Run("initial open on real file succeeds", func(t *testing.T) {
		tailer := makeProcessLogTailer(t, logFile, true)
		err := tailer.Start(0, io.SeekStart)
		require.NoError(t, err, "expected ProcessLog tailer to open real file successfully")
		tailer.Stop()
	})

	// Attacker replaces the discovered path with a symlink to the sensitive file
	require.NoError(t, os.Remove(logFile))
	require.NoError(t, os.Symlink(sensitiveFile, logFile))

	t.Run("process_log tailer rejects symlink", func(t *testing.T) {
		tailer := makeProcessLogTailer(t, logFile, true)
		err := tailer.Start(0, io.SeekStart)
		assert.Error(t, err, "expected ProcessLog tailer to reject symlinked path")
	})

	t.Run("non-process_log tailer follows symlink (admin-configured paths may be symlinks)", func(t *testing.T) {
		tailer := makeProcessLogTailer(t, logFile, false)
		err := tailer.Start(0, io.SeekStart)
		require.NoError(t, err, "expected non-ProcessLog tailer to follow symlink as before")
		tailer.Stop()
	})
}

// TestTailerProcessLogDidRotateRejectsSymlinkSwap verifies that the rotation
// re-open path (DidRotate) also rejects a symlink swap, not only the initial open.
func TestTailerProcessLogDidRotateRejectsSymlinkSwap(t *testing.T) {
	dir := t.TempDir()

	logFile := filepath.Join(dir, "app.log")
	require.NoError(t, os.WriteFile(logFile, []byte("line1\n"), 0644))

	sensitiveFile := filepath.Join(dir, "secret.dat")
	require.NoError(t, os.WriteFile(sensitiveFile, []byte("SECRET"), 0644))

	// Start the tailer on the real file
	tailer := makeProcessLogTailer(t, logFile, true)
	err := tailer.Start(0, io.SeekStart)
	require.NoError(t, err)

	// Swap the file for a symlink (the attack)
	require.NoError(t, os.Remove(logFile))
	require.NoError(t, os.Symlink(sensitiveFile, logFile))

	// DidRotate re-opens the path; it must fail, not follow the symlink
	didRotate, err := tailer.DidRotate()
	assert.Error(t, err, "expected DidRotate to return error when path became a symlink")
	assert.False(t, didRotate)

	tailer.Stop()
}
