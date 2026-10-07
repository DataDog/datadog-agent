// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package azurefiles

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
)

const (
	runOptIn       = "AZURE_FILES_E2E_RUN"
	runOptInValue  = "1"
	runWriterImage = "AZURE_FILES_E2E_WRITER_IMAGE"
	runProfile     = "AZURE_FILES_E2E_PROFILE"
	runCells       = "AZURE_FILES_E2E_CELLS"
	runStackName   = "AZURE_FILES_E2E_STACK"
	// The writer options apply to every cell of the run; see writerOptions.
	runRotationMode = "AZURE_FILES_E2E_ROTATION_MODE"
	runPeriodMs     = "AZURE_FILES_E2E_PERIOD_MS"
	runRate         = "AZURE_FILES_E2E_RATE_BYTES_PER_SEC"
	runStreams      = "AZURE_FILES_E2E_STREAMS"
	// runCalibrate records every marker's outcome instead of asserting it.
	runCalibrate      = "AZURE_FILES_E2E_CALIBRATE"
	runCalibrateValue = "1"
	// The smb cells need an Agent built with the native SMB log source,
	// which a stock build does not have, so they only run on request.
	smbOptIn       = "E2E_SMB_AZURE"
	smbOptInValue  = "1"
	completedFiles = 4
	assertedFiles  = 3

	// A "this marker must not arrive" assertion passes trivially on its first
	// evaluation, so the marker expectations are re-checked once this much time
	// has passed. The lost marker is already appended by then, so this only
	// has to cover a reader that is still draining picking it up and shipping
	// it to Fakeintake.
	markerSettleDelay = 30 * time.Second

	redactedAccountKey = "[redacted storage account key]"

	// A failure message lists at most this many record ranges.
	reportedRanges = 10
)

var (
	// recordPattern reads a writer record's run ID and sequence. Requiring the
	// record= field after the sequence keeps a line cut inside its sequence
	// number from counting as another sequence.
	recordPattern   = regexp.MustCompile(`\brun_id=(\S+) period=\S+ sequence=([0-9]+) record=`)
	markerIDPattern = regexp.MustCompile(`\bmarker_id=([^\s]+)`)
)

// ledgerEntry is one completed file of a writer stream. ledger.sh writes the
// fields up to ObservedAt in both of its modes; the others come from the
// Python writer's journal.
type ledgerEntry struct {
	RunID           string `json:"run_id"`
	Period          string `json:"period"`
	File            string `json:"file"`
	TargetBytes     int64  `json:"target_bytes"`
	Bytes           int64  `json:"bytes"`
	First2048SHA256 string `json:"first_2048_sha256"`
	First2048CRC64  string `json:"first_2048_crc64"`
	FirstLineSHA256 string `json:"first_line_sha256"`
	FirstLineCRC64  string `json:"first_line_crc64"`
	FirstSequence   int64  `json:"first_sequence"`
	LastSequence    int64  `json:"last_sequence"`
	LineCount       int64  `json:"line_count"`
	DiscoveredAt    string `json:"discovered_at"`
	ObservedAt      string `json:"observed_at"`

	Stream       string `json:"stream,omitempty"`
	RotationMode string `json:"rotation_mode,omitempty"`
	// Disposition is what the writer did with the file once it journalled
	// it: renamed, copied, compressed or deleted, or missing, empty or
	// copy-failed when the rotation did not happen.
	Disposition string `json:"disposition,omitempty"`
	Archive     string `json:"archive,omitempty"`
	// UnwrittenSequences are [first, last] ranges of sequences the writer
	// failed to write: they are in no file, so no reader can collect them.
	UnwrittenSequences [][2]int64 `json:"unwritten_sequences,omitempty"`
	// AtRiskSequences are [first, last] ranges of sequences that this
	// rotation may have destroyed before any reader could be expected to
	// read them: what copytruncate's writer appended between the start of
	// the copy and the truncation.
	AtRiskSequences [][2]int64 `json:"at_risk_sequences,omitempty"`
	RotatedAt       string     `json:"rotated_at,omitempty"`
	// Observed is what the share held for the file when the ledger recorded
	// it: file, archive, deleted or missing.
	Observed      string `json:"observed,omitempty"`
	ObservedBytes int64  `json:"observed_bytes,omitempty"`
}

// rotatedAt parses RotatedAt, the time the writer journalled the file.
func (e ledgerEntry) rotatedAt() (time.Time, bool) {
	at, err := time.Parse(time.RFC3339Nano, e.RotatedAt)
	return at, err == nil
}

// recordKey identifies one writer record: sequences start over in each
// stream.
type recordKey struct {
	runID    string
	sequence int64
}

// markerKey identifies one rotated file of one stream.
type markerKey struct {
	runID string
	file  string
}

// markerEntry is one post-rename append recorded by the appender sidecar.
type markerEntry struct {
	RunID       string `json:"run_id"`
	Rotation    int    `json:"rotation"`
	MarkerAgeMs int    `json:"marker_age_ms"`
	MarkerID    string `json:"marker_id"`
	RotatedFile string `json:"rotated_file"`
	AppendedAt  string `json:"appended_at"`
	Status      string `json:"status"`
}

type azureFilesSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
	spec     runSpec
	evidence *evidenceDir
}

