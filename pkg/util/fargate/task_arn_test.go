// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fargate

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const testTaskARN = "arn:aws:ecs:us-east-1:123456789012:task/cluster/0123456789abcdef"

func TestTaskARNTagDisabled(t *testing.T) {
	called := false
	tag := newTaskARNTag(false, func(context.Context) (string, error) {
		called = true
		return testTaskARN, nil
	}, time.Now)

	assert.Nil(t, tag.Tags())
	assert.False(t, called)
}

func TestTaskARNTagCachesTheARN(t *testing.T) {
	calls := 0
	tag := newTaskARNTag(true, func(context.Context) (string, error) {
		calls++
		return testTaskARN, nil
	}, time.Now)

	assert.Equal(t, []string{"task_arn:" + testTaskARN}, tag.Tags())
	assert.Equal(t, []string{"task_arn:" + testTaskARN}, tag.Tags())
	assert.Equal(t, 1, calls)
}

func TestTaskARNTagRetriesAfterDelay(t *testing.T) {
	now := time.Unix(1000, 0)
	calls := 0
	tag := newTaskARNTag(true, func(context.Context) (string, error) {
		calls++
		switch calls {
		case 1:
			return "", errors.New("metadata endpoint not ready")
		case 2:
			return "", nil
		default:
			return testTaskARN, nil
		}
	}, func() time.Time { return now })

	assert.Nil(t, tag.Tags(), "lookup error")
	assert.Nil(t, tag.Tags(), "no new lookup before the retry delay")
	assert.Equal(t, 1, calls)

	now = now.Add(taskARNLookupRetryDelay)
	assert.Nil(t, tag.Tags(), "empty ARN")
	assert.Equal(t, 2, calls)

	now = now.Add(taskARNLookupRetryDelay)
	assert.Equal(t, []string{"task_arn:" + testTaskARN}, tag.Tags())
	assert.Equal(t, 3, calls)
}
