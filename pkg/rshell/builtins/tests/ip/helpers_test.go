// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ip_test

import (
	"bytes"
	"context"
	"errors"
	"net"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/pkg/rshell/internal/interpoption"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

func runScript(t *testing.T, script, dir string, opts ...interp.RunnerOption) (string, string, int) {
	t.Helper()
	return runScriptCtx(context.Background(), t, script, dir, opts...)
}

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

// cmdRun runs an ip command. ip does not access the filesystem via the sandbox
// so no AllowedPaths restriction is needed.
func cmdRun(t *testing.T, script string) (stdout, stderr string, exitCode int) {
	t.Helper()
	return runScript(t, script, "")
}

// loopbackName returns the name of the system's loopback network interface.
// On Linux this is "lo"; on macOS it is "lo0"; on Windows it varies.
// Tests skip if no loopback is found.
func loopbackName(t *testing.T) string {
	t.Helper()
	ifaces, err := net.Interfaces()
	if err != nil {
		t.Skipf("cannot enumerate interfaces: %v", err)
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			return iface.Name
		}
	}
	t.Skip("no loopback interface found on this system")
	return ""
}