func TestAzureFiles(t *testing.T) {
	t.Parallel()

	if os.Getenv(runOptIn) != runOptInValue {
		t.Skipf("set %s=%s to run the Azure Files E2E", runOptIn, runOptInValue)
	}
	// Unset, the writer pods run the stock Python image with the workload
	// ConfigMap; set, they run this Java writer image.
	writerImage := os.Getenv(runWriterImage)

	runID := fmt.Sprintf("%s-%06d", time.Now().UTC().Format("20060102t150405z"), time.Now().UTC().Nanosecond()/1000)
	spec, err := newRunSpec(runOptions{
		runID:           runID,
		writerImage:     writerImage,
		profile:         os.Getenv(runProfile),
		cells:           os.Getenv(runCells),
		stackName:       os.Getenv(runStackName),
		smbEnabled:      os.Getenv(smbOptIn) == smbOptInValue,
		rotationMode:    os.Getenv(runRotationMode),
		periodMs:        os.Getenv(runPeriodMs),
		rateBytesPerSec: os.Getenv(runRate),
		streams:         os.Getenv(runStreams),
		calibrate:       os.Getenv(runCalibrate) == runCalibrateValue,
	})
	require.NoError(t, err, "one of %s is invalid", strings.Join([]string{
		runProfile, runCells, runStackName, runWriterImage, runRotationMode, runPeriodMs, runRate, runStreams,
	}, ", "))
	if len(spec.cells) == 0 {
		names := make([]string, 0, len(spec.gatedCells))
		for _, c := range spec.gatedCells {
			names = append(names, c.name)
		}
		t.Skipf("every selected cell (%s) is gated: %s", strings.Join(names, ","), gateReason())
	}
	e2e.Run(t, &azureFilesSuite{spec: spec},
		e2e.WithProvisioner(spec.storageProvisioner()),
		e2e.WithStackName(spec.stackName),
		// Keep the AKS node, storage accounts, shares, and pods of a failed run
		// for investigation. E2E_DEV_MODE=true keeps them after a pass too.
		e2e.WithSkipDeleteOnFailure(),
	)
}

// gateReason says how to enable the gated smb cells.
func gateReason() string {
	return fmt.Sprintf("set %s=%s to run the smb cells; they need an Agent built with the native SMB log source (see README.md)", smbOptIn, smbOptInValue)
}

func (suite *azureFilesSuite) TestRotatedFilesAreCollectedExactlyOnce() {
	suite.skipGatedCells()
	suite.installAgent()
	defer suite.captureEvidence()
	suite.requireAgentReady()

	for _, c := range suite.spec.cells {
		suite.Run(c.name, func() {
			suite.checkCell(c)
		})
	}
}

// skipGatedCells reports the gated cells as skipped subtests.
func (suite *azureFilesSuite) skipGatedCells() {
	for _, c := range suite.spec.gatedCells {
		suite.Run(c.name, func() {
			suite.T().Skip(gateReason())
		})
	}
}

// installAgent runs the Agent pass. A reused stack keeps its Fakeintake and
// the logs of earlier runs, which carry the same services and sequence
// numbers. The storage pass has just removed the Agent, so nothing new
// arrives before the Agent pass.
func (suite *azureFilesSuite) installAgent() {
	suite.T().Helper()
	require.NoError(suite.T(), suite.Env().FakeIntake.Client().FlushServerAndResetAggregators())
	suite.UpdateEnv(suite.spec.agentProvisioner())
	require.NoError(suite.T(), suite.writeRunMetadata())
}

// checkCell waits for the cell's writer to complete its files and checks that
// the Agent collected them: every record exactly once, except what the cell's
// rotation mode allows, and each marker as calibrated. It returns the asserted
// ledger entries.
func (suite *azureFilesSuite) checkCell(c cell) []ledgerEntry {
	suite.T().Helper()
	if c.reader == smbReader {
		// Fail with a clear message when the Agent cannot run the source at
		// all, rather than with every sequence missing, and before waiting
		// minutes for the writer's rotations.
		suite.requireSMBSourceRunning(c)
	}

	pod := suite.writerPod(c)
	streams := suite.spec.streamRunIDs(c)
	ledger := suite.waitForLedger(c, pod, completedFiles)
	asserted, atRisk := assertedLedger(ledger, streams)
	expected := expectedRecords(asserted)
	// A ledger whose records all failed to be written leaves nothing to
	// check, which must not pass as a success.
	require.NotEmpty(suite.T(), expected, "%s: the writer wrote none of the records of its asserted files", c.name)
	if unwritten := unwrittenRecords(asserted); unwritten > 0 {
		suite.T().Logf("%s: the writer failed to write %d records of its asserted files; they are in no file and not expected (see the ledger and the writer log)", c.name, unwritten)
	}
	var markers []markerEntry
	if c.markers.count() > 0 {
		markers = suite.postRotationMarkers(pod, c, asserted)
	}

	judge := func(t assert.TestingT, messages []string) recordCheck {
		counts := countRecords(messages, expected)
		check := checkRecords(expected, atRisk, counts)
		switch {
		case c.lossAccounted():
			suite.assertLossesReported(t, c, ledger, asserted, check, recordLineBytes(messages, expected))
			assertNoDuplicates(t, c, check)
		default:
			assertRecordsCollected(t, c, check)
		}
		if !suite.spec.calibrate {
			assertMarkerOutcome(t, c, markers, countMarkerIDs(messages))
		}
		return check
	}
	suite.EventuallyWithT(func(collect *assert.CollectT) {
		logs, err := suite.collectedLogs(c.service)
		require.NoError(collect, err)
		// Lines carrying the right service but not this source's metadata
		// come from a reader that builds a wrong origin.
		assertLogOrigin(collect, suite.spec.runID, c, logs)
		judge(collect, logMessages(logs))
	}, collectTimeout(c.writer), 10*time.Second)

	// A marker expected to be lost is asserted absent, which cannot fail on a
	// first look, so confirm the outcome once every drain has certainly ended.
	if c.markers.needsSettle() {
		time.Sleep(markerSettleDelay)
	}
	logs, err := suite.collectedLogs(c.service)
	require.NoError(suite.T(), err)
	messages := logMessages(logs)
	check := judge(suite.T(), messages)
	suite.T().Logf("%s: %d records expected, %d of them at risk; %d at-risk records were lost, which the rotation mode allows: %s",
		c.name, check.expected, check.atRisk, len(check.atRiskLost), formatRecordRanges(check.atRiskLost))
	if suite.spec.calibrate && len(markers) > 0 {
		suite.recordMarkerOutcomes(c, markerOutcomes(c, markers, countMarkerIDs(messages)))
	}

	if c.lossAccounted() {
		suite.checkMissedBytesTotals(c, ledger, asserted, check, recordLineBytes(messages, expected))
	}
	if c.reader == smbReader {
		suite.checkSMBSource(c)
	}
	if c.writer.paced() {
		suite.recordLoad(c, asserted, expected, logs)
	}
	return asserted
}

