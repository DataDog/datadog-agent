// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secretsimpl

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupRecordsTest(t *testing.T) (string, *clock.Mock, func()) {
	tmpdir, err := os.MkdirTemp("", "rotating*")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile := filepath.Join(tmpdir, "records.ndjson")

	mock := clock.NewMock()
	ts, _ := time.Parse(time.RFC3339, "2014-02-04T10:30:00+00:00")
	mock.Set(ts)

	cleanupFunc := func() {
		os.RemoveAll(tmpdir)
	}

	return tmpFile, mock, cleanupFunc
}

// Test the basic functionality of appending to a new audit file
func TestAppendNewFile(t *testing.T) {
	tmpFileName, mockClock, cleanupFunc := setupRecordsTest(t)
	defer cleanupFunc()

	records := newRotatingNDRecords(tmpFileName, config{})
	records.Add(mockClock.Now(), []auditRecord{{Handle: "apple"}})
	mockClock.Add(10 * time.Second)
	records.Add(mockClock.Now(), []auditRecord{{Handle: "banana"}})

	data, _ := os.ReadFile(tmpFileName)
	expect := `{"time":"2014-02-04T10:30:00Z","data":[{"handle":"apple"}]}
{"time":"2014-02-04T10:30:10Z","data":[{"handle":"banana"}]}
`
	assert.Equal(t, expect, string(data))
}

// Test appending to an existing audit file
func TestAppendExistingFile(t *testing.T) {
	tmpFileName, mockClock, cleanupFunc := setupRecordsTest(t)
	defer cleanupFunc()

	startingData := []byte(`{"time":"2014-02-04T10:30:00Z","data":[{"payload":"apple"}]}
{"time":"2014-02-04T10:30:10Z","data":[{"handle":"banana"}]}
`)
	os.WriteFile(tmpFileName, startingData, 0640)

	records := newRotatingNDRecords(tmpFileName, config{})
	mockClock.Add(20 * time.Second)
	records.Add(mockClock.Now(), []auditRecord{{Handle: "cherry"}})

	data, _ := os.ReadFile(tmpFileName)
	expect := `{"time":"2014-02-04T10:30:00Z","data":[{"payload":"apple"}]}
{"time":"2014-02-04T10:30:10Z","data":[{"handle":"banana"}]}
{"time":"2014-02-04T10:30:20Z","data":[{"handle":"cherry"}]}
`
	assert.Equal(t, expect, string(data))
}

// Test appending to an audit file with a given value
func TestAppendNameWithValue(t *testing.T) {
	tmpFileName, mockClock, cleanupFunc := setupRecordsTest(t)
	defer cleanupFunc()

	records := newRotatingNDRecords(tmpFileName, config{})
	records.Add(mockClock.Now(), []auditRecord{{Handle: "apple", Value: "red"}})
	mockClock.Add(10 * time.Second)
	records.Add(mockClock.Now(), []auditRecord{{Handle: "banana", Value: "yellow"}})

	data, _ := os.ReadFile(tmpFileName)
	expect := `{"time":"2014-02-04T10:30:00Z","data":[{"handle":"apple","value":"red"}]}
{"time":"2014-02-04T10:30:10Z","data":[{"handle":"banana","value":"yellow"}]}
`
	assert.Equal(t, expect, string(data))
}

// Test append multiple records
func TestAppendMultipleRecords(t *testing.T) {
	tmpFileName, mockClock, cleanupFunc := setupRecordsTest(t)
	defer cleanupFunc()

	records := newRotatingNDRecords(tmpFileName, config{})
	records.Add(mockClock.Now(), []auditRecord{{Handle: "apple"}, {Handle: "banana"}})

	data, _ := os.ReadFile(tmpFileName)
	expect := `{"time":"2014-02-04T10:30:00Z","data":[{"handle":"apple"},{"handle":"banana"}]}
`
	assert.Equal(t, expect, string(data))
}

