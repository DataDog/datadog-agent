// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package runners

import (
	"context"
	"fmt"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	traceroute "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/def"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/actions"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	privatebundles "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/credentials/resolver"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/encryptioncontext"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/observability"
	taskverifier "github.com/DataDog/datadog-agent/pkg/privateactionrunner/task-verifier"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	aperrorpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/errorcode"
)

type PreparedWorkflowTask struct {
	Task       *types.Task
	Credential *privateconnection.PrivateCredentials
}

type WorkflowTaskExecutor struct {
	registry     *privatebundles.Registry
	config       *config.Config
	taskVerifier taskverifier.TaskVerifier
	resolver     resolver.PrivateCredentialResolver
}

func NewWorkflowTaskExecutor(
	configuration *config.Config,
	taskVerifier taskverifier.TaskVerifier,
	traceroute traceroute.Component,
	eventPlatform eventplatform.Component,
	ipcClient ipc.HTTPClient,
	encryptionStore *encryptioncontext.Store,
) *WorkflowTaskExecutor {
	return &WorkflowTaskExecutor{
		registry:     privatebundles.NewRegistry(configuration, traceroute, eventPlatform, ipcClient, encryptionStore),
		config:       configuration,
		taskVerifier: taskVerifier,
		resolver:     resolver.NewPrivateCredentialResolver(),
	}
}

func (e *WorkflowTaskExecutor) PrepareTask(
	ctx context.Context,
	task *types.Task,
) (*PreparedWorkflowTask, *types.Task, error) {
	logger := log.FromContext(ctx)

	if err := task.Validate(); err != nil {
		logger.Error("could not validate workflow task", log.ErrorField(err))
		return nil, task, err
	}
	unwrappedTask, err := e.taskVerifier.UnwrapTask(task)
	if err != nil {
		logger.Error("could not verify workflow task", log.ErrorField(err))
		return nil, task, err
	}
	logger.Info("task verified successfully", log.String(observability.TaskIDTagName, unwrappedTask.Data.ID))

	// JobId is generated on dequeue so its not part of the signature, it will be checked by the backend when publishing the result
	unwrappedTask.Data.Attributes.JobId = task.Data.Attributes.JobId
	// TraceId/SpanId are dequeue-time observability metadata, not part of the signed task
	unwrappedTask.Data.Attributes.TraceId = task.Data.Attributes.TraceId
	unwrappedTask.Data.Attributes.SpanId = task.Data.Attributes.SpanId

	credential, err := e.resolver.ResolveConnectionInfoToCredential(ctx, unwrappedTask.Data.Attributes.ConnectionInfo, nil)
	if err != nil {
		logger.Error("could not resolve connection", log.String(observability.TaskIDTagName, unwrappedTask.Data.ID), log.ErrorField(err))
		return nil, unwrappedTask, err
	}

	return &PreparedWorkflowTask{
		Task:       unwrappedTask,
		Credential: credential,
	}, nil, nil
}

func (e *WorkflowTaskExecutor) RunTask(
	ctx context.Context,
	preparedTask *PreparedWorkflowTask,
) (output interface{}, err error) {
	task := preparedTask.Task
	fqn := task.GetFQN()
	bundleName, actionName := actions.SplitFQN(fqn)
	bundle := e.registry.GetBundle(bundleName)
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
	if !e.config.IsActionAllowed(bundleName, actionName) {
		return nil, util.DefaultActionError(fmt.Errorf("action %s is not in the allow list", fqn))
	}

	logger := log.FromContext(ctx)

	ctx = telemetry.WithService(ctx, observability.ParService)
	span, ctx := telemetry.StartSpanFromUint64IDs(ctx, observability.ActionRunOperation, task.Data.Attributes.TraceId, task.Data.Attributes.SpanId)
	span.SetResourceName(fqn)
	span.SetTag("task_id", task.Data.ID)
	defer func() { span.Finish(err) }()

	startTime := observability.ReportExecutionStart(e.config.MetricsClient, task.Data.Attributes.Client, fqn, task.Data.ID, logger)
	output, err = action.Run(ctx, task, preparedTask.Credential)
	observability.ReportExecutionCompleted(e.config.MetricsClient, task.Data.Attributes.Client, fqn, task.Data.ID, startTime, err, logger)

	if err != nil {
		return nil, util.DefaultActionError(err)
	}

	return output, nil
}
