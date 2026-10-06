// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package coredump

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// crashFileSuffix is the suffix of the file that receives the Go crash
	// output of the current run.
	crashFileSuffix = ".crash"
	// prevCrashFileSuffix is the suffix of the file that keeps the crash
	// output of the previous run after it was reported.
	prevCrashFileSuffix = ".crash.prev"

	// maxCrashReportSize is the maximum number of bytes of a crash file that
	// are put in the report log line. The beginning of the crash output holds
	// the "panic:" or "fatal error:" line and the stack of the goroutine that
	// crashed, so the beginning is kept.
	maxCrashReportSize = 64 * 1024

	// crashReportMarker starts the log message that reports the crash of the
	// previous run. The %s is the process name. Keep this text stable: log
	// searches and log-based metrics use it.
	crashReportMarker = "Previous run of %s crashed. Crash output:\n"

	// crashReportContextKey and crashReportContextValue are added to the log
	// line as structured context.
	crashReportContextKey   = "crash_report"
	crashReportContextValue = "previous_run"
)

// setupCrashReport reports the crash output that a previous run left in dir,
// then redirects the Go crash output of the current process to a file in dir.
// Errors are logged as warnings: a crash report problem must never stop the
// process from starting.
func setupCrashReport(dir, name string) {
	if dir == "" {
		log.Warn("go_crash_report is enabled but go_crash_report_dir is empty, Go crash output will not be saved")
		return
	}

	if err := os.MkdirAll(dir, 0o750); err != nil {
		log.Warnf("Cannot create Go crash report directory %q: %v", dir, err)
		return
	}

	crashFile := filepath.Join(dir, name+crashFileSuffix)

	if err := reportPreviousCrash(crashFile, name); err != nil {
		log.Warnf("Cannot report the Go crash output of the previous run: %v", err)
	}

	if err := redirectCrashOutput(crashFile); err != nil {
		log.Warnf("Cannot redirect Go crash output to %q: %v", crashFile, err)
		return
	}
	log.Infof("Go crash output is also written to %q", crashFile)
}

// reportPreviousCrash renames crashFile to <name>.crash.prev and logs its
// content in one error log record, if the file is not empty.
func reportPreviousCrash(crashFile, name string) error {
	f, err := openCrashFile(crashFile, os.O_RDONLY)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	data, err := io.ReadAll(io.LimitReader(f, maxCrashReportSize+1))
	_ = f.Close()
	if err != nil {
		return fmt.Errorf("cannot read %q: %w", crashFile, err)
	}
	if len(data) == 0 {
		return nil
	}

	// Rename before logging, so that a crash while logging does not report
	// the same crash twice.
	if err := os.Rename(crashFile, strings.TrimSuffix(crashFile, crashFileSuffix)+prevCrashFileSuffix); err != nil {
		log.Warnf("Cannot rename %q: %v", crashFile, err)
	}

	_ = log.Errorc(formatCrashReport(name, data), crashReportContextKey, crashReportContextValue)
	return nil
}

// formatCrashReport builds the report message. The crash output is kept
// verbatim (line breaks included) so that searches for "panic: " or
// "fatal error: " and stack frame parsers still work on it.
func formatCrashReport(name string, data []byte) string {
	truncated := len(data) > maxCrashReportSize
	if truncated {
		data = data[:maxCrashReportSize]
	}

	var b strings.Builder
	b.Grow(len(crashReportMarker) + len(name) + len(data) + 64)
	fmt.Fprintf(&b, crashReportMarker, name)
	// The cut can split a multi-byte character; replace invalid bytes.
	b.WriteString(strings.ToValidUTF8(strings.TrimRight(string(data), "\n"), "\uFFFD"))
	if truncated {
		fmt.Fprintf(&b, "\n[crash output truncated to the first %d bytes]", maxCrashReportSize)
	}
	return b.String()
}

// redirectCrashOutput truncates crashFile and makes the Go runtime write its
// crash output (unrecovered panic, fatal error, fatal signal) to it, in
// addition to stderr.
func redirectCrashOutput(crashFile string) error {
	// Truncate only after openCrashFile checked the file: O_TRUNC would
	// truncate the file before the checks.
	f, err := openCrashFile(crashFile, os.O_WRONLY|os.O_CREATE)
	if err != nil {
		return err
	}
	// SetCrashOutput duplicates the file descriptor and keeps the duplicate
	// open for the life of the process, so f can be closed.
	defer f.Close()
	if err := f.Truncate(0); err != nil {
		return err
	}
	return debug.SetCrashOutput(f, debug.CrashOptions{})
}

// openCrashFile opens a crash file safely. The directory can be shared by
// processes that run as different users (for example root for system-probe
// and dd-agent for the core Agent), so the file must not be a symbolic link,
// must not block the open (FIFO), must be a regular file, and must belong to
// the current user.
func openCrashFile(path string, flag int) (*os.File, error) {
	f, err := os.OpenFile(path, flag|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return nil, err
	}
	if err := checkCrashFile(f); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("%q: %w", path, err)
	}
	return f, nil
}

func checkCrashFile(f *os.File) error {
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("not a regular file (mode %s)", info.Mode())
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Geteuid() {
		return fmt.Errorf("owned by uid %d, not by the current user (uid %d)", st.Uid, os.Geteuid())
	}
	return nil
}

// processName returns the name used for the crash file of this process.
// Several Agent binaries can share the same directory, and the bundled Agent
// binary runs as another Agent when DD_BUNDLED_AGENT is set or when it is
// started through a link with another name (see cmd/agent/main.go). So the
// name follows the same logic, not the path of the executable.
func processName() string {
	if name := sanitizeName(os.Getenv("DD_BUNDLED_AGENT")); name != "" {
		return name
	}
	if len(os.Args) > 0 {
		if name := sanitizeName(os.Args[0]); name != "" {
			return name
		}
	}
	if exe, err := os.Executable(); err == nil {
		if name := sanitizeName(exe); name != "" {
			return name
		}
	}
	return "agent"
}

func sanitizeName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	name := filepath.Base(s)
	name = strings.TrimSuffix(name, filepath.Ext(name))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return ""
	}
	return name
}
