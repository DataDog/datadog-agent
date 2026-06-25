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
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/observability"
	taskverifier "github.com/DataDog/datadog-agent/pkg/privateactionrunner/task-verifier"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	aperrorpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/errorcode"
)

// TaskHandler is the executor-side per-task logic: verify the signed
// envelope, resolve credentials, and run the action. It owns nothing
// OPMS-related — the orchestrator manages dequeue, heartbeat, publish,
// and per-task lifecycle around each call to Execute.
type TaskHandler struct {
	registry     *privatebundles.Registry
	resolver     resolver.PrivateCredentialResolver
	config       *config.Config
	keysManager  taskverifier.KeysManager
	taskVerifier taskverifier.TaskVerifier
}

// NewTaskHandler builds a TaskHandler with the dependencies needed to verify
// and run a task. The action registry is constructed from the provided
// agent components.
func NewTaskHandler(
	configuration *config.Config,
	keysManager taskverifier.KeysManager,
	verifier taskverifier.TaskVerifier,
	traceroute traceroute.Component,
	eventPlatform eventplatform.Component,
	ipcClient ipc.HTTPClient,
) *TaskHandler {
	return &TaskHandler{
		registry:     privatebundles.NewRegistry(configuration, traceroute, eventPlatform, ipcClient),
		resolver:     resolver.NewPrivateCredentialResolver(),
		config:       configuration,
		keysManager:  keysManager,
		taskVerifier: verifier,
	}
}

// Prepare starts the verification keys manager and blocks until keys are
// ready. It is called once before the orchestrator dispatches any task.
func (h *TaskHandler) Prepare(ctx context.Context) error {
	startTime := time.Now()
	h.keysManager.Start(ctx)
	logger := log.FromContext(ctx)
	logger.Info("Waiting for KeysManager to be ready")
	h.keysManager.WaitForReady()
	observability.ReportKeysManagerReady(h.config.MetricsClient, logger, startTime)
	return nil
}

// Execute verifies the task signature, resolves credentials, looks up the
// action, runs it, and returns the action output. Publishing the result is
// the orchestrator's responsibility.
func (h *TaskHandler) Execute(ctx context.Context, task *types.Task) (output interface{}, err error) {
	unwrapped, err := h.taskVerifier.UnwrapTask(task)
	if err != nil {
		return nil, err
	}
	// JobId is generated on dequeue so it is not part of the signature, it
	// will be checked by the backend when publishing the result.
	unwrapped.Data.Attributes.JobId = task.Data.Attributes.JobId
	// TraceId/SpanId are dequeue-time observability metadata, not part of
	// the signed task.
	unwrapped.Data.Attributes.TraceId = task.Data.Attributes.TraceId
	unwrapped.Data.Attributes.SpanId = task.Data.Attributes.SpanId

	credential, err := h.resolver.ResolveConnectionInfoToCredential(ctx, unwrapped.Data.Attributes.ConnectionInfo, nil)
	if err != nil {
		return nil, err
	}

	fqn := unwrapped.GetFQN()
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

	ctx = telemetry.WithService(ctx, observability.ParService)
	span, ctx := telemetry.StartSpanFromUint64IDs(ctx, observability.ActionRunOperation, unwrapped.Data.Attributes.TraceId, unwrapped.Data.Attributes.SpanId)
	span.SetResourceName(fqn)
	span.SetTag("task_id", unwrapped.Data.ID)
	defer func() { span.Finish(err) }()

	startTime := observability.ReportExecutionStart(h.config.MetricsClient, unwrapped.Data.Attributes.Client, fqn, unwrapped.Data.ID, logger)
	output, err = action.Run(ctx, unwrapped, credential)
	observability.ReportExecutionCompleted(h.config.MetricsClient, unwrapped.Data.Attributes.Client, fqn, unwrapped.Data.ID, startTime, err, logger)

	if err != nil {
		return nil, util.DefaultActionError(err)
	}
	return output, nil
}
