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
	"sort"
	"strconv"
	"strings"
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
// logs-agent BytesMissed expvar, to the logs.bytes_missed telemetry counter
// and to the per-(source, service) missed bytes tracker, and logs a warning
// that names the file. Of these, only the warning can be read per cell:
//   - the expvar and the telemetry counter are Agent-wide;
//   - the tracker is only published by the health platform's
//     log_data_lost_after_rotation issue, which runs every 15 minutes and
//     only when the health platform is enabled;
//   - agent status lists no missed bytes, per source or at all.
//
// Each SMB cell has its own storage account, so the warning's smb://<host>/...
// identifier names the cell, and its read path names the rotated file. The
// test attributes the warnings to the ledger's files and checks their sum
// against the Agent-wide expvar, so a warning it could not read (a container
// log the kubelet rotated, a changed message) fails the cell instead of
// letting a loss pass as reported.

// lossAccountingToleranceBytes is how far the bytes the Agent reported missed
// for a file may differ from the bytes of the file it never delivered: the
// writer's padding, one record whose start was read before the drain ended,
// and an appender marker the Agent saw listed but did not read.
const lossAccountingToleranceBytes = 4096

// missedBytesReportPattern reads the warning of RecordMissedBytes, with the
// kubelet's timestamp when the log was read with timestamps:
//
//	<reason>: <N> bytes of SMB file <identifier> (last read as <path>) were not read and are lost
var missedBytesReportPattern = regexp.MustCompile(
	`^(?:(\d{4}-\d{2}-\d{2}T\S+Z) )?(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}) UTC \| [A-Z-]+ \| WARN \| (?:\([^)]*\) \| )?` +
		`(.+?): (\d+) bytes of SMB file (smb://\S+) \(last read as ([^)]*)\) were not read and are lost\s*$`)

// missedBytesReport is one RecordMissedBytes warning of the Agent.
type missedBytesReport struct {
	At         time.Time `json:"at"`
	Reason     string    `json:"reason"`
	Bytes      int64     `json:"bytes"`
	Identifier string    `json:"identifier"`
	ReadPath   string    `json:"read_path"`
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
		reports = append(reports, missedBytesReport{
			At: at.UTC(), Reason: match[3], Bytes: bytes, Identifier: match[5], ReadPath: match[6],
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
	prefix := "smb://" + strings.ToLower(c.host()) + "/" + strings.ToLower(c.shareName) + "/"
	var kept []missedBytesReport
	for _, report := range reports {
		if strings.HasPrefix(report.Identifier, prefix) {
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
	// Suffix says the missing records are the file's last ones: a drain cut
	// short can lose nothing else.
	Suffix bool `json:"suffix"`
	// UnreadBytes are the file's bytes that reached no collected record:
	// what the writer wrote minus the collected records' lines.
	UnreadBytes   int64               `json:"unread_bytes"`
	ReportedBytes int64               `json:"reported_bytes"`
	Reports       []missedBytesReport `json:"reports"`
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
				collectedBytes += lineBytes[key]
			}
		}
		if len(lost) == 0 {
			continue
		}
		matched := reportsForEntry(c, entry, next[fileKey{runID: entry.RunID, period: entry.Period}], reports)
		losses = append(losses, fileLoss{
			RunID: entry.RunID, Period: entry.Period, File: entry.File,
			FirstSequence: entry.FirstSequence, LastSequence: entry.LastSequence,
			MissingRecords: len(lost), MissingRanges: formatRecordRanges(lost),
			Suffix:        expectedAfterFirstLoss == len(lost),
			UnreadBytes:   max(0, entry.Bytes-collectedBytes),
			ReportedBytes: sumReportedBytes(matched),
			Reports:       matched,
		})
	}
	return losses
}

// assertLossesExplained requires every lost record to be accounted for by the
// Agent: only the end of a file may be lost, and the Agent must have reported
// about as many missed bytes for that file as it never delivered.
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
		case loss.ReportedBytes == 0:
			assert.Fail(t, "silent loss",
				"%s: %s lost its last %d records (%s), about %d bytes, and the Agent reported no missed bytes for it",
				c.name, file, loss.MissingRecords, loss.MissingRanges, loss.UnreadBytes)
		case loss.ReportedBytes+lossAccountingToleranceBytes < loss.UnreadBytes:
			assert.Fail(t, "partly silent loss",
				"%s: %s lost its last %d records (%s); the Agent reported %d missed bytes for it, but about %d bytes of it were never collected",
				c.name, file, loss.MissingRecords, loss.MissingRanges, loss.ReportedBytes, loss.UnreadBytes)
		case loss.ReportedBytes > loss.UnreadBytes+lossAccountingToleranceBytes:
			assert.Fail(t, "missed bytes do not match the file",
				"%s: the Agent reported %d missed bytes for %s, more than the %d bytes of it that were never collected; the reports were attributed to the wrong file",
				c.name, loss.ReportedBytes, file, loss.UnreadBytes)
		}
	}
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
	assertLossesExplained(t, c, explainLosses(c, ledger, asserted, check, lineBytes, reports))
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
	suite.T().Logf("%s: %d files lost records, the Agent made %d missed-bytes reports for the cell, %d bytes in all",
		c.name, len(losses), len(cellReports), sumReportedBytes(cellReports))
	if evidence, err := suite.evidenceDir(); err == nil {
		evidence.writeJSON(c.name+"-losses.json", map[string]any{"losses": losses, "reports": cellReports})
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
	// left, then two idle polls at the end of the file.
	smbDrainOpens = 1 + drainIdlePollOpens
	// drainIdlePollOpens are the opens of the idle polls that end a drain.
	drainIdlePollOpens = smbDrainIdlePolls * smbEOFReadOpens
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
