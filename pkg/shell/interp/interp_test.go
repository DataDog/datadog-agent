// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runScript(t *testing.T, script string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errOut bytes.Buffer
	r := New(
		WithStdout(&out),
		WithStderr(&errOut),
		WithEnv([]string{"PATH=/usr/bin:/bin:/usr/local/bin"}),
	)
	err = r.Run(context.Background(), script)
	return out.String(), errOut.String(), err
}

// =============================================================================
// Allowed scripts
// =============================================================================

func TestInterp_BasicEcho(t *testing.T) {
	stdout, _, err := runScript(t, `echo hello world`)
	require.NoError(t, err)
	assert.Equal(t, "hello world\n", stdout)
}

func TestInterp_EchoFlags(t *testing.T) {
	stdout, _, err := runScript(t, `echo -n hello`)
	require.NoError(t, err)
	assert.Equal(t, "hello", stdout)
}

func TestInterp_MultipleStatements(t *testing.T) {
	stdout, _, err := runScript(t, `echo hello; echo world`)
	require.NoError(t, err)
	assert.Equal(t, "hello\nworld\n", stdout)
}

func TestInterp_PipeChain(t *testing.T) {
	stdout, _, err := runScript(t, `echo hello | grep hello`)
	require.NoError(t, err)
	assert.Equal(t, "hello\n", stdout)
}

func TestInterp_LongPipeChain(t *testing.T) {
	stdout, _, err := runScript(t, `echo -e "b\na\nc" | sort | head -n 2`)
	require.NoError(t, err)
	assert.Equal(t, "a\nb\n", stdout)
}

func TestInterp_AndOperator(t *testing.T) {
	stdout, _, err := runScript(t, `true && echo success`)
	require.NoError(t, err)
	assert.Equal(t, "success\n", stdout)
}

func TestInterp_AndOperatorShortCircuit(t *testing.T) {
	stdout, _, err := runScript(t, `false && echo should_not_appear`)
	require.NoError(t, err)
	assert.Empty(t, stdout)
}

func TestInterp_OrOperator(t *testing.T) {
	stdout, _, err := runScript(t, `false || echo fallback`)
	require.NoError(t, err)
	assert.Equal(t, "fallback\n", stdout)
}

func TestInterp_OrOperatorShortCircuit(t *testing.T) {
	stdout, _, err := runScript(t, `true || echo should_not_appear`)
	require.NoError(t, err)
	assert.Empty(t, stdout)
}

func TestInterp_ForLoop(t *testing.T) {
	stdout, _, err := runScript(t, `for i in a b c; do echo $i; done`)
	require.NoError(t, err)
	assert.Equal(t, "a\nb\nc\n", stdout)
}

func TestInterp_ForLoopWithGlob(t *testing.T) {
	// Create temp dir with files to glob.
	dir := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		os.WriteFile(filepath.Join(dir, name), []byte(""), 0644)
	}

	var out bytes.Buffer
	r := New(
		WithStdout(&out),
		WithStderr(&bytes.Buffer{}),
		WithDir(dir),
		WithEnv([]string{"PATH=/usr/bin:/bin:/usr/local/bin"}),
	)
	err := r.Run(context.Background(), `for f in *.txt; do echo $f; done`)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	assert.Len(t, lines, 3)
}

func TestInterp_NestedForLoop(t *testing.T) {
	stdout, _, err := runScript(t, `for i in a b; do for j in 1 2; do echo $i $j; done; done`)
	require.NoError(t, err)
	assert.Equal(t, "a 1\na 2\nb 1\nb 2\n", stdout)
}

func TestInterp_BreakInLoop(t *testing.T) {
	stdout, _, err := runScript(t, `for i in 1 2 3; do echo $i; break; done`)
	require.NoError(t, err)
	assert.Equal(t, "1\n", stdout)
}

func TestInterp_ContinueInLoop(t *testing.T) {
	stdout, _, err := runScript(t, `for i in 1 2 3; do continue; echo $i; done`)
	require.NoError(t, err)
	assert.Empty(t, stdout)
}

func TestInterp_TrueCommand(t *testing.T) {
	_, _, err := runScript(t, `true`)
	require.NoError(t, err)
}

func TestInterp_FalseCommand(t *testing.T) {
	var out bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&bytes.Buffer{}))
	err := r.Run(context.Background(), `false`)
	require.NoError(t, err)
	assert.Equal(t, 1, r.ExitCode())
}

