// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import (
	"container/list"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
)

// coveredRule compiles the expression with coverage enabled
func coveredRule(t *testing.T, expr string) *Rule {
	t.Helper()

	model := &testModel{}
	opts := newOptsWithParams(testConstants, nil).WithRuleCoverage(true)

	rule, err := parseRule(expr, model, opts)
	require.NoError(t, err)

	return rule
}

// pathSignatures returns the enumerated paths as `A=true B=false => true`
// strings, mapped to their hit count
func pathSignatures(coverage *Coverage) map[string]uint64 {
	signatures := make(map[string]uint64, len(coverage.Paths))
	for _, path := range coverage.Paths {
		signatures[fmt.Sprintf("%s => %t", path, path.Result)] = path.Hits
	}
	return signatures
}

func TestCoveragePathEnumeration(t *testing.T) {
	for _, test := range []struct {
		expr     string
		skeleton string
		paths    []string
	}{
		{
			// the reference case: a conjunction over an alternative
			expr:     `process.name == "aaa" && (process.uid == 0 || process.gid == 0)`,
			skeleton: `A && (B || C)`,
			paths: []string{
				"A=false => false",
				"A=true B=false C=false => false",
				"A=true B=false C=true => true",
				"A=true B=true => true",
			},
		},
		{
			expr:     `process.name == "aaa"`,
			skeleton: `A`,
			paths: []string{
				"A=false => false",
				"A=true => true",
			},
		},
		{
			// SECL chains logical operators to the right, so a conjunction of
			// three terms is a conjunction over a conjunction
			expr:     `process.name == "aaa" && process.uid == 0 && process.gid == 0`,
			skeleton: `A && B && C`,
			paths: []string{
				"A=false => false",
				"A=true B=false => false",
				"A=true B=true C=false => false",
				"A=true B=true C=true => true",
			},
		},
		{
			expr:     `!(process.uid == 0 || process.gid == 0)`,
			skeleton: `!(A || B)`,
			paths: []string{
				"A=false B=false => true",
				"A=false B=true => false",
				"A=true => false",
			},
		},
		{
			// a negated comparison stays a single leaf
			expr:     `process.name != "aaa"`,
			skeleton: `A`,
			paths: []string{
				"A=false => false",
				"A=true => true",
			},
		},
		{
			expr:     `(process.uid == 0 || process.gid == 0) && (process.name == "aaa" || process.argv0 == "bbb")`,
			skeleton: `(A || B) && (C || D)`,
			paths: []string{
				"A=false B=false => false",
				"A=false B=true C=false D=false => false",
				"A=false B=true C=false D=true => true",
				"A=false B=true C=true => true",
				"A=true C=false D=false => false",
				"A=true C=false D=true => true",
				"A=true C=true => true",
			},
		},
	} {
		t.Run(test.expr, func(t *testing.T) {
			rule := coveredRule(t, test.expr)

			coverage := rule.GetCoverage()
			require.NotNil(t, coverage)

			report := coverage.Report()
			assert.Empty(t, report.Unsupported)
			assert.Equal(t, test.skeleton, report.Skeleton)

			var got []string
			for _, path := range report.Paths {
				got = append(got, fmt.Sprintf("%s => %t", path, path.Result))
			}
			assert.Equal(t, test.paths, got)
			assert.Equal(t, len(test.paths), report.TotalPaths)
			assert.Zero(t, report.CoveredPaths)
		})
	}
}

func TestCoverageLeafExpressions(t *testing.T) {
	expr := `process.name == "aaa" && (process.uid in [1, 2] || not process.is_root)`
	rule := coveredRule(t, expr)

	report := rule.GetCoverage().Report()
	require.Len(t, report.Leaves, 3)

	// the leaves are named after their position in the rule, and reported in that
	// order, whatever order the operators evaluate them in
	assert.Equal(t, `process.name == "aaa"`, report.Leaves[0].Expression)
	assert.Equal(t, "A", report.Leaves[0].Name)
	assert.Equal(t, `process.uid in [1, 2]`, report.Leaves[1].Expression)
	assert.Equal(t, "B", report.Leaves[1].Name)
	assert.Equal(t, `process.is_root`, report.Leaves[2].Expression)
	assert.Equal(t, "C", report.Leaves[2].Name)

	// the alternative tests the bare boolean field before the array membership,
	// which is the more expensive of the two
	assert.Equal(t, `A && (!C || B)`, report.Skeleton)

	// every leaf points back at its own text in the rule expression
	for _, leaf := range report.Leaves {
		assert.Equal(t, leaf.Expression, expr[leaf.Offset:leaf.Offset+leaf.Length])
	}
}

