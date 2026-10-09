// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package azurefiles

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash/crc64"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests run the ConfigMap scripts of the stock workload on the host and
// check them against the contract of the Java writer, which is what the suite's
// assertions read from a share. logwriter.py runs on a simulated clock, so four
// rotations take well under a second. Each test skips when the host lacks the
// interpreter or the tools its scripts call.

const (
	selfTestStart     = "2026-08-13T12:00:10Z"
	selfTestRotations = completedFiles
)

// javaRecordPattern is a record line of the Java writer, as log4j2.xml renders
// it: %d{yyyy-MM-dd HH:mm:ss.SSS}  %-5level %pid --- [%20t] %-40c{1.} : %msg.
// The writer is PID 1 in its container, the Spring scheduler thread writes
// every record, and %c{1.} shortens com.datadoghq.e2e.logwriter.
var javaRecordPattern = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{3})  (INFO |WARN |ERROR) 1 --- \[        scheduling-1\] c\.d\.e\.l\.LogWriterService {17}: ` +
	`run_id=(\S+) period=(\d{8}T\d{4}Z) sequence=(\d+) record=(\d+) phase=(head|fill) target_bytes=(\d+) host=(\S+) payload=x{128}$`)

// writerFile is a rotated file of a self-test run.
type writerFile struct {
	name    string
	content []byte
	// lines are the newline-terminated lines, without their newline.
	lines []string
	// tail follows the last newline: the writer's padding, then whatever the
	// appender added to the last line.
	tail string
}

func TestPythonWriterKeepsTheJavaRecordAndRotationContract(t *testing.T) {
	python := requirePython(t)
	spec := testRunSpec(t, runOptions{cells: "file-line"})
	c := spec.cells[0]
	scripts := writeWorkloadScripts(t)
	share := t.TempDir()
	runPythonWriterSelfTest(t, python, scripts, spec, c, share)

	targets := targetSequence(t)
	files := rotatedFiles(t, share)
	require.Len(t, files, selfTestRotations)
	runID := spec.writerRunID(c)
	sequence := int64(0)
	for i, file := range files {
		// Each file is named after the minute it covers, like the Log4j2
		// filePattern ${LOG_FILE}.%d{ddMMyyyy_HHmm}.
		minute := fmt.Sprintf("12%02d", i)
		assert.Equal(t, "app.log.13082026_"+minute, file.name)
		period := "20260813T" + minute + "Z"

		// The writer stops at the period's target with raw padding, without a
		// newline, so the next record is never in the rotated file.
		target := targets[i%len(targets)]
		assert.Len(t, file.content, int(target), file.name)
		assert.Regexp(t, `^p{1,512}$`, file.tail, file.name)

		require.NotEmpty(t, file.lines, file.name)
		for record, line := range file.lines {
			match := javaRecordPattern.FindStringSubmatch(line)
			require.NotNil(t, match, "%s line %d is not a Java writer record: %q", file.name, record+1, line)
			sequence++
			assert.Equal(t, runID, match[3])
			assert.Equal(t, period, match[4], "%s holds only its own period", file.name)
			assert.Equal(t, strconv.FormatInt(sequence, 10), match[5], "sequences continue across files")
			assert.Equal(t, strconv.Itoa(record+1), match[6])
			assert.Equal(t, strconv.FormatInt(target, 10), match[8])
			assert.Equal(t, c.writerName, match[9])
			assert.Equal(t, levelForSequence(sequence), match[2], "sequence %d", sequence)
			assert.True(t, strings.HasPrefix(match[1], "2026-08-13 12:"+minute[2:]+":"), "%s: %s is outside its minute", file.name, match[1])
			if record == 0 {
				assert.Equal(t, "head", match[7])
			} else {
				assert.Equal(t, "fill", match[7])
			}
		}
	}

	// The first record is exactly what the Java writer writes at that instant:
	// the first tick comes one second after start-up.
	assert.Equal(t,
		"2026-08-13 12:00:11.000  INFO  1 --- [        scheduling-1] c.d.e.l.LogWriterService                 : "+
			"run_id="+runID+" period=20260813T1200Z sequence=1 record=1 phase=head target_bytes=81792 host="+c.writerName+
			" payload="+strings.Repeat("x", 128),
		files[0].lines[0])

	// A reader that collects every line of the rotated files exactly once
	// passes the suite's sequence assertion.
	entries := make([]ledgerEntry, 0, len(files))
	var messages []string
	for _, file := range files {
		first, last := fileSequences(t, file)
		entries = append(entries, ledgerEntry{RunID: runID, FirstSequence: first, LastSequence: last})
		messages = append(messages, file.lines...)
	}
	expected := expectedRecords(entries)
	assert.Len(t, expected, int(sequence))
	check := checkRecords(expected, nil, countRecords(messages, expected))
	assert.Empty(t, check.missing)
	assert.Empty(t, check.duplicated)

	// The active file holds the next period, which starts the sequence after
	// the last rotated one.
	active, err := os.ReadFile(filepath.Join(share, activeLogName))
	require.NoError(t, err)
	assert.Contains(t, string(active), fmt.Sprintf(" period=20260813T12%02dZ sequence=%d record=1 phase=head ", selfTestRotations, sequence+1))
}

func TestPythonWriterDefersAFirstPeriodTooShortToFill(t *testing.T) {
	python := requirePython(t)
	spec := testRunSpec(t, runOptions{cells: "file-line"})
	scripts := writeWorkloadScripts(t)
	share := t.TempDir()
	// Four seconds before the minute: the Java writer waits for the next one
	// rather than leave a file it cannot fill.
	runPythonWriterSelfTestFrom(t, python, scripts, spec, spec.cells[0], share, "2026-08-13T12:00:56Z", 1)

	files := rotatedFiles(t, share)
	require.Len(t, files, 1)
	assert.Equal(t, "app.log.13082026_1201", files[0].name)
	assert.True(t, strings.HasPrefix(files[0].lines[0], "2026-08-13 12:01:00.000  INFO  1 "), files[0].lines[0])
	assert.Contains(t, files[0].lines[0], " sequence=1 record=1 phase=head ")
}

func TestPythonCRC64MatchesGo(t *testing.T) {
	python := requirePython(t)
	scripts := writeWorkloadScripts(t)
	dir := t.TempDir()
	files := map[string][]byte{
		"check":      []byte("123456789"),
		"crlf":       []byte("first line\r\nsecond line\n"),
		"no-newline": []byte("a single line without newline"),
		"empty":      nil,
		"long":       bytes.Repeat([]byte("0123456789abcdef"), 300),
	}
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), content, 0o600))
	}
	// Go's hash/crc64 check value for the ISO polynomial, which Crc64Test.java
	// asserts too.
	assert.Equal(t, "0xb90956c775a41001", pythonCRC64(t, python, scripts, "bytes", filepath.Join(dir, "check")))
	for name, content := range files {
		path := filepath.Join(dir, name)
		assert.Equal(t, goCRC64(head2048(content)), pythonCRC64(t, python, scripts, "bytes", path), name)
		assert.Equal(t, goCRC64(firstLine(content)), pythonCRC64(t, python, scripts, "line", path), name)
	}
}

// The ledger of the Java writer image scans the rotated files; the stock
// workload's reads the Python writer's journal, and so does the Python port
// the Windows file server runs. For the default rename mode, all three must
// record the same files the same way.
func TestLedgerScriptRecordsThePythonWriterFiles(t *testing.T) {
	for _, run := range []struct{ impl, source string }{
		{shellSidecar, "scan"}, {shellSidecar, "journal"}, {pythonSidecar, "journal"},
	} {
		t.Run(run.impl+"-"+run.source, func(t *testing.T) {
			testLedgerScriptRecordsThePythonWriterFiles(t, run.impl, run.source)
		})
	}
}

func testLedgerScriptRecordsThePythonWriterFiles(t *testing.T, impl, source string) {
	python := requirePython(t)
	if impl == shellSidecar {
		requireTools(t, "sh", "basename", "date", "wc", "tr", "dd", "sha256sum", "awk", "sed", "grep", "head", "tail", "cut", "mkdir", "sleep")
	}
	spec := testRunSpec(t, runOptions{cells: "file-line"})
	c := spec.cells[0]
	scripts := writeWorkloadScripts(t)
	share := t.TempDir()
	runPythonWriterSelfTest(t, python, scripts, spec, c, share)

	// ledger.sh splits the command on blanks, as it does in the pod.
	crc64Command := python + " " + scripts[pythonWriterScript] + " crc64"
	if len(strings.Fields(crc64Command)) != 3 {
		t.Skipf("the CRC64 command %q would not split into its words", crc64Command)
	}
	ledgerPath := filepath.Join(share, ledgerName)
	startSidecar(t, impl, ledgerContainerName, scripts, filepath.Join(t.TempDir(), "ledger.log"),
		"LOGWRITER_LOG_DIR="+share,
		"LOGWRITER_CRC64_COMMAND="+crc64Command,
		"LOGWRITER_LEDGER_SOURCE="+source,
	)
	ledger := waitForLedger(t, ledgerPath, selfTestRotations)

	targets := targetSequence(t)
	for i, file := range rotatedFiles(t, share) {
		entry := ledger[i]
		if source == "journal" {
			// What only the journal has.
			assert.Equal(t, "renamed", entry.Disposition, file.name)
			assert.Equal(t, "file", entry.Observed, file.name)
			assert.Equal(t, int64(len(file.content)), entry.ObservedBytes, file.name)
			assert.Equal(t, string(renameRotation), entry.RotationMode, file.name)
			assert.NotEmpty(t, entry.RotatedAt, file.name)
			entry.Stream, entry.RotationMode, entry.Disposition, entry.Archive = "", "", "", ""
			entry.UnwrittenSequences, entry.AtRiskSequences, entry.WriteTimes, entry.RotatedAt = nil, nil, nil, ""
			entry.Observed, entry.ObservedBytes = "", 0
		}
		first, last := fileSequences(t, file)
		assert.Equal(t, ledgerEntry{
			RunID:           spec.writerRunID(c),
			Period:          fmt.Sprintf("20260813T12%02dZ", i),
			File:            file.name,
			TargetBytes:     targets[i%len(targets)],
			Bytes:           int64(len(file.content)),
			First2048SHA256: sha256Hex(head2048(file.content)),
			First2048CRC64:  goCRC64(head2048(file.content)),
			FirstLineSHA256: sha256Hex(firstLine(file.content)),
			FirstLineCRC64:  goCRC64(firstLine(file.content)),
			FirstSequence:   first,
			LastSequence:    last,
			LineCount:       int64(len(file.lines)),
			DiscoveredAt:    entry.DiscoveredAt,
			ObservedAt:      entry.ObservedAt,
		}, entry, file.name)
	}
}

func TestAppenderScriptMarksThePythonWriterRotations(t *testing.T) {
	forEachSidecar(t, testAppenderScriptMarksThePythonWriterRotations)
}

func testAppenderScriptMarksThePythonWriterRotations(t *testing.T, impl string) {
	python := requirePython(t)
	if impl == shellSidecar {
		requireTools(t, "sh", "basename", "date", "tr", "sleep")
	}
	spec := testRunSpec(t, runOptions{cells: "file-line"})
	c := spec.cells[0]
	scripts := writeWorkloadScripts(t)
	share := t.TempDir()
	runID := spec.writerRunID(c)

	// The suite's marker ages, scaled down so the appender is done in a
	// fraction of a second.
	delays := markerDelays{earlyMs: 50, earlyExpect: markerCollected, lateMs: 100}
	journalPath := filepath.Join(share, postRotationMarkerJournalName)
	appenderLog := filepath.Join(t.TempDir(), "appender.log")
	startSidecar(t, impl, appenderContainerName, scripts, appenderLog,
		"LOGWRITER_LOG_DIR="+share,
		"LOGWRITER_RUN_ID="+runID,
		"LOGWRITER_MARKER_JOURNAL_PATH="+journalPath,
		"LOGWRITER_APPEND_DELAYS_MS="+delays.appenderValue(),
		"LOGWRITER_APPEND_POLL_MS=20",
		"TZ=UTC",
	)
	// Rotated files the appender finds on its first scan predate it and get
	// no markers, so the writer only starts once that scan is over.
	require.Eventually(t, func() bool {
		output, err := os.ReadFile(appenderLog)
		return err == nil && bytes.Contains(output, []byte("appender_ready "))
	}, 10*time.Second, 20*time.Millisecond, "the %s appender did not start", impl)
	time.Sleep(200 * time.Millisecond)
	runPythonWriterSelfTest(t, python, scripts, spec, c, share)

	names := make(map[markerKey]struct{}, selfTestRotations)
	for i := 0; i < selfTestRotations; i++ {
		names[markerKey{runID: runID, file: fmt.Sprintf("app.log.13082026_12%02d", i)}] = struct{}{}
	}
	var markers []markerEntry
	require.Eventually(t, func() bool {
		raw, err := os.ReadFile(journalPath)
		if err != nil {
			return false
		}
		journal, err := decodeJSONLines[markerEntry](string(raw), "marker journal")
		markers = markersForFiles(journal, names)
		return err == nil && len(markers) == 2*selfTestRotations
	}, 30*time.Second, 50*time.Millisecond, "the %s appender did not mark every rotation", impl)

	files := rotatedFiles(t, share)
	require.Len(t, files, selfTestRotations)
	var messages []string
	for _, file := range files {
		messages = append(messages, file.lines...)
	}
	counts := countMarkerIDs(messages)
	for _, marker := range markers {
		assert.Equal(t, fmt.Sprintf("%s-r%d-m%d", runID, marker.Rotation, marker.MarkerAgeMs), marker.MarkerID)
		assert.Equal(t, 1, counts[marker.MarkerID], marker.MarkerID)
	}
	for i, file := range files {
		rotation := i + 1
		early := fmt.Sprintf("post_rotation_marker run_id=%s rotation=%d marker_age_ms=%d marker_id=%s-r%d-m%d rotated_file=%s",
			runID, rotation, delays.earlyMs, runID, rotation, delays.earlyMs, file.name)
		late := fmt.Sprintf("post_rotation_marker run_id=%s rotation=%d marker_age_ms=%d marker_id=%s-r%d-m%d rotated_file=%s",
			runID, rotation, delays.lateMs, runID, rotation, delays.lateMs, file.name)
		// The rotated file ends in padding without a newline, so the first
		// marker extends the padding line, as it does with the Java writer.
		require.GreaterOrEqual(t, len(file.lines), 2, file.name)
		assert.Regexp(t, `^p{1,512}`+regexp.QuoteMeta(early)+`$`, file.lines[len(file.lines)-2], file.name)
		assert.Equal(t, late, file.lines[len(file.lines)-1], file.name)
		assert.Empty(t, file.tail, file.name)
	}
}

// writerLayouts are the shapes the rotation mode tests run each mode in: the
// Java writer's schedule on one app.log, and two paced streams with a short
// period, as a pod would run them.
var writerLayouts = map[string]runOptions{
	"one-file":    {},
	"two-streams": {periodMs: "10000", rateBytesPerSec: "40000", streams: "2"},
}

// recordLinePattern is a writer record with any payload size and period
// format.
var recordLinePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{3}  (?:INFO |WARN |ERROR) 1 --- \[        scheduling-1\] c\.d\.e\.l\.LogWriterService {17}: ` +
	`run_id=(\S+) period=(\d{8}T\d{4}(?:\d{2})?Z) sequence=(\d+) record=\d+ phase=(?:head|fill) target_bytes=\d+ host=\S+ payload=x+$`)