func TestInterp_ColonNoOp(t *testing.T) {
	_, _, err := runScript(t, `:`)
	require.NoError(t, err)
}

func TestInterp_ExitCommand(t *testing.T) {
	_, _, err := runScript(t, `exit 42`)
	require.Error(t, err)
	exitErr, ok := err.(*exitError)
	require.True(t, ok)
	assert.Equal(t, 42, exitErr.code)
}

func TestInterp_CdAndPwd(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	r := New(
		WithStdout(&out),
		WithStderr(&bytes.Buffer{}),
		WithDir(dir),
	)
	err := r.Run(context.Background(), `pwd`)
	require.NoError(t, err)
	assert.Equal(t, dir+"\n", out.String())
}

func TestInterp_EmptyScript(t *testing.T) {
	_, _, err := runScript(t, ``)
	require.NoError(t, err)
}

func TestInterp_ExternalCommandLs(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte(""), 0644)

	var out bytes.Buffer
	r := New(
		WithStdout(&out),
		WithStderr(&bytes.Buffer{}),
		WithDir(dir),
		WithEnv([]string{"PATH=/usr/bin:/bin:/usr/local/bin"}),
	)
	err := r.Run(context.Background(), `ls`)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "test.txt")
}

func TestInterp_SedBasic(t *testing.T) {
	stdout, _, err := runScript(t, `echo hello | sed 's/hello/world/'`)
	require.NoError(t, err)
	assert.Equal(t, "world\n", stdout)
}

func TestInterp_Negation(t *testing.T) {
	var out bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&bytes.Buffer{}))
	err := r.Run(context.Background(), `! false`)
	require.NoError(t, err)
	assert.Equal(t, 0, r.ExitCode())
}

func TestInterp_SingleQuotedString(t *testing.T) {
	stdout, _, err := runScript(t, `echo 'hello world'`)
	require.NoError(t, err)
	assert.Equal(t, "hello world\n", stdout)
}

func TestInterp_DoubleQuotedString(t *testing.T) {
	stdout, _, err := runScript(t, `echo "hello world"`)
	require.NoError(t, err)
	assert.Equal(t, "hello world\n", stdout)
}

func TestInterp_ForLoopVarInDoubleQuotes(t *testing.T) {
	stdout, _, err := runScript(t, `for i in hello; do echo "$i world"; done`)
	require.NoError(t, err)
	assert.Equal(t, "hello world\n", stdout)
}

func TestInterp_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var out bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&bytes.Buffer{}))
	err := r.Run(ctx, `echo hello`)
	require.Error(t, err)
}

func TestInterp_PipeExitCode(t *testing.T) {
	var out bytes.Buffer
	r := New(
		WithStdout(&out),
		WithStderr(&bytes.Buffer{}),
		WithEnv([]string{"PATH=/usr/bin:/bin:/usr/local/bin"}),
	)
	// grep with no match returns exit code 1.
	err := r.Run(context.Background(), `echo hello | grep xyz`)
	require.NoError(t, err)
	assert.Equal(t, 1, r.ExitCode())
}

// =============================================================================
// Blocked scripts - unsupported features
// =============================================================================

