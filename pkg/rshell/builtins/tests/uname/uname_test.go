// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package uname_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/pkg/rshell/internal/interpoption"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// writeFakeProc creates a fake /proc/sys/kernel/ tree in dir.
func writeFakeProc(t *testing.T, dir string, vals map[string]string) {
	t.Helper()
	kernelDir := filepath.Join(dir, "sys", "kernel")
	require.NoError(t, os.MkdirAll(kernelDir, 0755))
	for name, val := range vals {
		require.NoError(t, os.WriteFile(filepath.Join(kernelDir, name), []byte(val+"\n"), 0644))
	}
}

// defaultFakeProc returns a standard set of fake proc values.
func defaultFakeProc() map[string]string {
	return map[string]string{
		"ostype":    "Linux",
		"hostname":  "testhost",
		"osrelease": "5.15.0-test",
		"version":   "#1 SMP Test",
		"arch":      "x86_64",
	}
}

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
	allOpts := append([]interp.RunnerOption{
		interp.StdIO(nil, &outBuf, &errBuf),
		interpoption.AllowAllCommands().(interp.RunnerOption),
	}, opts...)
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
		} else if ctx.Err() != nil {
			exitCode = 1 // Context cancelled/timed out.
		} else {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

func cmdRun(t *testing.T, script, procDir string) (stdout, stderr string, exitCode int) {
	t.Helper()
	return runScript(t, script, procDir, interp.ProcPath(procDir))
}

func requireLinux(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("uname reads from /proc; skipping on " + runtime.GOOS)
	}
}

// --- Tests ---

func TestUnameDefault(t *testing.T) {
	requireLinux(t)
	dir := t.TempDir()
	writeFakeProc(t, dir, defaultFakeProc())
	stdout, _, code := cmdRun(t, "uname", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "Linux\n", stdout)
}

func TestUnameS(t *testing.T) {
	requireLinux(t)
	dir := t.TempDir()
	writeFakeProc(t, dir, defaultFakeProc())
	stdout, _, code := cmdRun(t, "uname -s", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "Linux\n", stdout)
}

func TestUnameN(t *testing.T) {
	requireLinux(t)
	dir := t.TempDir()
	writeFakeProc(t, dir, defaultFakeProc())
	stdout, _, code := cmdRun(t, "uname -n", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "testhost\n", stdout)
}

func TestUnameR(t *testing.T) {
	requireLinux(t)
	dir := t.TempDir()
	writeFakeProc(t, dir, defaultFakeProc())
	stdout, _, code := cmdRun(t, "uname -r", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "5.15.0-test\n", stdout)
}

func TestUnameV(t *testing.T) {
	requireLinux(t)
	dir := t.TempDir()
	writeFakeProc(t, dir, defaultFakeProc())
	stdout, _, code := cmdRun(t, "uname -v", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "#1 SMP Test\n", stdout)
}

func TestUnameM(t *testing.T) {
	requireLinux(t)
	dir := t.TempDir()
	writeFakeProc(t, dir, defaultFakeProc())
	stdout, _, code := cmdRun(t, "uname -m", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "x86_64\n", stdout)
}

func TestUnameA(t *testing.T) {
	requireLinux(t)
	dir := t.TempDir()
	writeFakeProc(t, dir, defaultFakeProc())
	stdout, _, code := cmdRun(t, "uname -a", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "Linux testhost 5.15.0-test #1 SMP Test x86_64\n", stdout)
}

func TestUnameCombinedFlags(t *testing.T) {
	requireLinux(t)
	dir := t.TempDir()
	writeFakeProc(t, dir, defaultFakeProc())
	stdout, _, code := cmdRun(t, "uname -sn", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "Linux testhost\n", stdout)
}

func TestUnameCombinedFlagsMR(t *testing.T) {
	requireLinux(t)
	dir := t.TempDir()
	writeFakeProc(t, dir, defaultFakeProc())
	stdout, _, code := cmdRun(t, "uname -mr", dir)
	assert.Equal(t, 0, code)
	// Output order follows POSIX: s, n, r, v, m — so -mr gives "release machine"
	assert.Equal(t, "5.15.0-test x86_64\n", stdout)
}

func TestUnameUnknownFlag(t *testing.T) {
	requireLinux(t)
	dir := t.TempDir()
	writeFakeProc(t, dir, defaultFakeProc())
	_, stderr, code := cmdRun(t, "uname -z", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "uname:")
}

func TestUnameMissingProcFile(t *testing.T) {
	requireLinux(t)
	dir := t.TempDir()
	// Don't create any proc files — all reads should fail.
	_, stderr, code := cmdRun(t, "uname", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "uname: cannot read")
}

func TestUnameCustomProcPath(t *testing.T) {
	requireLinux(t)
	dir := t.TempDir()
	customProc := filepath.Join(dir, "host", "proc")
	writeFakeProc(t, customProc, map[string]string{
		"ostype":    "Linux",
		"hostname":  "container-host",
		"osrelease": "6.1.0-custom",
		"version":   "#42 SMP Custom",
		"arch":      "aarch64",
	})
	stdout, _, code := runScript(t, "uname -a", dir, interp.ProcPath(customProc))
	assert.Equal(t, 0, code)
	assert.Equal(t, "Linux container-host 6.1.0-custom #42 SMP Custom aarch64\n", stdout)
}

