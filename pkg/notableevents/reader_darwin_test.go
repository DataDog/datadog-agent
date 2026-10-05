// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package notableevents

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// TestOpenSafeReportFileRejectsUnsafeBasenameBeforeOpenat verifies traversal names never reach the filesystem.
func TestOpenSafeReportFileRejectsUnsafeBasenameBeforeOpenat(t *testing.T) {
	unsafeNames := []string{
		"",
		".",
		"..",
		"../secret.ips",
		"/tmp/secret.ips",
		"nested/report.ips",
		`nested\report.ips`,
		"report.txt",
		"report.ips\x00ignored",
	}
	for _, name := range unsafeNames {
		t.Run(name, func(t *testing.T) {
			_, err := openSafeReportFile(nil, name)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "basename")
		})
	}
}

// TestOpenSafeReportFileRejectsSymlink verifies report files cannot redirect through symlinks.
func TestOpenSafeReportFileRejectsSymlink(t *testing.T) {
	dir := realTempDir(t)
	target := filepath.Join(dir, "target")
	require.NoError(t, os.WriteFile(target, []byte(sampleIPSReport("App", "com.example.app", "/Applications/App", "INCIDENT")), 0o600))
	require.NoError(t, os.Symlink(target, filepath.Join(dir, "report.ips")))

	directory, err := openDiagnosticReportDirectory(dir)
	require.NoError(t, err)
	defer directory.Close()

	_, err = openSafeReportFile(directory, "report.ips")
	require.Error(t, err)
	assert.True(t, errors.Is(err, unix.ELOOP), "Openat errno must remain wrapped")
}

// TestOpenSafeReportFileRejectsFIFOWithoutBlocking verifies special files are rejected safely.
func TestOpenSafeReportFileRejectsFIFOWithoutBlocking(t *testing.T) {
	dir := realTempDir(t)
	require.NoError(t, unix.Mkfifo(filepath.Join(dir, "report.ips"), 0o600))
	directory, err := openDiagnosticReportDirectory(dir)
	require.NoError(t, err)
	defer directory.Close()

	_, err = openSafeReportFile(directory, "report.ips")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a regular file")
	var policyError *diagnosticReportPolicyError
	require.ErrorAs(t, err, &policyError)
}

// TestOpenSafeReportFileEnforcesSizeLimit verifies oversized reports are rejected before reading.
func TestOpenSafeReportFileEnforcesSizeLimit(t *testing.T) {
	dir := realTempDir(t)
	path := filepath.Join(dir, "report.ips")
	file, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, file.Truncate(maxMacOSCrashReportSize+1))
	require.NoError(t, file.Close())

	directory, err := openDiagnosticReportDirectory(dir)
	require.NoError(t, err)
	defer directory.Close()
	_, err = openSafeReportFile(directory, "report.ips")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "size limit")
	var policyError *diagnosticReportPolicyError
	require.ErrorAs(t, err, &policyError)
}

// TestReadMacOSCrashReportFileAcceptsExactSizeLimit verifies boundary-sized reports remain valid.
func TestReadMacOSCrashReportFileAcceptsExactSizeLimit(t *testing.T) {
	content := []byte(sampleIPSReport("App", "com.example.app", "/Applications/App", "INCIDENT"))
	content = append(content, bytes.Repeat([]byte(" "), maxMacOSCrashReportSize-len(content))...)

	dir := realTempDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "report.ips"), content, 0o600))
	directory, err := openDiagnosticReportDirectory(dir)
	require.NoError(t, err)
	defer directory.Close()
	reportFile, err := openSafeReportFile(directory, "report.ips")
	require.NoError(t, err)
	defer reportFile.Close()

	report, isCrash, err := readMacOSCrashReportFile(reportFile)
	require.NoError(t, err)
	require.True(t, isCrash)
	assert.Equal(t, "INCIDENT", report.incidentID())
}

// TestReadMacOSCrashReportFileRejectsNonCrashBeforeBody verifies irrelevant reports are filtered from metadata alone.
func TestReadMacOSCrashReportFileRejectsNonCrashBeforeBody(t *testing.T) {
	dir := realTempDir(t)
	content := []byte(`{"bug_type":"288","name":"NotACrash"}` + "\n" + `not JSON`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "report.ips"), content, 0o600))
	directory, err := openDiagnosticReportDirectory(dir)
	require.NoError(t, err)
	defer directory.Close()
	reportFile, err := openSafeReportFile(directory, "report.ips")
	require.NoError(t, err)
	defer reportFile.Close()

	report, isCrash, err := readMacOSCrashReportFile(reportFile)
	require.NoError(t, err)
	assert.False(t, isCrash)
	assert.Nil(t, report)
}

