// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverless

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestComputeTimeout(t *testing.T) {
	fakeCurrentTime := time.Date(2021, time.January, 1, 0, 0, 0, 0, time.UTC)
	// add 10m
	fakeDeadLineInMs := fakeCurrentTime.UnixNano()/int64(time.Millisecond) + 10
	safetyBuffer := 3 * time.Millisecond
	assert.Equal(t, 7*time.Millisecond, computeTimeout(fakeCurrentTime, fakeDeadLineInMs, safetyBuffer))
}

func TestRemoveQualifierFromArnWithAlias(t *testing.T) {
	invokedFunctionArn := "arn:aws:lambda:eu-south-1:425362996713:function:inferred-spans-function-urls-dev-harv-function-urls:$latest"
	functionArn := removeQualifierFromArn(invokedFunctionArn)
	expectedArn := "arn:aws:lambda:eu-south-1:425362996713:function:inferred-spans-function-urls-dev-harv-function-urls"
	assert.Equal(t, functionArn, expectedArn)
}

func TestRemoveQualifierFromArnWithoutAlias(t *testing.T) {
	invokedFunctionArn := "arn:aws:lambda:eu-south-1:425362996713:function:inferred-spans-function-urls-dev-harv-function-urls"
	functionArn := removeQualifierFromArn(invokedFunctionArn)
	assert.Equal(t, functionArn, invokedFunctionArn)
}

type mockLifecycleProcessor struct {
	isError         bool
	isTimeout       bool
	isColdStart     bool
	isProactiveInit bool
}
