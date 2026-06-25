// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package executor

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/shared/types"
)

// InProcessExecutor runs tasks in the same process by delegating directly
// to a TaskHandler. No IPC, no goroutine pool — the orchestrator's
// per-task goroutine drives Execute synchronously.
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
