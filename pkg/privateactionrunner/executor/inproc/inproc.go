// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package inproc holds the in-process executor implementation: it
// forwards Prepare / Execute / Stop directly to a TaskHandler with no
// IPC or marshaling.
package inproc

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/executor"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/shared/types"
)

// Executor runs tasks in the same process by delegating directly to a
// TaskHandler. No IPC, no goroutine pool — the orchestrator's per-task
// goroutine drives Execute synchronously.
type Executor struct {
	handler *executor.TaskHandler
}

// New returns an in-process executor backed by the given TaskHandler.
// The returned *Executor satisfies executor.Executor.
func New(handler *executor.TaskHandler) *Executor {
	return &Executor{handler: handler}
}

// Prepare delegates to the underlying TaskHandler.
func (e *Executor) Prepare(ctx context.Context) error {
	return e.handler.Prepare(ctx)
}

// Execute delegates to the underlying TaskHandler.
func (e *Executor) Execute(ctx context.Context, task *types.Task) (interface{}, error) {
	return e.handler.Execute(ctx, task)
}

// Stop is a no-op for in-process; the only resource the executor holds
// is the TaskHandler, whose lifetime matches the process.
func (e *Executor) Stop(_ context.Context) error {
	return nil
}