func TestCoverageRecording(t *testing.T) {
	expr := `process.name == "aaa" && (process.uid == 0 || process.gid == 0)`
	rule := coveredRule(t, expr)

	newEvent := func(name string, uid, gid int) *Context {
		event := &testEvent{}
		event.process.name = name
		event.process.uid = uid
		event.process.gid = gid
		return NewContext(event)
	}

	// A=false: the alternative is never reached
	assert.False(t, rule.Eval(newEvent("bbb", 0, 0)))
	// A=true B=true
	assert.True(t, rule.Eval(newEvent("aaa", 0, 1)))
	// A=true B=false C=true
	assert.True(t, rule.Eval(newEvent("aaa", 1, 0)))
	// A=true B=false C=false, twice
	assert.False(t, rule.Eval(newEvent("aaa", 1, 1)))
	assert.False(t, rule.Eval(newEvent("aaa", 2, 2)))

	report := rule.GetCoverage().Report()
	assert.Equal(t, map[string]uint64{
		"A=false => false":                1,
		"A=true B=true => true":           1,
		"A=true B=false C=true => true":   1,
		"A=true B=false C=false => false": 2,
	}, pathSignatures(report))

	assert.Equal(t, 4, report.TotalPaths)
	assert.Equal(t, 4, report.CoveredPaths)
	assert.Equal(t, uint64(5), report.Evaluations)
	assert.Zero(t, report.Unmatched)

	// A was evaluated on every evaluation, B and C only when A was true
	assert.Equal(t, uint64(4), report.Leaves[0].True)
	assert.Equal(t, uint64(1), report.Leaves[0].False)
	assert.Equal(t, uint64(1), report.Leaves[1].True)
	assert.Equal(t, uint64(3), report.Leaves[1].False)
	assert.Equal(t, uint64(1), report.Leaves[2].True)
	assert.Equal(t, uint64(2), report.Leaves[2].False)
}

// TestCoveragePartialEvaluation checks that generating the partials of a covered
// rule, which recompiles its expression, does not record anything
func TestCoveragePartialEvaluation(t *testing.T) {
	rule := coveredRule(t, `process.name == "aaa" && process.uid == 0`)

	event := &testEvent{}
	event.process.name = "aaa"
	ctx := NewContext(event)

	_, err := rule.PartialEval(ctx, "process.name")
	require.NoError(t, err)

	assert.Zero(t, rule.GetCoverage().Report().Evaluations)
}

// TestCoverageIterator checks that a rule iterating over a register records one
// path per visited item rather than merging them into a single, bogus path
func TestCoverageIterator(t *testing.T) {
	rule := coveredRule(t, `process.list[A].key == 100 && process.list[A].value == "BBB"`)

	event := &testEvent{}
	event.process.list = list.New()
	event.process.list.PushBack(&testItem{key: 10, value: "AAA"})
	event.process.list.PushBack(&testItem{key: 100, value: "BBB"})
	event.process.list.PushBack(&testItem{key: 200, value: "CCC"})

	assert.True(t, rule.Eval(NewContext(event)))

	report := rule.GetCoverage().Report()
	assert.Equal(t, 3, report.TotalPaths)
	assert.Zero(t, report.Unmatched)
	// the first item does not match A, the second matches both leaves and stops
	// the iteration
	assert.Equal(t, map[string]uint64{
		"A=false => false":        1,
		"A=true B=false => false": 0,
		"A=true B=true => true":   1,
	}, pathSignatures(report))
}

