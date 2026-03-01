// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package procscan

import (
	"embed"
	"io/fs"
	"iter"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

//go:embed testdata/stat/*.txt
var testdataFS embed.FS

func TestStartTimeTicksFromProcStat(t *testing.T) {
	for tc := range testCases(t) {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.reader.read(tc.expected.pid)
			require.NoError(t, err, "unexpected error")
			require.Equal(t, tc.expected.startTime, got)
		})
	}
}

func BenchmarkStartTimeReader_Read(b *testing.B) {
	tcs := slices.Collect(testCases(b))
	tc := tcs[0] // just pick the first one
	b.ResetTimer()
	for b.Loop() {
		_, err := tc.reader.read(tc.expected.pid)
		require.NoError(b, err, "unexpected error")
	}
}

// expectation defines the expected values for each test data file.
type expectation struct {
	pid       int32
	startTime uint64
}

// expectedValues maps test data filenames to their expected PID and start
// time values.
var expectedValues = map[string]expectation{
	"simple":           {pid: 1234, startTime: 123456789},
	"long_name":        {pid: 5678, startTime: 987654321},
	"with_spaces":      {pid: 9999, startTime: 555666777},
	"golang":           {pid: 12345, startTime: 1234567890},
	"maxlen_comm":      {pid: 100000, startTime: 9999999999},
	"small_pid":        {pid: 1, startTime: 1},
	"parens_in_name":   {pid: 22222, startTime: 777888999},
	"multiple_parens":  {pid: 33333, startTime: 888999000},
	"closing_paren":    {pid: 44444, startTime: 666777888},
	"numbers_start":    {pid: 55555, startTime: 111222333},
	"special_chars":    {pid: 66666, startTime: 222333444},
	"single_char":      {pid: 77777, startTime: 333444555},
	"all_digits":       {pid: 88888, startTime: 444555666},
	"quotes_backslash": {pid: 99999, startTime: 555666777},
}

type testCase struct {
	name     string
	expected expectation
	reader   *startTimeReader
}

func testCases(t testing.TB) iter.Seq[testCase] {
	t.TempDir()
	entries, err := fs.Glob(testdataFS, "testdata/stat/*.txt")
	require.NoError(t, err)
	tempDir := t.TempDir()
	expectedValues := maps.Clone(expectedValues)
	return func(yield func(testCase) bool) {
		// Ensure we saw all expected test files.
		defer func() {
			if t.Failed() {
				return
			}
			require.Empty(t, expectedValues, "unexpected test files remaining")
		}()
		for _, entry := range entries {
			filename := filepath.Base(entry)
			name := strings.TrimSuffix(filename, ".txt")
			expected, ok := expectedValues[name]
			require.True(t, ok,
				"test file %s found but no expected values defined", name)
			delete(expectedValues, name)
			data, err := testdataFS.ReadFile(entry)
			require.NoError(t, err, "failed to read test data")
			procRoot := filepath.Join(tempDir, name, "proc")
			pidDir := filepath.Join(procRoot, strconv.FormatInt(int64(expected.pid), 10))
			require.NoError(t, os.MkdirAll(pidDir, 0o755))
			statPath := filepath.Join(pidDir, "stat")
			require.NoError(
				t,
				os.WriteFile(statPath, data, 0o644))
			if !yield(testCase{
				name:     name,
				expected: expected,
				reader:   newStartTimeReader(procRoot),
			}) {
				return
			}
		}
	}
}
