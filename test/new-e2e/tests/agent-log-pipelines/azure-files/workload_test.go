// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package azurefiles

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash/crc64"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
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
		entries = append(entries, ledgerEntry{FirstSequence: first, LastSequence: last})
		messages = append(messages, file.lines...)
	}
	expected := expectedSequences(entries)
	assert.Len(t, expected, int(sequence))
	counts := countSequences(messages, expected)
	assert.Len(t, counts, len(expected))
	for sequence, count := range counts {
		assert.Equal(t, 1, count, "sequence %d", sequence)
	}

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

func TestLedgerScriptRecordsThePythonWriterFiles(t *testing.T) {
	python := requirePython(t)
	requireTools(t, "sh", "basename", "date", "wc", "tr", "dd", "sha256sum", "awk", "sed", "grep", "head", "tail", "cut", "mkdir", "sleep")
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
	startScript(t, scripts[ledgerScript], filepath.Join(t.TempDir(), "ledger.log"),
		"LOGWRITER_LOG_DIR="+share,
		"LOGWRITER_CRC64_COMMAND="+crc64Command,
	)
	var ledger []ledgerEntry
	require.Eventually(t, func() bool {
		raw, err := os.ReadFile(ledgerPath)
		if err != nil {
			return false
		}
		ledger, err = decodeJSONLines[ledgerEntry](string(raw), "ledger")
		return err == nil && len(ledger) == selfTestRotations
	}, 30*time.Second, 100*time.Millisecond, "ledger.sh did not record the rotated files")

	sort.Slice(ledger, func(i, j int) bool { return ledger[i].Period < ledger[j].Period })
	targets := targetSequence(t)
	for i, file := range rotatedFiles(t, share) {
		entry := ledger[i]
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
	python := requirePython(t)
	requireTools(t, "sh", "basename", "date", "tr", "sleep")
	spec := testRunSpec(t, runOptions{cells: "file-line"})
	c := spec.cells[0]
	scripts := writeWorkloadScripts(t)
	share := t.TempDir()
	runID := spec.writerRunID(c)

	// The suite's marker ages, scaled down so the appender is done in a
	// fraction of a second.
	delays := markerDelays{survivingMs: 50, lostMs: 100}
	journalPath := filepath.Join(share, postRotationMarkerJournalName)
	appenderLog := filepath.Join(t.TempDir(), "appender.log")
	startScript(t, scripts[appenderScript], appenderLog,
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
	}, 10*time.Second, 20*time.Millisecond, "appender.sh did not start")
	time.Sleep(200 * time.Millisecond)
	runPythonWriterSelfTest(t, python, scripts, spec, c, share)

	names := make(map[string]struct{}, selfTestRotations)
	for i := 0; i < selfTestRotations; i++ {
		names[fmt.Sprintf("app.log.13082026_12%02d", i)] = struct{}{}
	}
	var markers []markerEntry
	require.Eventually(t, func() bool {
		raw, err := os.ReadFile(journalPath)
		if err != nil {
			return false
		}
		journal, err := decodeJSONLines[markerEntry](string(raw), "marker journal")
		markers = markersForFiles(journal, runID, names)
		return err == nil && len(markers) == 2*selfTestRotations
	}, 30*time.Second, 50*time.Millisecond, "appender.sh did not mark every rotation")

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
			runID, rotation, delays.survivingMs, runID, rotation, delays.survivingMs, file.name)
		late := fmt.Sprintf("post_rotation_marker run_id=%s rotation=%d marker_age_ms=%d marker_id=%s-r%d-m%d rotated_file=%s",
			runID, rotation, delays.lostMs, runID, rotation, delays.lostMs, file.name)
		// The rotated file ends in padding without a newline, so the first
		// marker extends the padding line, as it does with the Java writer.
		require.GreaterOrEqual(t, len(file.lines), 2, file.name)
		assert.Regexp(t, `^p{1,512}`+regexp.QuoteMeta(early)+`$`, file.lines[len(file.lines)-2], file.name)
		assert.Equal(t, late, file.lines[len(file.lines)-1], file.name)
		assert.Empty(t, file.tail, file.name)
	}
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
// provisioner, and returns their paths by file name.
func writeWorkloadScripts(t *testing.T) map[string]string {
	t.Helper()
	dir := t.TempDir()
	runtime := testRunSpec(t, runOptions{}).workloadRuntime("registry.example")
	paths := make(map[string]string, len(runtime.scripts))
	for name, script := range runtime.scripts {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte(script), 0o500))
		paths[name] = path
	}
	return paths
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
	logFile, err := os.Create(logPath)
	require.NoError(t, err)
	cmd := exec.Command("sh", script)
	cmd.Env = scriptEnv(vars...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = logFile.Close()
		if t.Failed() {
			if output, err := os.ReadFile(logPath); err == nil {
				t.Logf("%s:\n%s", filepath.Base(script), output)
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
		if match := sequencePattern.FindStringSubmatch(line); len(match) == 2 {
			sequence, err := strconv.ParseInt(match[1], 10, 64)
			require.NoError(t, err)
			sequences = append(sequences, sequence)
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
