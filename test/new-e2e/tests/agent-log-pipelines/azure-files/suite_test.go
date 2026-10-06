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
	// The smb cell needs an Agent built with the native SMB log source, which
	// a stock build does not have, so it only runs on request.
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
)

var (
	sequencePattern = regexp.MustCompile(`\bsequence=([0-9]+)\b`)
	markerIDPattern = regexp.MustCompile(`\bmarker_id=([^\s]+)`)
)

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
		runID:       runID,
		writerImage: writerImage,
		profile:     os.Getenv(runProfile),
		cells:       os.Getenv(runCells),
		stackName:   os.Getenv(runStackName),
		smbEnabled:  os.Getenv(smbOptIn) == smbOptInValue,
	})
	require.NoError(t, err, "%s, %s or %s is invalid", runProfile, runCells, runStackName)
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

// gateReason says how to enable the gated smb cell.
func gateReason() string {
	return fmt.Sprintf("set %s=%s to run the smb cell; it needs an Agent built with the native SMB log source (see README.md)", smbOptIn, smbOptInValue)
}

func (suite *azureFilesSuite) TestRotatedFilesAreCollectedExactlyOnce() {
	for _, c := range suite.spec.gatedCells {
		suite.Run(c.name, func() {
			suite.T().Skip(gateReason())
		})
	}

	// A reused stack keeps its Fakeintake and the logs of earlier runs, which
	// carry the same services and sequence numbers. The storage pass has just
	// removed the Agent, so nothing new arrives before the Agent pass below.
	require.NoError(suite.T(), suite.Env().FakeIntake.Client().FlushServerAndResetAggregators())
	suite.UpdateEnv(suite.spec.agentProvisioner())
	require.NoError(suite.T(), suite.writeRunMetadata())
	defer suite.captureEvidence()

	for _, c := range suite.spec.cells {
		suite.Run(c.name, func() {
			if c.reader == smbReader {
				// Fail with a clear message when the Agent cannot run the
				// source at all, rather than with every sequence missing, and
				// before waiting minutes for the writer's rotations.
				suite.requireSMBSourceRunning(c)
			}

			pod := suite.writerPod(c)
			var ledger []ledgerEntry
			suite.EventuallyWithT(func(collect *assert.CollectT) {
				var err error
				ledger, err = suite.readLedger(pod)
				require.NoError(collect, err)
				assert.GreaterOrEqual(collect, len(ledger), completedFiles,
					"writer has not completed %d rotations yet; current ledger has %d", completedFiles, len(ledger))
			}, 6*time.Minute, 5*time.Second)

			sort.Slice(ledger, func(i, j int) bool { return ledger[i].Period < ledger[j].Period })
			asserted := ledger[:assertedFiles]
			expected := expectedSequences(asserted)
			markers := suite.postRotationMarkers(pod, c, asserted)

			suite.EventuallyWithT(func(collect *assert.CollectT) {
				logs, err := suite.collectedLogs(c.service)
				require.NoError(collect, err)
				// Lines carrying the right service but not this source's
				// metadata come from a reader that builds a wrong origin.
				assertLogOrigin(collect, suite.spec.runID, c, logs)

				messages := logMessages(logs)
				actual := countSequences(messages, expected)
				assert.Equal(collect, len(expected), len(actual),
					"%s did not collect every ledger sequence", c.name)
				for sequence := range expected {
					assert.Equal(collect, 1, actual[sequence],
						"%s sequence %d must be collected exactly once", c.name, sequence)
				}
				assertMarkerOutcome(collect, c, markers, countMarkerIDs(messages))
			}, 2*time.Minute, 10*time.Second)

			// The lost marker is asserted absent, which cannot fail on a first
			// look, so confirm the outcome once every drain has certainly ended.
			time.Sleep(markerSettleDelay)
			messages, err := suite.collectedMessages(c.service)
			require.NoError(suite.T(), err)
			assertMarkerOutcome(suite.T(), c, markers, countMarkerIDs(messages))

			if c.reader == smbReader {
				suite.checkSMBSource(c)
			}
		})
	}
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
	runID := suite.spec.writerRunID(c)
	files := make(map[string]struct{}, len(asserted))
	for _, entry := range asserted {
		files[entry.File] = struct{}{}
	}
	wanted := assertedFiles * 2

	var markers []markerEntry
	suite.EventuallyWithT(func(collect *assert.CollectT) {
		journal, err := suite.readMarkerJournal(pod)
		require.NoError(collect, err)
		markers = markersForFiles(journal, runID, files)
		assert.Len(collect, markers, wanted,
			"the appender must record %d post-rename markers for the %d asserted rotations of %s; the journal has %d",
			wanted, assertedFiles, c.name, len(markers))
	}, 3*time.Minute, 5*time.Second)
	return markers
}

