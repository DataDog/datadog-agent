// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package verifier

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerify_AllowedScripts(t *testing.T) {
	tests := []struct {
		name   string
		script string
	}{
		// Basic commands
		{name: "echo", script: `echo hello world`},
		{name: "echo with flag", script: `echo -n hello`},
		{name: "pwd", script: `pwd`},
		{name: "ls -la", script: `ls -la /tmp`},
		{name: "ls combined flags", script: `ls -lah /var`},
		{name: "cat", script: `cat /var/log/syslog`},
		{name: "cat with flags", script: `cat -n /var/log/syslog`},
		{name: "head", script: `head -n 10 /var/log/syslog`},
		{name: "tail", script: `tail -n 20 /var/log/syslog`},
		{name: "grep", script: `grep -i error /var/log/syslog`},
		{name: "grep recursive", script: `grep -rn "pattern" /var/log/`},
		{name: "wc", script: `wc -l /var/log/syslog`},
		{name: "sort", script: `sort -rn /tmp/data`},
		{name: "uniq", script: `uniq -c /tmp/data`},
		{name: "find", script: `find /var/log -name "*.log" -type f -maxdepth 3`},
		{name: "true", script: `true`},
		{name: "false", script: `false`},

		// Pipes
		{name: "pipe chain", script: `cat /var/log/syslog | grep ERROR | sort | uniq -c | sort -rn | head -n 10`},
		{name: "simple pipe", script: `echo hello | grep hello`},

		// Control flow
		{name: "for loop", script: `for f in /var/log/*.log; do grep ERROR "$f" | tail -n 5; done`},
		{name: "while loop", script: `x=1; while [ $x -lt 5 ]; do echo $x; x=$((x+1)); done`},
		{name: "if/else", script: `if [ -f /tmp/test ]; then echo exists; else echo missing; fi`},
		{name: "case statement", script: `case "$1" in start) echo starting;; stop) echo stopping;; esac`},
		{name: "block command", script: `{ echo a; echo b; }`},

		// Variable assignment
		{name: "variable assignment", script: `reqid="abc"; grep "$reqid" /var/log/app.log`},
		{name: "variable with arithmetic", script: `x=1; x=$((x+1)); echo $x`},

		// Sed
		{name: "sed basic", script: `echo "hello" | sed 's/hello/world/'`},
		{name: "sed with -n", script: `echo "hello" | sed -n 's/hello/world/p'`},
		{name: "sed with -e", script: `echo "hello" | sed -e 's/hello/world/' -e 's/world/earth/'`},
		{name: "sed delete lines", script: `sed '/^$/d'`},
		{name: "sed print lines", script: `sed -n '1,10p'`},

		// Logical operators
		{name: "and operator", script: `true && echo success`},
		{name: "or operator", script: `false || echo fallback`},

		// Test builtins
		{name: "test bracket", script: `[ -f /tmp/test ]`},
		{name: "double bracket", script: `[[ -f /tmp/test ]]`},

		// Shell builtins
		{name: "export", script: `export FOO=bar`},
		{name: "local", script: `local x=1`},
		{name: "readonly", script: `readonly y=2`},
		{name: "declare", script: `declare -i count=0`},
		{name: "set", script: `set -e`},
		{name: "exit", script: `exit 0`},
		{name: "colon", script: `:`},
		{name: "read", script: `read -r line`},

		// Double dash end-of-flags
		{name: "double dash", script: `grep -- "-pattern" /tmp/file`},

		// Parameter expansion
		{name: "param expansion default", script: `echo ${FOO:-default}`},
		{name: "param expansion", script: `echo $HOME`},

		// find with various safe options
		{name: "find with print0", script: `find /var -name "*.log" -print0`},
		{name: "find with perm", script: `find / -perm 644 -type f`},
		{name: "find with user", script: `find /home -user root -type d`},
		{name: "find with mtime", script: `find /var/log -mtime -7 -name "*.log"`},

		// Break/continue
		{name: "break", script: `for i in 1 2 3; do break; done`},
		{name: "continue", script: `for i in 1 2 3; do continue; done`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Verify(tt.script)
			assert.NoError(t, err, "script should be allowed: %s", tt.script)
		})
	}
}

