// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package runners

import (
	"context"
	"fmt"
	"time"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	traceroute "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/def"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/actions"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	privatebundles "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/credentials/resolver"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/executor"
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
	taskVerifier taskverifier.TaskVerifier
	executor     executor.Executor
	taskLoop     *Loop
}

func NewWorkflowRunner(
	configuration *config.Config,
	keysManager taskverifier.KeysManager,
	verifier taskverifier.TaskVerifier,
	opmsClient opms.Client,
	traceroute traceroute.Component,
	eventPlatform eventplatform.Component,
	ipcClient ipc.HTTPClient,
	taskExecutor executor.Executor,
) (*WorkflowRunner, error) {
	return &WorkflowRunner{
		registry:     privatebundles.NewRegistry(configuration, traceroute, eventPlatform, ipcClient),
		opmsClient:   opmsClient,
		resolver:     resolver.NewPrivateCredentialResolver(),
		config:       configuration,
		keysManager:  keysManager,
		taskVerifier: verifier,
		executor:     taskExecutor,
	}, nil
}

func (n *WorkflowRunner) Start(ctx context.Context) error {
	log.FromContext(ctx).Info("Starting Workflow runner")
	if n.taskLoop != nil {
		log.FromContext(ctx).Warn("WorkflowRunner already started")
		return nil
	}
	n.taskLoop = NewLoop(n)
	if n.executor != nil {
		// The loop only dequeues tasks and submits them to the executor over IPC;
		// task verification (and thus the keys manager) lives in the executor, not
		// here. Start polling immediately without waiting on the keys manager.
		go n.taskLoop.Run(ctx)
		return nil
	}
	// All-in-one mode: the loop verifies tasks locally, so it needs the keys manager.
	startTime := time.Now()
	n.keysManager.Start(ctx)
	go func() {
		n.WaitForKeysManagerReady(ctx, startTime)
		n.taskLoop.Run(ctx)
	}()
	return nil
}

// Prepare readies the runner to handle tasks by starting the verification-key
// manager and blocking until keys are available. It satisfies executor.TaskHandler
// so an executor that runs the handler in this process can ready it before
// serving tasks.
func (n *WorkflowRunner) Prepare(ctx context.Context) error {
	startTime := time.Now()
	n.StartKeysManager(ctx)
	n.WaitForKeysManagerReady(ctx, startTime)
	return nil
}

// StartKeysManager starts remote-config key management.
func (n *WorkflowRunner) StartKeysManager(ctx context.Context) {
	n.keysManager.Start(ctx)
}

// WaitForKeysManagerReady waits until task verification keys are ready.
func (n *WorkflowRunner) WaitForKeysManagerReady(ctx context.Context, startTime time.Time) {
	log.FromContext(ctx).Info("Waiting for KeysManager to be ready")
	n.keysManager.WaitForReady()
	observability.ReportKeysManagerReady(n.config.MetricsClient, log.FromContext(ctx), startTime)
}

// HandleTask owns a dequeued task through validation, execution, and final
// result publication.
func (n *WorkflowRunner) HandleTask(ctx context.Context, task *types.Task) {
	logger := log.FromContext(ctx)
	if err := task.Validate(); err != nil {
		logger.Error("could not validate workflow task", log.ErrorField(err))
		n.publishFailure(ctx, task, err)
		return
	}
	unwrappedTask, err := n.taskVerifier.UnwrapTask(task)
	if err != nil {
		logger.Error("could not verify workflow task", log.ErrorField(err))
		n.publishFailure(ctx, task, err)
		return
	}
	logger.Info("task verified successfully", log.String(observability.TaskIDTagName, unwrappedTask.Data.ID))

	// JobId is generated on dequeue so it is not part of the signature; it will
	// be checked by the backend when publishing the result.
	unwrappedTask.Data.Attributes.JobId = task.Data.Attributes.JobId
	// TraceId/SpanId are dequeue-time observability metadata, not part of the signed task.
	unwrappedTask.Data.Attributes.TraceId = task.Data.Attributes.TraceId
	unwrappedTask.Data.Attributes.SpanId = task.Data.Attributes.SpanId
	task = unwrappedTask

	credential, err := n.resolver.ResolveConnectionInfoToCredential(ctx, task.Data.Attributes.ConnectionInfo, nil)
	if err != nil {
		logger.Error("could not resolve connection", log.String(observability.TaskIDTagName, task.Data.ID), log.ErrorField(err))
		n.publishFailure(ctx, task, err)
		return
	}

	n.handleTask(ctx, task, credential)
}

