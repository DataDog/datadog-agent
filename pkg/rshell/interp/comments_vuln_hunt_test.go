// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Vulnerability-hunt regression coverage for campaign 2026-05-19-codex.

func TestVulnHuntShellFeatureRedirectionChain_CommentsCannotCreateHiddenRedirects(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "allowed")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(allowed, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(allowed, "inside.txt"), []byte("inside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runSimpleCommandScript(t,
		"cat < inside.txt # < ../outside/secret.txt\necho ok # > created.txt\n",
		allowed,
		allowAllCommandsOpt(),
		AllowedPaths([]string{allowed}),
	)
	if code != 0 {
		t.Fatalf("exit code=%d stderr=%q", code, stderr)
	}
	if stdout != "inside\nok\n" {
		t.Fatalf("stdout=%q, want only inside file and echo output", stdout)
	}
	if _, err := os.Stat(filepath.Join(allowed, "created.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("commented output redirect created file, stat err=%v", err)
	}
}

func TestVulnHuntShellFeatureExpansionChain_CommentsDoNotRunExpansions(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "allowed")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(allowed, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runSimpleCommandScript(t,
		"A=$(echo safe) # $(<../outside/secret.txt)\necho \"$A\"\necho ok >/dev/null # $(<../outside/secret.txt)\necho visible\n",
		allowed,
		allowAllCommandsOpt(),
		AllowedPaths([]string{allowed}),
	)
	if code != 0 {
		t.Fatalf("exit code=%d stderr=%q", code, stderr)
	}
	if strings.Contains(stdout, "secret") || strings.Contains(stderr, "secret") {
		t.Fatalf("commented command substitution ran or leaked content: stdout=%q stderr=%q", stdout, stderr)
	}
	if stdout != "safe\nvisible\n" {
		t.Fatalf("stdout=%q, want safe expansion plus visible output", stdout)
	}
	if stderr != "" {
		t.Fatalf("commented outside read should not produce sandbox stderr, got %q", stderr)
	}
}

func TestVulnHuntShellFeatureReadonlyBypass_CommentsDoNotMaskReadonlyFailures(t *testing.T) {
	stdout, stderr := runScriptWithReadonly(t, "RO_VAR=hacked # trailing comment\necho after=$RO_VAR\n")
	if !strings.Contains(stderr, "readonly variable") {
		t.Fatalf("expected readonly error, stderr=%q", stderr)
	}
	if strings.Contains(stdout, "hacked") || !strings.Contains(stdout, "after=original") {
		t.Fatalf("readonly variable changed or was not preserved: stdout=%q stderr=%q", stdout, stderr)
	}

	stderr = runSimpleCommandValidationFailure(t, "readonly X=1 # trailing comment\n")
	if !strings.Contains(stderr, "readonly is not supported") {
		t.Fatalf("expected readonly declaration validation failure, stderr=%q", stderr)
	}
}

func TestVulnHuntShellFeatureSignalContext_CommentsDoNotConsumeCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	stdout, stderr, _, err := runSimpleCommandScriptCtx(t, ctx, "# comment only\necho SHOULD_NOT_RUN\n", "", allowAllCommandsOpt())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got err=%v stdout=%q stderr=%q", err, stdout, stderr)
	}
	if stdout != "" || stderr != "" {
		t.Fatalf("pre-canceled comment script produced output: stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestVulnHuntShellFeatureDeclaredVsImplemented_CommentOnlyScriptSizeCap(t *testing.T) {
	line := "# inert comment payload\n"
	script := strings.Repeat(line, MaxScriptBytes/len(line)+1)
	if len(script) <= MaxScriptBytes {
		t.Fatalf("test bug: generated script length %d <= cap %d", len(script), MaxScriptBytes)
	}

	_, err := ParseScript(script, "comment-only")
	if err == nil {
		t.Fatalf("over-cap comment-only script parsed successfully")
	}
	if !strings.Contains(err.Error(), "exceeds maximum") {
		t.Fatalf("expected script-size cap error, got %v", err)
	}
}

func TestVulnHuntShellFeatureSubshellIsolation_CommentsDoNotChangeScopeOrStdio(t *testing.T) {
	stdout, stderr, code := runSimpleCommandScript(t,
		"X=outer\n( X=inner # comment\n)\necho parent=$X\n( echo hidden # > /dev/null\n) > /dev/null\necho visible\n",
		"",
		allowAllCommandsOpt(),
	)
	if code != 0 {
		t.Fatalf("exit code=%d stderr=%q", code, stderr)
	}
	if stdout != "parent=outer\nvisible\n" {
		t.Fatalf("stdout=%q, want parent state and stdio restored", stdout)
	}
}
