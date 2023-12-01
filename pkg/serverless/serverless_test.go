// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverless

import (
	"fmt"
	"net"
	"runtime/debug"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/serverless/daemon"
	"github.com/DataDog/datadog-agent/comp/serverless/daemon/daemonimpl"
	"github.com/DataDog/datadog-agent/pkg/serverless/registration"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

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

		// wait for port 8125 to be available since the DogStatsD Server started by Daemon needs it open
		waitForUDPPort(8125, 1*time.Second)

		d := fxutil.Test[daemon.Mock](t, fx.Supply(daemonimpl.Params{Addr: "http://localhost:8124", SketchesBucketOffset: time.Second * 10}), daemonimpl.MockModule)
		d.Start(time.Now(), "/var/task/datadog.yaml", registration.ID("1"), registration.FunctionARN("arn:1"))
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

func waitForUDPPort(port int, timeout time.Duration) {
	address := fmt.Sprintf("127.0.0.1:%d", port)

	startTime := time.Now()

	for {
		conn, err := net.DialTimeout("udp", address, timeout)

		if err == nil {
			// Successfully dialed the address, so the port is available
			log.Debugf("Port %d is now open for listening.\n", port)
			conn.Close()
			return
		}

		// Check if the timeout has been reached
		if time.Since(startTime) >= timeout {
			log.Debugf("Timed out waiting for port %d to become available", port)
			return
		}

		// Port not available yet, retry after a short interval
		time.Sleep(100 * time.Millisecond)
	}
}
