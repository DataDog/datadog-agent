// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package xargs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// infiniteReader yields bytes forever. Used to verify that the tokenizer
// honours context cancellation under DoS conditions (RULES.md "Special File
// Handling" — /dev/zero analogue).
type infiniteReader struct{ b byte }

func (r *infiniteReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = r.b
	}
	return len(p), nil
}

func newPentestCallCtx(stdout, stderr *bytes.Buffer) *builtins.CallContext {
	return &builtins.CallContext{
		Stdout: stdout,
		Stderr: stderr,
	}
}

// TestInvokeCommandPrefersWithStdin verifies that when both RunCommand and
// RunCommandWithStdin are wired, xargs prefers the stdin-aware variant and
// passes a non-nil empty reader (POSIX /dev/null analogue).
func TestInvokeCommandPrefersWithStdin(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cc := newPentestCallCtx(&stdout, &stderr)
	cc.RunCommand = func(_ context.Context, _ string, _ string, _ []string) (uint8, error) {
		t.Fatal("RunCommand should not be called when RunCommandWithStdin is set")
		return 0, nil
	}
	withStdinCalled := false
	cc.RunCommandWithStdin = func(_ context.Context, _ string, _ string, _ []string, stdin io.Reader) (uint8, error) {
		withStdinCalled = true
		require.NotNil(t, stdin, "child stdin must be a real reader, not nil")
		buf := make([]byte, 4)
		n, err := stdin.Read(buf)
		assert.Equal(t, 0, n, "child stdin must immediately yield zero bytes")
		assert.Equal(t, io.EOF, err, "child stdin must immediately yield EOF")
		return 0, nil
	}
	o := options{cmdName: "echo", maxChars: DefaultMaxChars}

	code, stop := invokeCommand(context.Background(), cc, o, []string{"a"})
	assert.Equal(t, exitOK, code)
	assert.False(t, stop)
	assert.True(t, withStdinCalled, "RunCommandWithStdin must be the preferred path")
}

// TestInvokeCommandNilRunCommand verifies that a CallContext with no
// RunCommand wired surfaces an error and reports exit 125 (failed-to-start)
// without panicking.
func TestInvokeCommandNilRunCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cc := newPentestCallCtx(&stdout, &stderr)
	o := options{cmdName: "echo", maxChars: DefaultMaxChars}

	code, stop := invokeCommand(context.Background(), cc, o, []string{"a"})
	assert.Equal(t, exitSubCmdNotStart, code)
	assert.True(t, stop)
	assert.Contains(t, stderr.String(), "command execution not available")
}

// TestInvokeCommandBlockedByPolicy verifies that CommandAllowed is consulted
// before RunCommand and that a refusal yields exitSubCmdNotStart with stop=true
// and the run callback is never invoked.
func TestInvokeCommandBlockedByPolicy(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cc := newPentestCallCtx(&stdout, &stderr)
	called := false
	cc.RunCommand = func(_ context.Context, _ string, _ string, _ []string) (uint8, error) {
		called = true
		return 0, nil
	}
	cc.CommandAllowed = func(_ string) bool { return false }
	o := options{cmdName: "echo", maxChars: DefaultMaxChars}

	code, stop := invokeCommand(context.Background(), cc, o, []string{"a"})
	assert.Equal(t, exitSubCmdNotStart, code)
	assert.True(t, stop)
	assert.False(t, called, "RunCommand must not be called when policy denies")
	assert.Contains(t, stderr.String(), "not allowed")
}

// TestInvokeCommandSubProcess255Aborts verifies that an exit-255 sub-command
// triggers the POSIX "abort the entire xargs invocation" code path.
func TestInvokeCommandSubProcess255Aborts(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cc := newPentestCallCtx(&stdout, &stderr)
	cc.RunCommand = func(_ context.Context, _ string, _ string, _ []string) (uint8, error) {
		return 255, nil
	}
	o := options{cmdName: "echo", maxChars: DefaultMaxChars}

	code, stop := invokeCommand(context.Background(), cc, o, []string{"a"})
	assert.Equal(t, exitSubCmd255, code)
	assert.True(t, stop)
	assert.Contains(t, stderr.String(), "exited with status 255")
}

// TestInvokeCommandSubProcessFailureContinues verifies that an exit-1
// sub-command yields exitSubCmdFailed with stop=false (xargs keeps going).
func TestInvokeCommandSubProcessFailureContinues(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cc := newPentestCallCtx(&stdout, &stderr)
	cc.RunCommand = func(_ context.Context, _ string, _ string, _ []string) (uint8, error) {
		return 1, nil
	}
	o := options{cmdName: "false", maxChars: DefaultMaxChars}

	code, stop := invokeCommand(context.Background(), cc, o, nil)
	assert.Equal(t, exitSubCmdFailed, code)
	assert.False(t, stop)
}

// TestInvokeCommandRunErrorAborts verifies that a RunCommand error path
// reports exitSubCmdNotStart and writes the error to stderr.
func TestInvokeCommandRunErrorAborts(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cc := newPentestCallCtx(&stdout, &stderr)
	cc.RunCommand = func(_ context.Context, _ string, _ string, _ []string) (uint8, error) {
		return 0, errors.New("boom")
	}
	o := options{cmdName: "echo", maxChars: DefaultMaxChars}

	code, stop := invokeCommand(context.Background(), cc, o, []string{"a"})
	assert.Equal(t, exitSubCmdNotStart, code)
	assert.True(t, stop)
	assert.Contains(t, stderr.String(), "boom")
}

