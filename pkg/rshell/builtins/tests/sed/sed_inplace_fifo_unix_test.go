// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build unix

package sed_test

import (
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInPlaceRejectsFIFOTarget is an end-to-end companion to the
// allowedpaths-level TestWriteRegularFileRejectsFIFO* and
// TestOpenRegularRejectsFIFO* tests: it verifies that the full `sed -i`
// command refuses a FIFO target promptly instead of blocking on it.
//
// readAllBounded (engine.go) now opens the source through
// callCtx.OpenRegularFile, which rejects a non-regular target via a
// non-blocking Stat before ever attempting to open it for reading (see
// Sandbox.openRegular) — so the FIFO is rejected at the *read* stage here,
// before any writer would need to be involved at all. A background writer
// that never gets read still has its own blocking O_WRONLY open of the FIFO
// unblocked here (a reader connecting is what a blocking write-open of a
// FIFO waits for, per FIFO semantics — nothing in rshell reads from this
// FIFO once the target is rejected before ever being opened), so the test
// opens and immediately closes the read end itself after the assertions to
// release that goroutine deterministically rather than leaking it.
func TestInPlaceRejectsFIFOTarget(t *testing.T) {
	dir := t.TempDir()
	fifoPath := filepath.Join(dir, "pipe")
	require.NoError(t, syscall.Mkfifo(fifoPath, 0600))

	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		w, err := os.OpenFile(fifoPath, os.O_WRONLY, 0)
		if err != nil {
			return
		}
		_, _ = w.WriteString("a\n")
		_ = w.Close()
	}()

	_, stderr, code := runScript(t, `sed -i 's/a/b/' pipe`, dir,
		interp.AllowedPaths([]string{dir + ":rw"}),
		interp.WithMode(interp.ModeRemediation),
	)
	assert.Equal(t, 1, code, "sed -i on a FIFO must fail at read time, not hang")
	assert.Contains(t, stderr, "not a regular file")

	// The FIFO itself must still exist and be unharmed — sed -i must not
	// have attempted to remove or replace it.
	info, err := os.Lstat(fifoPath)
	require.NoError(t, err)
	assert.True(t, info.Mode()&os.ModeNamedPipe != 0, "pipe must still be a FIFO")

	// Release the background writer, which is still blocked in its own
	// O_WRONLY open waiting for a reader (sed -i never became one, since the
	// target was rejected before any read was attempted).
	r, err := os.OpenFile(fifoPath, os.O_RDONLY, 0)
	require.NoError(t, err)
	_, _ = io.Copy(io.Discard, r)
	_ = r.Close()
	<-writerDone
}