func TestUnameHelp(t *testing.T) {
	dir := t.TempDir()
	writeFakeProc(t, dir, defaultFakeProc())
	stdout, _, code := cmdRun(t, "uname --help", dir)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "uname")
}

func TestUnameNoProcFiles(t *testing.T) {
	requireLinux(t)
	// Point proc path at an empty directory — no kernel files exist.
	dir := t.TempDir()
	emptyProc := filepath.Join(dir, "empty_proc")
	require.NoError(t, os.MkdirAll(emptyProc, 0755))
	_, stderr, code := runScript(t, "uname", dir, interp.ProcPath(emptyProc))
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "uname: cannot read")
}

func TestUnameNonLinuxPlatform(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("this test verifies non-Linux behavior")
	}
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "uname", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "not supported")
}

func TestUnameDuplicateFlags(t *testing.T) {
	requireLinux(t)
	dir := t.TempDir()
	writeFakeProc(t, dir, defaultFakeProc())
	// -ss should print kernel name once, not twice.
	stdout, _, code := cmdRun(t, "uname -ss", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "Linux\n", stdout)
}

func TestUnameAllFlagsExplicit(t *testing.T) {
	requireLinux(t)
	dir := t.TempDir()
	writeFakeProc(t, dir, defaultFakeProc())
	// -snrvm should produce the same output as -a.
	stdout, _, code := cmdRun(t, "uname -snrvm", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "Linux testhost 5.15.0-test #1 SMP Test x86_64\n", stdout)
}

func TestUnameFlagOrderDoesntMatter(t *testing.T) {
	requireLinux(t)
	dir := t.TempDir()
	writeFakeProc(t, dir, defaultFakeProc())
	// -mrvns (reverse order) should still output in POSIX order: s,n,r,v,m.
	stdout, _, code := cmdRun(t, "uname -mrvns", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "Linux testhost 5.15.0-test #1 SMP Test x86_64\n", stdout)
}

func TestUnameAllOverridesIndividual(t *testing.T) {
	requireLinux(t)
	dir := t.TempDir()
	writeFakeProc(t, dir, defaultFakeProc())
	// -as should produce the same output as -a.
	stdout, _, code := cmdRun(t, "uname -as", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "Linux testhost 5.15.0-test #1 SMP Test x86_64\n", stdout)
}

func TestUnamePartialProcTreeSuccess(t *testing.T) {
	requireLinux(t)
	dir := t.TempDir()
	// Only create ostype — requesting -s should succeed.
	writeFakeProc(t, dir, map[string]string{"ostype": "Linux"})
	stdout, _, code := cmdRun(t, "uname -s", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "Linux\n", stdout)
}

func TestUnamePartialProcTreeFailure(t *testing.T) {
	requireLinux(t)
	dir := t.TempDir()
	// Only create ostype — requesting -n should fail (hostname missing).
	writeFakeProc(t, dir, map[string]string{"ostype": "Linux"})
	_, stderr, code := cmdRun(t, "uname -n", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "uname: cannot read hostname")
}

func TestUnameWhitespaceInProcValues(t *testing.T) {
	requireLinux(t)
	dir := t.TempDir()
	writeFakeProc(t, dir, map[string]string{
		"ostype":    "Linux",
		"hostname":  "myhost  \t",
		"osrelease": "5.15.0",
		"version":   "#1 SMP",
		"arch":      "x86_64",
	})
	stdout, _, code := cmdRun(t, "uname -n", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "myhost\n", stdout, "trailing whitespace should be trimmed")
}

func TestUnameEmptyProcFile(t *testing.T) {
	requireLinux(t)
	dir := t.TempDir()
	// ostype exists but writeFakeProc adds "\n" — write truly empty file.
	kernelDir := filepath.Join(dir, "sys", "kernel")
	require.NoError(t, os.MkdirAll(kernelDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(kernelDir, "ostype"), []byte(""), 0644))
	stdout, _, code := cmdRun(t, "uname -s", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\n", stdout, "empty proc file should produce empty field")
}

func TestUnamePipeIntegration(t *testing.T) {
	requireLinux(t)
	dir := t.TempDir()
	writeFakeProc(t, dir, defaultFakeProc())
	stdout, _, code := cmdRun(t, "uname -s | cat", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "Linux\n", stdout)
}

func TestUnameVariableCapture(t *testing.T) {
	requireLinux(t)
	dir := t.TempDir()
	writeFakeProc(t, dir, defaultFakeProc())
	stdout, _, code := cmdRun(t, `x=$(uname -s); echo "$x"`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "Linux\n", stdout)
}

func TestUnameContextCancellation(t *testing.T) {
	requireLinux(t)
	dir := t.TempDir()
	writeFakeProc(t, dir, defaultFakeProc())
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.
	_, _, code := runScriptCtx(ctx, t, "uname -a", dir, interp.ProcPath(dir))
	assert.NotEqual(t, 0, code, "cancelled context should result in non-zero exit")
}

func TestUnameHelpShortFlag(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := cmdRun(t, "uname -h", dir)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "uname")
	assert.Empty(t, stderr, "help should not write to stderr")
}

func TestUnameHelpStderrEmpty(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "uname --help", dir)
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
}

func TestUnameExtraOperandRejected(t *testing.T) {
	dir := t.TempDir()
	writeFakeProc(t, dir, defaultFakeProc())
	// GNU uname rejects extra operands.
	_, stderr, code := cmdRun(t, "uname foo", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "uname: extra operand")
}