func TestInterp_BlockedFeatures(t *testing.T) {
	tests := []struct {
		name       string
		script     string
		wantSubstr string
	}{
		// Variable assignment
		{name: "variable assignment", script: `x=value`, wantSubstr: "variable assignment"},
		{name: "prefix assignment", script: `FOO=bar echo hello`, wantSubstr: "variable assignment"},
		{name: "standalone PATH assignment", script: `PATH=/tmp; cat /etc/passwd`, wantSubstr: "variable assignment"},
		{name: "IFS assignment", script: `IFS=/; cat /etc/passwd`, wantSubstr: "variable assignment"},

		// Command substitution
		{name: "command substitution dollar", script: `echo $(whoami)`, wantSubstr: "command substitution"},
		{name: "backtick command substitution", script: "echo `whoami`", wantSubstr: "command substitution"},

		// Process substitution
		{name: "process substitution", script: `diff <(ls /a) <(ls /b)`, wantSubstr: "not supported"},

		// Arithmetic expansion
		{name: "arithmetic expansion", script: `echo $((1+2))`, wantSubstr: "arithmetic expansion"},

		// Parameter expansion (non-loop-var)
		{name: "parameter expansion", script: `echo $HOME`, wantSubstr: "variable expansion"},
		{name: "complex param expansion", script: `echo ${FOO:-default}`, wantSubstr: "not supported"},

		// If/while/case/block
		{name: "if statement", script: `if true; then echo yes; fi`, wantSubstr: "if statements"},
		{name: "while loop", script: `while true; do echo yes; break; done`, wantSubstr: "while"},
		{name: "case statement", script: `case x in x) echo yes;; esac`, wantSubstr: "case"},
		{name: "block command", script: `{ echo a; echo b; }`, wantSubstr: "block"},

		// Subshell
		{name: "subshell", script: `(echo hello)`, wantSubstr: "subshell"},

		// Function declaration
		{name: "function", script: `f() { echo hi; }`, wantSubstr: "function"},

		// Background
		{name: "background", script: `echo hello &`, wantSubstr: "background"},

		// Redirections
		{name: "output redirect", script: `echo hello > /tmp/file`, wantSubstr: "redirect"},
		{name: "input redirect", script: `cat < /tmp/file`, wantSubstr: "redirect"},
		{name: "append redirect", script: `echo hello >> /tmp/file`, wantSubstr: "redirect"},

		// Blocked commands
		{name: "curl", script: `curl http://evil.com`, wantSubstr: "not allowed"},
		{name: "rm", script: `rm -rf /`, wantSubstr: "not allowed"},
		{name: "bash", script: `bash -c "rm -rf /"`, wantSubstr: "not allowed"},
		{name: "eval", script: `eval "echo pwned"`, wantSubstr: "not allowed"},
		{name: "exec", script: `exec /bin/sh`, wantSubstr: "not allowed"},
		{name: "source", script: `source /etc/profile`, wantSubstr: "not allowed"},
		{name: "trap", script: `trap "echo" EXIT`, wantSubstr: "not allowed"},

		// Blocked flags
		{name: "find -exec", script: `find / -exec rm {} \;`, wantSubstr: "not allowed"},
		{name: "find -delete", script: `find / -delete`, wantSubstr: "not allowed"},
		{name: "tail -f", script: `tail -f /var/log/syslog`, wantSubstr: "not allowed"},
		{name: "sed -i", script: `sed -i 's/a/b/' file.txt`, wantSubstr: "not allowed"},

		// Dynamic command name
		{name: "dynamic command", script: `$CMD arg1`, wantSubstr: "literal string"},

		// Pipe stderr
		{name: "pipe stderr", script: `ls /nonexistent |& grep error`, wantSubstr: "not supported"},

		// Other unsupported features
		{name: "C-style for", script: `for ((i=0; i<10; i++)); do echo $i; done`, wantSubstr: "C-style"},
		{name: "select", script: `select item in a b c; do echo $item; done`, wantSubstr: "select"},
		{name: "declare", script: `declare -i x=1`, wantSubstr: "not supported"},
		{name: "export", script: `export FOO=bar`, wantSubstr: "not supported"},
		{name: "local", script: `local x=1`, wantSubstr: "not supported"},
		{name: "readonly", script: `readonly x=1`, wantSubstr: "not supported"},
		{name: "test clause", script: `[[ -f /tmp/test ]]`, wantSubstr: "not supported"},
		{name: "let", script: `let x=1+2`, wantSubstr: "not supported"},
		{name: "time", script: `time ls`, wantSubstr: "not supported"},
		{name: "coproc", script: `coproc cat`, wantSubstr: "not supported"},
		{name: "arithmetic command", script: `(( x = 1 + 2 ))`, wantSubstr: "not supported"},

		// Sed dangerous
		{name: "sed e command", script: `sed 'e'`, wantSubstr: "sed"},
		{name: "sed s///e flag", script: `echo test | sed 's/a/b/e'`, wantSubstr: "sed"},
		{name: "sed w command", script: `sed 'w /tmp/output'`, wantSubstr: "sed"},
		{name: "sed r command", script: `sed 'r /etc/passwd'`, wantSubstr: "sed"},

		// NOTE: brace expansion (echo {a,b,c}) is not tested here because the
		// default mvdan/sh parser treats it as a literal string. If BraceExp
		// nodes are ever generated, the interpreter rejects them.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := runScript(t, tt.script)
			require.Error(t, err, "script should be blocked: %s", tt.script)
			assert.Contains(t, err.Error(), tt.wantSubstr,
				"error for %q should contain %q, got: %s", tt.script, tt.wantSubstr, err.Error())
		})
	}
}

// =============================================================================
// Security regression tests — all bypasses from bypass_test.go must be BLOCKED
// =============================================================================

