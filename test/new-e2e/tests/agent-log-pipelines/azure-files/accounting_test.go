// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package azurefiles

import (
	"context"
	"fmt"
	"math"
	"path"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

// Loss accounting.
//
// An SMB source that drains a rotated file can only lose what it had not read
// when the file went away, and when it knew the file held more, it says so:
// RecordMissedBytes in pkg/logs/tailers/smb/tailer.go adds the bytes to the
// logs-agent BytesMissed expvar, to the logs.bytes_missed telemetry counter,
// to the per-(source, service) missed bytes tracker and to the source's Bytes
// Missed in agent status, and logs a warning that names the file. Of these,
// only the warning can be read per cell and per file:
//   - the expvar and the telemetry counter are Agent-wide;
//   - the tracker is only published by the health platform's
//     log_data_lost_after_rotation issue, which runs every 15 minutes and
//     only when the health platform is enabled;
//   - agent status shows one Bytes Missed per source, which a cell has one
//     of, but names no file.
//
// Each SMB cell has its own storage account, so the warning's smb://<host>/...
// identifier names the cell, and its read path names the rotated file. The
// test attributes the warnings to the ledger's files and checks their sum
// against the Agent-wide expvar, so a warning it could not read (a container
// log the kubelet rotated, a changed message) fails the cell instead of
// letting a loss pass as reported.
//
// What no warning can cover is what the source never saw: the bytes written
// after its last listing of the file and before the file went away. They are
// lost without a trace, which listing-driven polling cannot avoid, so the
// test bounds them instead of forbidding them (see silentLossWindowMs).
//
// So the test requires a report only for the loss the source can have known:
// a lost record older than the window. It cannot tell a source that never
// saw a record from one that saw it and did not report it, and it does not
// try to: a source that reads each listed file to its end right after the
// listing, as the active tailer does, reports about nothing when the writer
// deletes or compresses a file at once. The forced-loss cells therefore pass
// with an Agent that reports nothing, as long as every loss is inside the
// window; the reports of the Agent are exercised by the unit and Samba
// integration tests of pkg/logs/launchers/smb, and by a run where a loss reaches back
// past the window (a listing ahead of the reads, which this suite does not
// make: it needs a backlog past the 4 MiB a poll reads, far more than
// maxLostRecords allows). The evidence says which it was: reported_loss_exercised in
// <cell>-losses.json and a log line.

// lossAccountingToleranceBytes is how far the bytes the Agent reported missed
// for a file may differ from the bytes of the file it never delivered: the
// writer's padding, one record whose start was read before the drain ended,
// and an appender marker the Agent saw listed but did not read.
const lossAccountingToleranceBytes = 4096

// What a correct SMB source can lose of a file that goes away. It reads each
// active file once per poll interval, up to its size when the read opens it,
// so when the file goes away, what it has not read is what was written since
// its last read: at most one poll interval, plus the time a scan takes to
// reach the file (lossScanMarginSeconds). The Agent reports the file's size
// at its last listing minus what it read (UnreadBytes in
// pkg/logs/tailers/smb/tailer.go, a lower bound), so the part of that loss it
// cannot report is what was written since the last listing: the silent loss.
//
// Only a paced writer writes that close to the rotation. The Java schedule
// fills each file right after its head pause, tens of seconds before the file
// goes away, so a correct source loses none of it.
//
// A gzip rotation keeps the rotated file for gzipDelayMs, which is the
// drain's close_timeout: a drain starts at or after the rename and lasts at
// least close_timeout after its last new data, so it is still open when the
// file is compressed away. Its loss is then only explained when the file was
// compressed away while the drain was reading it, or when the drain reached
// its deadline. A drain that ends before the compression with records unread
// is a product failure.
const (
	lossScanMarginSeconds = 1
	// lossClockSkew allows for the writer's and the Agent's clocks, which
	// date the rotation and the report.
	lossClockSkew = time.Second
)

// drainDeadlineReasons start the reason of a drain that ran out of time
// instead of ending because its file stopped growing: since the drain keeps
// reading until its file has had no new data for logs_config.close_timeout, it
// is the deadline of drainMaxCloseTimeouts close timeouts (scanner.pollDrain in
// pkg/logs/launchers/smb). The second is the reason before that change.
var drainDeadlineReasons = []string{
	"SMB rotation drain reached its longest duration",
	"SMB rotation drain timed out",
}

func isDrainDeadlineReason(reason string) bool {
	for _, prefix := range drainDeadlineReasons {
		if strings.HasPrefix(reason, prefix) {
			return true
		}
	}
	return false
}

// lossWindowBytes is what one stream of the writer writes in a poll interval
// and a scan.
func lossWindowBytes(w writerOptions) int64 {
	if !w.paced() {
		return 0
	}
	return streamRateBytesPerSec(w) * (smbPollIntervalSeconds + lossScanMarginSeconds)
}

// streamRateBytesPerSec is what one stream of a paced writer writes per
// second: the pod's rate is shared by its streams.
func streamRateBytesPerSec(w writerOptions) int64 {
	return int64(w.rateBytesPerSec) / int64(max(1, w.streams))
}

// maxLostRecords is how many records of one file a loss-accounted cell may
// lose: those of lossWindowBytes, counted with the paced payload, which is
// less than a record's line, plus one buffered write, which lands at once.
func maxLostRecords(w writerOptions) int {
	if !w.paced() {
		return 0
	}
	return int(math.Ceil(float64(lossWindowBytes(w))/pacedWriterPayloadBytes)) + pacedWriterBufferBytes/pacedWriterPayloadBytes
}

// The silent-loss window.
//
// The source lists a file once per poll interval and reports the bytes the
// last listing showed and it did not read. A record written after that
// listing and before the file is deleted or compressed was never seen, so
// nothing can report it, and it is lost: that is inherent to polling the
// listing, not a defect. The design bounds it to the records written within
// one poll interval and the margin below before the file went away, because
// the file was listed at least once in the poll interval before it went away.
// A record written earlier was in a listing the source acted on, so if it is
// lost, the Agent knew it and has to say so.
//
// The ledger dates the file's rotation (rotated_at, journalled before the
// writer renames, deletes or compresses anything), and the writer's own
// pause dates the end: silentDisposalDelayMs after it. The Python writer's
// journal also dates its writes (write_times: the first sequence of each of
// the last writes, and when it returned), so a record is dated by when it was
// written, which a stalled share can put well after its place at the stream's
// rate: the writer catches up in one write. A journal without them (the Java
// writer's) is dated by the rate: a paced writer writes at its rate, so a
// record was written about as many seconds before the rotation as the bytes
// written after it take at the stream's rate.
const (
	// silentLossMarginMs is added to the poll interval:
	//   - lossScanMarginSeconds, the time between the poll's tick and the
	//     listing the scan makes of the file;
	//   - two writer ticks (writerTickMs): a tick's records are written
	//     together and dated at the end of the write, while the source's read
	//     of the file can have opened a tick before it; the file is journalled
	//     up to a tick after its last write. Without the journal's write
	//     times, a record's age is counted from its bytes at the stream's
	//     rate, so it can also be that much older than the write that held it.
	silentLossMarginMs = lossScanMarginSeconds*1000 + 2*writerTickMs
	// silentLossWindowMs is how long before a file goes away its records may
	// still be lost without a report.
	silentLossWindowMs = smbPollIntervalSeconds*1000 + silentLossMarginMs
)

// silentLossWindow is silentLossWindowMs as a duration.
func silentLossWindow() time.Duration {
	return silentLossWindowMs * time.Millisecond
}

// silentWindow is how long before a file goes away its records may still be
// lost without a report, for a stream of this writer when the record is dated
// by the stream's rate: silentLossWindow, plus the time the stream takes to
// write the 4 KiB tolerance (lossAccountingToleranceBytes). The tolerance
// counts as reported, so the first silent record is dated from the loss the
// report leaves, tolerance included, and the tolerance widens the window by
// its bytes at the stream's rate: 0.02s at 200 kB/s, 0.2s at 20 kB/s, 2s at
// 2 kB/s. The Java schedule has no rate to convert it with. A record the
// journal dates (write_times) is written when the journal says, however long
// its bytes took, so its window is silentLossWindow, without the tolerance.
func silentWindow(w writerOptions) time.Duration {
	window := silentLossWindow()
	if rate := streamRateBytesPerSec(w); rate > 0 {
		window += time.Duration(float64(lossAccountingToleranceBytes) / float64(rate) * float64(time.Second))
	}
	return window
}

// silentDisposalDelayMs is how long after its journal line the writer deletes
// or compresses a file of this mode: the pause of a delete-recreate rotation,
// and the delay of a gzip one. Both are zero when the run forces losses.
func silentDisposalDelayMs(w writerOptions) int {
	switch w.mode {
	case deleteRecreateRotation:
		return w.deletePauseMs()
	case gzipRotation:
		return w.gzipDelayMs()
	}
	return 0
}

// maxSilentBytes is the most a rotation can lose without a report: what a
// stream writes in the silent-loss window. A writer that is not paced writes
// nothing near the rotation.
func maxSilentBytes(w writerOptions) int64 {
	if !w.paced() {
		return 0
	}
	return streamRateBytesPerSec(w) * silentLossWindowMs / 1000
}

// maxSilentRecords is maxSilentBytes in records, counted with the paced
// payload, which is less than a record's line, so the bound is generous.
func maxSilentRecords(w writerOptions) int {
	return int(math.Ceil(float64(maxSilentBytes(w)) / pacedWriterPayloadBytes))
}

// silentLoss is what of a file's loss no report covers.
type silentLoss struct {
	// Records and Bytes are what the report leaves out: the file's last
	// records, since the report covers the first of the lost ones (those the
	// last listing showed), and what was written after it is the end.
	Records int
	Bytes   int64
	// Uncovered is how many of the file's last records the report does not
	// cover, the tolerance included: the oldest of them dates the loss, and
	// the window is widened by the tolerance (silentWindow). At least Records.
	Uncovered int
	// OldestAge is how long before the file went away the oldest uncovered
	// record was written, estimated at the stream's rate, for a journal that
	// does not say when it wrote each record (the Java writer's). Zero when
	// nothing is silent.
	OldestAge time.Duration
}

// assessSilentLoss splits a file's loss into the part the Agent reported and
// the part it could not know of. missing records, of which unreadBytes were
// never collected, are the end of the file; the Agent reported reportedBytes
// of them, the first ones. The tolerance of lossAccountingToleranceBytes (the
// padding, a record whose start was read, a marker) counts as reported, so
// a report that is that close to the loss leaves nothing silent.
//
// The oldest uncovered record dates the loss, and it is counted without the
// tolerance, since the tolerance is not time the source had to list the file in
// (silentWindow adds it to the window instead). Its time is the journal's
// (ledgerEntry.writtenAt) when it has it. Else it is estimated from the end of
// the file: the bytes written after it, at the stream's rate, plus the delay
// between the journal line and the file going away.
func assessSilentLoss(w writerOptions, missing int, unreadBytes, reportedBytes int64) silentLoss {
	if missing <= 0 {
		return silentLoss{}
	}
	uncoveredBytes := unreadBytes - reportedBytes
	silentBytes := uncoveredBytes - lossAccountingToleranceBytes
	if silentBytes <= 0 {
		return silentLoss{}
	}
	line := max(1, unreadBytes/int64(missing))
	loss := silentLoss{
		Records:   int(min(int64(missing), (silentBytes+line-1)/line)),
		Bytes:     silentBytes,
		Uncovered: int(min(int64(missing), (uncoveredBytes+line-1)/line)),
	}
	age := time.Duration(silentDisposalDelayMs(w)) * time.Millisecond
	if rate := streamRateBytesPerSec(w); rate > 0 {
		// The bytes written after the oldest uncovered record: the other
		// uncovered ones.
		after := max(0, uncoveredBytes-line)
		age += time.Duration(float64(after) / float64(rate) * float64(time.Second))
	} else {
		// The Java schedule fills the file right after its head pause and
		// leaves it be until the period ends.
		age += time.Duration(max(0, w.periodMs-w.headPauseMs())) * time.Millisecond
	}
	loss.OldestAge = age
	return loss
}

// missedBytesReportPattern reads the warnings that report missed bytes, with
// the kubelet's timestamp when the log was read with timestamps. Three
// messages, from pkg/logs/tailers/smb/tailer.go and
// pkg/logs/launchers/smb/scanner.go:
//
//	<reason>: <N> bytes of SMB file <identifier> (last read as <path>) were not read and are lost
//	<reason>: <N> bytes of SMB file <identifier> were not read and are lost
//	<reason>: <N> bytes of SMB file smb://<host>/<share> (<FileId ...>) were not read and are lost
//
// The first is RecordMissedBytes, of a file a tailer read. The second is
// RecordMissedBytesOf with the identifier of the path the file was last read at
// (a file whose stored position names a file that is no longer listed). The
// third is RecordMissedBytesOf with the share and the file's FileId, for the
// resume point of a drained file that is no longer listed.
var missedBytesReportPattern = regexp.MustCompile(
	`^(?:(\d{4}-\d{2}-\d{2}T\S+Z) )?(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}) UTC \| [A-Z-]+ \| WARN \| (?:\([^)]*\) \| )?` +
		`(.+?): (\d+) bytes of SMB file (smb://\S+)(?: \((FileId [^)]*)\))?(?: \(last read as ([^)]*)\))? were not read and are lost\s*$`)

// missedBytesReport is one warning of the Agent that reports missed bytes.
type missedBytesReport struct {
	At     time.Time `json:"at"`
	Reason string    `json:"reason"`
	Bytes  int64     `json:"bytes"`
	// Identifier is the file's registry identifier, smb://<host>/<share>/<path>,
	// or smb://<host>/<share> when the warning names the file by FileId.
	Identifier string `json:"identifier"`
	// ReadPath is the path the file was last read at: the warning's, or the
	// identifier's own path when it has none.
	ReadPath string `json:"read_path"`
	// FileID is "FileId <n> created <time>" for a warning that names the file
	// by it, and then the report is of no path.
	FileID string `json:"file_id,omitempty"`
}

// identifierPath returns the path of a registry identifier on its share,
// empty for the share itself.
func identifierPath(identifier string) string {
	_, rest, _ := strings.Cut(strings.TrimPrefix(identifier, "smb://"), "/")
	_, filePath, _ := strings.Cut(rest, "/")
	return filePath
}

// parseMissedBytesReports reads every missed-bytes warning of an Agent log.
func parseMissedBytesReports(log string) []missedBytesReport {
	var reports []missedBytesReport
	for _, line := range strings.Split(log, "\n") {
		match := missedBytesReportPattern.FindStringSubmatch(strings.TrimRight(line, "\r"))
		if match == nil {
			continue
		}
		bytes, err := strconv.ParseInt(match[4], 10, 64)
		if err != nil {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, match[1])
		if err != nil {
			at, _ = time.Parse("2006-01-02 15:04:05", match[2])
		}
		readPath := match[7]
		if match[7] == "" {
			readPath = identifierPath(match[5])
		}
		reports = append(reports, missedBytesReport{
			At: at.UTC(), Reason: match[3], Bytes: bytes, Identifier: match[5], ReadPath: readPath, FileID: match[6],
		})
	}
	return reports
}

func sumReportedBytes(reports []missedBytesReport) int64 {
	var total int64
	for _, report := range reports {
		total += report.Bytes
	}
	return total
}

// smbIdentifier is the registry identifier of a stream's app.log, as the SMB
// tailer names it (tailer.Identifier).
func smbIdentifier(c cell, stream string) string {
	return "smb://" + strings.ToLower(c.host()) + "/" + strings.ToLower(c.shareName) + "/" + path.Join(stream, activeLogName)
}

// reportsOfCell keeps the reports about the cell's share.
func reportsOfCell(c cell, reports []missedBytesReport) []missedBytesReport {
	share := "smb://" + strings.ToLower(c.host()) + "/" + strings.ToLower(c.shareName)
	var kept []missedBytesReport
	for _, report := range reports {
		identifier := strings.ToLower(report.Identifier)
		if identifier == share || strings.HasPrefix(identifier, share+"/") {
			kept = append(kept, report)
		}
	}
	return kept
}

// reportsForEntry returns the reports about one completed file. A rotated
// file keeps its FileId under its rotated name, which the drain reports as
// the read path once it has found the file there. A deleted or truncated
// file is reported under app.log, and so is a rotated file that was already
// gone when the source saw the rotation: those are the reports made under
// app.log between the file's rotation and the next one.
func reportsForEntry(c cell, entry ledgerEntry, nextRotation time.Time, reports []missedBytesReport) []missedBytesReport {
	identifier := smbIdentifier(c, entry.Stream)
	activePath := path.Join(entry.Stream, activeLogName)
	rotatedPath := path.Join(entry.Stream, entry.File)
	start, timed := entry.rotatedAt()
	end := nextRotation
	if end.IsZero() {
		end = start.Add(time.Duration(c.writer.periodMs) * time.Millisecond)
	}
	// The writer's journal has milliseconds, the Agent's log only seconds
	// when the kubelet's timestamps are missing.
	inWindow := func(report missedBytesReport) bool {
		return timed && !report.At.Before(start.Add(-time.Second)) && report.At.Before(end)
	}
	byName := c.writer.mode != copyTruncateRotation && c.writer.mode != deleteRecreateRotation
	var matched []missedBytesReport
	for _, report := range reports {
		if report.Identifier != identifier {
			continue
		}
		if (byName && report.ReadPath == rotatedPath) || (report.ReadPath == activePath && inWindow(report)) {
			matched = append(matched, report)
		}
	}
	return matched
}

// cellReportsInSpan returns the cell's reports that name a file by its path
// and were made while the ledger's files went away: from the first rotation
// to one period after the last. A report that names a file by its FileId has
// no stream to compare, and one made before the first rotation of the ledger
// or a period after its last is of an earlier or later run of a kept stack.
func cellReportsInSpan(c cell, ledger []ledgerEntry, reports []missedBytesReport) []missedBytesReport {
	var first, last time.Time
	for _, entry := range ledger {
		if at, ok := entry.rotatedAt(); ok {
			if first.IsZero() || at.Before(first) {
				first = at
			}
			if at.After(last) {
				last = at
			}
		}
	}
	if first.IsZero() {
		return nil
	}
	from, to := first.Add(-time.Second), last.Add(time.Duration(c.writer.periodMs)*time.Millisecond)
	var kept []missedBytesReport
	for _, report := range reportsOfCell(c, reports) {
		if report.FileID != "" || report.At.Before(from) || !report.At.Before(to) {
			continue
		}
		kept = append(kept, report)
	}
	return kept
}

// foreignReports returns the cell's reports, made while the ledger's files
// went away, that name a file no stream of the ledger has: the Agent said
// the bytes were lost from another file. A loss the report was needed for
// would otherwise pass as one that needed none, since a report under another
// identifier attributes to no file.
func foreignReports(c cell, ledger []ledgerEntry, reports []missedBytesReport) []missedBytesReport {
	streams := make(map[string]bool)
	for _, entry := range ledger {
		streams[smbIdentifier(c, entry.Stream)] = true
	}
	var foreign []missedBytesReport
	for _, report := range cellReportsInSpan(c, ledger, reports) {
		if !streams[report.Identifier] {
			foreign = append(foreign, report)
		}
	}
	return foreign
}

// strayReports is the bytes the Agent reported under one file of the cell's
// own streams that no file of the ledger lost: the report names the right
// stream, but not a file that lost records.
type strayReports struct {
	Identifier string
	ReadPath   string
	Bytes      int64
	Reports    int
	// Why says what the reports fail to match.
	Why string
}

// strayReportsOf groups, by the file they name, the cell's reports that name
// one of its own streams and are no report of a file that lost records
// (losses[].Reports): one that matches no file of the ledger (a read path or a
// time that is none of its files'), or the file of an asserted entry that lost
// no record, since a file whose records were all collected has nothing to
// report beyond the tolerance (lossAccountingToleranceBytes: a marker the
// source listed and did not read). Without it a report made under the wrong
// file of the same stream, or in the wrong time slot of a delete-recreate
// stream, attributes to no file and the loss it was needed for passes as one
// that needed none (assertLossesExplained only reads the reports it matched).
// A report that matches a ledger entry that is not asserted is left out: that
// file's records are not checked, so its report cannot be judged.
func strayReportsOf(c cell, ledger, asserted []ledgerEntry, losses []fileLoss, reports []missedBytesReport) []strayReports {
	streams := make(map[string]bool)
	for _, entry := range ledger {
		streams[smbIdentifier(c, entry.Stream)] = true
	}
	isAsserted := make(map[fileKey]bool, len(asserted))
	for _, entry := range asserted {
		isAsserted[fileKey{runID: entry.RunID, period: entry.Period}] = true
	}
	var attributed []missedBytesReport
	for _, loss := range losses {
		attributed = append(attributed, loss.Reports...)
	}
	next := nextRotations(ledger)
	type fileOfReport struct{ identifier, readPath string }
	grouped := make(map[fileOfReport]*strayReports)
	var order []fileOfReport
	for _, report := range cellReportsInSpan(c, ledger, reports) {
		if !streams[report.Identifier] || slices.Contains(attributed, report) {
			continue
		}
		why, judged := "the Agent's report matches no file of the ledger (not by its read path, nor by the time of its rotation)", true
		for _, entry := range ledger {
			key := fileKey{runID: entry.RunID, period: entry.Period}
			if len(reportsForEntry(c, entry, next[key], []missedBytesReport{report})) == 0 {
				continue
			}
			judged = isAsserted[key]
			why = fmt.Sprintf("%s of %s lost no record", entry.File, entry.RunID)
			break
		}
		if !judged {
			continue
		}
		key := fileOfReport{report.Identifier, report.ReadPath}
		if grouped[key] == nil {
			grouped[key] = &strayReports{Identifier: report.Identifier, ReadPath: report.ReadPath, Why: why}
			order = append(order, key)
		}
		grouped[key].Bytes += report.Bytes
		grouped[key].Reports++
	}
	var stray []strayReports
	for _, key := range order {
		stray = append(stray, *grouped[key])
	}
	return stray
}

// assertNoForeignReports fails the cell for each of its reports that names a
// file of no stream of its ledger (foreignReports), and for each file of its
// own streams that the Agent reported more than the tolerance for without the
// file having lost a record (strayReportsOf). The tolerance keeps a marker the
// source listed and did not read from failing a correct source, so a report of
// that size made under the wrong file is not caught.
func assertNoForeignReports(t assert.TestingT, c cell, ledger, asserted []ledgerEntry, losses []fileLoss, reports []missedBytesReport) {
	for _, report := range foreignReports(c, ledger, reports) {
		assert.Fail(t, "missed bytes reported for another file",
			"%s: the Agent reported %d missed bytes at %s under %s (read as %q), which is not a file of this cell's streams; the report was attributed to the wrong file",
			c.name, report.Bytes, report.At.Format(time.RFC3339), report.Identifier, report.ReadPath)
	}
	for _, stray := range strayReportsOf(c, ledger, asserted, losses, reports) {
		if stray.Bytes <= lossAccountingToleranceBytes {
			continue
		}
		assert.Fail(t, "missed bytes reported for a file that lost nothing",
			"%s: the Agent reported %d missed bytes in %d report(s) under %s (read as %q), more than the %d bytes of tolerance, but %s; the report was attributed to the wrong file",
			c.name, stray.Bytes, stray.Reports, stray.Identifier, stray.ReadPath, lossAccountingToleranceBytes, stray.Why)
	}
}

// fileKey identifies one completed file of one stream.
type fileKey struct {
	runID  string
	period string
}

// nextRotations maps each ledger entry to the time of its stream's next
// rotation, when the ledger has it.
func nextRotations(ledger []ledgerEntry) map[fileKey]time.Time {
	next := make(map[fileKey]time.Time)
	for _, entries := range ledgerByStream(ledger) {
		sorted := append([]ledgerEntry(nil), entries...)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].Period < sorted[j].Period })
		for i := 0; i+1 < len(sorted); i++ {
			if at, ok := sorted[i+1].rotatedAt(); ok {
				next[fileKey{runID: sorted[i].RunID, period: sorted[i].Period}] = at
			}
		}
	}
	return next
}