// Test that old entries at the file start get pruned
func TestPruneOldEntries(t *testing.T) {
	tmpFileName, mockClock, cleanupFunc := setupRecordsTest(t)
	defer cleanupFunc()

	startingData := []byte(`{"time":"2014-02-04T10:30:00Z","data":[{"handle":"apple"}]}
{"time":"2014-02-04T10:30:10Z","data":[{"handle":"banana"}]}
{"time":"2014-02-04T10:30:20Z","data":[{"handle":"cherry"}]}
`)
	os.WriteFile(tmpFileName, startingData, 0640)

	records := newRotatingNDRecords(tmpFileName, config{retention: time.Second * 15})
	mockClock.Add(30 * time.Second)
	records.Add(mockClock.Now(), []auditRecord{{Handle: "donut"}})

	// test that 2 entries have been pruned and 2 remain
	data, _ := os.ReadFile(tmpFileName)
	expect := `{"time":"2014-02-04T10:30:20Z","data":[{"handle":"cherry"}]}
{"time":"2014-02-04T10:30:30Z","data":[{"handle":"donut"}]}
`
	assert.Equal(t, expect, string(data))
}

// Test that unparsable entries will just be pruned
func TestPruneUnparsableEntries(t *testing.T) {
	tmpFileName, mockClock, cleanupFunc := setupRecordsTest(t)
	defer cleanupFunc()

	startingData := []byte(`{"time":"2014-02-04T10:30:00Z","data":[{"handle":"apple"}]}
not json text
{"comment":"no time, is pruned","data":[{"handle":"apricot"}]}
{"time":"2014-02-04T10:30:10Z","data":[{"handle":"banana"}]}
{"time":"2014-02-04T10:30:20Z","data":[{"handle":"cherry"}]}
`)
	os.WriteFile(tmpFileName, startingData, 0640)

	records := newRotatingNDRecords(tmpFileName, config{retention: time.Second * 15})
	mockClock.Add(30 * time.Second)
	records.Add(mockClock.Now(), []auditRecord{{Handle: "donut"}})

	data, _ := os.ReadFile(tmpFileName)
	expect := `{"time":"2014-02-04T10:30:20Z","data":[{"handle":"cherry"}]}
{"time":"2014-02-04T10:30:30Z","data":[{"handle":"donut"}]}
`
	assert.Equal(t, expect, string(data))
}

// Test that if everything in the audit file is old, everything is pruned
func TestPruneEverything(t *testing.T) {
	tmpFileName, mockClock, cleanupFunc := setupRecordsTest(t)
	defer cleanupFunc()

	startingData := []byte(`{"time":"2014-02-04T10:30:00Z","data":[{"handle":"apple"}]}
`)
	os.WriteFile(tmpFileName, startingData, 0640)

	records := newRotatingNDRecords(tmpFileName, config{retention: time.Second * 15})
	mockClock.Add(30 * time.Second)
	records.Add(mockClock.Now(), []auditRecord{{Handle: "donut"}})

	data, _ := os.ReadFile(tmpFileName)
	expect := `{"time":"2014-02-04T10:30:30Z","data":[{"handle":"donut"}]}
`
	assert.Equal(t, expect, string(data))
}

// Test that if appending makes the file too large, it will be rotated first
func TestRotateLargeFile(t *testing.T) {
	tmpFileName, mockClock, cleanupFunc := setupRecordsTest(t)
	defer cleanupFunc()

	startingData := []byte(`{"time":"2014-02-04T10:30:00Z","data":[{"handle":"apple"}]}
{"time":"2014-02-04T10:30:10Z","data":[{"handle":"banana"}]}
{"time":"2014-02-04T10:30:20Z","data":[{"handle":"cherry"}]}
`)
	os.WriteFile(tmpFileName, startingData, 0640)

	// create with a size limit would be reached by the next row
	records := newRotatingNDRecords(tmpFileName, config{sizeLimit: 150})
	mockClock.Add(30 * time.Second)
	records.Add(mockClock.Now(), []auditRecord{{Handle: "donut"}})

	// only 1 record in the current file, the latest
	data, _ := os.ReadFile(tmpFileName)
	expect := `{"time":"2014-02-04T10:30:30Z","data":[{"handle":"donut"}]}
`
	assert.Equal(t, expect, string(data))

	// there is 1 rotated file
	rotated := records.RotatedFiles()
	assert.Equal(t, 1, len(rotated))
	assert.True(t, strings.HasSuffix(rotated[0], ".000000.ndjson"), fmt.Sprintf("filename doesn't match: %v", rotated[0]))
}

