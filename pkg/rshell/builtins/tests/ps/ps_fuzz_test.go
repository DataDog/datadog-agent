// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ps_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/pkg/rshell/internal/interpoption"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// runPS runs a ps command script and returns stdout, stderr, and exit code.
func runPS(t testing.TB, script string) (string, string, int) {
	t.Helper()
	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader(script), "")
	if err != nil {
		return "", err.Error(), 1
	}
	var outBuf, errBuf bytes.Buffer
	runner, err := interp.New(interp.StdIO(nil, &outBuf, &errBuf), interpoption.AllowAllCommands().(interp.RunnerOption))
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
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// TestPSFuzzSeedCorpusBasic runs the fuzz seed corpus as a regular test.
func TestPSFuzzSeedCorpusBasic(t *testing.T) {
	_, _, _ = runPS(t, "ps -h")
	_, _, _ = runPS(t, "ps -e")
	_, _, _ = runPS(t, "ps -p 1")
	_, _, _ = runPS(t, "ps -p notapid")
}

// FuzzPSPidList fuzzes the -p flag with arbitrary PID list strings.
// The process must not panic or block regardless of input.
func FuzzPSPidList(f *testing.F) {
	f.Add("1")
	f.Add("1,2,3")
	f.Add("0")
	f.Add("-1")
	f.Add("notapid")
	f.Add("1 2 3")
	f.Add("99999999999")
	f.Add(",,,")
	f.Add("")
	f.Add("1,notapid,2")
	f.Add("2147483647")
	f.Add("  1  ,  2  ")

	f.Fuzz(func(t *testing.T, pidList string) {
		if t.Context().Err() != nil {
			return
		}
		// Bound input length to avoid overly long strings.
		if len(pidList) > 256 {
			return
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()

		parser := syntax.NewParser()
		// Shell-escape the PID list by wrapping in single quotes and
		// substituting any embedded single quotes to avoid injection.
		safe := strings.ReplaceAll(pidList, "'", "")
		script := "ps -p '" + safe + "'"
		prog, err := parser.Parse(strings.NewReader(script), "")
		if err != nil {
			return // unparseable, skip
		}
		var outBuf, errBuf bytes.Buffer
		runner, err := interp.New(interp.StdIO(nil, &outBuf, &errBuf), interpoption.AllowAllCommands().(interp.RunnerOption))
		if err != nil {
			t.Fatal(err)
		}
		defer runner.Close()
		runErr := runner.Run(ctx, prog)
		if runErr != nil {
			var es interp.ExitStatus
			if !errors.As(runErr, &es) && ctx.Err() == nil {
				t.Errorf("unexpected runner error: %v", runErr)
			}
		}
	})
}

// FuzzPSFlags fuzzes arbitrary flag combinations to ensure ps never panics.
func FuzzPSFlags(f *testing.F) {
	f.Add("-e")
	f.Add("-A")
	f.Add("-f")
	f.Add("-ef")
	f.Add("-h")
	f.Add("")
	f.Add("-e -f")
	f.Add("--unknownflag")

	f.Fuzz(func(t *testing.T, flags string) {
		if t.Context().Err() != nil {
			return
		}
		if len(flags) > 64 {
			return
		}
		// Only allow safe flag characters to avoid shell injection.
		for _, c := range flags {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '-' || c == ' ') {
				return
			}
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()

		script := "ps " + flags
		parser := syntax.NewParser()
		prog, err := parser.Parse(strings.NewReader(script), "")
		if err != nil {
			return
		}
		var outBuf, errBuf bytes.Buffer
		runner, err := interp.New(interp.StdIO(nil, &outBuf, &errBuf), interpoption.AllowAllCommands().(interp.RunnerOption))
		if err != nil {
			t.Fatal(err)
		}
		defer runner.Close()
		runErr := runner.Run(ctx, prog)
		if runErr != nil {
			var es interp.ExitStatus
			if !errors.As(runErr, &es) && ctx.Err() == nil {
				t.Errorf("unexpected runner error: %v", runErr)
			}
		}
	})
}
