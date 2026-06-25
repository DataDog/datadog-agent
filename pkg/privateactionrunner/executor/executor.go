// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package executor exposes the seam between the orchestrator (OPMS
// polling, concurrency, heartbeat, publish) and the code that actually
// runs a task (signature verification, credential resolution, action
// execution).
//
// The seam itself — the Executor interface, the Mode constants, the
// TaskHandler that runs a task in-process, and the IPC + transport
// helpers used by the binary-mode implementation — lives in this
// package. The three implementations live in subpackages so each
// process flavor only pulls in the code it actually uses:
//
//   - inproc  — InProcessExecutor (direct Go call to TaskHandler)
//   - binary  — BinaryExecutor (orchestrator-side gRPC client + spawn)
//   - server  — gRPC Server (the executor-side service handler)
//
// The orchestrator (comp/privateactionrunner) selects the appropriate
// implementation at startup based on the configured Mode.
package executor

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/shared/types"
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

// Mode names the executor implementation the orchestrator should
// construct.
const (
	// ModeInProcess (default) runs tasks directly in the orchestrator
	// process via a TaskHandler call.
	ModeInProcess = "in-process"
	// ModeBinary spawns a child process and forwards each task to it
	// over a local gRPC socket.
	ModeBinary = "binary"
)
