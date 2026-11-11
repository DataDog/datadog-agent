// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// Vulnerability-hunt regression coverage for campaign 2026-05-19-codex.

func TestVulnHuntSubsystemParserLexer_NULByteCannotBypassCommandPolicy(t *testing.T) {
	prog, err := ParseScript("ec\x00ho SHOULD_NOT_RUN\n", "nul-command")
	if err != nil {
		return
	}

	var stdout, stderr bytes.Buffer
	runner, err := New(StdIO(nil, &stdout, &stderr), AllowedCommands([]string{"rshell:false"}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = runner.Close() })

	runErr := runner.Run(context.Background(), prog)
	if runErr == nil {
		t.Fatalf("NUL-mutated command unexpectedly bypassed command policy: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), "SHOULD_NOT_RUN") {
		t.Fatalf("NUL-mutated command bypassed command policy: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}
