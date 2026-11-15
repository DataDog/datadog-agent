// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package cat_test

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

// FuzzCatDifferential compares rshell cat output against GNU cat.
func FuzzCatDifferential(f *testing.F) {
	if os.Getenv("RSHELL_BASH_TEST") == "" {
		f.Skip("set RSHELL_BASH_TEST=1 to run differential fuzz tests")
	}

	f.Add([]byte("hello\nworld\n"))
	f.Add([]byte(""))
	f.Add([]byte("no newline"))
	f.Add([]byte("a\x00b\n"))
	f.Add(bytes.Repeat([]byte("x"), 4097))
	f.Add([]byte("\n\n\n"))
	f.Add([]byte{0xff, 0xfe, 0x00, 0x01})
	f.Add([]byte("line1\nline2\nline3\n"))

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
		rshellOut, rshellErr, rshellCode := cmdRunCtx(ctx, t, "cat input.txt", dir)
		cancel()

		if t.Context().Err() != nil {
			return
		}

		if isSandboxError(rshellErr) {
			t.Skip("skipping: sandbox restriction")
		}

		gnuOut, gnuCode := runGNUInDir(t, dir, []string{"cat", "input.txt"})
		if gnuCode == -1 {
			return
		}

		if rshellOut != gnuOut {
			t.Errorf("stdout mismatch:\nrshell: %q\ngnu:    %q\ninput:  %q", rshellOut, gnuOut, input)
		}
		if rshellCode != gnuCode {
			t.Errorf("exit code mismatch: rshell=%d gnu=%d", rshellCode, gnuCode)
		}
	})
}
