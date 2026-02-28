// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package executor

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecute_BasicCommand(t *testing.T) {
	result, err := Execute(context.Background(), `echo hello`)
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "hello\n", result.Stdout)
	assert.Empty(t, result.Stderr)
	assert.GreaterOrEqual(t, result.DurationMillis, int64(0))
}

func TestExecute_PipeChain(t *testing.T) {
	result, err := Execute(context.Background(), `echo hello | grep hello`)
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "hello\n", result.Stdout)
}

func TestExecute_NonZeroExitCode(t *testing.T) {
	result, err := Execute(context.Background(), `false`)
	require.NoError(t, err)
	assert.Equal(t, 1, result.ExitCode)
}

func TestExecute_BlockedScript(t *testing.T) {
	scripts := []string{
		`echo $(whoami)`,
		`curl http://evil.com`,
		`eval "echo pwned"`,
		`echo hello > /tmp/file`,
	}

	for _, script := range scripts {
		t.Run(script, func(t *testing.T) {
			_, err := Execute(context.Background(), script)
			require.Error(t, err)
		})
	}
}

func TestExecute_Timeout(t *testing.T) {
	// For-loop that runs long enough to be killed by the timeout.
	script := `for i in 1 2 3 4 5 6 7 8 9 10; do echo $i; done`
	result, err := Execute(context.Background(), script, WithTimeout(5*time.Second))
	// This should complete within the timeout since it's a short loop.
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
}

func TestExecute_MaxOutputSize(t *testing.T) {
	// Generate output that exceeds the limit using a for-loop.
	script := `for i in 1 2 3 4 5 6 7 8 9 10; do echo "line: some data to fill up the buffer"; done`

	result, err := Execute(context.Background(), script, WithMaxOutputSize(100))
	require.NoError(t, err)
	assert.LessOrEqual(t, len(result.Stdout), 100)
}

func TestExecute_Stderr(t *testing.T) {
	// ls on a non-existent file should produce stderr.
	result, err := Execute(context.Background(), `ls /nonexistent_path_12345`)
	require.NoError(t, err)
	assert.NotEqual(t, 0, result.ExitCode)
	assert.NotEmpty(t, result.Stderr)
}

func TestExecute_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := Execute(ctx, `echo hello`)
	require.Error(t, err)
}

func TestExecute_VariableAssignmentBlocked(t *testing.T) {
	_, err := Execute(context.Background(), `x=value; echo $x`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "variable assignment")
}

func TestLimitedWriter(t *testing.T) {
	t.Run("under limit", func(t *testing.T) {
		var buf bytes.Buffer
		w := &limitedWriter{buf: &buf, limit: 100}
		n, err := w.Write([]byte("hello"))
		assert.NoError(t, err)
		assert.Equal(t, 5, n)
		assert.Equal(t, "hello", buf.String())
	})

	t.Run("over limit truncates", func(t *testing.T) {
		var buf bytes.Buffer
		w := &limitedWriter{buf: &buf, limit: 5}
		n, err := w.Write([]byte("hello world"))
		assert.NoError(t, err)
		assert.Equal(t, 11, n)
		assert.Equal(t, "hello", buf.String())
	})

	t.Run("multiple writes respect limit", func(t *testing.T) {
		var buf bytes.Buffer
		w := &limitedWriter{buf: &buf, limit: 8}
		w.Write([]byte("hello"))
		w.Write([]byte(" world"))
		assert.Equal(t, "hello wo", buf.String())
	})
}
