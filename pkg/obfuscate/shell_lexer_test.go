// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package obfuscate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type testShellCommand struct {
	command string
	tokens  []ShellToken
}

func compareGeneratedTokens(t testShellCommand) bool {
	generatedTokens := parseShell(t.command)
	expectedTokens := t.tokens

	if len(generatedTokens) != len(expectedTokens) {
		return false
	}

	for i, token := range generatedTokens {
		if token.kind != expectedTokens[i].kind || token.val != expectedTokens[i].val {
			return false
		}
	}

	return true
}

func TestParseBasicCommands(t *testing.T) {
	tests := []testShellCommand{
		{
			command: "echo",
			tokens: []ShellToken{
				{
					kind: Executable,
					val:  "echo",
				},
			},
		},
		{
			command: "echo   ",
			tokens: []ShellToken{
				{
					kind: Executable,
					val:  "echo",
				},
			},
		},
		{
			command: "   test echo",
			tokens: []ShellToken{
				{
					kind: Executable,
					val:  "test",
				},
				{
					kind: Field,
					val:  "echo",
				},
			},
		},
	}

	for _, test := range tests {
		assert.True(t, compareGeneratedTokens(test))
	}
}

func TestParseBasicCommandsWithArgs(t *testing.T) {
	tests := []testShellCommand{
		{
			command: "ls dir1 dir2",
			tokens: []ShellToken{
				{
					kind: Executable,
					val:  "ls",
				},
				{
					kind: Field,
					val:  "dir1",
				},
				{
					kind: Field,
					val:  "dir2",
				},
			},
		},
	}

	for _, test := range tests {
		assert.True(t, compareGeneratedTokens(test))
	}
}

func TestParseBasicCommandsWithStringArg(t *testing.T) {
	tests := []testShellCommand{
		{
			command: "ls \"dir1 dir2\"",
			tokens: []ShellToken{
				{
					kind: Executable,
					val:  "ls",
				},
				{
					kind: DoubleQuote,
					val:  "\"",
				},
				{
					kind: Field,
					val:  "dir1 dir2",
				},
				{
					kind: DoubleQuote,
					val:  "\"",
				},
			},
		},
	}

	for _, test := range tests {
		assert.True(t, compareGeneratedTokens(test))
	}
}

func TestParseBasicCommandsWithStringArgSimpleQuote(t *testing.T) {
	tests := []testShellCommand{
		{
			command: "ls 'dir1 dir2'",
			tokens: []ShellToken{
				{
					kind: Executable,
					val:  "ls",
				},
				{
					kind: SingleQuote,
					val:  "'",
				},
				{
					kind: Field,
					val:  "dir1 dir2",
				},
				{
					kind: SingleQuote,
					val:  "'",
				},
			},
		},
	}

	for _, test := range tests {
		assert.True(t, compareGeneratedTokens(test))
	}
}

func TestParseBasicCommandsWithBackticks(t *testing.T) {
	tests := []testShellCommand{
		{
			command: "ls `which w` -li",
			tokens: []ShellToken{
				{
					kind: Executable,
					val:  "ls",
				},
				{
					kind: Backticks,
					val:  "`",
				},
				{
					kind: Executable,
					val:  "which",
				},
				{
					kind: Field,
					val:  "w",
				},
				{
					kind: Backticks,
					val:  "`",
				},
				{
					kind: Field,
					val:  "-li",
				},
			},
		},
		{
			command: "ls `TOTO=plop which w` -li",
			tokens: []ShellToken{
				{
					kind: Executable,
					val:  "ls",
				},
				{
					kind: Backticks,
					val:  "`",
				},
				{
					kind: VariableDefinition,
					val:  "TOTO",
				},
				{
					kind: Equal,
					val:  "=",
				},
				{
					kind: Field,
					val:  "plop",
				},
				{
					kind: Executable,
					val:  "which",
				},
				{
					kind: Field,
					val:  "w",
				},
				{
					kind: Backticks,
					val:  "`",
				},
				{
					kind: Field,
					val:  "-li",
				},
			},
		},
		{
			command: "`echo echo` \"plop\"",
			tokens: []ShellToken{
				{
					kind: Backticks,
					val:  "`",
				},
				{
					kind: Executable,
					val:  "echo",
				},
				{
					kind: Field,
					val:  "echo",
				},
				{
					kind: Backticks,
					val:  "`",
				},
				{
					kind: DoubleQuote,
					val:  "\"",
				},
				{
					kind: Field,
					val:  "plop",
				},
				{
					kind: DoubleQuote,
					val:  "\"",
				},
			},
		},
	}

	for _, test := range tests {
		assert.True(t, compareGeneratedTokens(test))
	}
}

