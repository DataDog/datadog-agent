// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && test

package coredump

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	childDirEnv = "DD_TEST_COREDUMP_CRASH_CHILD_DIR"
	testName    = "test-binary"
	childPanic  = "crash report test panic"
)

// captureLogs sends the logs of the package logger to a buffer.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	logger, err := log.LoggerFromWriterWithMinLevelAndLvlFuncCtxMsgFormat(&buf, log.DebugLvl)
	require.NoError(t, err)
	log.SetupLogger(logger, "debug")
	return &buf
}

// resetCrashOutput stops sending the crash output of the test process to a
// file of the test.
func resetCrashOutput(t *testing.T) {
	t.Cleanup(func() { _ = debug.SetCrashOutput(nil, debug.CrashOptions{}) })
}

func TestReportPreviousCrash(t *testing.T) {
	resetCrashOutput(t)
	logs := captureLogs(t)
	dir := t.TempDir()
	crashFile := filepath.Join(dir, testName+crashFileSuffix)
	trace := "panic: boom\n\ngoroutine 1 [running]:\nmain.main()\n\t/src/main.go:5 +0x1d\nexit status 2\n"
	require.NoError(t, os.WriteFile(crashFile, []byte(trace), 0o600))
	// An old .prev file is replaced.
	require.NoError(t, os.WriteFile(filepath.Join(dir, testName+prevCrashFileSuffix), []byte("old"), 0o600))

	setupCrashReport(dir, testName)

	out := logs.String()
	assert.Equal(t, 1, strings.Count(out, "[ERROR]"), out)
	assert.Contains(t, out, "crash_report:previous_run | "+fmt.Sprintf(crashReportMarker, testName)+strings.TrimRight(trace, "\n"))
	// The lines of the trace are kept as they are.
	assert.Contains(t, out, "\npanic: boom\n")

	prev, err := os.ReadFile(filepath.Join(dir, testName+prevCrashFileSuffix))
	require.NoError(t, err)
	assert.Equal(t, trace, string(prev))

	// The crash file is created again, empty, for the current run.
	info, err := os.Stat(crashFile)
	require.NoError(t, err)
	assert.Zero(t, info.Size())
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestNoReportWithoutPreviousCrash(t *testing.T) {
	for name, content := range map[string]*string{
		"missing file": nil,
		"empty file":   new(string),
	} {
		t.Run(name, func(t *testing.T) {
			resetCrashOutput(t)
			logs := captureLogs(t)
			dir := t.TempDir()
			crashFile := filepath.Join(dir, testName+crashFileSuffix)
			if content != nil {
				require.NoError(t, os.WriteFile(crashFile, []byte(*content), 0o600))
			}

			setupCrashReport(dir, testName)

			assert.NotContains(t, logs.String(), "[ERROR]")
			assert.NotContains(t, logs.String(), "[WARN]")
			assert.NoFileExists(t, filepath.Join(dir, testName+prevCrashFileSuffix))
			assert.FileExists(t, crashFile)
		})
	}
}

func TestReportIsTruncated(t *testing.T) {
	resetCrashOutput(t)
	logs := captureLogs(t)
	dir := t.TempDir()
	head := "fatal error: concurrent map writes\n"
	data := head + strings.Repeat("x", 2*maxCrashReportSize)
	require.NoError(t, os.WriteFile(filepath.Join(dir, testName+crashFileSuffix), []byte(data), 0o600))

	setupCrashReport(dir, testName)

	out := logs.String()
	// The report keeps exactly the first maxCrashReportSize bytes.
	kept := head + strings.Repeat("x", maxCrashReportSize-len(head))
	assert.Contains(t, out, "\n"+kept+fmt.Sprintf("\n[crash output truncated to the first %d bytes]", maxCrashReportSize))
	// The .prev file keeps the full output.
	prev, err := os.ReadFile(filepath.Join(dir, testName+prevCrashFileSuffix))
	require.NoError(t, err)
	assert.Len(t, prev, len(data))
}

func TestSetupCreatesDirectory(t *testing.T) {
	resetCrashOutput(t)
	captureLogs(t)
	dir := filepath.Join(t.TempDir(), "a", "b")

	setupCrashReport(dir, testName)

	assert.FileExists(t, filepath.Join(dir, testName+crashFileSuffix))
}

func TestSetupErrorIsOnlyAWarning(t *testing.T) {
	resetCrashOutput(t)
	logs := captureLogs(t)
	// A regular file where the directory is expected.
	notADir := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(notADir, nil, 0o600))

	setupCrashReport(notADir, testName)

	assert.Contains(t, logs.String(), "[WARN]")
	assert.NotContains(t, logs.String(), "[ERROR]")
}

