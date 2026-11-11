// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package tee_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/pkg/rshell/internal/interpoption"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// runScript parses and runs the given shell script through interp.Runner with
// AllowAllCommands set, plus any additional options. It returns stdout,
// stderr, and the resolved exit code.
func runScript(t *testing.T, script, dir string, opts ...interp.RunnerOption) (string, string, int) {
	t.Helper()
	return runScriptStdin(t, script, dir, nil, opts...)
}

// runScriptStdin behaves like runScript but wires stdin to the given reader
// (nil leaves stdin unset, matching a script invoked with no piped input).
func runScriptStdin(t *testing.T, script, dir string, stdin io.Reader, opts ...interp.RunnerOption) (string, string, int) {
	t.Helper()
	return runScriptCtx(context.Background(), t, script, dir, stdin, opts...)
}

func runScriptCtx(ctx context.Context, t *testing.T, script, dir string, stdin io.Reader, opts ...interp.RunnerOption) (string, string, int) {
	t.Helper()
	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader(script), "")
	if err != nil {
		t.Fatal(err)
	}
	var outBuf, errBuf bytes.Buffer
	allOpts := append([]interp.RunnerOption{
		interp.StdIO(stdin, &outBuf, &errBuf),
		interpoption.AllowAllCommands().(interp.RunnerOption),
	}, opts...)
	runner, err := interp.New(allOpts...)
	if err != nil {
		t.Fatal(err)
	}
	defer runner.Close()
	if dir != "" {
		runner.Dir = dir
	}
	runErr := runner.Run(ctx, prog)
	exitCode := 0
	if runErr != nil {
		var es interp.ExitStatus
		if errors.As(runErr, &es) {
			exitCode = int(es)
		} else if ctx.Err() == nil {
			t.Fatalf("unexpected error: %v", runErr)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// teeRun runs script with AllowedPaths restricted to dir as read-write and
// remediation mode enabled. Use this wrapper for tests that exercise the
// happy path.
func teeRun(t *testing.T, script, dir string) (string, string, int) {
	t.Helper()
	return runScript(t, script, dir,
		interp.AllowedPaths([]string{dir + ":rw"}),
		interp.WithMode(interp.ModeRemediation),
	)
}

// teeRunStdin is teeRun but with stdin wired to the given content.
func teeRunStdin(t *testing.T, script, dir, stdin string) (string, string, int) {
	t.Helper()
	return runScriptStdin(t, script, dir, strings.NewReader(stdin),
		interp.AllowedPaths([]string{dir + ":rw"}),
		interp.WithMode(interp.ModeRemediation),
	)
}

// writeFile writes content to dir/name and returns the full path.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// readFile returns the content of path or fails the test.
func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}