func TestParseBasicCommandsWithDollarExec(t *testing.T) {
	tests := []testShellCommand{
		{
			command: "ls $(which w) -li",
			tokens: []ShellToken{
				{
					kind: Executable,
					val:  "ls",
				},
				{
					kind: Dollar,
					val:  "$",
				},
				{
					kind: ParentheseOpen,
					val:  "(",
				},
				{
					kind: Executable,
					val:  "which",
				},
				{
					kind: Field,
					val:  "w",
				},
				{
					kind: ParentheseClose,
					val:  ")",
				},
				{
					kind: Field,
					val:  "-li",
				},
			},
		},
		{
			command: "ls $(TOTO=plop which w) -li",
			tokens: []ShellToken{
				{
					kind: Executable,
					val:  "ls",
				},
				{
					kind: Dollar,
					val:  "$",
				},
				{
					kind: ParentheseOpen,
					val:  "(",
				},
				{
					kind: VariableDefinition,
					val:  "TOTO",
				},
				{
					kind: Equal,
					val:  "=",
				},
				{
					kind: Field,
					val:  "plop",
				},
				{
					kind: Executable,
					val:  "which",
				},
				{
					kind: Field,
					val:  "w",
				},
				{
					kind: ParentheseClose,
					val:  ")",
				},
				{
					kind: Field,
					val:  "-li",
				},
			},
		},
		{
			command: "$(echo echo) \"plop\"",
			tokens: []ShellToken{
				{
					kind: Dollar,
					val:  "$",
				},
				{
					kind: ParentheseOpen,
					val:  "(",
				},
				{
					kind: Executable,
					val:  "echo",
				},
				{
					kind: Field,
					val:  "echo",
				},
				{
					kind: ParentheseClose,
					val:  ")",
				},
				{
					kind: DoubleQuote,
					val:  "\"",
				},
				{
					kind: Field,
					val:  "plop",
				},
				{
					kind: DoubleQuote,
					val:  "\"",
				},
			},
		},
	}

	for _, test := range tests {
		assert.True(t, compareGeneratedTokens(test))
	}
}

