// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package executor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/stretchr/testify/require"
)

// serveExecutor stands up a real executor gRPC server on a short-lived local
// socket and returns its path. The server is torn down when the test ends.
func serveExecutor(t *testing.T, handler Handler) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "pe")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "s")

	server := NewServer(handler, "test-version", time.Minute, "", nil)
	listener, err := Listen(socket)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = server.Serve(ctx, listener) }()
	return socket
}

// newTestSubprocessExecutor builds a subprocessExecutor that talks to an
// already-running server (noAutoStart) so tests don't spawn a real binary.
func newTestSubprocessExecutor(socket string) Executor {
	sup := NewSupervisor(socket, "", nil, 1, "")
	sup.noAutoStart = true
	return newSubprocessExecutor(sup)
}

func TestSubprocessExecutorStopDrainsActiveTask(t *testing.T) {
	handler := newBlockingHandler()
	socket := serveExecutor(t, handler)
	exec := newTestSubprocessExecutor(socket)

	require.NoError(t, exec.SubmitTask(context.Background(), &types.Task{Raw: sampleTaskJSON()}))
	select {
	case <-handler.started:
	case <-time.After(time.Second):
		t.Fatal("task handler was not called")
	}

	stopped := make(chan struct{})
	go func() {
		_ = exec.Stop(context.Background())
		close(stopped)
	}()

	// Stop must not return while the task is still running.
	select {
	case <-stopped:
		t.Fatal("Stop returned before the active task finished draining")
	case <-time.After(50 * time.Millisecond):
	}

	close(handler.release)

	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return after the task finished")
	}
}

func TestSubprocessExecutorStopIsBoundedByContext(t *testing.T) {
	handler := newBlockingHandler() // never released: the task stays in-flight
	socket := serveExecutor(t, handler)
	exec := newTestSubprocessExecutor(socket)

	require.NoError(t, exec.SubmitTask(context.Background(), &types.Task{Raw: sampleTaskJSON()}))
	select {
	case <-handler.started:
	case <-time.After(time.Second):
		t.Fatal("task handler was not called")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	stopped := make(chan struct{})
	go func() {
		_ = exec.Stop(ctx)
		close(stopped)
	}()

	// A task that never finishes must not block Stop past the deadline.
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop hung on a task that never drained instead of honoring the deadline")
	}
}
