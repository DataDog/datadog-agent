// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package cat_test

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

func runScriptCtx(ctx context.Context, t *testing.T, script, dir string, opts ...interp.RunnerOption) (string, string, int) {
	t.Helper()
	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader(script), "")
	if err != nil {
		t.Fatal(err)
	}
	var outBuf, errBuf bytes.Buffer
	allOpts := append([]interp.RunnerOption{interp.StdIO(nil, &outBuf, &errBuf), interpoption.AllowAllCommands().(interp.RunnerOption)}, opts...)
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

func cmdRunCtx(ctx context.Context, t *testing.T, script, dir string) (string, string, int) {
	t.Helper()
	return runScriptCtx(ctx, t, script, dir, interp.AllowedPaths([]string{dir}))
}