// assertWriteTimes checks the journal's write times of one file: pairs of the
// first sequence of a write and when it returned, both ascending, inside the
// file's sequences, and no later than the journal line itself.
func assertWriteTimes(t *testing.T, entry ledgerEntry) {
	t.Helper()
	require.NotEmpty(t, entry.WriteTimes, "%s has no write times", entry.Period)
	rotatedAt, ok := entry.rotatedAt()
	require.True(t, ok, entry.Period)
	var previous [2]int64
	for i, pair := range entry.WriteTimes {
		assert.GreaterOrEqual(t, pair[0], entry.FirstSequence, "%s write %d", entry.Period, i)
		assert.LessOrEqual(t, pair[0], entry.LastSequence, "%s write %d", entry.Period, i)
		if i > 0 {
			assert.Greater(t, pair[0], previous[0], "%s write %d: the sequences ascend", entry.Period, i)
			assert.Greater(t, pair[1], previous[1], "%s write %d: one pair per millisecond, ascending", entry.Period, i)
		}
		assert.LessOrEqual(t, pair[1], rotatedAt.UnixMilli(), "%s write %d is journalled before it was written", entry.Period, i)
		previous = pair
	}
	// The record the file's last sequence names was written by the last pair
	// or an earlier one, which the dating relies on.
	written, ok := entry.writtenAt(entry.LastSequence)
	assert.True(t, ok, entry.Period)
	assert.False(t, written.After(rotatedAt), entry.Period)
}

