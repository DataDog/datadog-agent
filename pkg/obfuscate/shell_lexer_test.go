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
	generatedTokens := ParseShell(t.command)
	expectedTokens := t.tokens

	if len(generatedTokens) != len(expectedTokens) {
		return false
	}

	for i, token := range generatedTokens {
		if token.Kind != expectedTokens[i].Kind || token.Val != expectedTokens[i].Val {
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
					Kind: Executable,
					Val:  "echo",
				},
			},
		},
		{
			command: "echo   ",
			tokens: []ShellToken{
				{
					Kind: Executable,
					Val:  "echo",
				},
			},
		},
		{
			command: "   test echo",
			tokens: []ShellToken{
				{
					Kind: Executable,
					Val:  "test",
				},
				{
					Kind: Field,
					Val:  "echo",
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
					Kind: Executable,
					Val:  "ls",
				},
				{
					Kind: Field,
					Val:  "dir1",
				},
				{
					Kind: Field,
					Val:  "dir2",
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
					Kind: Executable,
					Val:  "ls",
				},
				{
					Kind: DoubleQuote,
					Val:  "\"",
				},
				{
					Kind: Field,
					Val:  "dir1 dir2",
				},
				{
					Kind: DoubleQuote,
					Val:  "\"",
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
					Kind: Executable,
					Val:  "ls",
				},
				{
					Kind: SingleQuote,
					Val:  "'",
				},
				{
					Kind: Field,
					Val:  "dir1 dir2",
				},
				{
					Kind: SingleQuote,
					Val:  "'",
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
					Kind: Executable,
					Val:  "ls",
				},
				{
					Kind: Backticks,
					Val:  "`",
				},
				{
					Kind: Executable,
					Val:  "which",
				},
				{
					Kind: Field,
					Val:  "w",
				},
				{
					Kind: Backticks,
					Val:  "`",
				},
				{
					Kind: Field,
					Val:  "-li",
				},
			},
		},
		{
			command: "ls `TOTO=plop which w` -li",
			tokens: []ShellToken{
				{
					Kind: Executable,
					Val:  "ls",
				},
				{
					Kind: Backticks,
					Val:  "`",
				},
				{
					Kind: VariableDefinition,
					Val:  "TOTO",
				},
				{
					Kind: Equal,
					Val:  "=",
				},
				{
					Kind: Field,
					Val:  "plop",
				},
				{
					Kind: Executable,
					Val:  "which",
				},
				{
					Kind: Field,
					Val:  "w",
				},
				{
					Kind: Backticks,
					Val:  "`",
				},
				{
					Kind: Field,
					Val:  "-li",
				},
			},
		},
		{
			command: "`echo echo` \"plop\"",
			tokens: []ShellToken{
				{
					Kind: Backticks,
					Val:  "`",
				},
				{
					Kind: Executable,
					Val:  "echo",
				},
				{
					Kind: Field,
					Val:  "echo",
				},
				{
					Kind: Backticks,
					Val:  "`",
				},
				{
					Kind: DoubleQuote,
					Val:  "\"",
				},
				{
					Kind: Field,
					Val:  "plop",
				},
				{
					Kind: DoubleQuote,
					Val:  "\"",
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
					Kind: Executable,
					Val:  "ls",
				},
				{
					Kind: Dollar,
					Val:  "$",
				},
				{
					Kind: ParentheseOpen,
					Val:  "(",
				},
				{
					Kind: Executable,
					Val:  "which",
				},
				{
					Kind: Field,
					Val:  "w",
				},
				{
					Kind: ParentheseClose,
					Val:  ")",
				},
				{
					Kind: Field,
					Val:  "-li",
				},
			},
		},
		{
			command: "ls $(TOTO=plop which w) -li",
			tokens: []ShellToken{
				{
					Kind: Executable,
					Val:  "ls",
				},
				{
					Kind: Dollar,
					Val:  "$",
				},
				{
					Kind: ParentheseOpen,
					Val:  "(",
				},
				{
					Kind: VariableDefinition,
					Val:  "TOTO",
				},
				{
					Kind: Equal,
					Val:  "=",
				},
				{
					Kind: Field,
					Val:  "plop",
				},
				{
					Kind: Executable,
					Val:  "which",
				},
				{
					Kind: Field,
					Val:  "w",
				},
				{
					Kind: ParentheseClose,
					Val:  ")",
				},
				{
					Kind: Field,
					Val:  "-li",
				},
			},
		},
		{
			command: "$(echo echo) \"plop\"",
			tokens: []ShellToken{
				{
					Kind: Dollar,
					Val:  "$",
				},
				{
					Kind: ParentheseOpen,
					Val:  "(",
				},
				{
					Kind: Executable,
					Val:  "echo",
				},
				{
					Kind: Field,
					Val:  "echo",
				},
				{
					Kind: ParentheseClose,
					Val:  ")",
				},
				{
					Kind: DoubleQuote,
					Val:  "\"",
				},
				{
					Kind: Field,
					Val:  "plop",
				},
				{
					Kind: DoubleQuote,
					Val:  "\"",
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
					Kind: Executable,
					Val:  "ping",
				},
				{
					Kind: Field,
					Val:  "-c",
				},
				{
					Kind: Field,
					Val:  "2",
				},
				{
					Kind: Field,
					Val:  "127.0.0.1",
				},
				{
					Kind: DoubleQuote,
					Val:  "\"",
				},
				{
					Kind: Dollar,
					Val:  "$",
				},
				{
					Kind: ParentheseOpen,
					Val:  "(",
				},
				{
					Kind: Executable,
					Val:  "cat",
				},
				{
					Kind: Field,
					Val:  "/etc/passwd",
				},
				{
					Kind: ParentheseClose,
					Val:  ")",
				},
				{
					Kind: DoubleQuote,
					Val:  "\"",
				},
			},
		},
		{
			command: "ping -c 2 127.0.0.1 \"some text: $(cat /etc/passwd) and other text\"",
			tokens: []ShellToken{
				{
					Kind: Executable,
					Val:  "ping",
				},
				{
					Kind: Field,
					Val:  "-c",
				},
				{
					Kind: Field,
					Val:  "2",
				},
				{
					Kind: Field,
					Val:  "127.0.0.1",
				},
				{
					Kind: DoubleQuote,
					Val:  "\"",
				},
				{
					Kind: Field,
					Val:  "some text: ",
				},
				{
					Kind: Dollar,
					Val:  "$",
				},
				{
					Kind: ParentheseOpen,
					Val:  "(",
				},
				{
					Kind: Executable,
					Val:  "cat",
				},
				{
					Kind: Field,
					Val:  "/etc/passwd",
				},
				{
					Kind: ParentheseClose,
					Val:  ")",
				},
				{
					Kind: Field,
					Val:  " and other text",
				},
				{
					Kind: DoubleQuote,
					Val:  "\"",
				},
			},
		},
		{
			command: "echo \"$(echo hello) $(echo world)\"",
			tokens: []ShellToken{
				{
					Kind: Executable,
					Val:  "echo",
				},
				{
					Kind: DoubleQuote,
					Val:  "\"",
				},
				{
					Kind: Dollar,
					Val:  "$",
				},
				{
					Kind: ParentheseOpen,
					Val:  "(",
				},
				{
					Kind: Executable,
					Val:  "echo",
				},
				{
					Kind: Field,
					Val:  "hello",
				},
				{
					Kind: ParentheseClose,
					Val:  ")",
				},
				{
					Kind: Field,
					Val:  " ",
				},
				{
					Kind: Dollar,
					Val:  "$",
				},
				{
					Kind: ParentheseOpen,
					Val:  "(",
				},
				{
					Kind: Executable,
					Val:  "echo",
				},
				{
					Kind: Field,
					Val:  "world",
				},
				{
					Kind: ParentheseClose,
					Val:  ")",
				},
				{
					Kind: DoubleQuote,
					Val:  "\"",
				},
			},
		},
		{
			command: "echo \"[DEBUG] $(echo hello) - $(echo $(echo $(echo world 1 2)))\" '$(echo nope)' $(echo plop) $(echo \"plop\")",
			tokens: []ShellToken{
				{
					Kind: Executable,
					Val:  "echo",
				},
				{
					Kind: DoubleQuote,
					Val:  "\"",
				},
				{
					Kind: Field,
					Val:  "[DEBUG] ",
				},
				{
					Kind: Dollar,
					Val:  "$",
				},
				{
					Kind: ParentheseOpen,
					Val:  "(",
				},
				{
					Kind: Executable,
					Val:  "echo",
				},
				{
					Kind: Field,
					Val:  "hello",
				},
				{
					Kind: ParentheseClose,
					Val:  ")",
				},
				{
					Kind: Field,
					Val:  " - ",
				},
				{
					Kind: Dollar,
					Val:  "$",
				},
				{
					Kind: ParentheseOpen,
					Val:  "(",
				},
				{
					Kind: Executable,
					Val:  "echo",
				},
				{
					Kind: Dollar,
					Val:  "$",
				},
				{
					Kind: ParentheseOpen,
					Val:  "(",
				},
				{
					Kind: Executable,
					Val:  "echo",
				},
				{
					Kind: Dollar,
					Val:  "$",
				},
				{
					Kind: ParentheseOpen,
					Val:  "(",
				},
				{
					Kind: Executable,
					Val:  "echo",
				},
				{
					Kind: Field,
					Val:  "world",
				},
				{
					Kind: Field,
					Val:  "1",
				},
				{
					Kind: Field,
					Val:  "2",
				},
				{
					Kind: ParentheseClose,
					Val:  ")",
				},
				{
					Kind: ParentheseClose,
					Val:  ")",
				},
				{
					Kind: ParentheseClose,
					Val:  ")",
				},
				{
					Kind: DoubleQuote,
					Val:  "\"",
				},
				{
					Kind: SingleQuote,
					Val:  "'",
				},
				{
					Kind: Field,
					Val:  "$(echo nope)",
				},
				{
					Kind: SingleQuote,
					Val:  "'",
				},
				{
					Kind: Dollar,
					Val:  "$",
				},
				{
					Kind: ParentheseOpen,
					Val:  "(",
				},
				{
					Kind: Executable,
					Val:  "echo",
				},
				{
					Kind: Field,
					Val:  "plop",
				},
				{
					Kind: ParentheseClose,
					Val:  ")",
				},
				{
					Kind: Dollar,
					Val:  "$",
				},
				{
					Kind: ParentheseOpen,
					Val:  "(",
				},
				{
					Kind: Executable,
					Val:  "echo",
				},
				{
					Kind: DoubleQuote,
					Val:  "\"",
				},
				{
					Kind: Field,
					Val:  "plop",
				},
				{
					Kind: DoubleQuote,
					Val:  "\"",
				},
				{
					Kind: ParentheseClose,
					Val:  ")",
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
					Kind: Executable,
					Val:  "echo",
				},
				{
					Kind: DoubleQuote,
					Val:  "\"",
				},
				{
					Kind: Dollar,
					Val:  "$",
				},
				{
					Kind: ParentheseOpen,
					Val:  "(",
				},
				{
					Kind: Executable,
					Val:  "echo",
				},
				{
					Kind: DoubleQuote,
					Val:  "\"",
				},
				{
					Kind: Field,
					Val:  "test",
				},
				{
					Kind: DoubleQuote,
					Val:  "\"",
				},
				{
					Kind: ParentheseClose,
					Val:  ")",
				},
				{
					Kind: DoubleQuote,
					Val:  "\"",
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
					Kind: Executable,
					Val:  "echo",
				},
				{
					Kind: Backticks,
					Val:  "`",
				},
				{
					Kind: Executable,
					Val:  "echo",
				},
				{
					Kind: Dollar,
					Val:  "$",
				},
				{
					Kind: ParentheseOpen,
					Val:  "(",
				},
				{
					Kind: Executable,
					Val:  "echo",
				},
				{
					Kind: Field,
					Val:  "hello",
				},
				{
					Kind: ParentheseClose,
					Val:  ")",
				},
				{
					Kind: Backticks,
					Val:  "`",
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
					Kind: Dollar,
					Val:  "$",
				},
				{
					Kind: ParentheseOpen,
					Val:  "(",
				},
				{
					Kind: Executable,
					Val:  "echo",
				},
				{
					Kind: Backticks,
					Val:  "`",
				},
				{
					Kind: Executable,
					Val:  "echo",
				},
				{
					Kind: Field,
					Val:  "echo",
				},
				{
					Kind: Backticks,
					Val:  "`",
				},
				{
					Kind: ParentheseClose,
					Val:  ")",
				},
				{
					Kind: DoubleQuote,
					Val:  "\"",
				},
				{
					Kind: Field,
					Val:  "toto",
				},
				{
					Kind: DoubleQuote,
					Val:  "\"",
				},
			},
		},
		{
			command: "`echo $(echo echo)` \"toto\"",
			tokens: []ShellToken{
				{
					Kind: Backticks,
					Val:  "`",
				},
				{
					Kind: Executable,
					Val:  "echo",
				},
				{
					Kind: Dollar,
					Val:  "$",
				},
				{
					Kind: ParentheseOpen,
					Val:  "(",
				},
				{
					Kind: Executable,
					Val:  "echo",
				},
				{
					Kind: Field,
					Val:  "echo",
				},
				{
					Kind: ParentheseClose,
					Val:  ")",
				},
				{
					Kind: Backticks,
					Val:  "`",
				},
				{
					Kind: DoubleQuote,
					Val:  "\"",
				},
				{
					Kind: Field,
					Val:  "toto",
				},
				{
					Kind: DoubleQuote,
					Val:  "\"",
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
Backticks that are escaped will be resolved are normal backticks, resulting of the End of the current backtick expression or the Start.
Executable tokens are not set as executable if they are in a backtick expression.
We can't know the Start and the End of a backtick expression if it's escaped.

func TestParseBasicCommandsWithNestedBackticks(t *testing.T) {
	tests := []testShellCommand{
		{
			command: "echo `echo `echo echo``",
			tokens: []ShellToken{
				{
					Kind: Executable,
					Val:  "echo",
				},
				{
					Kind: Backticks,
					Val:  "`",
				},
				{
					Kind: Executable,
					Val:  "echo",
				},
				{
					Kind: Backticks,
					Val:  "`",
				},
				{
					Kind: Executable,
					Val:  "echo",
				},
				{
					Kind: Field,
					Val:  "echo",
				},
				{
					Kind: Backticks,
					Val:  "`",
				},
				{
					Kind: Backticks,
					Val:  "`",
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
					Kind: Executable,
					Val:  "ls",
				},
				{
					Kind: DoubleQuote,
					Val:  "\"",
				},
				{
					Kind: Field,
					Val:  "dir1 dir2",
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
					Kind: Executable,
					Val:  "ls",
				},
				{
					Kind: SingleQuote,
					Val:  "'",
				},
				{
					Kind: Field,
					Val:  "dir1 dir2",
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
					Kind: Executable,
					Val:  "ls",
				},
				{
					Kind: SingleQuote,
					Val:  "'",
				},
				{
					Kind: Field,
					Val:  "dir1\\",
				},
				{
					Kind: SingleQuote,
					Val:  "'",
				},
				{
					Kind: Control,
					Val:  ";",
				},
				{
					Kind: Executable,
					Val:  "cat",
				},
				{
					Kind: SingleQuote,
					Val:  "'",
				},
				{
					Kind: Field,
					Val:  "dir2",
				},
				{
					Kind: SingleQuote,
					Val:  "'",
				},
			},
		},
		{
			command: "ls /etc; ls",
			tokens: []ShellToken{
				{
					Kind: Executable,
					Val:  "ls",
				},
				{
					Kind: Field,
					Val:  "/etc",
				},
				{
					Kind: Control,
					Val:  ";",
				},
				{
					Kind: Executable,
					Val:  "ls",
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
					Kind: Executable,
					Val:  "ls",
				},
				{
					Kind: DoubleQuote,
					Val:  "\"",
				},
				{
					Kind: Field,
					Val:  "dir1\\\"; cat \\\"di\\\"r2",
				},
				{
					Kind: DoubleQuote,
					Val:  "\"",
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
					Kind: VariableDefinition,
					Val:  "TEST",
				},
				{
					Kind: Equal,
					Val:  "=",
				},
				{
					Kind: Field,
					Val:  "1",
				},
				{
					Kind: VariableDefinition,
					Val:  "FOO",
				},
				{
					Kind: Equal,
					Val:  "=",
				},
				{
					Kind: DoubleQuote,
					Val:  "\"",
				},
				{
					Kind: Field,
					Val:  "bar",
				},
				{
					Kind: DoubleQuote,
					Val:  "\"",
				},
				{
					Kind: Executable,
					Val:  "ls",
				},
				{
					Kind: Field,
					Val:  "dir1",
				},
				{
					Kind: Field,
					Val:  "dir2",
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
					Kind: Executable,
					Val:  "ls",
				},
				{
					Kind: Dollar,
					Val:  "$",
				},
				{
					Kind: ShellVariable,
					Val:  "TOTO",
				},
			},
		},
		{
			command: "TOTO=ls $TOTO",
			tokens: []ShellToken{
				{
					Kind: VariableDefinition,
					Val:  "TOTO",
				},
				{
					Kind: Equal,
					Val:  "=",
				},
				{
					Kind: Field,
					Val:  "ls",
				},
				{
					Kind: Dollar,
					Val:  "$",
				},
				{
					Kind: ShellVariable,
					Val:  "TOTO",
				},
			},
		},
		{
			command: "ping google.com; TOTO=ls $TOTO",
			tokens: []ShellToken{
				{
					Kind: Executable,
					Val:  "ping",
				},
				{
					Kind: Field,
					Val:  "google.com",
				},
				{
					Kind: Control,
					Val:  ";",
				},
				{
					Kind: VariableDefinition,
					Val:  "TOTO",
				},
				{
					Kind: Equal,
					Val:  "=",
				},
				{
					Kind: Field,
					Val:  "ls",
				},
				{
					Kind: Dollar,
					Val:  "$",
				},
				{
					Kind: ShellVariable,
					Val:  "TOTO",
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
					Kind: VariableDefinition,
					Val:  "TEST",
				},
				{
					Kind: Equal,
					Val:  "=",
				},
				{
					Kind: Field,
					Val:  "1",
				},
				{
					Kind: VariableDefinition,
					Val:  "FOO",
				},
				{
					Kind: Equal,
					Val:  "=",
				},
				{
					Kind: DoubleQuote,
					Val:  "\"",
				},
				{
					Kind: Field,
					Val:  "item bar",
				},
				{
					Kind: DoubleQuote,
					Val:  "\"",
				},
				{
					Kind: Executable,
					Val:  "ls",
				},
				{
					Kind: Field,
					Val:  "dir1",
				},
				{
					Kind: Field,
					Val:  "dir2",
				},
			},
		},
		{
			command: "TEST=1 RAILS = production",
			tokens: []ShellToken{
				{
					Kind: VariableDefinition,
					Val:  "TEST",
				},
				{
					Kind: Equal,
					Val:  "=",
				},
				{
					Kind: Field,
					Val:  "1",
				},
				{
					Kind: Executable,
					Val:  "RAILS",
				},
				{
					Kind: Equal,
					Val:  "=",
				},
				{
					Kind: Field,
					Val:  "production",
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
					Kind: Executable,
					Val:  "ls",
				},
				{
					Kind: Control,
					Val:  ";",
				},
				{
					Kind: Executable,
					Val:  "echo",
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
					Kind: Executable,
					Val:  "ls",
				},
				{
					Kind: Control,
					Val:  ";",
				},
				{
					Kind: DoubleQuote,
					Val:  "\"",
				},
				{
					Kind: Executable,
					Val:  "echo",
				},
				{
					Kind: DoubleQuote,
					Val:  "\"",
				},
			},
		},
		{
			command: "ls;\"echo toto\"",
			tokens: []ShellToken{
				{
					Kind: Executable,
					Val:  "ls",
				},
				{
					Kind: Control,
					Val:  ";",
				},
				{
					Kind: DoubleQuote,
					Val:  "\"",
				},
				{
					Kind: Executable,
					Val:  "echo toto",
				},
				{
					Kind: DoubleQuote,
					Val:  "\"",
				},
			},
		},
		{
			command: "ping 127.0.0.1; a=eval \"ls\"",
			tokens: []ShellToken{
				{
					Kind: Executable,
					Val:  "ping",
				},
				{
					Kind: Field,
					Val:  "127.0.0.1",
				},
				{
					Kind: Control,
					Val:  ";",
				},
				{
					Kind: VariableDefinition,
					Val:  "a",
				},
				{
					Kind: Equal,
					Val:  "=",
				},
				{
					Kind: Field,
					Val:  "eval",
				},
				{
					Kind: DoubleQuote,
					Val:  "\"",
				},
				{
					Kind: Executable,
					Val:  "ls",
				},
				{
					Kind: DoubleQuote,
					Val:  "\"",
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
					Kind: Executable,
					Val:  "ping",
				},
				{
					Kind: Field,
					Val:  "-c",
				},
				{
					Kind: Field,
					Val:  "1",
				},
				{
					Kind: Field,
					Val:  "google.com",
				},
				{
					Kind: Control,
					Val:  ";",
				},
				{
					Kind: Dollar,
					Val:  "$",
				},
				{
					Kind: Executable,
					Val:  "{a:-ls}",
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
					Kind: Executable,
					Val:  "ls",
				},
				{
					Kind: Redirection,
					Val:  ">",
				},
				{
					Kind: Field,
					Val:  "/tmp/test",
				},
				{
					Kind: Field,
					Val:  "args",
				},
			},
		},
		{
			command: "ls args > /tmp/test",
			tokens: []ShellToken{
				{
					Kind: Executable,
					Val:  "ls",
				},
				{
					Kind: Field,
					Val:  "args",
				},
				{
					Kind: Redirection,
					Val:  ">",
				},
				{
					Kind: Field,
					Val:  "/tmp/test",
				},
			},
		},
		{
			command: "ls >                            /tmp/test args ",
			tokens: []ShellToken{
				{
					Kind: Executable,
					Val:  "ls",
				},
				{
					Kind: Redirection,
					Val:  ">",
				},
				{
					Kind: Field,
					Val:  "/tmp/test",
				},
				{
					Kind: Field,
					Val:  "args",
				},
			},
		},
		{
			command: "ls args 2> /tmp/test",
			tokens: []ShellToken{
				{
					Kind: Executable,
					Val:  "ls",
				},
				{
					Kind: Field,
					Val:  "args",
				},
				{
					Kind: Redirection,
					Val:  "2>",
				},
				{
					Kind: Field,
					Val:  "/tmp/test",
				},
			},
		},
		{
			command: "ls args >> /tmp/test",
			tokens: []ShellToken{
				{
					Kind: Executable,
					Val:  "ls",
				},
				{
					Kind: Field,
					Val:  "args",
				},
				{
					Kind: Redirection,
					Val:  ">>",
				},
				{
					Kind: Field,
					Val:  "/tmp/test",
				},
			},
		},
		{
			command: "ls args > /tmp/test 2> /etc/stderr",
			tokens: []ShellToken{
				{
					Kind: Executable,
					Val:  "ls",
				},
				{
					Kind: Field,
					Val:  "args",
				},
				{
					Kind: Redirection,
					Val:  ">",
				},
				{
					Kind: Field,
					Val:  "/tmp/test",
				},
				{
					Kind: Redirection,
					Val:  "2>",
				},
				{
					Kind: Field,
					Val:  "/etc/stderr",
				},
			},
		},
		{
			command: "ls args >(/tmp/test)",
			tokens: []ShellToken{
				{
					Kind: Executable,
					Val:  "ls",
				},
				{
					Kind: Field,
					Val:  "args",
				},
				{
					Kind: Redirection,
					Val:  ">",
				},
				{
					Kind: ParentheseOpen,
					Val:  "(",
				},
				{
					Kind: Field,
					Val:  "/tmp/test",
				},
				{
					Kind: ParentheseClose,
					Val:  ")",
				},
			},
		},
		{
			command: "ls args >/tmp/test",
			tokens: []ShellToken{
				{
					Kind: Executable,
					Val:  "ls",
				},
				{
					Kind: Field,
					Val:  "args",
				},
				{
					Kind: Redirection,
					Val:  ">",
				},
				{
					Kind: Field,
					Val:  "/tmp/test",
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
				Kind: Executable,
				Val:  "ls",
			},
			{
				Kind: Field,
				Val:  "args",
			},
			{
				Kind: Redirection,
				Val:  ">",
			},
			{
				Kind: Field,
				Val:  "file",
			},
		},
	}

	for _, redirection := range redirections {
		test.command = "ls args " + redirection + " file"
		test.tokens[2].Val = redirection
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
		tokens := ParseShell(cmd)

		assert.True(t, len(tokens) > 4)
		assert.True(t, tokens[2].Val != redirection)
	}
}

func TestParseBasicPrependRedirection(t *testing.T) {
	tests := []testShellCommand{
		{
			command: "> /tmp/test ls args",
			tokens: []ShellToken{
				{
					Kind: Redirection,
					Val:  ">",
				},
				{
					Kind: Field,
					Val:  "/tmp/test",
				},
				{
					Kind: Executable,
					Val:  "ls",
				},
				{
					Kind: Field,
					Val:  "args",
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
					Kind: Executable,
					Val:  "true",
				},
				{
					Kind: Control,
					Val:  "&&",
				},
				{
					Kind: Executable,
					Val:  "echo",
				},
				{
					Kind: DoubleQuote,
					Val:  "\"",
				},
				{
					Kind: Field,
					Val:  "hello",
				},
				{
					Kind: DoubleQuote,
					Val:  "\"",
				},
				{
					Kind: Control,
					Val:  "|",
				},
				{
					Kind: Executable,
					Val:  "tr",
				},
				{
					Kind: Field,
					Val:  "h",
				},
				{
					Kind: SingleQuote,
					Val:  "'",
				},
				{
					Kind: Field,
					Val:  " ",
				},
				{
					Kind: SingleQuote,
					Val:  "'",
				},
				{
					Kind: Control,
					Val:  "|",
				},
				{
					Kind: Executable,
					Val:  "tr",
				},
				{
					Kind: Field,
					Val:  "e",
				},
				{
					Kind: Field,
					Val:  "a",
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
					Kind: Executable,
					Val:  "cmd",
				},
				{
					Kind: Field,
					Val:  "hello",
				},
				{
					Kind: Control,
					Val:  "\n",
				},
				{
					Kind: Executable,
					Val:  "cat",
				},
				{
					Kind: Field,
					Val:  "/etc/passwd",
				},
			},
		},
	}

	for _, test := range tests {
		assert.True(t, compareGeneratedTokens(test))
	}
}
