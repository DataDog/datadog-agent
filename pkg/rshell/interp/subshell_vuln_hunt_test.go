// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// vuln-hunt 2026-05-18-initial-audit (target: subshell)
// Adversarial tests around subshell parsing & isolation. All cases must
// return a clean parse error — never a panic, hang, or sandbox bypass.

package interp

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"mvdan.cc/sh/v3/syntax"
)

func parseScriptWantErr(t *testing.T, src string) error {
	t.Helper()
	_, err := syntax.NewParser().Parse(strings.NewReader(src), "")
	return err
}

// TestVulnHuntShellParserConfusion_EmptySubshell verifies that `()` is rejected
// at parse time with a clean error message rather than panicking or being
// reinterpreted as a different construct that bypasses validation.
func TestVulnHuntShellParserConfusion_EmptySubshell(t *testing.T) {
	err := parseScriptWantErr(t, "()")
	if err == nil {
		t.Fatalf("expected parse error for empty subshell `()`, got nil")
	}
	// Parser treats `()` as a function definition stem and reports it.
	assert.Contains(t, err.Error(), "must be followed by a statement",
		"empty subshell should produce a clean parse error")
}

// TestVulnHuntShellParserConfusion_UnterminatedSubshell verifies that an
// unterminated subshell produces a parse error rather than hanging.
func TestVulnHuntShellParserConfusion_UnterminatedSubshell(t *testing.T) {
	err := parseScriptWantErr(t, "(echo hi")
	if err == nil {
		t.Fatalf("expected parse error for unterminated subshell, got nil")
	}
	assert.Contains(t, err.Error(), "reached EOF",
		"unterminated subshell should produce a clean parse error")
}

// TestVulnHuntShellParserConfusion_DeeplyNestedSubshell verifies that
// a deeply nested subshell parses without panicking. `((` collides with
// arithmetic in shell grammar, so spaces are required between opening
// parens for nested subshells to be recognized.
func TestVulnHuntShellParserConfusion_DeeplyNestedSubshell(t *testing.T) {
	src := strings.Repeat("( ", 50) + "echo ok" + strings.Repeat(" )", 50)
	_, err := syntax.NewParser().Parse(strings.NewReader(src), "")
	assert.NoError(t, err, "50-level nested subshell should parse cleanly")
}