// recordLineBytes returns the size of every collected expected record in its
// file: the message and its newline.
func recordLineBytes(messages []string, expected map[recordKey]struct{}) map[recordKey]int64 {
	sizes := make(map[recordKey]int64, len(expected))
	for _, message := range messages {
		key, ok := parseRecord(message)
		if !ok {
			continue
		}
		if _, wanted := expected[key]; wanted {
			sizes[key] = int64(len(message)) + 1
		}
	}
	return sizes
}

// fileLoss is what one completed file lost and what the Agent reported.
type fileLoss struct {
	RunID          string `json:"run_id"`
	Period         string `json:"period"`
	File           string `json:"file"`
	FirstSequence  int64  `json:"first_sequence"`
	LastSequence   int64  `json:"last_sequence"`
	MissingRecords int    `json:"missing_records"`
	MissingRanges  string `json:"missing_ranges"`
	// AllowedRecords is how many records the cell's rotation may lose of
	// one file (maxLostRecords), and CollectedRecords how many of the
	// file's written records were collected.
	AllowedRecords   int `json:"allowed_records"`
	CollectedRecords int `json:"collected_records"`
	// Suffix says the missing records are the file's last ones: a drain cut
	// short can lose nothing else.
	Suffix bool `json:"suffix"`
	// UnreadBytes are the file's bytes that reached no collected record:
	// what the writer wrote minus the collected records' lines.
	UnreadBytes   int64 `json:"unread_bytes"`
	ReportedBytes int64 `json:"reported_bytes"`
	// The silent loss: what of the loss the report does not cover (see
	// assessSilentLoss), which can only be what was written since the source's
	// last listing of the file. It is checked against the window before the
	// file went away (silentLossWindowMs) and the bound of one window of the
	// stream's rate (maxSilentRecords).
	SilentRecords        int   `json:"silent_records"`
	AllowedSilentRecords int   `json:"allowed_silent_records"`
	SilentBytes          int64 `json:"silent_bytes"`
	// DisposedAt is when the writer deleted or compressed the file, from the
	// ledger's rotated_at, and SilentWindowStart the earliest write that
	// can be lost without a report (silentWindow for a record dated by the
	// rate, silentLossWindow for one the journal dates). OldestSilentAt is the
	// write of the oldest record the report leaves uncovered (zero without a
	// silent loss), taken from the journal's write times when it has them
	// (OldestSilentDatedBy "journal"), else estimated at the stream's rate
	// ("rate").
	DisposedAt          time.Time           `json:"disposed_at"`
	SilentWindowStart   time.Time           `json:"silent_window_start"`
	OldestSilentAt      time.Time           `json:"oldest_silent_at"`
	OldestSilentDatedBy string              `json:"oldest_silent_dated_by,omitempty"`
	RotatedAt           time.Time           `json:"rotated_at"`
	Reports             []missedBytesReport `json:"reports"`
}

