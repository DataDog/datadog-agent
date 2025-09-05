// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package runners provides workflow execution functionality for private action runners.
package runners

import (
	"context"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Loop manages the execution loop for workflow tasks.
type Loop struct {
	runner          *WorkflowRunner
	sem             chan struct{}
	shutdownChannel chan struct{}
	wg              sync.WaitGroup
}

// NewLoop creates a new Loop instance.
func NewLoop(runner *WorkflowRunner) *Loop {
	return &Loop{
		runner:          runner,
		sem:             make(chan struct{}, runner.config.RunnerPoolSize), // todo: we may consider moving to the semaphore before release.
		shutdownChannel: make(chan struct{}),
	}
}

// Run starts the execution loop.
func (l *Loop) Run(ctx context.Context) {
	l.wg.Add(1) // Increment the WaitGroup counter

	log.Info("Starting loop")

	breaker := utils.NewCircuitBreaker(
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
			log.Info("Stopping loop")
			return
		default:
		}

		var task *types.Task
		breaker.Do(
			ctx,
			func() error {
				dequeuedTask, err := l.runner.opmsClient.DequeueTask(ctx)
				if err != nil {
					log.Errorf("failed to dequeue task %v", err)
					return err
				}

				task = dequeuedTask
				return nil
			},
		)

		if task == nil {
			time.Sleep(l.runner.config.LoopInterval)
			continue
		}

		if err := task.Validate(); err != nil {
			log.Errorf("could not validate workflow task %v", err)
			l.publishFailure(ctx, task, err)
			continue
		}
		unwrappedTask, err := l.runner.taskVerifier.UnwrapTaskFromSignedEnvelope(task.Data.Attributes.SignedEnvelope)
		if err != nil {
			log.Errorf("could not verify workflow task %v", err)
			l.publishFailure(ctx, task, err)
			continue
		}
		log.Infof("task verified successfully %s", unwrappedTask.Data.ID)

		// JobID is generated on dequeue so its not part of the signature, it will be checked by the backend when publishing the result
		unwrappedTask.Data.Attributes.JobID = task.Data.Attributes.JobID
		task = unwrappedTask

		credential, err := l.runner.resolver.ResolveConnectionInfoToCredential(ctx, task.Data.Attributes.ConnectionInfo, nil)
		if err != nil {
			log.Errorf("could not resolve connection %v", err)
			l.publishFailure(ctx, task, err)
			continue
		}
		l.sem <- struct{}{}
		go func() {
			l.handleTask(ctx, task, credential)
			<-l.sem
		}()
	}
}

func (l *Loop) handleTask(
	ctx context.Context,
	task *types.Task,
	credential interface{},
) {
	taskCtx, taskCtxCancel := context.WithCancel(ctx)
	defer taskCtxCancel()

	output, err := l.runner.RunTask(taskCtx, task, credential)
	if err == nil {
		l.publishSuccess(ctx, task, output)
	} else {
		l.publishFailure(ctx, task, err)
	}
}

// Close stops the execution loop and waits for all tasks to complete.
func (l *Loop) Close(ctx context.Context) {
	close(l.shutdownChannel)

	done := make(chan struct{})
	go func() {
		l.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		log.Info("Worker stopped gracefully.")
	case <-ctx.Done():
		log.Warn("Workflow loop timeout reached. Forcing shutdown.")
	}
}

func (l *Loop) publishFailure(ctx context.Context, task *types.Task, e error) {
	if task.Data.Attributes.JobID == "" {
		log.Error("publish failure error: no job id was provided")
		return
	}
	inputError := utils.DefaultPARError(e)
	err := l.runner.opmsClient.PublishFailure(
		ctx,
		task.Data.ID,
		task.Data.Attributes.JobID,
		task.GetFQN(),
		inputError.ErrorCode,
		inputError.Message,
		inputError.ExternalMessage,
	)
	if err != nil {
		log.Errorf("publish failure error: unable to publish workflow task failure %v", err)
	}
}

func (l *Loop) publishSuccess(ctx context.Context, task *types.Task, output interface{}) {
	if task.Data.Attributes.JobID == "" {
		log.Error("publish success error: no job id was provided")
		return
	}
	err := l.runner.opmsClient.PublishSuccess(
		ctx,
		task.Data.ID,
		task.Data.Attributes.JobID,
		task.GetFQN(),
		output,
		"",
	)
	if err != nil {
		log.Errorf("publish success error: unable to publish workflow task success %v", err)
	}
}
