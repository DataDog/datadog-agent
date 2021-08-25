// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverless

import (
	"os"
	"sort"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/serverless/daemon"
	"github.com/stretchr/testify/assert"
)

func TestHandleInvocationShouldSetExtraTags(t *testing.T) {
	d := daemon.StartDaemon("http://localhost:8124")
	defer d.Stop()

	d.SetClientReady(false)
	d.WaitForDaemon()

	d.StartInvocation()

	//deadline = current time + 20 ms
	deadlineMs := (time.Now().UnixNano())/1000000 + 20

	//setting DD_TAGS and DD_EXTRA_TAGS
	os.Setenv("DD_TAGS", "a1:valueA1,a2:valueA2,A_MAJ:valueAMaj")
	os.Setenv("DD_EXTRA_TAGS", "a3:valueA3 a4:valueA4")

	callInvocationHandler(d, "arn:aws:lambda:us-east-1:123456789012:function:my-function", deadlineMs, 0, "myRequestID", handleInvocation)

	expectedTagArray := []string{
		"a1:valuea1",
		"a2:valuea2",
		"a3:valuea3",
		"a4:valuea4",
		"a_maj:valueamaj",
		"account_id:123456789012",
		"aws_account:123456789012",
		"dd_extension_version:xxx",
		"function_arn:arn:aws:lambda:us-east-1:123456789012:function:my-function",
		"functionname:my-function",
		"region:us-east-1",
		"resource:my-function",
	}

	sort.Strings(d.ExtraTags.Tags)
	assert.Equal(t, expectedTagArray, d.ExtraTags.Tags)
	assert.Equal(t, "arn:aws:lambda:us-east-1:123456789012:function:my-function", d.ExecutionContext.ARN)
	assert.Equal(t, "myRequestID", d.ExecutionContext.LastRequestID)
}

func TestComputeTimeout(t *testing.T) {
	fakeCurrentTime := time.Date(2021, time.January, 1, 0, 0, 0, 0, time.UTC)
	// add 10ms
	fakeDeadLineInMs := fakeCurrentTime.UnixNano()/int64(time.Millisecond) + 10
	safetyBuffer := 3 * time.Millisecond
	assert.Equal(t, 7*time.Millisecond, computeTimeout(fakeCurrentTime, fakeDeadLineInMs, safetyBuffer))
}
