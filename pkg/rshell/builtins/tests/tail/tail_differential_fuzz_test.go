// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package tail_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// runGNUInDir runs a GNU command under LC_ALL=C.UTF-8 with its working
// directory set to dir. args[0] is the command name; args[1:] are arguments.
func runGNUInDir(t *testing.T, dir string, args []string) (stdout string, exitCode int) {
	t.Helper()
	if _, err := exec.LookPath(args[0]); err != nil {
		t.Skipf("%s not found: %v", args[0], err)
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "LC_ALL=C.UTF-8")

	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf

	err := cmd.Run()
	exitCode = 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Logf("gnu exec error: %v", err)
			return "", -1
		}
	}
	return outBuf.String(), exitCode
}

func isSandboxError(stderr string) bool {
	lower := strings.ToLower(stderr)
	return strings.Contains(lower, "permission denied") ||
		strings.Contains(lower, "not allowed") ||
		strings.Contains(lower, "sandbox")
}

// FuzzTailDifferential compares rshell tail -n N output against GNU tail.
func FuzzTailDifferential(f *testing.F) {
	if os.Getenv("RSHELL_BASH_TEST") == "" {
		f.Skip("set RSHELL_BASH_TEST=1 to run differential fuzz tests")
	}

	f.Add([]byte("line1\nline2\nline3\n"), int64(2))
	f.Add([]byte(""), int64(0))
	f.Add([]byte("no newline"), int64(1))
	f.Add([]byte("a\nb\nc\n"), int64(100))
	f.Add([]byte("\n\n\n"), int64(2))
	f.Add([]byte("a\x00b\nc\n"), int64(2))
	f.Add([]byte("single line\n"), int64(1))
	f.Add([]byte("a\nb\nc\nd\ne\n"), int64(3))
	f.Add(bytes.Repeat([]byte("line\n"), 20), int64(5))

	f.Fuzz(func(t *testing.T, input []byte, n int64) {
		if t.Context().Err() != nil {
			return
		}
		if len(input) > 64*1024 {
			return
		}
		if n < 0 || n > 10000 {
			return
		}

		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "input.txt"), input, 0644); err != nil {
			t.Fatal(err)
		}

		nStr := fmt.Sprintf("%d", n)

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		rshellOut, rshellErr, rshellCode := cmdRunCtx(ctx, t, fmt.Sprintf("tail -n %s input.txt", nStr), dir)
		cancel()

		if t.Context().Err() != nil {
			return
		}

		if isSandboxError(rshellErr) {
			t.Skip("skipping: sandbox restriction")
		}

		gnuOut, gnuCode := runGNUInDir(t, dir, []string{"tail", "-n", nStr, "input.txt"})
		if gnuCode == -1 {
			return
		}

		if rshellOut != gnuOut {
			t.Errorf("tail -n %d stdout mismatch:\nrshell: %q\ngnu:    %q\ninput:  %q", n, rshellOut, gnuOut, input)
		}
		if rshellCode != gnuCode {
			t.Errorf("tail -n %d exit code mismatch: rshell=%d gnu=%d", n, rshellCode, gnuCode)
		}
	})
}
