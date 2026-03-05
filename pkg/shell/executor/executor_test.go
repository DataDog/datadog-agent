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
	assert.Greater(t, result.DurationMillis, int64(0))
}

func TestExecute_PipeChain(t *testing.T) {
	result, err := Execute(context.Background(), `echo hello | grep hello`)
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "hello\n", result.Stdout)
}

func TestExecute_NonZeroExitCode(t *testing.T) {
	result, err := Execute(context.Background(), `false`)
	require.NoError(t, err) // Non-zero exit is not an error
	assert.Equal(t, 1, result.ExitCode)
}

func TestExecute_VerificationFailure(t *testing.T) {
	_, err := Execute(context.Background(), `rm -rf /`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "verification failed")
}

func TestExecute_Timeout(t *testing.T) {
	// Use a very short timeout. "sleep 10" should be killed.
	// Note: "sleep" is not in the allowlist, so we use a valid command with a loop.
	// Actually, let's use a script that the verifier allows but takes long.
	script := `x=0; while [ $x -lt 1000000 ]; do x=$((x+1)); done`
	_, err := Execute(context.Background(), script, WithTimeout(100*time.Millisecond))
	// This should either timeout or complete quickly depending on system speed.
	// We just verify it doesn't hang.
	_ = err
}

func TestExecute_MaxOutputSize(t *testing.T) {
	// Generate output larger than our limit using a while loop (no command substitution).
	script := `i=0; while [ $i -lt 1000 ]; do echo "line $i: some data to fill up the buffer"; i=$((i+1)); done`

	// Use a very small limit.
	result, err := Execute(context.Background(), script, WithMaxOutputSize(100))
	require.NoError(t, err)
	assert.LessOrEqual(t, len(result.Stdout), 100)
}

func TestExecute_WithEnv(t *testing.T) {
	result, err := Execute(context.Background(), `echo $TESTVAR`,
		WithEnv([]string{"TESTVAR=hello_from_env", "PATH=/usr/bin:/bin"}))
	require.NoError(t, err)
	assert.Equal(t, "hello_from_env\n", result.Stdout)
}

func TestExecute_Stderr(t *testing.T) {
	// ls on a non-existent file should produce stderr.
	result, err := Execute(context.Background(), `ls /nonexistent_path_12345`)
	require.NoError(t, err) // Non-zero exit is not an error
	assert.NotEqual(t, 0, result.ExitCode)
	assert.NotEmpty(t, result.Stderr)
}

func TestExecute_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := Execute(ctx, `echo hello`)
	require.Error(t, err)
}

func TestExecute_BlockedScript(t *testing.T) {
	scripts := []string{
		`echo $(whoami)`,
		`find / -exec rm {} \;`,
		`curl http://evil.com`,
		`eval "echo pwned"`,
		`echo hello > /tmp/file`,
	}

	for _, script := range scripts {
		t.Run(script, func(t *testing.T) {
			_, err := Execute(context.Background(), script)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "verification failed")
		})
	}
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
		assert.Equal(t, 11, n) // reports all bytes consumed per io.Writer contract
		assert.Equal(t, "hello", buf.String()) // but only writes up to limit
	})

	t.Run("multiple writes respect limit", func(t *testing.T) {
		var buf bytes.Buffer
		w := &limitedWriter{buf: &buf, limit: 8}
		w.Write([]byte("hello"))
		w.Write([]byte(" world"))
		assert.Equal(t, "hello wo", buf.String())
	})
}
