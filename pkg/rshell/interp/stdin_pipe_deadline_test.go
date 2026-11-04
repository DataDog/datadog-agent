// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"context"
	"io"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMaxExecutionTimeBoundsBuiltinBlockedOnStdinPipe verifies that a builtin
// blocked reading a runner stdin *os.File pipe that never receives data is
// released when the run's deadline fires.
func TestMaxExecutionTimeBoundsBuiltinBlockedOnStdinPipe(t *testing.T) {
	scripts := map[string]string{
		"cat":       "cat",
		"head":      "head",
		"tail":      "tail",
		"grep":      "grep x",
		"wc":        "wc",
		"sort":      "sort",
		"uniq":      "uniq",
		"tr":        "tr a b",
		"sed":       "sed p",
		"cut":       "cut -c1",
		"strings":   "strings",
		"xargs":     "xargs",
		"jq":        "jq .",
		"read":      "read x",
		"awk":       "awk '{print}'",
		"sha256sum": "sha256sum",
	}
	// read, awk and sha256sum wake at the deadline by themselves; the rest rely
	// on a read deadline on stdin, which Windows anonymous pipes do not support.
	selfBounded := map[string]bool{"read": true, "awk": true, "sha256sum": true}
	for name, script := range scripts {
		t.Run(name, func(t *testing.T) {
			if runtime.GOOS == "windows" && !selfBounded[name] {
				t.Skip("anonymous pipes on Windows have no read deadlines")
			}
			pr, pw, err := os.Pipe()
			require.NoError(t, err)
			t.Cleanup(func() { _ = pr.Close(); _ = pw.Close() })

			r, err := New(allowAllCommandsOpt(), StdIO(pr, io.Discard, io.Discard), MaxExecutionTime(200*time.Millisecond))
			require.NoError(t, err)
			t.Cleanup(func() { _ = r.Close() })

			done := make(chan error, 1)
			go func() { done <- r.Run(context.Background(), parseScript(t, script)) }()
			select {
			case err := <-done:
				assert.ErrorIs(t, err, context.DeadlineExceeded)
			case <-time.After(5 * time.Second):
				t.Fatalf("%s: Run did not return after MaxExecutionTime with an idle stdin pipe", name)
			}
		})
	}
}

// TestStdinPipeDeadlineDoesNotLeakOrStealBytes verifies a normal run leaves
// the caller's pipe usable: no deadline is left behind and no bytes are eaten.
func TestStdinPipeDeadlineDoesNotLeakOrStealBytes(t *testing.T) {
	pr, pw, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = pr.Close(); _ = pw.Close() })

	r, err := New(allowAllCommandsOpt(), StdIO(pr, io.Discard, io.Discard), MaxExecutionTime(5*time.Second))
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })

	_, err = pw.WriteString("one\n")
	require.NoError(t, err)
	require.NoError(t, r.Run(context.Background(), parseScript(t, "read x")))

	// The deadline must have been cleared after Run.
	_, err = pw.WriteString("two\n")
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond)
	buf := make([]byte, 16)
	n, err := pr.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, "two\n", string(buf[:n]))
}
