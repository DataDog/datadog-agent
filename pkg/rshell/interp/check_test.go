// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/expand"
)

func checkRunner(t *testing.T, options ...RunnerOption) *Runner {
	t.Helper()
	runner, err := New(options...)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, runner.Close()) })
	return runner
}

func checkScript(t *testing.T, runner *Runner, script string) *CheckResult {
	t.Helper()
	result, err := runner.Check(context.Background(), script)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, result.Status == CheckAllowed, result.Allowed)
	for _, command := range result.Commands {
		assert.Equal(t, command.Status == CheckAllowed, command.Allowed)
	}
	return result
}

func checkHasIssue(result *CheckResult, code CheckIssueCode) bool {
	for _, issue := range result.Issues {
		if issue.Code == code {
			return true
		}
	}
	for _, command := range result.Commands {
		for _, issue := range command.Issues {
			if issue.Code == code {
				return true
			}
		}
	}
	return false
}

func quoteCheckPath(path string) string {
	return "'" + strings.ReplaceAll(path, "'", "'\\''") + "'"
}

// These exercise a Go API and its structured report, which cannot be expressed
// by the executable shell scenario harness.
func TestCheckCommands(t *testing.T) {
	tests := []struct {
		name, script string
		allowed      []string
		status       CheckStatus
		issue        CheckIssueCode
		commands     int
	}{
		{"empty", "# comment\n", nil, CheckAllowed, "", 0},
		{"default deny", "echo hello", nil, CheckDenied, CheckCommandNotAllowed, 1},
		{"known", `e"ch"o '/not/a/file'`, []string{"rshell:echo"}, CheckAllowed, "", 1},
		{"unknown", "not-a-builtin", []string{"rshell:not-a-builtin"}, CheckDenied, CheckUnknownCommand, 1},
		{"external path", "/bin/echo hello", []string{"rshell:echo"}, CheckDenied, CheckCommandNotAllowed, 1},
		{"pipeline", "echo hi | cat; echo done", []string{"rshell:echo", "rshell:cat"}, CheckAllowed, "", 3},
		{"denied pipeline", "echo hi | cat", []string{"rshell:echo"}, CheckDenied, CheckCommandNotAllowed, 2},
		{"all branches", "if true; then echo yes; else cat; fi", []string{"rshell:true", "rshell:echo"}, CheckDenied, CheckCommandNotAllowed, 3},
		{"syntax", "echo '", []string{"rshell:echo"}, CheckDenied, CheckParseError, 0},
		{"unsupported", "echo $((1+1))", []string{"rshell:echo"}, CheckDenied, CheckUnsupportedSyntax, 0},
		{"blocked background", "echo hi &", []string{"rshell:echo"}, CheckDenied, CheckUnsupportedSyntax, 0},
		{"invalid flag", "cat --not-a-real-flag", []string{"rshell:cat"}, CheckDenied, CheckInvalidArguments, 1},
		{"help normalization", "cat --help --not-a-real-flag", []string{"rshell:cat"}, CheckAllowed, "", 1},
		{"find help", "find -L --help", []string{"rshell:find"}, CheckAllowed, "", 1},
		{"find unsupported flag", "find -H --help", []string{"rshell:find"}, CheckDenied, CheckInvalidArguments, 1},
		{"flag before help", "cat --not-a-real-flag --help", []string{"rshell:cat"}, CheckDenied, CheckInvalidArguments, 1},
		{"mode gate on help", "rm --help", []string{"rshell:rm"}, CheckDenied, CheckRemediationRequired, 1},
		{"sed mode", "sed -i --help", []string{"rshell:sed"}, CheckDenied, CheckRemediationRequired, 1},
		{"quoted glob", "echo '*'", []string{"rshell:echo"}, CheckAllowed, "", 1},
		{"escaped glob", `echo \*`, []string{"rshell:echo"}, CheckAllowed, "", 1},
		{"glob", "cat *", []string{"rshell:cat"}, CheckIndeterminate, CheckRequiresExecution, 1},
		{"braces", "echo {a,b}", []string{"rshell:echo"}, CheckIndeterminate, CheckRequiresExecution, 1},
		{"substitution", `echo "$(echo hi)"`, []string{"rshell:echo"}, CheckIndeterminate, CheckRequiresExecution, 2},
		{"shortcut substitution", `echo "$(<file)"`, []string{"rshell:echo"}, CheckDenied, CheckCommandNotAllowed, 2},
		{"denied substitution", `echo "$(cat)"`, []string{"rshell:echo"}, CheckDenied, CheckCommandNotAllowed, 2},
		{"dynamic name", `$(echo cat)`, []string{"rshell:echo", "rshell:cat"}, CheckIndeterminate, CheckRequiresExecution, 2},
		{"loop", `for f in a b; do cat "$f"; done`, []string{"rshell:cat"}, CheckIndeterminate, CheckRequiresExecution, 1},
		{"infinite loop", "while true; do echo hello; done", []string{"rshell:true", "rshell:echo"}, CheckAllowed, "", 2},
		{"exit status not predicted", "false", []string{"rshell:false"}, CheckAllowed, "", 1},
		{"nested invocation", "xargs rm", []string{"rshell:xargs"}, CheckDenied, CheckCommandNotAllowed, 1},
		{"xargs runtime args", "xargs cat", []string{"rshell:xargs", "rshell:cat"}, CheckIndeterminate, CheckRequiresExecution, 1},
		{"missing sudo name", "sudo", nil, CheckDenied, CheckInvalidArguments, 1},
		{"elevation denied", "sudo echo hello", []string{"rshell:echo"}, CheckDenied, CheckElevationNotAllowed, 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := checkRunner(t, AllowedCommands(test.allowed))
			result := checkScript(t, runner, test.script)
			assert.Equal(t, test.status, result.Status, "%+v", result)
			assert.Len(t, result.Commands, test.commands)
			if test.issue != "" {
				assert.True(t, checkHasIssue(result, test.issue), "%+v", result)
			}
			// Both entry points must share syntax, flags, traversal, command
			// policy, and the complete report envelope without a second matrix.
			remote, err := CheckRemote(context.Background(), test.script, RemotePolicy{AllowedCommands: test.allowed})
			require.NoError(t, err)
			assert.Equal(t, result.Status, remote.Status)
			assert.Equal(t, result.Allowed, remote.Allowed)
			assert.Len(t, remote.Commands, test.commands)
			if test.issue != "" {
				assert.True(t, checkHasIssue(remote, test.issue), "%+v", remote)
			}
		})
	}
}

