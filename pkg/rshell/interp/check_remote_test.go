// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func checkRemoteScript(t *testing.T, script string, policy RemotePolicy) *CheckResult {
	t.Helper()
	result, err := CheckRemote(context.Background(), script, policy)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, result.Status == CheckAllowed, result.Allowed)
	for _, command := range result.Commands {
		assert.Equal(t, command.Status == CheckAllowed, command.Allowed)
	}
	assert.Empty(t, result.Warnings, "remote grants must not produce local-root warnings")
	return result
}

func TestCheckRemotePaths(t *testing.T) {
	base := filepath.Join(t.TempDir(), "remote-only")
	root := filepath.Join(base, "logs")
	private := filepath.Join(root, "private")
	writable := filepath.Join(private, "writable")
	file := filepath.Join(root, "app.log")
	out := filepath.Join(base, "outside")
	policy := RemotePolicy{
		AllowedCommands: []string{"rshell:cat", "rshell:echo", "rshell:rm", "rshell:truncate", "rshell:cd", "rshell:ls", "rshell:test"},
		AllowedPaths:    []string{root + ":rw", private + ":ro", writable + ":rw"}, Mode: ModeRemediation,
	}
	require.NoFileExists(t, root)
	tests := []struct {
		name, script string
		status       CheckStatus
		issue        CheckIssueCode
		path         string
	}{
		{"nonexistent grant", "cat " + quoteCheckPath(file), CheckIndeterminate, CheckRequiresExecution, file},
		{"write", "echo hi > " + quoteCheckPath(file), CheckIndeterminate, CheckRequiresExecution, file},
		{"remove", "rm " + quoteCheckPath(file), CheckIndeterminate, CheckRequiresExecution, file},
		{"read only overlap", "echo hi > " + quoteCheckPath(filepath.Join(private, "x")), CheckDenied, CheckPathNotAllowed, filepath.Join(private, "x")},
		{"writable overlap", "truncate -s 0 " + quoteCheckPath(filepath.Join(writable, "x")), CheckIndeterminate, CheckRequiresExecution, filepath.Join(writable, "x")},
		{"read only read", "cat " + quoteCheckPath(filepath.Join(private, "x")), CheckIndeterminate, CheckRequiresExecution, filepath.Join(private, "x")},
		{"boundary", "cat " + quoteCheckPath(root+"-other/x"), CheckDenied, CheckPathNotAllowed, root + "-other/x"},
		{"traversal", "cat " + quoteCheckPath(root+"/../outside"), CheckDenied, CheckPathNotAllowed, root + "/../outside"},
		{"unknown cwd", "cat app.log", CheckIndeterminate, CheckRequiresExecution, "app.log"},
		{"default path", "ls", CheckIndeterminate, CheckRequiresExecution, "."},
		{"input redirect", "cat < " + quoteCheckPath(out), CheckDenied, CheckPathNotAllowed, out},
		{"metadata", "test -f " + quoteCheckPath(file), CheckIndeterminate, CheckRequiresExecution, file},
		{"all commands", "cat " + quoteCheckPath(file) + "; cat " + quoteCheckPath(out), CheckDenied, CheckPathNotAllowed, out},
		{"substitution", "echo \"$(cat " + quoteCheckPath(file) + ")\"", CheckIndeterminate, CheckRequiresExecution, file},
		{"substitution denied", "echo \"$(cat " + quoteCheckPath(out) + ")\"", CheckDenied, CheckPathNotAllowed, out},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			report := checkRemoteScript(t, test.script, policy)
			assert.Equal(t, test.status, report.Status, "%+v", report)
			found := false
			for _, command := range report.Commands {
				for _, issue := range command.Issues {
					found = found || (issue.Code == test.issue && issue.Path == test.path)
				}
			}
			assert.True(t, found, "missing %s for original path %q: %+v", test.issue, test.path, report)
		})
	}
	policy.Dir = root
	assert.Equal(t, CheckIndeterminate, checkRemoteScript(t, "cat app.log", policy).Status)
	assert.Equal(t, CheckDenied, checkRemoteScript(t, "cat ../outside", policy).Status)
	assert.Equal(t, CheckIndeterminate, checkRemoteScript(t, "cd .; cat ../outside", policy).Status)
	policy.AllowedPaths = nil
	assert.Equal(t, CheckDenied, checkRemoteScript(t, "cat app.log", policy).Status)
	policy.Dir = ""
	assert.Equal(t, CheckDenied, checkRemoteScript(t, "cat app.log", policy).Status)
}