// waitForLedger waits until every stream of the cell's writer has completed
// files files, and returns its ledger.
func (suite *azureFilesSuite) waitForLedger(c cell, pod corev1.Pod, files int) []ledgerEntry {
	suite.T().Helper()
	streams := suite.spec.streamRunIDs(c)
	var ledger []ledgerEntry
	suite.EventuallyWithT(func(collect *assert.CollectT) {
		var err error
		ledger, err = suite.readLedger(c, pod)
		require.NoError(collect, err)
		byStream := ledgerByStream(ledger)
		for _, stream := range streams {
			assert.GreaterOrEqual(collect, len(byStream[stream]), files,
				"writer %s has not completed %d rotations yet; its ledger has %d", stream, files, len(byStream[stream]))
		}
	}, ledgerTimeout(c.writer), 5*time.Second)
	return ledger
}

// postRotationMarkers waits until the appender has written both markers for
// every rotation this cell asserts on. A short journal means the appender never
// ran or never saw the rotations, which invalidates the marker assertions
// rather than saying anything about the Agent.
func (suite *azureFilesSuite) postRotationMarkers(
	pod corev1.Pod,
	c cell,
	asserted []ledgerEntry,
) []markerEntry {
	suite.T().Helper()
	files := make(map[markerKey]struct{}, len(asserted))
	for _, entry := range asserted {
		files[markerKey{runID: entry.RunID, file: entry.File}] = struct{}{}
	}
	wanted := len(asserted) * c.markers.count()

	var markers []markerEntry
	suite.EventuallyWithT(func(collect *assert.CollectT) {
		journal, err := suite.readMarkerJournal(c, pod)
		require.NoError(collect, err)
		markers = markersForFiles(journal, files)
		assert.Len(collect, markers, wanted,
			"the appender must record %d post-rename markers for the %d asserted rotations of %s; the journal has %d",
			wanted, len(asserted), c.name, len(markers))
	}, 3*time.Minute, 5*time.Second)
	return markers
}

// ledgerTimeout is how long a writer may take to complete its files: the
// rotations, and the Agent and writer start-up before them.
func ledgerTimeout(w writerOptions) time.Duration {
	return max(6*time.Minute, time.Duration(completedFiles+2)*time.Duration(w.periodMs)*time.Millisecond)
}

// collectTimeout is how long the Agent may take to ship what was written. A
// paced writer writes far more records, which also take Fakeintake longer to
// return.
func collectTimeout(w writerOptions) time.Duration {
	if w.paced() {
		return 5 * time.Minute
	}
	return 2 * time.Minute
}

func ledgerByStream(ledger []ledgerEntry) map[string][]ledgerEntry {
	byStream := make(map[string][]ledgerEntry)
	for _, entry := range ledger {
		byStream[entry.RunID] = append(byStream[entry.RunID], entry)
	}
	return byStream
}

// assertedLedger returns the first assertedFiles files of every stream, and
// the records any rotation of those streams put at risk. A rotation's at-risk
// records belong to the next period, which may be an asserted one.
func assertedLedger(ledger []ledgerEntry, streams []string) ([]ledgerEntry, map[recordKey]struct{}) {
	byStream := ledgerByStream(ledger)
	var asserted []ledgerEntry
	atRisk := make(map[recordKey]struct{})
	for _, stream := range streams {
		entries := slices.Clone(byStream[stream])
		sort.Slice(entries, func(i, j int) bool { return entries[i].Period < entries[j].Period })
		asserted = append(asserted, entries[:min(assertedFiles, len(entries))]...)
		for _, entry := range entries {
			for _, sequences := range entry.AtRiskSequences {
				for sequence := sequences[0]; sequence <= sequences[1]; sequence++ {
					atRisk[recordKey{runID: entry.RunID, sequence: sequence}] = struct{}{}
				}
			}
		}
	}
	return asserted, atRisk
}

// recordCheck sorts the expected records by how often they were collected.
type recordCheck struct {
	expected int
	atRisk   int
	// missing were never collected although no rotation put them at risk.
	missing []recordKey
	// duplicated were collected more than once.
	duplicated []recordKey
	// atRiskLost were never collected, which their rotation allows.
	atRiskLost []recordKey
}

func checkRecords(expected, atRisk map[recordKey]struct{}, counts map[recordKey]int) recordCheck {
	check := recordCheck{expected: len(expected)}
	for key := range expected {
		_, risky := atRisk[key]
		if risky {
			check.atRisk++
		}
		switch count := counts[key]; {
		case count > 1:
			check.duplicated = append(check.duplicated, key)
		case count == 0 && risky:
			check.atRiskLost = append(check.atRiskLost, key)
		case count == 0:
			check.missing = append(check.missing, key)
		}
	}
	for _, keys := range [][]recordKey{check.missing, check.duplicated, check.atRiskLost} {
		sortRecordKeys(keys)
	}
	return check
}

// assertRecordsCollected requires every expected record exactly once, except
// that a record its rotation put at risk may be missing. It reports ranges
// rather than one failure per record: a paced writer writes many thousands.
func assertRecordsCollected(t assert.TestingT, c cell, check recordCheck) {
	assert.Zero(t, len(check.missing),
		"%s: %d of %d expected records were never collected although no rotation put them at risk: %s",
		c.name, len(check.missing), check.expected, formatRecordRanges(check.missing))
	assert.Zero(t, len(check.duplicated),
		"%s: %d of %d expected records were collected more than once: %s",
		c.name, len(check.duplicated), check.expected, formatRecordRanges(check.duplicated))
}