func TestCheckPathsAndExpansion(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret")
	file := filepath.Join(root, "file with spaces")
	require.NoError(t, os.WriteFile(file, []byte("original\n"), 0600))
	runner := checkRunner(t, allowAllCommandsOpt(), AllowedPaths([]string{root + ":rw"}), WithMode(ModeRemediation), Env("FILE="+file))
	tests := []struct {
		script string
		status CheckStatus
		issue  CheckIssueCode
	}{
		{"cat " + quoteCheckPath(file), CheckAllowed, ""},
		{`cat "$FILE"`, CheckAllowed, ""},
		{`F="file with spaces"; cat "$F"`, CheckAllowed, ""},
		{`F='file with spaces'; G="$F"; cat "$G"`, CheckAllowed, ""},
		{`CMD=cat; $CMD "$FILE"`, CheckAllowed, ""},
		{`CMD='cat ` + outside + `'; $CMD`, CheckDenied, CheckPathNotAllowed},
		{`F='*'; cat $F`, CheckIndeterminate, CheckRequiresExecution},
		{`cat -`, CheckAllowed, ""},
		{`head -5 "file with spaces"`, CheckAllowed, ""},
		{`grep /not/a/path "file with spaces"`, CheckAllowed, ""},
		{`grep -e /not/a/path "file with spaces"`, CheckAllowed, ""},
		{`echo hi > new-file`, CheckAllowed, ""},
		{`echo hi > "file with spaces"`, CheckAllowed, ""},
		{`cat < "$FILE"`, CheckAllowed, ""},
		{`cat < $FILE`, CheckDenied, CheckInvalidArguments},
		{`echo hi > $FILE`, CheckDenied, CheckInvalidArguments},
		{`F=''; cat < $F`, CheckDenied, CheckInvalidArguments},
		{`cat < *`, CheckIndeterminate, CheckRequiresExecution},
		{`F='*'; echo hi > $F`, CheckIndeterminate, CheckRequiresExecution},
		{`rm "file with spaces"`, CheckAllowed, ""},
		{`tee -`, CheckAllowed, ""},
		{"cat " + quoteCheckPath(outside), CheckDenied, CheckPathNotAllowed},
		{"cat < " + quoteCheckPath(outside), CheckDenied, CheckPathNotAllowed},
		{"< " + quoteCheckPath(outside), CheckDenied, CheckPathNotAllowed},
		{"echo hi > " + quoteCheckPath(outside), CheckDenied, CheckPathNotAllowed},
		{`F=` + quoteCheckPath(outside) + `; cat "$F"`, CheckDenied, CheckPathNotAllowed},
		{`HOME=` + quoteCheckPath(outside) + ` cd`, CheckDenied, CheckPathNotAllowed},
		{`F='file with spaces'; if true; then F=` + quoteCheckPath(outside) + `; fi; cat "$F"`, CheckIndeterminate, CheckRequiresExecution},
		{`F='file with spaces'; (F=` + quoteCheckPath(outside) + `); cat "$F"`, CheckAllowed, ""},
		{`cd .; cat "file with spaces"`, CheckIndeterminate, CheckRequiresExecution},
		{`cd .; cat ` + quoteCheckPath(file), CheckAllowed, ""},
		{`sha256sum -c "file with spaces"`, CheckIndeterminate, CheckRequiresExecution},
		{`sed 'p' "file with spaces"`, CheckIndeterminate, CheckRequiresExecution},
		{`awk '{print}' "file with spaces"`, CheckIndeterminate, CheckRequiresExecution},
		{`find . -exec cat '{}' ';'`, CheckIndeterminate, CheckRequiresExecution},
	}
	for _, test := range tests {
		t.Run(test.script, func(t *testing.T) {
			result := checkScript(t, runner, test.script)
			assert.Equal(t, test.status, result.Status, "%+v", result)
			if test.issue != "" {
				assert.True(t, checkHasIssue(result, test.issue), "%+v", result)
			}
		})
	}
	contents, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, "original\n", string(contents))
	_, err = os.Stat(filepath.Join(root, "new-file"))
	assert.True(t, os.IsNotExist(err))
}