// diskRecord is a record found in a file of a stream directory.
type diskRecord struct {
	runID    string
	period   string
	sequence int64
}

func TestPythonWriterRotationModesJournalEveryRecord(t *testing.T) {
	python := requirePython(t)
	scripts := writeWorkloadScripts(t)
	for _, mode := range knownRotationModes {
		for layout, opts := range writerLayouts {
			t.Run(string(mode)+"/"+layout, func(t *testing.T) {
				opts.cells, opts.rotationMode = "file-line", string(mode)
				spec := testRunSpec(t, opts)
				c := spec.cells[0]
				share := t.TempDir()
				runPythonWriterSelfTest(t, python, scripts, spec, c, share)
				streams := spec.streamRunIDs(c)
				for i, dir := range spec.writer.streamDirs() {
					checkStreamFiles(t, mode, filepath.Join(share, dir), dir, streams[i])
				}
			})
		}
	}
}

// checkStreamFiles checks what a stream left on the share against its
// journal: the journal accounts for every record the writer wrote, each
// rotated file holds exactly what its journal line says, and a record is only
// missing from the share when the mode deleted it or put it at risk.
func checkStreamFiles(t *testing.T, mode rotationMode, dir, stream, runID string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, periodsJournalName))
	require.NoError(t, err)
	journal, err := decodeJSONLines[ledgerEntry](string(raw), "journal")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(journal), selfTestRotations)

	files := streamFiles(t, dir)
	seen := make(map[int64]int)
	records := make(map[string][]diskRecord, len(files))
	for name, content := range files {
		records[name] = parseDiskRecords(t, name, content)
		for _, record := range records[name] {
			assert.Equal(t, runID, record.runID, name)
			seen[record.sequence]++
		}
	}

	atRisk := make(map[int64]bool)
	next := int64(1)
	for _, entry := range journal {
		assert.Equal(t, runID, entry.RunID)
		assert.Equal(t, stream, entry.Stream)
		assert.Equal(t, string(mode), entry.RotationMode)
		assert.Equal(t, next, entry.FirstSequence, "%s follows the previous period", entry.Period)
		next = entry.LastSequence + 1
		assert.Empty(t, entry.UnwrittenSequences, entry.Period)
		assert.Equal(t, entry.LastSequence-entry.FirstSequence+1, entry.LineCount, entry.Period)
		assertWriteTimes(t, entry)
		for _, sequences := range entry.AtRiskSequences {
			// The window opens with the first record after the copied period.
			assert.Equal(t, entry.LastSequence+1, sequences[0], entry.Period)
			for sequence := sequences[0]; sequence <= sequences[1]; sequence++ {
				atRisk[sequence] = true
			}
		}

		switch mode {
		case renameRotation, gzipRotation:
			if mode == gzipRotation {
				assert.Equal(t, "compressed", entry.Disposition)
				assert.Equal(t, entry.File+".gz", entry.Archive)
				assert.NoFileExists(t, filepath.Join(dir, entry.File), "the compressed file is deleted")
			} else {
				assert.Equal(t, "renamed", entry.Disposition)
			}
			// The rotated file is exactly what the writer journalled.
			content, ok := files[entry.File]
			require.True(t, ok, "%s is not on the share", entry.File)
			assert.Equal(t, entry.Bytes, int64(len(content)), entry.File)
			assertHeadChecksums(t, entry, content)
			require.NotEmpty(t, records[entry.File], entry.File)
			for i, record := range records[entry.File] {
				assert.Equal(t, entry.FirstSequence+int64(i), record.sequence, entry.File)
				assert.Equal(t, entry.Period, record.period, entry.File)
			}
			assert.Len(t, records[entry.File], int(entry.LineCount), entry.File)
		case copyTruncateRotation:
			assert.Equal(t, "copied", entry.Disposition)
			content, ok := files[entry.File]
			require.True(t, ok, "%s is not on the share", entry.File)
			assertHeadChecksums(t, entry, content)
			// A copy holds its period, and at most what the writer appended
			// while the copy was running.
			for _, record := range records[entry.File] {
				if record.period != entry.Period {
					assert.True(t, atRisk[record.sequence], "%s holds sequence %d of %s, which is not at risk", entry.File, record.sequence, record.period)
				}
			}
		case deleteRecreateRotation:
			assert.Equal(t, "deleted", entry.Disposition)
			assert.Equal(t, activeLogName, entry.File)
		}
	}
	if mode == copyTruncateRotation {
		assert.NotEmpty(t, atRisk, "a copytruncate rotation always has the next head record at risk")
	} else {
		assert.Empty(t, atRisk)
	}

	// Every record up to the last one written is either journalled, and then
	// on the share once unless its mode deleted it or put it at risk, or in
	// the active file the writer has not rotated yet. A self-test of a paced
	// copytruncate writer ends right after a truncation, with an empty active
	// file: its last record is then the end of the last at-risk window.
	active := records[activeLogName]
	lastWritten := next - 1
	for sequence := range atRisk {
		lastWritten = max(lastWritten, sequence)
	}
	if mode != copyTruncateRotation {
		require.NotEmpty(t, active)
	}
	for _, record := range active {
		assert.GreaterOrEqual(t, record.sequence, next, "the active file only holds what is not journalled yet")
		lastWritten = max(lastWritten, record.sequence)
	}
	for sequence := int64(1); sequence <= lastWritten; sequence++ {
		switch {
		case atRisk[sequence]:
			assert.LessOrEqual(t, seen[sequence], 1, "at-risk sequence %d", sequence)
		case sequence < next && mode == deleteRecreateRotation:
			assert.Zero(t, seen[sequence], "sequence %d was deleted with its file", sequence)
		default:
			assert.Equal(t, 1, seen[sequence], "sequence %d", sequence)
		}
	}
}

