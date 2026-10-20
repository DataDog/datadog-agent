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

func TestVulnHuntShellFeatureRedirectionChain_LineContinuationCannotHideOutputRedirect(t *testing.T) {
	dir := t.TempDir()

	stdout, stderr, code, err := runSimpleCommandScriptCtx(t,
		context.Background(),
		"echo safe \\\n> created.txt\n",
		dir,
		allowAllCommandsOpt(),
		AllowedPaths([]string{dir}),
	)
	var es ExitStatus
	if !errors.As(err, &es) || code != 2 {
		t.Fatalf("expected validation exit status 2, got code=%d err=%v stdout=%q stderr=%q", code, err, stdout, stderr)
	}
	if !strings.Contains(stderr, "> file redirection is not supported") {
		t.Fatalf("expected output redirect rejection, stderr=%q", stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "created.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("line-continued output redirect created file, stat err=%v", err)
	}
}

func TestVulnHuntShellFeatureRedirectionChain_LineContinuedInputRedirectUsesSandbox(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "allowed")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(allowed, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "data.txt"), []byte("classified\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, _ := runSimpleCommandScript(t,
		"cat < ../out\\\nside/data.txt\necho after\n",
		allowed,
		AllowedCommands([]string{"rshell:cat", "rshell:echo"}),
		AllowedPaths([]string{allowed}),
	)
	if strings.Contains(stdout, "classified") || strings.Contains(stderr, "classified") {
		t.Fatalf("line-continued input redirect leaked outside file: stdout=%q stderr=%q", stdout, stderr)
	}
	if !strings.Contains(stdout, "after") {
		t.Fatalf("script did not continue after blocked redirect: stdout=%q stderr=%q", stdout, stderr)
	}
	if stderr == "" {
		t.Fatalf("expected sandbox stderr for blocked line-continued input redirect")
	}
}

func TestVulnHuntShellFeatureExpansionChain_LineContinuedCommandUsesPolicyAndSandbox(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "allowed")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(allowed, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "data.txt"), []byte("classified\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, _ := runSimpleCommandScript(t,
		"c\\\nat ../outside/data.txt\necho after\n",
		allowed,
		AllowedCommands([]string{"rshell:cat", "rshell:echo"}),
		AllowedPaths([]string{allowed}),
	)
	if strings.Contains(stdout, "classified") || strings.Contains(stderr, "classified") {
		t.Fatalf("line-continued command leaked outside file: stdout=%q stderr=%q", stdout, stderr)
	}
	if !strings.Contains(stdout, "after") {
		t.Fatalf("script did not continue after blocked command read: stdout=%q stderr=%q", stdout, stderr)
	}
	if stderr == "" {
		t.Fatalf("expected sandbox stderr for line-continued cat command")
	}
}

func TestVulnHuntShellFeatureExpansionChain_LineContinuedCommandSubstitutionUsesSandbox(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "allowed")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(allowed, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "data.txt"), []byte("classified\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, _ := runSimpleCommandScript(t,
		"X=$(<../out\\\nside/data.txt)\necho \"X=$X\"\n",
		allowed,
		AllowedCommands([]string{"rshell:cat", "rshell:echo"}),
		AllowedPaths([]string{allowed}),
	)
	if strings.Contains(stdout, "classified") || strings.Contains(stderr, "classified") {
		t.Fatalf("line-continued command substitution leaked outside file: stdout=%q stderr=%q", stdout, stderr)
	}
	if !strings.Contains(stdout, "X=\n") {
		t.Fatalf("expected blocked substitution to expand empty, stdout=%q stderr=%q", stdout, stderr)
	}
	if stderr == "" {
		t.Fatalf("expected sandbox stderr for line-continued command substitution")
	}
}

func TestVulnHuntShellFeatureReadonlyBypass_LineContinuedReadonlyAssignmentRejected(t *testing.T) {
	stdout, stderr := runScriptWithReadonly(t,
		"RO_\\\nVAR=hacked\necho after=$RO_VAR\n")
	if !strings.Contains(stderr, "readonly variable") {
		t.Fatalf("expected readonly error, stderr=%q", stderr)
	}
	if strings.Contains(stdout, "hacked") || !strings.Contains(stdout, "after=original") {
		t.Fatalf("line-continued assignment changed readonly variable: stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestVulnHuntShellFeatureReadonlyBypass_LineContinuedForVariableRejected(t *testing.T) {
	stdout, stderr := runScriptWithReadonly(t,
		"for RO_\\\nVAR in hacked; do echo loop=$RO_VAR; done\necho after=$RO_VAR\n")
	if !strings.Contains(stderr, "readonly variable") {
		t.Fatalf("expected readonly error, stderr=%q", stderr)
	}
	if strings.Contains(stdout, "hacked") || !strings.Contains(stdout, "after=original") {
		t.Fatalf("line-continued for variable changed readonly variable: stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestVulnHuntShellFeatureSignalContext_PreCanceledLineContinuationDoesNotDispatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	stdout, stderr, _, err := runSimpleCommandScriptCtx(t, ctx, "ec\\\nho SHOULD_NOT_RUN\n", "", allowAllCommandsOpt())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got err=%v stdout=%q stderr=%q", err, stdout, stderr)
	}
	if stdout != "" || stderr != "" {
		t.Fatalf("pre-canceled line-continuation script produced output: stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestVulnHuntShellFeatureSubshellIsolation_LineContinuedSubshellAssignmentDoesNotLeak(t *testing.T) {
	stdout, stderr, code := runSimpleCommandScript(t,
		"X=outer\n( X=in\\\nner; echo sub=$X )\necho parent=$X\n",
		"",
		allowAllCommandsOpt(),
	)
	if code != 0 {
		t.Fatalf("exit code=%d stderr=%q", code, stderr)
	}
	if stdout != "sub=inner\nparent=outer\n" {
		t.Fatalf("line-continued subshell assignment leaked state: stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestVulnHuntShellFeatureParserConfusion_LineContinuationDoesNotHideBlockedConstructs(t *testing.T) {
	cases := map[string]string{
		"readonly": "read\\\nonly X=1\n",
		"pipe_all": "echo err \\\n|& cat\n",
		"arith":    "echo $((1\\\n+2))\n",
	}
	for name, script := range cases {
		t.Run(name, func(t *testing.T) {
			stderr := runSimpleCommandValidationFailure(t, script)
			if stderr == "" {
				t.Fatalf("expected validation stderr for %q", script)
			}
		})
	}
}

func TestVulnHuntShellFeatureDeclaredVsImplemented_LineContinuationScriptSizeCap(t *testing.T) {
	line := "echo x\\\n"
	script := strings.Repeat(line, MaxScriptBytes/len(line)+1)
	if len(script) <= MaxScriptBytes {
		t.Fatalf("test bug: generated script length %d <= cap %d", len(script), MaxScriptBytes)
	}

	_, err := ParseScript(script, "continued")
	if err == nil {
		t.Fatalf("over-cap line-continuation script parsed successfully")
	}
	if !strings.Contains(err.Error(), "exceeds maximum") {
		t.Fatalf("expected script-size cap error, got %v", err)
	}
}
