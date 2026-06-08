// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package runners

import (
	"context"
	"sync"
	"time"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/observability"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
)

type Loop struct {
	runner          *WorkflowRunner
	sem             chan struct{}
	shutdownChannel chan struct{}
	wg              sync.WaitGroup
}

func NewLoop(runner *WorkflowRunner) *Loop {
	return &Loop{
		runner:          runner,
		sem:             make(chan struct{}, runner.config.RunnerPoolSize), // todo: we may consider moving to the semaphore before release.
		shutdownChannel: make(chan struct{}),
	}
}

func (l *Loop) Run(parentCtx context.Context) {
	// Detach from the parent context's deadline and cancellation so the
	// polling loop isn't bounded by the startup timeout.
	// Proper shutdown is handled by the Close method through the shutdownChannel which will let in flight task complete.
	ctx, cancel := context.WithCancel(context.WithoutCancel(parentCtx))
	defer cancel()
	logger := log.FromContext(ctx)
	l.wg.Add(1) // Increment the WaitGroup counter

	logger.Info("Starting loop")

	breaker := util.NewCircuitBreaker(
		"wf-par-polling",
		l.runner.config.MinBackoff,
		l.runner.config.MaxBackoff,
		l.runner.config.WaitBeforeRetry,
		l.runner.config.MaxAttempts,
	)

	defer l.wg.Done()
	for {
		select {
		case <-l.shutdownChannel:
			logger.Info("Stopping loop")
			return
		default:
		}

		if l.runner.executor != nil {
			if err := l.runner.executor.WaitForCapacity(ctx); err != nil {
				logger.Error("executor capacity wait failed", log.ErrorField(err))
				continue
			}
		}

		var task *types.Task
		var retryAfterDuration time.Duration
		breaker.Do(
			ctx,
			func() error {
				dequeuedTask, retryAfter, err := l.runner.opmsClient.DequeueTask(ctx)
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
			sleepDuration := l.runner.config.LoopInterval
			if retryAfterDuration > 0 {
				sleepDuration = retryAfterDuration
			}
			select {
			case <-l.shutdownChannel:
				logger.Info("Stopping loop")
				return
			case <-time.After(sleepDuration):
			}
			continue
		}

		if l.runner.executor != nil {
			if err := l.runner.executor.SubmitTask(ctx, task); err != nil {
				logger.Error("failed to submit task to executor", log.String(observability.TaskIDTagName, task.Data.ID), log.ErrorField(err))
			}
			continue
		}

		if err := task.Validate(); err != nil {
			logger.Error("could not validate workflow task", log.ErrorField(err))
			l.runner.publishFailure(ctx, task, err)
			continue
		}
		unwrappedTask, err := l.runner.taskVerifier.UnwrapTask(task)
		if err != nil {
			logger.Error("could not verify workflow task", log.ErrorField(err))
			l.runner.publishFailure(ctx, task, err)
			continue
		}
		logger.Info("task verified successfully", log.String(observability.TaskIDTagName, unwrappedTask.Data.ID))

		// JobId is generated on dequeue so its not part of the signature, it will be checked by the backend when publishing the result
		unwrappedTask.Data.Attributes.JobId = task.Data.Attributes.JobId
		// TraceId/SpanId are dequeue-time observability metadata, not part of the signed task
		unwrappedTask.Data.Attributes.TraceId = task.Data.Attributes.TraceId
		unwrappedTask.Data.Attributes.SpanId = task.Data.Attributes.SpanId
		task = unwrappedTask

		credential, err := l.runner.resolver.ResolveConnectionInfoToCredential(ctx, task.Data.Attributes.ConnectionInfo, nil)
		if err != nil {
			logger.Error("could not resolve connection", log.String(observability.TaskIDTagName, task.Data.ID), log.ErrorField(err))
			l.runner.publishFailure(ctx, task, err)
			continue
		}
		l.sem <- struct{}{}
		go func() {
			l.runner.handleTask(ctx, task, credential)
			<-l.sem
		}()
	}
}

func (l *Loop) Close(ctx context.Context) {
	close(l.shutdownChannel)

	done := make(chan struct{})
	go func() {
		l.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		log.FromContext(ctx).Info("Worker stopped gracefully.")
	case <-ctx.Done():
		log.FromContext(ctx).Warn("Workflow loop timeout reached. Forcing shutdown.")
	}
}
