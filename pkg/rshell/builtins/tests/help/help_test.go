// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package help_test

import (
	"bytes"
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
	"github.com/DataDog/datadog-agent/pkg/rshell/internal/interpoption"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

func runScript(t *testing.T, script, dir string, opts ...interp.RunnerOption) (string, string, int) {
	t.Helper()
	return runScriptCtx(context.Background(), t, script, dir, opts...)
}

func runScriptCtx(ctx context.Context, t *testing.T, script, dir string, opts ...interp.RunnerOption) (string, string, int) {
	t.Helper()
	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader(script), "")
	require.NoError(t, err)
	var outBuf, errBuf bytes.Buffer
	allOpts := append([]interp.RunnerOption{interp.StdIO(nil, &outBuf, &errBuf)}, opts...)
	runner, err := interp.New(allOpts...)
	require.NoError(t, err)
	defer runner.Close()
	if dir != "" {
		runner.Dir = dir
	}
	err = runner.Run(ctx, prog)
	exitCode := 0
	if err != nil {
		var es interp.ExitStatus
		if errors.As(err, &es) {
			exitCode = int(es)
		} else if ctx.Err() == nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

func tableLines(output, header string) []string {
	lines := strings.Split(output, "\n")
	var out []string
	inSection := false
	for _, line := range lines {
		if strings.HasPrefix(line, header) {
			inSection = true
			continue
		}
		if !inSection {
			continue
		}
		if line == "" || strings.HasSuffix(line, ":") || strings.HasPrefix(line, "Require remediation mode") || strings.HasPrefix(line, "Disabled by policy") || strings.HasPrefix(line, "Run '") {
			break
		}
		out = append(out, line)
	}
	return out
}

func sectionText(output, header string) string {
	_, section, ok := strings.Cut(output, "\n"+header)
	if !ok {
		return ""
	}
	section, _, _ = strings.Cut(section, "\n\n")
	return header + section
}

func assertTableAligned(t *testing.T, lines []string) {
	t.Helper()
	descCol := -1
	for _, line := range lines {
		col := -1
		for i := len(line) - 1; i >= 2; i-- {
			if line[i] != ' ' && line[i-1] == ' ' && line[i-2] == ' ' {
				col = i
				break
			}
		}
		require.NotEqual(t, -1, col, "should have found description column in line: %q", line)
		if descCol == -1 {
			descCol = col
			continue
		}
		assert.Equal(t, descCol, col,
			"description column should be aligned across all lines, mismatch on line: %q", line)
	}
	assert.Greater(t, descCol, 0, "should have found description column")
}

// --- Exit code ---

func TestHelpExitCode(t *testing.T) {
	stdout, stderr, code := runScript(t, "help", "", interpoption.AllowAllCommands().(interp.RunnerOption))
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.NotEmpty(t, stdout)
}

// --- Header ---

func TestHelpHeaderShowsRshell(t *testing.T) {
	stdout, _, code := runScript(t, "help", "", interpoption.AllowAllCommands().(interp.RunnerOption))
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "rshell")
}

func TestHelpRestrictedShowsAvailableCount(t *testing.T) {
	stdout, _, code := runScript(t, "help", "",
		interp.AllowedCommands([]string{"rshell:echo", "rshell:help"}))
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Commands available now (2):")
}

// --- Output content ---

func TestHelpListsAllCommands(t *testing.T) {
	// Drive registration so builtins.Names()/Meta() are populated.
	r, err := interp.New()
	require.NoError(t, err)
	r.Close()

	// Run in remediation mode so all builtins (including remediation-only ones
	// like truncate) appear in the enabled Commands table with their descriptions.
	stdout, _, code := runScript(t, "help", "", interpoption.AllowAllCommands().(interp.RunnerOption), interp.WithMode(interp.ModeRemediation))
	assert.Equal(t, 0, code)

	// Drive the inventory check off the registry itself so adding a builtin
	// never requires editing this test (or a YAML scenario) — registering the
	// command is what makes it expected.
	names := builtins.Names()
	require.NotEmpty(t, names, "registry should not be empty")
	for _, name := range names {
		meta, ok := builtins.Meta(name)
		require.True(t, ok, "Meta(%q) should exist", name)
		// Every registered builtin must carry a non-empty Description so it
		// shows up in `help`. Without this guard the row regex below would
		// collapse to `^name\s+$` and silently match the blank padded row
		// that printCommandTable emits when Description is empty.
		require.NotEmpty(t, meta.Description, "builtin %q must set a non-empty Description", name)
		// Match the rendered table row (name, ≥2 spaces of column padding,
		// then description) so missing rows are caught even when descriptions
		// repeat across builtins (e.g. "[" and "test" both describe
		// "evaluate conditional expression").
		rowRe := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(name) + `\s+` + regexp.QuoteMeta(meta.Description) + `$`)
		assert.Regexp(t, rowRe, stdout, "help output should render row for %q", name)
	}
}

