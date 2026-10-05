// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package azurefiles

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testRunID       = "20260826t120000z-123456"
	testWriterImage = "registry.example/log-writer@sha256:abc"
)

func TestRunSpecBuildsEveryCell(t *testing.T) {
	spec, err := newRunSpec(testRunID, testWriterImage, "", "")
	require.NoError(t, err)
	require.Len(t, spec.cells, 3)
	assert.Equal(t, defaultProfile, spec.profile)

	values := spec.agentHelmValues()
	assert.Contains(t, values, "fingerprint_strategy: line_checksum")
	assert.Contains(t, values, "fingerprint_strategy: byte_checksum")
	assert.Contains(t, values, "count: 2048")
	assert.Len(t, regexp.MustCompile(`(?m)^\s+- type: file$`).FindAllStringIndex(values, -1), 3)
	// The per-source rotation keys were replaced by the node-wide profile.
	assert.NotContains(t, values, "rotation_handoff_mode")
	assert.NotContains(t, values, "sequential_rotation_")
	assert.NotContains(t, values, "open_flags")

	for _, c := range spec.cells {
		assert.Regexp(t, `^[a-z0-9]{3,24}$`, c.accountName)
		assert.Regexp(t, `^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`, c.shareName)
		assert.Contains(t, values, "service: "+c.service)
		assert.Contains(t, values, "- e2e_cell:"+c.name)
		assert.Equal(t, fileReader, c.reader)
		assert.Contains(t, values, "storageAccount: "+c.accountName)
		assert.Contains(t, values, "shareName: "+c.shareName)
		assert.Contains(t, values, fmt.Sprintf("mountOptions: %q", c.mountOptions))
		assert.Contains(t, values, "mountPath: "+agentMountPath(c))
	}
}

func TestRunSpecSelectsTheAgentProfile(t *testing.T) {
	defaultSpec, err := newRunSpec(testRunID, testWriterImage, "default", "")
	require.NoError(t, err)
	assert.Contains(t, defaultSpec.agentHelmValues(),
		"- name: DD_LOGS_CONFIG_UNRELIABLE_MOUNT_ENABLED\n      value: \"false\"\n")
	assert.Contains(t, defaultSpec.agentHelmValues(), "- e2e_profile:default\n")

	unreliableSpec, err := newRunSpec(testRunID, testWriterImage, " unreliable-mount ", "")
	require.NoError(t, err)
	assert.True(t, unreliableSpec.profile.unreliableMount)
	assert.Contains(t, unreliableSpec.agentHelmValues(),
		"- name: DD_LOGS_CONFIG_UNRELIABLE_MOUNT_ENABLED\n      value: \"true\"\n")
	assert.Contains(t, unreliableSpec.agentHelmValues(), "- e2e_profile:unreliable-mount\n")

	// The profile changes only the node-wide setting, not the cells.
	assert.Equal(t, defaultSpec.cells, unreliableSpec.cells)

	_, err = newRunSpec(testRunID, testWriterImage, "sequential", "")
	assert.ErrorContains(t, err, `unknown profile "sequential"`)
}

func TestRunSpecPairsEveryMountConfigurationWithACell(t *testing.T) {
	spec, err := newRunSpec(testRunID, testWriterImage, "", "")
	require.NoError(t, err)

	byMountOptions := make(map[string][]string)
	for _, c := range spec.cells {
		byMountOptions[c.mountOptions] = append(byMountOptions[c.mountOptions], c.name)
	}
	assert.Equal(t, []string{"file-line", "file-byte"}, byMountOptions[mountOptionsActimeo1])
	assert.Equal(t, []string{"file-line-actimeo30"}, byMountOptions[mountOptionsActimeo30])

	// The cells must differ only in the attribute cache lifetime, so a result
	// difference cannot come from any other mount option.
	assert.Equal(t,
		strings.Replace(mountOptionsActimeo1, "actimeo=1", "actimeo=30", 1),
		mountOptionsActimeo30)
}

func TestRunSpecSelectsIndividualCells(t *testing.T) {
	spec, err := newRunSpec(testRunID, testWriterImage, "", " file-line-actimeo30 , file-byte ")
	require.NoError(t, err)
	require.Len(t, spec.cells, 2)
	assert.Equal(t, "file-line-actimeo30", spec.cells[0].name)
	assert.Equal(t, "file-byte", spec.cells[1].name)

	_, err = newRunSpec(testRunID, testWriterImage, "", "line")
	assert.ErrorContains(t, err, `unknown cell "line"`)
}

