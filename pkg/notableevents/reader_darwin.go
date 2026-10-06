// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package notableevents

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

const (
	// Apple does not publish a maximum .ips size. This is an Agent safety limit.
	maxMacOSCrashReportSize = 10 * 1024 * 1024
	maxMacOSMetadataSize    = 64 * 1024
	maxReportBasenameSize   = 255
)

// diagnosticReportPolicyError marks a report that the collector intentionally
// will not read. Policy failures are permanent for baseline completion.
type diagnosticReportPolicyError struct {
	message string
}

func (e *diagnosticReportPolicyError) Error() string {
	return e.message
}

type safeReportFile struct {
	file        *os.File
	directoryFD int
	name        string
	initialStat unix.Stat_t
	fingerprint string
}

// openDiagnosticReportDirectory opens every path component without following symlinks.
func openDiagnosticReportDirectory(path string) (*os.File, error) {
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("DiagnosticReports path must be absolute: %s", path)
	}

	parts := strings.Split(filepath.Clean(path), string(filepath.Separator))
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	for index := 1; index < len(parts); index++ {
		if parts[index] == "" {
			continue
		}
		nextFD, openErr := unix.Openat(fd, parts[index], unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
		if openErr != nil {
			_ = unix.Close(fd)
			return nil, openErr
		}
		_ = unix.Close(fd)
		fd = nextFD
	}

	return os.NewFile(uintptr(fd), path), nil
}

// validateReportBasename rejects traversal, alternate paths, and non-report names
// before any value reaches openat.
func validateReportBasename(name string) error {
	if name == "" || len(name) > maxReportBasenameSize {
		return errors.New("invalid crash report basename")
	}
	if name == "." || name == ".." || filepath.Base(name) != name {
		return fmt.Errorf("invalid crash report basename %q", name)
	}
	if strings.ContainsAny(name, "/\\\x00") || !strings.HasSuffix(name, ".ips") {
		return fmt.Errorf("invalid crash report basename %q", name)
	}
	return nil
}

// openSafeReportFile opens a validated report basename and records its initial fingerprint.
func openSafeReportFile(directory *os.File, name string) (*safeReportFile, error) {
	if err := validateReportBasename(name); err != nil {
		return nil, err
	}
	if directory == nil {
		return nil, errors.New("nil DiagnosticReports directory")
	}

	fd, err := unix.Openat(int(directory.Fd()), name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("open crash report: %w", err)
	}

	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("failed to create crash report file handle")
	}

	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("stat crash report: %w", err)
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG {
		_ = file.Close()
		return nil, &diagnosticReportPolicyError{message: "crash report is not a regular file"}
	}
	if stat.Size > maxMacOSCrashReportSize {
		_ = file.Close()
		return nil, oversizedDiagnosticReportError()
	}

	return &safeReportFile{
		file:        file,
		directoryFD: int(directory.Fd()),
		name:        name,
		initialStat: stat,
		fingerprint: reportFingerprint(&stat),
	}, nil
}

// Close releases the securely opened report file.
func (f *safeReportFile) Close() error {
	return f.file.Close()
}

// unchanged verifies that the opened report still refers to its original file contents.
func (f *safeReportFile) unchanged() bool {
	var current unix.Stat_t
	if err := unix.Fstat(int(f.file.Fd()), &current); err != nil {
		return false
	}
	if !sameReportStat(&f.initialStat, &current) {
		return false
	}

	var pathStat unix.Stat_t
	if err := unix.Fstatat(f.directoryFD, f.name, &pathStat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return false
	}
	return sameReportStat(&f.initialStat, &pathStat)
}

// readMacOSCrashReportFile reads, bounds, and parses a stable crash report file.
func readMacOSCrashReportFile(file *safeReportFile) (*macOSCrashReport, bool, error) {
	limited := io.LimitReader(file.file, maxMacOSCrashReportSize+1)
	reader := bufio.NewReaderSize(limited, maxMacOSMetadataSize+1)

	metadataLine, err := reader.ReadSlice('\n')
	if errors.Is(err, bufio.ErrBufferFull) {
		return nil, false, fmt.Errorf("crash report metadata exceeds %d bytes", maxMacOSMetadataSize)
	}
	if err != nil {
		return nil, false, fmt.Errorf("failed to read crash report metadata: %w", err)
	}

	metadata, err := parseMacOSCrashMetadata(metadataLine)
	if err != nil {
		return nil, false, err
	}
	if getString(metadata, "bug_type") != macosCrashBugType {
		return nil, false, nil
	}

	remainingLimit := int64(maxMacOSCrashReportSize - len(metadataLine))
	body, err := io.ReadAll(io.LimitReader(reader, remainingLimit+1))
	if err != nil {
		return nil, false, fmt.Errorf("failed to read crash report body: %w", err)
	}
	if int64(len(body)) > remainingLimit {
		return nil, false, oversizedDiagnosticReportError()
	}

	report, err := parseMacOSCrashReportBody(body)
	if err != nil {
		return nil, false, err
	}
	if !file.unchanged() {
		return nil, false, errors.New("crash report changed while being read")
	}

	return &macOSCrashReport{metadata: metadata, report: report}, true, nil
}

func oversizedDiagnosticReportError() error {
	return &diagnosticReportPolicyError{
		message: fmt.Sprintf("crash report exceeds the %d-byte size limit", maxMacOSCrashReportSize),
	}
}

// reportFingerprint derives a stable identity from a report file's metadata.
func reportFingerprint(stat *unix.Stat_t) string {
	return fmt.Sprintf(
		"%d:%d:%d:%d:%d:%d:%d",
		stat.Dev,
		stat.Ino,
		stat.Size,
		stat.Mtim.Sec,
		stat.Mtim.Nsec,
		stat.Ctim.Sec,
		stat.Ctim.Nsec,
	)
}

// sameReportStat reports whether two snapshots identify the same unchanged file.
func sameReportStat(left, right *unix.Stat_t) bool {
	return left.Dev == right.Dev &&
		left.Ino == right.Ino &&
		left.Size == right.Size &&
		left.Mtim.Sec == right.Mtim.Sec &&
		left.Mtim.Nsec == right.Mtim.Nsec &&
		left.Ctim.Sec == right.Ctim.Sec &&
		left.Ctim.Nsec == right.Ctim.Nsec
}
