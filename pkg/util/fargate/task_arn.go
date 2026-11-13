// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fargate

import (
	"context"
	"errors"
	"sync"
	"time"

	ecsmeta "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TaskARNTagName is the name of the tag that carries the ARN of the ECS task the Agent runs in.
const TaskARNTagName = "task_arn"

const (
	taskARNLookupTimeout    = 2 * time.Second
	taskARNLookupRetryDelay = 30 * time.Second
)

// TaskARNTag provides the task_arn:<task ARN> tag of the ECS Fargate task the Agent runs in as a
// sidecar. It's used to identify the task in payloads that don't otherwise carry it.
//
// The ARN is looked up from the ECS task metadata endpoint on first use and cached once found.
// The endpoint can be briefly unavailable when a task starts, so a failed lookup is retried on a
// later call, at most once every taskARNLookupRetryDelay.
type TaskARNTag struct {
	enabled    bool
	getTaskARN func(context.Context) (string, error)
	now        func() time.Time

	mu         sync.Mutex
	tags       []string
	nextLookup time.Time
}

// NewTaskARNTag returns a TaskARNTag for the current environment. Outside of ECS Fargate, its Tags
// method always returns nil.
func NewTaskARNTag() *TaskARNTag {
	return newTaskARNTag(GetOrchestrator() == ECS, getECSTaskARN, time.Now)
}

func newTaskARNTag(enabled bool, getTaskARN func(context.Context) (string, error), now func() time.Time) *TaskARNTag {
	return &TaskARNTag{enabled: enabled, getTaskARN: getTaskARN, now: now}
}

// Tags returns []string{"task_arn:<task ARN>"} when running in ECS Fargate and the ARN is known,
// and nil otherwise.
func (t *TaskARNTag) Tags() []string {
	if !t.enabled {
		return nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.tags != nil || t.now().Before(t.nextLookup) {
		return t.tags
	}

	ctx, cancel := context.WithTimeout(context.Background(), taskARNLookupTimeout)
	defer cancel()
	arn, err := t.getTaskARN(ctx)
	if err == nil && arn == "" {
		err = errors.New("empty task ARN")
	}
	if err != nil {
		t.nextLookup = t.now().Add(taskARNLookupRetryDelay)
		log.Warnf("unable to get the ECS task ARN, retrying in %s: %v", taskARNLookupRetryDelay, err)
		return nil
	}

	t.tags = []string{TaskARNTagName + ":" + arn}
	return t.tags
}

// getECSTaskARN uses the v2 metadata endpoint, which ECS Fargate always exposes and which
// doesn't need the docker build tag.
func getECSTaskARN(ctx context.Context) (string, error) {
	client, err := ecsmeta.V2()
	if err != nil {
		return "", err
	}
	task, err := client.GetTask(ctx)
	if err != nil {
		return "", err
	}
	return task.TaskARN, nil
}