func TestCheckReadOnlyPathsAndNullRedirect(t *testing.T) {
	root := t.TempDir()
	runner := checkRunner(t, allowAllCommandsOpt(), AllowedPaths([]string{root}), WithMode(ModeRemediation))
	for _, script := range []string{"echo hi > file", "rm file", "tee file", "truncate -s 0 file", "sed -i 's/a/b/' file"} {
		result := checkScript(t, runner, script)
		assert.Equal(t, CheckDenied, result.Status, script)
		assert.True(t, checkHasIssue(result, CheckPathNotAllowed), script)
	}
	noPaths := checkRunner(t, allowAllCommandsOpt())
	for _, script := range []string{`echo hi > /dev/null`, `echo hi > "/dev/null"`, `F=/dev/null; echo hi > "$F"`} {
		assert.True(t, checkScript(t, noPaths, script).Allowed, script)
	}
	assert.True(t, checkHasIssue(checkScript(t, noPaths, `cat file`), CheckPathNotAllowed))
	assert.True(t, checkHasIssue(checkScript(t, noPaths, `echo hi > "file"`), CheckRemediationRequired))
}

func TestCheckBuiltinHelpPolicyMatchesRun(t *testing.T) {
	root := t.TempDir()
	for _, paths := range [][]string{nil, {}, {root}, {root + ":rw"}} {
		for _, script := range []string{"rm --help", "tee --help", "truncate --help", "logrotate --help", "sed -i --help"} {
			t.Run(script+strings.Join(paths, ","), func(t *testing.T) {
				options := []RunnerOption{allowAllCommandsOpt(), WithMode(ModeRemediation)}
				if paths != nil {
					options = append(options, AllowedPaths(paths))
				}
				runner := checkRunner(t, options...)
				result := checkScript(t, runner, script)
				program, err := ParseScript(script, "")
				require.NoError(t, err)
				err = runner.Run(context.Background(), program)
				assert.Equal(t, err == nil, result.Allowed, "%+v", result)
			})
		}
	}
}