func assertHeadChecksums(t *testing.T, entry ledgerEntry, content []byte) {
	t.Helper()
	assert.Equal(t, goCRC64(head2048(content)), entry.First2048CRC64, entry.File)
	assert.Equal(t, sha256Hex(head2048(content)), entry.First2048SHA256, entry.File)
	assert.Equal(t, goCRC64(firstLine(content)), entry.FirstLineCRC64, entry.File)
	assert.Equal(t, sha256Hex(firstLine(content)), entry.FirstLineSHA256, entry.File)
}

// streamFiles reads the active and rotated files of a stream directory, by
// name, with a compressed file under the name it had before compression.
func streamFiles(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	files := make(map[string][]byte)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || (name != activeLogName && !strings.HasPrefix(name, activeLogName+".")) {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, name))
		require.NoError(t, err)
		if strings.HasSuffix(name, ".gz") {
			reader, err := gzip.NewReader(bytes.NewReader(content))
			require.NoError(t, err, name)
			content, err = io.ReadAll(reader)
			require.NoError(t, err, name)
			name = strings.TrimSuffix(name, ".gz")
			require.NotContains(t, files, name, "%s is on the share both compressed and not", name)
		}
		files[name] = content
	}
	return files
}

// parseDiskRecords reads the records of a file. Each line is a record; the
// padding of the Java writer's schedule ends a file without a newline.
func parseDiskRecords(t *testing.T, name string, content []byte) []diskRecord {
	t.Helper()
	end := bytes.LastIndexByte(content, '\n')
	assert.Regexp(t, `^p*$`, string(content[end+1:]), "%s ends in something else than padding", name)
	if end < 0 {
		return nil
	}
	var records []diskRecord
	for number, line := range strings.Split(string(content[:end]), "\n") {
		match := recordLinePattern.FindStringSubmatch(line)
		if !assert.NotNil(t, match, "%s line %d is not a writer record: %.200q", name, number+1, line) {
			continue
		}
		sequence, err := strconv.ParseInt(match[3], 10, 64)
		require.NoError(t, err)
		records = append(records, diskRecord{runID: match[1], period: match[2], sequence: sequence})
	}
	return records
}

func TestPythonCopyTruncateWriterAppendsThroughTheCopy(t *testing.T) {
	python := requirePython(t)
	scripts := writeWorkloadScripts(t)
	// An SMB source polls app.log every second, so only the last
	// copyTruncateSMBAtRiskMs before each truncation are at risk.
	spec := testRunSpec(t, runOptions{cells: "smb-copytruncate", smbEnabled: true, periodMs: "10000", rateBytesPerSec: "40000"})
	c := spec.cells[0]
	share := t.TempDir()
	runPythonWriterSelfTest(t, python, scripts, spec, c, share)

	raw, err := os.ReadFile(filepath.Join(share, periodsJournalName))
	require.NoError(t, err)
	journal, err := decodeJSONLines[ledgerEntry](string(raw), "journal")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(journal), selfTestRotations)
	files := streamFiles(t, share)
	headPause := time.Duration(spec.writer.headPauseMs()) * time.Millisecond
	for _, entry := range journal {
		require.Len(t, entry.AtRiskSequences, 1, entry.Period)
		// The next period's head record lands at the boundary, while the
		// copy is running: the copy holds it, and it stays in app.log for
		// the whole hold, so it is not at risk.
		head := entry.LastSequence + 1
		var copied []int64
		for _, record := range parseDiskRecords(t, entry.File, files[entry.File]) {
			if record.period != entry.Period {
				copied = append(copied, record.sequence)
			}
		}
		assert.Equal(t, []int64{head}, copied, "%s holds what was appended during the copy", entry.File)
		// The fill starts after the head pause, which ends less than
		// copyTruncateSMBAtRiskMs before the truncation: every fill record
		// of the hold is at risk, and only those.
		require.Greater(t, headPause, time.Duration(copyTruncateHoldMs-copyTruncateSMBAtRiskMs)*time.Millisecond)
		assert.Equal(t, head+1, entry.AtRiskSequences[0][0], entry.Period)
	}
}

func TestPythonWriterIdlesAfterItsPeriods(t *testing.T) {
	python := requirePython(t)
	scripts := writeWorkloadScripts(t)
	for _, mode := range knownRotationModes {
		t.Run(string(mode), func(t *testing.T) {
			spec := testRunSpec(t, runOptions{cells: "file-line", rotationMode: string(mode), periodMs: "10000", rateBytesPerSec: "40000"})
			c := spec.cells[0]
			share := t.TempDir()
			var vars []string
			for _, v := range spec.writerEnv(c) {
				if v.name == "LOGWRITER_LOG_DIR" {
					v.value = share
				}
				vars = append(vars, v.name+"="+v.value)
			}
			// Far more rotations than the writer's periods allow: the
			// self-test ends once the writer idles.
			cmd := exec.Command(python, scripts[pythonWriterScript], "selftest", "--start", selfTestStart, "--rotations", "100")
			cmd.Env = scriptEnv(append(vars, "HOSTNAME="+c.writerName)...)
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, "logwriter.py selftest: %s", output)
			assert.Contains(t, string(output), fmt.Sprintf("writer_idle run_id=%s periods=%d reason=LOGWRITER_MAX_PERIODS", spec.writerRunID(c), pacedWriterMaxPeriods))

			raw, err := os.ReadFile(filepath.Join(share, periodsJournalName))
			require.NoError(t, err)
			journal, err := decodeJSONLines[ledgerEntry](string(raw), "journal")
			require.NoError(t, err)
			// Every period but the last is rotated; copytruncate's rotator
			// also copies the last one at the next boundary.
			want := pacedWriterMaxPeriods - 1
			if mode == copyTruncateRotation {
				want = pacedWriterMaxPeriods
			}
			assert.Len(t, journal, want)
			assert.GreaterOrEqual(t, len(journal), completedFiles+1, "a cell still gets its files")
		})
	}
}