func TestHelpListsSorted(t *testing.T) {
	stdout, _, code := runScript(t, "help", "", interpoption.AllowAllCommands().(interp.RunnerOption))
	assert.Equal(t, 0, code)

	// Extract command names from the first column of the Commands table.
	var names []string
	for _, line := range tableLines(stdout, "Commands") {
		fields := strings.Fields(line)
		if len(fields) > 0 {
			names = append(names, fields[0])
		}
	}

	// Verify sorted order.
	for i := 1; i < len(names); i++ {
		assert.True(t, names[i-1] <= names[i],
			"commands should be sorted, but %q > %q", names[i-1], names[i])
	}
}

func TestHelpIncludesDescriptions(t *testing.T) {
	stdout, _, code := runScript(t, "help", "", interpoption.AllowAllCommands().(interp.RunnerOption))
	assert.Equal(t, 0, code)

	// Spot-check a few descriptions.
	assert.Contains(t, stdout, "concatenate and print files")
	assert.Contains(t, stdout, "write arguments to stdout")
	assert.Contains(t, stdout, "display help for features and commands")
	assert.Contains(t, stdout, "list directory contents")
	assert.Contains(t, stdout, "Assignments, expansion, inline env")
}

func TestHelpIncludesFooterHint(t *testing.T) {
	stdout, _, code := runScript(t, "help", "", interpoption.AllowAllCommands().(interp.RunnerOption))
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Run 'help <feature|command>' for more information on a specific topic.")
}

func TestHelpColumnsAligned(t *testing.T) {
	stdout, _, code := runScript(t, "help", "", interpoption.AllowAllCommands().(interp.RunnerOption))
	assert.Equal(t, 0, code)

	// The format is "%-*s  %s\n" — name padded to maxLen, two spaces, description.
	assertTableAligned(t, tableLines(stdout, "Features:"))
	assertTableAligned(t, tableLines(stdout, "Commands"))
}

func TestHelpListsFeaturesAndUnsupportedSummary(t *testing.T) {
	stdout, _, code := runScript(t, "help", "", interpoption.AllowAllCommands().(interp.RunnerOption))
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Features:")
	assert.Contains(t, stdout, "variables")
	assert.Contains(t, stdout, "control-flow")
	assert.Contains(t, stdout, "pipes-redirections")
	assert.Contains(t, stdout, "Not supported:")
	assert.Contains(t, stdout, "arithmetic $((...))")
	assert.Contains(t, stdout, "case, select")
	assert.Contains(t, stdout, "arbitrary output file redirects")
}

func TestHelpShowsFeatureHelp(t *testing.T) {
	stdout, stderr, code := runScript(t, "help variables", "", interpoption.AllowAllCommands().(interp.RunnerOption))
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Contains(t, stdout, "variables - Assignments")
	assert.Contains(t, stdout, "Supported:")
	assert.Contains(t, stdout, "VAR=value assignment")
	assert.Contains(t, stdout, "Not supported:")
	assert.Contains(t, stdout, "Arithmetic expansion: $(( expr )).")
}

func TestHelpShowsFeatureHelpWhenOnlyHelpAllowed(t *testing.T) {
	stdout, stderr, code := runScript(t, "help variables", "",
		interp.AllowedCommands([]string{"rshell:help"}))
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Contains(t, stdout, "variables - Assignments")
}

func TestUnsupportedIsSummaryNotTopic(t *testing.T) {
	_, stderr, code := runScript(t, "help unsupported", "", interpoption.AllowAllCommands().(interp.RunnerOption))
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "no help topics match 'unsupported'")
}

// --- Restricted commands ---

func TestHelpRestrictedShowsOnlyAllowedInTable(t *testing.T) {
	stdout, stderr, code := runScript(t, "help", "",
		interp.AllowedCommands([]string{"rshell:echo", "rshell:help"}))
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Contains(t, stdout, "echo")
	assert.Contains(t, stdout, "help")
	for _, line := range tableLines(stdout, "Commands") {
		fields := strings.Fields(line)
		if len(fields) > 0 {
			assert.True(t, fields[0] == "echo" || fields[0] == "help",
				"unexpected command in allowed table: %q", fields[0])
		}
	}
}

