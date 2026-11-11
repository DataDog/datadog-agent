// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package ps_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/pkg/rshell/internal/interpoption"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

func runScriptWithProcPath(t *testing.T, script, procPath string) (stdout, stderr string, code int) {
	t.Helper()
	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader(script), "")
	if err != nil {
		t.Fatal(err)
	}
	var outBuf, errBuf bytes.Buffer
	runner, err := interp.New(
		interp.StdIO(nil, &outBuf, &errBuf),
		interpoption.AllowAllCommands().(interp.RunnerOption),
		interp.ProcPath(procPath),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer runner.Close()
	runErr := runner.Run(context.Background(), prog)
	exitCode := 0
	if runErr != nil {
		var es interp.ExitStatus
		if errors.As(runErr, &es) {
			exitCode = int(es)
		} else {
			t.Fatalf("unexpected runner error: %v", runErr)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// TestProcPathNonexistentDirErrors ensures ps -e fails gracefully when the
// configured proc path does not exist.
func TestProcPathNonexistentDirErrors(t *testing.T) {
	_, stderr, code := runScriptWithProcPath(t, "ps -e", "/nonexistent/proc/path")
	if code != 1 {
		t.Errorf("expected exit code 1 for nonexistent proc path, got %d", code)
	}
	if !strings.Contains(stderr, "ps:") {
		t.Errorf("expected error message in stderr, got: %s", stderr)
	}
	require.NotContains(t, stderr, "ps: ps:")
}

// TestProcPathNonexistentDirErrorsByPID ensures ps -p fails gracefully when
// the configured proc path does not exist.
func TestProcPathNonexistentDirErrorsByPID(t *testing.T) {
	_, stderr, code := runScriptWithProcPath(t, "ps -p 1", "/nonexistent/proc/path")
	if code != 1 {
		t.Errorf("expected exit code 1 for nonexistent proc path, got %d", code)
	}
	if !strings.Contains(stderr, "ps:") {
		t.Errorf("expected error message in stderr, got: %s", stderr)
	}
}

// TestProcPathNotADirErrors_ListAll ensures ps -e fails with a clear error
// when the configured proc path exists but is a regular file, not a directory.
func TestProcPathNotADirErrors_ListAll(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "not-a-dir")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	_, stderr, code := runScriptWithProcPath(t, "ps -e", f.Name())
	if code != 1 {
		t.Errorf("expected exit code 1 for file proc path, got %d", code)
	}
	if !strings.Contains(stderr, "ps:") {
		t.Errorf("expected ps: error in stderr, got: %s", stderr)
	}
}

// TestProcPathNotADirErrors_ByPID ensures ps -p fails with a clear error
// when the configured proc path exists but is a regular file, not a directory.
func TestProcPathNotADirErrors_ByPID(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "not-a-dir")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	_, stderr, code := runScriptWithProcPath(t, "ps -p 1", f.Name())
	if code != 1 {
		t.Errorf("expected exit code 1 for file proc path, got %d", code)
	}
	if !strings.Contains(stderr, "ps:") {
		t.Errorf("expected ps: error in stderr, got: %s", stderr)
	}
	if !strings.Contains(stderr, "not a directory") {
		t.Errorf("expected 'not a directory' in stderr, got: %s", stderr)
	}
}

// TestProcPathEmptyUsesDefault ensures an empty ProcPath falls back to /proc
// and ps -e runs successfully.
func TestProcPathEmptyUsesDefault(t *testing.T) {
	_, stderr, code := runScriptWithProcPath(t, "ps -e", "")
	if code != 0 {
		t.Fatalf("ps -e with empty ProcPath exited %d; stderr: %s", code, stderr)
	}
}

// writeFakeProc creates a minimal fake /proc filesystem under dir and returns
// the procPath. It writes a single fake process with the given pid and name.
func writeFakeProc(t *testing.T, pid int, name string) string {
	return writeFakeProcWithCmdline(t, pid, name, []byte(name+"\x00"))
}

// writeFakeProcWithCmdline creates a minimal fake /proc filesystem under dir.
// If cmdline is nil, <procPath>/<pid>/cmdline is left absent.
func writeFakeProcWithCmdline(t *testing.T, pid int, name string, cmdline []byte) string {
	t.Helper()
	dir := t.TempDir()

	// Write <procPath>/stat for boot time.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stat"), []byte("cpu 0 0 0 0\nbtime 1000000000\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "uptime"), []byte("1000.00 0.00\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "meminfo"), []byte("MemTotal: 1048576 kB\n"), 0o644))

	// Create the PID subdirectory using the provided pid.
	pidDir := filepath.Join(dir, strconv.Itoa(pid))
	require.NoError(t, os.MkdirAll(pidDir, 0o755))

	// Write <procPath>/<pid>/stat.
	// Format: pid (comm) state ppid pgroup session tty_nr tpgid flags minflt
	//         cminflt majflt cmajflt utime stime cutime cstime priority nice
	//         numthreads itrealvalue starttime ...
	// Fields after (comm): at least 20 required by readProc.
	statContent := fmt.Sprintf("%d (%s) S 0 %d %d 0 -1 4194560 0 0 0 0 200 100 0 0 20 0 1 0 100 8388608 256\n", pid, name, pid, pid)
	require.NoError(t, os.WriteFile(filepath.Join(pidDir, "stat"), []byte(statContent), 0o644))

	// Write <procPath>/<pid>/status for UID lookup.
	require.NoError(t, os.WriteFile(filepath.Join(pidDir, "status"), []byte("Name:\t"+name+"\nUid:\t1000 1000 1000 1000\n"), 0o644))

	if cmdline != nil {
		require.NoError(t, os.WriteFile(filepath.Join(pidDir, "cmdline"), cmdline, 0o644))
	}

	return dir
}

// TestProcPathFakeProc ensures ps -e reads from the custom proc path and
// returns entries from the fake proc tree rather than the real /proc.
func TestProcPathFakeProc(t *testing.T) {
	procPath := writeFakeProc(t, 1, "fakeinit")

	stdout, stderr, code := runScriptWithProcPath(t, "ps -e", procPath)
	if code != 0 {
		t.Fatalf("ps -e with fake proc path exited %d; stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "fakeinit") {
		t.Errorf("expected 'fakeinit' in ps output; got:\n%s", stdout)
	}
	// The output should not contain real system processes.
	if strings.Count(stdout, "\n") > 5 {
		t.Errorf("expected only fake proc entries, but got many lines:\n%s", stdout)
	}
}

// TestProcPathFakeProcFullFormat ensures ps -ef also reads from the custom
// proc path and includes UID and PPID columns.
func TestProcPathFakeProcFullFormat(t *testing.T) {
	procPath := writeFakeProc(t, 1, "fakeinit")

	stdout, stderr, code := runScriptWithProcPath(t, "ps -ef", procPath)
	if code != 0 {
		t.Fatalf("ps -ef with fake proc path exited %d; stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "fakeinit") {
		t.Errorf("expected 'fakeinit' in ps -ef output; got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "PPID") {
		t.Errorf("expected PPID column in ps -ef output; got:\n%s", stdout)
	}
}

// TestProcPathFakeProcByPID ensures ps -p also reads from the custom proc path.
func TestProcPathFakeProcByPID(t *testing.T) {
	procPath := writeFakeProc(t, 1, "fakeinit")

	stdout, stderr, code := runScriptWithProcPath(t, "ps -p 1", procPath)
	if code != 0 {
		t.Fatalf("ps -p 1 with fake proc path exited %d; stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "1") {
		t.Errorf("expected PID 1 in output; got:\n%s", stdout)
	}
}

func TestProcPathFakeProcResourceFields(t *testing.T) {
	procPath := writeFakeProc(t, 1, "fakeinit")

	stdout, stderr, code := runScriptWithProcPath(
		t,
		"ps -e -o pid,ppid,pcpu,pmem,rss,vsz,etime,comm",
		procPath,
	)
	require.Equalf(t, 0, code, "stderr: %s", stderr)

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	require.Len(t, lines, 2)
	require.Equal(t,
		[]string{"PID", "PPID", "%CPU", "%MEM", "RSS", "VSZ", "ELAPSED", "COMMAND"},
		strings.Fields(lines[0]),
	)
	require.Equal(t,
		[]string{
			"1",
			"0",
			"0.3",
			fmt.Sprintf("%.1f", 100*float64(256*os.Getpagesize()/1024)/1_048_576),
			strconv.Itoa(256 * os.Getpagesize() / 1024),
			"8192",
			"16:39",
			"fakeinit",
		},
		strings.Fields(lines[1]),
	)
}

func TestProcPathCustomSortUsesRawRSSAndUnavailableLast(t *testing.T) {
	procPath := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(procPath, "stat"), []byte(
		"cpu 0 0 0 0\nbtime 1000000000\n",
	), 0o644))

	writeEntry := func(pid int, name, rssPages string) {
		pidDir := filepath.Join(procPath, strconv.Itoa(pid))
		require.NoError(t, os.Mkdir(pidDir, 0o755))
		stat := fmt.Sprintf(
			"%d (%s) S 0 %d %d 0 -1 0 0 0 0 0 0 0 0 0 20 0 1 0 100 8388608 %s\n",
			pid,
			name,
			pid,
			pid,
			rssPages,
		)
		require.NoError(t, os.WriteFile(filepath.Join(pidDir, "stat"), []byte(stat), 0o644))
		require.NoError(t, os.WriteFile(
			filepath.Join(pidDir, "status"),
			[]byte("Name:\t"+name+"\nUid:\t1000 1000 1000 1000\n"),
			0o644,
		))
	}
	writeEntry(2, "high", "512")
	writeEntry(3, "tie-low-pid", "256")
	writeEntry(10, "tie-high-pid", "256")
	writeEntry(4, "unavailable", "not-a-number")

	stdout, stderr, code := runScriptWithProcPath(
		t,
		"ps -e -o pid,comm --sort=-rss,+pid",
		procPath,
	)
	require.Equalf(t, 0, code, "stderr: %s", stderr)
	require.Equal(t,
		"PID COMMAND\n"+
			"  2 high\n"+
			"  3 tie-low-pid\n"+
			" 10 tie-high-pid\n"+
			"  4 unavailable\n",
		stdout,
	)
}

func TestProcPathFullFormatMarksUnavailableCPU(t *testing.T) {
	procPath := writeFakeProc(t, 1, "fakeinit")
	require.NoError(t, os.Remove(filepath.Join(procPath, "uptime")))

	stdout, stderr, code := runScriptWithProcPath(t, "ps -ef", procPath)
	require.Equalf(t, 0, code, "stderr: %s", stderr)

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	require.Len(t, lines, 2)
	require.Equal(t, "-", strings.Fields(lines[1])[3])
}

// TestProcPathCmdlineArgvNotLeaked ensures ps never exposes argv from
// <procPath>/<pid>/cmdline, even when the proc entry contains sensitive flags.
func TestProcPathCmdlineArgvNotLeaked(t *testing.T) {
	const (
		name   = "safeproc"
		secret = "rshell-secret-argv-leak"
	)
	procPath := writeFakeProcWithCmdline(t, 1, name, []byte(name+"\x00--token="+secret+"\x00--password=hunter2\x00"))

	for _, script := range []string{"ps -e", "ps -A", "ps -ef", "ps -p 1", "ps -f -p 1"} {
		t.Run(script, func(t *testing.T) {
			stdout, stderr, code := runScriptWithProcPath(t, script, procPath)
			require.Equalf(t, 0, code, "%s stderr: %s", script, stderr)
			require.Contains(t, stdout, name)
			require.NotContains(t, stdout, secret)
			require.NotContains(t, stdout, "--token")
			require.NotContains(t, stdout, "--password")
		})
	}
}

// TestProcPathBarePsCmdlineArgvNotLeaked covers the default bare "ps" path,
// which filters through the current process session instead of listing all
// processes or selecting explicit PIDs.
func TestProcPathBarePsCmdlineArgvNotLeaked(t *testing.T) {
	const (
		name   = "sessionproc"
		secret = "rshell-secret-bare-ps"
	)
	pid := os.Getpid()
	procPath := writeFakeProcWithCmdline(t, pid, name, []byte(name+"\x00--token="+secret+"\x00--password=hunter2\x00"))

	stdout, stderr, code := runScriptWithProcPath(t, "ps", procPath)
	require.Equalf(t, 0, code, "stderr: %s", stderr)
	require.Contains(t, stdout, strconv.Itoa(pid))
	require.Contains(t, stdout, name)
	require.NotContains(t, stdout, secret)
	require.NotContains(t, stdout, "--token")
	require.NotContains(t, stdout, "--password")
}

// TestProcPathMissingCmdlineStillUsesUnbracketedComm verifies CMD comes from
// stat comm directly and does not depend on a readable cmdline file.
func TestProcPathMissingCmdlineStillUsesUnbracketedComm(t *testing.T) {
	procPath := writeFakeProcWithCmdline(t, 1, "fakeworker", nil)

	stdout, stderr, code := runScriptWithProcPath(t, "ps -p 1", procPath)
	require.Equalf(t, 0, code, "stderr: %s", stderr)
	require.Contains(t, stdout, "fakeworker")
	require.NotContains(t, stdout, "[fakeworker]")
}

func TestProcPathUnsafeCommCannotForgeOutputRows(t *testing.T) {
	procPath := writeFakeProc(t, 1, "placeholder")
	statPath := filepath.Join(procPath, "1", "stat")
	data, err := os.ReadFile(statPath)
	require.NoError(t, err)
	data = []byte(strings.Replace(
		string(data),
		"(placeholder)",
		"(safe)name\nline\t\x1b[31m\x7f)",
		1,
	))
	require.NoError(t, os.WriteFile(statPath, data, 0o644))

	stdout, stderr, code := runScriptWithProcPath(t, "ps -p 1 -o pid,comm", procPath)
	require.Equalf(t, 0, code, "stderr: %s", stderr)
	require.Equal(t, "PID COMMAND\n  1 safe)name?line??[31m?\n", stdout)
}

// TestProcPathFakeProcSession ensures bare ps (no flags) reads from the custom
// proc path via GetSession rather than the real /proc.
func TestProcPathFakeProcSession(t *testing.T) {
	procPath := writeFakeProc(t, 1, "fakeinit")
	// Bare ps uses GetSession. Our process's PID is not present in the fake
	// proc tree (which only has PID 1), so the session walk returns no
	// ancestors and the SID check finds no matches. The output must contain
	// only the header line — if real /proc were read, many additional lines
	// would appear.
	stdout, stderr, code := runScriptWithProcPath(t, "ps", procPath)
	if code != 0 {
		t.Fatalf("ps with fake proc path exited %d; stderr: %s", code, stderr)
	}
	// Only the header line should be present (session is empty for the fake proc).
	lineCount := strings.Count(stdout, "\n")
	if lineCount > 1 {
		t.Errorf("expected only header line (real /proc must not be read), got %d lines:\n%s", lineCount, stdout)
	}
}