func TestPostRotationMarkerDelaysStraddleEveryDrainWindow(t *testing.T) {
	// The shortest drain is the default profile's: the rotated tailer keeps
	// reading for close_timeout after it sees the rotation.
	assert.Less(t, postRotationMarkerSurvivingDelayMs, closeTimeoutSeconds*1000)

	// The longest drain at actimeo=1 is the unreliable-mount handoff: it sees
	// the rotation after a scan and an attribute cache refresh, reads the
	// surviving marker, then waits out its quiet period.
	longestDrainMs := (fileScanPeriodSeconds+1)*1000 + postRotationMarkerSurvivingDelayMs + fileHandoffQuietSeconds*1000
	assert.Greater(t, postRotationMarkerLostDelayMs, longestDrainMs)
	assert.Greater(t, postRotationMarkerLostDelayMs, (fileScanPeriodSeconds+1+closeTimeoutSeconds)*1000)

	// The appender appends inline, so it must be done before the next rotation.
	assert.Less(t, postRotationMarkerLostDelayMs, writerRotationSeconds*1000)
	assert.Equal(t, "1500,45000", postRotationMarkerDelaysMs())
}

func TestExpectedSequencesUsesLedgerRanges(t *testing.T) {
	expected := expectedSequences([]ledgerEntry{
		{FirstSequence: 1, LastSequence: 3},
		{FirstSequence: 7, LastSequence: 8},
	})
	assert.Equal(t, map[int64]struct{}{1: {}, 2: {}, 3: {}, 7: {}, 8: {}}, expected)
}

func TestCountSequencesOnlyCountsExpectedSequences(t *testing.T) {
	counts := countSequences([]string{
		"run_id=r period=p sequence=1 phase=head",
		"run_id=r period=p sequence=1 phase=head",
		"run_id=r period=p sequence=9 phase=fill",
		"post_rotation_marker run_id=r rotation=1 marker_id=r-r1-m1500",
	}, map[int64]struct{}{1: {}, 2: {}})
	assert.Equal(t, map[int64]int{1: 2}, counts)
}

func TestCountMarkerIDsIgnoresOrdinaryRecords(t *testing.T) {
	counts := countMarkerIDs([]string{
		"post_rotation_marker run_id=r rotation=1 marker_age_ms=1500 marker_id=r-r1-m1500 rotated_file=app.log.1",
		"post_rotation_marker run_id=r rotation=1 marker_age_ms=1500 marker_id=r-r1-m1500 rotated_file=app.log.1",
		"run_id=r period=p sequence=4 phase=fill",
	})
	assert.Equal(t, map[string]int{"r-r1-m1500": 2}, counts)
}

func TestMarkersForFilesKeepsOnlyThisRunAndTheseRotations(t *testing.T) {
	journal := []markerEntry{
		{RunID: "run-file-line", MarkerID: "keep", RotatedFile: "app.log.a", Status: "appended"},
		{RunID: "run-file-line", MarkerID: "other-run-file", RotatedFile: "app.log.z", Status: "appended"},
		{RunID: "older-run", MarkerID: "older-run", RotatedFile: "app.log.a", Status: "appended"},
		{RunID: "run-file-line", MarkerID: "failed-append", RotatedFile: "app.log.a", Status: "failed"},
	}
	markers := markersForFiles(journal, "run-file-line", map[string]struct{}{"app.log.a": {}})
	require.Len(t, markers, 1)
	assert.Equal(t, "keep", markers[0].MarkerID)
}

func TestAssertMarkerOutcomeRequiresTheEarlyMarkerAndForbidsTheLateOne(t *testing.T) {
	markers := []markerEntry{
		{MarkerID: "early", MarkerAgeMs: postRotationMarkerSurvivingDelayMs, RotatedFile: "app.log.a"},
		{MarkerID: "late", MarkerAgeMs: postRotationMarkerLostDelayMs, RotatedFile: "app.log.a"},
	}
	c := cell{name: "file-line"}

	calibrated := new(recordingT)
	assertMarkerOutcome(calibrated, c, markers, map[string]int{"early": 1})
	assert.Empty(t, calibrated.failures)

	lostEarlyMarker := new(recordingT)
	assertMarkerOutcome(lostEarlyMarker, c, markers, map[string]int{})
	assert.Len(t, lostEarlyMarker.failures, 1)

	uncalibrated := new(recordingT)
	assertMarkerOutcome(uncalibrated, c, markers, map[string]int{"early": 1, "late": 1})
	assert.Len(t, uncalibrated.failures, 1)
}

func TestFirstStorageAccountKey(t *testing.T) {
	key, err := firstStorageAccountKey([]any{map[string]any{"value": "secret-value"}})
	require.NoError(t, err)
	assert.Equal(t, "secret-value", key)

	_, err = firstStorageAccountKey(nil)
	assert.Error(t, err)
}

// recordingT captures assertion failures instead of failing the test, so the
// marker calibration itself can be asserted on.
type recordingT struct {
	failures []string
}

func (t *recordingT) Errorf(format string, args ...any) {
	t.failures = append(t.failures, fmt.Sprintf(format, args...))
}
