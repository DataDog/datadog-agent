// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package runners implements entrypoint for privateactionrunner execution
package runners

import "context"

// WorkflowRunner executes workflows and manages task execution.
type WorkflowRunner struct {
}

// NewWorkflowRunner creates a new WorkflowRunner instance.
func NewWorkflowRunner() *WorkflowRunner {
	return &WorkflowRunner{}
}

// Start begins the workflow runner execution.
func (n *WorkflowRunner) Start(_ context.Context) error {
	return nil
}

// Close stops the workflow runner and cleans up resources.
func (n *WorkflowRunner) Close(_ context.Context) error {
	return nil
}
