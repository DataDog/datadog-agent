// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package lsof_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/pkg/rshell/internal/interpoption"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// cmdRun runs script through the real interpreter with every command
// allowed and the given AllowedPaths roots (nil/empty means no filesystem
// paths are reachable, matching the shell's documented default), and
// returns stdout, stderr, and the exit code.
func cmdRun(t *testing.T, script string, allowedPaths []string) (string, string, int) {
	t.Helper()
	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader(script), "")
	if err != nil {
		t.Fatal(err)
	}
	var outBuf, errBuf bytes.Buffer
	opts := []interp.RunnerOption{
		interp.StdIO(nil, &outBuf, &errBuf),
		interpoption.AllowAllCommands().(interp.RunnerOption),
		interp.AllowedPaths(allowedPaths),
	}
	runner, err := interp.New(opts...)
	if err != nil {
		t.Fatal(err)
	}
	defer runner.Close()
	runErr := runner.Run(context.Background(), prog)
	exitCode := 0
	if runErr != nil {
		var es interp.ExitStatus
		if errors.As(runErr, &es) {
			exitCode = int(es)
		} else {
			t.Fatalf("unexpected error: %v", runErr)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}