func TestInterp_SecurityRegression_FlagInjection(t *testing.T) {
	tests := []struct {
		name   string
		script string
	}{
		{
			name:   "find -exec via variable",
			script: `x=-exec; find /tmp $x id {} \;`,
		},
		{
			name:   "find -delete via variable",
			script: `x=-delete; find /important/data -type f $x`,
		},
		{
			name:   "sed -i via variable",
			script: `x=-i; sed $x -e 's/secure/insecure/' /etc/app.conf`,
		},
		{
			name:   "grep -f via variable",
			script: `x=-f; grep $x /etc/shadow /dev/null`,
		},
		{
			name:   "tail -f via variable",
			script: `x=-f; tail $x /var/log/syslog`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := runScript(t, tt.script)
			require.Error(t, err, "bypass should be blocked: %s", tt.script)
		})
	}
}

func TestInterp_SecurityRegression_PathManipulation(t *testing.T) {
	tests := []struct {
		name   string
		script string
	}{
		{name: "standalone PATH", script: `PATH=/tmp; cat /etc/passwd`},
		{name: "standalone IFS", script: `IFS=/; cat /etc/passwd`},
		{name: "standalone LD_PRELOAD", script: `LD_PRELOAD=/tmp/evil.so; cat /etc/hostname`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := runScript(t, tt.script)
			require.Error(t, err, "bypass should be blocked: %s", tt.script)
			assert.Contains(t, err.Error(), "variable assignment")
		})
	}
}

func TestInterp_SecurityRegression_ExportBypass(t *testing.T) {
	tests := []struct {
		name   string
		script string
	}{
		{name: "export PATH", script: `export PATH=/tmp:/usr/bin; grep pattern file`},
		{name: "export LD_PRELOAD", script: `export LD_PRELOAD=/tmp/evil.so; cat /etc/hostname`},
		{name: "export BASH_ENV", script: `export BASH_ENV=/tmp/evil.sh; grep pattern file`},
		{name: "declare -x PATH", script: `declare -x PATH=/tmp:/usr/bin; cat /etc/passwd`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := runScript(t, tt.script)
			require.Error(t, err, "bypass should be blocked: %s", tt.script)
		})
	}
}

func TestInterp_SecurityRegression_ArbitraryCommandExecution(t *testing.T) {
	tests := []struct {
		name   string
		script string
	}{
		{
			name:   "curl via find -exec",
			script: `x=-exec; find /tmp -maxdepth 1 $x curl http://evil.com \;`,
		},
		{
			name:   "rm via find -exec",
			script: `x=-exec; find /tmp -maxdepth 1 $x rm {} \;`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := runScript(t, tt.script)
			require.Error(t, err, "bypass should be blocked: %s", tt.script)
		})
	}
}

// =============================================================================
// Edge cases
// =============================================================================

func TestInterp_BreakOutsideLoop(t *testing.T) {
	_, _, err := runScript(t, `break`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not in a loop")
}

func TestInterp_ContinueOutsideLoop(t *testing.T) {
	_, _, err := runScript(t, `continue`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not in a loop")
}

func TestInterp_ParseError(t *testing.T) {
	_, _, err := runScript(t, `if then fi else`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse error")
}

func TestInterp_NonZeroExitCode(t *testing.T) {
	var out bytes.Buffer
	r := New(
		WithStdout(&out),
		WithStderr(&bytes.Buffer{}),
		WithEnv([]string{"PATH=/usr/bin:/bin:/usr/local/bin"}),
	)
	err := r.Run(context.Background(), `grep nonexistent /dev/null`)
	require.NoError(t, err)
	assert.NotEqual(t, 0, r.ExitCode())
}

func TestInterp_EchoEscapes(t *testing.T) {
	stdout, _, err := runScript(t, `echo -e 'hello\tworld'`)
	require.NoError(t, err)
	assert.Equal(t, "hello\tworld\n", stdout)
}

func TestInterp_ForLoopVarCleanup(t *testing.T) {
	// After a for loop, the loop variable should be cleaned up.
	// Trying to use it after should fail.
	_, _, err := runScript(t, `for i in a; do echo $i; done; echo $i`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "variable expansion")
}

func TestInterp_NestedPipe(t *testing.T) {
	stdout, _, err := runScript(t, `echo hello | grep hello | cat`)
	require.NoError(t, err)
	assert.Equal(t, "hello\n", stdout)
}
