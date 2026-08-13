// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCoverageOverTestdata runs the measurement over a handful of rules written the way
// the real ones are — a macro of plain strings, a macro of patterns including a `**` glob,
// a flags macro over SECL constants, a path glob, `allin`, a list-valued leaf — and
// requires that every one of them plans and that both engines answer what the rule says
// they should.
//
// It is the guard on the tool and on the front end at once: each of those constructs was
// a gap or a divergence at some point, and each case here is one that would have failed
// then.
func TestCoverageOverTestdata(t *testing.T) {
	set, err := readPolicies("testdata/policies")
	require.NoError(t, err)

	assert.Len(t, set.macros, 3)
	assert.Len(t, set.rules, 3, "the windows rule is not measured against the unix model")

	results, macroFailures, err := measure(set)
	require.NoError(t, err)
	assert.Empty(t, macroFailures)

	var cases, agreed int
	for _, result := range results {
		assert.Equal(t, stageDone, result.stopped, "%s: %v", result.rule.name, result.err)
		assert.Empty(t, result.disagreed, result.rule.name)
		assert.Empty(t, result.bothWrong, result.rule.name)
		assert.Empty(t, result.unevaluable, result.rule.name)

		cases += result.cases
		agreed += result.agreed
	}

	assert.Equal(t, cases, agreed)
	assert.Equal(t, 8, cases, "every declared case is compared")
}

// TestCoverageOverRealPolicies is the same measurement over a real rule set, which is not
// in this repository: point CEL_POLICY_DIR at a checkout of the agent rules to run it.
//
// It fails on a disagreement rather than on a rule that cannot be planned, because the
// two are different things — an unsupported construct is a gap with a number attached,
// while a disagreement is a rule that would fire differently in production.
func TestCoverageOverRealPolicies(t *testing.T) {
	dir := os.Getenv("CEL_POLICY_DIR")
	if dir == "" {
		t.Skip("set CEL_POLICY_DIR to a directory of agent rules")
	}

	set, err := readPolicies(dir)
	require.NoError(t, err)
	require.NotEmpty(t, set.rules)

	results, _, err := measure(set)
	require.NoError(t, err)

	var planned, cases, agreed int
	for _, result := range results {
		if result.stopped == stageDone {
			planned++
		}
		cases += result.cases
		agreed += result.agreed
		assert.Empty(t, result.disagreed, result.rule.name)
	}

	t.Logf("%d of %d rules planned, %d of %d declared cases compared and agreed",
		planned, len(set.rules), agreed, cases)
}
