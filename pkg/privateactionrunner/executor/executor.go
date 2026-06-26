// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package executor exposes the seam between the orchestrator (OPMS
// polling, concurrency, heartbeat, publish) and the code that actually
// runs a task (signature verification, credential resolution, action
// execution). The in-process implementation calls a TaskHandler
// directly; the binary implementation forwards over a local gRPC socket
// to a child executor process.
package executor

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// Mode names the executor implementation NewExecutor builds.
const (
	// ModeInProcess (default) runs tasks directly in the orchestrator
	// process via a TaskHandler call.
	ModeInProcess = "in-process"
	// ModeBinary spawns a child process and forwards each task to it
	// over a local gRPC socket.
	ModeBinary = "binary"
)

// Executor is the seam between the orchestrator and the per-task compute.
type Executor interface {
	// Prepare runs once before any Execute call. In-process: starts the
	// keys manager and blocks until keys are ready. Binary: dials the
	// child (which does its own key bootstrap on first request).
	Prepare(ctx context.Context) error
	// Execute runs the dequeued task end-to-end inside the executor and
	// returns the action's output (caller-defined Go value) or an error.
	// The orchestrator does not interpret output; it forwards to
	// opmsClient.PublishSuccess.
	Execute(ctx context.Context, task *types.Task) (output interface{}, err error)
	// Stop releases executor-side resources. The orchestrator has
	// already drained its in-flight tasks before calling this.
	Stop(ctx context.Context) error
}

// Params configures which Executor implementation NewExecutor builds and
// the parameters of the IPC transport when running in binary mode.
type Params struct {
	// Mode is one of ModeInProcess or ModeBinary. Empty defaults to ModeInProcess.
	Mode string
	// Handler is the in-process TaskHandler. Required for ModeInProcess.
	// Binary mode ignores it — the child has its own TaskHandler.
	Handler *TaskHandler
	// SocketPath is the local socket or named pipe used for IPC. Only
	// applies to ModeBinary. Empty defers to defaultSocketPath().
	SocketPath string
	// AuthToken authenticates IPC requests between the orchestrator and
	// child. Empty generates a random token at startup.
	AuthToken string
	// DrainTimeout bounds how long Stop waits for a clean teardown of
	// the child before sending SIGKILL.
	DrainTimeout time.Duration
	// ConfPath and ExtraConfFiles are forwarded to the executor
	// subprocess via flags (ModeBinary only).
	ConfPath       string
	ExtraConfFiles []string
}

// NewExecutor builds the Executor implementation selected by p.Mode.
func NewExecutor(p Params) (Executor, error) {
	switch p.Mode {
	case ModeInProcess, "":
		if p.Handler == nil {
			return nil, fmt.Errorf("executor mode %q requires a non-nil Handler", ModeInProcess)
		}
		return NewInProcessExecutor(p.Handler), nil
	case ModeBinary:
		return newBinaryExecutor(p)
	default:
		return nil, fmt.Errorf("unknown executor mode %q (want %q or %q)", p.Mode, ModeInProcess, ModeBinary)
	}
}
