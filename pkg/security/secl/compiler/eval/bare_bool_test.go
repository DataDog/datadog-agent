// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import "testing"

// TestBareBoolFieldRecordedRegardlessOfPosition pins that a bare boolean field records its
// value wherever it is written.
//
// Unary is the only place that records it, and it is reached from two positions: an
// Expression, and the operand of a negation. Recording from fewer than both would tie the
// value to the syntax surrounding the field, its place in a logical chain, whether it is
// parenthesised, whether it is negated, none of which the language distinguishes.
//
// It matters because approvers are derived from the recorded values: for a rule whose only
// approver-capable field is a bare boolean, losing the record gives up kernel side filtering
// for the whole event type.
func TestBareBoolFieldRecordedRegardlessOfPosition(t *testing.T) {
	tests := []struct {
		expr     string
		expected bool
	}{
		{`process.is_root`, true},
		{`process.is_root && process.name == "ls"`, true},
		{`process.name == "ls" && process.is_root`, true},
		{`process.name == "ls" && process.is_root && process.uid == 0`, true},
		{`process.is_root || process.name == "ls"`, true},
		{`(process.is_root) && process.name == "ls"`, true},
		{`process.is_root == true && process.name == "ls"`, true},

		// negation records too, and does not depend on parentheses
		{`!process.is_root`, true},
		{`!(process.is_root)`, true},
		{`!process.is_root && process.name == "ls"`, true},
		{`!(process.is_root || process.name == "ls")`, true},

		// a rule that never mentions the field records nothing for it
		{`process.name == "ls"`, false},
	}

	model := &testModel{}

	for _, test := range tests {
		t.Run(test.expr, func(t *testing.T) {
			opts := newOptsWithParams(testConstants, nil)

			rule, err := parseRule(test.expr, model, opts)
			if err != nil {
				t.Fatalf("failed to compile: %s", err)
			}

			values := rule.GetFieldValues("process.is_root")
			if got := len(values) > 0; got != test.expected {
				t.Fatalf("expected recorded=%t for `%s`, got %t (%v)",
					test.expected, test.expr, got, values)
			}

			for _, v := range values {
				if v.Value != true {
					t.Errorf("expected the recorded value to be true, got %v", v.Value)
				}
			}
		})
	}
}