func TestParseCommandsWithDollarExecInQuotedString(t *testing.T) {
	tests := []testShellCommand{
		{
			command: "ping -c 2 127.0.0.1 \"$(cat /etc/passwd)\"",
			tokens: []ShellToken{
				{
					kind: Executable,
					val:  "ping",
				},
				{
					kind: Field,
					val:  "-c",
				},
				{
					kind: Field,
					val:  "2",
				},
				{
					kind: Field,
					val:  "127.0.0.1",
				},
				{
					kind: DoubleQuote,
					val:  "\"",
				},
				{
					kind: Dollar,
					val:  "$",
				},
				{
					kind: ParentheseOpen,
					val:  "(",
				},
				{
					kind: Executable,
					val:  "cat",
				},
				{
					kind: Field,
					val:  "/etc/passwd",
				},
				{
					kind: ParentheseClose,
					val:  ")",
				},
				{
					kind: DoubleQuote,
					val:  "\"",
				},
			},
		},
		{
			command: "ping -c 2 127.0.0.1 \"some text: $(cat /etc/passwd) and other text\"",
			tokens: []ShellToken{
				{
					kind: Executable,
					val:  "ping",
				},
				{
					kind: Field,
					val:  "-c",
				},
				{
					kind: Field,
					val:  "2",
				},
				{
					kind: Field,
					val:  "127.0.0.1",
				},
				{
					kind: DoubleQuote,
					val:  "\"",
				},
				{
					kind: Field,
					val:  "some text: ",
				},
				{
					kind: Dollar,
					val:  "$",
				},
				{
					kind: ParentheseOpen,
					val:  "(",
				},
				{
					kind: Executable,
					val:  "cat",
				},
				{
					kind: Field,
					val:  "/etc/passwd",
				},
				{
					kind: ParentheseClose,
					val:  ")",
				},
				{
					kind: Field,
					val:  " and other text",
				},
				{
					kind: DoubleQuote,
					val:  "\"",
				},
			},
		},
		{
			command: "echo \"$(echo hello) $(echo world)\"",
			tokens: []ShellToken{
				{
					kind: Executable,
					val:  "echo",
				},
				{
					kind: DoubleQuote,
					val:  "\"",
				},
				{
					kind: Dollar,
					val:  "$",
				},
				{
					kind: ParentheseOpen,
					val:  "(",
				},
				{
					kind: Executable,
					val:  "echo",
				},
				{
					kind: Field,
					val:  "hello",
				},
				{
					kind: ParentheseClose,
					val:  ")",
				},
				{
					kind: Field,
					val:  " ",
				},
				{
					kind: Dollar,
					val:  "$",
				},
				{
					kind: ParentheseOpen,
					val:  "(",
				},
				{
					kind: Executable,
					val:  "echo",
				},
				{
					kind: Field,
					val:  "world",
				},
				{
					kind: ParentheseClose,
					val:  ")",
				},
				{
					kind: DoubleQuote,
					val:  "\"",
				},
			},
		},
		{
			command: "echo \"[DEBUG] $(echo hello) - $(echo $(echo $(echo world 1 2)))\" '$(echo nope)' $(echo plop) $(echo \"plop\")",
			tokens: []ShellToken{
				{
					kind: Executable,
					val:  "echo",
				},
				{
					kind: DoubleQuote,
					val:  "\"",
				},
				{
					kind: Field,
					val:  "[DEBUG] ",
				},
				{
					kind: Dollar,
					val:  "$",
				},
				{
					kind: ParentheseOpen,
					val:  "(",
				},
				{
					kind: Executable,
					val:  "echo",
				},
				{
					kind: Field,
					val:  "hello",
				},
				{
					kind: ParentheseClose,
					val:  ")",
				},
				{
					kind: Field,
					val:  " - ",
				},
				{
					kind: Dollar,
					val:  "$",
				},
				{
					kind: ParentheseOpen,
					val:  "(",
				},
				{
					kind: Executable,
					val:  "echo",
				},
				{
					kind: Dollar,
					val:  "$",
				},
				{
					kind: ParentheseOpen,
					val:  "(",
				},
				{
					kind: Executable,
					val:  "echo",
				},
				{
					kind: Dollar,
					val:  "$",
				},
				{
					kind: ParentheseOpen,
					val:  "(",
				},
				{
					kind: Executable,
					val:  "echo",
				},
				{
					kind: Field,
					val:  "world",
				},
				{
					kind: Field,
					val:  "1",
				},
				{
					kind: Field,
					val:  "2",
				},
				{
					kind: ParentheseClose,
					val:  ")",
				},
				{
					kind: ParentheseClose,
					val:  ")",
				},
				{
					kind: ParentheseClose,
					val:  ")",
				},
				{
					kind: DoubleQuote,
					val:  "\"",
				},
				{
					kind: SingleQuote,
					val:  "'",
				},
				{
					kind: Field,
					val:  "$(echo nope)",
				},
				{
					kind: SingleQuote,
					val:  "'",
				},
				{
					kind: Dollar,
					val:  "$",
				},
				{
					kind: ParentheseOpen,
					val:  "(",
				},
				{
					kind: Executable,
					val:  "echo",
				},
				{
					kind: Field,
					val:  "plop",
				},
				{
					kind: ParentheseClose,
					val:  ")",
				},
				{
					kind: Dollar,
					val:  "$",
				},
				{
					kind: ParentheseOpen,
					val:  "(",
				},
				{
					kind: Executable,
					val:  "echo",
				},
				{
					kind: DoubleQuote,
					val:  "\"",
				},
				{
					kind: Field,
					val:  "plop",
				},
				{
					kind: DoubleQuote,
					val:  "\"",
				},
				{
					kind: ParentheseClose,
					val:  ")",
				},
			},
		},
	}

	for _, test := range tests {
		assert.True(t, compareGeneratedTokens(test))
	}
}

