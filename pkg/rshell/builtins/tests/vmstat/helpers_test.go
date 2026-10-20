// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package vmstat_test

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

func runScript(t *testing.T, script string) (stdout, stderr string, exitCode int) {
	t.Helper()
	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader(script), "")
	if err != nil {
		t.Fatal(err)
	}
	var outBuf, errBuf bytes.Buffer
	runner, err := interp.New(
		interp.StdIO(nil, &outBuf, &errBuf),
		interpoption.AllowAllCommands().(interp.RunnerOption),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer runner.Close()
	runErr := runner.Run(context.Background(), prog)
	code := 0
	if runErr != nil {
		var es interp.ExitStatus
		if errors.As(runErr, &es) {
			code = int(es)
		} else {
			t.Fatalf("unexpected error: %v", runErr)
		}
	}
	return outBuf.String(), errBuf.String(), code
}

// cmdRun runs a vmstat command. vmstat does not access the filesystem via
// the AllowedPaths sandbox (it delegates to the internal vmstat package,
// which reads hardcoded kernel pseudo-files directly), so no AllowedPaths
// configuration is needed here.
func cmdRun(t *testing.T, script string) (stdout, stderr string, exitCode int) {
	t.Helper()
	return runScript(t, script)
}
