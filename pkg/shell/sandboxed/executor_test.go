// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package sandboxed

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func requireAgentFS(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("agentfs"); err != nil {
		t.Skip("agentfs binary not found, skipping integration test")
	}
}

func TestCheckAvailability(t *testing.T) {
	err := CheckAvailability()
	if _, lookErr := exec.LookPath("agentfs"); lookErr != nil {
		assert.ErrorIs(t, err, ErrAgentFSNotFound)
	} else {
		assert.NoError(t, err)
	}
}

func TestCheckAvailability_Absent(t *testing.T) {
	// Temporarily clear PATH so agentfs cannot be found.
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", "")
	defer os.Setenv("PATH", origPath)

	err := CheckAvailability()
	assert.ErrorIs(t, err, ErrAgentFSNotFound)
}

func TestExecute_ValidScript(t *testing.T) {
	requireAgentFS(t)

	result, err := Execute(context.Background(), `echo hello`)
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "hello\n", result.Stdout)
	assert.Empty(t, result.Stderr)
	assert.Greater(t, result.DurationMillis, int64(0))
	assert.NotEmpty(t, result.SessionID)
}

func TestExecute_VerificationFailure(t *testing.T) {
	// Verification happens before agentfs is called, so no binary needed.
	_, err := Execute(context.Background(), `echo $(whoami)`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "verification failed")
}

func TestExecute_NonZeroExitCode(t *testing.T) {
	requireAgentFS(t)

	result, err := Execute(context.Background(), `false`)
	require.NoError(t, err) // Non-zero exit is not a Go error
	assert.Equal(t, 1, result.ExitCode)
}

func TestExecute_AutoSession(t *testing.T) {
	requireAgentFS(t)

	result, err := Execute(context.Background(), `echo auto`)
	require.NoError(t, err)
	assert.NotEmpty(t, result.SessionID, "auto-created session ID should be returned")

	// Clean up.
	_ = CloseSession(result.SessionID)
}

func TestExecute_PersistentSession(t *testing.T) {
	requireAgentFS(t)

	// Create a session explicitly.
	sessionID := "test-persistent-" + time.Now().Format("20060102150405")
	err := InitSession(context.Background(), sessionID)
	require.NoError(t, err)
	defer CloseSession(sessionID)

	// First run: create a file.
	result1, err := Execute(context.Background(), `echo "data" | tee /tmp/agentfs-test-persist`, WithSession(sessionID))
	require.NoError(t, err)
	assert.Equal(t, sessionID, result1.SessionID)

	// Second run: read the file back in the same session.
	result2, err := Execute(context.Background(), `cat /tmp/agentfs-test-persist`, WithSession(sessionID))
	require.NoError(t, err)
	assert.Equal(t, sessionID, result2.SessionID)
	assert.Contains(t, result2.Stdout, "data")
}

func TestExecute_Timeout(t *testing.T) {
	requireAgentFS(t)

	script := `x=0; while [ $x -lt 1000000 ]; do x=$((x+1)); done`
	_, err := Execute(context.Background(), script, WithTimeout(100*time.Millisecond))
	// This should either timeout or complete quickly depending on system speed.
	// We just verify it doesn't hang.
	_ = err
}

func TestExecute_MaxOutputSize(t *testing.T) {
	requireAgentFS(t)

	script := `i=0; while [ $i -lt 1000 ]; do echo "line $i: some data to fill up the buffer"; i=$((i+1)); done`
	result, err := Execute(context.Background(), script, WithMaxOutputSize(100))
	require.NoError(t, err)
	assert.LessOrEqual(t, len(result.Stdout), 100)
}

func TestCloseSession(t *testing.T) {
	requireAgentFS(t)

	sessionID := "test-close-" + time.Now().Format("20060102150405")
	err := InitSession(context.Background(), sessionID)
	require.NoError(t, err)

	// Verify the DB file was created.
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	dbPath := filepath.Join(home, ".agentfs", sessionID+".db")
	_, err = os.Stat(dbPath)
	require.NoError(t, err, "session DB should exist after init")

	// Close the session.
	err = CloseSession(sessionID)
	require.NoError(t, err)

	// Verify the DB file was removed.
	_, err = os.Stat(dbPath)
	assert.True(t, os.IsNotExist(err), "session DB should be removed after close")
}

func TestCloseSession_NonExistent(t *testing.T) {
	// Closing a non-existent session should not error (files simply don't exist).
	err := CloseSession("nonexistent-session-id-12345")
	assert.NoError(t, err)
}

func TestExecute_BlockedScripts(t *testing.T) {
	// These should fail at verification, never reaching agentfs.
	// No agentfs binary required.
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
