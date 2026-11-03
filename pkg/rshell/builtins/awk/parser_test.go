// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package awk

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePracticalAwkProgram(t *testing.T) {
	prog, err := parseProgram(`BEGIN { label = "sum=" } $2 > 1 { if ($1 == "skip") next; total += $2; printf "%s%d\n", label, total } END { print length($0), total }`)
	require.NoError(t, err)
	require.Len(t, prog.rules, 3)
	assert.Equal(t, ruleBegin, prog.rules[0].kind)
	assert.Equal(t, ruleNormal, prog.rules[1].kind)
	assert.Equal(t, ruleEnd, prog.rules[2].kind)
}

func TestParseRejectsUnsafeFeatures(t *testing.T) {
	for _, src := range []string{
		`{ system("sh") }`,
		`{ print $1 > "out" }`,
	} {
		_, err := parseProgram(src)
		require.Error(t, err, src)
	}
}

func TestParseRejectsLiteralZeroDivisor(t *testing.T) {
	tests := []struct {
		src  string
		want string
	}{
		{`BEGIN { print 1 || (1 / 0) }`, "division by zero attempted"},
		{`BEGIN { print 1 || (1 % -0) }`, "division by zero attempted in `%'"},
	}
	for _, test := range tests {
		_, err := parseProgram(test.src)
		require.EqualError(t, err, test.want, test.src)
	}
}

func TestParseFunctionParameterLimit(t *testing.T) {
	program := func(count int) string {
		params := make([]string, count)
		for i := range params {
			params[i] = fmt.Sprintf("p%d", i)
		}
		return fmt.Sprintf("function f(%s) { return f() } BEGIN { f() }", strings.Join(params, ","))
	}

	_, err := parseProgram(program(maxFunctionParameters))
	require.NoError(t, err)

	_, err = parseProgram(program(maxFunctionParameters + 1))
	require.EqualError(t, err, fmt.Sprintf(`function "f" has too many parameters (maximum %d)`, maxFunctionParameters))
}

func TestParseFunctionArgumentLimit(t *testing.T) {
	program := func(count int) string {
		args := make([]string, count)
		for i := range args {
			args[i] = "0"
		}
		return fmt.Sprintf("BEGIN { sprintf(%s) }", strings.Join(args, ","))
	}

	_, err := parseProgram(program(maxFunctionArguments))
	require.NoError(t, err)

	_, err = parseProgram(program(maxFunctionArguments + 1))
	require.EqualError(t, err, fmt.Sprintf(`function "sprintf" has too many arguments (maximum %d)`, maxFunctionArguments))
}

func TestParseRejectsUndefinedFunction(t *testing.T) {
	_, err := parseProgram(`BEGIN { missing(sprintf("%1048576s", "")) }`)
	require.EqualError(t, err, `function "missing" not defined`)
}

func TestCommandLineAssignmentNames(t *testing.T) {
	for _, name := range []string{
		"BEGIN", "else", "getline",
		"BEGINFILE", "switch",
		"eval", "include", "load", "namespace",
	} {
		assert.False(t, validCommandLineAssignmentName(name, nil), name)
	}
	for name := range supportedBuiltinFunctions {
		assert.False(t, validCommandLineAssignmentName(name, nil), name)
	}
	for name := range unsupportedBuiltinFunctions {
		assert.False(t, validCommandLineAssignmentName(name, nil), name)
	}

	prog := &program{functions: map[string]*functionDef{"userfunc": {}}}
	assert.True(t, validCommandLineAssignmentName("ordinary", prog))
	assert.True(t, validCommandLineAssignmentName("userfunc", nil))
	assert.False(t, validCommandLineAssignmentName("userfunc", prog))
}