/*
Test parsing a command with a quoted string and an escaped quoted string
This test is not supported by the current parser, no check on escaping is done, meaning this test will fail

func TestParseCommandWithQuotedStringAndEscapedQuotedString(t *testing.T) {
	tests := []testShellCommand{
		{
			command: "echo \"$(echo \\\"test\\\")\"",
			tokens: []ShellToken{
				{
					kind: Executable,
					val:  "echo",
				},
				{
					kind: DoubleQuote,
					val:  "\"",
				},
				{
					kind: Dollar,
					val:  "$",
				},
				{
					kind: ParentheseOpen,
					val:  "(",
				},
				{
					kind: Executable,
					val:  "echo",
				},
				{
					kind: DoubleQuote,
					val:  "\"",
				},
				{
					kind: Field,
					val:  "test",
				},
				{
					kind: DoubleQuote,
					val:  "\"",
				},
				{
					kind: ParentheseClose,
					val:  ")",
				},
				{
					kind: DoubleQuote,
					val:  "\"",
				},
			},
		},
	}

	for _, test := range tests {
		assert.True(t, compareGeneratedTokens(test))
	}
}
*/

func TestParseCommandWithDollarExecWithinBackticks(t *testing.T) {
	tests := []testShellCommand{
		{
			command: "echo `echo $(echo hello)`",
			tokens: []ShellToken{
				{
					kind: Executable,
					val:  "echo",
				},
				{
					kind: Backticks,
					val:  "`",
				},
				{
					kind: Executable,
					val:  "echo",
				},
				{
					kind: Dollar,
					val:  "$",
				},
				{
					kind: ParentheseOpen,
					val:  "(",
				},
				{
					kind: Executable,
					val:  "echo",
				},
				{
					kind: Field,
					val:  "hello",
				},
				{
					kind: ParentheseClose,
					val:  ")",
				},
				{
					kind: Backticks,
					val:  "`",
				},
			},
		},
	}

	for _, test := range tests {
		assert.True(t, compareGeneratedTokens(test))
	}
}

func TestParseBasicCommandsWithBackticksAndDollarExec(t *testing.T) {
	tests := []testShellCommand{
		{
			command: "$(echo `echo echo`) \"toto\"",
			tokens: []ShellToken{
				{
					kind: Dollar,
					val:  "$",
				},
				{
					kind: ParentheseOpen,
					val:  "(",
				},
				{
					kind: Executable,
					val:  "echo",
				},
				{
					kind: Backticks,
					val:  "`",
				},
				{
					kind: Executable,
					val:  "echo",
				},
				{
					kind: Field,
					val:  "echo",
				},
				{
					kind: Backticks,
					val:  "`",
				},
				{
					kind: ParentheseClose,
					val:  ")",
				},
				{
					kind: DoubleQuote,
					val:  "\"",
				},
				{
					kind: Field,
					val:  "toto",
				},
				{
					kind: DoubleQuote,
					val:  "\"",
				},
			},
		},
		{
			command: "`echo $(echo echo)` \"toto\"",
			tokens: []ShellToken{
				{
					kind: Backticks,
					val:  "`",
				},
				{
					kind: Executable,
					val:  "echo",
				},
				{
					kind: Dollar,
					val:  "$",
				},
				{
					kind: ParentheseOpen,
					val:  "(",
				},
				{
					kind: Executable,
					val:  "echo",
				},
				{
					kind: Field,
					val:  "echo",
				},
				{
					kind: ParentheseClose,
					val:  ")",
				},
				{
					kind: Backticks,
					val:  "`",
				},
				{
					kind: DoubleQuote,
					val:  "\"",
				},
				{
					kind: Field,
					val:  "toto",
				},
				{
					kind: DoubleQuote,
					val:  "\"",
				},
			},
		},
	}

	for _, test := range tests {
		assert.True(t, compareGeneratedTokens(test))
	}
}

/*
Unsupported test case:
Backticks subcommands are supported but at the first level (not nested).
Backticks that are escaped will be resolved are normal backticks, resulting of the end of the current backtick expression or the start.
Executable tokens are not set as executable if they are in a backtick expression.
We can't know the start and the end of a backtick expression if it's escaped.

func TestParseBasicCommandsWithNestedBackticks(t *testing.T) {
	tests := []testShellCommand{
		{
			command: "echo `echo `echo echo``",
			tokens: []ShellToken{
				{
					kind: Executable,
					val:  "echo",
				},
				{
					kind: Backticks,
					val:  "`",
				},
				{
					kind: Executable,
					val:  "echo",
				},
				{
					kind: Backticks,
					val:  "`",
				},
				{
					kind: Executable,
					val:  "echo",
				},
				{
					kind: Field,
					val:  "echo",
				},
				{
					kind: Backticks,
					val:  "`",
				},
				{
					kind: Backticks,
					val:  "`",
				},
			},
		},
	}

	for _, test := range tests {
		assert.True(t, compareGeneratedTokens(test))
	}
}
*/