func TestCheckMetadataOperandsDoNotFollowFinalSymlinks(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink(filepath.Join(t.TempDir(), "secret"), filepath.Join(root, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	runner := checkRunner(t, allowAllCommandsOpt(), AllowedPaths([]string{root}))
	for _, script := range []string{"ls -l link", "du link", "find link", "test -L link"} {
		result := checkScript(t, runner, script)
		assert.True(t, result.Allowed, "%s: %+v", script, result)
	}
	for _, script := range []string{"cat link", "du -L link", "find -L link", "test -f link"} {
		assert.True(t, checkHasIssue(checkScript(t, runner, script), CheckPathNotAllowed), script)
	}
}

func TestCheckDoesNotExecuteOrChangeRunner(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file")
	require.NoError(t, os.WriteFile(file, []byte("original\n"), 0600))
	stdin, err := os.Open(file)
	require.NoError(t, err)
	defer stdin.Close()
	var stdout, stderr bytes.Buffer
	called := false
	runner := checkRunner(t, allowAllCommandsOpt(), AllowedPaths([]string{root + ":rw"}), WithMode(ModeRemediation),
		Env("X=original"), StdIO(stdin, &stdout, &stderr),
		SelectiveElevation([]string{"rshell:echo"}, func(context.Context, string, func()) error {
			called = true
			panic("check must never elevate")
		}))
	script := `X=changed; cd .; echo replacement > file; rm file; truncate -s 0 file; tee new-file; sed -i 's/original/replaced/' file; sudo echo hi; echo "$(echo substituted > created)"`
	result := checkScript(t, runner, script)
	assert.False(t, result.Allowed) // cd makes subsequent relative paths unknown.
	assert.False(t, called)
	assert.False(t, runner.didReset)
	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	contents, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, "original\n", string(contents))
	position, err := stdin.Seek(0, io.SeekCurrent)
	require.NoError(t, err)
	assert.Zero(t, position)
	program, err := ParseScript(`echo "$X"`, "")
	require.NoError(t, err)
	require.NoError(t, runner.Run(context.Background(), program))
	assert.Equal(t, "original\n", stdout.String())
	assert.True(t, checkScript(t, runner, `sudo echo hi`).Allowed)
	assert.True(t, checkHasIssue(checkScript(t, runner, `echo hi | (sudo echo there)`), CheckElevationNotAllowed))
	assert.False(t, called)
	assert.Equal(t, CheckIndeterminate, checkScript(t, runner, `if true; then sudo echo hi; fi; cat file`).Status)
}

func TestCheckExpansionBudgets(t *testing.T) {
	runner := checkRunner(t, allowAllCommandsOpt(), Env("BIG="+strings.Repeat("x", MaxVarBytes)))
	result := checkScript(t, runner, `echo "`+strings.Repeat("$BIG", 64)+`"`)
	assert.Equal(t, CheckIndeterminate, result.Status)
	assert.True(t, checkHasIssue(result, CheckRequiresExecution))
	// Sequential assignments must not accumulate unbounded retained strings.
	result = checkScript(t, runner, "A=$BIG; B=$BIG")
	assert.Equal(t, CheckDenied, result.Status)
	assert.True(t, checkHasIssue(result, CheckLimitExceeded))
	result = checkScript(t, runner, strings.Repeat("{ ", maxCheckDepth+2)+"echo hi; "+strings.Repeat("}; ", maxCheckDepth+2))
	assert.False(t, result.Allowed)
	assert.True(t, checkHasIssue(result, CheckLimitExceeded))
}

func TestCheckUsesCurrentRunnerState(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "subdir"), 0700))
	runner := checkRunner(t, allowAllCommandsOpt(), AllowedPaths([]string{root}))
	program, err := ParseScript(`F=../../outside; cd subdir`, "")
	require.NoError(t, err)
	require.NoError(t, runner.Run(context.Background(), program))
	result := checkScript(t, runner, `cat "$F"`)
	assert.Equal(t, CheckDenied, result.Status)
	assert.True(t, checkHasIssue(result, CheckPathNotAllowed))
	before := runner.Dir
	checkScript(t, runner, `F=changed; cd ..`)
	assert.Equal(t, before, runner.Dir)
	assert.Equal(t, "../../outside", runner.writeEnv.Get("F").Str)
}

func TestCheckSystemServices(t *testing.T) {
	runner := checkRunner(t, allowAllCommandsOpt(), WithMode(ModeRemediation), AllowedSystemServices([]SystemServiceControlGrant{
		{Service: "example.service", Actions: []SystemServiceAction{SystemServiceRead, SystemServiceRestart}},
	}))
	// Nil backends make accidental calls fail loudly; Check needs only grants.
	runner.systemd = nil
	for _, script := range []string{"systemctl restart example.service", "systemctl status example.service", "journalctl -u example.service", "systemctl list-units"} {
		assert.True(t, checkScript(t, runner, script).Allowed, script)
	}
	for _, script := range []string{"systemctl stop example.service", "systemctl restart other.service", "journalctl -u other.service", "journalctl --rotate"} {
		assert.True(t, checkHasIssue(checkScript(t, runner, script), CheckSystemServiceNotAllowed), script)
	}
	readOnly := checkRunner(t, allowAllCommandsOpt())
	assert.True(t, checkHasIssue(checkScript(t, readOnly, "systemctl --help"), CheckRemediationRequired))
	assert.True(t, checkHasIssue(checkScript(t, readOnly, "journalctl --rotate"), CheckRemediationRequired))
}

