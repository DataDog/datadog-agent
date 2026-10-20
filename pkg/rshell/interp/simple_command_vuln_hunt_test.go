// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mvdan.cc/sh/v3/syntax"
)

// Vulnerability-hunt regression coverage for campaign 2026-05-19-codex.

func runSimpleCommandScriptCtx(t *testing.T, ctx context.Context, script, dir string, opts ...RunnerOption) (stdout, stderr string, exitCode int, runErr error) {
	t.Helper()

	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader(script), "")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var outBuf, errBuf bytes.Buffer
	allOpts := append([]RunnerOption{StdIO(nil, &outBuf, &errBuf)}, opts...)
	runner, err := New(allOpts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { runner.Close() })
	if dir != "" {
		runner.Dir = dir
	}

	runErr = runner.Run(ctx, prog)
	if runErr != nil {
		var es ExitStatus
		if errors.As(runErr, &es) {
			exitCode = int(es)
		} else {
			exitCode = -1
		}
	}
	return outBuf.String(), errBuf.String(), exitCode, runErr
}

func runSimpleCommandScript(t *testing.T, script, dir string, opts ...RunnerOption) (stdout, stderr string, exitCode int) {
	t.Helper()
	stdout, stderr, exitCode, err := runSimpleCommandScriptCtx(t, context.Background(), script, dir, opts...)
	if err != nil {
		var es ExitStatus
		if !errors.As(err, &es) {
			t.Fatalf("Run: %v", err)
		}
	}
	return stdout, stderr, exitCode
}

func runSimpleCommandValidationFailure(t *testing.T, script string) string {
	t.Helper()
	stdout, stderr, code, err := runSimpleCommandScriptCtx(t, context.Background(), script, "", allowAllCommandsOpt())
	if err == nil {
		t.Fatalf("expected validation error for %q", script)
	}
	var es ExitStatus
	if !errors.As(err, &es) || code != 2 {
		t.Fatalf("expected validation exit status 2 for %q, got code=%d err=%v stdout=%q stderr=%q", script, code, err, stdout, stderr)
	}
	if stderr == "" {
		t.Fatalf("expected validation stderr for %q", script)
	}
	return stderr
}

func TestVulnHuntShellFeatureRedirectionChain_FailedRedirectPreventsAssignmentOnlyWrite(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "allowed")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(allowed, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, _ := runSimpleCommandScript(t,
		"VAR=leak < ../outside/secret\necho \"VAR=$VAR\"\n",
		allowed,
		allowAllCommandsOpt(),
		AllowedPaths([]string{allowed}),
	)
	if strings.Contains(stdout, "leak") {
		t.Fatalf("assignment-only command ran after failed redirect: stdout=%q stderr=%q", stdout, stderr)
	}
	if !strings.Contains(stdout, "VAR=\n") {
		t.Fatalf("expected VAR to remain unset, stdout=%q stderr=%q", stdout, stderr)
	}
	if stderr == "" {
		t.Fatalf("expected sandbox redirect error")
	}
}