func TestParseBasicCommandsWithUnfinishedStringDoubleQuote(t *testing.T) {
	tests := []testShellCommand{
		{
			command: "ls \"dir1 dir2",
			tokens: []ShellToken{
				{
					kind: Executable,
					val:  "ls",
				},
				{
					kind: DoubleQuote,
					val:  "\"",
				},
				{
					kind: Field,
					val:  "dir1 dir2",
				},
			},
		},
	}

	for _, test := range tests {
		assert.True(t, compareGeneratedTokens(test))
	}
}

func TestParseBasicCommandsWithUnfinishedStringSingleQuote(t *testing.T) {
	tests := []testShellCommand{
		{
			command: "ls 'dir1 dir2",
			tokens: []ShellToken{
				{
					kind: Executable,
					val:  "ls",
				},
				{
					kind: SingleQuote,
					val:  "'",
				},
				{
					kind: Field,
					val:  "dir1 dir2",
				},
			},
		},
	}

	for _, test := range tests {
		assert.True(t, compareGeneratedTokens(test))
	}
}

func TestParseBasicCommandsWithStringArgPseudoMulti(t *testing.T) {
	tests := []testShellCommand{
		{
			command: "ls 'dir1\\'; cat 'dir2'",
			tokens: []ShellToken{
				{
					kind: Executable,
					val:  "ls",
				},
				{
					kind: SingleQuote,
					val:  "'",
				},
				{
					kind: Field,
					val:  "dir1\\",
				},
				{
					kind: SingleQuote,
					val:  "'",
				},
				{
					kind: Control,
					val:  ";",
				},
				{
					kind: Executable,
					val:  "cat",
				},
				{
					kind: SingleQuote,
					val:  "'",
				},
				{
					kind: Field,
					val:  "dir2",
				},
				{
					kind: SingleQuote,
					val:  "'",
				},
			},
		},
		{
			command: "ls /etc; ls",
			tokens: []ShellToken{
				{
					kind: Executable,
					val:  "ls",
				},
				{
					kind: Field,
					val:  "/etc",
				},
				{
					kind: Control,
					val:  ";",
				},
				{
					kind: Executable,
					val:  "ls",
				},
			},
		},
	}

	for _, test := range tests {
		assert.True(t, compareGeneratedTokens(test))
	}
}

func TestParseMultiCommandWithStringArg(t *testing.T) {
	tests := []testShellCommand{
		{
			command: "ls \"dir1\\\"; cat \\\"di\\\"r2\"",
			tokens: []ShellToken{
				{
					kind: Executable,
					val:  "ls",
				},
				{
					kind: DoubleQuote,
					val:  "\"",
				},
				{
					kind: Field,
					val:  "dir1\\\"; cat \\\"di\\\"r2",
				},
				{
					kind: DoubleQuote,
					val:  "\"",
				},
			},
		},
	}

	for _, test := range tests {
		assert.True(t, compareGeneratedTokens(test))
	}
}

func TestParseBasicCommandWithEnv(t *testing.T) {
	tests := []testShellCommand{
		{
			command: "TEST=1 FOO=\"bar\" ls dir1 dir2",
			tokens: []ShellToken{
				{
					kind: VariableDefinition,
					val:  "TEST",
				},
				{
					kind: Equal,
					val:  "=",
				},
				{
					kind: Field,
					val:  "1",
				},
				{
					kind: VariableDefinition,
					val:  "FOO",
				},
				{
					kind: Equal,
					val:  "=",
				},
				{
					kind: DoubleQuote,
					val:  "\"",
				},
				{
					kind: Field,
					val:  "bar",
				},
				{
					kind: DoubleQuote,
					val:  "\"",
				},
				{
					kind: Executable,
					val:  "ls",
				},
				{
					kind: Field,
					val:  "dir1",
				},
				{
					kind: Field,
					val:  "dir2",
				},
			},
		},
	}

	for _, test := range tests {
		assert.True(t, compareGeneratedTokens(test))
	}
}

