// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunnerRedir_ReadOnlyModeBlocksFileTarget verifies that file-target output
// redirects are blocked at validation time (exit 2) in read-only mode.
func TestRunnerRedir_ReadOnlyModeBlocksFileTarget(t *testing.T) {
	scripts := []string{
		"echo x > out.txt",
		"echo x >> out.txt",
		"echo x 2> out.txt",
		"echo x &> out.txt",
	}
	for _, script := range scripts {
		t.Run(script, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			r, err := New(allowAllCommandsOpt(), StdIO(nil, &stdout, &stderr))
			require.NoError(t, err)
			t.Cleanup(func() { _ = r.Close() })

			err = r.Run(context.Background(), parseScript(t, script))
			require.Error(t, err)
			var es ExitStatus
			require.True(t, errors.As(err, &es))
			assert.Equal(t, ExitStatus(2), es, "read-only file redirect must be exit 2, not %d", int(es))
			assert.Empty(t, stdout.String())
		})
	}
}

// TestRunnerRedir_DevNullAlwaysAccepted verifies /dev/null redirects work in
// both modes without touching the sandbox.
func TestRunnerRedir_DevNullAlwaysAccepted(t *testing.T) {
	cases := []struct {
		script     string
		wantStdout string
	}{
		// > /dev/null discards stdout; nothing on stdout.
		{"echo x > /dev/null", ""},
		// 2> /dev/null discards stderr; stdout still has the echo output.
		{"echo x 2> /dev/null", "x\n"},
		// &> /dev/null discards both; nothing on stdout.
		{"echo x &> /dev/null", ""},
	}
	for _, remMode := range []bool{false, true} {
		for _, tc := range cases {
			t.Run(tc.script, func(t *testing.T) {
				var stdout, stderr bytes.Buffer
				opts := []RunnerOption{allowAllCommandsOpt(), StdIO(nil, &stdout, &stderr)}
				if remMode {
					opts = append(opts, WithMode(ModeRemediation))
				}
				r, err := New(opts...)
				require.NoError(t, err)
				t.Cleanup(func() { _ = r.Close() })

				err = r.Run(context.Background(), parseScript(t, tc.script))
				require.NoError(t, err, "stderr=%q", stderr.String())
				assert.Equal(t, tc.wantStdout, stdout.String())
			})
		}
	}
}

// TestRunnerRedir_RemediationModeTruncatesOnOverwrite verifies that a second >
// truncates the previous content.
func TestRunnerRedir_RemediationModeTruncatesOnOverwrite(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "out.txt"), []byte("old-content\n"), 0o600))

	var stdout, stderr bytes.Buffer
	r, err := New(
		allowAllCommandsOpt(),
		StdIO(nil, &stdout, &stderr),
		AllowedPaths([]string{dir + ":rw"}),
		WithMode(ModeRemediation),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })
	r.Dir = dir

	err = r.Run(context.Background(), parseScript(t, "echo new > out.txt"))
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "out.txt"))
	require.NoError(t, err)
	assert.Equal(t, "new\n", string(data), "second > must truncate, not append")
}

// TestRunnerRedir_FdCheckPreventsCommandSubstitution verifies that fd validation
// happens before word expansion. In remediation mode, 3>$(cmd) must be caught
// at validation time (exit 2) — the substitution must not execute.
func TestRunnerRedir_FdCheckPreventsCommandSubstitution(t *testing.T) {
	dir := t.TempDir()
	sentinel := filepath.Join(dir, "executed")

	var stdout, stderr bytes.Buffer
	r, err := New(
		allowAllCommandsOpt(),
		StdIO(nil, &stdout, &stderr),
		AllowedPaths([]string{dir}),
		WithMode(ModeRemediation),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })
	r.Dir = dir

	// If the substitution runs, it creates the sentinel file.
	script := `echo x 3>$(echo ` + sentinel + `)`
	err = r.Run(context.Background(), parseScript(t, script))
	require.Error(t, err)

	var es ExitStatus
	require.True(t, errors.As(err, &es))
	assert.Equal(t, ExitStatus(2), es, "3> must be rejected at validation, not at runtime")

	_, statErr := os.Stat(sentinel)
	assert.True(t, errors.Is(statErr, os.ErrNotExist),
		"command substitution inside 3> target must not execute; sentinel was created")
}