// TestCoverageOperandOrder checks that the paths are enumerated in the order the
// operators evaluate their operands, which is by increasing cost rather than by
// position in the rule
func TestCoverageOperandOrder(t *testing.T) {
	// scanning an array costs more than comparing a single integer, so the
	// conjunction tests its second operand first
	rule := coveredRule(t, `process.uid in [1, 2, 3] && process.gid == 0`)

	report := rule.GetCoverage().Report()
	assert.Equal(t, "B && A", report.Skeleton)
	assert.Equal(t, `process.uid in [1, 2, 3]`, report.Leaves[0].Expression)
	assert.Equal(t, `process.gid == 0`, report.Leaves[1].Expression)

	var got []string
	for _, path := range report.Paths {
		got = append(got, fmt.Sprintf("%s => %t", path, path.Result))
	}
	assert.Equal(t, []string{
		"B=false => false",
		"B=true A=false => false",
		"B=true A=true => true",
	}, got)

	// the array is never scanned when the single comparison already fails
	event := &testEvent{}
	event.process.uid = 1
	event.process.gid = 1
	assert.False(t, rule.Eval(NewContext(event)))

	report = rule.GetCoverage().Report()
	assert.Zero(t, report.Unmatched)
	assert.Equal(t, map[string]uint64{
		"B=false => false":        1,
		"B=true A=false => false": 0,
		"B=true A=true => true":   0,
	}, pathSignatures(report))
	assert.Zero(t, report.Leaves[0].True+report.Leaves[0].False)
}

// TestCoverageSharedMacro checks that two rules sharing a macro get leaves of
// their own: the evaluator of a macro is shared by every rule that uses it, so
// instrumenting it in place would make the two rules record into each other
func TestCoverageSharedMacro(t *testing.T) {
	model := &testModel{}
	opts := newOptsWithParams(testConstants, nil).WithRuleCoverage(true)

	macro, err := NewMacro("is_passwd", `open.filename == "/etc/passwd"`, model, ast.NewParsingContext(false), opts)
	require.NoError(t, err)
	opts.MacroStore.Add(macro)

	first, err := parseRule(`is_passwd && process.uid == 0`, model, opts)
	require.NoError(t, err)
	second, err := parseRule(`is_passwd && process.gid == 0`, model, opts)
	require.NoError(t, err)

	// each rule sees the macro as its own first leaf
	assert.Equal(t, "A && B", first.GetCoverage().Report().Skeleton)
	assert.Equal(t, "A && B", second.GetCoverage().Report().Skeleton)
	assert.Equal(t, "is_passwd", first.GetCoverage().Report().Leaves[0].Expression)
	assert.Equal(t, "is_passwd", second.GetCoverage().Report().Leaves[0].Expression)

	event := &testEvent{}
	event.open.filename = "/etc/passwd"
	event.process.uid = 0
	event.process.gid = 1

	assert.True(t, first.Eval(NewContext(event)))

	// only the rule that was evaluated recorded anything
	assert.Equal(t, map[string]uint64{
		"A=false => false":        0,
		"A=true B=false => false": 0,
		"A=true B=true => true":   1,
	}, pathSignatures(first.GetCoverage().Report()))
	assert.Zero(t, second.GetCoverage().Report().Evaluations)

	assert.False(t, second.Eval(NewContext(event)))
	assert.Equal(t, map[string]uint64{
		"A=false => false":        0,
		"A=true B=false => false": 1,
		"A=true B=true => true":   0,
	}, pathSignatures(second.GetCoverage().Report()))
	assert.Equal(t, uint64(1), first.GetCoverage().Report().Evaluations)
}

// TestCoverageConstantOperand covers the operands that are folded to a constant
// at compile time, which the generated evaluator does not always short-circuit
// the way a comparison would
func TestCoverageConstantOperand(t *testing.T) {
	for _, test := range []struct {
		expr   string
		uid    int
		paths  []string
		result bool
	}{
		{
			// Or combines both sides instead of testing them in turn, so the
			// comparison is evaluated even though the constant already decides
			expr:   `true || process.uid == 0`,
			uid:    1,
			paths:  []string{"A=false => true", "A=true => true"},
			result: true,
		},
		{
			expr:   `false || process.uid == 0`,
			uid:    1,
			paths:  []string{"A=false => false", "A=true => true"},
			result: false,
		},
		{
			// a false constant does short-circuit an And, so nothing is evaluated
			expr:   `false && process.uid == 0`,
			uid:    0,
			paths:  []string{"<constant>"},
			result: false,
		},
		{
			expr:   `true && process.uid == 0`,
			uid:    0,
			paths:  []string{"A=false => false", "A=true => true"},
			result: true,
		},
		{
			expr:   `process.uid == 0 || false`,
			uid:    1,
			paths:  []string{"A=false => false", "A=true => true"},
			result: false,
		},
		{
			expr:   `process.uid == 0 && true`,
			uid:    1,
			paths:  []string{"A=false => false", "A=true => true"},
			result: false,
		},
	} {
		t.Run(test.expr, func(t *testing.T) {
			rule := coveredRule(t, test.expr)

			var got []string
			for _, path := range rule.GetCoverage().Report().Paths {
				if len(path.Conditions) == 0 {
					got = append(got, path.String())
					continue
				}
				got = append(got, fmt.Sprintf("%s => %t", path, path.Result))
			}
			assert.Equal(t, test.paths, got)

			event := &testEvent{}
			event.process.uid = test.uid
			assert.Equal(t, test.result, rule.Eval(NewContext(event)))

			// the path walked at runtime is one of the enumerated ones
			report := rule.GetCoverage().Report()
			assert.Zero(t, report.Unmatched)
			assert.Equal(t, uint64(1), report.Evaluations)
			assert.Equal(t, 1, report.CoveredPaths)
		})
	}
}

