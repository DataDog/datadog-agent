// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package initcontainer

import (
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"syscall"
	"testing"
	"time"

	serverlessLog "github.com/DataDog/datadog-agent/cmd/serverless-init/log"

	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/stretchr/testify/assert"
)

func TestBuildCommandParamWithArgs(t *testing.T) {
	name, args := buildCommandParam([]string{"superCmd", "--verbose", "path", "-i", "."})
	assert.Equal(t, "superCmd", name)
	assert.Equal(t, []string{"--verbose", "path", "-i", "."}, args)
}

func TestBuildCommandParam(t *testing.T) {
	name, args := buildCommandParam([]string{"superCmd"})
	assert.Equal(t, "superCmd", name)
	assert.Equal(t, []string{}, args)
}

type TestTimeoutFlushableAgent struct {
	hasBeenCalled bool
}

type TestFlushableAgent struct {
	hasBeenCalled bool
}

func (tfa *TestTimeoutFlushableAgent) Flush() {
	time.Sleep(1 * time.Hour)
	tfa.hasBeenCalled = true
}

func (tfa *TestFlushableAgent) Flush() {
	tfa.hasBeenCalled = true
}

func TestFlushSucess(t *testing.T) {
	metricAgent := &TestFlushableAgent{}
	traceAgent := &TestFlushableAgent{}
	mockLogsAgent := logsAgent.NewMockServerlessLogsAgent()
	flush(100*time.Millisecond, metricAgent, traceAgent, mockLogsAgent)
	assert.Equal(t, true, metricAgent.hasBeenCalled)
	assert.Equal(t, true, mockLogsAgent.DidFlush())
}

func TestFlushTimeout(t *testing.T) {
	metricAgent := &TestTimeoutFlushableAgent{}
	traceAgent := &TestTimeoutFlushableAgent{}
	mockLogsAgent := logsAgent.NewMockServerlessLogsAgent()
	mockLogsAgent.SetFlushDelay(time.Hour)

	flush(100*time.Millisecond, metricAgent, traceAgent, mockLogsAgent)
	assert.Equal(t, false, metricAgent.hasBeenCalled)
	assert.Equal(t, false, mockLogsAgent.DidFlush())
}

func TestPropagateChildSuccess(t *testing.T) {
	runTestOnLinuxOnly(t, func(t *testing.T) {
		err := execute(&serverlessLog.Config{}, []string{"bash", "-c", "exit 0"})
		assert.Equal(t, nil, err)
	})
}

func TestPropagateChildError(t *testing.T) {
	runTestOnLinuxOnly(t, func(t *testing.T) {
		expectedError := 123
		err := execute(&serverlessLog.Config{}, []string{"bash", "-c", "exit " + strconv.Itoa(expectedError)})
		assert.Equal(t, expectedError<<8, int(err.(*exec.ExitError).ProcessState.Sys().(syscall.WaitStatus)))
	})
}

func TestForwardSignalToChild(t *testing.T) {
	runTestOnLinuxOnly(t, func(t *testing.T) {
		resultChan := make(chan error)
		terminatingSignal := syscall.SIGUSR1
		cmd := exec.Command("sleep", "2s")
		cmd.Start()
		sigs := make(chan os.Signal, 1)
		go forwardSignals(cmd.Process, sigs)

		go func() {
			err := cmd.Wait()
			resultChan <- err
		}()

		sigs <- syscall.SIGSTOP
		sigs <- syscall.SIGCONT
		sigs <- terminatingSignal

		err := <-resultChan
		assert.Equal(t, int(terminatingSignal), int(err.(*exec.ExitError).ProcessState.Sys().(syscall.WaitStatus)))
	},
	)
}

func runTestOnLinuxOnly(t *testing.T, targetTest func(*testing.T)) {
	if runtime.GOOS == "linux" {
		t.Run("Test on Linux", func(t *testing.T) {
			targetTest(t)
		})
	} else {
		t.Skip("Test case is skipped on this platform")
	}
}