// explainLosses groups the records never collected by the file that held
// them, and attributes the Agent's missed-bytes reports to each file.
func explainLosses(c cell, ledger, asserted []ledgerEntry, check recordCheck, lineBytes map[recordKey]int64, reports []missedBytesReport) []fileLoss {
	if len(check.missing) == 0 {
		return nil
	}
	missing := make(map[recordKey]bool, len(check.missing))
	for _, key := range check.missing {
		missing[key] = true
	}
	next := nextRotations(ledger)
	entries := append([]ledgerEntry(nil), asserted...)
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].RunID != entries[j].RunID {
			return entries[i].RunID < entries[j].RunID
		}
		return entries[i].Period < entries[j].Period
	})

	var losses []fileLoss
	for _, entry := range entries {
		var lost []recordKey
		var collectedBytes int64
		collected := 0
		expectedAfterFirstLoss := 0
		for sequence := entry.FirstSequence; sequence <= entry.LastSequence; sequence++ {
			if inRanges(entry.UnwrittenSequences, sequence) {
				continue
			}
			key := recordKey{runID: entry.RunID, sequence: sequence}
			if len(lost) > 0 || missing[key] {
				expectedAfterFirstLoss++
			}
			if missing[key] {
				lost = append(lost, key)
			} else {
				collected++
				collectedBytes += lineBytes[key]
			}
		}
		if len(lost) == 0 {
			continue
		}
		matched := reportsForEntry(c, entry, next[fileKey{runID: entry.RunID, period: entry.Period}], reports)
		rotatedAt, _ := entry.rotatedAt()
		loss := fileLoss{
			RunID: entry.RunID, Period: entry.Period, File: entry.File,
			FirstSequence: entry.FirstSequence, LastSequence: entry.LastSequence,
			MissingRecords: len(lost), MissingRanges: formatRecordRanges(lost),
			AllowedRecords: maxLostRecords(c.writer), CollectedRecords: collected,
			Suffix:               expectedAfterFirstLoss == len(lost),
			UnreadBytes:          max(0, entry.Bytes-collectedBytes),
			ReportedBytes:        sumReportedBytes(matched),
			AllowedSilentRecords: maxSilentRecords(c.writer),
			RotatedAt:            rotatedAt,
			Reports:              matched,
		}
		silent := assessSilentLoss(c.writer, len(lost), loss.UnreadBytes, loss.ReportedBytes)
		allowJournalledStalls(c.writer, entry, rotatedAt, &loss)
		loss.SilentRecords, loss.SilentBytes = silent.Records, silent.Bytes
		if !rotatedAt.IsZero() {
			loss.DisposedAt = rotatedAt.Add(time.Duration(silentDisposalDelayMs(c.writer)) * time.Millisecond)
			loss.SilentWindowStart = loss.DisposedAt.Add(-silentWindow(c.writer))
			if silent.Records > 0 {
				// The lost records are in sequence order, and the report
				// covers the first of them.
				oldest := lost[len(lost)-silent.Uncovered]
				if at, ok := entry.writtenAt(oldest.sequence); ok {
					loss.OldestSilentAt, loss.OldestSilentDatedBy = at, "journal"
					// The journal says when the record was written, so the
					// time the tolerance's bytes take at the stream's rate
					// (a rate estimate) does not widen the window.
					loss.SilentWindowStart = loss.DisposedAt.Add(-silentLossWindow())
				} else {
					loss.OldestSilentAt, loss.OldestSilentDatedBy = loss.DisposedAt.Add(-silent.OldestAge), "rate"
				}
			}
		}
		losses = append(losses, loss)
	}
	return losses
}

