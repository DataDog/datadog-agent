// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package shell

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestConfig() (runConfig, *bytes.Buffer, *bytes.Buffer) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cfg := runConfig{
		stdin:  strings.NewReader(""),
		stdout: stdout,
		stderr: stderr,
	}
	return cfg, stdout, stderr
}

func TestRunShellCommand(t *testing.T) {
	cfg, stdout, stderr := newTestConfig()

	exitCode := runShell("echo hi", nil, cfg)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if got := stdout.String(); got != "hi\n" {
		t.Fatalf("unexpected stdout: %q", got)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("unexpected stderr: %q", got)
	}
}

func TestRunShellCommandIgnoresArgs(t *testing.T) {
	cfg, stdout, stderr := newTestConfig()

	dir := t.TempDir()
	path := filepath.Join(dir, "script.sh")
	if err := os.WriteFile(path, []byte("echo file\n"), 0o600); err != nil {
		t.Fatalf("write script: %v", err)
	}

	exitCode := runShell("echo cmd", []string{path}, cfg)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if got := stdout.String(); got != "cmd\n" {
		t.Fatalf("unexpected stdout: %q", got)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("unexpected stderr: %q", got)
	}
}

func TestRunShellPathsReset(t *testing.T) {
	cfg, stdout, stderr := newTestConfig()

	dir := t.TempDir()
	first := filepath.Join(dir, "first.sh")
	second := filepath.Join(dir, "second.sh")
	if err := os.WriteFile(first, []byte("foo=bar\n"), 0o600); err != nil {
		t.Fatalf("write first script: %v", err)
	}
	if err := os.WriteFile(second, []byte("echo ${foo:-unset}\n"), 0o600); err != nil {
		t.Fatalf("write second script: %v", err)
	}

	exitCode := runShell("", []string{first, second}, cfg)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if got := stdout.String(); got != "unset\n" {
		t.Fatalf("unexpected stdout: %q", got)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("unexpected stderr: %q", got)
	}
}

func TestRunShellExitStatus(t *testing.T) {
	cfg, stdout, stderr := newTestConfig()

	exitCode := runShell("exit 7", nil, cfg)

	if exitCode != 7 {
		t.Fatalf("expected exit code 7, got %d", exitCode)
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("unexpected stdout: %q", got)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("unexpected stderr: %q", got)
	}
}