func TestParseCommandUsingVariables(t *testing.T) {
	tests := []testShellCommand{
		{
			command: "ls $TOTO",
			tokens: []ShellToken{
				{
					kind: Executable,
					val:  "ls",
				},
				{
					kind: Dollar,
					val:  "$",
				},
				{
					kind: ShellVariable,
					val:  "TOTO",
				},
			},
		},
		{
			command: "TOTO=ls $TOTO",
			tokens: []ShellToken{
				{
					kind: VariableDefinition,
					val:  "TOTO",
				},
				{
					kind: Equal,
					val:  "=",
				},
				{
					kind: Field,
					val:  "ls",
				},
				{
					kind: Dollar,
					val:  "$",
				},
				{
					kind: ShellVariable,
					val:  "TOTO",
				},
			},
		},
		{
			command: "ping google.com; TOTO=ls $TOTO",
			tokens: []ShellToken{
				{
					kind: Executable,
					val:  "ping",
				},
				{
					kind: Field,
					val:  "google.com",
				},
				{
					kind: Control,
					val:  ";",
				},
				{
					kind: VariableDefinition,
					val:  "TOTO",
				},
				{
					kind: Equal,
					val:  "=",
				},
				{
					kind: Field,
					val:  "ls",
				},
				{
					kind: Dollar,
					val:  "$",
				},
				{
					kind: ShellVariable,
					val:  "TOTO",
				},
			},
		},
	}

	for _, test := range tests {
		assert.True(t, compareGeneratedTokens(test))
	}
}

func TestParseBasicCommandWithWhitespaceEnv(t *testing.T) {
	tests := []testShellCommand{
		{
			command: "TEST=1 FOO=\"item bar\" ls dir1 dir2",
			tokens: []ShellToken{
				{
					kind: VariableDefinition,
					val:  "TEST",
				},
				{
					kind: Equal,
					val:  "=",
				},
				{
					kind: Field,
					val:  "1",
				},
				{
					kind: VariableDefinition,
					val:  "FOO",
				},
				{
					kind: Equal,
					val:  "=",
				},
				{
					kind: DoubleQuote,
					val:  "\"",
				},
				{
					kind: Field,
					val:  "item bar",
				},
				{
					kind: DoubleQuote,
					val:  "\"",
				},
				{
					kind: Executable,
					val:  "ls",
				},
				{
					kind: Field,
					val:  "dir1",
				},
				{
					kind: Field,
					val:  "dir2",
				},
			},
		},
		{
			command: "TEST=1 RAILS = production",
			tokens: []ShellToken{
				{
					kind: VariableDefinition,
					val:  "TEST",
				},
				{
					kind: Equal,
					val:  "=",
				},
				{
					kind: Field,
					val:  "1",
				},
				{
					kind: Executable,
					val:  "RAILS",
				},
				{
					kind: Equal,
					val:  "=",
				},
				{
					kind: Field,
					val:  "production",
				},
			},
		},
	}

	for _, test := range tests {
		assert.True(t, compareGeneratedTokens(test))
	}
}

func TestParseMultipleCommands(t *testing.T) {
	tests := []testShellCommand{
		{
			command: "ls;echo",
			tokens: []ShellToken{
				{
					kind: Executable,
					val:  "ls",
				},
				{
					kind: Control,
					val:  ";",
				},
				{
					kind: Executable,
					val:  "echo",
				},
			},
		},
	}

	for _, test := range tests {
		assert.True(t, compareGeneratedTokens(test))
	}
}

func TestParseDoubleQuotedExecutable(t *testing.T) {
	tests := []testShellCommand{
		{
			command: "ls;\"echo\"",
			tokens: []ShellToken{
				{
					kind: Executable,
					val:  "ls",
				},
				{
					kind: Control,
					val:  ";",
				},
				{
					kind: DoubleQuote,
					val:  "\"",
				},
				{
					kind: Executable,
					val:  "echo",
				},
				{
					kind: DoubleQuote,
					val:  "\"",
				},
			},
		},
		{
			command: "ls;\"echo toto\"",
			tokens: []ShellToken{
				{
					kind: Executable,
					val:  "ls",
				},
				{
					kind: Control,
					val:  ";",
				},
				{
					kind: DoubleQuote,
					val:  "\"",
				},
				{
					kind: Executable,
					val:  "echo toto",
				},
				{
					kind: DoubleQuote,
					val:  "\"",
				},
			},
		},
		{
			command: "ping 127.0.0.1; a=eval \"ls\"",
			tokens: []ShellToken{
				{
					kind: Executable,
					val:  "ping",
				},
				{
					kind: Field,
					val:  "127.0.0.1",
				},
				{
					kind: Control,
					val:  ";",
				},
				{
					kind: VariableDefinition,
					val:  "a",
				},
				{
					kind: Equal,
					val:  "=",
				},
				{
					kind: Field,
					val:  "eval",
				},
				{
					kind: DoubleQuote,
					val:  "\"",
				},
				{
					kind: Executable,
					val:  "ls",
				},
				{
					kind: DoubleQuote,
					val:  "\"",
				},
			},
		},
	}

	for _, test := range tests {
		assert.True(t, compareGeneratedTokens(test))
	}
}