// assertMarkerOutcome encodes the calibration described by the marker constants
// in provisioner.go: the early marker must survive, and the late one must not.
func assertMarkerOutcome(t assert.TestingT, c cell, markers []markerEntry, counts map[string]int) {
	for _, marker := range markers {
		switch marker.MarkerAgeMs {
		case c.markers.survivingMs:
			assert.Equal(t, postRotationMarkerSurvivingCount, counts[marker.MarkerID],
				"%s: marker %s was appended to %s %dms after its rename, inside the drain window of its %s reader, so it must be collected exactly once; losing it means appends to the rotated file are dropped",
				c.name, marker.MarkerID, marker.RotatedFile, marker.MarkerAgeMs, c.reader)
		case c.markers.lostMs:
			// An unexpected survival is a harness signal, not a product win: it
			// means this suite is no longer proving that it can see the loss.
			// Check the drain window constants and the rotation-detection lag
			// before reading anything into the result.
			assert.Equal(t, postRotationMarkerLostCount, counts[marker.MarkerID],
				"%s: marker %s was appended to %s %dms after its rename, past every drain window, so it is expected to be lost; its survival means the harness is no longer calibrated and must be investigated before this run counts as a pass",
				c.name, marker.MarkerID, marker.RotatedFile, marker.MarkerAgeMs)
		}
	}
}

// markersForFiles keeps successful markers this run appended to the asserted
// rotations.
func markersForFiles(entries []markerEntry, runID string, files map[string]struct{}) []markerEntry {
	markers := make([]markerEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.RunID != runID || entry.Status != "appended" {
			continue
		}
		if _, wanted := files[entry.RotatedFile]; !wanted {
			continue
		}
		markers = append(markers, entry)
	}
	return markers
}

