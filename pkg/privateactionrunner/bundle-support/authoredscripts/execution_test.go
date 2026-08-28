// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package authoredscripts

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteCommand_Success(t *testing.T) {
	cmd := exec.CommandContext(context.Background(), "/bin/sh", "-c", "echo hello; echo world 1>&2")

	result, err := ExecuteCommand(context.Background(), cmd)

	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "hello\n", result.Stdout)
	assert.Equal(t, "world\n", result.Stderr)
}

func TestExecuteCommand_NonZeroExit(t *testing.T) {
	cmd := exec.CommandContext(context.Background(), "/bin/sh", "-c", "echo boom 1>&2; exit 3")

	result, err := ExecuteCommand(context.Background(), cmd)

	require.Error(t, err)
	assert.Equal(t, 3, result.ExitCode)
	assert.Contains(t, err.Error(), "exit code 3")
	assert.Contains(t, err.Error(), "boom")
}

func TestExecuteCommand_NilContext(t *testing.T) {
	cmd := exec.Command("/bin/echo", "hi")

	_, err := ExecuteCommand(nil, cmd) //nolint:staticcheck

	require.Error(t, err)
	assert.Contains(t, err.Error(), "context is required")
}

func TestExecuteCommand_ContextTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", "sleep 5")

	_, err := ExecuteCommand(ctx, cmd)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
	assert.True(t, errors.Is(err, context.DeadlineExceeded))
}

func TestExecuteCommand_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", "sleep 5")
	cancel()

	_, err := ExecuteCommand(ctx, cmd)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "canceled")
	assert.NotContains(t, err.Error(), "timed out")
}

func TestExecuteCommand_OutputLimitExceeded(t *testing.T) {
	cmd := exec.CommandContext(context.Background(), "yes")

	result, err := executeCommand(context.Background(), cmd, 100)

	require.Error(t, err)
	assert.True(t, errors.Is(err, errOutputLimitExceeded))
	assert.LessOrEqual(t, len(result.Stdout), 100)
}

func TestExecuteCommand_ReapsProcessGroup(t *testing.T) {
	cmd := exec.CommandContext(context.Background(), "/bin/sh", "-c", "sleep 30 </dev/null >/dev/null 2>&1 & echo $!")
	configureCommand(cmd)

	result, err := ExecuteCommand(context.Background(), cmd)

	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	childPID, err := strconv.Atoi(strings.TrimSpace(result.Stdout))
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = syscall.Kill(childPID, syscall.SIGKILL)
	})
	require.Eventually(t, func() bool {
		return errors.Is(syscall.Kill(childPID, 0), syscall.ESRCH)
	}, time.Second, 10*time.Millisecond, "background process %d was not terminated", childPID)
}

func TestExecuteCommand_OutputLimitTerminatesProcessIgnoringSIGPIPE(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestExecuteCommand_SIGPIPEHelperProcess")
	cmd.Env = append(os.Environ(), "GO_WANT_SIGPIPE_HELPER_PROCESS=1")
	configureCommand(cmd)

	result, err := executeCommand(ctx, cmd, 100)

	require.ErrorIs(t, err, errOutputLimitExceeded)
	require.NoError(t, ctx.Err(), "output exhaustion should terminate the process before the safety timeout")
	assert.LessOrEqual(t, len(result.Stdout)+len(result.Stderr), 100)
}

func TestExecuteCommand_SIGPIPEHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_SIGPIPE_HELPER_PROCESS") != "1" {
		return
	}

	signal.Ignore(syscall.SIGPIPE)
	output := bytes.Repeat([]byte("x"), 4096)
	for {
		_, _ = os.Stdout.Write(output)
	}
}

func TestExecuteCommand_ConcurrentExecutionsAreIndependent(t *testing.T) {
	const executionCount = 8
	type outcome struct {
		shouldReachLimit bool
		result           Result
		err              error
		ctxErr           error
	}

	start := make(chan struct{})
	outcomes := make(chan outcome, executionCount)
	for i := 0; i < executionCount; i++ {
		shouldReachLimit := i%2 == 0
		go func(shouldReachLimit bool) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			var cmd *exec.Cmd
			if shouldReachLimit {
				cmd = exec.CommandContext(ctx, os.Args[0], "-test.run=TestExecuteCommand_SIGPIPEHelperProcess")
				cmd.Env = append(os.Environ(), "GO_WANT_SIGPIPE_HELPER_PROCESS=1")
			} else {
				cmd = exec.CommandContext(ctx, "/bin/echo", "ok")
			}
			configureCommand(cmd)

			<-start
			result, err := executeCommand(ctx, cmd, 100)
			ctxErr := ctx.Err()
			cancel()
			outcomes <- outcome{shouldReachLimit: shouldReachLimit, result: result, err: err, ctxErr: ctxErr}
		}(shouldReachLimit)
	}
	close(start)

	for i := 0; i < executionCount; i++ {
		result := <-outcomes
		require.NoError(t, result.ctxErr, "execution should finish before its safety timeout")
		if result.shouldReachLimit {
			require.ErrorIs(t, result.err, errOutputLimitExceeded)
			assert.LessOrEqual(t, len(result.result.Stdout)+len(result.result.Stderr), 100)
			continue
		}
		require.NoError(t, result.err)
		assert.Equal(t, "ok\n", result.result.Stdout)
	}
}

func TestExecuteCommand_StderrTruncatedInError(t *testing.T) {
	longStderr := strings.Repeat("e", maxErrorStderrSize+1000)
	cmd := exec.CommandContext(context.Background(), "/bin/sh", "-c", "printf '%s' \"$LONG_STDERR\" 1>&2; exit 1")
	cmd.Env = append(cmd.Env, "LONG_STDERR="+longStderr)

	_, err := ExecuteCommand(context.Background(), cmd)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "[truncated]")
	assert.LessOrEqual(t, len(err.Error()), maxErrorStderrSize+500)
}

func TestExecuteCommand_CommandRequired(t *testing.T) {
	_, err := executeCommand(context.Background(), nil, defaultOutputLimit)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "command is required")
}

func TestExecuteCommand_OutputLimitMustBePositive(t *testing.T) {
	cmd := exec.Command("/bin/echo", "hi")

	_, err := executeCommand(context.Background(), cmd, 0)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be positive")
}

func TestErrorStderr_ShortIsUnchanged(t *testing.T) {
	assert.Equal(t, "boom", errorStderr("  boom  \n"))
}

func TestErrorStderr_LongIsTruncatedFromTheEnd(t *testing.T) {
	long := strings.Repeat("a", maxErrorStderrSize) + "TAIL"

	result := errorStderr(long)

	assert.True(t, strings.HasPrefix(result, "[truncated] "))
	assert.True(t, strings.HasSuffix(result, "TAIL"))
	assert.LessOrEqual(t, len(result)-len("[truncated] "), maxErrorStderrSize)
}