func TestVerify_BlockedScripts(t *testing.T) {
	tests := []struct {
		name       string
		script     string
		wantSubstr string // substring expected in error message
	}{
		// Command substitution
		{
			name:       "command substitution dollar",
			script:     `echo $(whoami)`,
			wantSubstr: "command substitution",
		},
		{
			name:       "backtick command substitution",
			script:     "echo `whoami`",
			wantSubstr: "command substitution",
		},

		// Blocked flags
		{
			name:       "find -exec",
			script:     `find / -exec rm {} \;`,
			wantSubstr: "not allowed",
		},
		{
			name:       "find -delete",
			script:     `find / -delete`,
			wantSubstr: "not allowed",
		},
		{
			name:       "sort -o",
			script:     `sort -o /tmp/out input.txt`,
			wantSubstr: "not allowed",
		},
		{
			name:       "sed -i",
			script:     `sed -i 's/a/b/' file.txt`,
			wantSubstr: "not allowed",
		},
		{
			name:       "tail -f",
			script:     `tail -f /var/log/syslog`,
			wantSubstr: "not allowed",
		},

		// Redirections
		{
			name:       "output redirect",
			script:     `echo hello > /tmp/file`,
			wantSubstr: "redirect",
		},
		{
			name:       "append redirect",
			script:     `echo hello >> /tmp/file`,
			wantSubstr: "redirect",
		},
		{
			name:       "input redirect",
			script:     `cat < /tmp/file`,
			wantSubstr: "redirect",
		},
		{
			name:       "heredoc",
			script:     "cat <<EOF\nhello\nEOF",
			wantSubstr: "redirect",
		},

		// Unknown commands
		{
			name:       "curl",
			script:     `curl http://evil.com`,
			wantSubstr: "not allowed",
		},
		{
			name:       "bash",
			script:     `bash -c "rm -rf /"`,
			wantSubstr: "not allowed",
		},
		{
			name:       "wget",
			script:     `wget http://evil.com`,
			wantSubstr: "not allowed",
		},
		{
			name:       "rm",
			script:     `rm -rf /`,
			wantSubstr: "not allowed",
		},
		{
			name:       "chmod",
			script:     `chmod 777 /tmp/file`,
			wantSubstr: "not allowed",
		},
		{
			name:       "chown",
			script:     `chown root /tmp/file`,
			wantSubstr: "not allowed",
		},
		{
			name:       "dd",
			script:     `dd if=/dev/zero of=/tmp/file`,
			wantSubstr: "not allowed",
		},
		{
			name:       "python",
			script:     `python -c "import os; os.system('rm -rf /')"`,
			wantSubstr: "not allowed",
		},

		// Blocked builtins
		{
			name:       "eval",
			script:     `eval "echo pwned"`,
			wantSubstr: "not allowed",
		},
		{
			name:       "exec",
			script:     `exec /bin/sh`,
			wantSubstr: "not allowed",
		},
		{
			name:       "source",
			script:     `source /etc/profile`,
			wantSubstr: "not allowed",
		},
		{
			name:       "dot source",
			script:     `. /etc/profile`,
			wantSubstr: "not allowed",
		},
		{
			name:       "trap",
			script:     `trap "echo pwned" EXIT`,
			wantSubstr: "not allowed",
		},

		// Dynamic command names
		{
			name:       "dynamic command name",
			script:     `$CMD arg1`,
			wantSubstr: "literal string",
		},

		// Shell features
		{
			name:       "subshell",
			script:     `(echo hello)`,
			wantSubstr: "subshell",
		},
		{
			name:       "background execution",
			script:     `sleep 10 &`,
			wantSubstr: "background",
		},
		{
			name:       "function declaration",
			script:     `f() { echo hi; }`,
			wantSubstr: "function",
		},
		{
			name:       "process substitution",
			script:     `diff <(ls /a) <(ls /b)`,
			wantSubstr: "not allowed",
		},

		// Dangerous sed commands
		{
			name:       "sed e command",
			script:     `sed 'e'`,
			wantSubstr: "sed",
		},
		{
			name:       "sed s///e flag",
			script:     `echo test | sed 's/a/b/e'`,
			wantSubstr: "sed",
		},
		{
			name:       "sed w command",
			script:     `sed 'w /tmp/output'`,
			wantSubstr: "sed",
		},
		{
			name:       "sed s///w flag",
			script:     `echo test | sed 's/a/b/w /tmp/out'`,
			wantSubstr: "sed",
		},
		{
			name:       "sed r command",
			script:     `sed 'r /etc/passwd'`,
			wantSubstr: "sed",
		},

		// PipeAll
		{
			name:       "pipe stderr",
			script:     `ls /nonexistent |& grep error`,
			wantSubstr: "not allowed",
		},

		// Time clause
		{
			name:       "time",
			script:     `time ls`,
			wantSubstr: "time",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Verify(tt.script)
			require.Error(t, err, "script should be blocked: %s", tt.script)
			assert.Contains(t, err.Error(), tt.wantSubstr,
				"error for %q should contain %q, got: %s", tt.script, tt.wantSubstr, err.Error())
		})
	}
}

