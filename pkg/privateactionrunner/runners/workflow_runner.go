// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package runners

import (
	"context"
	"fmt"
	"sync"
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

type WorkflowRunner struct {
	registry     *privatebundles.Registry
	opmsClient   opms.Client
	resolver     resolver.PrivateCredentialResolver
	config       *config.Config
	keysManager  taskverifier.KeysManager
	taskVerifier taskverifier.TaskVerifier
	taskLoop     *Loop
	mu           sync.Mutex
	started      bool
	cancel       context.CancelFunc
	wg           sync.WaitGroup
}

func NewWorkflowRunner(
	configuration *config.Config,
	keysManager taskverifier.KeysManager,
	verifier taskverifier.TaskVerifier,
	opmsClient opms.Client,
	traceroute traceroute.Component,
	eventPlatform eventplatform.Component,
	ipcClient ipc.HTTPClient,
) (*WorkflowRunner, error) {
	return &WorkflowRunner{
		registry:     privatebundles.NewRegistry(configuration, traceroute, eventPlatform, ipcClient),
		opmsClient:   opmsClient,
		resolver:     resolver.NewPrivateCredentialResolver(),
		config:       configuration,
		keysManager:  keysManager,
		taskVerifier: verifier,
	}, nil
}

func (n *WorkflowRunner) Start(ctx context.Context) error {
	log.FromContext(ctx).Info("Starting Workflow runner")
	startTime := time.Now()
	runnerCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))

	n.mu.Lock()
	if n.started {
		n.mu.Unlock()
		cancel()
		log.FromContext(ctx).Warn("WorkflowRunner already started")
		return nil
	}
	n.started = true
	n.cancel = cancel
	n.mu.Unlock()

	n.keysManager.Start(ctx)
	n.wg.Add(1)
	go func() {
		defer n.wg.Done()
		logger := log.FromContext(runnerCtx)
		logger.Info("Waiting for KeysManager to be ready")
		select {
		case <-n.keysManager.Ready():
		case <-runnerCtx.Done():
			logger.Info("Workflow runner stopped before KeysManager was ready")
			return
		}

		observability.ReportKeysManagerReady(n.config.MetricsClient, logger, startTime)
		taskLoop := NewLoop(n)

		n.mu.Lock()
		if runnerCtx.Err() != nil {
			n.mu.Unlock()
			return
		}
		n.taskLoop = taskLoop
		n.mu.Unlock()

		taskLoop.Run(runnerCtx)
	}()
	return nil
}

func (n *WorkflowRunner) Stop(ctx context.Context) error {
	log.FromContext(ctx).Info("Stopping Workflow runner")

	n.mu.Lock()
	cancel := n.cancel
	taskLoop := n.taskLoop
	n.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if taskLoop != nil {
		taskLoop.Close(ctx)
	}

	done := make(chan struct{})
	go func() {
		n.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		log.FromContext(ctx).Warn("Workflow runner stop timeout reached.")
		return ctx.Err()
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