func TestCheckRemoteModesAndDuplicateGrants(t *testing.T) {
	root := filepath.Join(t.TempDir(), "remote")
	policy := RemotePolicy{AllowedCommands: []string{"rshell:echo", "rshell:rm", "rshell:tee", "rshell:sed"}, AllowedPaths: []string{root + ":rw"}}
	for _, script := range []string{"rm --help", "tee --help", "sed -i --help", "echo hi > " + quoteCheckPath(filepath.Join(root, "out"))} {
		report := checkRemoteScript(t, script, policy)
		assert.Equal(t, CheckDenied, report.Status)
	}
	// The interpreter's unconditional null-output sink remains authorized.
	assert.Equal(t, CheckAllowed, checkRemoteScript(t, "echo hi > /dev/null", policy).Status)
	policy.Mode = ModeRemediation
	policy.AllowedPaths = []string{root + ":ro"}
	assert.Equal(t, CheckDenied, checkRemoteScript(t, "echo hi > relative", policy).Status, "unknown cwd cannot supply a missing write grant")
	for _, grants := range [][]string{{root}, {root + ":ro"}, {root + ":ro", root + ":rw"}, {root + ":rw", root + ":ro"}} {
		policy.AllowedPaths = grants
		report := checkRemoteScript(t, "echo hi > "+quoteCheckPath(filepath.Join(root, "out")), policy)
		assert.Equal(t, CheckDenied, report.Status, "%v", grants)
		assert.True(t, checkHasIssue(report, CheckPathNotAllowed))
	}
	policy.AllowedPaths = []string{root + ":rw"}
	assert.Equal(t, CheckIndeterminate, checkRemoteScript(t, "tee --help", policy).Status)
	assert.Equal(t, CheckIndeterminate, checkRemoteScript(t, "tee "+quoteCheckPath(filepath.Join(root, "out")), policy).Status)
}

func TestCheckRemoteIndependentOfLocalState(t *testing.T) {
	local := t.TempDir()
	root := filepath.Join(local, "remote-root")
	file := filepath.Join(root, "file")
	link := filepath.Join(root, "link")
	policy := RemotePolicy{
		AllowedCommands: []string{"rshell:cat", "rshell:echo", "rshell:rm", "rshell:truncate", "rshell:ls"},
		AllowedPaths:    []string{root + ":rw"}, Mode: ModeRemediation, Env: []string{"FILE=" + file},
	}
	script := `cat "$FILE"; cat ` + quoteCheckPath(link) + "; cat relative; ls " + quoteCheckPath(root) +
		"; echo change > " + quoteCheckPath(file) + "; truncate -s 0 " + quoteCheckPath(file) + "; rm " + quoteCheckPath(file)
	before := checkRemoteScript(t, script, policy)
	require.Equal(t, CheckIndeterminate, before.Status)
	// A file where the remote grant expects a directory must have no effect.
	require.NoError(t, os.WriteFile(root, []byte("not a local directory"), 0600))
	assert.Equal(t, before, checkRemoteScript(t, script, policy))
	require.NoError(t, os.Remove(root))
	require.NoError(t, os.Mkdir(root, 0700))
	require.NoError(t, os.WriteFile(file, []byte("unchanged"), 0600))
	if runtime.GOOS != "windows" {
		require.NoError(t, os.Symlink(filepath.Join(local, "outside"), link))
		// Literal suffix directories change execution's interpretation, but
		// must not change a remote check based on the embedding host's state.
		require.NoError(t, os.Mkdir(root+":rw", 0700))
		require.NoError(t, os.Chmod(root, 0000))
		t.Cleanup(func() { _ = os.Chmod(root, 0700) })
	}
	t.Chdir(local)
	t.Setenv("FILE", filepath.Join(local, "forbidden"))
	t.Setenv("HOME", filepath.Join(local, "private-home"))
	assert.Equal(t, before, checkRemoteScript(t, script, policy))
	require.NoError(t, os.Chmod(root, 0700))
	contents, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, "unchanged", string(contents), "checking must not write, truncate, or remove")
}