// Test that pruning old entries happens before checking the size of file that may be rotated
func TestPrunePreventsRotation(t *testing.T) {
	tmpFileName, mockClock, cleanupFunc := setupRecordsTest(t)
	defer cleanupFunc()

	startingData := []byte(`{"time":"2014-02-04T10:30:00Z","data":[{"handle":"apple"}]}
{"time":"2014-02-04T10:30:10Z","data":[{"handle":"banana"}]}
{"time":"2014-02-04T10:30:20Z","data":[{"handle":"cherry"}]}
`)
	os.WriteFile(tmpFileName, startingData, 0640)

	// create with a size limit would be reached by the next row
	records := newRotatingNDRecords(tmpFileName, config{
		sizeLimit: 150,
		retention: time.Second * 15,
	})
	mockClock.Add(30 * time.Second)
	records.Add(mockClock.Now(), []auditRecord{{Handle: "donut"}})

	// only 2 records in the current file, old were pruned
	data, _ := os.ReadFile(tmpFileName)
	expect := `{"time":"2014-02-04T10:30:20Z","data":[{"handle":"cherry"}]}
{"time":"2014-02-04T10:30:30Z","data":[{"handle":"donut"}]}
`
	assert.Equal(t, expect, string(data))

	// there are 0 rotated files, no rotation happened because pruning happened first
	rotated := records.RotatedFiles()
	assert.Equal(t, 0, len(rotated))
}

// Test that if there's already rotated files, a further rotation uses the next available name
func TestRotateWithExistingFiles(t *testing.T) {
	tmpFileName, mockClock, cleanupFunc := setupRecordsTest(t)
	defer cleanupFunc()

	startingData := []byte(`{"time":"2014-02-04T10:30:00Z","data":[{"handle":"apple"}]}
{"time":"2014-02-04T10:30:10Z","data":[{"handle":"banana"}]}
{"time":"2014-02-04T10:30:20Z","data":[{"handle":"cherry"}]}
`)
	os.WriteFile(tmpFileName, startingData, 0640)

	// create 3 files that match the rotated pattern
	ext := filepath.Ext(tmpFileName)
	prefix := strings.TrimSuffix(tmpFileName, ext)
	for i := 0; i < 3; i++ {
		filename := fmt.Sprintf("%s.%06d%s", prefix, i, ext)
		os.WriteFile(filename, []byte(" "), 0640)
	}

	// create with a size limit would be reached by the next row
	records := newRotatingNDRecords(tmpFileName, config{sizeLimit: 150})
	mockClock.Add(30 * time.Second)
	records.Add(mockClock.Now(), []auditRecord{{Handle: "donut"}})

	// only 1 record in the current file, the latest
	data, _ := os.ReadFile(tmpFileName)
	expect := `{"time":"2014-02-04T10:30:30Z","data":[{"handle":"donut"}]}
`
	assert.Equal(t, expect, string(data))

	// there are 4 rotated files
	rotated := records.RotatedFiles()
	assert.Equal(t, 4, len(rotated))
	assert.True(t, strings.HasSuffix(rotated[0], ".000000.ndjson"), fmt.Sprintf("filename doesn't match: %v", rotated[0]))
	assert.True(t, strings.HasSuffix(rotated[1], ".000001.ndjson"), fmt.Sprintf("filename doesn't match: %v", rotated[1]))
	assert.True(t, strings.HasSuffix(rotated[2], ".000002.ndjson"), fmt.Sprintf("filename doesn't match: %v", rotated[2]))
	assert.True(t, strings.HasSuffix(rotated[3], ".000003.ndjson"), fmt.Sprintf("filename doesn't match: %v", rotated[3]))

	// the last rotated file has the previous contents
	data, _ = os.ReadFile(rotated[3])
	assert.Equal(t, startingData, data)
}

// Test that old files that were rotated get removed once they pass the retention time
func TestRemoveOldFiles(t *testing.T) {
	tmpFileName, mockClock, cleanupFunc := setupRecordsTest(t)
	defer cleanupFunc()

	// create 5 files that match the rotated pattern, set their mtime
	ext := filepath.Ext(tmpFileName)
	prefix := strings.TrimSuffix(tmpFileName, ext)
	for i := 0; i < 5; i++ {
		filename := fmt.Sprintf("%s.%06d%s", prefix, i, ext)
		os.WriteFile(filename, []byte(" "), 0640)
		os.Chtimes(filename, time.Now(), mockClock.Now())
		mockClock.Add(10 * time.Second)
	}

	records := newRotatingNDRecords(tmpFileName, config{retention: time.Second * 35})
	records.Add(mockClock.Now(), []auditRecord{{Handle: "apple"}})

	// only 1 record in the current file, the latest
	data, _ := os.ReadFile(tmpFileName)
	expect := `{"time":"2014-02-04T10:30:50Z","data":[{"handle":"apple"}]}
`
	assert.Equal(t, expect, string(data))

	// there are 3 rotated files instead of 5, the first 2 were deleted
	rotated := records.RotatedFiles()
	assert.Equal(t, 3, len(rotated))
	assert.True(t, strings.HasSuffix(rotated[0], ".000002.ndjson"), fmt.Sprintf("filename doesn't match: %v", rotated[0]))
	assert.True(t, strings.HasSuffix(rotated[1], ".000003.ndjson"), fmt.Sprintf("filename doesn't match: %v", rotated[1]))
	assert.True(t, strings.HasSuffix(rotated[2], ".000004.ndjson"), fmt.Sprintf("filename doesn't match: %v", rotated[2]))
}

