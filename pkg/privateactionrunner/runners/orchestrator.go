// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package runners

import (
	"context"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/executor"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/observability"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
)

// Orchestrator owns the full task lifecycle around an Executor: OPMS
// polling, concurrency (semaphore sized by RunnerPoolSize), heartbeat
// ticker, and publish-success/failure. The Executor only handles per-task
// compute (verify, resolve, run) and returns an output value or an error.
type Orchestrator struct {
	opmsClient      opms.Client
	config          *config.Config
	executor        executor.Executor
	sem             chan struct{}
	shutdownChannel chan struct{}
	wg              sync.WaitGroup
	started         bool
}

// NewOrchestrator builds an Orchestrator wired to an OPMS client and an
// Executor.
func NewOrchestrator(cfg *config.Config, opmsClient opms.Client, exec executor.Executor) *Orchestrator {
	return &Orchestrator{
		opmsClient:      opmsClient,
		config:          cfg,
		executor:        exec,
		sem:             make(chan struct{}, cfg.RunnerPoolSize),
		shutdownChannel: make(chan struct{}),
	}
}

// Start prepares the executor and then begins the polling loop in a
// background goroutine. It returns as soon as the goroutine is scheduled;
// the loop itself only begins polling after Executor.Prepare succeeds.
func (o *Orchestrator) Start(ctx context.Context) error {
	logger := log.FromContext(ctx)
	logger.Info("Starting Workflow runner")
	if o.started {
		logger.Warn("Orchestrator already started")
		return nil
	}
	o.started = true
	go func() {
		if err := o.executor.Prepare(ctx); err != nil {
			logger.Error("executor prepare failed; orchestrator will not poll", log.ErrorField(err))
			return
		}
		o.run(ctx)
	}()
	return nil
}

// Stop signals the polling loop to exit, waits for in-flight per-task
// goroutines to drain (bounded by the configured ExecutorDrainTimeout
// and by ctx), then stops the executor.
func (o *Orchestrator) Stop(ctx context.Context) error {
	logger := log.FromContext(ctx)
	logger.Info("Stopping Workflow runner")
	if !o.started {
		return nil
	}
	close(o.shutdownChannel)

	drainCtx := ctx
	if o.config.ExecutorDrainTimeout > 0 {
		var cancel context.CancelFunc
		drainCtx, cancel = context.WithTimeout(ctx, o.config.ExecutorDrainTimeout)
		defer cancel()
	}

	done := make(chan struct{})
	go func() {
		o.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		logger.Info("Worker stopped gracefully.")
	case <-drainCtx.Done():
		logger.Warn("Workflow loop drain timeout reached; in-flight tasks will be cancelled.")
	}
	return o.executor.Stop(ctx)
}

