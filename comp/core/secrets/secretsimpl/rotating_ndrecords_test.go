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

func TestPruneOldEntries(t *testing.T) {
	tmpFileName, mockClock, cleanupFunc := setupRecordsTest(t)
	defer cleanupFunc()

	startingData := []byte(`{"time":"2014-02-04T10:30:00Z","data":[{"handle":"apple"}]}
{"time":"2014-02-04T10:30:10Z","data":[{"handle":"banana"}]}
{"time":"2014-02-04T10:30:20Z","data":[{"handle":"cherry"}]}
`)
	os.WriteFile(tmpFileName, startingData, 0640)

	records := newRotatingNDRecords(tmpFileName, config{retention: time.Second * 25})
	mockClock.Add(30 * time.Second)
	records.Add(mockClock.Now(), []auditRecord{{Handle: "donut"}})

	// test that 2 entries have been pruned and 2 remain
	data, _ := os.ReadFile(tmpFileName)
	expect := `{"time":"2014-02-04T10:30:20Z","data":[{"handle":"cherry"}]}
{"time":"2014-02-04T10:30:30Z","data":[{"handle":"donut"}]}
`
	assert.Equal(t, expect, string(data))
}

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

	records := newRotatingNDRecords(tmpFileName, config{retention: time.Second * 25})
	mockClock.Add(30 * time.Second)
	records.Add(mockClock.Now(), []auditRecord{{Handle: "donut"}})

	data, _ := os.ReadFile(tmpFileName)
	expect := `{"time":"2014-02-04T10:30:20Z","data":[{"handle":"cherry"}]}
{"time":"2014-02-04T10:30:30Z","data":[{"handle":"donut"}]}
`
	assert.Equal(t, expect, string(data))
}

func TestPruneEverything(t *testing.T) {
	tmpFileName, mockClock, cleanupFunc := setupRecordsTest(t)
	defer cleanupFunc()

	startingData := []byte(`{"time":"2014-02-04T10:30:00Z","data":[{"handle":"apple"}]}
`)
	os.WriteFile(tmpFileName, startingData, 0640)

	records := newRotatingNDRecords(tmpFileName, config{retention: time.Second * 25})
	mockClock.Add(30 * time.Second)
	records.Add(mockClock.Now(), []auditRecord{{Handle: "donut"}})

	data, _ := os.ReadFile(tmpFileName)
	expect := `{"time":"2014-02-04T10:30:30Z","data":[{"handle":"donut"}]}
`
	assert.Equal(t, expect, string(data))
}

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
		retention: time.Second * 25,
	})
	mockClock.Add(30 * time.Second)
	records.Add(mockClock.Now(), []auditRecord{{Handle: "donut"}})

	// only 1 record in the current file, the latest
	data, _ := os.ReadFile(tmpFileName)
	expect := `{"time":"2014-02-04T10:30:20Z","data":[{"handle":"cherry"}]}
{"time":"2014-02-04T10:30:30Z","data":[{"handle":"donut"}]}
`
	assert.Equal(t, expect, string(data))

	// there are 0 rotated files
	rotated := records.RotatedFiles()
	assert.Equal(t, 0, len(rotated))
}

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

func TestBuildRotationRegexAndFmtPattern(t *testing.T) {
	type testCase struct {
		description  string
		filename     string
		spacer       int
		expectRegex  string
		expectFmtPat string
	}

	testCases := []testCase{
		{
			description:  "build regexp with spacer 4",
			filename:     "/tmp/file.txt",
			spacer:       4,
			expectRegex:  `file\.(\d{4})\.txt`,
			expectFmtPat: "/tmp/file.%04d.txt",
		},
		{
			description:  "build regexp with spacer 8",
			filename:     "/tmp/file.txt",
			spacer:       8,
			expectRegex:  `file\.(\d{8})\.txt`,
			expectFmtPat: "/tmp/file.%08d.txt",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			re, err := buildRotationRegex(tc.filename, tc.spacer)
			if err != nil {
				t.Error(err)
			}
			assert.Equal(t, tc.expectRegex, re.String())
			fmtPat, err := buildRotationFmtPattern(tc.filename, tc.spacer)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectFmtPat, fmtPat)
		})
	}
}