// allowJournalledStalls raises the record bounds of a file to the records the
// journal says the writer wrote in the window before the file went away. A
// stalled share makes the writer catch up in one write, so more bytes than
// the stream's rate gives can land in the last seconds (see
// MAX_CATCH_UP_MS in workload/logwriter.py): the rate-based bounds
// (maxSilentRecords, maxLostRecords) would then fail a correct source for the
// writer's burst. They stay the floor, so the bounds never get stricter. The
// loss bound only applies to a file that the writer disposes of while the
// source reads it up to then; a gzip file kept for its delay lost its
// records before the compression.
func allowJournalledStalls(w writerOptions, entry ledgerEntry, rotatedAt time.Time, loss *fileLoss) {
	if !w.paced() || rotatedAt.IsZero() {
		return
	}
	disposedAt := rotatedAt.Add(time.Duration(silentDisposalDelayMs(w)) * time.Millisecond)
	written, ok := entry.recordsWrittenSince(disposedAt.Add(-silentWindow(w)))
	if !ok {
		return
	}
	loss.AllowedSilentRecords = max(loss.AllowedSilentRecords, written)
	if w.mode == deleteRecreateRotation || (w.mode == gzipRotation && w.forceLoss) {
		loss.AllowedRecords = max(loss.AllowedRecords, written)
	}
}

// assertLossesExplained requires every lost record to be one the cell's
// rotation can lose and the Agent accounted for: only the end of a file, no
// more of it than was written in the last poll interval and scan, never all of
// it, for gzip only while the drain was still reading, and every record the
// Agent did not report missed written in the silent-loss window before the
// file went away (assertSilentLossInWindow).
func assertLossesExplained(t assert.TestingT, c cell, losses []fileLoss) {
	for _, loss := range losses {
		file := fmt.Sprintf("%s of %s (sequences %d-%d)", loss.File, loss.RunID, loss.FirstSequence, loss.LastSequence)
		if !loss.Suffix {
			assert.Fail(t, "records lost inside a file",
				"%s: %s lost %d records that are not its last ones (%s); a drain cut short can only lose the end of a file",
				c.name, file, loss.MissingRecords, loss.MissingRanges)
			continue
		}
		switch {
		case loss.CollectedRecords == 0:
			assert.Fail(t, "a whole file lost",
				"%s: %s lost every one of its %d records (%s); a source that reads the file while it is written loses at most its end",
				c.name, file, loss.MissingRecords, loss.MissingRanges)
		case loss.MissingRecords > loss.AllowedRecords:
			assert.Fail(t, "more lost than the rotation allows",
				"%s: %s lost its last %d records (%s), more than the %d a %s rotation can take from a source that reads every %ds (%s)",
				c.name, file, loss.MissingRecords, loss.MissingRanges, loss.AllowedRecords, c.writer.mode, smbPollIntervalSeconds, lossBasis(c.writer))
		}
		if problem := gzipLossProblem(c, loss); problem != "" {
			assert.Fail(t, "drain ended before the compression", "%s: %s %s", c.name, file, problem)
		}
		assertSilentLossInWindow(t, c, file, loss)
		if loss.ReportedBytes > loss.UnreadBytes+lossAccountingToleranceBytes {
			assert.Fail(t, "missed bytes do not match the file",
				"%s: the Agent reported %d missed bytes for %s, more than the %d bytes of it that were never collected; the reports were attributed to the wrong file",
				c.name, loss.ReportedBytes, file, loss.UnreadBytes)
		}
	}
}

// assertSilentLossInWindow requires what the Agent did not report of a file's
// loss to be what listing-driven polling cannot see: the records written in
// the silent-loss window before the file was deleted or compressed, and no
// more of them than the writer writes in it. A record lost earlier was in a
// listing the source acted on, so the Agent had to report it.
func assertSilentLossInWindow(t assert.TestingT, c cell, file string, loss fileLoss) {
	if loss.SilentRecords == 0 {
		return
	}
	if loss.SilentRecords > loss.AllowedSilentRecords {
		assert.Fail(t, "silent loss beyond the bound",
			"%s: %s lost about %d records (%d bytes) that the Agent did not report missed, more than the %d a %s rotation can lose without a report: %s",
			c.name, file, loss.SilentRecords, loss.SilentBytes, loss.AllowedSilentRecords, c.writer.mode, silentBasis(c.writer))
	}
	switch {
	case loss.DisposedAt.IsZero():
		assert.Fail(t, "silent loss that cannot be dated",
			"%s: %s lost about %d records without a report, and the ledger has no rotation time to date them against the window", c.name, file, loss.SilentRecords)
	case loss.OldestSilentAt.Before(loss.SilentWindowStart):
		assert.Fail(t, "silent loss outside the window",
			"%s: %s lost about %d records (%d bytes) without a report, the oldest written about %s before the file went away (%s; dated from the %s), outside the %s that a poll interval, its margin and, for a record dated by the rate, the tolerance allow: "+
				"the source listed the file after that, so it knew of those bytes and had to report them (if the Agent's scans were slow, for example a throttled share, the window is too tight for this run: look for slow scans in the Agent's log before blaming the product)",
			c.name, file, loss.SilentRecords, loss.SilentBytes, loss.DisposedAt.Sub(loss.OldestSilentAt).Round(10*time.Millisecond),
			loss.DisposedAt.Format(time.RFC3339Nano), datedByName(loss.OldestSilentDatedBy), loss.DisposedAt.Sub(loss.SilentWindowStart))
	}
}

// datedByName says where a silent loss's time comes from.
func datedByName(datedBy string) string {
	if datedBy == "journal" {
		return "writer's journalled write times"
	}
	return "stream's rate"
}

// silentBasis says what a writer's silent-loss bound comes from.
func silentBasis(w writerOptions) string {
	if !w.paced() {
		return "the Java schedule fills each file right after its head pause, so nothing is written near the rotation"
	}
	return fmt.Sprintf("%d B/s per stream for %s (a %ds poll interval and %dms of margin), in records of at least %d bytes",
		streamRateBytesPerSec(w), silentLossWindow(), smbPollIntervalSeconds, silentLossMarginMs, pacedWriterPayloadBytes)
}

// lossBasis says what a writer's loss bound comes from.
func lossBasis(w writerOptions) string {
	if !w.paced() {
		return "the Java schedule fills each file right after its head pause, so nothing is left to lose"
	}
	return fmt.Sprintf("%d B/s per stream for %ds, plus one %d-byte write, in records of at least %d bytes",
		w.rateBytesPerSec/max(1, w.streams), smbPollIntervalSeconds+lossScanMarginSeconds, pacedWriterBufferBytes, pacedWriterPayloadBytes)
}

// gzipLossProblem says why a gzip file's loss is not explained by its
// compression: a report made before the writer compressed the rotated file
// away, by a drain that had not timed out, means the drain stopped while the
// file was still there to read.
func gzipLossProblem(c cell, loss fileLoss) string {
	if c.writer.mode != gzipRotation || c.writer.gzipDelayMs() == 0 || loss.RotatedAt.IsZero() {
		return ""
	}
	compressedAt := loss.RotatedAt.Add(time.Duration(c.writer.gzipDelayMs()) * time.Millisecond)
	for _, report := range loss.Reports {
		if isDrainDeadlineReason(report.Reason) || !report.At.Before(compressedAt.Add(-lossClockSkew)) {
			continue
		}
		return fmt.Sprintf("was reported lost (%q) %s after its rotation, before the writer compressed it away %s after the rotation: the drain ended with the rotated file still there to read",
			report.Reason, report.At.Sub(loss.RotatedAt).Round(100*time.Millisecond), time.Duration(c.writer.gzipDelayMs())*time.Millisecond)
	}
	return ""
}

func assertNoDuplicates(t assert.TestingT, c cell, check recordCheck) {
	assert.Zero(t, len(check.duplicated),
		"%s: %d of %d expected records were collected more than once: %s",
		c.name, len(check.duplicated), check.expected, formatRecordRanges(check.duplicated))
}

// assertLossesReported checks the cell's lost records against the Agent's
// missed-bytes reports.
func (suite *azureFilesSuite) assertLossesReported(t assert.TestingT, c cell, ledger, asserted []ledgerEntry, check recordCheck, lineBytes map[recordKey]int64) {
	if len(check.missing) == 0 {
		return
	}
	byPod, err := suite.missedBytesReports()
	if !assert.NoError(t, err, "%s: read the Agent's missed-bytes reports", c.name) {
		return
	}
	var reports []missedBytesReport
	for _, podReports := range byPod {
		reports = append(reports, podReports...)
	}
	losses := explainLosses(c, ledger, asserted, check, lineBytes, reports)
	assertLossesExplained(t, c, losses)
	assertNoForeignReports(t, c, ledger, asserted, losses, reports)
}