func TestPythonWriterPacesItsRecords(t *testing.T) {
	python := requirePython(t)
	scripts := writeWorkloadScripts(t)
	spec := testRunSpec(t, runOptions{cells: "file-line", periodMs: "10000", rateBytesPerSec: "40000", streams: "2"})
	c := spec.cells[0]
	share := t.TempDir()
	runPythonWriterSelfTest(t, python, scripts, spec, c, share)

	// Each stream gets half the rate, from the end of the head pause to the
	// end of the period, in records with the paced payload.
	headPause := int64(spec.writer.headPauseMs())
	for _, dir := range spec.writer.streamDirs() {
		raw, err := os.ReadFile(filepath.Join(share, dir, periodsJournalName))
		require.NoError(t, err)
		journal, err := decodeJSONLines[ledgerEntry](string(raw), "journal")
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(journal), selfTestRotations)
		// The first period starts with the writer, one second in.
		for _, entry := range journal[1:] {
			want := (10000 - headPause) * 20000 / 1000
			// A tick of 250ms is the schedule's resolution.
			assert.InDelta(t, want, entry.Bytes, float64(20000/4+2*1200), entry.Period)
			assert.Equal(t, int64(20000*10), entry.TargetBytes, entry.Period)
		}
		content, err := os.ReadFile(filepath.Join(share, dir, journal[1].File))
		require.NoError(t, err)
		line := firstLine(content)
		assert.Contains(t, string(line), " payload="+strings.Repeat("x", pacedWriterPayloadBytes))
		assert.True(t, bytes.HasPrefix(line, []byte("2026-08-13 12:00:20.000  INFO  1 ")), "%s", line)
	}
}

func TestPythonWriterRunsItsStreamsInRealTime(t *testing.T) {
	python := requirePython(t)
	scripts := writeWorkloadScripts(t)
	share := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "writer.log")
	logFile, err := os.Create(logPath)
	require.NoError(t, err)
	defer logFile.Close()

	// One-second periods: each stream thread rotates, compresses and journals
	// on the real clock, and the writer stops on SIGTERM like a pod.
	streams := 3
	cmd := exec.Command(python, scripts[pythonWriterScript])
	cmd.Env = scriptEnv(
		"LOGWRITER_LOG_DIR="+share, "LOGWRITER_RUN_ID=realtime", "HOSTNAME=writer-test",
		"LOGWRITER_PERIOD_MS=1000", "LOGWRITER_HEAD_PAUSE_MS=100", "LOGWRITER_INITIAL_FILL_RUNWAY_MS=0",
		"LOGWRITER_INITIAL_DELAY_MS=0", "LOGWRITER_INTERVAL_MS=20",
		"LOGWRITER_RATE_BYTES_PER_SEC=300000", "LOGWRITER_BUFFER_BYTES=65536", "LOGWRITER_STREAMS="+strconv.Itoa(streams),
		"LOGWRITER_CONSOLE_RECORDS=false", "LOGWRITER_ROTATION_MODE=gzip", "LOGWRITER_GZIP_DELAY_MS=100",
	)
	cmd.Stdout, cmd.Stderr = logFile, logFile
	require.NoError(t, cmd.Start())
	stopped := false
	defer func() {
		if !stopped {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
		if t.Failed() {
			if output, err := os.ReadFile(logPath); err == nil {
				t.Logf("logwriter.py:\n%s", output)
			}
		}
	}()

	journals := func() [][]ledgerEntry {
		all := make([][]ledgerEntry, streams)
		for i := range all {
			raw, err := os.ReadFile(filepath.Join(share, fmt.Sprintf("svc-%d", i+1), periodsJournalName))
			if err == nil {
				all[i], _ = decodeJSONLines[ledgerEntry](string(raw), "journal")
			}
		}
		return all
	}
	require.Eventually(t, func() bool {
		for _, journal := range journals() {
			if len(journal) < 2 {
				return false
			}
		}
		return true
	}, 20*time.Second, 50*time.Millisecond, "every stream must journal two periods")

	require.NoError(t, cmd.Process.Signal(syscall.SIGTERM))
	err = cmd.Wait()
	stopped = true
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 128+int(syscall.SIGTERM), exitErr.ExitCode())

	for i, journal := range journals() {
		next := journal[0].FirstSequence
		assert.Equal(t, int64(1), next)
		for _, entry := range journal {
			assert.Equal(t, fmt.Sprintf("realtime-svc-%d", i+1), entry.RunID)
			assert.Regexp(t, `^\d{8}T\d{6}Z$`, entry.Period)
			assert.Regexp(t, `^app\.log\.\d{8}_\d{6}$`, entry.File)
			assert.Equal(t, next, entry.FirstSequence)
			assert.Positive(t, entry.Bytes)
			next = entry.LastSequence + 1
		}
	}
}

func TestLedgerScriptRecordsTheWriterJournalOfEveryMode(t *testing.T) {
	forEachSidecar(t, testLedgerScriptRecordsTheWriterJournalOfEveryMode)
}

func testLedgerScriptRecordsTheWriterJournalOfEveryMode(t *testing.T, impl string) {
	python := requirePython(t)
	if impl == shellSidecar {
		requireTools(t, "sh", "basename", "date", "wc", "tr", "sed", "mkdir", "sleep")
	}
	scripts := writeWorkloadScripts(t)
	for name, opts := range map[string]runOptions{
		"gzip-two-streams": {rotationMode: "gzip", periodMs: "10000", rateBytesPerSec: "40000", streams: "2"},
		"delete-recreate":  {rotationMode: "delete-recreate"},
		"copytruncate":     {rotationMode: "copytruncate"},
	} {
		t.Run(name, func(t *testing.T) {
			opts.cells = "file-line"
			spec := testRunSpec(t, opts)
			c := spec.cells[0]
			share := t.TempDir()
			runPythonWriterSelfTest(t, python, scripts, spec, c, share)
			startLedgerScript(t, impl, python, scripts, spec, share)

			for _, dir := range spec.writer.streamDirs() {
				raw, err := os.ReadFile(filepath.Join(share, dir, periodsJournalName))
				require.NoError(t, err)
				journal, err := decodeJSONLines[ledgerEntry](string(raw), "journal")
				require.NoError(t, err)
				ledger := waitForLedger(t, filepath.Join(share, dir, ledgerName), len(journal))
				for i, entry := range ledger {
					// The ledger is the journal line with what the share holds
					// for its file.
					observed := entry
					observed.DiscoveredAt, observed.ObservedAt, observed.Observed, observed.ObservedBytes = "", "", "", 0
					assert.Equal(t, journal[i], observed, entry.File)
					assert.NotEmpty(t, entry.DiscoveredAt)
					switch spec.writer.mode {
					case gzipRotation:
						info, err := os.Stat(filepath.Join(share, dir, entry.Archive))
						require.NoError(t, err)
						assert.Equal(t, "archive", entry.Observed, entry.File)
						assert.Equal(t, info.Size(), entry.ObservedBytes, entry.File)
					case deleteRecreateRotation:
						assert.Equal(t, "deleted", entry.Observed, entry.File)
						assert.Equal(t, int64(-1), entry.ObservedBytes, entry.File)
					case copyTruncateRotation:
						info, err := os.Stat(filepath.Join(share, dir, entry.File))
						require.NoError(t, err)
						assert.Equal(t, "file", entry.Observed, entry.File)
						assert.Equal(t, info.Size(), entry.ObservedBytes, entry.File)
						assert.NotEmpty(t, entry.AtRiskSequences, entry.File)
					}
				}
			}
		})
	}
}