// TestCrashOutputIsReportedOnNextRun runs a child process that enables the
// crash report through Setup and panics, then checks that the next Setup call
// reports the panic of the child.
func TestCrashOutputIsReportedOnNextRun(t *testing.T) {
	if dir := os.Getenv(childDirEnv); dir != "" {
		cfg := configmock.New(t)
		cfg.Set("go_crash_report", true, model.SourceFile)
		cfg.Set("go_crash_report_dir", dir, model.SourceFile)
		_ = Setup(cfg)
		panic(childPanic)
	}

	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestCrashOutputIsReportedOnNextRun$", "-test.count=1")
	cmd.Env = append(os.Environ(), childDirEnv+"="+dir, "DD_BUNDLED_AGENT="+testName)
	out, err := cmd.CombinedOutput()
	require.Error(t, err, "the child process must crash: %s", out)
	// Go still writes the crash output to stderr.
	require.Contains(t, string(out), "panic: "+childPanic)

	crash, err := os.ReadFile(filepath.Join(dir, testName+crashFileSuffix))
	require.NoError(t, err)
	require.Contains(t, string(crash), "panic: "+childPanic)
	require.Contains(t, string(crash), "goroutine ")

	resetCrashOutput(t)
	logs := captureLogs(t)
	t.Setenv("DD_BUNDLED_AGENT", testName)
	cfg := configmock.New(t)
	cfg.Set("go_crash_report", true, model.SourceFile)
	cfg.Set("go_crash_report_dir", dir, model.SourceFile)
	require.NoError(t, Setup(cfg))

	report := logs.String()
	assert.Contains(t, report, "crash_report:previous_run | "+fmt.Sprintf(crashReportMarker, testName)+"panic: "+childPanic)
	assert.Contains(t, report, "\ngoroutine ")
	assert.FileExists(t, filepath.Join(dir, testName+prevCrashFileSuffix))
}

func TestCrashReportDisabledByDefault(t *testing.T) {
	for name, cfg := range map[string]model.Reader{
		"core":         configmock.New(t),
		"system-probe": configmock.NewSystemProbe(t),
	} {
		t.Run(name, func(t *testing.T) {
			assert.False(t, cfg.GetBool("go_crash_report"))
			// The default is the Agent log directory: ${log_path} is resolved.
			dir := cfg.GetString("go_crash_report_dir")
			assert.True(t, filepath.IsAbs(dir), dir)
			assert.NotContains(t, dir, "${")
		})
	}
}

func TestSymlinkIsNotFollowed(t *testing.T) {
	resetCrashOutput(t)
	logs := captureLogs(t)
	dir := t.TempDir()
	secret := filepath.Join(t.TempDir(), "secret")
	require.NoError(t, os.WriteFile(secret, []byte("panic: secret content"), 0o600))
	require.NoError(t, os.Symlink(secret, filepath.Join(dir, testName+crashFileSuffix)))

	setupCrashReport(dir, testName)

	assert.NotContains(t, logs.String(), "secret content")
	assert.NotContains(t, logs.String(), "[ERROR]")
	assert.Contains(t, logs.String(), "[WARN]")
	// The target of the link is not truncated.
	content, err := os.ReadFile(secret)
	require.NoError(t, err)
	assert.Equal(t, "panic: secret content", string(content))
}

func TestFIFODoesNotBlock(t *testing.T) {
	resetCrashOutput(t)
	logs := captureLogs(t)
	dir := t.TempDir()
	require.NoError(t, unix.Mkfifo(filepath.Join(dir, testName+crashFileSuffix), 0o600))

	done := make(chan struct{})
	go func() {
		setupCrashReport(dir, testName)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("setupCrashReport blocked on a FIFO")
	}
	assert.Contains(t, logs.String(), "not a regular file")
}

func TestProcessName(t *testing.T) {
	t.Setenv("DD_BUNDLED_AGENT", " trace-agent ")
	assert.Equal(t, "trace-agent", processName())

	t.Setenv("DD_BUNDLED_AGENT", "")
	assert.Equal(t, sanitizeName(os.Args[0]), processName())

	assert.Equal(t, "system-probe", sanitizeName("/opt/datadog-agent/embedded/bin/system-probe"))
	assert.Equal(t, "", sanitizeName("/"))
	assert.Equal(t, "", sanitizeName("  "))
}