// missedBytesReports reads the missed-bytes warnings of every Agent pod's core
// Agent container, by pod.
func (suite *azureFilesSuite) missedBytesReports() (map[string][]missedBytesReport, error) {
	pods, err := suite.agentPods()
	if err != nil {
		return nil, err
	}
	reports := make(map[string][]missedBytesReport, len(pods))
	for _, pod := range pods {
		log, err := suite.Env().KubernetesCluster.Client().CoreV1().Pods(agentNamespace).
			GetLogs(pod.Name, &corev1.PodLogOptions{Container: "agent", Timestamps: true}).DoRaw(context.Background())
		if err != nil {
			return nil, fmt.Errorf("read the agent container log of %s: %w", pod.Name, err)
		}
		reports[pod.Name] = parseMissedBytesReports(string(log))
	}
	return reports, nil
}

// checkMissedBytesTotal requires the Agent-wide BytesMissed counter to be the
// sum of the warnings the Agent logged, read just before and just after it:
// every missed byte then has a warning that says which file it was lost from.
func checkMissedBytesTotal(before, after []missedBytesReport, agentTotal int64) error {
	low, high := sumReportedBytes(before), sumReportedBytes(after)
	if agentTotal < low || agentTotal > high {
		return fmt.Errorf("the Agent counts %d missed bytes, but its log reports %d to %d: some missed bytes have no warning to say which file lost them (was the container log rotated?)", agentTotal, low, high)
	}
	return nil
}

// silentRecordsOf is how many records the Agent did not report missed, of
// all the files that lost records.
func silentRecordsOf(losses []fileLoss) int {
	total := 0
	for _, loss := range losses {
		total += loss.SilentRecords
	}
	return total
}

// checkMissedBytesTotals records a loss-accounted cell's losses and reports in
// the evidence, and, when every cell of the run is an SMB cell, checks that
// the Agent logged a warning for every missed byte it counted. File cells
// count missed bytes too, under warnings of their own.
func (suite *azureFilesSuite) checkMissedBytesTotals(c cell, ledger, asserted []ledgerEntry, check recordCheck, lineBytes map[recordKey]int64) {
	suite.T().Helper()
	byPod, err := suite.missedBytesReports()
	require.NoError(suite.T(), err)
	var all []missedBytesReport
	for _, reports := range byPod {
		all = append(all, reports...)
	}
	losses := explainLosses(c, ledger, asserted, check, lineBytes, all)
	cellReports := reportsOfCell(c, all)
	suite.T().Logf("%s: %d of %d asserted files lost records, %d records in all, %d of them without a report (within %s of the file going away); the Agent made %d missed-bytes reports for the cell, %d bytes in all",
		c.name, len(losses), len(asserted), len(check.missing), silentRecordsOf(losses), silentWindow(c.writer), len(cellReports), sumReportedBytes(cellReports))
	exercised := len(losses) > 0
	reportedBytes := int64(0)
	for _, loss := range losses {
		reportedBytes += loss.ReportedBytes
	}
	if exercised && reportedBytes == 0 {
		suite.T().Logf("%s: the Agent reported none of the %d lost records: they are all inside the %s window before their files went away, which no listing could have shown it, so this run did not exercise its reporting of unread bytes (see README.md, The silent-loss bound)",
			c.name, len(check.missing), silentWindow(c.writer))
	}
	if !exercised {
		hint := fmt.Sprintf("set %s=%s with a paced writer to make the rotations lose data (see README.md)", runForceLoss, runForceLossValue)
		if c.writer.forceLoss {
			hint = "even with forced losses: the source read every file to its end before it went away; a higher rate makes that less likely"
		}
		suite.T().Logf("%s: nothing was lost, so this run did not exercise the loss accounting; %s", c.name, hint)
	}
	if evidence, err := suite.evidenceDir(); err == nil {
		evidence.writeJSON(c.name+"-losses.json", map[string]any{
			"loss_exercised": exercised, "reported_loss_exercised": reportedBytes > 0, "force_loss": c.writer.forceLoss,
			"allowed_records_per_file": maxLostRecords(c.writer), "losses": losses, "reports": cellReports,
			// What may be lost without a report: the records written in
			// this window before a file goes away, and at most as many.
			"silent_loss_window_ms": silentWindow(c.writer).Milliseconds(), "allowed_silent_records_per_file": maxSilentRecords(c.writer),
			"silent_records": silentRecordsOf(losses),
		})
	}

	if suite.spec.hasReader(fileReader) {
		suite.T().Logf("%s: not checking the Agent-wide BytesMissed counter against the SMB reports: the file cells of this run count missed bytes too", c.name)
		return
	}
	keys, err := suite.cellKeys(c)
	require.NoError(suite.T(), err)
	pods, err := suite.agentPods()
	require.NoError(suite.T(), err)
	for _, pod := range pods {
		flare, err := suite.agentFlare(pod, keys...)
		require.NoError(suite.T(), err)
		total, ok := flare.logsAgentBytesMissed()
		require.True(suite.T(), ok, "the flare of %s has no logs-agent BytesMissed", pod.Name)
		after, err := suite.missedBytesReports()
		require.NoError(suite.T(), err)
		assert.NoError(suite.T(), checkMissedBytesTotal(byPod[pod.Name], after[pod.Name], total), "%s on %s", c.name, pod.Name)
	}
}

// Agent restart.

// restartRule is what an Agent restart may resend.
type restartRule struct {
	// bound is how many records of a stream may be collected twice.
	bound int
	// graceful says the stopped Agent logged that its logs agent stopped
	// within its grace period.
	graceful bool
}

// restartDuplicateBound is how many records an Agent restart can collect
// twice. The registry holds the offset of the last record the destination
// acknowledged, so a restart resends what was acknowledged after the
// registry was last written:
//   - a graceful stop writes the registry once the pipeline has flushed
//     (registryAuditor.Stop), so nothing is resent;
//   - otherwise the registry is at most one auditor flush period old, and
//     the payloads acknowledged in that time, or in flight when the Agent
//     was killed, were each filled within batch_wait. That is the records
//     written in a flush period, a batch wait and a second of send latency,
//     plus one write of the writer, whose records arrive together.
//
// Records are counted with the paced payload size, which is less than their
// line, so the bound is generous.
func restartDuplicateBound(w writerOptions, graceful bool) int {
	if graceful || !w.paced() {
		return 0
	}
	seconds := auditorFlushPeriodSeconds + logsBatchWaitSeconds + 1
	perSecond := float64(w.rateBytesPerSec) / pacedWriterPayloadBytes
	return int(math.Ceil(perSecond*float64(seconds))) + pacedWriterBufferBytes/pacedWriterPayloadBytes
}

// restartDuplicateProblems lists how the duplicates of a restart break the
// rule: more of them than the bound, a record collected more than twice, or
// duplicates of a stream that are not one run of sequences, while a restart
// resumes each file at one offset.
func restartDuplicateProblems(check recordCheck, counts map[recordKey]int, rule restartRule) []string {
	var problems []string
	if len(check.duplicated) > rule.bound {
		problems = append(problems, fmt.Sprintf("%d records were collected twice, more than the %d the Agent can resend after a %s restart: %s",
			len(check.duplicated), rule.bound, map[bool]string{true: "graceful", false: "forced"}[rule.graceful], formatRecordRanges(check.duplicated)))
	}
	var thrice []recordKey
	byStream := make(map[string][]int64)
	for _, key := range check.duplicated {
		if counts[key] > 2 {
			thrice = append(thrice, key)
		}
		byStream[key.runID] = append(byStream[key.runID], key.sequence)
	}
	if len(thrice) > 0 {
		problems = append(problems, fmt.Sprintf("%d records were collected more than twice, while one restart resends a record once: %s", len(thrice), formatRecordRanges(thrice)))
	}
	streams := make([]string, 0, len(byStream))
	for stream := range byStream {
		streams = append(streams, stream)
	}
	sort.Strings(streams)
	for _, stream := range streams {
		sequences := byStream[stream]
		sort.Slice(sequences, func(i, j int) bool { return sequences[i] < sequences[j] })
		if span := sequences[len(sequences)-1] - sequences[0] + 1; span != int64(len(sequences)) {
			keys := make([]recordKey, 0, len(sequences))
			for _, sequence := range sequences {
				keys = append(keys, recordKey{runID: stream, sequence: sequence})
			}
			problems = append(problems, fmt.Sprintf("the duplicates of %s are not one run of sequences, while a restart resumes each file at one offset: %s", stream, formatRecordRanges(keys)))
		}
	}
	return problems
}

// assertRestartRecords requires every record at least once, and at most what
// the restart can resend twice.
func assertRestartRecords(t assert.TestingT, c cell, check recordCheck, counts map[recordKey]int, rule restartRule) {
	assert.Zero(t, len(check.missing),
		"%s: %d of %d expected records were never collected across the Agent restart: %s",
		c.name, len(check.missing), check.expected, formatRecordRanges(check.missing))
	for _, problem := range restartDuplicateProblems(check, counts, rule) {
		assert.Fail(t, "duplicates beyond what a restart resends", "%s: %s", c.name, problem)
	}
}

// Load.

// The SMB client's read sizes, from pkg/logs/internal/smb/client: each chunk
// of a read is a compound CREATE, READ and CLOSE of at most readChunkSize-1
// new bytes, and a read at the end of the file costs a second open. A tailer
// whose listing shows nothing new reads anyway every DefaultForceReadEvery
// polls (pkg/logs/tailers/smb/tailer.go).
const (
	smbReadChunkBytes = 64*1024 - 1
	smbEOFReadOpens   = 2
	smbForceReadEvery = 10
	// smbDrainOpens is a drain's opens per rotation: the read of what is
	// left, then the first and the last read at the end of the file. The
	// polls between them only list the share, and a drain that lasts
	// close_timeout (5s) is shorter than smbForceReadEvery polls.
	smbDrainOpens = 1 + drainEdgeReadOpens
	// drainEdgeReadOpens are the opens of the reads at the end of the file
	// that start and end a drain.
	drainEdgeReadOpens = 2 * smbEOFReadOpens
)

