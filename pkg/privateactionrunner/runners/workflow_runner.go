// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package runners

import (
	"context"
	"errors"
	"fmt"
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	traceroute "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/def"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/actions"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	privatebundles "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/credentials/resolver"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/observability"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	taskverifier "github.com/DataDog/datadog-agent/pkg/privateactionrunner/task-verifier"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	aperrorpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/errorcode"
)

type WorkflowRunner struct {
	registry     *privatebundles.Registry
	opmsClient   opms.Client
	resolver     resolver.PrivateCredentialResolver
	config       *config.Config
	keysManager  taskverifier.KeysManager
	taskVerifier *taskverifier.TaskVerifier
	taskLoop     *Loop
}

func NewWorkflowRunner(
	configuration *config.Config,
	keysManager taskverifier.KeysManager,
	verifier *taskverifier.TaskVerifier,
	opmsClient opms.Client,
	wmeta workloadmeta.Component,
	traceroute traceroute.Component,
	eventPlatform eventplatform.Component,
) (*WorkflowRunner, error) {
	return &WorkflowRunner{
		registry:     privatebundles.NewRegistry(configuration, wmeta, traceroute, eventPlatform),
		opmsClient:   opmsClient,
		resolver:     resolver.NewPrivateCredentialResolver(),
		config:       configuration,
		keysManager:  keysManager,
		taskVerifier: verifier,
	}, nil
}

func (n *WorkflowRunner) Start(ctx context.Context) error {
	log.FromContext(ctx).Info("Starting Workflow runner")
	if n.taskLoop != nil {
		log.FromContext(ctx).Warn("WorkflowRunner already started")
		return nil
	}
	startTime := time.Now()
	n.keysManager.Start(ctx)
	n.taskLoop = NewLoop(n)
	go func() {
		log.FromContext(ctx).Info("Waiting for KeysManager to be ready")
		n.keysManager.WaitForReady()
		observability.ReportKeysManagerReady(n.config.MetricsClient, log.FromContext(ctx), startTime)
		n.taskLoop.Run(ctx)
	}()
	return nil
}

func (n *WorkflowRunner) Stop(ctx context.Context) error {
	log.FromContext(ctx).Info("Stopping Workflow runner")

	if n.taskLoop != nil {
		n.taskLoop.Close(ctx)
	}
	return nil
}

func (n *WorkflowRunner) RunTask(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (interface{}, error) {
	fqn := task.GetFQN()
	bundleName, actionName := actions.SplitFQN(fqn)
	bundle := n.registry.GetBundle(bundleName)
	if bundle == nil {
		return nil, util.NewPARError(
			aperrorpb.ActionPlatformErrorCode_INTERNAL_ERROR,
			fmt.Errorf("could not find bundle for %s", bundleName),
		)
	}
	action := bundle.GetAction(actionName)
	if action == nil {
		return nil, util.NewPARError(
			aperrorpb.ActionPlatformErrorCode_INTERNAL_ERROR,
			fmt.Errorf("could not find action for %s", actionName),
		)
	}
	if !n.config.IsActionAllowed(bundleName, actionName) {
		return nil, util.DefaultActionError(fmt.Errorf("action %s is not in the allow list", fqn))
	}
	if actions.IsHttpBundle(bundleName) {
		url, ok := task.Data.Attributes.Inputs["url"].(string)
		if !ok {
			return nil, util.DefaultActionError(errors.New("missing required field url"))
		}
		if !n.config.IsURLInAllowlist(url) {
			return nil, util.DefaultActionError(errors.New("request url is not allowed by runner policy: check your configuration file"))
		}
	}

	logger := log.FromContext(ctx)

	heartbeatCtx, heartbeatCancel := context.WithCancel(ctx)
	defer heartbeatCancel()
	go n.startHeartbeat(heartbeatCtx, task, logger)

	startTime := observability.ReportExecutionStart(n.config.MetricsClient, task.Data.Attributes.Client, fqn, task.Data.ID, logger)
	output, err := action.Run(ctx, task, credential)
	observability.ReportExecutionCompleted(n.config.MetricsClient, task.Data.Attributes.Client, fqn, task.Data.ID, startTime, err, logger)

	if err != nil {
		return nil, util.DefaultActionError(err)
	}

	return output, nil
}

func (n *WorkflowRunner) startHeartbeat(ctx context.Context, task *types.Task, logger log.Logger) {
	ticker := time.NewTicker(n.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Heartbeat stopped for task", log.String("task_id", task.Data.ID))
			return
		case <-ticker.C:
			err := n.opmsClient.Heartbeat(ctx, task.Data.Attributes.Client, task.Data.ID, task.GetFQN(), task.Data.Attributes.JobId)

			logger := log.FromContext(ctx).With(
				log.String(observability.TaskIDTagName, task.Data.ID),
				log.String(observability.ActionFqnTagName, task.GetFQN()),
				log.String(observability.JobIDTagName, task.Data.Attributes.JobId))

			if err != nil {
				logger.Error("Failed to send heartbeat", log.ErrorField(err))
			} else {
				logger.Info("Heartbeat sent successfully")
			}
		}
	}
}