func TestParseVariableSubstitution(t *testing.T) {
	tests := []testShellCommand{
		{
			command: "ping -c 1 google.com; ${a:-ls}",
			tokens: []ShellToken{
				{
					kind: Executable,
					val:  "ping",
				},
				{
					kind: Field,
					val:  "-c",
				},
				{
					kind: Field,
					val:  "1",
				},
				{
					kind: Field,
					val:  "google.com",
				},
				{
					kind: Control,
					val:  ";",
				},
				{
					kind: Dollar,
					val:  "$",
				},
				{
					kind: Executable,
					val:  "{a:-ls}",
				},
			},
		},
	}

	for _, test := range tests {
		assert.True(t, compareGeneratedTokens(test))
	}
}

func TestParseBasicRedirection(t *testing.T) {
	tests := []testShellCommand{
		{
			command: "ls > /tmp/test args",
			tokens: []ShellToken{
				{
					kind: Executable,
					val:  "ls",
				},
				{
					kind: Redirection,
					val:  ">",
				},
				{
					kind: Field,
					val:  "/tmp/test",
				},
				{
					kind: Field,
					val:  "args",
				},
			},
		},
		{
			command: "ls args > /tmp/test",
			tokens: []ShellToken{
				{
					kind: Executable,
					val:  "ls",
				},
				{
					kind: Field,
					val:  "args",
				},
				{
					kind: Redirection,
					val:  ">",
				},
				{
					kind: Field,
					val:  "/tmp/test",
				},
			},
		},
		{
			command: "ls >                            /tmp/test args ",
			tokens: []ShellToken{
				{
					kind: Executable,
					val:  "ls",
				},
				{
					kind: Redirection,
					val:  ">",
				},
				{
					kind: Field,
					val:  "/tmp/test",
				},
				{
					kind: Field,
					val:  "args",
				},
			},
		},
		{
			command: "ls args 2> /tmp/test",
			tokens: []ShellToken{
				{
					kind: Executable,
					val:  "ls",
				},
				{
					kind: Field,
					val:  "args",
				},
				{
					kind: Redirection,
					val:  "2>",
				},
				{
					kind: Field,
					val:  "/tmp/test",
				},
			},
		},
		{
			command: "ls args >> /tmp/test",
			tokens: []ShellToken{
				{
					kind: Executable,
					val:  "ls",
				},
				{
					kind: Field,
					val:  "args",
				},
				{
					kind: Redirection,
					val:  ">>",
				},
				{
					kind: Field,
					val:  "/tmp/test",
				},
			},
		},
		{
			command: "ls args > /tmp/test 2> /etc/stderr",
			tokens: []ShellToken{
				{
					kind: Executable,
					val:  "ls",
				},
				{
					kind: Field,
					val:  "args",
				},
				{
					kind: Redirection,
					val:  ">",
				},
				{
					kind: Field,
					val:  "/tmp/test",
				},
				{
					kind: Redirection,
					val:  "2>",
				},
				{
					kind: Field,
					val:  "/etc/stderr",
				},
			},
		},
		{
			command: "ls args >(/tmp/test)",
			tokens: []ShellToken{
				{
					kind: Executable,
					val:  "ls",
				},
				{
					kind: Field,
					val:  "args",
				},
				{
					kind: Redirection,
					val:  ">",
				},
				{
					kind: ParentheseOpen,
					val:  "(",
				},
				{
					kind: Field,
					val:  "/tmp/test",
				},
				{
					kind: ParentheseClose,
					val:  ")",
				},
			},
		},
		{
			command: "ls args >/tmp/test",
			tokens: []ShellToken{
				{
					kind: Executable,
					val:  "ls",
				},
				{
					kind: Field,
					val:  "args",
				},
				{
					kind: Redirection,
					val:  ">",
				},
				{
					kind: Field,
					val:  "/tmp/test",
				},
			},
		},
	}

	for _, test := range tests {
		assert.True(t, compareGeneratedTokens(test))
	}
}

