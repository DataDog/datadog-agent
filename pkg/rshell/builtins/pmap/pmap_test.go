// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package pmap_test

import (
	"os"
	"runtime"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

func runScript(t *testing.T, script string) (string, string, int) {
	t.Helper()
	return testutil.RunScript(t, script, "")
}

// --- Help / usage ---

func TestPmapHelp(t *testing.T) {
	stdout, stderr, code := runScript(t, "pmap --help")
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Contains(t, stdout, "Usage: pmap")
	assert.Contains(t, stdout, "--extended")
	assert.Contains(t, stdout, "--help")
}

func TestPmapHelpShortFlag(t *testing.T) {
	stdout, stderr, code := runScript(t, "pmap -h")
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Contains(t, stdout, "Usage: pmap")
}

func TestPmapHelpShortCircuitsBeforeOtherErrors(t *testing.T) {
	stdout, stderr, code := runScript(t, "pmap --help --bogus")
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Contains(t, stdout, "Usage: pmap")
}

// --- Argument validation (platform-independent) ---

func TestPmapNoPID(t *testing.T) {
	stdout, stderr, code := runScript(t, "pmap")
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Equal(t, "pmap: no process ID specified\nTry 'pmap --help' for more information.\n", stderr)
}

func TestPmapInvalidPID(t *testing.T) {
	stdout, stderr, code := runScript(t, "pmap notapid")
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Equal(t, "pmap: invalid PID: notapid\n", stderr)
}

func TestPmapZeroPID(t *testing.T) {
	_, stderr, code := runScript(t, "pmap 0")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "invalid PID: 0")
}

func TestPmapNegativePID(t *testing.T) {
	_, stderr, code := runScript(t, "pmap -- -1")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "invalid PID: -1")
}

func TestPmapUnknownFlag(t *testing.T) {
	_, stderr, code := runScript(t, "pmap --bogus")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "pmap:")
}

func TestPmapRejectedFlags(t *testing.T) {
	for _, flag := range []string{
		"-p", "--show-path",
		"-d", "--device",
		"-q", "--quiet",
		"-A", "--range",
		"-k", "--use-kernel-name",
		"-c", "--read-rc",
		"-C", "--read-rc-from",
		"-n", "--create-rc",
		"-N", "--create-rc-to",
		"-X", "-XX",
		"-V", "--version",
	} {
		t.Run(flag, func(t *testing.T) {
			_, stderr, code := runScript(t, "pmap "+flag+" 1")
			assert.Equal(t, 1, code)
			assert.Contains(t, stderr, "pmap:")
		})
	}
}

func TestPmapHelpRejectsExplicitValue(t *testing.T) {
	_, stderr, code := runScript(t, "pmap --help=true")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "doesn't allow an argument")
}

func TestPmapExtendedRejectsExplicitValue(t *testing.T) {
	_, stderr, code := runScript(t, "pmap --extended=true 1")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "doesn't allow an argument")
}

// --- Platform behavior ---

// TestPmapNotSupportedOffLinuxWindowsAndDarwin ensures the platforms without
// a procmaps backend fail closed with a clear message rather than
// fabricating output.
func TestPmapNotSupportedOffLinuxWindowsAndDarwin(t *testing.T) {
	if runtime.GOOS == "linux" || runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		t.Skip("pmap is supported on linux, windows, and darwin; see the happy-path tests")
	}
	selfPID := os.Getpid()
	stdout, stderr, code := runScript(t, "pmap "+strconv.Itoa(selfPID))
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Equal(t, "pmap: not supported on this platform\n", stderr)
}