func TestHelpRestrictedShowsNotAllowedList(t *testing.T) {
	stdout, _, code := runScript(t, "help", "",
		interp.AllowedCommands([]string{"rshell:echo", "rshell:help"}))
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Disabled by policy (")
	assert.Contains(t, stdout, "cat")
	assert.Contains(t, stdout, "grep")
	assert.Contains(t, stdout, "ls")
}

func TestHelpDistinguishesRemediationRequirementFromPolicyDenial(t *testing.T) {
	allowed := []string{"rshell:echo", "rshell:help", "rshell:rm", "rshell:truncate"}
	stdout, _, code := runScript(t, "help", "", interp.AllowedCommands(allowed))
	assert.Equal(t, 0, code)

	available := sectionText(stdout, "Commands available now")
	requiresRemediation := sectionText(stdout, "Require remediation mode")
	disabledByPolicy := sectionText(stdout, "Disabled by policy")

	assert.Contains(t, available, "Commands available now (2):")
	assert.Regexp(t, `(?m)^echo\s+write arguments to stdout$`, available)
	assert.Regexp(t, `(?m)^help\s+display help for features and commands$`, available)

	assert.Contains(t, requiresRemediation, "Require remediation mode (2):")
	assert.Regexp(t, `(?m)^rm\s+remove files$`, requiresRemediation)
	assert.Regexp(t, `(?m)^truncate\s+shrink or extend file size$`, requiresRemediation)

	assert.Regexp(t, `\blogrotate\b`, disabledByPolicy)
	assert.Regexp(t, `\bsystemctl\b`, disabledByPolicy)
	assert.NotRegexp(t, `\brm\b`, disabledByPolicy)
	assert.NotRegexp(t, `\btruncate\b`, disabledByPolicy)
}

func TestHelpRemediationModeMakesAuthorizedRemediationCommandsAvailable(t *testing.T) {
	allowed := []string{"rshell:echo", "rshell:help", "rshell:rm", "rshell:truncate"}
	stdout, _, code := runScript(t, "help", "",
		interp.AllowedCommands(allowed),
		interp.WithMode(interp.ModeRemediation))
	assert.Equal(t, 0, code)

	available := sectionText(stdout, "Commands available now")
	disabledByPolicy := sectionText(stdout, "Disabled by policy")

	assert.Contains(t, available, "Commands available now (4):")
	assert.Regexp(t, `(?m)^rm\s+remove files$`, available)
	assert.Regexp(t, `(?m)^truncate\s+shrink or extend file size$`, available)
	assert.NotContains(t, stdout, "Require remediation mode")

	assert.Regexp(t, `\blogrotate\b`, disabledByPolicy)
	assert.Regexp(t, `\bsystemctl\b`, disabledByPolicy)
	assert.NotRegexp(t, `\brm\b`, disabledByPolicy)
	assert.NotRegexp(t, `\btruncate\b`, disabledByPolicy)
}

func TestHelpRestrictedSingleCommand(t *testing.T) {
	stdout, _, code := runScript(t, "help", "",
		interp.AllowedCommands([]string{"rshell:ls", "rshell:help"}))
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "help")
	assert.Contains(t, stdout, "ls")
}

func TestHelpRestrictedAlignmentAdjusts(t *testing.T) {
	// With "wc" (2-char), "strings" (7-char), and "help" (4-char),
	// the column width should match the longest allowed name.
	stdout, _, code := runScript(t, "help", "",
		interp.AllowedCommands([]string{"rshell:wc", "rshell:strings", "rshell:help"}))
	assert.Equal(t, 0, code)

	for _, line := range tableLines(stdout, "Commands") {
		// "strings" is the longest name (7 chars), so the description should
		// start at the same column for all lines.
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "wc" {
			// "wc" should be padded to the same width as "strings".
			assert.True(t, strings.HasPrefix(line, "wc       "),
				"short name should be padded, got: %q", line)
		}
	}
}

func TestHelpNotAllowedWhenNotInList(t *testing.T) {
	// help is not in the allowed list and should be blocked.
	_, stderr, code := runScript(t, "help", "",
		interp.AllowedCommands([]string{"rshell:echo", "rshell:ls"}))
	assert.Equal(t, 127, code)
	assert.Contains(t, stderr, "command not allowed")
}