func sortRecordKeys(keys []recordKey) {
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].runID != keys[j].runID {
			return keys[i].runID < keys[j].runID
		}
		return keys[i].sequence < keys[j].sequence
	})
}

// formatRecordRanges prints sorted keys as run ID ranges, the first
// reportedRanges of them.
func formatRecordRanges(keys []recordKey) string {
	if len(keys) == 0 {
		return "none"
	}
	var ranges []string
	for i := 0; i < len(keys); {
		j := i
		for j+1 < len(keys) && keys[j+1].runID == keys[i].runID && keys[j+1].sequence == keys[j].sequence+1 {
			j++
		}
		if i == j {
			ranges = append(ranges, fmt.Sprintf("%s:%d", keys[i].runID, keys[i].sequence))
		} else {
			ranges = append(ranges, fmt.Sprintf("%s:%d-%d", keys[i].runID, keys[i].sequence, keys[j].sequence))
		}
		i = j + 1
	}
	if len(ranges) > reportedRanges {
		return strings.Join(ranges[:reportedRanges], ", ") + fmt.Sprintf(" and %d more ranges", len(ranges)-reportedRanges)
	}
	return strings.Join(ranges, ", ")
}

// assertMarkerOutcome encodes the calibration described by the marker constants
// in provisioner.go: the early marker must survive, and the late one must not.
func assertMarkerOutcome(t assert.TestingT, c cell, markers []markerEntry, counts map[string]int) {
	for _, marker := range markers {
		expect, ok := c.markers.expectationFor(marker.MarkerAgeMs)
		if !ok {
			continue
		}
		count := counts[marker.MarkerID]
		switch {
		case expect == markerCollected:
			assert.Equal(t, postRotationMarkerSurvivingCount, count,
				"%s: marker %s was appended to %s %dms after its rename, inside the drain window of its %s reader, so it must be collected exactly once; losing it means appends to the rotated file are dropped",
				c.name, marker.MarkerID, marker.RotatedFile, marker.MarkerAgeMs, c.reader)
		case marker.MarkerAgeMs == c.markers.lateMs:
			// An unexpected survival is a harness signal, not a product win: it
			// means this suite is no longer proving that it can see the loss.
			// Check the drain window constants and the rotation-detection lag
			// before reading anything into the result.
			assert.Equal(t, postRotationMarkerLostCount, count,
				"%s: marker %s was appended to %s %dms after its rename, past every drain window, so it is expected to be lost; its survival means the harness is no longer calibrated and must be investigated before this run counts as a pass",
				c.name, marker.MarkerID, marker.RotatedFile, marker.MarkerAgeMs)
		case expect == markerLost:
			assert.Equal(t, postRotationMarkerLostCount, count,
				"%s: marker %s was appended to %s %dms after its rename, which lateMarkerProbes in provisioner.go predicts is past the %s reader's drain window; it was collected %d times. Run the cell with %s=%s and update the table",
				c.name, marker.MarkerID, marker.RotatedFile, marker.MarkerAgeMs, c.reader, count, runCalibrate, runCalibrateValue)
		case expect == markerEither:
			assert.LessOrEqual(t, count, 1,
				"%s: marker %s was appended to %s %dms after its rename, at the edge of the %s reader's drain window, so it may be lost, but it was collected %d times",
				c.name, marker.MarkerID, marker.RotatedFile, marker.MarkerAgeMs, c.reader, count)
		}
	}
}

// markerOutcome is what became of one marker, as calibration records it.
type markerOutcome struct {
	MarkerID    string            `json:"marker_id"`
	RotatedFile string            `json:"rotated_file"`
	AgeMs       int               `json:"marker_age_ms"`
	Expected    markerExpectation `json:"expected"`
	Collected   int               `json:"collected"`
	Outcome     markerExpectation `json:"outcome"`
	// Matches says whether the outcome is what the cell would assert
	// without calibration.
	Matches bool `json:"matches"`
}

// markerOutcomes records what became of each marker against its expectation.
func markerOutcomes(c cell, markers []markerEntry, counts map[string]int) []markerOutcome {
	outcomes := make([]markerOutcome, 0, len(markers))
	for _, marker := range markers {
		expect, ok := c.markers.expectationFor(marker.MarkerAgeMs)
		if !ok {
			continue
		}
		count := counts[marker.MarkerID]
		outcome := markerOutcome{
			MarkerID: marker.MarkerID, RotatedFile: marker.RotatedFile, AgeMs: marker.MarkerAgeMs,
			Expected: expect, Collected: count, Outcome: markerLost,
		}
		if count > 0 {
			outcome.Outcome = markerCollected
		}
		switch expect {
		case markerCollected:
			outcome.Matches = count == 1
		case markerLost:
			outcome.Matches = count == 0
		case markerEither:
			outcome.Matches = count <= 1
		}
		outcomes = append(outcomes, outcome)
	}
	sort.Slice(outcomes, func(i, j int) bool {
		if outcomes[i].AgeMs != outcomes[j].AgeMs {
			return outcomes[i].AgeMs < outcomes[j].AgeMs
		}
		return outcomes[i].MarkerID < outcomes[j].MarkerID
	})
	return outcomes
}