func countSequences(messages []string, expected map[int64]struct{}) map[int64]int {
	actual := make(map[int64]int, len(expected))
	for _, message := range messages {
		match := sequencePattern.FindStringSubmatch(message)
		if len(match) != 2 {
			continue
		}
		sequence, err := strconv.ParseInt(match[1], 10, 64)
		if err != nil {
			continue
		}
		if _, wanted := expected[sequence]; wanted {
			actual[sequence]++
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
}

func (suite *azureFilesSuite) collectedLogs(service string) ([]collectedLog, error) {
	logs, err := suite.Env().FakeIntake.Client().FilterLogs(service)
	if err != nil {
		return nil, err
	}
	collected := make([]collectedLog, 0, len(logs))
	for _, log := range logs {
		collected = append(collected, collectedLog{message: log.Message, source: log.Source, tags: log.GetTags()})
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

func expectedSequences(entries []ledgerEntry) map[int64]struct{} {
	expected := make(map[int64]struct{})
	for _, entry := range entries {
		for sequence := entry.FirstSequence; sequence <= entry.LastSequence; sequence++ {
			expected[sequence] = struct{}{}
		}
	}
	return expected
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

func (suite *azureFilesSuite) readLedger(pod corev1.Pod) ([]ledgerEntry, error) {
	stdout, err := suite.readLedgerRaw(pod)
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

func (suite *azureFilesSuite) readLedgerRaw(pod corev1.Pod) (string, error) {
	stdout, err := suite.readShareFile(pod, ledgerContainerName, logMountPath+"/"+ledgerName)
	if err != nil {
		return "", fmt.Errorf("read ledger: %w", err)
	}
	return stdout, nil
}

func (suite *azureFilesSuite) readMarkerJournal(pod corev1.Pod) ([]markerEntry, error) {
	stdout, err := suite.readMarkerJournalRaw(pod)
	if err != nil {
		return nil, err
	}
	return decodeJSONLines[markerEntry](stdout, "marker journal")
}

func (suite *azureFilesSuite) readMarkerJournalRaw(pod corev1.Pod) (string, error) {
	stdout, err := suite.readShareFile(pod, appenderContainerName, logMountPath+"/"+postRotationMarkerJournalName)
	if err != nil {
		return "", fmt.Errorf("read marker journal: %w", err)
	}
	return stdout, nil
}

func (suite *azureFilesSuite) readShareFile(pod corev1.Pod, container, path string) (string, error) {
	stdout, stderr, err := suite.Env().KubernetesCluster.KubernetesClient.PodExec(
		e2eNamespace,
		pod.Name,
		container,
		[]string{"/bin/sh", "-c", "test -f " + path + " && cat " + path},
	)
	if err != nil {
		return "", fmt.Errorf("%s: %w (stderr: %s)", path, err, strings.TrimSpace(stderr))
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
				"surviving_delay_ms": c.markers.survivingMs, "lost_delay_ms": c.markers.lostMs,
			},
		}
		switch c.reader {
		case fileReader:
			entry["fingerprint"] = map[string]any{
				"strategy": c.fingerprint.strategy, "count": c.fingerprint.count, "max_bytes": c.fingerprint.maxBytes,
			}
		case smbReader:
			// The mount options apply to the writer only.
			entry["smb"] = map[string]any{
				"host": c.host(), "share": c.shareName, "username": c.accountName, "path": activeLogName,
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
		return map[string]any{"workload": customWorkload, "image": spec.writerImage}
	}
	return map[string]any{
		"workload":       stockWorkload,
		"image":          stockWorkloadImage,
		"config_map":     workloadConfigMapName,
		"scripts_sha256": spec.workloadRuntime("").scriptsChecksum(),
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
		if rawLedger, err := suite.readLedgerRaw(pod); err == nil {
			evidence.write(c.name+"-ledger.jsonl", []byte(rawLedger))
		}
		if ledger, err := suite.readLedger(pod); err == nil {
			evidence.writeJSON(c.name+"-ledger.json", ledger)
		}
		if rawMarkers, err := suite.readMarkerJournalRaw(pod); err == nil {
			evidence.write(c.name+"-markers.jsonl", []byte(rawMarkers))
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
				"uname -a; cat /proc/mounts | grep %[1]s || true; stat -c '%%D %%i %%s %%y %%n' %[1]s/%[2]s %[1]s/%[2]s.* 2>&1 || true; grep -H post_rotation_marker %[1]s/%[2]s.* 2>&1 || true",
				logMountPath, activeLogName,
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
					"uname -a; cat /proc/mounts | grep %[1]s || true; stat -c '%%D %%i %%s %%y %%n' %[1]s/*/%[2]s 2>&1 || true; ls -l /proc/1/fd 2>&1 || true",
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
		key, err := suite.accountKey(c)
		if err != nil {
			suite.T().Logf("cannot read the %s account key to redact it from evidence: %v", c.name, err)
			evidence.unredacted = append(evidence.unredacted, c.name)
			continue
		}
		evidence.secrets = append(evidence.secrets, key)
	}
	suite.evidence = evidence
	return evidence, nil
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