func TestVulnHuntShellFeatureRedirectionChain_OutputRedirectTargetsStayLiteral(t *testing.T) {
	// A literal non-/dev/null target is still rejected statically (exit 2).
	stderr := runSimpleCommandValidationFailure(t, `echo data > /tmp/out`)
	if !strings.Contains(stderr, "> file redirection is not supported") {
		t.Fatalf("expected literal output redirect rejection, got %q", stderr)
	}

	// A dynamic target cannot be resolved statically, so it is refused by the
	// runtime check instead: the command fails, no file is written, and the
	// rest of the script still runs.
	target := filepath.Join(t.TempDir(), "out")
	stdout, stderr, code := runSimpleCommandScript(t,
		`TARGET=`+target+`; echo data > "$TARGET"; echo after`,
		"",
		allowAllCommandsOpt(),
	)
	if code != 0 || stdout != "after\n" {
		t.Fatalf("expected script to continue after refused dynamic redirect, code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "file redirection is only supported for /dev/null") {
		t.Fatalf("expected dynamic output redirect rejection, got %q", stderr)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("dynamic redirect must not create %q (err=%v)", target, err)
	}

	stdout, stderr, code = runSimpleCommandScript(t,
		"X=ok >/dev/null\necho \"after=$X\"\necho hidden >/dev/null\necho visible\n",
		"",
		allowAllCommandsOpt(),
	)
	if code != 0 {
		t.Fatalf("exit code=%d stderr=%q", code, stderr)
	}
	if stdout != "after=ok\nvisible\n" {
		t.Fatalf("stdout=%q, want assignment persistence and restored stdout", stdout)
	}
}

func TestVulnHuntShellFeatureExpansionChain_DynamicCommandStillUsesPolicyAndSandbox(t *testing.T) {
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

	stdout, stderr, _ := runSimpleCommandScript(t,
		"cmd=cat\n\"$cmd\" inside.txt\necho after\n",
		allowed,
		AllowedCommands([]string{"rshell:echo"}),
		AllowedPaths([]string{allowed}),
	)
	if strings.Contains(stdout, "inside") {
		t.Fatalf("expanded command bypassed AllowedCommands: stdout=%q stderr=%q", stdout, stderr)
	}
	if !strings.Contains(stderr, "command not allowed") {
		t.Fatalf("expected command-policy rejection, stderr=%q", stderr)
	}

	stdout, stderr, _ = runSimpleCommandScript(t,
		"cmd=cat\n\"$cmd\" ../outside/secret.txt\necho after\n",
		allowed,
		AllowedCommands([]string{"rshell:cat", "rshell:echo"}),
		AllowedPaths([]string{allowed}),
	)
	if strings.Contains(stdout, "secret") {
		t.Fatalf("expanded command bypassed AllowedPaths: stdout=%q stderr=%q", stdout, stderr)
	}
	if stderr == "" {
		t.Fatalf("expected sandbox rejection for outside file")
	}
}

func TestVulnHuntShellFeatureExpansionChain_AssignmentCommandSubstitutionUsesSandbox(t *testing.T) {
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

	stdout, stderr, _ := runSimpleCommandScript(t,
		"X=$(<../outside/secret.txt)\necho \"X=$X\"\n",
		allowed,
		AllowedCommands([]string{"rshell:cat", "rshell:echo"}),
		AllowedPaths([]string{allowed}),
	)
	if strings.Contains(stdout, "secret") {
		t.Fatalf("assignment command substitution read outside AllowedPaths: stdout=%q stderr=%q", stdout, stderr)
	}
	if !strings.Contains(stdout, "X=\n") {
		t.Fatalf("expected empty assignment after blocked shortcut, stdout=%q stderr=%q", stdout, stderr)
	}
	if stderr == "" {
		t.Fatalf("expected sandbox error for blocked assignment shortcut")
	}
}

func TestVulnHuntShellFeatureReadonlyBypass_AssignmentOnlyReadonlyTarget(t *testing.T) {
	stdout, stderr := runScriptWithReadonly(t, "RO_VAR=hacked\necho after=$RO_VAR\n")
	if !strings.Contains(stderr, "readonly variable") {
		t.Fatalf("expected readonly error, stderr=%q", stderr)
	}
	if strings.Contains(stdout, "hacked") {
		t.Fatalf("assignment-only command mutated readonly variable: stdout=%q stderr=%q", stdout, stderr)
	}
	if !strings.Contains(stdout, "after=original") {
		t.Fatalf("readonly variable not preserved: stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestVulnHuntShellFeatureSignalContext_PreCanceledSimpleCommandDoesNotMutateOrDispatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	stdout, stderr, _, err := runSimpleCommandScriptCtx(t, ctx, "X=leak\necho SHOULD_NOT_RUN\n", "", allowAllCommandsOpt())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got err=%v stdout=%q stderr=%q", err, stdout, stderr)
	}
	if stdout != "" || stderr != "" {
		t.Fatalf("pre-canceled run produced output: stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestVulnHuntShellFeatureSignalContext_CommandSubstitutionTimeoutIsFatal(t *testing.T) {
	stdout, stderr, _, err := runSimpleCommandScriptCtx(t,
		context.Background(),
		"X=$(while true; do true; done)\necho after\n",
		"",
		allowAllCommandsOpt(),
		MaxExecutionTime(20*time.Millisecond),
	)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got err=%v stdout=%q stderr=%q", err, stdout, stderr)
	}
	if strings.Contains(stdout, "after") {
		t.Fatalf("outer simple command continued after timed-out command substitution: stdout=%q", stdout)
	}
}

func TestVulnHuntShellFeatureSubshellIsolation_AssignmentOnlyAndInlineDoNotLeak(t *testing.T) {
	stdout, stderr, code := runSimpleCommandScript(t,
		"X=outer\n( X=inner; echo sub=$X )\necho parent=$X\n( X=inline echo inline=$X )\necho parent2=$X\n",
		"",
		allowAllCommandsOpt(),
	)
	if code != 0 {
		t.Fatalf("exit code=%d stderr=%q", code, stderr)
	}
	for _, want := range []string{"sub=inner", "parent=outer", "inline=outer", "parent2=outer"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout missing %q: %q", want, stdout)
		}
	}
}

func TestVulnHuntShellFeatureParserConfusion_AssignmentFormsRejectedBeforeExecution(t *testing.T) {
	cases := []string{
		"A+=x",
		"A[0]=x",
		"A=(x)",
		"A=~/secret echo \"$A\"",
		"~/bin/echo hi",
	}
	for _, script := range cases {
		t.Run(script, func(t *testing.T) {
			_ = runSimpleCommandValidationFailure(t, script)
		})
	}
}

func TestVulnHuntShellFeatureCompositionAttack_AssignmentStatusAndRedirectionTokenOrder(t *testing.T) {
	stdout, stderr, code := runSimpleCommandScript(t,
		"A=$(false) B=ok\necho \"status=$? B=$B\"\necho noisy 2>/dev/null X=ok\n",
		"",
		allowAllCommandsOpt(),
	)
	if code != 0 {
		t.Fatalf("exit code=%d stderr=%q stdout=%q", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "status=1 B=ok") {
		t.Fatalf("assignment-only command substitution status or later assignment lost: stdout=%q stderr=%q", stdout, stderr)
	}
	if !strings.Contains(stdout, "noisy X=ok") {
		t.Fatalf("assignment-looking argument after command word was misclassified: stdout=%q stderr=%q", stdout, stderr)
	}
}