// summarizeMarkerOutcomes prints one line per marker age: how many of its
// markers were collected and lost, against what the cell expects.
func summarizeMarkerOutcomes(outcomes []markerOutcome) string {
	type tally struct {
		expected                   markerExpectation
		collected, lost, duplicate int
	}
	var ages []int
	tallies := make(map[int]*tally)
	for _, outcome := range outcomes {
		t, ok := tallies[outcome.AgeMs]
		if !ok {
			t = &tally{expected: outcome.Expected}
			tallies[outcome.AgeMs] = t
			ages = append(ages, outcome.AgeMs)
		}
		switch {
		case outcome.Collected > 1:
			t.duplicate++
		case outcome.Collected == 1:
			t.collected++
		default:
			t.lost++
		}
	}
	sort.Ints(ages)
	parts := make([]string, 0, len(ages))
	for _, age := range ages {
		t := tallies[age]
		total := t.collected + t.lost + t.duplicate
		part := fmt.Sprintf("%dms: collected %d/%d, lost %d/%d", age, t.collected, total, t.lost, total)
		if t.duplicate > 0 {
			part += fmt.Sprintf(", collected more than once %d/%d", t.duplicate, total)
		}
		parts = append(parts, part+fmt.Sprintf(" (expected %s)", t.expected))
	}
	return strings.Join(parts, "; ")
}

// recordMarkerOutcomes prints and keeps the calibration of a cell's markers.
func (suite *azureFilesSuite) recordMarkerOutcomes(c cell, outcomes []markerOutcome) {
	suite.T().Logf("%s calibration (%s=%s, not asserted): %s", c.name, runCalibrate, runCalibrateValue, summarizeMarkerOutcomes(outcomes))
	if evidence, err := suite.evidenceDir(); err == nil {
		evidence.writeJSON(c.name+"-marker-outcomes.json", outcomes)
	}
}

// markersForFiles keeps successful markers this run appended to the asserted
// rotations, which are keyed by the run ID of their stream.
func markersForFiles(entries []markerEntry, files map[markerKey]struct{}) []markerEntry {
	markers := make([]markerEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Status != "appended" {
			continue
		}
		if _, wanted := files[markerKey{runID: entry.RunID, file: entry.RotatedFile}]; !wanted {
			continue
		}
		markers = append(markers, entry)
	}
	return markers
}

// parseRecord returns the record a message carries, if any.
func parseRecord(message string) (recordKey, bool) {
	match := recordPattern.FindStringSubmatch(message)
	if len(match) != 3 {
		return recordKey{}, false
	}
	sequence, err := strconv.ParseInt(match[2], 10, 64)
	if err != nil {
		return recordKey{}, false
	}
	return recordKey{runID: match[1], sequence: sequence}, true
}

func countRecords(messages []string, expected map[recordKey]struct{}) map[recordKey]int {
	actual := make(map[recordKey]int, len(expected))
	for _, message := range messages {
		key, ok := parseRecord(message)
		if !ok {
			continue
		}
		if _, wanted := expected[key]; wanted {
			actual[key]++
		}
	}
	return actual
}

func countMarkerIDs(messages []string) map[string]int {
	counts := make(map[string]int)
	for _, message := range messages {
		if match := markerIDPattern.FindStringSubmatch(message); len(match) == 2 {
			counts[match[1]]++
		}
	}
	return counts
}

// collectedLog is the part of a Fakeintake log this suite checks.
type collectedLog struct {
	message string
	source  string
	tags    []string
	// timestamp is the log's timestamp, set by the Agent when it read the
	// line, in milliseconds since the epoch.
	timestamp int64
	// arrived is when Fakeintake received the payload.
	arrived time.Time
}

func (suite *azureFilesSuite) collectedLogs(service string) ([]collectedLog, error) {
	logs, err := suite.Env().FakeIntake.Client().FilterLogs(service)
	if err != nil {
		return nil, err
	}
	collected := make([]collectedLog, 0, len(logs))
	for _, log := range logs {
		collected = append(collected, collectedLog{
			message: log.Message, source: log.Source, tags: log.GetTags(),
			timestamp: int64(log.Timestamp), arrived: log.GetCollectedTime(),
		})
	}
	return collected, nil
}

func (suite *azureFilesSuite) collectedMessages(service string) ([]string, error) {
	logs, err := suite.collectedLogs(service)
	if err != nil {
		return nil, err
	}
	return logMessages(logs), nil
}

func logMessages(logs []collectedLog) []string {
	messages := make([]string, 0, len(logs))
	for _, log := range logs {
		messages = append(messages, log.message)
	}
	return messages
}

// assertLogOrigin requires every log of a cell's service to carry the source
// and tags its log source configures. Fakeintake is flushed before the Agent
// is installed, so a log of an earlier run on a reused stack fails it too.
// Only the first mismatch is reported.
func assertLogOrigin(t assert.TestingT, runID string, c cell, logs []collectedLog) {
	wantedTags := []string{"e2e_run_id:" + runID, "e2e_cell:" + c.name}
	for _, log := range logs {
		if !assert.Equal(t, "java", log.source, "%s: a collected log has the wrong source", c.name) {
			return
		}
		for _, tag := range wantedTags {
			if !assert.Contains(t, log.tags, tag, "%s: a collected log is missing the tag of its log source", c.name) {
				return
			}
		}
	}
}

// expectedRecords are the records of the ledger entries' sequence ranges that
// the writer did write.
func expectedRecords(entries []ledgerEntry) map[recordKey]struct{} {
	expected := make(map[recordKey]struct{})
	for _, entry := range entries {
		for sequence := entry.FirstSequence; sequence <= entry.LastSequence; sequence++ {
			if !inRanges(entry.UnwrittenSequences, sequence) {
				expected[recordKey{runID: entry.RunID, sequence: sequence}] = struct{}{}
			}
		}
	}
	return expected
}

// unwrittenRecords counts the records the writer failed to write.
func unwrittenRecords(entries []ledgerEntry) int64 {
	var count int64
	for _, entry := range entries {
		for _, sequences := range entry.UnwrittenSequences {
			count += sequences[1] - sequences[0] + 1
		}
	}
	return count
}

func inRanges(ranges [][2]int64, sequence int64) bool {
	for _, r := range ranges {
		if sequence >= r[0] && sequence <= r[1] {
			return true
		}
	}
	return false
}

