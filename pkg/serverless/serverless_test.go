// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverless

import (
	"fmt"
	"os"
	"runtime/debug"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/serverless/daemon"
	"github.com/DataDog/datadog-agent/pkg/serverless/flush"
	"github.com/DataDog/datadog-agent/pkg/serverless/tags"
)

func TestMain(m *testing.M) {
	origShutdownDelay := daemon.ShutdownDelay
	daemon.ShutdownDelay = 0
	defer func() { daemon.ShutdownDelay = origShutdownDelay }()
	os.Exit(m.Run())
}

func TestHandleInvocationShouldSetExtraTags(t *testing.T) {
	d := daemon.StartDaemon("http://localhost:8124")
	defer d.Stop()

	// force daemon not to wait for flush at end of handleInvocation
	d.SetFlushStrategy(flush.NewPeriodically(time.Second))
	d.UseAdaptiveFlush(false)

	// deadline = current time + 5s
	deadlineMs := (time.Now().UnixNano())/1000000 + 5000

	// setting DD_TAGS and DD_EXTRA_TAGS
	t.Setenv("DD_TAGS", "a1:valueA1,a2:valueA2,A_MAJ:valueAMaj")
	t.Setenv("DD_EXTRA_TAGS", "a3:valueA3 a4:valueA4")

	callInvocationHandler(d, "arn:aws:lambda:us-east-1:123456789012:function:my-function", deadlineMs, 0, "myRequestID", handleInvocation)
	architecture := fmt.Sprintf("architecture:%s", tags.ResolveRuntimeArch())

	assert.Equal(t, 14, len(d.ExtraTags.Tags))

	sort.Strings(d.ExtraTags.Tags)
	assert.Equal(t, "a1:valuea1", d.ExtraTags.Tags[0])
	assert.Equal(t, "a2:valuea2", d.ExtraTags.Tags[1])
	assert.Equal(t, "a3:valuea3", d.ExtraTags.Tags[2])
	assert.Equal(t, "a4:valuea4", d.ExtraTags.Tags[3])
	assert.Equal(t, "a_maj:valueamaj", d.ExtraTags.Tags[4])
	assert.Equal(t, "account_id:123456789012", d.ExtraTags.Tags[5])
	assert.Equal(t, architecture, d.ExtraTags.Tags[6])
	assert.Equal(t, "aws_account:123456789012", d.ExtraTags.Tags[7])
	assert.Equal(t, "dd_extension_version:xxx", d.ExtraTags.Tags[8])
	assert.Equal(t, "function_arn:arn:aws:lambda:us-east-1:123456789012:function:my-function", d.ExtraTags.Tags[9])
	assert.Equal(t, "functionname:my-function", d.ExtraTags.Tags[10])
	assert.Equal(t, "region:us-east-1", d.ExtraTags.Tags[11])
	assert.Equal(t, "resource:my-function", d.ExtraTags.Tags[12])
	assert.True(t, d.ExtraTags.Tags[13] == "runtime:unknown" || d.ExtraTags.Tags[13] == "runtime:provided.al2")

	ecs := d.ExecutionContext.GetCurrentState()
	assert.Equal(t, "arn:aws:lambda:us-east-1:123456789012:function:my-function", ecs.ARN)
	assert.Equal(t, "myRequestID", ecs.LastRequestID)
}

func TestHandleInvocationShouldNotSIGSEGVWhenTimedOut(t *testing.T) {
	currentPanicOnFaultBehavior := debug.SetPanicOnFault(true)
	defer debug.SetPanicOnFault(currentPanicOnFaultBehavior)
	defer func() {
		r := recover()
		if r != nil {
			assert.Fail(t, "Expected no panic, instead got ", r)
		}
	}()

	for i := 0; i < 10; i++ { // each one of these takes about a second on my laptop
		fmt.Printf("Running this test the %d time\n", i)
		d := daemon.StartDaemon("http://localhost:8124")
		d.WaitForDaemon()

		//deadline = current time - 20 ms
		deadlineMs := (time.Now().UnixNano())/1000000 - 20

		callInvocationHandler(d, "arn:aws:lambda:us-east-1:123456789012:function:my-function", deadlineMs, 0, "myRequestID", handleInvocation)
		d.Stop()
	}
	//before 8682842e9202a4984a38b00fdf427837c9e2d46b, if this was the Daemon's first invocation, the Go scheduler (trickster spirit)
	//might try to execute TellDaemonRuntimeDone before TellDaemonRuntimeStarted, which would result in a SIGSEGV. Now this should never happen.
}

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