func TestCheckPositionsWarningsAndJSON(t *testing.T) {
	runner := checkRunner(t, AllowedCommands([]string{"rshell:echo"}), AllowedPaths([]string{filepath.Join(t.TempDir(), "missing")}))
	result := checkScript(t, runner, "echo first\n  cat\n")
	require.Len(t, result.Commands, 2)
	assert.Equal(t, uint(2), result.Commands[1].Line)
	assert.Equal(t, uint(3), result.Commands[1].Column)
	assert.Equal(t, "cat", result.Commands[1].Command)
	assert.Equal(t, runner.Warnings(), result.Warnings)
	require.NotEmpty(t, result.Warnings)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"allowed":false`)
	assert.Contains(t, string(encoded), `"code":"command_not_allowed"`)
}

func TestCheckCancellationAndLimits(t *testing.T) {
	runner := checkRunner(t, allowAllCommandsOpt())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := runner.Check(ctx, "echo hi")
	assert.Nil(t, result)
	assert.ErrorIs(t, err, context.Canceled)
	bounded := checkRunner(t, allowAllCommandsOpt(), MaxExecutionTime(time.Nanosecond))
	_, err = bounded.Check(context.Background(), strings.Repeat("echo hi;", 1000))
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	result = checkScript(t, runner, strings.Repeat(" ", MaxScriptBytes+1))
	assert.True(t, checkHasIssue(result, CheckParseError))
	result = checkScript(t, runner, strings.Repeat("echo hi;", MaxCheckCommands+1))
	assert.False(t, result.Allowed)
	assert.True(t, checkHasIssue(result, CheckLimitExceeded))
	assert.Len(t, result.Commands, MaxCheckCommands)
	var invalid Runner
	_, err = invalid.Check(context.Background(), "")
	require.Error(t, err)
}

type cancelAssignmentEnvironment struct {
	cancel context.CancelFunc
	reads  int
}

func (e *cancelAssignmentEnvironment) Get(name string) expand.Variable {
	if strings.HasPrefix(name, "V") {
		e.reads++
		if e.reads == 100 {
			e.cancel()
		}
	}
	return expand.Variable{}
}

func (*cancelAssignmentEnvironment) Each(func(string, expand.Variable) bool) {}

func TestCheckCancellationDuringAssignments(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := &cancelAssignmentEnvironment{cancel: cancel}
	runner := checkRunner(t, func(r *Runner) error { r.Env = env; return nil })
	var script strings.Builder
	for i := 0; i < 25000; i++ {
		fmt.Fprintf(&script, "V%d= ", i)
	}
	// Cancel only once checking has reached assignment 100. This exercises
	// cancellation inside one large statement, not before parsing/traversal.
	result, err := runner.Check(ctx, script.String())
	assert.Nil(t, result)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 100, env.reads, "assignment checking must stop promptly on cancellation")
}

func TestCheckAssignmentStorageAccounting(t *testing.T) {
	env := []string{"HALF=" + strings.Repeat("x", MaxTotalVarsBytes/2)}
	commands := []string{"rshell:echo", "rshell:true"}
	runner := checkRunner(t, Env(env...), AllowedCommands(commands))
	for _, test := range []struct {
		script string
		status CheckStatus
	}{
		{`V=$HALF; V=$HALF; V=$HALF`, CheckAllowed},
		{`A=$HALF; B=$HALF; C=x`, CheckDenied},
		{`A=$HALF; A=; B=$HALF; C=$HALF`, CheckAllowed},
		{`A=$HALF echo hi; B=$HALF; C=$HALF`, CheckAllowed},
		{`A=$HALF; if true; then A=; fi; B=$HALF; C=$HALF`, CheckAllowed},
	} {
		assert.Equal(t, test.status, checkScript(t, runner, test.script).Status, test.script)
		policy := RemotePolicy{Env: env, AllowedCommands: commands}
		assert.Equal(t, test.status, checkRemoteScript(t, test.script, policy).Status, test.script)
	}
}