func (n *WorkflowRunner) handleTask(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) {
	logger := log.FromContext(ctx).With(
		log.String(observability.TaskIDTagName, task.Data.ID),
		log.String(observability.ActionFqnTagName, task.GetFQN()),
	)
	taskCtx, taskCtxCancel := context.WithCancel(ctx)
	defer taskCtxCancel()

	timeoutSeconds := task.TimeoutSeconds()
	if timeoutSeconds == nil {
		timeoutSeconds = n.config.TaskTimeoutSeconds
	}
	timeoutCtx, timeoutCancel := util.CreateTimeoutContext(taskCtx, timeoutSeconds)
	defer timeoutCancel()

	output, err := n.RunTask(timeoutCtx, task, credential)

	if isTimeout, timeoutErr := util.HandleTimeoutError(timeoutCtx, err, timeoutSeconds, logger); isTimeout {
		n.publishFailure(ctx, task, timeoutErr)
		return
	}

	if err == nil {
		n.publishSuccess(ctx, task, output)
	} else {
		logger.Warn("task execution failed", log.ErrorField(err))
		n.publishFailure(ctx, task, err)
	}
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
) (output interface{}, err error) {
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

	logger := log.FromContext(ctx)

	heartbeatCtx, heartbeatCancel := context.WithCancel(ctx)
	defer heartbeatCancel()
	go n.startHeartbeat(heartbeatCtx, task, logger)

	ctx = telemetry.WithService(ctx, observability.ParService)
	span, ctx := telemetry.StartSpanFromUint64IDs(ctx, observability.ActionRunOperation, task.Data.Attributes.TraceId, task.Data.Attributes.SpanId)
	span.SetResourceName(fqn)
	span.SetTag("task_id", task.Data.ID)
	defer func() { span.Finish(err) }()

	startTime := observability.ReportExecutionStart(n.config.MetricsClient, task.Data.Attributes.Client, fqn, task.Data.ID, logger)
	output, err = action.Run(ctx, task, credential)
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

func (n *WorkflowRunner) publishFailure(ctx context.Context, task *types.Task, e error) {
	logger := log.FromContext(ctx)
	if task == nil || task.Data.Attributes == nil || task.Data.Attributes.JobId == "" {
		logger.Error("publish failure error: no job id was provided")
		return
	}
	inputError := util.DefaultPARError(e)
	err := n.opmsClient.PublishFailure(
		ctx,
		task.Data.Attributes.Client,
		task.Data.ID,
		task.Data.Attributes.JobId,
		task.GetFQN(),
		inputError.ErrorCode,
		inputError.Message,
		inputError.ExternalMessage,
	)
	if err != nil {
		logger.Error("publish failure error: unable to publish workflow task failure", log.ErrorField(err))
	}
}

func (n *WorkflowRunner) publishSuccess(ctx context.Context, task *types.Task, output interface{}) {
	logger := log.FromContext(ctx)
	if task.Data.Attributes.JobId == "" {
		logger.Error("publish success error: no job id was provided")
		return
	}
	err := n.opmsClient.PublishSuccess(
		ctx,
		task.Data.Attributes.Client,
		task.Data.ID,
		task.Data.Attributes.JobId,
		task.GetFQN(),
		output,
		"",
	)
	if err != nil {
		logger.Error("publish success error: unable to publish workflow task success", log.ErrorField(err))
	}
}