func TestCheckRemoteEnvironment(t *testing.T) {
	root := filepath.Join(t.TempDir(), "remote")
	policy := RemotePolicy{AllowedCommands: []string{"rshell:echo", "rshell:cat"}, AllowedPaths: []string{root}}
	t.Setenv("REMOTE_COMMAND", "cat")
	t.Setenv("REMOTE_FILE", filepath.Join(t.TempDir(), "outside"))
	assert.Equal(t, CheckAllowed, checkRemoteScript(t, `$REMOTE_COMMAND`, policy).Status)
	policy.Env = []string{"REMOTE_COMMAND=cat", "REMOTE_FILE=" + filepath.Join(root, "file")}
	assert.Equal(t, CheckIndeterminate, checkRemoteScript(t, `$REMOTE_COMMAND "$REMOTE_FILE"`, policy).Status)
	assert.Equal(t, CheckIndeterminate, checkRemoteScript(t, `echo "$PWD"`, policy).Status)
	policy.Env = append(policy.Env, "PWD="+root)
	assert.Equal(t, CheckIndeterminate, checkRemoteScript(t, `echo "$PWD"`, policy).Status, "Env PWD is not a working directory")
	policy.Dir = root
	assert.Equal(t, CheckAllowed, checkRemoteScript(t, `echo "$PWD"`, policy).Status)
	assert.Equal(t, CheckIndeterminate, checkRemoteScript(t, `echo "$ALLOWED_PATHS"`, policy).Status)
	assert.Equal(t, CheckIndeterminate, checkRemoteScript(t, `FILE='file'; cat "$FILE"`, policy).Status)
	assert.Equal(t, CheckDenied, checkRemoteScript(t, `FILE='../outside'; cat "$FILE"`, policy).Status)
}

func TestCheckRemoteServiceAndElevationGrants(t *testing.T) {
	policy := RemotePolicy{
		AllowedCommands: []string{"rshell:echo", "rshell:systemctl", "rshell:journalctl"},
		Mode:            ModeRemediation, ElevatableCommands: []string{"rshell:echo", "rshell:systemctl"},
		AllowedSystemServices: []SystemServiceControlGrant{
			{Service: "app.service", Actions: []SystemServiceAction{SystemServiceRead, SystemServiceRestart}},
			{Service: "systemd-journald.service", Actions: []SystemServiceAction{SystemServiceClean}},
		},
	}
	for _, script := range []string{"sudo echo hi", "systemctl status app.service", "sudo systemctl restart app.service", "journalctl -u app.service", "journalctl --rotate"} {
		assert.Equal(t, CheckAllowed, checkRemoteScript(t, script, policy).Status, script)
	}
	for _, script := range []string{"systemctl restart other.service", "systemctl stop app.service", "journalctl -u other.service"} {
		report := checkRemoteScript(t, script, policy)
		assert.Equal(t, CheckDenied, report.Status, script)
		assert.True(t, checkHasIssue(report, CheckSystemServiceNotAllowed))
	}
	for _, script := range []string{"sudo journalctl -u app.service", "sudo echo hi | echo output"} {
		assert.True(t, checkHasIssue(checkRemoteScript(t, script, policy), CheckElevationNotAllowed), script)
	}
	policy.Mode = ModeReadOnly
	for _, script := range []string{"systemctl status app.service", "sudo systemctl restart app.service", "journalctl --rotate"} {
		assert.True(t, checkHasIssue(checkRemoteScript(t, script, policy), CheckRemediationRequired), script)
	}
	policy.AllowedSystemServices = nil
	assert.True(t, checkHasIssue(checkRemoteScript(t, "journalctl -u app.service", policy), CheckSystemServiceNotAllowed))
	policy.AllowedCommands = nil
	assert.True(t, checkHasIssue(checkRemoteScript(t, "sudo echo hi", policy), CheckCommandNotAllowed), "elevation never grants command access")
}

