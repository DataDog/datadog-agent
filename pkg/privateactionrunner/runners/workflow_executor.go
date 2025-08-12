package runners

import (
	"context"
	"sync"
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/helpers"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type Loop struct {
	log             log.Component
	runner          *WorkflowRunner
	sem             chan struct{}
	shutdownChannel chan struct{}
	wg              sync.WaitGroup
}

func NewLoop(runner *WorkflowRunner, log log.Component) *Loop {
	return &Loop{
		log:             log,
		runner:          runner,
		sem:             make(chan struct{}, runner.config.RunnerPoolSize), // todo: we may consider moving to the semaphore before release.
		shutdownChannel: make(chan struct{}),
	}
}

func (l *Loop) Run(ctx context.Context) {
	l.wg.Add(1) // Increment the WaitGroup counter

	l.log.Info("Starting loop")

	breaker := helpers.NewCircuitBreaker(
		l.log,
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
			l.log.Info("Stopping loop")
			return
		default:
		}

		var task *types.Task
		breaker.Do(
			ctx,
			func() error {
				dequeuedTask, err := l.runner.opmsClient.DequeueTask(ctx)
				if err != nil {
					l.log.Errorf("failed to dequeue task %v", err)
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
			l.log.Errorf("could not validate workflow task %v", err)
			l.publishFailure(ctx, task, err)
			continue
		}
		unwrappedTask, err := l.runner.taskVerifier.UnwrapTaskFromSignedEnvelope(task.Data.Attributes.SignedEnvelope)
		if err != nil {
			l.log.Errorf("could not verify workflow task %v", err)
			l.publishFailure(ctx, task, err)
			continue
		}
		l.log.Infof("task verified successfully %s", unwrappedTask.Data.ID)

		// JobId is generated on dequeue so its not part of the signature, it will be checked by the backend when publishing the result
		unwrappedTask.Data.Attributes.JobId = task.Data.Attributes.JobId
		task = unwrappedTask

		credential, err := l.runner.resolver.ResolveConnectionInfoToCredential(ctx, task.Data.Attributes.ConnectionInfo, nil)
		if err != nil {
			l.log.Errorf("could not resolve connection %v", err)
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

func (l *Loop) Close(ctx context.Context) {
	close(l.shutdownChannel)

	done := make(chan struct{})
	go func() {
		l.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		l.log.Info("Worker stopped gracefully.")
	case <-ctx.Done():
		l.log.Warn("Workflow loop timeout reached. Forcing shutdown.")
	}
}

func (l *Loop) publishFailure(ctx context.Context, task *types.Task, e error) {
	if task.Data.Attributes.JobId == "" {
		l.log.Error("publish failure error: no job id was provided")
		return
	}
	inputError := helpers.DefaultPARError(e)
	err := l.runner.opmsClient.PublishFailure(
		ctx,
		task.Data.ID,
		task.Data.Attributes.JobId,
		task.GetFQN(),
		inputError.ErrorCode,
		inputError.Message,
		inputError.ExternalMessage,
	)
	if err != nil {
		l.log.Errorf("publish failure error: unable to publish workflow task failure %v", err)
	}
}

func (l *Loop) publishSuccess(ctx context.Context, task *types.Task, output interface{}) {
	if task.Data.Attributes.JobId == "" {
		l.log.Error("publish success error: no job id was provided")
		return
	}
	err := l.runner.opmsClient.PublishSuccess(
		ctx,
		task.Data.ID,
		task.Data.Attributes.JobId,
		task.GetFQN(),
		output,
		"",
	)
	if err != nil {
		l.log.Errorf("publish success error: unable to publish workflow task success %v", err)
	}
}
