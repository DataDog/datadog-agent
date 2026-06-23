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
	shutdownChannel chan struct{}
	wg              sync.WaitGroup
}

func NewLoop(runner *WorkflowRunner) *Loop {
	return &Loop{
		runner:          runner,
		shutdownChannel: make(chan struct{}),
	}
}

func (l *Loop) Run(parentCtx context.Context) {
	// Detach from the parent context's deadline and cancellation so the
	// polling loop isn't bounded by the startup timeout.
	// Shutdown is handled by Close through shutdownChannel; accepted task
	// completion is owned by the executor.
	ctx, cancel := context.WithCancel(context.WithoutCancel(parentCtx))
	defer cancel()
	logger := log.FromContext(ctx)
	l.wg.Add(1)

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

		if err := l.runner.executor.WaitForCapacity(ctx); err != nil {
			logger.Error("executor capacity wait failed", log.ErrorField(err))
			continue
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

		if err := l.runner.executor.SubmitTask(ctx, task); err != nil {
			logger.Error("failed to submit task to executor", log.String(observability.TaskIDTagName, task.Data.ID), log.ErrorField(err))
			publishFailure(ctx, l.runner.opmsClient, task, err)
		}
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