// TestPmapLinuxHappyPath exercises the real /proc/<pid>/maps of the test
// process itself. It only checks structural output (header, total line,
// exit code) since exact address ranges are host/run-dependent — exact-value
// coverage lives in the procmaps package's own parser tests against fixed
// fixtures.
func TestPmapLinuxHappyPath(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("pmap /proc backend is linux-only")
	}
	selfPID := os.Getpid()
	stdout, stderr, code := runScript(t, "pmap "+strconv.Itoa(selfPID))
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Contains(t, stdout, strconv.Itoa(selfPID)+":")
	assert.Contains(t, stdout, "total")
}

func TestPmapLinuxExtendedHappyPath(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("pmap -x /proc/<pid>/smaps backend is linux-only")
	}
	selfPID := os.Getpid()
	stdout, stderr, code := runScript(t, "pmap -x "+strconv.Itoa(selfPID))
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Contains(t, stdout, "Address")
	assert.Contains(t, stdout, "RSS")
	assert.Contains(t, stdout, "Dirty")
	assert.Contains(t, stdout, "total kB")
}

func TestPmapLinuxMissingPID(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("pmap /proc backend is linux-only")
	}
	// PID 2147483647 (max int32) is extremely unlikely to exist.
	stdout, stderr, code := runScript(t, "pmap 2147483647")
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Equal(t, "pmap: 2147483647: no such process\n", stderr)
}

// TestPmapDoesNotTouchAllowedPaths verifies pmap works with an empty
// AllowedPaths sandbox — the /proc/<pid>/{comm,maps,smaps} reads are
// documented as exempt.
func TestPmapDoesNotTouchAllowedPaths(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("pmap /proc backend is linux-only")
	}
	selfPID := os.Getpid()
	stdout, stderr, code := testutil.RunScript(t, "pmap "+strconv.Itoa(selfPID), "", interp.AllowedPaths(nil))
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Contains(t, stdout, strconv.Itoa(selfPID)+":")
}

// TestPmapMultiplePIDs ensures multiple PID arguments are each reported.
func TestPmapMultiplePIDs(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("pmap /proc backend is linux-only")
	}
	selfPID := os.Getpid()
	stdout, stderr, code := runScript(t, "pmap "+strconv.Itoa(selfPID)+" 2147483647")
	assert.Equal(t, 1, code) // second PID does not exist
	assert.Contains(t, stdout, strconv.Itoa(selfPID)+":")
	assert.Contains(t, stderr, "2147483647: no such process")
}

func TestPmapWindowsHappyPath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("pmap VirtualQueryEx backend is windows-only")
	}
	selfPID := os.Getpid()
	stdout, stderr, code := runScript(t, "pmap "+strconv.Itoa(selfPID))
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Contains(t, stdout, strconv.Itoa(selfPID)+":")
	assert.Contains(t, stdout, "total")
}

func TestPmapWindowsExtendedNotSupported(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only: extended mode is unsupported on windows")
	}
	selfPID := os.Getpid()
	stdout, stderr, code := runScript(t, "pmap -x "+strconv.Itoa(selfPID))
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Equal(t, "pmap: -x is not supported on this platform\n", stderr)
}

func TestPmapDarwinHappyPath(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("pmap proc_pidinfo backend is darwin-only")
	}
	selfPID := os.Getpid()
	stdout, stderr, code := runScript(t, "pmap "+strconv.Itoa(selfPID))
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Contains(t, stdout, strconv.Itoa(selfPID)+":")
	assert.Contains(t, stdout, "total")
}

func TestPmapDarwinExtendedNotSupported(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only: extended mode is unsupported on darwin")
	}
	selfPID := os.Getpid()
	stdout, stderr, code := runScript(t, "pmap -x "+strconv.Itoa(selfPID))
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Equal(t, "pmap: -x is not supported on this platform\n", stderr)
}

func TestPmapDarwinMissingPID(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("pmap proc_pidinfo backend is darwin-only")
	}
	// PID 2147483647 (max int32) is extremely unlikely to exist.
	stdout, stderr, code := runScript(t, "pmap 2147483647")
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Equal(t, "pmap: 2147483647: no such process\n", stderr)
}