// smbOpensEstimate is how many SMB opens per second a cell's source makes,
// from the scanner's poll logic; Azure bills each open as a transaction.
type smbOpensEstimate struct {
	PollIntervalSeconds int     `json:"poll_interval_seconds"`
	ListingsPerScan     int     `json:"listings_per_scan"`
	ReadOpensPerScan    float64 `json:"read_opens_per_scan"`
	DrainOpensPerSecond float64 `json:"drain_opens_per_second"`
	OpensPerSecond      float64 `json:"opens_per_second"`
	Basis               string  `json:"basis"`
}

func estimateSMBOpens(w writerOptions) smbOpensEstimate {
	streams := max(1, w.streams)
	listings := 1
	if w.streams > 0 {
		// The share root, then each stream directory the pattern matched.
		listings += w.streams
	}
	interval := float64(smbPollIntervalSeconds)
	var readOpens float64
	if w.paced() {
		perPoll := float64(w.rateBytesPerSec) / float64(streams) * interval
		readOpens = float64(streams) * math.Ceil(perPoll/smbReadChunkBytes)
	} else {
		// Nothing new between fills: only the forced reads at the end.
		readOpens = float64(streams) * smbEOFReadOpens / smbForceReadEvery
	}
	drains := float64(streams*smbDrainOpens) / (float64(w.periodMs) / 1000)
	return smbOpensEstimate{
		PollIntervalSeconds: smbPollIntervalSeconds,
		ListingsPerScan:     listings,
		ReadOpensPerScan:    readOpens,
		DrainOpensPerSecond: drains,
		OpensPerSecond:      (float64(listings)+readOpens)/interval + drains,
		Basis: fmt.Sprintf("estimated from the scanner's poll logic, not measured: per scan, one open per listed directory and, per active file, one per %d new bytes "+
			"(%d at the end of a file, every %d polls when nothing is new); per rotation, %d opens of its drain. Azure Monitor's Transactions metric of the storage account measures them",
			smbReadChunkBytes, smbEOFReadOpens, smbForceReadEvery, smbDrainOpens),
	}
}

// latencySummary summarizes a set of latencies in milliseconds.
type latencySummary struct {
	Count int     `json:"count"`
	P50   float64 `json:"p50_ms"`
	P95   float64 `json:"p95_ms"`
	P99   float64 `json:"p99_ms"`
	Max   float64 `json:"max_ms"`
}

func summarizeLatencies(values []float64) latencySummary {
	if len(values) == 0 {
		return latencySummary{}
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	quantile := func(q float64) float64 {
		return sorted[min(len(sorted)-1, int(math.Ceil(q*float64(len(sorted))))-1)]
	}
	return latencySummary{Count: len(sorted), P50: quantile(0.50), P95: quantile(0.95), P99: quantile(0.99), Max: sorted[len(sorted)-1]}
}

// loadReport is what a paced cell's run shows of the pipeline under load.
type loadReport struct {
	Cell            string `json:"cell"`
	Streams         int    `json:"streams"`
	RateBytesPerSec int    `json:"rate_bytes_per_sec"`
	PeriodMs        int    `json:"period_ms"`
	// What the writer wrote in the asserted files.
	Files        int   `json:"files"`
	Records      int   `json:"records"`
	WrittenBytes int64 `json:"written_bytes"`
	// WriterBytesPerSec leaves out each stream's first file, which can start
	// mid-period.
	WriterBytesPerSec float64 `json:"writer_bytes_per_sec"`
	// What Fakeintake received of them.
	CollectedRecords     int     `json:"collected_records"`
	CollectedBytes       int64   `json:"collected_bytes"`
	ArrivalWindowSeconds float64 `json:"arrival_window_seconds"`
	CollectedBytesPerSec float64 `json:"collected_bytes_per_sec"`
	// PipelineLag is from the record's write, as the writer timestamped it,
	// to its arrival at Fakeintake; AgentReadLag to the Agent's timestamp,
	// set when it read the line; DeliveryLag from there to the arrival.
	PipelineLag  latencySummary `json:"pipeline_lag"`
	AgentReadLag latencySummary `json:"agent_read_lag"`
	DeliveryLag  latencySummary `json:"delivery_lag"`
	// FileCompletionLag is from a file's rotation, as the writer journalled
	// it, to the arrival of its last record.
	FileCompletionLag latencySummary   `json:"file_completion_lag"`
	SMBOpens          smbOpensEstimate `json:"smb_opens"`
}

// recordWriteTime reads the writer's timestamp at the start of a record line.
func recordWriteTime(message string) (time.Time, bool) {
	const layout = "2006-01-02 15:04:05.000"
	if len(message) < len(layout) {
		return time.Time{}, false
	}
	at, err := time.Parse(layout, message[:len(layout)])
	return at.UTC(), err == nil
}

func milliseconds(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}

// buildLoadReport measures the throughput and the lags of a cell's asserted
// records from the ledger and from what Fakeintake received.
func buildLoadReport(c cell, asserted []ledgerEntry, expected map[recordKey]struct{}, logs []collectedLog) loadReport {
	report := loadReport{
		Cell: c.name, Streams: c.writer.streams, RateBytesPerSec: c.writer.rateBytesPerSec, PeriodMs: c.writer.periodMs,
		Files: len(asserted), Records: len(expected),
		SMBOpens: estimateSMBOpens(c.writer),
	}
	var steadyBytes int64
	steadyFiles := 0
	for _, entries := range ledgerByStream(asserted) {
		sorted := append([]ledgerEntry(nil), entries...)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].Period < sorted[j].Period })
		for i, entry := range sorted {
			report.WrittenBytes += entry.Bytes
			if i > 0 {
				steadyBytes += entry.Bytes
				steadyFiles++
			}
		}
	}
	streams := max(1, c.writer.streams)
	if steadyFiles > 0 {
		periods := float64(steadyFiles) / float64(streams)
		report.WriterBytesPerSec = float64(steadyBytes) / (periods * float64(c.writer.periodMs) / 1000)
	}

	lastArrival := make(map[recordKey]time.Time, len(expected))
	var first, last time.Time
	var pipeline, read, delivery []float64
	for _, log := range logs {
		key, ok := parseRecord(log.message)
		if !ok {
			continue
		}
		if _, wanted := expected[key]; !wanted {
			continue
		}
		report.CollectedRecords++
		report.CollectedBytes += int64(len(log.message)) + 1
		if log.arrived.After(lastArrival[key]) {
			lastArrival[key] = log.arrived
		}
		if !log.arrived.IsZero() {
			if first.IsZero() || log.arrived.Before(first) {
				first = log.arrived
			}
			if log.arrived.After(last) {
				last = log.arrived
			}
		}
		written, ok := recordWriteTime(log.message)
		if !ok {
			continue
		}
		agentTime := time.UnixMilli(log.timestamp).UTC()
		if !log.arrived.IsZero() {
			pipeline = append(pipeline, milliseconds(log.arrived.Sub(written)))
		}
		if log.timestamp > 0 {
			read = append(read, milliseconds(agentTime.Sub(written)))
			if !log.arrived.IsZero() {
				delivery = append(delivery, milliseconds(log.arrived.Sub(agentTime)))
			}
		}
	}
	if window := last.Sub(first).Seconds(); window > 0 {
		report.ArrivalWindowSeconds = window
		report.CollectedBytesPerSec = float64(report.CollectedBytes) / window
	}
	report.PipelineLag = summarizeLatencies(pipeline)
	report.AgentReadLag = summarizeLatencies(read)
	report.DeliveryLag = summarizeLatencies(delivery)

	var completion []float64
	for _, entry := range asserted {
		rotated, ok := entry.rotatedAt()
		if !ok {
			continue
		}
		var fileLast time.Time
		for sequence := entry.FirstSequence; sequence <= entry.LastSequence; sequence++ {
			if at := lastArrival[recordKey{runID: entry.RunID, sequence: sequence}]; at.After(fileLast) {
				fileLast = at
			}
		}
		if !fileLast.IsZero() {
			completion = append(completion, milliseconds(fileLast.Sub(rotated)))
		}
	}
	report.FileCompletionLag = summarizeLatencies(completion)
	return report
}

// recordLoad keeps a paced cell's load report in the evidence and logs it.
func (suite *azureFilesSuite) recordLoad(c cell, asserted []ledgerEntry, expected map[recordKey]struct{}, logs []collectedLog) {
	report := buildLoadReport(c, asserted, expected, logs)
	suite.T().Logf("%s load: %d streams, writer %.0f B/s, Fakeintake %.0f B/s over %.0fs; pipeline lag p50 %.0fms p99 %.0fms max %.0fms; file completion after rotation p50 %.0fms max %.0fms; about %.1f SMB opens/s (estimated)",
		c.name, max(1, report.Streams), report.WriterBytesPerSec, report.CollectedBytesPerSec, report.ArrivalWindowSeconds,
		report.PipelineLag.P50, report.PipelineLag.P99, report.PipelineLag.Max,
		report.FileCompletionLag.P50, report.FileCompletionLag.Max, report.SMBOpens.OpensPerSecond)
	if evidence, err := suite.evidenceDir(); err == nil {
		evidence.writeJSON(c.name+"-load.json", report)
	}
}

// Silent-loss bound: unit tests. They run without Azure, next to the
// functions they check (see README.md, "Checks that do not create cloud
// resources").