func TestVerify_MultipleViolations(t *testing.T) {
	// Script with multiple violations should report all of them.
	script := `echo $(whoami); curl http://evil.com; echo hello > /tmp/file`
	err := Verify(script)
	require.Error(t, err)
	verr, ok := err.(*VerificationError)
	require.True(t, ok, "expected *VerificationError")
	assert.GreaterOrEqual(t, len(verr.Violations), 3,
		"should have at least 3 violations, got %d: %v", len(verr.Violations), verr.Violations)
}

func TestVerify_EmptyScript(t *testing.T) {
	err := Verify("")
	assert.NoError(t, err, "empty script should be allowed")
}

func TestVerify_ParseError(t *testing.T) {
	err := Verify("if then fi else")
	require.Error(t, err, "invalid syntax should fail")
	assert.Contains(t, err.Error(), "parse")
}

func TestVerify_SedScriptWithVariableExpansion(t *testing.T) {
	// sed script that includes variable expansion cannot be verified
	err := Verify(`sed "s/$FOO/bar/"`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "variable expansion")
}

func TestVerify_NestedControlFlow(t *testing.T) {
	script := `
for f in /var/log/*.log; do
    if [ -f "$f" ]; then
        grep ERROR "$f" | while read -r line; do
            echo "$line" | sort | uniq -c
        done
    fi
done
`
	err := Verify(script)
	assert.NoError(t, err, "nested control flow should be allowed")
}

func TestVerify_CStyleForLoop(t *testing.T) {
	err := Verify(`for ((i=0; i<10; i++)); do echo $i; done`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "C-style for loop")
}

