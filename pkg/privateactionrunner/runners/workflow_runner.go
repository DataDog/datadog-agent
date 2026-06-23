// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package runners

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/executor"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
)

// WorkflowRunner owns OPMS polling and hands dequeued tasks to an executor.
type WorkflowRunner struct {
	opmsClient opms.Client
	config     *config.Config
	executor   executor.Executor
	taskLoop   *Loop
}

func NewWorkflowRunner(
	configuration *config.Config,
	opmsClient opms.Client,
	taskExecutor executor.Executor,
) (*WorkflowRunner, error) {
	if taskExecutor == nil {
		return nil, fmt.Errorf("workflow runner requires an executor")
	}
	return &WorkflowRunner{
		opmsClient: opmsClient,
		config:     configuration,
		executor:   taskExecutor,
	}, nil
}

func (n *WorkflowRunner) Start(ctx context.Context) error {
	log.FromContext(ctx).Info("Starting Workflow runner")
	if n.taskLoop != nil {
		log.FromContext(ctx).Warn("WorkflowRunner already started")
		return nil
	}
	n.taskLoop = NewLoop(n)
	go n.taskLoop.Run(ctx)
	return nil
}

func (n *WorkflowRunner) Stop(ctx context.Context) error {
	log.FromContext(ctx).Info("Stopping Workflow runner")

	if n.taskLoop != nil {
		n.taskLoop.Close(ctx)
	}
	return nil
}