// startLedgerScript runs the ledger with the configuration of the cell's
// ledger container, on share.
func startLedgerScript(t *testing.T, impl, python string, scripts map[string]string, spec runSpec, share string) {
	t.Helper()
	var vars []string
	for _, v := range spec.ledgerEnv(spec.cells[0], spec.workloadRuntime("registry.example")) {
		switch v.name {
		case "LOGWRITER_LOG_DIR":
			v.value = share
		case "LOGWRITER_CRC64_COMMAND":
			v.value = python + " " + scripts[pythonWriterScript] + " crc64"
		}
		vars = append(vars, v.name+"="+v.value)
	}
	startSidecar(t, impl, ledgerContainerName, scripts, filepath.Join(t.TempDir(), "ledger.log"), vars...)
}

func waitForLedger(t *testing.T, ledgerPath string, entries int) []ledgerEntry {
	t.Helper()
	var ledger []ledgerEntry
	require.Eventually(t, func() bool {
		raw, err := os.ReadFile(ledgerPath)
		if err != nil {
			return false
		}
		ledger, err = decodeJSONLines[ledgerEntry](string(raw), "ledger")
		return err == nil && len(ledger) == entries
	}, 30*time.Second, 100*time.Millisecond, "the ledger did not record %d files in %s", entries, ledgerPath)
	sort.Slice(ledger, func(i, j int) bool { return ledger[i].Period < ledger[j].Period })
	return ledger
}

func TestAppenderScriptMarksEveryStream(t *testing.T) {
	forEachSidecar(t, testAppenderScriptMarksEveryStream)
}

func testAppenderScriptMarksEveryStream(t *testing.T, impl string) {
	python := requirePython(t)
	if impl == shellSidecar {
		requireTools(t, "sh", "basename", "date", "tr", "sleep")
	}
	spec := testRunSpec(t, runOptions{cells: "file-line", periodMs: "10000", rateBytesPerSec: "40000", streams: "2"})
	c := spec.cells[0]
	scripts := writeWorkloadScripts(t)
	share := t.TempDir()

	delays := markerDelays{earlyMs: 50, earlyExpect: markerCollected, lateMs: 100}
	var vars []string
	for _, v := range spec.appenderEnv(c) {
		switch v.name {
		case "LOGWRITER_LOG_DIR":
			v.value = share
		case "LOGWRITER_APPEND_DELAYS_MS":
			v.value = delays.appenderValue()
		case "LOGWRITER_APPEND_POLL_MS":
			v.value = "20"
		}
		// LOGWRITER_MARKER_JOURNAL_PATH stays the pod's: the streams must not
		// use it.
		vars = append(vars, v.name+"="+v.value)
	}
	appenderLog := filepath.Join(t.TempDir(), "appender.log")
	startSidecar(t, impl, appenderContainerName, scripts, appenderLog, vars...)
	require.Eventually(t, func() bool {
		output, err := os.ReadFile(appenderLog)
		return err == nil && bytes.Count(output, []byte("appender_ready ")) == 2
	}, 10*time.Second, 20*time.Millisecond, "the %s appender did not watch both streams", impl)
	time.Sleep(200 * time.Millisecond)
	runPythonWriterSelfTest(t, python, scripts, spec, c, share)

	streams := spec.streamRunIDs(c)
	for i, dir := range spec.writer.streamDirs() {
		files := rotatedFiles(t, filepath.Join(share, dir))
		require.Len(t, files, selfTestRotations)
		names := make(map[markerKey]struct{}, len(files))
		for _, file := range files {
			names[markerKey{runID: streams[i], file: file.name}] = struct{}{}
		}
		var markers []markerEntry
		require.Eventually(t, func() bool {
			raw, err := os.ReadFile(filepath.Join(share, dir, postRotationMarkerJournalName))
			if err != nil {
				return false
			}
			journal, err := decodeJSONLines[markerEntry](string(raw), "marker journal")
			markers = markersForFiles(journal, names)
			return err == nil && len(markers) == 2*selfTestRotations
		}, 30*time.Second, 50*time.Millisecond, "the %s appender did not mark every rotation of %s", impl, dir)

		files = rotatedFiles(t, filepath.Join(share, dir))
		var messages []string
		for _, file := range files {
			messages = append(messages, file.lines...)
		}
		counts := countMarkerIDs(messages)
		for _, marker := range markers {
			assert.Equal(t, fmt.Sprintf("%s-r%d-m%d", streams[i], marker.Rotation, marker.MarkerAgeMs), marker.MarkerID)
			assert.Equal(t, 1, counts[marker.MarkerID], marker.MarkerID)
		}
		for _, file := range files {
			// A paced file ends with a whole record, so each marker is a line
			// of its own.
			require.GreaterOrEqual(t, len(file.lines), 3, file.name)
			assert.Regexp(t, `^post_rotation_marker run_id=`+regexp.QuoteMeta(streams[i])+` .* marker_age_ms=50 `, file.lines[len(file.lines)-2], file.name)
			assert.Regexp(t, `^post_rotation_marker run_id=`+regexp.QuoteMeta(streams[i])+` .* marker_age_ms=100 `, file.lines[len(file.lines)-1], file.name)
			assert.Empty(t, file.tail, file.name)
		}
	}
	_, err := os.Stat(filepath.Join(share, postRotationMarkerJournalName))
	assert.True(t, os.IsNotExist(err), "the single-file journal is not used by streams")
}

func TestAppenderScriptOnlyMarksRenamedFilesThatAreStillThere(t *testing.T) {
	forEachSidecar(t, testAppenderScriptOnlyMarksRenamedFilesThatAreStillThere)
}

