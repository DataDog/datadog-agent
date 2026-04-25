// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package runners

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
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

		var task *types.Task
		breaker.Do(
			ctx,
			func() error {
				dequeuedTask, err := l.runner.opmsClient.DequeueTask(ctx)
				if err != nil {
					logger.Error("failed to dequeue task", log.ErrorField(err))
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
			logger.Error("could not validate workflow task", log.ErrorField(err))
			l.publishFailure(ctx, task, err)
			continue
		}
		unwrappedTask, err := l.runner.taskVerifier.UnwrapTask(task)
		if err != nil {
			logger.Error("could not verify workflow task", log.ErrorField(err))
			l.publishFailure(ctx, task, err)
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
	credential *privateconnection.PrivateCredentials,
) {
	logger := log.FromContext(ctx).With(
		log.String(observability.TaskIDTagName, task.Data.ID),
		log.String(observability.ActionFqnTagName, task.GetFQN()),
	)
	taskCtx, taskCtxCancel := context.WithCancel(ctx)
	defer taskCtxCancel()

	span, taskCtx := l.startTaskSpan(taskCtx, task)
	var taskErr error
	defer func() { span.Finish(tracer.WithError(taskErr)) }()

	timeoutSeconds := task.TimeoutSeconds()
	if timeoutSeconds == nil {
		timeoutSeconds = l.runner.config.TaskTimeoutSeconds
	}
	timeoutCtx, timeoutCancel := util.CreateTimeoutContext(taskCtx, timeoutSeconds)
	defer timeoutCancel()

	output, err := l.runner.RunTask(timeoutCtx, task, credential)

	if isTimeout, timeoutErr := util.HandleTimeoutError(timeoutCtx, err, timeoutSeconds, logger); isTimeout {
		taskErr = timeoutErr
		l.publishFailure(ctx, task, timeoutErr)
		return
	}

	if err == nil {
		l.publishSuccess(ctx, task, output)
	} else {
		taskErr = err
		logger.Warn("task execution failed", log.ErrorField(err))
		l.publishFailure(ctx, task, err)
	}
}

// startTaskSpan creates a span for the task execution, continuing the backend trace when trace
// propagation info is present in the task. The caller is responsible for finishing the span.
func (l *Loop) startTaskSpan(ctx context.Context, task *types.Task) (*tracer.Span, context.Context) {
	opts := []tracer.StartSpanOption{
		tracer.ResourceName(task.GetFQN()),
		tracer.Tag(observability.TaskIDTagName, task.Data.ID),
		tracer.Tag(observability.ActionFqnTagName, task.GetFQN()),
	}

	traceID := task.Data.Attributes.TraceId
	spanID := task.Data.Attributes.SpanId
	if traceID != 0 && spanID != 0 {
		carrier := tracer.TextMapCarrier{
			tracer.DefaultTraceIDHeader:  strconv.FormatUint(traceID, 10),
			tracer.DefaultParentIDHeader: strconv.FormatUint(spanID, 10),
		}
		if spanCtx, err := tracer.Extract(carrier); err == nil {
			opts = append(opts, tracer.ChildOf(spanCtx))
		}
	}

	return tracer.StartSpanFromContext(ctx, "par.execute_action", opts...)
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

func (l *Loop) publishFailure(ctx context.Context, task *types.Task, e error) {
	logger := log.FromContext(ctx)
	if task == nil || task.Data.Attributes == nil || task.Data.Attributes.JobId == "" {
		logger.Error("publish failure error: no job id was provided")
		return
	}
	inputError := util.DefaultPARError(e)
	err := l.runner.opmsClient.PublishFailure(
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

func (l *Loop) publishSuccess(ctx context.Context, task *types.Task, output interface{}) {
	logger := log.FromContext(ctx)
	if task.Data.Attributes.JobId == "" {
		logger.Error("publish success error: no job id was provided")
		return
	}
	err := l.runner.opmsClient.PublishSuccess(
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
