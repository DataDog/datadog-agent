// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package ecs

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/require"

	v1 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v1"
	"github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v3or4"
)

func TestWorker(t *testing.T) {
	cached := cache.New(3*time.Minute, 30*time.Second)
	cached.SetDefault("task-cached", struct{}{})

	processFunc := func(ctx context.Context, task v1.Task) (v3or4.Task, error) {
		switch task.Arn {
		case "task-error":
			return v3or4.Task{}, errors.New("task2 error")
		case "task-delay":
			time.Sleep(3 * time.Second)
			return v1TaskToV4Task(task), nil
		default:
			return v1TaskToV4Task(task), nil
		}
	}

	worker := newWorker(35, 60, cached, processFunc)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	tasks := append(generateRunningTask(0, 10),
		[]v1.Task{
			{Arn: "task-stopped", KnownStatus: "STOPPED"},
			{Arn: "task-error", KnownStatus: "RUNNING"},
			{Arn: "task-cached", KnownStatus: "RUNNING"},
			{Arn: "task-delay", KnownStatus: "RUNNING"},
		}...)
	tasks = append(tasks, generateRunningTask(10, 20)...)

	processed, rest, skipped := worker.execute(ctx, tasks)

	expected := make([]v3or4.Task, 0, 10)
	for _, t := range generateRunningTask(0, 10) {
		expected = append(expected, v1TaskToV4Task(t))
	}
	expected = append(expected, v1TaskToV4Task(v1.Task{Arn: "task-delay", KnownStatus: "RUNNING"}))
	require.ElementsMatch(t, processed, expected)

	require.ElementsMatch(t, rest, append(generateRunningTask(10, 20), v1.Task{Arn: "task-error", KnownStatus: "RUNNING"}))

	require.ElementsMatch(t, skipped, []v1.Task{
		{
			Arn:         "task-cached",
			KnownStatus: "RUNNING",
		},
	})

}

func generateRunningTask(f, l int) []v1.Task {
	tasks := make([]v1.Task, 0, l-f)
	for i := f; i < l; i++ {
		tasks = append(tasks, v1.Task{
			Arn:         fmt.Sprintf("task-%d", i),
			KnownStatus: "RUNNING",
		})
	}
	return tasks
}