func TestSilentLossWindowIsAPollIntervalAndItsMargin(t *testing.T) {
	// One poll interval, the scan's second, and two writer ticks.
	assert.Equal(t, smbPollIntervalSeconds*1000+lossScanMarginSeconds*1000+2*writerTickMs, silentLossWindowMs)
	assert.Equal(t, 2500, silentLossWindowMs)
	assert.Equal(t, 2500*time.Millisecond, silentLossWindow())
	// The 4 KiB tolerance counts as reported, so a stream's window is as much
	// longer as its bytes take to write: little at a high rate, 2s at 2 kB/s.
	for rate, extra := range map[int]time.Duration{
		200000: 20480 * time.Microsecond,
		20000:  204800 * time.Microsecond,
		2000:   2048 * time.Millisecond,
	} {
		w := writerOptions{mode: deleteRecreateRotation, periodMs: 10000, rateBytesPerSec: rate}
		assert.Equal(t, silentLossWindow()+extra, silentWindow(w), "%d B/s", rate)
	}
	assert.Equal(t, silentLossWindow(), silentWindow(defaultWriterOptions()), "the Java schedule has no rate")
	// The window is wider than what a drain's loss covers (lossWindowBytes)
	// by the writer's ticks only, never by a whole poll.
	paced := writerOptions{mode: deleteRecreateRotation, periodMs: 10000, rateBytesPerSec: 200000}
	assert.Greater(t, maxSilentBytes(paced), lossWindowBytes(paced))
	assert.Less(t, maxSilentBytes(paced), lossWindowBytes(paced)+streamRateBytesPerSec(paced)*smbPollIntervalSeconds)
}

func TestSilentLossBoundFollowsTheWriterRate(t *testing.T) {
	assert.Zero(t, maxSilentBytes(defaultWriterOptions()))
	assert.Zero(t, maxSilentRecords(defaultWriterOptions()))

	// The pod's rate is shared by its streams.
	one := writerOptions{mode: gzipRotation, periodMs: 10000, rateBytesPerSec: 200000}
	two := one
	two.streams = 2
	assert.Equal(t, int64(200000), streamRateBytesPerSec(one))
	assert.Equal(t, int64(100000), streamRateBytesPerSec(two))
	assert.Equal(t, int64(500000), maxSilentBytes(one))
	assert.Equal(t, int64(250000), maxSilentBytes(two))
	assert.Equal(t, 489, maxSilentRecords(one))
	assert.Equal(t, 245, maxSilentRecords(two))

	// The bound grows with the window, not with the period.
	slow := one
	slow.periodMs = 600000
	assert.Equal(t, maxSilentBytes(one), maxSilentBytes(slow))
}

func TestSilentLossFollowsTheDisposalOfEachMode(t *testing.T) {
	for _, tc := range []struct {
		mode   rotationMode
		forced bool
		delay  int
	}{
		{deleteRecreateRotation, false, deleteRecreatePauseMs},
		{deleteRecreateRotation, true, 0},
		{gzipRotation, false, gzipDelayMs},
		{gzipRotation, true, 0},
		{renameRotation, false, 0},
		{copyTruncateRotation, false, 0},
	} {
		w := writerOptions{mode: tc.mode, periodMs: 10000, rateBytesPerSec: 20000, forceLoss: tc.forced}
		assert.Equal(t, tc.delay, silentDisposalDelayMs(w), "%s forced=%t", tc.mode, tc.forced)
	}
}

func TestAssessSilentLoss(t *testing.T) {
	forced := writerOptions{mode: deleteRecreateRotation, periodMs: 10000, rateBytesPerSec: 20000, forceLoss: true}
	const line = 1000

	// Nothing lost, or a loss the Agent reported in full or to within the
	// tolerance, leaves nothing silent.
	assert.Equal(t, silentLoss{}, assessSilentLoss(forced, 0, 0, 0))
	assert.Equal(t, silentLoss{}, assessSilentLoss(forced, 30, 30*line, 30*line))
	assert.Equal(t, silentLoss{}, assessSilentLoss(forced, 30, 30*line, 30*line-lossAccountingToleranceBytes))
	assert.Equal(t, silentLoss{}, assessSilentLoss(forced, 4, 4*line, 0), "four records are under the tolerance")

	// Nothing reported: all of it, less the tolerance, is silent. The oldest
	// record the report leaves uncovered is dated without the tolerance, which
	// silentWindow adds to the window instead: it has the other 29 after it,
	// at 20000 B/s.
	got := assessSilentLoss(forced, 30, 30*line, 0)
	assert.Equal(t, int64(30*line-lossAccountingToleranceBytes), got.Bytes)
	assert.Equal(t, 26, got.Records)
	assert.Equal(t, 30, got.Uncovered)
	assert.Equal(t, time.Duration(float64(29*line)/20000*float64(time.Second)), got.OldestAge)

	// A report covers the first of the lost records, so only the end is silent.
	got = assessSilentLoss(forced, 60, 60*line, 40*line)
	assert.Equal(t, int64(60*line-40*line-lossAccountingToleranceBytes), got.Bytes)
	assert.Equal(t, 16, got.Records)
	assert.Equal(t, 20, got.Uncovered)
	assert.Equal(t, time.Duration(float64(19*line)/20000*float64(time.Second)), got.OldestAge)

	// Silent records never outnumber the lost ones.
	assert.Equal(t, 5, assessSilentLoss(forced, 5, 5*line+10*lossAccountingToleranceBytes, 0).Records)

	// The writer's pause before it deletes or compresses the file ages every
	// silent record: the oldest of a gzip file is 5s older than the bytes
	// after it say.
	gzip := writerOptions{mode: gzipRotation, periodMs: 10000, rateBytesPerSec: 20000}
	paused := assessSilentLoss(gzip, 30, 30*line, 0)
	unpaused := assessSilentLoss(writerOptions{mode: gzipRotation, periodMs: 10000, rateBytesPerSec: 20000, forceLoss: true}, 30, 30*line, 0)
	assert.Equal(t, gzipDelayMs*time.Millisecond, paused.OldestAge-unpaused.OldestAge)

	// A writer that is not paced writes nothing near the rotation: it filled
	// the file right after its 5s head pause, 55s before the end of a 60s
	// period, and then the pause before the delete.
	java := assessSilentLoss(writerOptions{mode: deleteRecreateRotation, periodMs: 60000}, 30, 30*line, 0)
	assert.Equal(t, (55*time.Second)+deleteRecreatePauseMs*time.Millisecond, java.OldestAge)
}

// silentFixture is one file of a hundred 1000-byte records, which a writer of
// 20000 B/s wrote in five seconds and rotated at 12:00:00.
func silentFixture(w writerOptions) (cell, []ledgerEntry, map[recordKey]struct{}) {
	c := cell{name: "smb-" + string(w.mode), reader: smbReader, accountName: "acct", shareName: "share", writer: w}
	ledger := []ledgerEntry{
		{RunID: "r", Period: "p1", File: "app.log.1", FirstSequence: 1, LastSequence: 100, Bytes: 100000, RotatedAt: "2026-10-06T12:00:00.000Z"},
		{RunID: "r", Period: "p2", File: "app.log.2", FirstSequence: 101, LastSequence: 102, Bytes: 2000, RotatedAt: "2026-10-06T12:01:00.000Z"},
	}
	if w.mode == deleteRecreateRotation {
		ledger[0].File, ledger[1].File = activeLogName, activeLogName
	}
	return c, ledger, expectedRecords(ledger[:1])
}

// lostOf collects the first collected records of silentFixture's file, so that
// the file loses its last 100-collected records, each 1000 bytes.
func lostOf(collected int) (map[recordKey]int, map[recordKey]int64) {
	counts, sizes := map[recordKey]int{}, map[recordKey]int64{}
	for sequence := int64(1); sequence <= int64(collected); sequence++ {
		counts[recordKey{"r", sequence}]++
		sizes[recordKey{"r", sequence}] = 1000
	}
	return counts, sizes
}