func TestHelpAlwaysAvailableNoCommands(t *testing.T) {
	// Even with an empty allowed list, help should work.
	_, stderr, code := runScript(t, "help", "",
		interp.AllowedCommands([]string{}))
	assert.Equal(t, 127, code)
	assert.Contains(t, stderr, "command not allowed")
}

// --- --all flag ---

func TestHelpAllFlagShowsNotAllowedWithDescriptions(t *testing.T) {
	stdout, _, code := runScript(t, "help --all", "",
		interp.AllowedCommands([]string{"rshell:echo", "rshell:help"}))
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Disabled by policy (")
	// --all shows a full description table for commands disabled by policy.
	assert.Contains(t, stdout, "concatenate and print files")     // cat description
	assert.Contains(t, stdout, "print lines that match patterns") // grep description
}

func TestHelpAllFlagNoRestrictions(t *testing.T) {
	// When all commands are allowed in remediation mode, --all should not show
	// "Disabled by policy" but should confirm that all commands are allowed.
	stdout, _, code := runScript(t, "help --all", "", interpoption.AllowAllCommands().(interp.RunnerOption), interp.WithMode(interp.ModeRemediation))
	assert.Equal(t, 0, code)
	assert.NotContains(t, stdout, "Disabled by policy")
	assert.Contains(t, stdout, "All commands are allowed in this session.")
}

func TestHelpAllFlagStillShowsAllowed(t *testing.T) {
	stdout, _, code := runScript(t, "help --all", "",
		interp.AllowedCommands([]string{"rshell:echo", "rshell:help"}))
	assert.Equal(t, 0, code)
	// Allowed commands should still appear with descriptions.
	assert.Contains(t, stdout, "write arguments to stdout")
	assert.Contains(t, stdout, "display help for features and commands")
}

// --- Error handling ---

func TestHelpUnknownTopicShowsError(t *testing.T) {
	_, stderr, code := runScript(t, "help foo", "", interpoption.AllowAllCommands().(interp.RunnerOption))
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "no help topics match 'foo'")
}

func TestHelpShowsCommandHelp(t *testing.T) {
	stdout, _, code := runScript(t, "help echo", "", interpoption.AllowAllCommands().(interp.RunnerOption))
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "echo: echo [-neE]")
}

func TestHelpFlagPrintsUsage(t *testing.T) {
	stdout, _, code := runScript(t, "help --help", "", interpoption.AllowAllCommands().(interp.RunnerOption))
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Usage: help [--all] [feature|command]")
	assert.Contains(t, stdout, "Display help for rshell features and commands.")
}

func TestHelpUnknownFlagRejected(t *testing.T) {
	_, stderr, code := runScript(t, "help --verbose", "", interpoption.AllowAllCommands().(interp.RunnerOption))
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "help:")
}

// --- Pipeline / composition ---

func TestHelpInPipeline(t *testing.T) {
	stdout, _, code := runScript(t, "help | grep echo", "", interpoption.AllowAllCommands().(interp.RunnerOption))
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "echo")
	assert.Contains(t, stdout, "write arguments to stdout")
}

func TestHelpExitCodeInScript(t *testing.T) {
	stdout, _, code := runScript(t, "help; echo $?", "", interpoption.AllowAllCommands().(interp.RunnerOption))
	assert.Equal(t, 0, code)
	// The last line before the footer should be "0" from echo $?.
	assert.True(t, strings.HasSuffix(strings.TrimSpace(stdout), "0"))
}

func TestHelpFailExitCodeInScript(t *testing.T) {
	stdout, _, code := runScript(t, "help badarg; echo $?", "", interpoption.AllowAllCommands().(interp.RunnerOption))
	assert.Equal(t, 0, code) // overall script exits 0 because echo $? succeeds
	assert.True(t, strings.HasSuffix(strings.TrimSpace(stdout), "1"))
}

// --- Help lists itself ---

func TestHelpListsItself(t *testing.T) {
	stdout, _, code := runScript(t, "help", "", interpoption.AllowAllCommands().(interp.RunnerOption))
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "help")
	assert.Contains(t, stdout, "display help for features and commands")
}

// --- Empty stderr on success ---

func TestHelpNoStderrOnSuccess(t *testing.T) {
	_, stderr, code := runScript(t, "help", "", interpoption.AllowAllCommands().(interp.RunnerOption))
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
}