func (suite *azureFilesSuite) agentPods() ([]corev1.Pod, error) {
	pods, err := suite.Env().KubernetesCluster.Client().CoreV1().Pods(agentNamespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: "app=" + suite.Env().Agent.LinuxNodeAgent.LabelSelectors["app"],
	})
	if err != nil {
		return nil, err
	}
	return pods.Items, nil
}

// requireAgentReady waits for the agent container of every Agent pod to be
// ready. The operator creates the pods after the Agent pass returns, so a pod
// that cannot start, for example on a volume it cannot mount, would otherwise
// only show up as every sequence missing.
func (suite *azureFilesSuite) requireAgentReady() {
	suite.T().Helper()
	suite.EventuallyWithT(func(collect *assert.CollectT) {
		pods, err := suite.agentPods()
		require.NoError(collect, err)
		require.NotEmpty(collect, pods, "no Agent pod")
		for _, pod := range pods {
			assert.True(collect, agentContainerReady(pod),
				"the Agent on %s is not ready: %s", pod.Name, suite.podProblems(pod))
		}
	}, 5*time.Minute, 10*time.Second)
}

func agentContainerReady(pod corev1.Pod) bool {
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name == "agent" {
			return status.Ready
		}
	}
	return false
}

// podProblems describes why a pod is not ready from its waiting containers and
// its Warning events; a volume that fails to mount only shows up as an event.
func (suite *azureFilesSuite) podProblems(pod corev1.Pod) string {
	var problems []string
	statuses := append(slices.Clone(pod.Status.InitContainerStatuses), pod.Status.ContainerStatuses...)
	for _, status := range statuses {
		if waiting := status.State.Waiting; waiting != nil {
			problems = append(problems, fmt.Sprintf("%s is waiting (%s) %s", status.Name, waiting.Reason, waiting.Message))
		}
	}
	events, err := suite.Env().KubernetesCluster.Client().CoreV1().Events(pod.Namespace).List(context.Background(), metav1.ListOptions{
		FieldSelector: "involvedObject.name=" + pod.Name + ",type=Warning",
	})
	if err != nil {
		problems = append(problems, "listing its events failed: "+err.Error())
	} else {
		for _, event := range events.Items {
			problems = append(problems, event.Reason+": "+event.Message)
		}
	}
	if len(problems) == 0 {
		return "phase " + string(pod.Status.Phase)
	}
	return strings.Join(problems, "; ")
}

func (suite *azureFilesSuite) writerPod(c cell) corev1.Pod {
	suite.T().Helper()
	pod, err := suite.findWriterPod(c)
	require.NoError(suite.T(), err)
	return pod
}

func (suite *azureFilesSuite) findWriterPod(c cell) (corev1.Pod, error) {
	pods, err := suite.Env().KubernetesCluster.Client().CoreV1().Pods(e2eNamespace).List(
		context.Background(),
		metav1.ListOptions{LabelSelector: cellLabel + "=" + c.name},
	)
	if err != nil {
		return corev1.Pod{}, err
	}
	if len(pods.Items) != 1 {
		return corev1.Pod{}, fmt.Errorf("expected one writer pod for %s, got %d", c.name, len(pods.Items))
	}
	return pods.Items[0], nil
}

func (suite *azureFilesSuite) readLedger(c cell, pod corev1.Pod) ([]ledgerEntry, error) {
	stdout, err := suite.readLedgerRaw(c, pod)
	if err != nil {
		return nil, err
	}
	return decodeJSONLines[ledgerEntry](stdout, "ledger")
}

