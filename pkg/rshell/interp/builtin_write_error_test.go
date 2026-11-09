// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"syscall"
	"testing"

	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/pkg/rshell/internal/interpoption"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// failingWriter models a redirect target that fails every write (ENOSPC,
// EIO, or EFBIG past RLIMIT_FSIZE).
type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func runWithStdout(t *testing.T, script string, stdout io.Writer) (stderr string, code int) {
	t.Helper()
	prog, err := syntax.NewParser().Parse(strings.NewReader(script), "")
	if err != nil {
		t.Fatal(err)
	}
	var errBuf bytes.Buffer
	runner, err := interp.New(
		interp.StdIO(strings.NewReader("b\na\nb\n"), stdout, &errBuf),
		interpoption.AllowAllCommands().(interp.RunnerOption),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer runner.Close()
	runErr := runner.Run(context.Background(), prog)
	var es interp.ExitStatus
	if runErr != nil && !errors.As(runErr, &es) {
		t.Fatalf("unexpected error: %v", runErr)
	}
	return errBuf.String(), int(es)
}

// A builtin whose stdout write fails must exit 1 and say so once, like
// bash's "echo: write error: No space left on device".
func TestBuiltinWriteErrorReported(t *testing.T) {
	scripts := map[string]string{
		"echo":   "echo hello",
		"printf": "printf 'hello\\n'",
		"sed":    "sed p",
		"sort":   "sort",
		"grep":   "grep a",
		"wc":     "wc -l",
		"cut":    "cut -c1",
		"cat":    "cat",
		"head":   "head",
		"uniq":   "uniq",
	}
	for name, script := range scripts {
		t.Run(name, func(t *testing.T) {
			stderr, code := runWithStdout(t, script, failingWriter{syscall.ENOSPC})
			if code != 1 {
				t.Errorf("exit code = %d, want 1 (stderr %q)", code, stderr)
			}
			// Builtins that already checked their writes (cat, head, uniq)
			// word the message themselves; either way the line appears once.
			if n := strings.Count(stderr, "\n"); n != 1 {
				t.Errorf("stderr has %d lines, want one: %q", n, stderr)
			}
		})
	}
}

func TestBuiltinWriteErrorFormat(t *testing.T) {
	stderr, _ := runWithStdout(t, "echo hello", failingWriter{syscall.ENOSPC})
	if want := "echo: write error: no space left on device\n"; stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
}

// A broken pipe is not an error bash prints for a builtin.
func TestBuiltinWriteErrorBrokenPipeSilent(t *testing.T) {
	stderr, _ := runWithStdout(t, "echo hello", failingWriter{syscall.EPIPE})
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
}
