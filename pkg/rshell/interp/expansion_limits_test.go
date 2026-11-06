// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
)

func runExpansionScript(t *testing.T, script string, opts ...RunnerOption) (string, string, error) {
	t.Helper()
	prog, err := ParseScript(script, "")
	if err != nil {
		t.Fatalf("ParseScript: %v", err)
	}
	var stdout, stderr bytes.Buffer
	opts = append(opts, StdIO(nil, &stdout, &stderr))
	runner, err := New(opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { runner.Close() })
	err = runner.Run(context.Background(), prog)
	return stdout.String(), stderr.String(), err
}

func requireExitStatus(t *testing.T, err error, want ExitStatus) {
	t.Helper()
	var status ExitStatus
	if !errors.As(err, &status) || status != want {
		t.Fatalf("exit error = %v, want status %d", err, want)
	}
}

func TestDeniedCommandSkipsRemainingArgumentExpansion(t *testing.T) {
	stdout, stderr, err := runExpansionScript(t,
		"cat \"$(echo SIDE_EFFECT >&2)\"\nevilcmd_not_allowed {1..3000000}",
		AllowedCommands([]string{"rshell:echo"}),
	)
	requireExitStatus(t, err, 127)
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if strings.Contains(stderr, "SIDE_EFFECT") {
		t.Fatalf("denied command expanded a later argument: stderr=%q", stderr)
	}
	if !strings.Contains(stderr, "cat: command not allowed") {
		t.Fatalf("stderr = %q, want command-policy rejection", stderr)
	}
	if !strings.Contains(stderr, "evilcmd_not_allowed: command not allowed") {
		t.Fatalf("stderr = %q, want brace PoC command-policy rejection", stderr)
	}
	if strings.Contains(stderr, "brace expansion") {
		t.Fatalf("denied command evaluated brace expansion: stderr=%q", stderr)
	}
}

func TestBraceExpansionLimit(t *testing.T) {
	_, stderr, err := runExpansionScript(t,
		"true {1..100000}",
		AllowedCommands([]string{"rshell:true"}),
	)
	requireExitStatus(t, err, 1)
	if !strings.Contains(stderr, "brace expansion would exceed 16384 elements") {
		t.Fatalf("stderr = %q, want brace expansion limit", stderr)
	}
}

func TestRedirectExpansionLimits(t *testing.T) {
	for _, script := range []string{
		"true < {1..100000}",
		"true < " + strings.Repeat("$X", 11),
		"value=$(< " + strings.Repeat("$X", 11) + ")",
	} {
		t.Run(script, func(t *testing.T) {
			stdout, stderr, err := runExpansionScript(t, script+"\necho SHOULD_NOT_RUN",
				Env("X="+strings.Repeat("x", 1<<20)),
				AllowedCommands([]string{"rshell:true", "rshell:echo", "rshell:cat"}),
			)
			requireExitStatus(t, err, 1)
			if stdout != "" || !strings.Contains(stderr, "expansion exceeds") && !strings.Contains(stderr, "brace expansion would exceed") {
				t.Fatalf("stdout=%q, stderr=%q, want fatal expansion limit", stdout, stderr)
			}
		})
	}
}

func TestExpandedArgumentCountLimitSpansWords(t *testing.T) {
	_, stderr, err := runExpansionScript(t,
		"true {1..8192} {1..8193}",
		AllowedCommands([]string{"rshell:true"}),
	)
	requireExitStatus(t, err, 1)
	if !strings.Contains(stderr, "expansion exceeds maximum field count") {
		t.Fatalf("stderr = %q, want field-count limit", stderr)
	}
}

func TestExpandedArgumentCountLimitAllowsBoundary(t *testing.T) {
	_, stderr, err := runExpansionScript(t,
		"true {1..16384}",
		AllowedCommands([]string{"rshell:true"}),
	)
	if err != nil {
		t.Fatalf("Run: %v, stderr=%q", err, stderr)
	}
}

func TestExpandedArgumentByteLimit(t *testing.T) {
	value := strings.Repeat("x", 1<<20)
	script := "true " + strings.TrimSpace(strings.Repeat("$X ", 11))
	_, stderr, err := runExpansionScript(t, script,
		Env("X="+value),
		AllowedCommands([]string{"rshell:true"}),
	)
	requireExitStatus(t, err, 1)
	if !strings.Contains(stderr, "expansion exceeds maximum command size") {
		t.Fatalf("stderr = %q, want command expansion byte limit", stderr)
	}
}

func TestAssignmentExpansionCheckedBeforeJoining(t *testing.T) {
	value := strings.Repeat("x", 768<<10)
	stdout, stderr, err := runExpansionScript(t,
		"Y=$X$X\necho SHOULD_NOT_RUN",
		Env("X="+value),
		AllowedCommands([]string{"rshell:echo"}),
	)
	requireExitStatus(t, err, 1)
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "Y: value too large") {
		t.Fatalf("stderr = %q, want assignment size limit", stderr)
	}
}

func TestAssignmentCommandSubstitutionWithLiteralAllowed(t *testing.T) {
	stdout, stderr, err := runExpansionScript(t,
		"X=prefix$(echo value)\necho $X",
		AllowedCommands([]string{"rshell:echo"}),
	)
	if err != nil {
		t.Fatalf("Run: %v, stderr=%q", err, stderr)
	}
	if stdout != "prefixvalue\n" {
		t.Fatalf("stdout = %q, want %q", stdout, "prefixvalue\\n")
	}
}

func TestHeredocExpansionCheckedBeforeJoining(t *testing.T) {
	value := strings.Repeat("x", 1<<20)
	script := "cat <<EOF\n" + strings.Repeat("$X", 11) + "\nEOF\necho SHOULD_NOT_RUN"
	stdout, stderr, err := runExpansionScript(t, script,
		Env("X="+value),
		AllowedCommands([]string{"rshell:cat", "rshell:echo"}),
	)
	requireExitStatus(t, err, 1)
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "heredoc: content exceeds maximum size") {
		t.Fatalf("stderr = %q, want heredoc expansion limit", stderr)
	}
}

func TestExpansionRunBudgetSharedWithSubshell(t *testing.T) {
	runner, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { runner.Close() })
	runner.Reset()
	runner.expansionByteCount = &atomic.Int64{}
	sub := runner.subshell(false)
	if sub.expansionByteCount != runner.expansionByteCount {
		t.Fatal("subshell did not inherit the run-wide expansion budget")
	}
	if err := runner.chargeExpansionBytes(MaxExpandedBytesPerRun); err != nil {
		t.Fatalf("charging exactly the run limit: %v", err)
	}
	if err := sub.chargeExpansionBytes(1); err == nil {
		t.Fatal("subshell expansion exceeded the shared run budget without an error")
	}
}

func TestExpansionLimitStopsOuterScript(t *testing.T) {
	for name, script := range map[string]string{
		"command substitution": `echo "$(true {1..100000})"`,
		"subshell":             `(true {1..100000})`,
		"pipeline":             `true {1..100000} | true`,
	} {
		t.Run(name, func(t *testing.T) {
			stdout, stderr, err := runExpansionScript(t,
				script+"\necho SHOULD_NOT_RUN",
				AllowedCommands([]string{"rshell:echo", "rshell:true"}),
			)
			requireExitStatus(t, err, 1)
			if stdout != "" {
				t.Fatalf("stdout = %q, want empty", stdout)
			}
			if !strings.Contains(stderr, "brace expansion would exceed 16384 elements") {
				t.Fatalf("stderr = %q, want brace expansion limit", stderr)
			}
		})
	}
}