// --- Allowed paths section ---

func TestHelpListsConfiguredAllowedPaths(t *testing.T) {
	tmp := t.TempDir()
	stdout, _, code := runScript(t, "help", "",
		interpoption.AllowAllCommands().(interp.RunnerOption),
		interp.AllowedPaths([]string{tmp}))
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Allowed paths:")
	assert.Contains(t, stdout, "  Read-only:\n")
	assert.Contains(t, stdout, "\n    "+tmp+"\n")
	assert.Contains(t, stdout, "  Read-write:\n    (none)\n")
}

func TestHelpListsMultipleAllowedPathsLinePerLine(t *testing.T) {
	a := t.TempDir()
	b := t.TempDir()
	stdout, _, code := runScript(t, "help", "",
		interpoption.AllowAllCommands().(interp.RunnerOption),
		interp.AllowedPaths([]string{a, b}))
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Allowed paths:")
	assert.Contains(t, stdout, "\n    "+a+"\n")
	assert.Contains(t, stdout, "\n    "+b+"\n")
}

func TestHelpListsAllowedPathModes(t *testing.T) {
	readOnly := t.TempDir()
	readWrite := t.TempDir()
	stdout, _, code := runScript(t, "help", "",
		interpoption.AllowAllCommands().(interp.RunnerOption),
		interp.AllowedPaths([]string{readOnly, readWrite + ":rw"}))
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Allowed paths:")
	assert.Contains(t, stdout, "  Read-only:\n    "+readOnly+"\n")
	assert.Contains(t, stdout, "  Read-write:\n    "+readWrite+"\n")
	assert.Contains(t, stdout, "(write access requires remediation mode)")
}

func TestHelpOmitsReadWriteModeNoteInRemediationMode(t *testing.T) {
	readWrite := t.TempDir()
	stdout, _, code := runScript(t, "help", "",
		interpoption.AllowAllCommands().(interp.RunnerOption),
		interp.WithMode(interp.ModeRemediation),
		interp.AllowedPaths([]string{readWrite + ":rw"}))
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "  Read-write:\n    "+readWrite+"\n")
	assert.NotContains(t, stdout, "(write access requires remediation mode)")
}

func TestHelpEmptyAllowedPathsShowsBlockedNotice(t *testing.T) {
	stdout, _, code := runScript(t, "help", "",
		interpoption.AllowAllCommands().(interp.RunnerOption),
		interp.AllowedPaths(nil))
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Allowed paths:")
	assert.Contains(t, stdout, "(no allowed paths configured — no filesystem paths are reachable)")
}

// --- Elevatable commands section ---

func TestHelpListsSortedEffectiveElevatableCommands(t *testing.T) {
	stdout, _, code := runScript(t, "help", "",
		interp.AllowedCommands([]string{"rshell:help", "rshell:truncate", "rshell:echo"}),
		interp.WithMode(interp.ModeRemediation),
		interp.SelectiveElevation([]string{"rshell:truncate", "rshell:cat", "rshell:echo"}, func(context.Context, string, func()) error {
			return nil
		}))
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Elevatable commands:\n  sudo echo\n  sudo truncate\n")
	assert.NotContains(t, stdout, "sudo cat")
}

// TestHelpElevatableCommandsExcludesRemediationOnlyOutsideRemediationMode
// covers the bug where printElevatableCommands listed a command as
// elevatable purely because it was in AllowedCommands + SelectiveElevation,
// without checking whether it was actually in "Commands available now" for
// the current mode. truncate is RemediationOnly, so in read-only mode it
// must not be listed as elevatable even though it is allowed and
// selectively elevatable.
func TestHelpElevatableCommandsExcludesRemediationOnlyOutsideRemediationMode(t *testing.T) {
	stdout, _, code := runScript(t, "help", "",
		interp.AllowedCommands([]string{"rshell:help", "rshell:truncate", "rshell:echo"}),
		interp.SelectiveElevation([]string{"rshell:truncate", "rshell:cat", "rshell:echo"}, func(context.Context, string, func()) error {
			return nil
		}))
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Elevatable commands:\n  sudo echo\n")
	assert.NotContains(t, stdout, "sudo truncate")
	assert.NotContains(t, stdout, "sudo cat")
}