func testAppenderScriptOnlyMarksRenamedFilesThatAreStillThere(t *testing.T, impl string) {
	if impl == shellSidecar {
		requireTools(t, "sh", "basename", "date", "tr", "sleep")
	}
	scripts := writeWorkloadScripts(t)
	share := t.TempDir()
	journalPath := filepath.Join(share, postRotationMarkerJournalName)
	appenderLog := filepath.Join(t.TempDir(), "appender.log")
	startSidecar(t, impl, appenderContainerName, scripts, appenderLog,
		"LOGWRITER_LOG_DIR="+share,
		"LOGWRITER_RUN_ID=run",
		"LOGWRITER_MARKER_JOURNAL_PATH="+journalPath,
		"LOGWRITER_APPEND_DELAYS_MS=300,600",
		"LOGWRITER_APPEND_POLL_MS=20",
	)
	require.Eventually(t, func() bool {
		output, err := os.ReadFile(appenderLog)
		return err == nil && bytes.Contains(output, []byte("appender_ready "))
	}, 10*time.Second, 20*time.Millisecond, "the %s appender did not start", impl)
	time.Sleep(100 * time.Millisecond)

	// A compressed file is not a rotation, and a rotated file deleted before
	// its markers is not recreated by them.
	require.NoError(t, os.WriteFile(filepath.Join(share, "app.log.13082026_1200.gz"), []byte("gzip"), 0o600))
	gone := filepath.Join(share, "app.log.13082026_1201")
	require.NoError(t, os.WriteFile(gone, []byte("record\n"), 0o600))
	require.Eventually(t, func() bool {
		output, err := os.ReadFile(appenderLog)
		return err == nil && bytes.Contains(output, []byte("appender_rotation_detected file=app.log.13082026_1201 "))
	}, 10*time.Second, 10*time.Millisecond)
	require.NoError(t, os.Remove(gone))

	var journal []markerEntry
	require.Eventually(t, func() bool {
		raw, err := os.ReadFile(journalPath)
		if err != nil {
			return false
		}
		journal, err = decodeJSONLines[markerEntry](string(raw), "marker journal")
		return err == nil && len(journal) == 2
	}, 10*time.Second, 50*time.Millisecond)
	for _, marker := range journal {
		assert.Equal(t, "skipped", marker.Status, marker.MarkerID)
		assert.Equal(t, "app.log.13082026_1201", marker.RotatedFile)
	}
	assert.NoFileExists(t, gone)
}

func TestAppenderScriptIdlesWithoutMarkers(t *testing.T) {
	forEachSidecar(t, testAppenderScriptIdlesWithoutMarkers)
}

func testAppenderScriptIdlesWithoutMarkers(t *testing.T, impl string) {
	if impl == shellSidecar {
		requireTools(t, "sh", "sleep")
	}
	scripts := writeWorkloadScripts(t)
	share := t.TempDir()
	appenderLog := filepath.Join(t.TempDir(), "appender.log")
	startSidecar(t, impl, appenderContainerName, scripts, appenderLog,
		"LOGWRITER_LOG_DIR="+share, "LOGWRITER_RUN_ID=run", "LOGWRITER_APPEND_DELAYS_MS=none", "LOGWRITER_APPEND_POLL_MS=20")
	require.Eventually(t, func() bool {
		output, err := os.ReadFile(appenderLog)
		return err == nil && bytes.Contains(output, []byte("appender_idle run_id=run reason=no_markers"))
	}, 10*time.Second, 20*time.Millisecond)
	rotated := filepath.Join(share, "app.log.13082026_1200")
	require.NoError(t, os.WriteFile(rotated, []byte("record\n"), 0o600))
	time.Sleep(300 * time.Millisecond)
	content, err := os.ReadFile(rotated)
	require.NoError(t, err)
	assert.Equal(t, "record\n", string(content))
	assert.NoFileExists(t, filepath.Join(share, postRotationMarkerJournalName))
}

// requirePython returns the python3 that runs logwriter.py, or skips.
func requirePython(t *testing.T) string {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not installed; the stock workload's writer cannot be checked")
	}
	if err := exec.Command(python, "-c", "import sys; sys.exit(sys.version_info < (3, 8))").Run(); err != nil {
		t.Skipf("%s is older than Python 3.8", python)
	}
	return python
}

func requireTools(t *testing.T, tools ...string) {
	t.Helper()
	var missing []string
	for _, tool := range tools {
		if _, err := exec.LookPath(tool); err != nil {
			missing = append(missing, tool)
		}
	}
	if len(missing) > 0 {
		t.Skipf("missing %s", strings.Join(missing, ", "))
	}
}

// writeWorkloadScripts writes the workload ConfigMap files, as embedded in the
// provisioner, and the Windows file server's sidecars.py, and returns their
// paths by file name.
func writeWorkloadScripts(t *testing.T) map[string]string {
	t.Helper()
	dir := t.TempDir()
	runtime := testRunSpec(t, runOptions{}).workloadRuntime("registry.example")
	scripts := maps.Clone(runtime.scripts)
	scripts[sidecarsScript] = sidecarsSource
	paths := make(map[string]string, len(scripts))
	for name, script := range scripts {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte(script), 0o500))
		paths[name] = path
	}
	return paths
}

// The two implementations of the ledger and the appender: the shell scripts
// of a writer pod, and the Python port the Windows file server runs. Both are
// held to the same tests.
const (
	shellSidecar  = "shell"
	pythonSidecar = "python"
)

// forEachSidecar runs test once with each implementation.
func forEachSidecar(t *testing.T, test func(t *testing.T, impl string)) {
	for _, impl := range []string{shellSidecar, pythonSidecar} {
		t.Run(impl, func(t *testing.T) {
			test(t, impl)
		})
	}
}

// startSidecar runs the ledger or the appender container's command until the
// test ends: ledger.sh or appender.sh with sh, or sidecars.py with python3.
func startSidecar(t *testing.T, impl, container string, scripts map[string]string, logPath string, vars ...string) {
	t.Helper()
	if impl == pythonSidecar {
		startCommand(t, logPath, scriptEnv(vars...), requirePython(t), scripts[sidecarsScript], container)
		return
	}
	script := map[string]string{ledgerContainerName: ledgerScript, appenderContainerName: appenderScript}[container]
	startScript(t, scripts[script], logPath, vars...)
}

// scriptEnv is the environment of a script: the pod's variables, PATH to find
// the tools, and nothing else from the host.
func scriptEnv(vars ...string) []string {
	return append([]string{"PATH=" + os.Getenv("PATH")}, vars...)
}

func runPythonWriterSelfTest(t *testing.T, python string, scripts map[string]string, spec runSpec, c cell, share string) {
	t.Helper()
	runPythonWriterSelfTestFrom(t, python, scripts, spec, c, share, selfTestStart, selfTestRotations)
}

// runPythonWriterSelfTestFrom runs the writer with the configuration of the
// cell's writer container, writing to share.
func runPythonWriterSelfTestFrom(t *testing.T, python string, scripts map[string]string, spec runSpec, c cell, share, start string, rotations int) {
	t.Helper()
	var vars []string
	for _, v := range spec.writerEnv(c) {
		if v.name == "LOGWRITER_LOG_DIR" {
			v.value = share
		}
		vars = append(vars, v.name+"="+v.value)
	}
	cmd := exec.Command(python, scripts[pythonWriterScript], "selftest", "--start", start, "--rotations", strconv.Itoa(rotations))
	cmd.Env = scriptEnv(append(vars, "HOSTNAME="+c.writerName)...)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "logwriter.py selftest: %s", output)
}