func TestCheckRemoteInvalidConfiguration(t *testing.T) {
	root := filepath.Join(t.TempDir(), "remote")
	for name, policy := range map[string]RemotePolicy{
		"command namespace":   {AllowedCommands: []string{"cat"}},
		"unknown namespace":   {AllowedCommands: []string{"remote:cat"}},
		"empty command":       {AllowedCommands: []string{"rshell:"}},
		"multiple colons":     {AllowedCommands: []string{"rshell:cat:cat"}},
		"elevation namespace": {ElevatableCommands: []string{"otherx:echo"}},
		"relative root":       {AllowedPaths: []string{"remote:rw"}},
		"empty root":          {AllowedPaths: []string{""}},
		"mode suffix":         {AllowedPaths: []string{root + ":write"}},
		"stacked suffix":      {AllowedPaths: []string{root + ":rw:ro"}},
		"nul root":            {AllowedPaths: []string{root + "\x00"}},
		"relative cwd":        {Dir: "remote"},
		"nul cwd":             {Dir: root + "\x00"},
		"mode":                {Mode: "write"},
		"timeout":             {Timeout: -1},
		"env pair":            {Env: []string{"HOME"}},
		"empty env key":       {Env: []string{"=value"}},
		"nul env":             {Env: []string{"KEY=value\x00"}},
		"service glob":        {AllowedSystemServices: []SystemServiceControlGrant{{Service: "*.service", Actions: []SystemServiceAction{SystemServiceRead}}}},
		"service action":      {AllowedSystemServices: []SystemServiceControlGrant{{Service: "app.service", Actions: []SystemServiceAction{"launch"}}}},
	} {
		t.Run(name, func(t *testing.T) {
			report, err := CheckRemote(context.Background(), "", policy)
			require.Error(t, err)
			assert.Nil(t, report)
		})
	}
}

func TestCheckRemoteEnvelopeAndLimits(t *testing.T) {
	policy := RemotePolicy{AllowedCommands: []string{"rshell:echo", "rshell:cat"}}
	report := checkRemoteScript(t, "echo hi\n  cat relative", policy)
	encoded, err := json.Marshal(report)
	require.NoError(t, err)
	assert.JSONEq(t, `{"allowed":false,"status":"denied","commands":[
		{"command":"echo","line":1,"column":1,"allowed":true,"status":"allowed"},
		{"command":"cat","line":2,"column":3,"allowed":false,"status":"denied",
		 "issues":[{"code":"path_not_allowed","message":"no path grants are configured","path":"relative"}]}]}`, string(encoded))
	report = checkRemoteScript(t, "echo '", policy)
	require.Len(t, report.Issues, 1)
	assert.Equal(t, CheckParseError, report.Issues[0].Code)
	assert.Empty(t, report.Commands)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := CheckRemote(ctx, "echo hi", policy)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, context.Canceled)
	policy.Timeout = time.Nanosecond
	result, err = CheckRemote(context.Background(), strings.Repeat("echo hi;", 1000), policy)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	policy.Timeout = time.Minute
	ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	_, err = CheckRemote(ctx, "echo hi", policy)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.True(t, checkHasIssue(checkRemoteScript(t, strings.Repeat(" ", MaxScriptBytes+1), policy), CheckParseError))
	report = checkRemoteScript(t, strings.Repeat("echo hi;", MaxCheckCommands+1), policy)
	assert.Len(t, report.Commands, MaxCheckCommands)
	assert.True(t, checkHasIssue(report, CheckLimitExceeded))
	report = checkRemoteScript(t, strings.Repeat("( ", maxCheckDepth+2)+"echo hi"+strings.Repeat(" )", maxCheckDepth+2), policy)
	assert.False(t, report.Allowed)
	assert.True(t, checkHasIssue(report, CheckLimitExceeded))
	policy.Env = []string{"BIG=" + strings.Repeat("x", MaxVarBytes)}
	assert.False(t, checkRemoteScript(t, `echo "`+strings.Repeat("$BIG", MaxExpandedBytesPerCommand/MaxVarBytes+1)+`"`, policy).Allowed)
}