func TestHelpShowsEmptyElevatableCommandsWhenAllRequireRemediation(t *testing.T) {
	for _, script := range []string{"help", "help --all"} {
		t.Run(script, func(t *testing.T) {
			stdout, stderr, code := runScript(t, script, "",
				interp.AllowedCommands([]string{"rshell:help", "rshell:truncate"}),
				interp.SelectiveElevation([]string{"rshell:truncate"}, func(context.Context, string, func()) error {
					t.Fatal("help must not invoke elevation")
					return nil
				}))
			assert.Equal(t, 0, code)
			assert.Empty(t, stderr)
			assert.Equal(t, 1, strings.Count(stdout, "Elevatable commands:"))
			assert.Equal(t, "Elevatable commands:\n  (no effective elevatable commands — sudo is unavailable)",
				sectionText(stdout, "Elevatable commands:"))
		})
	}
}

func TestHelpShowsEmptyElevatableCommandsWhenAllDeniedByPolicy(t *testing.T) {
	for _, script := range []string{"help", "help --all"} {
		t.Run(script, func(t *testing.T) {
			stdout, stderr, code := runScript(t, script, "",
				interp.AllowedCommands([]string{"rshell:help"}),
				interp.SelectiveElevation([]string{"rshell:cat"}, func(context.Context, string, func()) error {
					t.Fatal("help must not invoke elevation")
					return nil
				}))
			assert.Equal(t, 0, code)
			assert.Empty(t, stderr)
			assert.Equal(t, "Elevatable commands:\n  (no effective elevatable commands — sudo is unavailable)",
				sectionText(stdout, "Elevatable commands:"))
		})
	}
}

// --- Allowed systemd units section ---

func TestHelpListsConfiguredAllowedSystemdUnits(t *testing.T) {
	stdout, _, code := runScript(t, "help", "",
		interpoption.AllowAllCommands().(interp.RunnerOption),
		interp.AllowedSystemServices([]interp.SystemdControlGrant{
			{
				Service: "all.service",
				Actions: []interp.SystemServiceAction{interp.SystemServiceAllActions},
			},
			{
				Service: "worker.service",
				Actions: []interp.SystemServiceAction{
					interp.SystemServiceEnable,
					interp.SystemServiceClean,
					interp.SystemServiceRestart,
				},
			},
			{
				Service: "api.socket",
				Actions: []interp.SystemServiceAction{
					interp.SystemServiceStop,
					interp.SystemServiceRead,
					interp.SystemServiceStop,
				},
			},
			{
				Service: "nightly.timer",
				Actions: []interp.SystemServiceAction{interp.SystemServiceStart},
			},
		}))
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout,
		"Allowed systemd units:\n"+
			"  all.service:read+clean+start+stop+reload+restart+enable+disable\n"+
			"  api.socket:read+stop\n"+
			"  nightly.timer:start\n"+
			"  worker.service:clean+restart+enable\n"+
			"  (systemctl requires remediation mode; non-read actions are inactive in read-only mode)\n")
	assert.NotContains(t, stdout, "all.service:*")
}

func TestHelpEmptyAllowedSystemdUnitsShowsBlockedNotice(t *testing.T) {
	stdout, _, code := runScript(t, "help", "",
		interpoption.AllowAllCommands().(interp.RunnerOption))
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Allowed systemd units:\n")
	assert.Contains(t, stdout, "(no effective systemd unit grants — all systemd operations are blocked)")
}

func TestHelpOmitsSystemdModeNoteInRemediationMode(t *testing.T) {
	stdout, _, code := runScript(t, "help", "",
		interpoption.AllowAllCommands().(interp.RunnerOption),
		interp.WithMode(interp.ModeRemediation),
		interp.AllowedSystemServices([]interp.SystemdControlGrant{
			{
				Service: "worker.service",
				Actions: []interp.SystemServiceAction{interp.SystemServiceRestart},
			},
		}))
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "  worker.service:restart\n")
	assert.NotContains(t, stdout, "non-read actions are inactive in read-only mode")
}

// --- Invariant: Help field only on NoFlags commands ---

func TestHelpFieldOnlyOnNoFlagsCommands(t *testing.T) {
	// Trigger registration so Names()/Meta() are populated.
	_, _ = interp.New()

	for _, name := range builtins.Names() {
		meta, ok := builtins.Meta(name)
		require.True(t, ok)
		if meta.Help != "" && meta.HasFlags {
			t.Errorf("%s: Help field must not be set on commands that register flags — "+
				"use --help instead so that 'help %s' and '%s --help' produce the same output",
				name, name, name)
		}
	}
}
