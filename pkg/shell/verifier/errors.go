// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package verifier

import (
	"fmt"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// VerificationViolation represents a single safety violation found during AST verification.
type VerificationViolation struct {
	Pos      syntax.Pos
	Category string // "command", "flag", "shell_feature", "redirect"
	Message  string
}

// String returns a human-readable representation of the violation.
func (v VerificationViolation) String() string {
	if v.Pos.IsValid() {
		return fmt.Sprintf("%d:%d: [%s] %s", v.Pos.Line(), v.Pos.Col(), v.Category, v.Message)
	}
	return fmt.Sprintf("[%s] %s", v.Category, v.Message)
}

// VerificationError is returned when one or more safety violations are found.
type VerificationError struct {
	Violations []VerificationViolation
}

// Error returns a human-readable summary of all violations.
func (e *VerificationError) Error() string {
	if len(e.Violations) == 1 {
		return fmt.Sprintf("verification failed: %s", e.Violations[0].Message)
	}
	msgs := make([]string, len(e.Violations))
	for i, v := range e.Violations {
		msgs[i] = v.String()
	}
	return fmt.Sprintf("verification failed with %d violations:\n%s", len(e.Violations), strings.Join(msgs, "\n"))
}