// decodeJSONLines decodes a JSON Lines file of the share, the ledger or the
// marker journal.
func decodeJSONLines[T any](raw, what string) ([]T, error) {
	var entries []T
	for lineNumber, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry T
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, fmt.Errorf("decode %s line %d: %w", what, lineNumber+1, err)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// readLedgerRaw reads the ledgers of every stream of a writer pod, one after
// the other.
func (suite *azureFilesSuite) readLedgerRaw(c cell, pod corev1.Pod) (string, error) {
	stdout, err := suite.readShareFiles(pod, ledgerContainerName, c.writer.sharePaths(ledgerName))
	if err != nil {
		return "", fmt.Errorf("read ledger: %w", err)
	}
	return stdout, nil
}

func (suite *azureFilesSuite) readMarkerJournal(c cell, pod corev1.Pod) ([]markerEntry, error) {
	stdout, err := suite.readMarkerJournalRaw(c, pod)
	if err != nil {
		return nil, err
	}
	return decodeJSONLines[markerEntry](stdout, "marker journal")
}

func (suite *azureFilesSuite) readMarkerJournalRaw(c cell, pod corev1.Pod) (string, error) {
	stdout, err := suite.readShareFiles(pod, appenderContainerName, c.writer.sharePaths(postRotationMarkerJournalName))
	if err != nil {
		return "", fmt.Errorf("read marker journal: %w", err)
	}
	return stdout, nil
}

// readShareFilesScript prints every file of its arguments that exists, and
// fails when none does.
const readShareFilesScript = `found=; for f; do if [ -f "$f" ]; then cat "$f"; found=1; fi; done; test -n "$found"`

func (suite *azureFilesSuite) readShareFiles(pod corev1.Pod, container string, paths []string) (string, error) {
	stdout, stderr, err := suite.Env().KubernetesCluster.KubernetesClient.PodExec(
		e2eNamespace,
		pod.Name,
		container,
		append([]string{"/bin/sh", "-c", readShareFilesScript, "sh"}, paths...),
	)
	if err != nil {
		return "", fmt.Errorf("%s: %w (stderr: %s)", strings.Join(paths, ","), err, strings.TrimSpace(stderr))
	}
	return stdout, nil
}

func (suite *azureFilesSuite) writeRunMetadata() error {
	cells := make([]map[string]any, 0, len(suite.spec.cells))
	for _, c := range suite.spec.cells {
		entry := map[string]any{
			"name": c.name, "reader": c.reader, "service": c.service,
			"storage_account": c.accountName, "share": c.shareName,
			"mount_options": strings.Split(c.mountOptions, ","),
			"post_rotation_markers": map[string]any{
				"early_delay_ms": c.markers.earlyMs, "early_expectation": c.markers.earlyExpect,
				"late_delay_ms": c.markers.lateMs,
			},
			"writer":         c.writer.metadata(),
			"loss_accounted": c.lossAccounted(),
		}
		switch c.reader {
		case fileReader:
			entry["fingerprint"] = map[string]any{
				"strategy": c.fingerprint.strategy, "count": c.fingerprint.count, "max_bytes": c.fingerprint.maxBytes,
			}
		case smbReader:
			// The mount options apply to the writer only.
			entry["smb"] = map[string]any{
				"host": c.host(), "share": c.shareName, "username": c.accountName, "path": c.writer.activeLogPattern(),
				"password_handle": smbPasswordHandle(c), "poll_interval": smbPollIntervalSeconds,
			}
		}
		cells = append(cells, entry)
	}
	gated := make([]string, 0, len(suite.spec.gatedCells))
	for _, c := range suite.spec.gatedCells {
		gated = append(gated, c.name)
	}
	metadata := map[string]any{
		"run_id":     suite.spec.runID,
		"stack_name": suite.spec.stackName,
		"namespace":  e2eNamespace,
		"writer":     suite.spec.writerMetadata(),
		"profile": map[string]any{
			"name":             suite.spec.profile.name,
			"unreliable_mount": suite.spec.profile.unreliableMount,
			"file_scan_period": fileScanPeriodSeconds,
			"close_timeout":    closeTimeoutSeconds,
		},
		"calibrate": suite.spec.calibrate,
		// The marker ages depend on the reader and are listed per cell.
		"post_rotation_markers": map[string]any{
			"expected_surviving": postRotationMarkerSurvivingCount,
			"expected_lost":      postRotationMarkerLostCount,
		},
		"cells":       cells,
		"gated_cells": gated,
	}
	return writeJSON(filepath.Join(suite.SessionOutputDir(), "azure-files-run.json"), metadata)
}

// writerMetadata describes the writer pods. The stock image is recorded as
// its Docker Hub reference; the pod manifests in the evidence show the image
// each pod resolved.
func (spec runSpec) writerMetadata() map[string]any {
	if spec.workloadKind() == customWorkload {
		return map[string]any{"workload": customWorkload, "image": spec.writerImage, "options": spec.writer.metadata()}
	}
	return map[string]any{
		"workload":       stockWorkload,
		"image":          stockWorkloadImage,
		"config_map":     workloadConfigMapName,
		"scripts_sha256": spec.workloadRuntime("").scriptsChecksum(),
		"options":        spec.writer.metadata(),
	}
}

func (suite *azureFilesSuite) captureEvidence() {
	evidence, err := suite.evidenceDir()
	if err != nil {
		suite.T().Logf("cannot create evidence directory: %v", err)
		return
	}
	client := suite.Env().KubernetesCluster.Client()

	for _, c := range suite.spec.cells {
		pod, podErr := suite.findWriterPod(c)
		if podErr != nil {
			evidence.write(c.name+"-writer-pod-error.txt", []byte(podErr.Error()+"\n"))
			continue
		}
		evidence.writeJSON(c.name+"-writer-pod.json", pod)
		if rawLedger, err := suite.readLedgerRaw(c, pod); err == nil {
			evidence.write(c.name+"-ledger.jsonl", []byte(rawLedger))
		}
		if ledger, err := suite.readLedger(c, pod); err == nil {
			evidence.writeJSON(c.name+"-ledger.json", ledger)
		}
		if rawMarkers, err := suite.readMarkerJournalRaw(c, pod); err == nil {
			evidence.write(c.name+"-markers.jsonl", []byte(rawMarkers))
		}
		if rawPeriods, err := suite.readShareFiles(pod, writerContainerName, c.writer.sharePaths(periodsJournalName)); err == nil {
			evidence.write(c.name+"-periods.jsonl", []byte(rawPeriods))
		}
		for _, container := range []string{writerContainerName, ledgerContainerName, appenderContainerName} {
			logs, err := client.CoreV1().Pods(e2eNamespace).GetLogs(pod.Name, &corev1.PodLogOptions{Container: container}).DoRaw(context.Background())
			if err == nil {
				evidence.write(c.name+"-"+container+".log", logs)
			}
		}
		stdout, stderr, err := suite.Env().KubernetesCluster.KubernetesClient.PodExec(
			e2eNamespace, pod.Name, writerContainerName,
			[]string{"/bin/sh", "-c", fmt.Sprintf(
				"uname -a; cat /proc/mounts | grep %[1]s || true; ls -la %[1]s %[1]s/*/ 2>&1 || true; stat -c '%%D %%i %%s %%y %%n' %[1]s/%[2]s %[1]s/%[2]s.* 2>&1 || true; grep -H -a post_rotation_marker %[1]s/%[2]s.* 2>&1 || true",
				logMountPath, c.writer.activeLogPattern(),
			)},
		)
		evidence.writeCommandResult(c.name+"-writer-filesystem.txt", stdout, stderr, err)
		if !evidence.redactsEverySecret() {
			continue
		}
		if logs, err := suite.Env().FakeIntake.Client().FilterLogs(c.service); err == nil {
			evidence.writeJSON(c.name+"-fakeintake-logs.json", logs)
		}
	}

	// Agent output and shipped logs are where a leaked key would show up, so
	// they are only kept when every key can be redacted from them.
	agentPods, err := suite.agentPods()
	if !evidence.redactsEverySecret() {
		suite.T().Logf("skipping Agent and Fakeintake evidence: cannot redact the account key of %s", strings.Join(evidence.unredacted, ","))
	} else if err == nil {
		for _, pod := range agentPods {
			evidence.writeJSON(pod.Name+"-pod.json", pod)
			for _, container := range pod.Spec.Containers {
				logs, logErr := client.CoreV1().Pods(agentNamespace).GetLogs(pod.Name, &corev1.PodLogOptions{Container: container.Name}).DoRaw(context.Background())
				if logErr == nil {
					evidence.write(pod.Name+"-"+container.Name+".log", logs)
				}
			}
			stdout, stderr, execErr := suite.Env().KubernetesCluster.KubernetesClient.PodExec(
				agentNamespace, pod.Name, "agent",
				[]string{"/bin/sh", "-c", fmt.Sprintf(
					"uname -a; cat /proc/mounts | grep %[1]s || true; stat -c '%%D %%i %%s %%y %%n' %[1]s/*/%[2]s %[1]s/*/*/%[2]s 2>&1 || true; ls -l /proc/1/fd 2>&1 || true",
					logMountPath, activeLogName,
				)},
			)
			evidence.writeCommandResult(pod.Name+"-filesystem.txt", stdout, stderr, execErr)
			// The status shows each source's errors, including SMB
			// authentication and connection failures, and, verbose, each
			// tailer with the bytes it read.
			stdout, stderr, execErr = suite.Env().KubernetesCluster.KubernetesClient.PodExec(
				agentNamespace, pod.Name, "agent", []string{"agent", "status", "--verbose"},
			)
			evidence.writeCommandResult(pod.Name+"-status.txt", stdout, stderr, execErr)
			stdout, stderr, execErr = suite.Env().KubernetesCluster.KubernetesClient.PodExec(
				agentNamespace, pod.Name, "agent", []string{"agent", "status", "--json", "--verbose"},
			)
			evidence.writeCommandResult(pod.Name+"-status.json.txt", stdout, stderr, execErr)
			registry, registryStderr, registryErr := suite.Env().KubernetesCluster.KubernetesClient.PodExec(
				agentNamespace, pod.Name, "agent",
				[]string{"/bin/sh", "-c", "cat /opt/datadog-agent/run/registry.json"},
			)
			evidence.writeCommandResult(pod.Name+"-registry.json.txt", registry, registryStderr, registryErr)
		}
	}

	suite.T().Logf("evidence: %s", evidence.dir)
	suite.T().Logf("run resources: stack=%s namespace=%s accounts=%s", suite.spec.stackName, e2eNamespace, suite.storageAccountNames())
}

// evidenceDir returns the evidence directory of this run, creating it on first
// use once the Agent pass has run. Everything written to it has the SMB cells'
// storage account keys redacted, so a leak that the assertions catch is not
// copied into the artifacts as well.
func (suite *azureFilesSuite) evidenceDir() (*evidenceDir, error) {
	if suite.evidence != nil {
		return suite.evidence, nil
	}
	dir := filepath.Join(suite.SessionOutputDir(), "azure-files-evidence", suite.spec.runID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	evidence := &evidenceDir{dir: dir}
	for _, c := range suite.spec.cells {
		if c.reader != smbReader {
			continue
		}
		keys, err := suite.cellKeys(c)
		if err != nil {
			suite.T().Logf("cannot read the %s account keys to redact them from evidence: %v", c.name, err)
			evidence.unredacted = append(evidence.unredacted, c.name)
			continue
		}
		evidence.secrets = append(evidence.secrets, keys...)
	}
	suite.evidence = evidence
	return evidence, nil
}

// cellKeys are the storage account keys of an SMB cell that the test knows:
// the writer's, and the Agent's copy when it holds another key.
func (suite *azureFilesSuite) cellKeys(c cell) ([]string, error) {
	writerKey, err := suite.accountKey(c)
	if err != nil {
		return nil, err
	}
	agentKey, err := suite.agentAccountKey(c)
	if err != nil {
		return nil, err
	}
	keys := []string{writerKey}
	if agentKey != writerKey {
		keys = append(keys, agentKey)
	}
	return keys, nil
}

func (suite *azureFilesSuite) storageAccountNames() string {
	names := make([]string, 0, len(suite.spec.cells))
	for _, c := range suite.spec.cells {
		names = append(names, c.accountName)
	}
	return strings.Join(names, ",")
}

// evidenceDir writes best-effort evidence files with secrets redacted.
type evidenceDir struct {
	dir     string
	secrets []string
	// unredacted lists the SMB cells whose key could not be read, and so
	// cannot be redacted.
	unredacted []string
}

// redactsEverySecret reports whether every SMB cell's key is known. Output
// that may contain a key is only written when it is.
func (e *evidenceDir) redactsEverySecret() bool {
	return len(e.unredacted) == 0
}

func (e *evidenceDir) redact(content []byte) []byte {
	return []byte(redactSecrets(string(content), e.secrets...))
}

// redactSecrets replaces every secret in text, for evidence files and for any
// command output that a failure message quotes.
func redactSecrets(text string, secrets ...string) string {
	for _, secret := range secrets {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, redactedAccountKey)
		}
	}
	return text
}

func (e *evidenceDir) write(name string, content []byte) {
	_ = os.WriteFile(filepath.Join(e.dir, name), e.redact(content), 0o600)
}

func (e *evidenceDir) writeJSON(name string, value any) {
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		e.write(name+".error.txt", []byte(err.Error()+"\n"))
		return
	}
	e.write(name, append(content, '\n'))
}

func (e *evidenceDir) writeCommandResult(name, stdout, stderr string, err error) {
	e.write(name, []byte(fmt.Sprintf("error: %v\nstdout:\n%s\nstderr:\n%s\n", err, stdout, stderr)))
}

func writeJSON(path string, value any) error {
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	return os.WriteFile(path, content, 0o600)
}