// TestInvokeCommandPreCancelledContext verifies that a cancelled context
// exits the function before any RunCommand call.
func TestInvokeCommandPreCancelledContext(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cc := newPentestCallCtx(&stdout, &stderr)
	called := false
	cc.RunCommand = func(_ context.Context, _ string, _ string, _ []string) (uint8, error) {
		called = true
		return 0, nil
	}
	o := options{cmdName: "echo", maxChars: DefaultMaxChars}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	code, stop := invokeCommand(ctx, cc, o, []string{"a"})
	assert.Equal(t, exitUsage, code)
	assert.True(t, stop)
	assert.False(t, called)
}

// TestDecodeDelimEmpty exercises the empty-string error path.
func TestDecodeDelimEmpty(t *testing.T) {
	_, err := decodeDelim("")
	assert.Error(t, err)
}

// TestDecodeDelimMultiByteRejected verifies multi-byte non-escape forms.
func TestDecodeDelimMultiByteRejected(t *testing.T) {
	_, err := decodeDelim("ab")
	assert.Error(t, err)
}

// TestResolveCmdReplaceWithEmptyBatch verifies the safe path when -I is set
// but the batch arrives empty (defensive fallthrough — must not panic).
func TestResolveCmdReplaceWithEmptyBatch(t *testing.T) {
	o := options{replStr: "{}", cmdName: "echo {}", initialArgs: []string{"x{}"}}
	cmd, args := resolveCmd(o, nil)
	// Empty item replaces every {} with "".
	assert.Equal(t, "echo ", cmd)
	assert.Equal(t, []string{"x"}, args)
}

// TestCommandLineLenAccountsForAllParts ensures the running-budget helper
// includes cmdName, initial args, and batch with one byte of overhead per
// arg.
func TestCommandLineLenAccountsForAllParts(t *testing.T) {
	o := options{cmdName: "echo", initialArgs: []string{"PRE"}}
	got := commandLineLen(o, []string{"a", "bb"})
	want := len("echo") + 1 + len("PRE") + 1 + len("a") + 1 + len("bb") + 1
	assert.Equal(t, want, got)
}

// TestTokenizerInfiniteSafety feeds each tokenizer mode an infinite stream
// and asserts that next() returns within a short bound — either because
// ctx.Err() fired (preferred) or because the per-token cap fired (also a
// safe DoS-bounding outcome). RULES.md requires *both* mechanisms exist;
// either is sufficient evidence the implementation does not hang.
func TestTokenizerInfiniteSafety(t *testing.T) {
	cases := []struct {
		name string
		o    options
		// reader byte: never hits the mode's separator so the tokenizer
		// keeps consuming and either exits via cap or via ctx cancel.
		b byte
	}{
		{"whitespace", options{mode: modeWhitespace, maxChars: DefaultMaxChars}, 'x'},
		{"null", options{mode: modeNull, maxChars: DefaultMaxChars}, 'a'},
		{"line", options{mode: modeLine, replStr: "{}", maxChars: DefaultMaxChars}, 'a'},
		{"delim", options{mode: modeDelim, delim: ',', maxChars: DefaultMaxChars}, 'a'},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tok := newTokenizer(&infiniteReader{b: tc.b}, tc.o)
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()

			done := make(chan struct{})
			var err error
			go func() {
				_, _, _, err = tok.next(ctx)
				close(done)
			}()

			select {
			case <-done:
				require.Error(t, err)
				// Either context cancellation fired (preferred when reader
				// keeps yielding indefinitely) or the per-token cap kicked
				// in. Both are acceptable DoS bounds.
				ok := errors.Is(err, context.DeadlineExceeded) ||
					err.Error() == fmt.Sprintf("argument exceeds %d byte limit", MaxTokenBytes)
				assert.True(t, ok, "expected deadline or token-cap error, got %v", err)
			case <-time.After(2 * time.Second):
				t.Fatal("tokenizer.next did not return within 2s")
			}
		})
	}
}

// TestTokenizerInfiniteCancelOnSeparators uses a stream that emits the
// mode's separator regularly, so per-token caps never fire — only ctx
// cancellation can stop the loop. This proves the ctx-poll path is wired
// correctly and not just relying on the token cap.
func TestTokenizerInfiniteCancelOnSeparators(t *testing.T) {
	cases := []struct {
		name string
		o    options
		b    byte // emit only this byte; chosen to be the mode's separator
	}{
		{"whitespace_blank_lines", options{mode: modeWhitespace, maxChars: DefaultMaxChars}, '\n'},
		{"null_empty_tokens", options{mode: modeNull, maxChars: DefaultMaxChars}, 0},
		{"line_blank_lines", options{mode: modeLine, replStr: "{}", maxChars: DefaultMaxChars}, '\n'},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tok := newTokenizer(&infiniteReader{b: tc.b}, tc.o)
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()

			done := make(chan struct{})
			var err error
			go func() {
				// Loop calling next so that even if a single call returns
				// quickly with an empty token, we keep cycling until ctx
				// cancellation breaks us out.
				for {
					_, _, more, e := tok.next(ctx)
					if e != nil {
						err = e
						break
					}
					if !more {
						break
					}
				}
				close(done)
			}()

			select {
			case <-done:
				require.Error(t, err)
				assert.True(t, errors.Is(err, context.DeadlineExceeded),
					"expected DeadlineExceeded, got %v", err)
			case <-time.After(2 * time.Second):
				t.Fatal("tokenizer.next did not honour context deadline within 2s")
			}
		})
	}
}

// silence unused imports when the file is open in isolation.
var _ = io.EOF