func TestDiagnosticReportJSONReadErrorClassification(t *testing.T) {
	tests := []struct {
		name              string
		body              string
		wantTransient     bool
		wantSyntax        bool
		wantEOF           bool
		wantUnexpectedEOF bool
	}{
		{
			name:              "object missing closing brace",
			body:              `{"bug_type":"309"`,
			wantUnexpectedEOF: true,
		},
		{
			name:              "nested object truncated",
			body:              `{"bug_type":"309","nested":{"value":1}`,
			wantUnexpectedEOF: true,
		},
		{
			name:    "empty body",
			body:    "",
			wantEOF: true,
		},
		{
			name:       "invalid token in stable object",
			body:       `{"bug_type":"309","nested":]}`,
			wantSyntax: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := realTempDir(t)
			content := `{"bug_type":"309","incident_id":"INCIDENT"}` + "\n" + test.body
			require.NoError(t, os.WriteFile(filepath.Join(dir, "report.ips"), []byte(content), 0o600))

			directory, err := openDiagnosticReportDirectory(dir)
			require.NoError(t, err)
			defer directory.Close()
			reportFile, err := openSafeReportFile(directory, "report.ips")
			require.NoError(t, err)
			defer reportFile.Close()

			_, _, readErr := readMacOSCrashReportFile(reportFile)
			require.Error(t, readErr)
			if test.wantSyntax {
				var syntaxErr *json.SyntaxError
				assert.ErrorAs(t, readErr, &syntaxErr)
			}
			if test.wantEOF {
				assert.ErrorIs(t, readErr, io.EOF)
			}
			if test.wantUnexpectedEOF {
				assert.ErrorIs(t, readErr, io.ErrUnexpectedEOF)
			}
			assert.Equal(t, test.wantTransient, isTransientDiagnosticReportReadError(readErr, reportFile))
		})
	}
}

func TestDiagnosticReportReadErrorBecomesTransientWhenFileChanges(t *testing.T) {
	dir := realTempDir(t)
	path := filepath.Join(dir, "report.ips")
	content := `{"bug_type":"309","incident_id":"INCIDENT"}` + "\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	directory, err := openDiagnosticReportDirectory(dir)
	require.NoError(t, err)
	defer directory.Close()
	reportFile, err := openSafeReportFile(directory, "report.ips")
	require.NoError(t, err)
	defer reportFile.Close()

	_, _, readErr := readMacOSCrashReportFile(reportFile)
	require.ErrorIs(t, readErr, io.EOF)
	appender, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	require.NoError(t, err)
	_, err = appender.WriteString(`{}`)
	require.NoError(t, err)
	require.NoError(t, appender.Close())

	assert.True(t, isTransientDiagnosticReportReadError(readErr, reportFile))
}

func TestIncompleteDiagnosticReportSyntaxErrors(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		incomplete bool
	}{
		{
			name:       "value truncated after colon",
			body:       `{"bug_type":"309","nested":`,
			incomplete: true,
		},
		{
			name:       "unterminated string",
			body:       `{"bug_type":"309","nested":"unfinished`,
			incomplete: true,
		},
		{
			name: "invalid stable token",
			body: `{"bug_type":"309","nested":]}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var value map[string]interface{}
			parseErr := json.Unmarshal([]byte(test.body), &value)
			require.Error(t, parseErr)
			var syntaxErr *json.SyntaxError
			require.ErrorAs(t, parseErr, &syntaxErr)

			err := newDiagnosticReportJSONError(parseErr, []byte(test.body))
			assert.Equal(t, test.incomplete, isIncompleteDiagnosticReportJSONError(err))
		})
	}
}

// TestSafeReportFileDetectsReplacement verifies a pathname swap invalidates an open report.
func TestSafeReportFileDetectsReplacement(t *testing.T) {
	dir := realTempDir(t)
	path := filepath.Join(dir, "report.ips")
	content := []byte(sampleIPSReport("App", "com.example.app", "/Applications/App", "INCIDENT"))
	require.NoError(t, os.WriteFile(path, content, 0o600))
	directory, err := openDiagnosticReportDirectory(dir)
	require.NoError(t, err)
	defer directory.Close()
	reportFile, err := openSafeReportFile(directory, "report.ips")
	require.NoError(t, err)
	defer reportFile.Close()

	require.NoError(t, os.Rename(path, filepath.Join(dir, "original.ips")))
	require.NoError(t, os.WriteFile(path, content, 0o600))
	assert.False(t, reportFile.unchanged())
}

// TestSameReportStatDetectsRestoredMtimeMutation verifies ctime detects an
// equal-size rewrite even when an untrusted owner restores the original mtime.
func TestSameReportStatDetectsRestoredMtimeMutation(t *testing.T) {
	original := unix.Stat_t{
		Dev:  1,
		Ino:  2,
		Size: 128,
	}
	original.Mtim.Sec = 10
	original.Mtim.Nsec = 20
	original.Ctim.Sec = 30
	original.Ctim.Nsec = 40
	mutated := original
	mutated.Ctim.Nsec++

	assert.False(t, sameReportStat(&original, &mutated))
	assert.NotEqual(t, reportFingerprint(&original), reportFingerprint(&mutated))
}
