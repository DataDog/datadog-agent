// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package verifier provides AST-based safety verification for shell scripts.
// It parses POSIX shell commands using mvdan/sh, walks the AST to verify that
// only allowed commands, flags, and shell features are used, and rejects any
// script that contains potentially dangerous constructs.
package verifier

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// Verify parses a shell script string and verifies it for safety.
// Returns nil if the script is safe, or a *VerificationError with all violations.
func Verify(script string) error {
	parser := syntax.NewParser(syntax.KeepComments(false))
	f, err := parser.Parse(strings.NewReader(script), "")
	if err != nil {
		return &VerificationError{
			Violations: []VerificationViolation{
				{
					Category: "shell_feature",
					Message:  "failed to parse script: " + err.Error(),
				},
			},
		}
	}
	return VerifyAST(f)
}

// VerifyAST verifies a pre-parsed AST for safety.
// Returns nil if the script is safe, or a *VerificationError with all violations.
func VerifyAST(f *syntax.File) error {
	v := &verifier{}
	v.verifyFile(f)
	if len(v.violations) > 0 {
		return &VerificationError{Violations: v.violations}
	}
	return nil
}

// verifier accumulates violations during AST traversal.
type verifier struct {
	violations []VerificationViolation
}

// addViolation records a safety violation.
func (v *verifier) addViolation(pos syntax.Pos, category, message string) {
	v.violations = append(v.violations, VerificationViolation{
		Pos:      pos,
		Category: category,
		Message:  message,
	})
}
