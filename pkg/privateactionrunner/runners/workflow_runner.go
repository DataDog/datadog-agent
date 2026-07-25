// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package runners

import (
	"context"
	"errors"
	"sync"
	"time"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	traceroute "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/def"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/encryptioncontext"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/observability"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	taskverifier "github.com/DataDog/datadog-agent/pkg/privateactionrunner/task-verifier"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
)

type WorkflowRunner struct {
	config          *config.Config
	opmsClient      opms.Client
	keysManager     taskverifier.KeysManager
	taskExecutor    *WorkflowTaskExecutor
	encryptionStore *encryptioncontext.Store
	sem             chan struct{}
	shutdownChannel chan struct{}
	wg              sync.WaitGroup
	started         bool
}

func NewWorkflowRunner(
	configuration *config.Config,
	keysManager taskverifier.KeysManager,
	verifier taskverifier.TaskVerifier,
	opmsClient opms.Client,
	traceroute traceroute.Component,
	eventPlatform eventplatform.Component,
	ipcClient ipc.HTTPClient,
	secretResolver secrets.Component,
) (*WorkflowRunner, error) {
	encryptionStore := encryptioncontext.NewStore()
	taskExecutor := NewWorkflowTaskExecutor(configuration, verifier, traceroute, eventPlatform, ipcClient, encryptionStore, secretResolver)

	return &WorkflowRunner{
		config:          configuration,
		opmsClient:      opmsClient,
		keysManager:     keysManager,
		taskExecutor:    taskExecutor,
		encryptionStore: encryptionStore,
		sem:             make(chan struct{}, configuration.RunnerPoolSize), // todo: we may consider moving to the semaphore before release.
		shutdownChannel: make(chan struct{}),
	}, nil
}

func (n *WorkflowRunner) Start(ctx context.Context) error {
	log.FromContext(ctx).Info("Starting Workflow runner")
	if n.started {
		log.FromContext(ctx).Warn("WorkflowRunner already started")
		return nil
	}
	n.started = true
	startTime := time.Now()
	go n.encryptionStore.Start()
	n.keysManager.Start(ctx)
	go func() {
		log.FromContext(ctx).Info("Waiting for KeysManager to be ready")
		n.keysManager.WaitForReady()
		observability.ReportKeysManagerReady(n.config.MetricsClient, log.FromContext(ctx), startTime)
		n.run(ctx)
	}()
	return nil
}

func (n *WorkflowRunner) Stop(ctx context.Context) error {
	log.FromContext(ctx).Info("Stopping Workflow runner")

	close(n.shutdownChannel)
	done := make(chan struct{})
	go func() {
		n.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		log.FromContext(ctx).Info("Workflow runner stopped gracefully.")
	case <-ctx.Done():
		log.FromContext(ctx).Warn("Workflow runner timeout reached. Forcing shutdown.")
	}

	n.encryptionStore.Stop()
	return nil
}

func (n *WorkflowRunner) run(parentCtx context.Context) {
	// taskCtx is detached from the parent and NOT cancelled when this loop
	// returns, so in-flight tasks drain gracefully on shutdown (Stop waits via n.wg).
	taskCtx := context.WithoutCancel(parentCtx)
	// pollCtx is cancelled on loop exit to abort any in-flight poll/prepare.
	pollCtx, cancelPoll := context.WithCancel(taskCtx)
	defer cancelPoll()
	logger := log.FromContext(pollCtx)
	n.wg.Add(1)
	defer n.wg.Done()

	logger.Info("Starting loop")

	breaker := util.NewCircuitBreaker(
		"wf-par-polling",
		n.config.MinBackoff,
		n.config.MaxBackoff,
		n.config.WaitBeforeRetry,
		n.config.MaxAttempts,
	)

	for {
		select {
		case <-n.shutdownChannel:
			logger.Info("Stopping loop")
			return
		default:
		}

		var task *types.Task
		var retryAfterDuration time.Duration
		breaker.Do(
			pollCtx,
			func() error {
				dequeuedTask, retryAfter, err := n.opmsClient.DequeueTask(pollCtx)
				if err != nil {
					logger.Error("failed to dequeue task", log.ErrorField(err))
					return err
				}

				task = dequeuedTask
				retryAfterDuration = retryAfter
				return nil
			},
		)

		if task == nil {
			sleepDuration := n.config.LoopInterval
			if retryAfterDuration > 0 {
				sleepDuration = retryAfterDuration
			}
			select {
			case <-n.shutdownChannel:
				logger.Info("Stopping loop")
				return
			case <-time.After(sleepDuration):
			}
			continue
		}

		preparedTask, failureTask, err := n.taskExecutor.PrepareTask(pollCtx, task)
		if err != nil {
			n.publishFailure(taskCtx, failureTask, err)
			continue
		}

		n.sem <- struct{}{}
		n.wg.Add(1)
		go func() {
			defer n.wg.Done()
			defer func() { <-n.sem }()
			n.handleTask(taskCtx, preparedTask)
		}()
	}
}

func (n *WorkflowRunner) handleTask(ctx context.Context, preparedTask *PreparedWorkflowTask) {
	task := preparedTask.Task
	logger := log.FromContext(ctx).With(
		log.String(observability.TaskIDTagName, task.Data.ID),
		log.String(observability.ActionFqnTagName, task.GetFQN()),
	)
	taskCtx, taskCtxCancel := context.WithCancel(log.ContextWithLogger(ctx, logger))
	defer taskCtxCancel()

	heartbeatCtx, heartbeatCancel := context.WithCancel(ctx)
	defer heartbeatCancel()
	go n.startHeartbeat(heartbeatCtx, task, logger)

	output, err := n.taskExecutor.RunPrepared(taskCtx, preparedTask)

	if err == nil {
		n.publishSuccess(ctx, task, output)
	} else {
		logger.Warn("task execution failed", log.ErrorField(err))
		n.publishFailure(ctx, task, err)
	}
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
				if errors.Is(err, opms.ErrJobNotFound) {
					logger.Info("Task no longer exists remotely; stopping heartbeat")
					return
				}
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
