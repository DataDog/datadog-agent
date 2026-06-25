// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package runners

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// Executor is the seam between the orchestrator (OPMS polling, concurrency,
// heartbeat, publish) and the code that actually runs a task (signature
// verification, credential resolution, action execution).
//
// The in-process implementation forwards directly to a TaskHandler. A
// future out-of-process implementation forwards over IPC to a child
// executor process; the contract is the same.
type Executor interface {
	// Prepare runs once before any Execute call. In-process: starts the
	// keys manager and blocks until keys are ready. Other implementations
	// may use this to dial a child process or warm any required state.
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

// InProcessExecutor runs tasks in the same process by delegating directly
// to a TaskHandler. No IPC, no goroutine pool — the orchestrator's per-task
// goroutine drives Execute synchronously.
type InProcessExecutor struct {
	handler *TaskHandler
}

// NewInProcessExecutor returns an Executor backed by the given TaskHandler.
func NewInProcessExecutor(handler *TaskHandler) *InProcessExecutor {
	return &InProcessExecutor{handler: handler}
}

// Prepare delegates to the underlying TaskHandler.
func (e *InProcessExecutor) Prepare(ctx context.Context) error {
	return e.handler.Prepare(ctx)
}

// Execute delegates to the underlying TaskHandler.
func (e *InProcessExecutor) Execute(ctx context.Context, task *types.Task) (interface{}, error) {
	return e.handler.Execute(ctx, task)
}

// Stop is a no-op for in-process; there are no executor-side resources
// beyond what TaskHandler holds (which is process-lifetime).
func (e *InProcessExecutor) Stop(_ context.Context) error {
	return nil
}