func TestSilentLossMustBeInTheWindow(t *testing.T) {
	// A delete-recreate rotation that deletes the file at once (forced), with
	// a stream of 20000 B/s: the window holds 50000 bytes, 2.5s.
	w := writerOptions{mode: deleteRecreateRotation, periodMs: 10000, rateBytesPerSec: 20000, forceLoss: true}
	c, ledger, expected := silentFixture(w)
	report := func(bytes int64) []missedBytesReport {
		return []missedBytesReport{{At: time.Date(2026, 10, 6, 12, 0, 1, 0, time.UTC), Reason: "Rotated SMB file is no longer listed",
			Bytes: bytes, Identifier: smbIdentifier(c, ""), ReadPath: activeLogName}}
	}
	judge := func(c cell, ledger []ledgerEntry, expected map[recordKey]struct{}, collected int, reports []missedBytesReport) ([]fileLoss, *recordingT) {
		counts, sizes := lostOf(collected)
		losses := explainLosses(c, ledger, ledger[:1], checkRecords(expected, nil, counts), sizes, reports)
		failed := new(recordingT)
		assertLossesExplained(failed, c, losses)
		return losses, failed
	}

	// The last 40 records, 2s of the writer, with no report at all: written
	// after the source's last listing, as the design allows.
	losses, failed := judge(c, ledger, expected, 60, nil)
	require.Len(t, losses, 1)
	assert.Equal(t, 40, losses[0].MissingRecords)
	assert.Equal(t, int64(40000), losses[0].UnreadBytes)
	assert.Equal(t, 36, losses[0].SilentRecords)
	assert.Equal(t, 49, losses[0].AllowedSilentRecords)
	assert.Equal(t, time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC), losses[0].DisposedAt)
	// 2.5s, and the 4 KiB tolerance at 20000 B/s: 0.2048s.
	assert.Equal(t, time.Date(2026, 10, 6, 11, 59, 57, 295200000, time.UTC), losses[0].SilentWindowStart)
	assert.Equal(t, "rate", losses[0].OldestSilentDatedBy)
	assert.False(t, losses[0].OldestSilentAt.Before(losses[0].SilentWindowStart))
	assert.Empty(t, failed.failures)

	// The last 50, 2.5s: the oldest one the report leaves uncovered is still
	// inside the window, once the record that dates it is not counted among
	// the bytes after it.
	_, failed = judge(c, ledger, expected, 50, nil)
	assert.Empty(t, failed.failures)

	// The last 70, 3.5s, with no report: more than the bound, and written
	// earlier than the window. The source listed them, so it had to say so.
	losses, failed = judge(c, ledger, expected, 30, nil)
	require.Len(t, losses, 1)
	assert.Equal(t, 66, losses[0].SilentRecords)
	require.Len(t, failed.failures, 2)
	assert.Contains(t, failed.failures[0], "silent loss beyond the bound")
	assert.Contains(t, failed.failures[0], "more than the 49")
	assert.Contains(t, failed.failures[1], "silent loss outside the window")
	assert.Contains(t, failed.failures[1], "the oldest written about 3.45s before the file went away")

	// The same loss, reported up to the last 2s that no listing showed: the
	// reported records are old, but reported, and the rest is in the window.
	losses, failed = judge(c, ledger, expected, 30, report(30000))
	assert.Empty(t, failed.failures)
	assert.Equal(t, int64(30000), losses[0].ReportedBytes)
	assert.Equal(t, 36, losses[0].SilentRecords)
	// Reported short of that, the unreported rest reaches back out of the
	// window and past the bound.
	losses, failed = judge(c, ledger, expected, 30, report(10000))
	assert.Equal(t, 56, losses[0].SilentRecords)
	require.Len(t, failed.failures, 2)
	assert.Contains(t, failed.failures[0], "silent loss beyond the bound")
	assert.Contains(t, failed.failures[1], "silent loss outside the window")

	// A writer that pauses 1s before it deletes the file leaves the source
	// that long to list the records it wrote: the same 40 lost records, with
	// no report, were written 2.95s before the file went away.
	paused := w
	paused.forceLoss = false
	pc, pledger, pexpected := silentFixture(paused)
	losses, failed = judge(pc, pledger, pexpected, 60, nil)
	require.Len(t, failed.failures, 1)
	assert.Contains(t, failed.failures[0], "silent loss outside the window")
	assert.Equal(t, time.Date(2026, 10, 6, 12, 0, 1, 0, time.UTC), losses[0].DisposedAt)
	// Fewer of them are inside the window again.
	_, failed = judge(pc, pledger, pexpected, 70, nil)
	assert.Empty(t, failed.failures)

	// A ledger with no rotation time cannot date a silent loss.
	undated := slices.Clone(ledger)
	undated[0].RotatedAt = ""
	losses, failed = judge(c, undated, expected, 60, nil)
	assert.True(t, losses[0].DisposedAt.IsZero())
	require.Len(t, failed.failures, 1)
	assert.Contains(t, failed.failures[0], "silent loss that cannot be dated")

	// The Java schedule writes each file right after its head pause: nothing
	// is near the rotation, so a loss without a report fails three times.
	java := writerOptions{mode: deleteRecreateRotation, periodMs: 60000}
	jc, jledger, jexpected := silentFixture(java)
	_, failed = judge(jc, jledger, jexpected, 20, nil)
	require.Len(t, failed.failures, 3)
	assert.Contains(t, failed.failures[0], "more than the 0 a delete-recreate rotation can take")
	assert.Contains(t, failed.failures[1], "silent loss beyond the bound")
	assert.Contains(t, failed.failures[2], "silent loss outside the window")
}

// journalledWrites dates the records of silentFixture's file from the journal:
// the pairs [first sequence, epoch ms of the write].
func journalledWrites(pairs ...[2]int64) []ledgerEntry {
	_, ledger, _ := silentFixture(writerOptions{mode: deleteRecreateRotation, periodMs: 10000, rateBytesPerSec: 20000, forceLoss: true})
	ledger[0].WriteTimes = pairs
	return ledger
}

func TestLedgerEntryDatesRecordsByTheJournalsWriteTimes(t *testing.T) {
	entry := ledgerEntry{FirstSequence: 1, LastSequence: 100, WriteTimes: [][2]int64{{1, 1000}, {41, 2000}, {91, 3500}}}
	at := func(sequence int64) (int64, bool) {
		written, ok := entry.writtenAt(sequence)
		return written.UnixMilli(), ok
	}
	// A record was written with the write that began at or before it.
	for sequence, want := range map[int64]int64{1: 1000, 40: 1000, 41: 2000, 90: 2000, 91: 3500, 100: 3500} {
		got, ok := at(sequence)
		assert.True(t, ok, "sequence %d", sequence)
		assert.Equal(t, want, got, "sequence %d", sequence)
	}
	// Before the first pair, which the journal keeps only the last of, or
	// without any: nothing says when.
	trimmed := ledgerEntry{FirstSequence: 1, LastSequence: 100, WriteTimes: [][2]int64{{41, 2000}}}
	_, ok := trimmed.writtenAt(40)
	assert.False(t, ok)
	_, ok = (ledgerEntry{FirstSequence: 1, LastSequence: 100}).writtenAt(5)
	assert.False(t, ok)

	// The records of the writes at or after a time.
	written, ok := entry.recordsWrittenSince(time.UnixMilli(2000))
	assert.True(t, ok)
	assert.Equal(t, 60, written)
	written, _ = entry.recordsWrittenSince(time.UnixMilli(2001))
	assert.Equal(t, 10, written)
	written, _ = entry.recordsWrittenSince(time.UnixMilli(3501))
	assert.Zero(t, written)
	_, ok = (ledgerEntry{LastSequence: 100}).recordsWrittenSince(time.UnixMilli(0))
	assert.False(t, ok)
}

// A writer that stalled on the share catches up in one write, so the bytes
// written in the last second of real time can be several seconds of the
// stream's rate. Dated by the rate, a correct source that lost the burst
// looked as if it had lost records written seconds before the file went away.
func TestASilentLossAfterAWriterStallIsDatedByItsWrite(t *testing.T) {
	// 20000 B/s, 1000-byte records, the file rotated and deleted at 12:00:00:
	// records 1-40 were written at 11:59:50, and the writer stalled for
	// more than 3s, then caught up with records 41-100, three seconds of the
	// rate, in one write that returned at 11:59:59.6. The source's last read
	// came just before that write, and lost all 60 records.
	rotatedAt := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	w := writerOptions{mode: deleteRecreateRotation, periodMs: 10000, rateBytesPerSec: 20000, forceLoss: true}
	c, _, expected := silentFixture(w)
	counts, sizes := lostOf(40)
	check := checkRecords(expected, nil, counts)
	judge := func(ledger []ledgerEntry) ([]fileLoss, *recordingT) {
		losses := explainLosses(c, ledger, ledger[:1], check, sizes, nil)
		failed := new(recordingT)
		assertLossesExplained(failed, c, losses)
		return losses, failed
	}

	// By the rate, the oldest record is 2.95s before the end: outside the
	// 2.7s window, and the 60 records are more than the 49 of its bound.
	_, ledger, _ := silentFixture(w)
	losses, failed := judge(ledger)
	require.Len(t, losses, 1)
	assert.Equal(t, "rate", losses[0].OldestSilentDatedBy)
	require.NotEmpty(t, failed.failures)

	// By the journal, the oldest was written 0.4s before the end, and no more
	// records were lost than the writer wrote in the window.
	ledger = journalledWrites([2]int64{1, rotatedAt.Add(-10 * time.Second).UnixMilli()}, [2]int64{41, rotatedAt.Add(-400 * time.Millisecond).UnixMilli()})
	losses, failed = judge(ledger)
	require.Len(t, losses, 1)
	assert.Equal(t, "journal", losses[0].OldestSilentDatedBy)
	assert.Equal(t, rotatedAt.Add(-400*time.Millisecond), losses[0].OldestSilentAt)
	assert.Equal(t, 60, losses[0].MissingRecords)
	assert.Equal(t, 60, losses[0].AllowedSilentRecords)
	assert.Equal(t, 104, losses[0].AllowedRecords, "the rate-based bound is the floor")
	assert.Empty(t, failed.failures)

	// A burst written before the window is still a silent loss outside it.
	old := journalledWrites([2]int64{1, rotatedAt.Add(-10 * time.Second).UnixMilli()}, [2]int64{41, rotatedAt.Add(-3 * time.Second).UnixMilli()})
	losses, failed = judge(old)
	require.Len(t, losses, 1)
	require.NotEmpty(t, failed.failures)
	assert.Contains(t, failed.failures[len(failed.failures)-1], "silent loss outside the window")
	assert.Contains(t, failed.failures[len(failed.failures)-1], "dated from the writer's journalled write times")

	// The journal dates the record, so the window is the plain 2.5s: the
	// tolerance's 4 KiB, 0.2s at 20000 B/s, only widen the window of a record
	// dated by the rate (the other tests).
	edge := journalledWrites([2]int64{1, rotatedAt.Add(-10 * time.Second).UnixMilli()}, [2]int64{41, rotatedAt.Add(-2500 * time.Millisecond).UnixMilli()})
	_, failed = judge(edge)
	assert.Empty(t, failed.failures)
	edge = journalledWrites([2]int64{1, rotatedAt.Add(-10 * time.Second).UnixMilli()}, [2]int64{41, rotatedAt.Add(-2510 * time.Millisecond).UnixMilli()})
	losses, failed = judge(edge)
	require.NotEmpty(t, failed.failures)
	assert.Contains(t, failed.failures[len(failed.failures)-1], "silent loss outside the window")
	assert.Equal(t, silentLossWindow(), losses[0].DisposedAt.Sub(losses[0].SilentWindowStart))
}

func TestDrainDeadlineReasons(t *testing.T) {
	assert.True(t, isDrainDeadlineReason("SMB rotation drain reached its longest duration, 50s (10 times logs_config.close_timeout)"))
	assert.True(t, isDrainDeadlineReason("SMB rotation drain timed out after 5s (logs_config.close_timeout)"))
	// A drain that ended because its file stopped growing, or because the
	// file was gone, did not run out of time.
	assert.False(t, isDrainDeadlineReason("SMB rotation drain ended after 5s without new data (logs_config.close_timeout)"))
	assert.False(t, isDrainDeadlineReason("Rotated SMB file is no longer listed"))
}