func TestCoverageDisabledByDefault(t *testing.T) {
	model := &testModel{}
	rule, err := parseRule(`process.name == "aaa"`, model, newOptsWithParams(testConstants, nil))
	require.NoError(t, err)

	assert.Nil(t, rule.GetCoverage())
}

func TestCoverageReset(t *testing.T) {
	rule := coveredRule(t, `process.name == "aaa"`)

	event := &testEvent{}
	event.process.name = "aaa"
	assert.True(t, rule.Eval(NewContext(event)))
	assert.Equal(t, uint64(1), rule.GetCoverage().Report().Evaluations)

	rule.GetCoverage().Reset()

	report := rule.GetCoverage().Report()
	assert.Zero(t, report.Evaluations)
	assert.Zero(t, report.CoveredPaths)
}

func TestCoverageSpan(t *testing.T) {
	for _, test := range []struct {
		expr     string
		start    int
		expected string
	}{
		{expr: `a == "x" && b == "y"`, start: 0, expected: `a == "x"`},
		{expr: `a == "x" && b == "y"`, start: 12, expected: `b == "y"`},
		{expr: `a == "x" and b == "y"`, start: 0, expected: `a == "x"`},
		{expr: `(a == "x" || b == "y") && c == 1`, start: 1, expected: `a == "x"`},
		{expr: `(a == "x" || b == "y") && c == 1`, start: 13, expected: `b == "y"`},
		// operators inside a quoted literal are not operators
		{expr: `a == "x && y" || b == 1`, start: 0, expected: `a == "x && y"`},
		{expr: `a == "x) y" || b == 1`, start: 0, expected: `a == "x) y"`},
		{expr: `a == "x \" && y" || b == 1`, start: 0, expected: `a == "x \" && y"`},
		// an identifier ending in `or` is not the `or` operator
		{expr: `a.factor == 1 || b == 1`, start: 0, expected: `a.factor == 1`},
		{expr: `a == 1 || b.and_c == 1`, start: 10, expected: `b.and_c == 1`},
		// a comparison over a parenthesized boolean expression is one leaf
		{expr: `(a == 1 || b == 1) == true && c == 1`, start: 0, expected: `(a == 1 || b == 1) == true`},
	} {
		t.Run(test.expr, func(t *testing.T) {
			offset, length := covSpan(test.expr, test.start)
			assert.Equal(t, test.expected, test.expr[offset:offset+length])
		})
	}
}

func TestCoverageLeafName(t *testing.T) {
	for index, expected := range map[int]string{0: "A", 25: "Z", 26: "AA", 51: "AZ", 52: "BA"} {
		assert.Equal(t, expected, covLeafName(index))
	}
}

func TestCoverageTooManyLeaves(t *testing.T) {
	var builder strings.Builder
	builder.WriteString("process.uid == 0")
	for i := 0; i < maxCoverageLeaves; i++ {
		fmt.Fprintf(&builder, " || process.uid == %d", i+1)
	}

	rule := coveredRule(t, builder.String())

	report := rule.GetCoverage().Report()
	assert.Equal(t, errTooManyCoverageLeaves.Error(), report.Unsupported)
	assert.Empty(t, report.Paths)

	// the instrumentation is inert, and the rule still evaluates
	event := &testEvent{}
	event.process.uid = 3
	assert.True(t, rule.Eval(NewContext(event)))
	assert.Zero(t, rule.GetCoverage().Report().Evaluations)
}