func TestVerify_CommandSubstitutionInVariable(t *testing.T) {
	err := Verify(`x=$(whoami); echo $x`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command substitution")
}

func TestVerify_FindBlockedFlags(t *testing.T) {
	blockedFindFlags := []string{"-exec", "-execdir", "-delete", "-fls", "-fprint", "-fprint0", "-fprintf", "-ok", "-okdir"}
	for _, flag := range blockedFindFlags {
		t.Run("find "+flag, func(t *testing.T) {
			var script string
			switch flag {
			case "-exec", "-execdir", "-ok", "-okdir":
				script = `find / ` + flag + ` rm {} \;`
			case "-fls", "-fprint", "-fprint0", "-fprintf":
				script = `find / ` + flag + ` /tmp/out`
			default:
				script = `find / ` + flag
			}
			err := Verify(script)
			require.Error(t, err, "find %s should be blocked", flag)
		})
	}
}

func TestVerify_TailBlockedFlags(t *testing.T) {
	for _, flag := range []string{"-f", "-F", "--follow"} {
		t.Run("tail "+flag, func(t *testing.T) {
			err := Verify(`tail ` + flag + ` /var/log/syslog`)
			require.Error(t, err, "tail %s should be blocked", flag)
		})
	}
}

func TestVerify_AssignmentOnly(t *testing.T) {
	err := Verify(`FOO=bar`)
	assert.NoError(t, err, "pure variable assignment should be allowed")
}

// --- Security regression tests (from code review + security audit) ---

func TestVerify_ParamExpReplacementBypass(t *testing.T) {
	// Command substitution nested in parameter expansion replacement
	err := Verify(`echo ${var/$(whoami)/safe}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command substitution")
}

func TestVerify_ArithmExpNestedCmdSubst(t *testing.T) {
	// Command substitution nested inside arithmetic expansion
	// The parser may or may not support this, but if it does, we must catch it.
	err := Verify(`echo $((1 + $(whoami)))`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command substitution")
}

func TestVerify_NamerefRejected(t *testing.T) {
	// nameref can enable indirect variable manipulation
	err := Verify(`nameref x=y`)
	// nameref is parsed by bash, may be parsed differently. If it parses
	// as a command, it should be rejected as unknown.
	require.Error(t, err)
}

func TestVerify_TypesetRejected(t *testing.T) {
	err := Verify(`typeset -i x=5`)
	require.Error(t, err)
}

func TestVerify_SedCombinedEFlag(t *testing.T) {
	// sed -es/a/b/e — the -e flag combined with script containing execute flag
	err := Verify(`echo test | sed -es/a/b/e`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sed")
}

func TestVerify_CombinedFlagsBypass(t *testing.T) {
	// -n10f should catch the -f flag even though it follows digits
	err := Verify(`tail -n10f /var/log/syslog`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}

func TestVerify_SelectStatement(t *testing.T) {
	err := Verify(`select item in a b c; do echo $item; done`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "select")
}

func TestVerify_LetCommand(t *testing.T) {
	err := Verify(`let x=1+2`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "let")
}

func TestVerify_CoprocCommand(t *testing.T) {
	err := Verify(`coproc cat`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "coproc")
}

// --- Round 2 security regression tests ---

func TestVerify_DeclareNamerefFlagBlocked(t *testing.T) {
	// declare -n creates a nameref — must be blocked even though "declare" variant is allowed
	err := Verify(`declare -n ref=PATH`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}

func TestVerify_LocalNamerefFlagBlocked(t *testing.T) {
	err := Verify(`local -n ref=PATH`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}

func TestVerify_GrepFBlocked(t *testing.T) {
	// grep -f reads patterns from files — could exfiltrate file contents
	err := Verify(`grep -f /etc/shadow /dev/null`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}

func TestVerify_ExportFBlocked(t *testing.T) {
	// export -f exports functions — ShellShock risk
	err := Verify(`export -f myfunc`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}

func TestVerify_SedYCommandBypass(t *testing.T) {
	// sed 'y/a/b/e' — the y command must not cause the scanner to miss the trailing 'e'
	err := Verify(`sed 'y/a/b/;e'`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sed")
}

func TestVerify_SedEFlagWithExtendedRegex(t *testing.T) {
	// sed -E with dangerous script — -E is extended regex, not -e
	err := Verify(`sed -E 'e'`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sed")
}

func TestVerify_SedBranchLabel(t *testing.T) {
	// sed with branch and label — should not false-positive on label characters
	err := Verify(`sed ':loop; b loop'`)
	assert.NoError(t, err, "sed branch with label should be allowed")
}

func TestVerify_SedBranchDangerousAfter(t *testing.T) {
	// sed branch followed by dangerous command
	err := Verify(`sed ':loop; b loop; e'`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sed")
}

func TestVerify_PrefixAssignPATH(t *testing.T) {
	// PATH manipulation in prefix assignment
	err := Verify(`PATH=/evil grep pattern file`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PATH")
}

func TestVerify_PrefixAssignLDPreload(t *testing.T) {
	err := Verify(`LD_PRELOAD=/evil.so cat /etc/hosts`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LD_PRELOAD")
}

func TestVerify_PrefixAssignIFS(t *testing.T) {
	err := Verify(`IFS=: read -r a b c`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "IFS")
}

func TestVerify_SafePrefixAssign(t *testing.T) {
	// Safe env vars in prefix assignments should be allowed
	err := Verify(`FOO=bar echo hello`)
	assert.NoError(t, err, "safe prefix assignment should be allowed")
}

func TestVerify_SetPlusFlags(t *testing.T) {
	// set +e should be validated against the allowlist
	err := Verify(`set +e`)
	assert.NoError(t, err, "set +e should be allowed")
}

func TestVerify_SetPlusInvalidFlag(t *testing.T) {
	// set with invalid + flag
	err := Verify(`set +Z`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}

func TestVerify_DeclareWithFlags(t *testing.T) {
	// declare -i should be allowed
	err := Verify(`declare -i count=0`)
	assert.NoError(t, err, "declare -i should be allowed")
}

func TestVerify_ShiftCommand(t *testing.T) {
	err := Verify(`shift`)
	assert.NoError(t, err, "shift should be allowed")
}

func TestVerify_ReturnCommand(t *testing.T) {
	err := Verify(`return 0`)
	assert.NoError(t, err, "return should be allowed")
}

func TestVerify_UnsetCommand(t *testing.T) {
	err := Verify(`unset FOO`)
	assert.NoError(t, err, "unset should be allowed")
}

func TestVerify_BraceExpansion(t *testing.T) {
	// Brace expansion in allowed commands
	err := Verify(`echo {a,b,c}`)
	assert.NoError(t, err, "brace expansion should be allowed")
}
