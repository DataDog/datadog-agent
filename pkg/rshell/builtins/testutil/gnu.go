// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package testutil

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// GNUCommand resolves a GNU coreutils reference once, before fuzz iterations.
// Ubuntu can provide uutils under the unprefixed command names; Ubuntu's GNU
// commands use a gnu prefix, while Homebrew uses a g prefix.
// Enabled differential tests must fail when their reference is unavailable.
func GNUCommand(t testing.TB, name string) string {
	t.Helper()
	path, err := findGNUCommand(t.Context(), name)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("GNU reference for %s: %s", name, path)
	return path
}

func findGNUCommand(ctx context.Context, name string) (string, error) {
	var failures []error
	for _, candidate := range []string{"gnu" + name, "g" + name, name} {
		path, err := exec.LookPath(candidate)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		versionCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		command := exec.CommandContext(versionCtx, path, "--version")
		command.Env = append(os.Environ(), "LC_ALL=C")
		output, err := command.CombinedOutput()
		cancel()
		firstLine, _, _ := strings.Cut(string(output), "\n")
		if err != nil || !strings.Contains(firstLine, "(GNU coreutils)") {
			failures = append(failures, fmt.Errorf("%s is not a usable GNU coreutils reference: %q (error: %v)", path, firstLine, err))
			continue
		}
		if name == "wc" && !modernGNUWords(firstLine) {
			failures = append(failures, fmt.Errorf("%s: GNU wc 9.5+ is required for non-printable word counting (%s)", path, firstLine))
			continue
		}
		return path, nil
	}
	return "", fmt.Errorf("GNU coreutils %s is required for differential tests; install GNU coreutils and expose its executables on PATH: %w", name, errors.Join(failures...))
}

// GNU changed word counting for non-printable and invalid input in 9.5.
func modernGNUWords(versionLine string) bool {
	fields := strings.Fields(versionLine)
	if len(fields) == 0 {
		return false
	}
	version := strings.Split(fields[len(fields)-1], ".")
	if len(version) < 2 {
		return false
	}
	major, majorErr := strconv.Atoi(version[0])
	minor, minorErr := strconv.Atoi(version[1])
	return majorErr == nil && minorErr == nil && (major > 9 || major == 9 && minor >= 5)
}

// RunGNUInDir executes a reference resolved by GNUCommand, with a bounded
// lifetime and the C.UTF-8 locale used by rshell's differential tests.
func RunGNUInDir(t testing.TB, dir, command string, args ...string) (string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "LC_ALL=C.UTF-8")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if t.Context().Err() != nil {
		t.Skip("differential iteration cancelled by test context")
	}
	if ctx.Err() != nil {
		t.Fatalf("GNU reference %s %v: %v", command, args, ctx.Err())
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return stdout.String(), exitErr.ExitCode()
		}
		t.Fatalf("GNU reference %s %v: %v: %s", command, args, err, stderr.String())
	}
	return stdout.String(), 0
}
