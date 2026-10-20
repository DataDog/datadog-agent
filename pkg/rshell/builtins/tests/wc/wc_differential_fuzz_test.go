// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package wc_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
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

// FuzzWcDifferentialLines compares rshell wc -l output against GNU wc.
func FuzzWcDifferentialLines(f *testing.F) {
	if os.Getenv("RSHELL_BASH_TEST") == "" {
		f.Skip("set RSHELL_BASH_TEST=1 to run differential fuzz tests")
	}

	f.Add([]byte("line1\nline2\nline3\n"))
	f.Add([]byte(""))
	f.Add([]byte("no newline"))
	f.Add([]byte("a\nb\nc\n"))
	f.Add([]byte("\n\n\n"))
	f.Add([]byte("a\x00b\nc\n"))
	f.Add([]byte("single line\n"))
	f.Add(bytes.Repeat([]byte("x\n"), 100))

	baseDir := f.TempDir()
	var counter atomic.Int64

	f.Fuzz(func(t *testing.T, input []byte) {
		if t.Context().Err() != nil {
			return
		}
		if len(input) > 64*1024 {
			return
		}

		dir, cleanup := testutil.FuzzIterDir(t, baseDir, &counter)
		defer cleanup()

		if err := os.WriteFile(filepath.Join(dir, "input.txt"), input, 0644); err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		rshellOut, rshellErr, rshellCode := cmdRunCtx(ctx, t, "wc -l input.txt", dir)
		cancel()

		if t.Context().Err() != nil {
			return
		}

		if isSandboxError(rshellErr) {
			t.Skip("skipping: sandbox restriction")
		}

		gnuOut, gnuCode := runGNUInDir(t, dir, []string{"wc", "-l", "input.txt"})
		if gnuCode == -1 {
			return
		}

		if rshellOut != gnuOut {
			t.Errorf("wc -l stdout mismatch:\nrshell: %q\ngnu:    %q\ninput:  %q", rshellOut, gnuOut, input)
		}
		if rshellCode != gnuCode {
			t.Errorf("wc -l exit code mismatch: rshell=%d gnu=%d", rshellCode, gnuCode)
		}
	})
}

// FuzzWcDifferentialWords compares rshell wc -w output against GNU wc.
func FuzzWcDifferentialWords(f *testing.F) {
	if os.Getenv("RSHELL_BASH_TEST") == "" {
		f.Skip("set RSHELL_BASH_TEST=1 to run differential fuzz tests")
	}

	f.Add([]byte("hello world\n"))
	f.Add([]byte(""))
	f.Add([]byte("  spaces  \n"))
	f.Add([]byte("one\ntwo three\n"))
	f.Add([]byte("\t\ttabs\t\n"))
	f.Add([]byte("a\x00b c\n"))
	f.Add([]byte("word"))
	f.Add(bytes.Repeat([]byte("a b "), 50))

	baseDir := f.TempDir()
	var counter atomic.Int64

	f.Fuzz(func(t *testing.T, input []byte) {
		if t.Context().Err() != nil {
			return
		}
		if len(input) > 64*1024 {
			return
		}

		dir, cleanup := testutil.FuzzIterDir(t, baseDir, &counter)
		defer cleanup()

		if err := os.WriteFile(filepath.Join(dir, "input.txt"), input, 0644); err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		rshellOut, rshellErr, rshellCode := cmdRunCtx(ctx, t, "wc -w input.txt", dir)
		cancel()

		if t.Context().Err() != nil {
			return
		}

		if isSandboxError(rshellErr) {
			t.Skip("skipping: sandbox restriction")
		}

		gnuOut, gnuCode := runGNUInDir(t, dir, []string{"wc", "-w", "input.txt"})
		if gnuCode == -1 {
			return
		}

		if rshellOut != gnuOut {
			t.Errorf("wc -w stdout mismatch:\nrshell: %q\ngnu:    %q\ninput:  %q", rshellOut, gnuOut, input)
		}
		if rshellCode != gnuCode {
			t.Errorf("wc -w exit code mismatch: rshell=%d gnu=%d", rshellCode, gnuCode)
		}
	})
}

// FuzzWcDifferentialBytes compares rshell wc -c output against GNU wc.
func FuzzWcDifferentialBytes(f *testing.F) {
	if os.Getenv("RSHELL_BASH_TEST") == "" {
		f.Skip("set RSHELL_BASH_TEST=1 to run differential fuzz tests")
	}

	f.Add([]byte("hello\nworld\n"))
	f.Add([]byte(""))
	f.Add([]byte("no newline"))
	f.Add([]byte("a\x00b\nc\n"))
	f.Add([]byte{0xff, 0xfe, 0x00, 0x01})
	f.Add(bytes.Repeat([]byte("x"), 100))
	f.Add([]byte("\n\n\n"))

	baseDir := f.TempDir()
	var counter atomic.Int64

	f.Fuzz(func(t *testing.T, input []byte) {
		if t.Context().Err() != nil {
			return
		}
		if len(input) > 64*1024 {
			return
		}

		dir, cleanup := testutil.FuzzIterDir(t, baseDir, &counter)
		defer cleanup()

		if err := os.WriteFile(filepath.Join(dir, "input.txt"), input, 0644); err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		rshellOut, rshellErr, rshellCode := cmdRunCtx(ctx, t, "wc -c input.txt", dir)
		cancel()

		if t.Context().Err() != nil {
			return
		}

		if isSandboxError(rshellErr) {
			t.Skip("skipping: sandbox restriction")
		}

		gnuOut, gnuCode := runGNUInDir(t, dir, []string{"wc", "-c", "input.txt"})
		if gnuCode == -1 {
			return
		}

		if rshellOut != gnuOut {
			t.Errorf("wc -c stdout mismatch:\nrshell: %q\ngnu:    %q\ninput:  %q", rshellOut, gnuOut, input)
		}
		if rshellCode != gnuCode {
			t.Errorf("wc -c exit code mismatch: rshell=%d gnu=%d", rshellCode, gnuCode)
		}
	})
}