// Test that old files get removed even if there were no old files at first
func TestOldFilesGetRemovedOverTime(t *testing.T) {
	tmpFileName, mockClock, cleanupFunc := setupRecordsTest(t)
	defer cleanupFunc()

	startingData := []byte(`{"time":"2014-02-04T10:30:00Z","data":[{"handle":"apple"}]}
{"time":"2014-02-04T10:30:10Z","data":[{"handle":"banana"}]}
{"time":"2014-02-04T10:30:20Z","data":[{"handle":"cherry"}]}
`)
	os.WriteFile(tmpFileName, startingData, 0640)

	// create with a size limit would be reached by the next row
	records := newRotatingNDRecords(tmpFileName, config{sizeLimit: 150, retention: 35 * time.Second})
	mockClock.Add(30 * time.Second)
	records.Add(mockClock.Now(), []auditRecord{{Handle: "donut"}})

	// there is 1 rotated file, set its mtime so it will be removed soon
	rotated := records.RotatedFiles()
	assert.Equal(t, 1, len(rotated))
	os.Chtimes(rotated[0], time.Now(), mockClock.Now())

	// only 1 record in the current file, the latest
	data, _ := os.ReadFile(tmpFileName)
	expect := `{"time":"2014-02-04T10:30:30Z","data":[{"handle":"donut"}]}
`
	assert.Equal(t, expect, string(data))

	// advance time and add another record
	mockClock.Add(20 * time.Second)
	records.Add(mockClock.Now(), []auditRecord{{Handle: "egg"}})

	// there are 2 records in the audit file
	data, _ = os.ReadFile(tmpFileName)
	expect = `{"time":"2014-02-04T10:30:30Z","data":[{"handle":"donut"}]}
{"time":"2014-02-04T10:30:50Z","data":[{"handle":"egg"}]}
`
	assert.Equal(t, expect, string(data))

	// there is 1 rotated file still
	rotated = records.RotatedFiles()
	assert.Equal(t, 1, len(rotated))

	// advance time again and add another record
	mockClock.Add(20 * time.Second)
	records.Add(mockClock.Now(), []auditRecord{{Handle: "fruit"}})

	// there are 0 rotated files because the last append removed the old rotated file
	rotated = records.RotatedFiles()
	assert.Equal(t, 0, len(rotated))
}

// Test the regex and fmt pattern that gets built from config
func TestBuildRotationRegexAndName(t *testing.T) {
	type testCase struct {
		description string
		filename    string
		spacer      int
		num         int
		expectRegex string
		expectFile  string
	}

	testCases := []testCase{
		{
			description: "build regexp with spacer 4",
			filename:    "/tmp/file.txt",
			spacer:      4,
			num:         7,
			expectRegex: `file\.(\d{4})\.txt`,
			expectFile:  "/tmp/file.0007.txt",
		},
		{
			description: "build regexp with spacer 8",
			filename:    "/tmp/file.txt",
			spacer:      8,
			num:         19,
			expectRegex: `file\.(\d{8})\.txt`,
			expectFile:  "/tmp/file.00000019.txt",
		},
		{
			description: "build regexp with no file extension",
			filename:    "/tmp/file",
			spacer:      4,
			num:         121,
			expectRegex: `file\.(\d{4})`,
			expectFile:  "/tmp/file.0121",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			re, err := buildRotationRegex(tc.filename, tc.spacer)
			require.NoError(t, err)
			assert.Equal(t, tc.expectRegex, re.String())
			rotName, err := buildRotationName(tc.filename, tc.spacer, tc.num)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectFile, rotName)
			assert.True(t, re.MatchString(rotName))
		})
	}
}