func TestParseAllPossibleRedirections(t *testing.T) {
	redirections := []string{
		">", "<", ">>", "<<", "<&", ">&", "<<-", "&>", "<>", ">|", "<&1", ">&1", "0>", "0>>", "0<", "0<<", "0<<<",
		"0<&", "0<&-", "0>&", "0>&1", "0>&-", "0>|", "0<>",
	}

	test := testShellCommand{
		command: "ls args > file",
		tokens: []ShellToken{
			{
				kind: Executable,
				val:  "ls",
			},
			{
				kind: Field,
				val:  "args",
			},
			{
				kind: Redirection,
				val:  ">",
			},
			{
				kind: Field,
				val:  "file",
			},
		},
	}

	for _, redirection := range redirections {
		test.command = "ls args " + redirection + " file"
		test.tokens[2].val = redirection
		assert.True(t, compareGeneratedTokens(test))
	}
}

func TestParseAllTrickyRedirections(t *testing.T) {
	redirections := []string{
		"<-", "0&>", "0>&-1", "0>&1-", "0<&-1", "0<&1-", ">1",
		"<1", ">>1", "<<1", "<<<1", "<<-1", "&>1", "<>1", ">|1",
	}

	for _, redirection := range redirections {
		cmd := "ls args " + redirection + " file"
		tokens := parseShell(cmd)

		assert.True(t, len(tokens) > 4)
		assert.True(t, tokens[2].val != redirection)
	}
}

func TestParseBasicPrependRedirection(t *testing.T) {
	tests := []testShellCommand{
		{
			command: "> /tmp/test ls args",
			tokens: []ShellToken{
				{
					kind: Redirection,
					val:  ">",
				},
				{
					kind: Field,
					val:  "/tmp/test",
				},
				{
					kind: Executable,
					val:  "ls",
				},
				{
					kind: Field,
					val:  "args",
				},
			},
		},
	}

	for _, test := range tests {
		assert.True(t, compareGeneratedTokens(test))
	}
}

func TestParseMultipleCommandsPipeline(t *testing.T) {
	tests := []testShellCommand{
		{
			command: "true && echo \"hello\" | tr h ' ' | tr e a",
			tokens: []ShellToken{
				{
					kind: Executable,
					val:  "true",
				},
				{
					kind: Control,
					val:  "&&",
				},
				{
					kind: Executable,
					val:  "echo",
				},
				{
					kind: DoubleQuote,
					val:  "\"",
				},
				{
					kind: Field,
					val:  "hello",
				},
				{
					kind: DoubleQuote,
					val:  "\"",
				},
				{
					kind: Control,
					val:  "|",
				},
				{
					kind: Executable,
					val:  "tr",
				},
				{
					kind: Field,
					val:  "h",
				},
				{
					kind: SingleQuote,
					val:  "'",
				},
				{
					kind: Field,
					val:  " ",
				},
				{
					kind: SingleQuote,
					val:  "'",
				},
				{
					kind: Control,
					val:  "|",
				},
				{
					kind: Executable,
					val:  "tr",
				},
				{
					kind: Field,
					val:  "e",
				},
				{
					kind: Field,
					val:  "a",
				},
			},
		},
	}

	for _, test := range tests {
		assert.True(t, compareGeneratedTokens(test))
	}
}

func TestParseCommandWithNewLines(t *testing.T) {
	tests := []testShellCommand{
		{
			command: "cmd hello\ncat /etc/passwd",
			tokens: []ShellToken{
				{
					kind: Executable,
					val:  "cmd",
				},
				{
					kind: Field,
					val:  "hello",
				},
				{
					kind: Control,
					val:  "\n",
				},
				{
					kind: Executable,
					val:  "cat",
				},
				{
					kind: Field,
					val:  "/etc/passwd",
				},
			},
		},
	}

	for _, test := range tests {
		assert.True(t, compareGeneratedTokens(test))
	}
}
