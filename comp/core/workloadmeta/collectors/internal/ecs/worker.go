// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package ecs

import (
	"context"
	"time"

	"github.com/patrickmn/go-cache"
	"golang.org/x/time/rate"
	"k8s.io/client-go/util/workqueue"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	v1 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v1"
)

// worker is a rate limited worker that processes tasks to avoid hitting the ECS API rate limit
type worker[T any] struct {
	processFunc func(ctx context.Context, task v1.Task) (T, error)
	taskQueue   workqueue.RateLimitingInterface
	taskCache   *cache.Cache
}

func newWorker[T any](taskRateRPS, taskRateBurst int, cache *cache.Cache, processFunc func(ctx context.Context, task v1.Task) (T, error)) *worker[T] {
	return &worker[T]{
		processFunc: processFunc,
		taskCache:   cache,
		taskQueue: workqueue.NewRateLimitingQueue(workqueue.NewMaxOfRateLimiter(
			&workqueue.BucketRateLimiter{
				Limiter: rate.NewLimiter(rate.Every(time.Duration(1/taskRateRPS)*time.Second), taskRateBurst),
			},
		)),
	}
}

// execute runs the worker until all tasks are processed or the context is done
// returns the processed tasks, the tasks that are still in the queue and the skipped tasks
func (w *worker[T]) execute(ctx context.Context, tasks []v1.Task) ([]T, []v1.Task, []v1.Task) {
	tasksInQueue, skipped := w.addTasks(tasks)
	processed := w.run(ctx, tasksInQueue)

	rest := make([]v1.Task, 0, len(tasksInQueue))
	for _, task := range tasksInQueue {
		rest = append(rest, task)
	}

	return processed, rest, skipped
}

func (w *worker[T]) addTasks(tasks []v1.Task) (map[string]v1.Task, []v1.Task) {
	tasksInQueue := make(map[string]v1.Task, len(tasks))
	skipped := make([]v1.Task, 0, len(tasks))

	for _, t := range tasks {
		task := t
		if task.KnownStatus == workloadmeta.ECSTaskKnownStatusStopped {
			continue
		}

		// if task is not expired in cache, skip it
		if _, ok := w.taskCache.Get(task.Arn); ok {
			skipped = append(skipped, task)
			continue
		}

		w.taskQueue.AddRateLimited(&task)
		tasksInQueue[task.Arn] = task
	}
	return tasksInQueue, skipped
}

func (w *worker[T]) run(ctx context.Context, tasksInQueue map[string]v1.Task) []T {
	defer w.taskQueue.ShutDown()
	taskNumber := 0
	tasksInQueueLength := len(tasksInQueue)
	tasks := make([]T, 0, tasksInQueueLength)

	for {
		select {
		// if ctx is done, shutdown the worker
		case <-ctx.Done():
			return tasks
		default:
			// if worker has processed targetTaskNumber tasks, stop
			if taskNumber == tasksInQueueLength {
				return tasks
			}

			task, shutdown := w.taskQueue.Get()
			if shutdown {
				return tasks
			}

			processedTask, err := w.processFunc(ctx, *task.(*v1.Task))
			// if no error, add the processed task to the list of processed tasks
			// add the task to the cache and remove it from the tasksInQueue
			if err == nil {
				tasks = append(tasks, processedTask)
				w.taskCache.SetDefault(task.(*v1.Task).Arn, struct{}{})
				delete(tasksInQueue, task.(*v1.Task).Arn)
			}

			w.taskQueue.Done(task)
			taskNumber++
		}
	}
}
