// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package runners

import (
	"context"
	"fmt"
	"time"

	privatebundles "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/credentials"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/remoteconfig"
	taskverifier "github.com/DataDog/datadog-agent/pkg/privateactionrunner/task-verifier"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/utils"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/errorcode"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// WorkflowRunner executes workflows and manages task execution.
type WorkflowRunner struct {
	registry     *privatebundles.Registry
	opmsClient   opms.Client
	resolver     credentials.PrivateCredentialResolver
	config       *config.Config
	taskVerifier *taskverifier.TaskVerifier
	keysManager  remoteconfig.KeysManager
	taskLoop     *Loop
}

// NewWorkflowRunner creates a new WorkflowRunner instance.
func NewWorkflowRunner(configuration *config.Config, keysManager remoteconfig.KeysManager, verifier *taskverifier.TaskVerifier, opmsClient opms.Client) *WorkflowRunner {

	return &WorkflowRunner{
		registry:     privatebundles.NewRegistry(configuration),
		opmsClient:   opmsClient,
		resolver:     credentials.NewPrivateCredentialResolver(),
		config:       configuration,
		keysManager:  keysManager,
		taskVerifier: verifier,
	}
}

// Start begins the workflow runner execution.
func (n *WorkflowRunner) Start(ctx context.Context) {
	if n.taskLoop != nil {
		log.Warn("WorkflowRunner already started")
		return
	}
	n.taskLoop = NewLoop(n)
	go func() {
		log.Info("Waiting for KeysManager to be ready")
		n.keysManager.WaitForReady()
		n.taskLoop.Run(ctx)
	}()
}

// Close stops the workflow runner and cleans up resources.
func (n *WorkflowRunner) Close(ctx context.Context) {
	if n.taskLoop != nil {
		n.taskLoop.Close(ctx)
	}
}

// RunTask executes a specific task.
func (n *WorkflowRunner) RunTask(
	ctx context.Context,
	task *types.Task,
	credential interface{},
) (interface{}, error) {
	fqn := task.GetFQN()
	bundleName, actionName := utils.SplitFQN(fqn)
	bundle := n.registry.GetBundle(bundleName)
	if bundle == nil {
		return nil, utils.NewPARError(
			errorcode.ActionPlatformErrorCode_INTERNAL_ERROR,
			fmt.Errorf("could not find bundle for %s", bundleName),
		)
	}
	action := bundle.GetAction(actionName)
	if action == nil {
		return nil, utils.NewPARError(
			errorcode.ActionPlatformErrorCode_INTERNAL_ERROR,
			fmt.Errorf("could not find action for %s", actionName),
		)
	}
	// TODO check action allowlist and URL allowlist for http

	heartbeatCtx, heartbeatCancel := context.WithCancel(ctx)
	defer heartbeatCancel()
	go n.startHeartbeat(heartbeatCtx, task)

	output, err := action.Run(ctx, actionName, task, credential)

	if err != nil {
		return nil, utils.DefaultActionError(err)
	}

	return output, nil
}

func (n *WorkflowRunner) startHeartbeat(ctx context.Context, task *types.Task) {
	ticker := time.NewTicker(n.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Infof("Heartbeat stopped for task %s", task.Data.ID)
			return
		case <-ticker.C:
			err := n.opmsClient.Heartbeat(ctx, task.Data.ID, task.GetFQN(), task.Data.Attributes.JobID)

			if err != nil {
				log.Errorf("Failed to send heartbeat %v", err)
			} else {
				log.Info("Heartbeat sent successfully")
			}
		}
	}
}