// startScript runs a looping workload script with sh until the test ends.
func startScript(t *testing.T, script, logPath string, vars ...string) {
	t.Helper()
	startCommand(t, logPath, scriptEnv(vars...), "sh", script)
}

// startCommand runs a looping workload command until the test ends, and
// prints its output when the test failed.
func startCommand(t *testing.T, logPath string, env []string, name string, args ...string) {
	t.Helper()
	logFile, err := os.Create(logPath)
	require.NoError(t, err)
	cmd := exec.Command(name, args...)
	cmd.Env = env
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = logFile.Close()
		if t.Failed() {
			if output, err := os.ReadFile(logPath); err == nil {
				t.Logf("%s:\n%s", strings.Join(append([]string{name}, args...), " "), output)
			}
		}
	})
}

func pythonCRC64(t *testing.T, python string, scripts map[string]string, mode, path string) string {
	t.Helper()
	output, err := exec.Command(python, scripts[pythonWriterScript], "crc64", mode, path).CombinedOutput()
	require.NoError(t, err, "logwriter.py crc64: %s", output)
	return strings.TrimSpace(string(output))
}

// rotatedFiles reads the rotated files of a share, oldest first.
func rotatedFiles(t *testing.T, share string) []writerFile {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(share, activeLogName+".*"))
	require.NoError(t, err)
	sort.Strings(paths)
	files := make([]writerFile, 0, len(paths))
	for _, path := range paths {
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		file := writerFile{name: filepath.Base(path), content: content}
		end := bytes.LastIndexByte(content, '\n')
		if end >= 0 {
			file.lines = strings.Split(string(content[:end]), "\n")
		}
		file.tail = string(content[end+1:])
		files = append(files, file)
	}
	return files
}

func fileSequences(t *testing.T, file writerFile) (int64, int64) {
	t.Helper()
	var sequences []int64
	for _, line := range file.lines {
		if key, ok := parseRecord(line); ok {
			sequences = append(sequences, key.sequence)
		}
	}
	require.NotEmpty(t, sequences, file.name)
	return sequences[0], sequences[len(sequences)-1]
}

func targetSequence(t *testing.T) []int64 {
	t.Helper()
	var targets []int64
	for _, value := range strings.Split(writerTargetSequence, ",") {
		target, err := strconv.ParseInt(value, 10, 64)
		require.NoError(t, err)
		targets = append(targets, target)
	}
	return targets
}

// levelForSequence is the Java writer's level choice.
func levelForSequence(sequence int64) string {
	switch {
	case sequence%20 == 0:
		return "ERROR"
	case sequence%10 == 0:
		return "WARN "
	default:
		return "INFO "
	}
}

// head2048 is what ledger.sh checksums as the head of a file.
func head2048(content []byte) []byte {
	if len(content) > 2048 {
		return content[:2048]
	}
	return content
}

// firstLine is what ledger.sh checksums as the first line of a file.
func firstLine(content []byte) []byte {
	if end := bytes.IndexByte(content, '\n'); end >= 0 {
		content = content[:end]
	}
	return bytes.TrimSuffix(content, []byte("\r"))
}

func goCRC64(content []byte) string {
	return fmt.Sprintf("0x%x", crc64.Checksum(content, crc64.MakeTable(crc64.ISO)))
}

func sha256Hex(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// The Windows file server's tasks, with the environment the test gives them
// and the share moved to a local directory, must produce a ledger and marker
// journal the suite can judge, like a writer pod's.
func TestWindowsFileServerTasksWorkTogether(t *testing.T) {
	python := requirePython(t)
	spec, c := windowsTestSpec(t)
	scripts := writeWorkloadScripts(t)
	share := t.TempDir()
	localEnv := func(task string, overrides map[string]string) []string {
		var vars []string
		for _, v := range spec.windowsTaskEnv(c, task) {
			if override, ok := overrides[v.name]; ok {
				v.value = override
			}
			// The VM's paths, on this host.
			if strings.HasPrefix(v.value, windowsShareDir(c)) {
				v.value = filepath.Join(share, strings.ReplaceAll(strings.TrimPrefix(v.value, windowsShareDir(c)), `\`, "/"))
			}
			vars = append(vars, v.name+"="+v.value)
		}
		return vars
	}
	startCommand(t, filepath.Join(t.TempDir(), "ledger.log"), scriptEnv(localEnv(ledgerContainerName, nil)...),
		python, scripts[sidecarsScript], ledgerContainerName)
	appenderLog := filepath.Join(t.TempDir(), "appender.log")
	// The marker ages scaled down, as in the appender tests.
	startCommand(t, appenderLog, scriptEnv(localEnv(appenderContainerName, map[string]string{
		"LOGWRITER_APPEND_DELAYS_MS": "50,100", "LOGWRITER_APPEND_POLL_MS": "20",
	})...), python, scripts[sidecarsScript], appenderContainerName)
	require.Eventually(t, func() bool {
		output, err := os.ReadFile(appenderLog)
		return err == nil && bytes.Contains(output, []byte("appender_ready "))
	}, 10*time.Second, 20*time.Millisecond, "the appender did not start")
	time.Sleep(200 * time.Millisecond)

	cmd := exec.Command(python, scripts[pythonWriterScript], "selftest", "--start", selfTestStart, "--rotations", strconv.Itoa(selfTestRotations))
	cmd.Env = scriptEnv(localEnv(writerContainerName, nil)...)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "logwriter.py selftest: %s", output)

	ledger := waitForLedger(t, filepath.Join(share, ledgerName), selfTestRotations)
	files := rotatedFiles(t, share)
	require.Len(t, files, selfTestRotations)
	names := make(map[markerKey]struct{}, len(files))
	var messages []string
	for i, file := range files {
		assert.Equal(t, spec.writerRunID(c), ledger[i].RunID)
		assert.Equal(t, file.name, ledger[i].File)
		assert.Equal(t, "file", ledger[i].Observed)
		names[markerKey{runID: spec.writerRunID(c), file: file.name}] = struct{}{}
		messages = append(messages, file.lines...)
	}
	var markers []markerEntry
	require.Eventually(t, func() bool {
		raw, err := os.ReadFile(filepath.Join(share, postRotationMarkerJournalName))
		if err != nil {
			return false
		}
		journal, err := decodeJSONLines[markerEntry](string(raw), "marker journal")
		markers = markersForFiles(journal, names)
		return err == nil && len(markers) == 2*selfTestRotations
	}, 30*time.Second, 50*time.Millisecond, "the appender did not mark every rotation")

	// Every record of the ledger is in the files once, with the cell's
	// records and markers.
	files = rotatedFiles(t, share)
	messages = messages[:0]
	for _, file := range files {
		messages = append(messages, file.lines...)
	}
	expected := expectedRecords(ledger)
	check := checkRecords(expected, nil, countRecords(messages, expected))
	assert.Empty(t, check.missing)
	assert.Empty(t, check.duplicated)
	counts := countMarkerIDs(messages)
	for _, marker := range markers {
		assert.Equal(t, 1, counts[marker.MarkerID], marker.MarkerID)
	}
	assert.Contains(t, files[0].lines[0], " host="+c.writerName+" ")
}
