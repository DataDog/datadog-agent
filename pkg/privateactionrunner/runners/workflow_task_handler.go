// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

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
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/observability"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	taskverifier "github.com/DataDog/datadog-agent/pkg/privateactionrunner/task-verifier"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	aperrorpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/errorcode"
)

// WorkflowTaskHandler owns task verification, execution, and result publication.
type WorkflowTaskHandler struct {
	registry     *privatebundles.Registry
	opmsClient   opms.Client
	resolver     resolver.PrivateCredentialResolver
	config       *config.Config
	keysManager  taskverifier.KeysManager
	taskVerifier taskverifier.TaskVerifier
}

func NewWorkflowTaskHandler(
	configuration *config.Config,
	keysManager taskverifier.KeysManager,
	verifier taskverifier.TaskVerifier,
	opmsClient opms.Client,
	traceroute traceroute.Component,
	eventPlatform eventplatform.Component,
	ipcClient ipc.HTTPClient,
) (*WorkflowTaskHandler, error) {
	return &WorkflowTaskHandler{
		registry:     privatebundles.NewRegistry(configuration, traceroute, eventPlatform, ipcClient),
		opmsClient:   opmsClient,
		resolver:     resolver.NewPrivateCredentialResolver(),
		config:       configuration,
		keysManager:  keysManager,
		taskVerifier: verifier,
	}, nil
}

// Prepare readies the handler to process tasks by starting the verification-key
// manager and blocking until keys are available.
func (h *WorkflowTaskHandler) Prepare(ctx context.Context) error {
	startTime := time.Now()
	h.startKeysManager(ctx)
	h.waitForKeysManagerReady(ctx, startTime)
	return nil
}

func (h *WorkflowTaskHandler) startKeysManager(ctx context.Context) {
	h.keysManager.Start(ctx)
}

func (h *WorkflowTaskHandler) waitForKeysManagerReady(ctx context.Context, startTime time.Time) {
	log.FromContext(ctx).Info("Waiting for KeysManager to be ready")
	h.keysManager.WaitForReady()
	observability.ReportKeysManagerReady(h.config.MetricsClient, log.FromContext(ctx), startTime)
}

// HandleTask owns a dequeued task through validation, execution, and final
// result publication.
func (h *WorkflowTaskHandler) HandleTask(ctx context.Context, task *types.Task) {
	logger := log.FromContext(ctx)
	if err := task.Validate(); err != nil {
		logger.Error("could not validate workflow task", log.ErrorField(err))
		publishFailure(ctx, h.opmsClient, task, err)
		return
	}
	unwrappedTask, err := h.taskVerifier.UnwrapTask(task)
	if err != nil {
		logger.Error("could not verify workflow task", log.ErrorField(err))
		publishFailure(ctx, h.opmsClient, task, err)
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

	credential, err := h.resolver.ResolveConnectionInfoToCredential(ctx, task.Data.Attributes.ConnectionInfo, nil)
	if err != nil {
		logger.Error("could not resolve connection", log.String(observability.TaskIDTagName, task.Data.ID), log.ErrorField(err))
		publishFailure(ctx, h.opmsClient, task, err)
		return
	}

	h.handleTask(ctx, task, credential)
}

func (h *WorkflowTaskHandler) handleTask(
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
		timeoutSeconds = h.config.TaskTimeoutSeconds
	}
	timeoutCtx, timeoutCancel := util.CreateTimeoutContext(taskCtx, timeoutSeconds)
	defer timeoutCancel()

	output, err := h.RunTask(timeoutCtx, task, credential)

	if isTimeout, timeoutErr := util.HandleTimeoutError(timeoutCtx, err, timeoutSeconds, logger); isTimeout {
		publishFailure(ctx, h.opmsClient, task, timeoutErr)
		return
	}

	if err == nil {
		publishSuccess(ctx, h.opmsClient, task, output)
	} else {
		logger.Warn("task execution failed", log.ErrorField(err))
		publishFailure(ctx, h.opmsClient, task, err)
	}
}

func (h *WorkflowTaskHandler) RunTask(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (output interface{}, err error) {
	fqn := task.GetFQN()
	bundleName, actionName := actions.SplitFQN(fqn)
	bundle := h.registry.GetBundle(bundleName)
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
	if !h.config.IsActionAllowed(bundleName, actionName) {
		return nil, util.DefaultActionError(fmt.Errorf("action %s is not in the allow list", fqn))
	}

	logger := log.FromContext(ctx)

	heartbeatCtx, heartbeatCancel := context.WithCancel(ctx)
	defer heartbeatCancel()
	go h.startHeartbeat(heartbeatCtx, task, logger)

	ctx = telemetry.WithService(ctx, observability.ParService)
	span, ctx := telemetry.StartSpanFromUint64IDs(ctx, observability.ActionRunOperation, task.Data.Attributes.TraceId, task.Data.Attributes.SpanId)
	span.SetResourceName(fqn)
	span.SetTag("task_id", task.Data.ID)
	defer func() { span.Finish(err) }()

	startTime := observability.ReportExecutionStart(h.config.MetricsClient, task.Data.Attributes.Client, fqn, task.Data.ID, logger)
	output, err = action.Run(ctx, task, credential)
	observability.ReportExecutionCompleted(h.config.MetricsClient, task.Data.Attributes.Client, fqn, task.Data.ID, startTime, err, logger)

	if err != nil {
		return nil, util.DefaultActionError(err)
	}

	return output, nil
}

func (h *WorkflowTaskHandler) startHeartbeat(ctx context.Context, task *types.Task, logger log.Logger) {
	ticker := time.NewTicker(h.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Heartbeat stopped for task", log.String("task_id", task.Data.ID))
			return
		case <-ticker.C:
			err := h.opmsClient.Heartbeat(ctx, task.Data.Attributes.Client, task.Data.ID, task.GetFQN(), task.Data.Attributes.JobId)

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