func (o *Orchestrator) run(parentCtx context.Context) {
	// Detach from the parent context's deadline / cancellation so the
	// polling loop is not bounded by the startup or lifecycle context.
	// Shutdown is driven by shutdownChannel; in-flight tasks finish on
	// the per-task ctx derived from this one.
	ctx, cancel := context.WithCancel(context.WithoutCancel(parentCtx))
	defer cancel()
	logger := log.FromContext(ctx)

	logger.Info("Starting loop")

	breaker := util.NewCircuitBreaker(
		"wf-par-polling",
		o.config.MinBackoff,
		o.config.MaxBackoff,
		o.config.WaitBeforeRetry,
		o.config.MaxAttempts,
	)

	for {
		select {
		case <-o.shutdownChannel:
			logger.Info("Stopping loop")
			return
		default:
		}

		var task *types.Task
		var retryAfterDuration time.Duration
		breaker.Do(
			ctx,
			func() error {
				dequeuedTask, retryAfter, err := o.opmsClient.DequeueTask(ctx)
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
			sleepDuration := o.config.LoopInterval
			if retryAfterDuration > 0 {
				sleepDuration = retryAfterDuration
			}
			select {
			case <-o.shutdownChannel:
				logger.Info("Stopping loop")
				return
			case <-time.After(sleepDuration):
			}
			continue
		}

		if err := task.Validate(); err != nil {
			logger.Error("could not validate workflow task", log.ErrorField(err))
			o.publishFailure(ctx, task, err)
			continue
		}

		o.sem <- struct{}{}
		o.wg.Add(1)
		go func(task *types.Task) {
			defer func() {
				<-o.sem
				o.wg.Done()
			}()
			o.handleTask(ctx, task)
		}(task)
	}
}

// handleTask runs the per-task lifecycle: timeout context, heartbeat
// ticker, executor.Execute, publish. Fields used for heartbeat and publish
// come from the outer dequeued envelope, so the orchestrator does not need
// to unwrap the signed payload (the executor does that).
func (o *Orchestrator) handleTask(ctx context.Context, task *types.Task) {
	logger := log.FromContext(ctx).With(
		log.String(observability.TaskIDTagName, task.Data.ID),
		log.String(observability.ActionFqnTagName, task.GetFQN()),
	)

	timeoutSeconds := task.TimeoutSeconds()
	if timeoutSeconds == nil {
		timeoutSeconds = o.config.TaskTimeoutSeconds
	}
	timeoutCtx, timeoutCancel := util.CreateTimeoutContext(ctx, timeoutSeconds)
	defer timeoutCancel()

	hbCtx, hbCancel := context.WithCancel(timeoutCtx)
	defer hbCancel()
	go o.heartbeatLoop(hbCtx, task)

	output, err := o.executor.Execute(timeoutCtx, task)
	hbCancel()

	if isTimeout, timeoutErr := util.HandleTimeoutError(timeoutCtx, err, timeoutSeconds, logger); isTimeout {
		o.publishFailure(ctx, task, timeoutErr)
		return
	}
	if err != nil {
		logger.Warn("task execution failed", log.ErrorField(err))
		o.publishFailure(ctx, task, err)
		return
	}
	o.publishSuccess(ctx, task, output)
}

func (o *Orchestrator) heartbeatLoop(ctx context.Context, task *types.Task) {
	ticker := time.NewTicker(o.config.HeartbeatInterval)
	defer ticker.Stop()
	logger := log.FromContext(ctx).With(
		log.String(observability.TaskIDTagName, task.Data.ID),
		log.String(observability.ActionFqnTagName, task.GetFQN()),
		log.String(observability.JobIDTagName, task.Data.Attributes.JobId),
	)
	for {
		select {
		case <-ctx.Done():
			logger.Info("Heartbeat stopped for task")
			return
		case <-ticker.C:
			err := o.opmsClient.Heartbeat(ctx, task.Data.Attributes.Client, task.Data.ID, task.GetFQN(), task.Data.Attributes.JobId)
			if err != nil {
				logger.Error("Failed to send heartbeat", log.ErrorField(err))
			} else {
				logger.Info("Heartbeat sent successfully")
			}
		}
	}
}

func (o *Orchestrator) publishSuccess(ctx context.Context, task *types.Task, output interface{}) {
	logger := log.FromContext(ctx)
	if task.Data.Attributes.JobId == "" {
		logger.Error("publish success error: no job id was provided")
		return
	}
	if err := o.opmsClient.PublishSuccess(
		ctx,
		task.Data.Attributes.Client,
		task.Data.ID,
		task.Data.Attributes.JobId,
		task.GetFQN(),
		output,
		"",
	); err != nil {
		logger.Error("publish success error: unable to publish workflow task success", log.ErrorField(err))
	}
}

func (o *Orchestrator) publishFailure(ctx context.Context, task *types.Task, e error) {
	logger := log.FromContext(ctx)
	if task == nil || task.Data.Attributes == nil || task.Data.Attributes.JobId == "" {
		logger.Error("publish failure error: no job id was provided")
		return
	}
	inputError := util.DefaultPARError(e)
	if err := o.opmsClient.PublishFailure(
		ctx,
		task.Data.Attributes.Client,
		task.Data.ID,
		task.Data.Attributes.JobId,
		task.GetFQN(),
		inputError.ErrorCode,
		inputError.Message,
		inputError.ExternalMessage,
	); err != nil {
		logger.Error("publish failure error: unable to publish workflow task failure", log.ErrorField(err))
	}
}
