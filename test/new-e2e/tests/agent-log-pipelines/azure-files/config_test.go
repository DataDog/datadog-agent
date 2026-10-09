// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package azurefiles

import (
	"archive/zip"
	"bytes"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	k8scorev1 "k8s.io/api/core/v1"
)

const (
	testRunID       = "20260826t120000z-123456"
	testWriterImage = "registry.example/log-writer@sha256:abc"
)

// testRunSpec builds a run spec with the test run ID unless opts sets its own.
// Like a run, it uses the stock workload unless opts names a writer image.
func testRunSpec(t *testing.T, opts runOptions) runSpec {
	t.Helper()
	if opts.runID == "" {
		opts.runID = testRunID
	}
	spec, err := newRunSpec(opts)
	require.NoError(t, err)
	return spec
}

func TestRunSpecBuildsEveryCell(t *testing.T) {
	spec := testRunSpec(t, runOptions{smbEnabled: true})
	require.Len(t, spec.cells, 4)
	assert.Empty(t, spec.gatedCells)
	assert.Equal(t, defaultProfile, spec.profile)

	values := spec.agentHelmValues()
	assert.Contains(t, values, "fingerprint_strategy: line_checksum")
	assert.Contains(t, values, "fingerprint_strategy: byte_checksum")
	assert.Contains(t, values, "count: 2048")
	assert.Len(t, regexp.MustCompile(`(?m)^\s+- type: file$`).FindAllStringIndex(values, -1), 3)
	assert.Len(t, regexp.MustCompile(`(?m)^\s+- type: smb$`).FindAllStringIndex(values, -1), 1)
	// The per-source rotation keys were replaced by the node-wide profile.
	assert.NotContains(t, values, "rotation_handoff_mode")
	assert.NotContains(t, values, "sequential_rotation_")
	assert.NotContains(t, values, "open_flags")
	// The CSI driver reads an inline volume's Secret from the Agent pod's
	// namespace, where the storage pass copies it.
	assert.Len(t, regexp.MustCompile(`(?m)^\s+secretNamespace: `+agentNamespace+`$`).FindAllStringIndex(values, -1), 3)
	assert.NotContains(t, values, "secretNamespace: "+e2eNamespace)

	for _, c := range spec.cells {
		assert.Regexp(t, `^[a-z0-9]{3,24}$`, c.accountName)
		assert.Regexp(t, `^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`, c.shareName)
		assert.Contains(t, values, "service: "+c.service)
		assert.Contains(t, values, "- e2e_cell:"+c.name)
		switch c.reader {
		case fileReader:
			assert.Contains(t, values, "storageAccount: "+c.accountName)
			assert.Contains(t, values, "shareName: "+c.shareName)
			assert.Contains(t, values, fmt.Sprintf("mountOptions: %q", c.mountOptions))
			assert.Contains(t, values, "mountPath: "+agentMountPath(c))
		case smbReader:
			// The Agent reads the share itself, so it mounts nothing for it.
			assert.NotContains(t, values, "storageAccount: "+c.accountName)
			assert.Contains(t, values, "host: "+c.accountName+".file.core.windows.net")
			assert.Contains(t, values, "share: "+c.shareName)
			assert.Contains(t, values, "username: "+c.accountName)
		}
	}
}

func TestSMBCellReadsItsKeyFromAMountedSecret(t *testing.T) {
	spec := testRunSpec(t, runOptions{cells: "smb", smbEnabled: true})
	require.Len(t, spec.cells, 1)
	c := spec.cells[0]

	values := spec.agentHelmValues()
	assert.Contains(t, values, `password: "ENC[file@/etc/azure-files-secrets/smb/azurestorageaccountkey]"`)
	assert.Contains(t, values, "poll_interval: 1")
	assert.Contains(t, values, "path: "+activeLogName+"\n")
	// The key is a file of the Secret copied into the Agent namespace.
	assert.Contains(t, values, "    - name: azure-files-smb-key\n      secret:\n        secretName: "+c.secretName()+"\n")
	assert.Contains(t, values, "          - key: "+accountKeySecretKey+"\n            path: "+accountKeySecretKey+"\n")
	assert.Contains(t, values, "    - name: azure-files-smb-key\n      mountPath: /etc/azure-files-secrets/smb\n      readOnly: true\n")
	// Reading a mounted file takes no Kubernetes API permission.
	assert.Contains(t, values, "  secretBackend:\n    command: /readsecret_multiple_providers.sh\n    enableGlobalPermissions: false\n  confd:")
	assert.NotContains(t, values, "roles:")
	assert.NotContains(t, values, "k8s_secret@")
	// The Agent reads the share itself, so it mounts no CSI volume for it.
	assert.NotContains(t, values, "csi:")
}

func TestSMBCellIsGatedWithoutItsOptIn(t *testing.T) {
	spec := testRunSpec(t, runOptions{})
	require.Len(t, spec.gatedCells, 1)
	assert.Equal(t, "smb", spec.gatedCells[0].name)
	assert.Len(t, spec.cells, 3)
	for _, c := range spec.cells {
		assert.Equal(t, fileReader, c.reader)
	}
	values := spec.agentHelmValues()
	assert.NotContains(t, values, "type: smb")
	assert.NotContains(t, values, "secretBackend:")

	// Selecting only the smb cell without its opt-in leaves nothing to run.
	onlySMB := testRunSpec(t, runOptions{cells: "smb"})
	assert.Empty(t, onlySMB.cells)
	require.Len(t, onlySMB.gatedCells, 1)
	assert.False(t, onlySMB.hasReader(smbReader))
}

func TestFileCellsNeedNoSecretBackend(t *testing.T) {
	spec := testRunSpec(t, runOptions{cells: "file-line,file-byte", smbEnabled: true})

	values := spec.agentHelmValues()
	assert.NotContains(t, values, "secretBackend:")
	assert.NotContains(t, values, "ENC[")
	assert.NotContains(t, values, "secret:")
	assert.Contains(t, values, "\n  volumes:\n    - name: azure-files-file-line\n      csi:\n")
}

func TestReusedStackKeepsAccountsAndRenewsShares(t *testing.T) {
	first := testRunSpec(t, runOptions{stackName: "azure-files-dev", smbEnabled: true})
	second := testRunSpec(t, runOptions{runID: "20260826t130000z-654321", stackName: " azure-files-dev ", smbEnabled: true})
	assert.Equal(t, "azure-files-dev", first.stackName)
	assert.Equal(t, first.stackName, second.stackName)
	require.Len(t, second.cells, len(first.cells))
	for i := range first.cells {
		assert.Equal(t, first.cells[i].accountName, second.cells[i].accountName)
		assert.NotEqual(t, first.cells[i].shareName, second.cells[i].shareName)
	}

	// Without a reused stack every run gets its own stack and accounts.
	own := testRunSpec(t, runOptions{smbEnabled: true})
	other := testRunSpec(t, runOptions{runID: "20260826t130000z-654321", smbEnabled: true})
	assert.Regexp(t, `^azure-files-[0-9a-f]{8}$`, own.stackName)
	assert.NotEqual(t, own.stackName, other.stackName)
	assert.NotEqual(t, own.cells[0].accountName, other.cells[0].accountName)

	for _, invalid := range []string{"Azure-Files", "azure_files", "-azure", strings.Repeat("a", 41)} {
		_, err := newRunSpec(runOptions{runID: testRunID, writerImage: testWriterImage, stackName: invalid})
		assert.ErrorContains(t, err, "stack name", invalid)
	}
}

func TestRunSpecSelectsTheAgentProfile(t *testing.T) {
	defaultSpec := testRunSpec(t, runOptions{profile: "default"})
	assert.Contains(t, defaultSpec.agentHelmValues(),
		"agents:\n  containers:\n    agent:\n      envDict:\n"+
			"        DD_LOGS_CONFIG_FILE_SCAN_PERIOD: \"1\"\n"+
			"        DD_LOGS_CONFIG_CLOSE_TIMEOUT: \"5\"\n"+
			"        DD_LOGS_CONFIG_UNRELIABLE_MOUNT_ENABLED: \"false\"\n")
	assert.Contains(t, defaultSpec.agentHelmValues(), "- e2e_profile:default\n")
	// A datadog.env list would replace the framework's, which sends the
	// Agent's payloads to Fakeintake.
	assert.NotRegexp(t, `(?m)^  env:`, defaultSpec.agentHelmValues())

	unreliableSpec := testRunSpec(t, runOptions{profile: " unreliable-mount "})
	assert.True(t, unreliableSpec.profile.unreliableMount)
	assert.Contains(t, unreliableSpec.agentHelmValues(),
		"        DD_LOGS_CONFIG_UNRELIABLE_MOUNT_ENABLED: \"true\"\n")
	assert.Contains(t, unreliableSpec.agentHelmValues(), "- e2e_profile:unreliable-mount\n")

	// The profile changes only the node-wide setting, not the cells.
	assert.Equal(t, defaultSpec.cells, unreliableSpec.cells)

	_, err := newRunSpec(runOptions{runID: testRunID, writerImage: testWriterImage, profile: "sequential"})
	assert.ErrorContains(t, err, `unknown profile "sequential"`)
}

func TestRunSpecPairsEveryMountConfigurationWithACell(t *testing.T) {
	spec := testRunSpec(t, runOptions{smbEnabled: true})

	byMountOptions := make(map[string][]string)
	for _, c := range spec.cells {
		byMountOptions[c.mountOptions] = append(byMountOptions[c.mountOptions], c.name)
	}
	assert.Equal(t, []string{"file-line", "file-byte", "smb"}, byMountOptions[mountOptionsActimeo1])
	assert.Equal(t, []string{"file-line-actimeo30"}, byMountOptions[mountOptionsActimeo30])

	// The cells must differ only in the attribute cache lifetime, so a result
	// difference cannot come from any other mount option.
	assert.Equal(t,
		strings.Replace(mountOptionsActimeo1, "actimeo=1", "actimeo=30", 1),
		mountOptionsActimeo30)
}

func TestRunSpecSelectsIndividualCells(t *testing.T) {
	spec := testRunSpec(t, runOptions{cells: " file-line-actimeo30 , smb ", smbEnabled: true})
	require.Len(t, spec.cells, 2)
	assert.Equal(t, "file-line-actimeo30", spec.cells[0].name)
	assert.Equal(t, "smb", spec.cells[1].name)

	_, err := newRunSpec(runOptions{runID: testRunID, writerImage: testWriterImage, cells: "line"})
	assert.ErrorContains(t, err, `unknown cell "line"`)
}

func TestPostRotationMarkerDelaysStraddleEveryDrainWindow(t *testing.T) {
	spec := testRunSpec(t, runOptions{smbEnabled: true})
	for _, c := range spec.cells {
		assert.Equal(t, markerDelaysFor(c.reader, writerOptions{mode: renameRotation}), c.markers, c.name)
		assert.Equal(t, 2, c.markers.count(), c.name)
	}

	// A file source reads the rotated file for at least close_timeout after it
	// sees the rotation, which cannot come before the rename.
	file := markerDelaysFor(fileReader, writerOptions{mode: renameRotation})
	assert.Less(t, file.earlyMs, closeTimeoutSeconds*1000)

	// The longest file source drain at actimeo=1 is the unreliable-mount
	// handoff: it sees the rotation after a scan and an attribute cache
	// refresh, reads the surviving marker, then waits out its quiet period.
	longestFileDrainMs := (fileScanPeriodSeconds+1)*1000 + file.earlyMs + fileHandoffQuietSeconds*1000
	assert.Greater(t, file.lateMs, longestFileDrainMs)
	assert.Greater(t, file.lateMs, (fileScanPeriodSeconds+1+closeTimeoutSeconds)*1000)

	// The SMB source can drop an idle rotated file at the first poll after the
	// scan that saw the rotation, and that scan can come right after the
	// rename. The appender notices the rename up to one of its polls late, so
	// the surviving marker must land within a poll interval even then.
	smb := markerDelaysFor(smbReader, writerOptions{mode: renameRotation})
	assert.Less(t, smb.earlyMs+postRotationMarkerPollMs, (smbDrainIdlePolls-1)*smbPollIntervalSeconds*1000)
	// It sees the rotation within a poll of the rename and stops at
	// close_timeout at the latest, however often new data restarts its idle
	// polls.
	assert.Greater(t, smb.lateMs, (smbPollIntervalSeconds+closeTimeoutSeconds)*1000)

	assert.Equal(t, "1500,45000", file.appenderValue())
	assert.Equal(t, "500,45000", smb.appenderValue())
}

func TestPostRotationMarkersFollowTheRotationMode(t *testing.T) {
	for _, reader := range []readerKind{fileReader, smbReader} {
		renamed := markerDelaysFor(reader, writerOptions{mode: renameRotation})

		// gzip deletes the rotated file after gzipDelayMs: the surviving
		// marker must land before, however late the appender notices the
		// rename, and the lost marker would only find the file gone.
		gzipped := markerDelaysFor(reader, writerOptions{mode: gzipRotation})
		assert.Equal(t, markerDelays{earlyMs: renamed.earlyMs, earlyExpect: markerCollected}, gzipped, reader)
		assert.Equal(t, strconv.Itoa(renamed.earlyMs), gzipped.appenderValue(), reader)
		assert.Less(t, gzipped.earlyMs+postRotationMarkerPollMs, gzipDelayMs, reader)

		// copytruncate and delete-recreate leave no renamed file.
		for _, mode := range []rotationMode{copyTruncateRotation, deleteRecreateRotation} {
			none := markerDelaysFor(reader, writerOptions{mode: mode})
			assert.Zero(t, none.count(), "%s %s", reader, mode)
			assert.Equal(t, "none", none.appenderValue(), "%s %s", reader, mode)
		}
	}

	spec := testRunSpec(t, runOptions{rotationMode: "copytruncate", smbEnabled: true})
	for _, c := range spec.cells {
		assert.Zero(t, c.markers.count(), c.name)
		assert.Contains(t, spec.appenderEnv(c), envVar{"LOGWRITER_APPEND_DELAYS_MS", "none"}, c.name)
	}
}

func TestWriterOptionsDefaultToTheJavaWriter(t *testing.T) {
	spec := testRunSpec(t, runOptions{})
	assert.Equal(t, defaultWriterOptions(), spec.writer)
	assert.True(t, spec.writer.isDefault())
	assert.Empty(t, spec.writer.writerEnv())
	assert.Equal(t, []string{""}, spec.writer.streamDirs())
	assert.Equal(t, []string{"/mnt/azure-files/ledger.jsonl"}, spec.writer.sharePaths(ledgerName))
	assert.Equal(t, []string{spec.writerRunID(spec.cells[0])}, spec.streamRunIDs(spec.cells[0]))
	assert.Equal(t, 5000, spec.writer.headPauseMs())
	assert.Zero(t, spec.writer.retainedRotations())

	// The Java writer image knows none of the options.
	for name, opts := range map[string]runOptions{
		"mode":    {rotationMode: "gzip"},
		"period":  {periodMs: "10000"},
		"rate":    {rateBytesPerSec: "1000000"},
		"streams": {streams: "4"},
	} {
		opts.runID, opts.writerImage = testRunID, testWriterImage
		_, err := newRunSpec(opts)
		assert.ErrorContains(t, err, "Java writer image", name)
	}
	// Spelled-out defaults are still the defaults.
	java := testRunSpec(t, runOptions{writerImage: testWriterImage, rotationMode: "rename", periodMs: "60000", rateBytesPerSec: "0", streams: "0"})
	assert.True(t, java.writer.isDefault())
}

func TestWriterOptionsAreValidated(t *testing.T) {
	for name, opts := range map[string]runOptions{
		`unknown rotation mode "logrotate"`: {rotationMode: "logrotate"},
		"writer period 4000":                {periodMs: "4000"},
		"writer period 10500ms":             {periodMs: "10500"},
		"writer period 700000":              {periodMs: "700000"},
		`writer rate "fast"`:                {rateBytesPerSec: "fast"},
		"writer rate -1":                    {rateBytesPerSec: "-1"},
		"writer rate 30000000":              {rateBytesPerSec: "30000000"},
		"writer streams 33":                 {streams: "33"},
		// Six 10-minute files at 5 MB/s do not fit in the share.
		"keeps 24000 MB on the share": {rateBytesPerSec: "5000000", periodMs: "600000"},
	} {
		opts.runID = testRunID
		_, err := newRunSpec(opts)
		assert.ErrorContains(t, err, name)
	}
}

func TestWriterEnvCarriesTheOptions(t *testing.T) {
	spec := testRunSpec(t, runOptions{
		cells: "smb", smbEnabled: true,
		rotationMode: "gzip", periodMs: "10000", rateBytesPerSec: "5000000", streams: "10",
	})
	c := spec.cells[0]
	assert.Equal(t, []envVar{
		{"LOGWRITER_LOG_DIR", "/mnt/azure-files"},
		{"LOGWRITER_RUN_ID", testRunID + "-smb"},
		{"LOGWRITER_TARGET_BYTES_SEQUENCE", writerTargetSequence},
		// A sixth of the period, like the first-period runway.
		{"LOGWRITER_HEAD_PAUSE_MS", "1666"},
		{"LOGWRITER_MAX_RECORDS_PER_PERIOD", "5000"},
		{"TZ", "UTC"},
		{"LOGWRITER_ROTATION_MODE", "gzip"},
		{"LOGWRITER_GZIP_DELAY_MS", "5000"},
		{"LOGWRITER_PERIOD_MS", "10000"},
		{"LOGWRITER_INITIAL_FILL_RUNWAY_MS", "1666"},
		{"LOGWRITER_RATE_BYTES_PER_SEC", "5000000"},
		{"LOGWRITER_BUFFER_BYTES", "65536"},
		{"LOGWRITER_PAYLOAD_BYTES", "1024"},
		{"LOGWRITER_CONSOLE_RECORDS", "false"},
		// Two minutes of 10s periods.
		{"LOGWRITER_MAX_ROTATED_FILES", "12"},
		// Then it idles, so a kept stack stops writing.
		{"LOGWRITER_MAX_PERIODS", "7"},
		{"LOGWRITER_STREAMS", "10"},
	}, spec.writerEnv(c))

	runtime := spec.workloadRuntime("registry-1.docker.io")
	assert.Equal(t, []envVar{
		{"LOGWRITER_LOG_DIR", logMountPath},
		{"LOGWRITER_CRC64_COMMAND", "python3 /app/logwriter.py crc64"},
		{"LOGWRITER_LEDGER_SOURCE", "journal"},
		{"LOGWRITER_STREAMS", "10"},
	}, spec.ledgerEnv(c, runtime))
	assert.Contains(t, spec.appenderEnv(c), envVar{"LOGWRITER_STREAMS", "10"})
	assert.Contains(t, spec.appenderEnv(c), envVar{"LOGWRITER_APPEND_DELAYS_MS", "500"})

	streams := spec.streamRunIDs(c)
	require.Len(t, streams, 10)
	assert.Equal(t, testRunID+"-smb-svc-1", streams[0])
	assert.Equal(t, testRunID+"-smb-svc-10", streams[9])
	for _, id := range streams {
		// logwriter.py refuses a longer stream run ID.
		assert.Regexp(t, `^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$`, id)
	}
	// The longest cell name with the most streams still fits.
	longest := testRunSpec(t, runOptions{cells: "file-line-actimeo30", streams: strconv.Itoa(maxWriterStreams)})
	for _, id := range longest.streamRunIDs(longest.cells[0]) {
		assert.Regexp(t, `^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$`, id)
	}
	assert.Equal(t, "/mnt/azure-files/svc-3/markers.jsonl", spec.writer.sharePaths(postRotationMarkerJournalName)[2])

	// The writer is ready once its first stream has a fingerprintable head.
	containers := writerContainers(t, spec.writerPodSpec(c, runtime, false))
	assert.Equal(t, spec.writerEnv(c), containers[0].env)
	probe := spec.writerPodSpec(c, runtime, false).Containers.(corev1.ContainerArray)[0].(corev1.ContainerArgs).ReadinessProbe.(corev1.ProbeArgs)
	command := probe.Exec.(corev1.ExecActionArgs).Command.(pulumi.StringArray)
	assert.Equal(t, pulumi.String("test $(wc -c < /mnt/azure-files/svc-1/app.log) -ge 4096"), command[2])
}

func TestStreamsShareOneSourcePerCell(t *testing.T) {
	spec := testRunSpec(t, runOptions{cells: "file-line,smb", smbEnabled: true, streams: "3"})
	values := spec.agentHelmValues()
	assert.Contains(t, values, "        path: /mnt/azure-files/file-line/*/app.log\n        exclude_paths:\n          - /mnt/azure-files/file-line/*/app.log.*\n")
	// Unquoted, YAML would read the pattern as an alias.
	assert.Contains(t, values, "      - type: smb\n        path: \"*/app.log\"\n")
	assert.Len(t, regexp.MustCompile(`(?m)^\s+- type: (file|smb)$`).FindAllStringIndex(values, -1), 2)

	// One rotation period that is shorter does not change the sources.
	short := testRunSpec(t, runOptions{cells: "file-line,smb", smbEnabled: true, periodMs: "10000"})
	assert.Contains(t, short.agentHelmValues(), "        path: /mnt/azure-files/file-line/app.log\n")
	assert.Contains(t, short.agentHelmValues(), "      - type: smb\n        path: app.log\n")
}

func TestPacedWritersKeepEnoughRotatedFiles(t *testing.T) {
	// The documented high-rate runs fit in the share.
	for _, opts := range []runOptions{
		{rateBytesPerSec: "5000000", periodMs: "10000", streams: "10"},
		{rateBytesPerSec: "5000000"},
		{rateBytesPerSec: strconv.Itoa(maxWriterRateBytesPerSec), periodMs: "20000"},
	} {
		opts.runID = testRunID
		_, err := newRunSpec(opts)
		assert.NoError(t, err, "%+v", opts)
	}
	for _, period := range []int{minWriterPeriodMs, 10000, defaultWriterPeriodMs, maxWriterPeriodMs} {
		w := writerOptions{mode: renameRotation, periodMs: period, rateBytesPerSec: 5000000}
		kept := w.retainedRotations()
		assert.GreaterOrEqual(t, kept, minRetainedRotations, period)
		// The retention never deletes a rotated file before its lost marker,
		// or while any reader may still drain it.
		assert.GreaterOrEqual(t, kept*period, retainedRotationsMs, period)
		assert.Greater(t, kept*period, postRotationMarkerLostDelayMs, period)
	}
	// The ledger waits long enough for the slowest period.
	assert.Equal(t, 6*time.Minute, ledgerTimeout(defaultWriterOptions()))
	assert.Equal(t, 60*time.Minute, ledgerTimeout(writerOptions{periodMs: maxWriterPeriodMs}))
}

func TestCopyTruncateHoldOutlastsEveryReaderPoll(t *testing.T) {
	// A record written before the copy started stays in app.log for at least
	// the hold, which must give a reader that keeps up a scan or a poll to
	// read it; only what is written during the copy and the hold is at risk.
	assert.Greater(t, copyTruncateHoldMs, 2*fileScanPeriodSeconds*1000)
	assert.Greater(t, copyTruncateHoldMs, 2*smbPollIntervalSeconds*1000)
	// The delete-recreate pause and the gzip delay are shorter than any
	// period, so a rotation is over before the next one.
	assert.Less(t, deleteRecreatePauseMs, minWriterPeriodMs)
	assert.Less(t, gzipDelayMs+copyTruncateHoldMs, minWriterPeriodMs*2)
}

func TestStockWorkloadRunsTheConfigMapScriptsOnAPinnedPythonImage(t *testing.T) {
	spec := testRunSpec(t, runOptions{cells: "file-line"})
	c := spec.cells[0]
	assert.Equal(t, stockWorkload, spec.workloadKind())
	runtime := spec.workloadRuntime("registry-1.docker.io")
	assert.Equal(t, "registry-1.docker.io/"+stockWorkloadImage, runtime.image)
	// An exact Python release, pinned to the digest of its multi-arch index.
	assert.Regexp(t, `^library/python:3\.12\.[0-9]+-slim-[a-z]+@sha256:[0-9a-f]{64}$`, stockWorkloadImage)

	require.Len(t, runtime.scripts, 3)
	assert.Contains(t, runtime.scripts[pythonWriterScript], "def crc64_main(")
	assert.Contains(t, runtime.scripts[ledgerScript], "LOGWRITER_CRC64_COMMAND")
	assert.Contains(t, runtime.scripts[appenderScript], "post_rotation_marker")

	podSpec := spec.writerPodSpec(c, runtime, false)
	containers := writerContainers(t, podSpec)
	require.Len(t, containers, 3)
	writer, ledger, appender := containers[0], containers[1], containers[2]
	assert.Equal(t, []string{"python3", "/app/logwriter.py"}, writer.command)
	assert.Equal(t, []string{"/bin/sh", "/app/ledger.sh"}, ledger.command)
	assert.Equal(t, []string{"/bin/sh", "/app/appender.sh"}, appender.command)
	for _, container := range containers {
		assert.Equal(t, runtime.image, container.image, container.name)
		assert.Equal(t, map[string]writerMount{
			logMountPath: {volume: c.volumeName},
			workloadDir:  {volume: workloadVolumeName, readOnly: true},
		}, container.mounts, container.name)
		// Each command runs a script of the ConfigMap.
		script := container.command[len(container.command)-1]
		assert.Contains(t, runtime.scripts, strings.TrimPrefix(script, workloadDir+"/"), container.name)
	}
	// The Python writer reads the Java writer's configuration; it has no JVM.
	assert.Equal(t, spec.writerEnv(c), writer.env)
	// The ledger reads the Python writer's journal.
	assert.Equal(t, []envVar{
		{"LOGWRITER_LOG_DIR", logMountPath},
		{"LOGWRITER_CRC64_COMMAND", "python3 /app/logwriter.py crc64"},
		{"LOGWRITER_LEDGER_SOURCE", "journal"},
	}, ledger.env)

	volumes := podVolumes(t, podSpec)
	require.Len(t, volumes, 2)
	assert.Equal(t, workloadVolumeName, pulumiString(t, volumes[1].Name))
	configMap, ok := volumes[1].ConfigMap.(corev1.ConfigMapVolumeSourceArgs)
	require.True(t, ok)
	assert.Equal(t, workloadConfigMapName, pulumiString(t, configMap.Name))
	assert.Equal(t, pulumi.Int(0o555), configMap.DefaultMode)

	securityContext, ok := podSpec.SecurityContext.(corev1.PodSecurityContextArgs)
	require.True(t, ok)
	assert.Equal(t, pulumi.Bool(true), securityContext.RunAsNonRoot)
	assert.Equal(t, pulumi.Int(stockWorkloadUser), securityContext.RunAsUser)
	assert.Nil(t, podSpec.ImagePullSecrets)
}

func TestWriterImageKeepsTheJavaWorkload(t *testing.T) {
	spec := testRunSpec(t, runOptions{cells: "file-line", writerImage: testWriterImage})
	c := spec.cells[0]
	assert.Equal(t, customWorkload, spec.workloadKind())
	runtime := spec.workloadRuntime("registry-1.docker.io")
	assert.Equal(t, testWriterImage, runtime.image)
	assert.Empty(t, runtime.scripts)

	podSpec := spec.writerPodSpec(c, runtime, true)
	containers := writerContainers(t, podSpec)
	require.Len(t, containers, 3)
	// The image entrypoint runs the Java writer, and the scripts are the
	// image's own copies.
	assert.Nil(t, containers[0].command)
	assert.Equal(t, []string{"/bin/sh", "/app/ledger.sh"}, containers[1].command)
	assert.Equal(t, []string{"/bin/sh", "/app/appender.sh"}, containers[2].command)
	for _, container := range containers {
		assert.Equal(t, testWriterImage, container.image, container.name)
		assert.Equal(t, map[string]writerMount{logMountPath: {volume: c.volumeName}}, container.mounts, container.name)
	}
	assert.Equal(t, append(spec.writerEnv(c), envVar{"JAVA_TOOL_OPTIONS", "-Duser.timezone=UTC"}), containers[0].env)
	// Without LOGWRITER_CRC64_COMMAND, ledger.sh runs the image's Crc64 class.
	assert.Equal(t, []envVar{{"LOGWRITER_LOG_DIR", logMountPath}}, containers[1].env)
	assert.Contains(t, ledgerSource, "${LOGWRITER_CRC64_COMMAND:-java -cp /app/classes com.datadoghq.e2e.logwriter.Crc64}")

	assert.Len(t, podVolumes(t, podSpec), 1)
	assert.Nil(t, podSpec.SecurityContext)
	assert.NotNil(t, podSpec.ImagePullSecrets)
}

func TestWriterEnvIsTheJavaWriterConfiguration(t *testing.T) {
	spec := testRunSpec(t, runOptions{smbEnabled: true})
	assert.Equal(t, []envVar{
		{"LOGWRITER_LOG_DIR", "/mnt/azure-files"},
		{"LOGWRITER_RUN_ID", testRunID + "-file-line"},
		{"LOGWRITER_TARGET_BYTES_SEQUENCE", "81792,82885,84060,81920,85000,82500"},
		{"LOGWRITER_HEAD_PAUSE_MS", "5000"},
		{"LOGWRITER_MAX_RECORDS_PER_PERIOD", "5000"},
		{"TZ", "UTC"},
	}, spec.writerEnv(spec.cells[0]))
	// Both writers refuse any other run ID.
	for _, c := range spec.cells {
		assert.Regexp(t, `^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$`, spec.writerRunID(c), c.name)
	}
}

func TestWorkloadChecksumFollowsTheScripts(t *testing.T) {
	runtime := testRunSpec(t, runOptions{}).workloadRuntime("registry-1.docker.io")
	checksum := runtime.scriptsChecksum()
	assert.Regexp(t, `^[0-9a-f]{64}$`, checksum)
	// The checksum depends on the scripts only.
	other := testRunSpec(t, runOptions{runID: "20260826t130000z-654321"}).workloadRuntime("mirror.example")
	assert.Equal(t, checksum, other.scriptsChecksum())

	changed := workloadRuntime{scripts: map[string]string{}}
	for name, script := range runtime.scripts {
		changed.scripts[name] = script
	}
	changed.scripts[ledgerScript] += "\n"
	assert.NotEqual(t, checksum, changed.scriptsChecksum())
}

func TestRunMetadataNamesTheWriterWorkload(t *testing.T) {
	stock := testRunSpec(t, runOptions{}).writerMetadata()
	assert.Equal(t, stockWorkload, stock["workload"])
	assert.Equal(t, stockWorkloadImage, stock["image"])
	assert.Equal(t, workloadConfigMapName, stock["config_map"])

	custom := testRunSpec(t, runOptions{writerImage: testWriterImage}).writerMetadata()
	assert.Equal(t, map[string]any{"workload": customWorkload, "image": testWriterImage, "options": defaultWriterOptions().metadata()}, custom)

	paced := testRunSpec(t, runOptions{rotationMode: "copytruncate", rateBytesPerSec: "1000000", streams: "2"}).writerMetadata()
	options, ok := paced["options"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, copyTruncateRotation, options["rotation_mode"])
	assert.Equal(t, copyTruncateHoldMs, options["copytruncate_hold_ms"])
	assert.Equal(t, 2, options["streams"])
	assert.Equal(t, "*/app.log", options["active_log_pattern"])
	assert.Equal(t, pacedWriterBufferBytes, options["buffer_bytes"])
}

func TestExpectedRecordsUseLedgerRangesWithoutUnwrittenOnes(t *testing.T) {
	expected := expectedRecords([]ledgerEntry{
		{RunID: "a", FirstSequence: 1, LastSequence: 3},
		{RunID: "a", FirstSequence: 7, LastSequence: 10, UnwrittenSequences: [][2]int64{{8, 9}}},
		// Sequences start over in each stream.
		{RunID: "b", FirstSequence: 1, LastSequence: 2},
	})
	assert.Equal(t, map[recordKey]struct{}{
		{"a", 1}: {}, {"a", 2}: {}, {"a", 3}: {}, {"a", 7}: {}, {"a", 10}: {},
		{"b", 1}: {}, {"b", 2}: {},
	}, expected)
	assert.Equal(t, int64(2), unwrittenRecords([]ledgerEntry{{UnwrittenSequences: [][2]int64{{8, 9}}}}))
}

func TestCountRecordsOnlyCountsExpectedRecords(t *testing.T) {
	counts := countRecords([]string{
		"run_id=r period=p sequence=1 record=1 phase=head",
		"run_id=r period=p sequence=1 record=1 phase=head",
		"run_id=other period=p sequence=1 record=1 phase=head",
		"run_id=r period=p sequence=9 record=2 phase=fill",
		// A line cut inside its sequence number is no record.
		"run_id=r period=p sequence=2",
		"post_rotation_marker run_id=r rotation=1 marker_id=r-r1-m1500",
	}, map[recordKey]struct{}{{"r", 1}: {}, {"r", 2}: {}})
	assert.Equal(t, map[recordKey]int{{"r", 1}: 2}, counts)
}

func TestAssertedLedgerTakesTheFirstFilesOfEveryStream(t *testing.T) {
	ledger := []ledgerEntry{
		{RunID: "s1", Period: "20260813T1203Z", FirstSequence: 7, LastSequence: 8},
		{RunID: "s1", Period: "20260813T1200Z", FirstSequence: 1, LastSequence: 2, AtRiskSequences: [][2]int64{{3, 4}}},
		{RunID: "s1", Period: "20260813T1201Z", FirstSequence: 3, LastSequence: 4},
		{RunID: "s1", Period: "20260813T1202Z", FirstSequence: 5, LastSequence: 6, AtRiskSequences: [][2]int64{{7, 7}}},
		{RunID: "s2", Period: "20260813T1200Z", FirstSequence: 1, LastSequence: 1},
		{RunID: "other", Period: "20260813T1200Z", FirstSequence: 1, LastSequence: 1, AtRiskSequences: [][2]int64{{2, 2}}},
	}
	asserted, atRisk := assertedLedger(ledger, []string{"s1", "s2"})
	var periods []string
	for _, entry := range asserted {
		periods = append(periods, entry.RunID+"/"+entry.Period)
	}
	assert.Equal(t, []string{"s1/20260813T1200Z", "s1/20260813T1201Z", "s1/20260813T1202Z", "s2/20260813T1200Z"}, periods)
	// Every rotation of the streams counts, including one past the asserted
	// files, and no other stream does.
	assert.Equal(t, map[recordKey]struct{}{{"s1", 3}: {}, {"s1", 4}: {}, {"s1", 7}: {}}, atRisk)
}

func TestCheckRecordsAllowsOnlyAtRiskLosses(t *testing.T) {
	c := cell{name: "smb"}
	expected := map[recordKey]struct{}{{"s", 1}: {}, {"s", 2}: {}, {"s", 3}: {}, {"s", 4}: {}, {"s", 5}: {}}
	atRisk := map[recordKey]struct{}{{"s", 3}: {}, {"s", 4}: {}}

	allowed := checkRecords(expected, atRisk, map[recordKey]int{{"s", 1}: 1, {"s", 2}: 1, {"s", 4}: 1, {"s", 5}: 1})
	assert.Equal(t, []recordKey{{"s", 3}}, allowed.atRiskLost)
	assert.Equal(t, 2, allowed.atRisk)
	passed := new(recordingT)
	assertRecordsCollected(passed, c, allowed)
	assert.Empty(t, passed.failures)

	real := checkRecords(expected, atRisk, map[recordKey]int{{"s", 3}: 2, {"s", 5}: 1})
	assert.Equal(t, []recordKey{{"s", 1}, {"s", 2}}, real.missing)
	// An at-risk record may be lost, but never duplicated.
	assert.Equal(t, []recordKey{{"s", 3}}, real.duplicated)
	failed := new(recordingT)
	assertRecordsCollected(failed, c, real)
	require.Len(t, failed.failures, 2)
	assert.Contains(t, failed.failures[0], "2 of 5 expected records were never collected although no rotation put them at risk: s:1-2")
	assert.Contains(t, failed.failures[1], "1 of 5 expected records were collected more than once: s:3")
}

func TestFormatRecordRangesSummarizes(t *testing.T) {
	assert.Equal(t, "none", formatRecordRanges(nil))
	assert.Equal(t, "a:1-3, a:5, b:1", formatRecordRanges([]recordKey{{"a", 1}, {"a", 2}, {"a", 3}, {"a", 5}, {"b", 1}}))
	var many []recordKey
	for i := int64(0); i < 30; i++ {
		many = append(many, recordKey{"a", i * 2})
	}
	summary := formatRecordRanges(many)
	assert.True(t, strings.HasSuffix(summary, " and 20 more ranges"), summary)
}

func TestCountMarkerIDsIgnoresOrdinaryRecords(t *testing.T) {
	counts := countMarkerIDs([]string{
		"post_rotation_marker run_id=r rotation=1 marker_age_ms=1500 marker_id=r-r1-m1500 rotated_file=app.log.1",
		"post_rotation_marker run_id=r rotation=1 marker_age_ms=1500 marker_id=r-r1-m1500 rotated_file=app.log.1",
		"run_id=r period=p sequence=4 phase=fill",
	})
	assert.Equal(t, map[string]int{"r-r1-m1500": 2}, counts)
}

func TestMarkersForFilesKeepsOnlyTheseStreamsAndRotations(t *testing.T) {
	journal := []markerEntry{
		{RunID: "run-file-line", MarkerID: "keep", RotatedFile: "app.log.a", Status: "appended"},
		{RunID: "run-file-line", MarkerID: "other-run-file", RotatedFile: "app.log.z", Status: "appended"},
		{RunID: "older-run", MarkerID: "older-run", RotatedFile: "app.log.a", Status: "appended"},
		{RunID: "run-file-line", MarkerID: "failed-append", RotatedFile: "app.log.a", Status: "failed"},
		{RunID: "run-file-line", MarkerID: "file-gone", RotatedFile: "app.log.a", Status: "skipped"},
		// Every stream rotates a file of the same name at the same time.
		{RunID: "run-file-line-svc-2", MarkerID: "keep-stream", RotatedFile: "app.log.a", Status: "appended"},
		{RunID: "run-file-line-svc-3", MarkerID: "other-stream", RotatedFile: "app.log.a", Status: "appended"},
	}
	markers := markersForFiles(journal, map[markerKey]struct{}{
		{runID: "run-file-line", file: "app.log.a"}:       {},
		{runID: "run-file-line-svc-2", file: "app.log.a"}: {},
	})
	require.Len(t, markers, 2)
	assert.Equal(t, "keep", markers[0].MarkerID)
	assert.Equal(t, "keep-stream", markers[1].MarkerID)
}

func TestAssertMarkerOutcomeRequiresTheEarlyMarkerAndForbidsTheLateOne(t *testing.T) {
	for _, reader := range []readerKind{fileReader, smbReader} {
		c := cell{name: string(reader), reader: reader, markers: markerDelaysFor(reader, writerOptions{mode: renameRotation})}
		markers := []markerEntry{
			{MarkerID: "early", MarkerAgeMs: c.markers.earlyMs, RotatedFile: "app.log.a"},
			{MarkerID: "late", MarkerAgeMs: c.markers.lateMs, RotatedFile: "app.log.a"},
		}

		calibrated := new(recordingT)
		assertMarkerOutcome(calibrated, c, markers, map[string]int{"early": 1})
		assert.Empty(t, calibrated.failures, reader)

		lostEarlyMarker := new(recordingT)
		assertMarkerOutcome(lostEarlyMarker, c, markers, map[string]int{})
		assert.Len(t, lostEarlyMarker.failures, 1, reader)

		uncalibrated := new(recordingT)
		assertMarkerOutcome(uncalibrated, c, markers, map[string]int{"early": 1, "late": 1})
		assert.Len(t, uncalibrated.failures, 1, reader)
	}

	// The SMB cell's appender writes no 1.5s marker; one in its journal is
	// not one of its assertions.
	smb := cell{name: "smb", reader: smbReader, markers: markerDelaysFor(smbReader, writerOptions{mode: renameRotation})}
	other := new(recordingT)
	assertMarkerOutcome(other, smb, []markerEntry{{MarkerID: "other", MarkerAgeMs: fileSurvivingMarkerDelayMs}}, map[string]int{})
	assert.Empty(t, other.failures)
}

func TestAssertLogOriginRequiresTheSourceMetadata(t *testing.T) {
	c := cell{name: "smb"}
	good := collectedLog{source: "java", tags: []string{"kube_namespace:x", "e2e_run_id:run", "e2e_cell:smb"}}

	ok := new(recordingT)
	assertLogOrigin(ok, "run", c, []collectedLog{good, good})
	assert.Empty(t, ok.failures)

	for name, bad := range map[string]collectedLog{
		"wrong source":    {source: "smb", tags: good.tags},
		"earlier run":     {source: "java", tags: []string{"e2e_run_id:older", "e2e_cell:smb"}},
		"no source tags":  {source: "java", tags: []string{"kube_namespace:x"}},
		"other cell tags": {source: "java", tags: []string{"e2e_run_id:run", "e2e_cell:file-line"}},
	} {
		failed := new(recordingT)
		assertLogOrigin(failed, "run", c, []collectedLog{good, bad, bad})
		assert.Len(t, failed.failures, 1, name)
	}
}

func TestFirstStorageAccountKey(t *testing.T) {
	key, err := firstStorageAccountKey([]any{map[string]any{"value": "secret-value"}})
	require.NoError(t, err)
	assert.Equal(t, "secret-value", key)

	_, err = firstStorageAccountKey(nil)
	assert.Error(t, err)
}

func TestEvidenceRedactsAccountKeys(t *testing.T) {
	evidence := &evidenceDir{dir: t.TempDir(), secrets: []string{"key-one", ""}}
	assert.Equal(t,
		"password=[redacted storage account key] other=[redacted storage account key]",
		string(evidence.redact([]byte("password=key-one other=key-one"))))
	assert.Equal(t, "stderr: [redacted storage account key]", redactSecrets("stderr: key-one", "key-one"))
}

func TestFindLogSourceMatchesTypeAndService(t *testing.T) {
	statusJSON := `warning before the JSON
2026-10-06 18:29:07 UTC | CORE | DEBUG | (pkg/config/setup/config.go:233) | Set('proxy.no_proxy'): converting value from []interface {} to []string to match default type
{"logsStats": {"integrations": [
  {"name": "azure_files", "sources": [
    {"type": "file", "configuration": {"Service": "azure-files-smb", "Path": "/mnt/azure-files/file-line/app.log"}, "status": "OK", "info": {"Bytes Read": ["1"]}},
    {"type": "smb", "configuration": {"Service": "azure-files-file-line"}, "status": "OK", "info": {}},
    {"type": "smb", "configuration": {"Service": "azure-files-smb", "Host": "acct.file.core.windows.net"}, "status": "OK",
     "inputs": ["smb://acct.file.core.windows.net/share/app.log"], "info": {"Bytes Read": ["4096"]}}
  ]}
],
"tailers": [
  {"id": "/mnt/azure-files/file-line/app.log", "type": "file", "info": {"Bytes Read": ["1"]}},
  {"id": "smb://acct.file.core.windows.net/share/app.log", "type": "smb", "info": {"Bytes Read": ["4000"]}},
  {"id": "smb://acct.file.core.windows.net/share/app.log", "type": "smb", "info": {"Bytes Read": ["96"], "Draining Since": ["2026-10-05 12:00:00 UTC"]}},
  {"id": "smb://acct.file.core.windows.net/share2/app.log", "type": "smb", "info": {"Bytes Read": ["7"]}}
]}}
`
	source, found, err := findLogSource(statusJSON, "smb", "azure-files-smb")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "OK", source.Status)
	assert.Equal(t, []string{"4096"}, source.Info[bytesReadInfoKey])
	assert.Equal(t, []string{"smb://acct.file.core.windows.net/share/app.log"}, source.Inputs)

	status, err := decodeAgentStatus(statusJSON)
	require.NoError(t, err)
	tailers := status.smbTailers(cell{accountName: "acct", shareName: "share"})
	require.Len(t, tailers, 2)
	assert.Equal(t, []string{"4000"}, tailers[0].Info[bytesReadInfoKey])
	assert.Contains(t, tailers[1].Info, "Draining Since")

	_, found, err = findLogSource(statusJSON, "smb", "azure-files-other")
	require.NoError(t, err)
	assert.False(t, found)

	_, _, err = findLogSource("Error: unable to reach the Agent", "smb", "azure-files-smb")
	assert.Error(t, err)
}

func TestSecretHandleResolvedReadsTheSecretInfo(t *testing.T) {
	handle := smbPasswordHandle(cell{name: "smb"})
	resolved := `=== Secrets stats ===
Number of secrets resolved: 1
Secrets handle resolved:

- 'file@/etc/azure-files-secrets/smb/azurestorageaccountkey':
  - used in 'azure_files' configuration in entry 'logs/0/smb/password'
`
	assert.True(t, secretHandleResolved(resolved, handle))
	assert.False(t, secretHandleResolved(resolved, smbPasswordHandle(cell{name: "other"})))

	unresolved := `=== Secrets stats ===
Number of secrets resolved: 0
Secrets handle resolved:

Secrets not resolved:
  - file@/etc/azure-files-secrets/smb/azurestorageaccountkey
`
	assert.False(t, secretHandleResolved(unresolved, handle))
	assert.False(t, secretHandleResolved("No secret_backend_command set: secrets feature is not enabled", handle))
}

func TestFlareArchivePathIsReadFromTheOutput(t *testing.T) {
	path, err := flareArchivePath("Asking the agent to build the flare archive.\n" +
		"/tmp/datadog-agent-2026-10-05-12-00-00.zip is going to be uploaded to Datadog\n" +
		"Aborting. (You can still use /tmp/datadog-agent-2026-10-05-12-00-00.zip)\n")
	require.NoError(t, err)
	assert.Equal(t, "/tmp/datadog-agent-2026-10-05-12-00-00.zip", path)

	_, err = flareArchivePath("Error: unable to contact the Agent")
	assert.Error(t, err)
}

func TestFlareArchiveFindsSecretsAndMissedBytes(t *testing.T) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range map[string]string{
		"host/logs/agent.log":       "connected to share with password key-one",
		"host/config-check.log":     "password: ********",
		"host/expvar/logs-agent":    "BytesMissed: 1.234567e+06\nBytesSent: 10\n",
		"host/expvar/logs-agent-ok": "BytesMissed: 3\n",
		"host/etc/confd/a.yaml":     "password: \"ENC[file@/etc/azure-files-secrets/smb/azurestorageaccountkey]\"",
		"host/secrets.log":          "handle key-one used by azure_files",
	} {
		entry, err := writer.Create(name)
		require.NoError(t, err)
		_, err = entry.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())

	archive, err := readFlareArchive(buffer.Bytes())
	require.NoError(t, err)
	assert.Equal(t, []string{"host/logs/agent.log", "host/secrets.log"}, archive.entriesContaining("key-one"))
	assert.Empty(t, archive.entriesContaining("key-two"))

	missed, ok := archive.logsAgentBytesMissed()
	require.True(t, ok)
	assert.Equal(t, int64(1234567), missed)

	_, ok = flareArchive{"host/expvar/forwarder": []byte("BytesMissed: 1\n")}.logsAgentBytesMissed()
	assert.False(t, ok)

	_, err = readFlareArchive([]byte("not a zip"))
	assert.Error(t, err)
}

// writerContainer is a container of a writer pod spec, read back from its
// Pulumi arguments.
type writerContainer struct {
	name    string
	image   string
	command []string
	env     []envVar
	// mounts are keyed by mount path.
	mounts map[string]writerMount
}

type writerMount struct {
	volume   string
	readOnly bool
}

func writerContainers(t *testing.T, podSpec *corev1.PodSpecArgs) []writerContainer {
	t.Helper()
	inputs, ok := podSpec.Containers.(corev1.ContainerArray)
	require.True(t, ok, "containers are %T", podSpec.Containers)
	containers := make([]writerContainer, 0, len(inputs))
	for _, input := range inputs {
		args, ok := input.(corev1.ContainerArgs)
		require.True(t, ok, "container is %T", input)
		container := writerContainer{
			name:   pulumiString(t, args.Name),
			image:  pulumiString(t, args.Image),
			mounts: map[string]writerMount{},
		}
		if args.Command != nil {
			command, ok := args.Command.(pulumi.StringArray)
			require.True(t, ok, "command is %T", args.Command)
			for _, word := range command {
				container.command = append(container.command, pulumiString(t, word))
			}
		}
		env, ok := args.Env.(corev1.EnvVarArray)
		require.True(t, ok, "env is %T", args.Env)
		for _, input := range env {
			variable, ok := input.(corev1.EnvVarArgs)
			require.True(t, ok, "env var is %T", input)
			container.env = append(container.env, envVar{pulumiString(t, variable.Name), pulumiString(t, variable.Value)})
		}
		mounts, ok := args.VolumeMounts.(corev1.VolumeMountArray)
		require.True(t, ok, "volume mounts are %T", args.VolumeMounts)
		for _, input := range mounts {
			mount, ok := input.(corev1.VolumeMountArgs)
			require.True(t, ok, "volume mount is %T", input)
			readOnly := false
			if mount.ReadOnly != nil {
				readOnly = bool(mount.ReadOnly.(pulumi.Bool))
			}
			container.mounts[pulumiString(t, mount.MountPath)] = writerMount{volume: pulumiString(t, mount.Name), readOnly: readOnly}
		}
		containers = append(containers, container)
	}
	return containers
}

func podVolumes(t *testing.T, podSpec *corev1.PodSpecArgs) []corev1.VolumeArgs {
	t.Helper()
	inputs, ok := podSpec.Volumes.(corev1.VolumeArray)
	require.True(t, ok, "volumes are %T", podSpec.Volumes)
	volumes := make([]corev1.VolumeArgs, 0, len(inputs))
	for _, input := range inputs {
		volume, ok := input.(corev1.VolumeArgs)
		require.True(t, ok, "volume is %T", input)
		volumes = append(volumes, volume)
	}
	return volumes
}

func pulumiString(t *testing.T, input any) string {
	t.Helper()
	value, ok := input.(pulumi.String)
	require.True(t, ok, "%T is not a pulumi.String", input)
	return string(value)
}

// recordingT captures assertion failures instead of failing the test, so the
// marker calibration itself can be asserted on.
type recordingT struct {
	failures []string
}

func (t *recordingT) Errorf(format string, args ...any) {
	t.failures = append(t.failures, fmt.Sprintf(format, args...))
}

// newSMBCells are the opt-in SMB cells, each only provisioned when named.
var newSMBCells = []string{
	"smb-copytruncate", "smb-delete-recreate", "smb-gzip",
	"smb-late-1000", "smb-late-2000", "smb-late-3000",
	"smb-glob-load",
}

func TestDefaultMatrixKeepsItsFourCells(t *testing.T) {
	spec := testRunSpec(t, runOptions{smbEnabled: true})
	var names []string
	for _, c := range spec.cells {
		names = append(names, c.name)
	}
	assert.Equal(t, []string{"file-line", "file-byte", "file-line-actimeo30", "smb"}, names)

	// Every cell, named, has an account (but the Windows server's), a share
	// and a service of its own.
	all := allCells(spec.stackName, testRunID)
	accounts, shares, services := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, c := range all {
		if c.onWindows() {
			assert.Empty(t, c.accountName, c.name)
		} else {
			assert.Equal(t, azureFilesServer, c.server, c.name)
			assert.Regexp(t, `^[a-z0-9]{3,24}$`, c.accountName, c.name)
			assert.False(t, accounts[c.accountName], "%s reuses an account name", c.name)
			accounts[c.accountName] = true
		}
		assert.Regexp(t, `^[a-z0-9]([a-z0-9-]{1,61}[a-z0-9])$`, c.shareName, c.name)
		assert.LessOrEqual(t, len(c.volumeName+"-key"), 63, c.name)
		assert.False(t, shares[c.shareName], "%s reuses a share name", c.name)
		assert.False(t, services[c.service], "%s reuses a service", c.name)
		shares[c.shareName], services[c.service] = true, true
	}
	for _, name := range newSMBCells {
		found := false
		for _, c := range all {
			if c.name == name {
				found = true
				assert.Equal(t, smbReader, c.reader, name)
				assert.False(t, c.inDefaultMatrix, name)
			}
		}
		assert.True(t, found, name)
	}
}

func TestNewSMBCellsAreGatedLikeTheSMBCell(t *testing.T) {
	for _, name := range newSMBCells {
		gated := testRunSpec(t, runOptions{cells: name})
		assert.Empty(t, gated.cells, name)
		require.Len(t, gated.gatedCells, 1, name)
		assert.Equal(t, name, gated.gatedCells[0].name)

		enabled := testRunSpec(t, runOptions{cells: name, smbEnabled: true})
		require.Len(t, enabled.cells, 1, name)
		c := enabled.cells[0]
		values := enabled.agentHelmValues()
		assert.Contains(t, values, "      - type: smb\n", name)
		assert.Contains(t, values, "service: "+c.service+"\n", name)
		assert.Contains(t, values, "host: "+c.host()+"\n", name)
		assert.Contains(t, values, fmt.Sprintf("password: %q", smbPasswordHandle(c)), name)
		assert.Contains(t, values, "secretBackend:", name)
		assert.NotContains(t, values, "csi:", name)
	}
	// They can all run next to the default matrix in one run.
	spec := testRunSpec(t, runOptions{cells: "smb," + strings.Join(newSMBCells, ","), smbEnabled: true})
	assert.Len(t, spec.cells, len(newSMBCells)+1)
	assert.Len(t, regexp.MustCompile(`(?m)^\s+- type: smb$`).FindAllStringIndex(spec.agentHelmValues(), -1), len(newSMBCells)+1)
}

func TestModeCellsFixTheirRotationMode(t *testing.T) {
	spec := testRunSpec(t, runOptions{cells: "smb,smb-copytruncate,smb-delete-recreate,smb-gzip", smbEnabled: true})
	modes := map[string]rotationMode{}
	for _, c := range spec.cells {
		modes[c.name] = c.writer.mode
		assert.Equal(t, markerDelaysFor(smbReader, c.writer), c.markers, c.name)
		if c.writer.mode != renameRotation {
			assert.Contains(t, spec.writerEnv(c), envVar{"LOGWRITER_ROTATION_MODE", string(c.writer.mode)}, c.name)
		}
	}
	assert.Equal(t, map[string]rotationMode{
		"smb": renameRotation, "smb-copytruncate": copyTruncateRotation,
		"smb-delete-recreate": deleteRecreateRotation, "smb-gzip": gzipRotation,
	}, modes)

	// Only the modes that delete a rotated file account for losses with the
	// Agent's missed bytes; copytruncate has its at-risk records.
	for _, c := range spec.cells {
		assert.Equal(t, c.writer.mode == gzipRotation || c.writer.mode == deleteRecreateRotation, c.lossAccounted(), c.name)
	}
	assert.False(t, (cell{reader: fileReader, writer: writerOptions{mode: gzipRotation}}).lossAccounted())

	// The run's other options apply to the mode cells.
	paced := testRunSpec(t, runOptions{cells: "smb-gzip", smbEnabled: true, periodMs: "10000", rateBytesPerSec: "100000"})
	assert.Equal(t, writerOptions{mode: gzipRotation, periodMs: 10000, rateBytesPerSec: 100000}, paced.cells[0].writer)
	same := testRunSpec(t, runOptions{cells: "smb-gzip", smbEnabled: true, rotationMode: "gzip"})
	assert.Equal(t, gzipRotation, same.cells[0].writer.mode)

	_, err := newRunSpec(runOptions{runID: testRunID, cells: "smb-copytruncate", smbEnabled: true, rotationMode: "gzip"})
	assert.ErrorContains(t, err, "cell smb-copytruncate always rotates by copytruncate")
	_, err = newRunSpec(runOptions{runID: testRunID, cells: "smb-gzip", smbEnabled: true, writerImage: testWriterImage})
	assert.ErrorContains(t, err, "cell smb-gzip: the Java writer image")
}

func TestLateMarkerCellsProbeOneAgeEach(t *testing.T) {
	require.Len(t, lateMarkerProbes, 3)
	for _, probe := range lateMarkerProbes {
		name := lateMarkerCellName(probe)
		spec := testRunSpec(t, runOptions{cells: name, smbEnabled: true})
		require.Len(t, spec.cells, 1, name)
		c := spec.cells[0]
		assert.Equal(t, renameRotation, c.writer.mode, name)
		// The probe is the early marker; the 45s marker still calibrates.
		assert.Equal(t, markerDelays{earlyMs: probe.ageMs, earlyExpect: probe.expect, lateMs: postRotationMarkerLostDelayMs}, c.markers, name)
		assert.Equal(t, strconv.Itoa(probe.ageMs)+",45000", c.markers.appenderValue(), name)
		assert.Contains(t, spec.appenderEnv(c), envVar{"LOGWRITER_APPEND_DELAYS_MS", strconv.Itoa(probe.ageMs) + ",45000"}, name)
		assert.True(t, c.markers.needsSettle(), name)

		// The prediction follows the drain's end, one to two poll intervals
		// after the rename (see lateMarkerProbes): an age the drain cannot
		// have reached is collected, one past its latest end is lost, and
		// one in between may go either way.
		drainEndsAfterMs := (smbDrainIdlePolls - 1) * smbPollIntervalSeconds * 1000
		drainEndsByMs := smbDrainIdlePolls * smbPollIntervalSeconds * 1000
		switch probe.expect {
		case markerCollected:
			assert.Less(t, probe.ageMs+postRotationMarkerPollMs, drainEndsAfterMs, name)
		case markerLost:
			assert.GreaterOrEqual(t, probe.ageMs, drainEndsByMs, name)
		case markerEither:
			assert.GreaterOrEqual(t, probe.ageMs+postRotationMarkerPollMs, drainEndsAfterMs, name)
			assert.Less(t, probe.ageMs, drainEndsByMs, name)
		}

		_, err := newRunSpec(runOptions{runID: testRunID, cells: name, smbEnabled: true, rotationMode: "gzip"})
		assert.ErrorContains(t, err, "always rotates by rename", name)
	}
}

func TestMarkerExpectationsDriveTheAssertion(t *testing.T) {
	c := cell{name: "smb-late-1000", reader: smbReader, markers: markerDelays{earlyMs: 1000, earlyExpect: markerEither, lateMs: 45000}}
	markers := []markerEntry{
		{MarkerID: "r1-m1000", MarkerAgeMs: 1000, RotatedFile: "app.log.1"},
		{MarkerID: "r2-m1000", MarkerAgeMs: 1000, RotatedFile: "app.log.2"},
		{MarkerID: "r1-m45000", MarkerAgeMs: 45000, RotatedFile: "app.log.1"},
	}
	// Either outcome of the probe passes; twice does not, nor the late one.
	passed := new(recordingT)
	assertMarkerOutcome(passed, c, markers, map[string]int{"r1-m1000": 1})
	assert.Empty(t, passed.failures)
	failed := new(recordingT)
	assertMarkerOutcome(failed, c, markers, map[string]int{"r1-m1000": 2, "r1-m45000": 1})
	assert.Len(t, failed.failures, 2)

	lost := cell{name: "smb-late-2000", reader: smbReader, markers: markerDelays{earlyMs: 2000, earlyExpect: markerLost, lateMs: 45000}}
	survived := new(recordingT)
	assertMarkerOutcome(survived, lost, []markerEntry{{MarkerID: "m2000", MarkerAgeMs: 2000}}, map[string]int{"m2000": 1})
	require.Len(t, survived.failures, 1)
	assert.Contains(t, survived.failures[0], "lateMarkerProbes in provisioner.go predicts")
	assert.Contains(t, survived.failures[0], runCalibrate+"=1")

	// Calibration records the same outcomes instead.
	outcomes := markerOutcomes(c, markers, map[string]int{"r1-m1000": 1})
	require.Len(t, outcomes, 3)
	assert.Equal(t, markerOutcome{MarkerID: "r1-m1000", RotatedFile: "app.log.1", AgeMs: 1000, Expected: markerEither, Collected: 1, Outcome: markerCollected, Matches: true}, outcomes[0])
	assert.Equal(t, markerLost, outcomes[1].Outcome)
	assert.Equal(t, markerLost, outcomes[2].Expected)
	assert.True(t, outcomes[2].Matches)
	assert.Equal(t, "1000ms: collected 1/2, lost 1/2 (expected either); 45000ms: collected 0/1, lost 1/1 (expected lost)",
		summarizeMarkerOutcomes(outcomes))

	assert.False(t, markerDelays{earlyMs: 500, earlyExpect: markerCollected}.needsSettle())
	_, ok := c.markers.expectationFor(1500)
	assert.False(t, ok)
	_, ok = markerDelays{}.expectationFor(0)
	assert.False(t, ok)
}

func TestCalibrateIsARunOption(t *testing.T) {
	assert.False(t, testRunSpec(t, runOptions{}).calibrate)
	spec := testRunSpec(t, runOptions{cells: "smb-late-1000", smbEnabled: true, calibrate: true})
	assert.True(t, spec.calibrate)
	// It changes no cell: the markers stay, only their assertion is skipped.
	assert.Equal(t, testRunSpec(t, runOptions{cells: "smb-late-1000", smbEnabled: true}).cells, spec.cells)
}

func TestGlobLoadCellWritesManyServicesUnderOneSource(t *testing.T) {
	spec := testRunSpec(t, runOptions{cells: "smb,smb-glob-load", smbEnabled: true})
	smb, load := spec.cells[0], spec.cells[1]
	assert.Equal(t, defaultWriterOptions(), smb.writer)
	assert.Equal(t, writerOptions{mode: renameRotation, periodMs: 10000, rateBytesPerSec: 2000000, streams: 8}, load.writer)
	assert.Len(t, spec.streamRunIDs(load), 8)
	assert.Equal(t, testRunID+"-smb-glob-load-svc-8", spec.streamRunIDs(load)[7])
	assert.NoError(t, load.writer.validate())

	values := spec.agentHelmValues()
	// One source for the eight services; the smb cell keeps its one file.
	assert.Contains(t, values, "      - type: smb\n        path: \"*/app.log\"\n        service: azure-files-smb-glob-load\n")
	assert.Contains(t, values, "      - type: smb\n        path: app.log\n        service: azure-files-smb\n")
	assert.Contains(t, spec.writerEnv(load), envVar{"LOGWRITER_STREAMS", "8"})
	assert.Contains(t, spec.ledgerEnv(load, spec.workloadRuntime("registry.example")), envVar{"LOGWRITER_STREAMS", "8"})
	assert.NotContains(t, spec.writerEnv(smb), envVar{"LOGWRITER_STREAMS", "8"})

	// The run's options win over the cell's defaults.
	override := testRunSpec(t, runOptions{cells: "smb-glob-load", smbEnabled: true, streams: "4", rateBytesPerSec: "4000000"})
	assert.Equal(t, writerOptions{mode: renameRotation, periodMs: 10000, rateBytesPerSec: 4000000, streams: 4}, override.cells[0].writer)
	// The cell's own rate with the run's longest period would not fit in
	// its share, although the run's options alone, unpaced, do.
	_, err := newRunSpec(runOptions{runID: testRunID, cells: "smb-glob-load", smbEnabled: true, periodMs: "600000"})
	assert.ErrorContains(t, err, "cell smb-glob-load: writer rate 2000000 with a 600000ms period")

	// About 2 MB/s over 8 files: 250 KB per file and poll, 4 chunks each, and
	// 9 listings, plus the drains.
	opens := estimateSMBOpens(load.writer)
	assert.Equal(t, 9, opens.ListingsPerScan)
	assert.Equal(t, float64(8*4), opens.ReadOpensPerScan)
	assert.InDelta(t, 8.0*5/10, opens.DrainOpensPerSecond, 0.001)
	assert.InDelta(t, 9+32+4.0, opens.OpensPerSecond, 0.001)
	idle := estimateSMBOpens(defaultWriterOptions())
	assert.Equal(t, 1, idle.ListingsPerScan)
	assert.InDelta(t, 0.2, idle.ReadOpensPerScan, 0.001)
}

func TestScenariosAreValidated(t *testing.T) {
	for name, opts := range map[string]runOptions{
		`unknown scenario "reboot"`:                   {scenario: "reboot"},
		"only applies to the network-drop scenario":   {scenario: "agent-restart", networkDropSeconds: "60"},
		"network drop 30 must be between 31 and 510":  {scenario: "network-drop", networkDropSeconds: "30"},
		"network drop 511 must be between 31 and 510": {scenario: "network-drop", networkDropSeconds: "511"},
		"rotates by rename":                           {scenario: "agent-restart", rotationMode: "gzip"},
		"reads one app.log":                           {scenario: "key-rotation", streams: "2"},
		"needs a paced writer":                        {scenario: "network-drop", rateBytesPerSec: "0"},
		"period of at least 120000ms":                 {scenario: "agent-restart", periodMs: "60000"},
		"period of at least 240000ms":                 {scenario: "key-rotation", periodMs: "120000"},
		"period of at least 210000ms":                 {scenario: "network-drop", networkDropSeconds: "120", periodMs: "180000"},
		"runs with the smb cell alone, not with file": {scenario: "agent-restart", cells: "smb,file-line"},
		"runs with the smb cell alone, not with smb-": {scenario: "key-rotation", cells: "smb,smb-gzip"},
		"SMB cells may share the blocked":             {scenario: "network-drop", cells: "smb,smb-gzip"},
		"runs on the smb cell":                        {scenario: "network-drop", cells: "file-line"},
		"the Java writer image":                       {scenario: "agent-restart", writerImage: testWriterImage},
	} {
		opts.runID, opts.smbEnabled = testRunID, true
		_, err := newRunSpec(opts)
		assert.ErrorContains(t, err, name)
	}
}

func TestScenariosRunOnTheSMBCell(t *testing.T) {
	for _, kind := range disruptionScenarios {
		spec := testRunSpec(t, runOptions{scenario: " " + string(kind) + " ", smbEnabled: true})
		assert.Equal(t, kind, spec.scenario.kind)
		require.Len(t, spec.cells, 1, kind)
		c := spec.cells[0]
		assert.Equal(t, scenarioCellName, c.name)
		// A disruption delays the drains past every marker.
		assert.Zero(t, c.markers.count(), kind)
		assert.Contains(t, spec.appenderEnv(c), envVar{"LOGWRITER_APPEND_DELAYS_MS", "none"}, kind)
		// A paced writer with a period that holds the disruption.
		assert.Equal(t, renameRotation, c.writer.mode, kind)
		assert.Zero(t, c.writer.streams, kind)
		assert.True(t, c.writer.paced(), kind)
		assert.GreaterOrEqual(t, c.writer.periodMs, spec.scenario.minPeriodMs(), kind)
		assert.Equal(t, spec.writer, c.writer, kind)
		assert.Equal(t, kind == keyRotationScenario, c.agentKeyIndex == 1, kind)
		assert.Equal(t, kind, spec.scenarioMetadata()["name"])

		// The gate still applies: without it, nothing is provisioned.
		gated := testRunSpec(t, runOptions{scenario: string(kind)})
		assert.Empty(t, gated.cells, kind)
	}
	assert.Nil(t, testRunSpec(t, runOptions{}).scenarioMetadata())

	assert.Equal(t, writerOptions{mode: renameRotation, periodMs: 120000, rateBytesPerSec: 20000},
		testRunSpec(t, runOptions{scenario: "agent-restart", smbEnabled: true}).writer)
	assert.Equal(t, writerOptions{mode: renameRotation, periodMs: 300000, rateBytesPerSec: 10000},
		testRunSpec(t, runOptions{scenario: "key-rotation", smbEnabled: true}).writer)
	drop := testRunSpec(t, runOptions{scenario: "network-drop", smbEnabled: true})
	assert.Equal(t, defaultNetworkDropSeconds, drop.scenario.dropSeconds)
	assert.Equal(t, writerOptions{mode: renameRotation, periodMs: 180000, rateBytesPerSec: 10000}, drop.writer)
	// A longer drop gets a longer default period.
	long := testRunSpec(t, runOptions{scenario: "network-drop", networkDropSeconds: "200", smbEnabled: true})
	assert.Equal(t, (200+networkDropMarginSeconds)*1000, long.writer.periodMs)
	// Explicit options are kept when they hold the disruption.
	explicit := testRunSpec(t, runOptions{scenario: "agent-restart", periodMs: "180000", rateBytesPerSec: "5000", smbEnabled: true})
	assert.Equal(t, writerOptions{mode: renameRotation, periodMs: 180000, rateBytesPerSec: 5000}, explicit.writer)

	// network-drop only blocks the Agent pod's namespace, so the file cells
	// can run next to it with their usual markers.
	withFiles := testRunSpec(t, runOptions{scenario: "network-drop", cells: "smb,file-line", smbEnabled: true})
	require.Len(t, withFiles.cells, 2)
	assert.Equal(t, markerDelaysFor(fileReader, writerOptions{mode: renameRotation}), withFiles.cells[1].markers)

	// The drop outlasts an operation timeout, and the period holds the drop,
	// the longest backoff after it and a margin.
	assert.Greater(t, minNetworkDropSeconds, smbOpTimeoutSeconds)
	assert.Greater(t, keyRotationForcedDropSeconds, smbOpTimeoutSeconds)
	assert.GreaterOrEqual(t, networkDropPeriodMs, (defaultNetworkDropSeconds+networkDropMarginSeconds)*1000)
	assert.Greater(t, agentRestartMinPeriodMs, agentRestartDeleteAfterMs)
	assert.Less(t, keyRotationStartAfterMs+int(keyRotationNaturalAuthWait/time.Millisecond)+keyRotationForcedDropSeconds*1000+(smbMaxBackoffSeconds+60+3*secretRefreshIntervalSeconds)*1000,
		keyRotationPeriodMs)
}

func TestKeyRotationEnablesTheSecretRefresh(t *testing.T) {
	values := testRunSpec(t, runOptions{scenario: "key-rotation", smbEnabled: true}).agentHelmValues()
	assert.Contains(t, values, "        DD_LOGS_CONFIG_UNRELIABLE_MOUNT_ENABLED: \"false\"\n"+
		"        DD_SECRET_REFRESH_INTERVAL: \"15\"\n"+
		"        DD_SECRET_REFRESH_SCATTER: \"false\"\n")
	// The Agent still reads the key from its mounted Secret.
	assert.Contains(t, values, `password: "ENC[file@/etc/azure-files-secrets/smb/azurestorageaccountkey]"`)

	for _, opts := range []runOptions{
		{smbEnabled: true},
		{scenario: "agent-restart", smbEnabled: true},
		{scenario: "network-drop", smbEnabled: true},
	} {
		assert.NotContains(t, testRunSpec(t, opts).agentHelmValues(), "DD_SECRET_REFRESH", opts.scenario)
	}
}

func TestStorageAccountKeyAtPicksKey1OrKey2(t *testing.T) {
	keys := []any{map[string]any{"keyName": "key1", "value": "first"}, map[string]any{"keyName": "key2", "value": "second"}}
	key, err := storageAccountKeyAt(keys, 1)
	require.NoError(t, err)
	assert.Equal(t, "second", key)
	key, err = firstStorageAccountKey(keys)
	require.NoError(t, err)
	assert.Equal(t, "first", key)
	_, err = storageAccountKeyAt(keys[:1], 1)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "first")
}

func TestStorageAccountIDAddressesTheAzureCLI(t *testing.T) {
	account, err := parseStorageAccountID("/subscriptions/sub-id/resourceGroups/dd-agent-sandbox/providers/Microsoft.Storage/storageAccounts/ddafsmbabc")
	require.NoError(t, err)
	assert.Equal(t, storageAccountRef{subscription: "sub-id", resourceGroup: "dd-agent-sandbox", name: "ddafsmbabc"}, account)
	_, err = parseStorageAccountID("ddafsmbabc")
	assert.Error(t, err)

	// The renewal prints nothing; the key is read on its own.
	renew := strings.Join(account.renewKeyArgs(), " ")
	assert.Equal(t, "storage account keys renew --subscription sub-id --resource-group dd-agent-sandbox --account-name ddafsmbabc --key secondary --output none", renew)
	list := account.secondaryKeyArgs()
	assert.Equal(t, []string{"--query", "[?keyName=='key2'].value | [0]", "--output", "tsv"}, list[len(list)-4:])
}

func TestMissedBytesReportsAreReadFromTheAgentLog(t *testing.T) {
	log := strings.Join([]string{
		"2026-10-06T18:29:07.123456789Z 2026-10-06 18:29:07 UTC | CORE | WARN | (pkg/logs/tailers/smb/tailer.go:427 in RecordMissedBytes) | " +
			"Rotated SMB file is no longer listed: 1234 bytes of SMB file smb://acct.file.core.windows.net/share/svc-1/app.log (last read as svc-1/app.log.06102026_182900) were not read and are lost",
		"2026-10-06 18:30:01 UTC | CORE | WARN | (pkg/logs/tailers/smb/tailer.go:427 in RecordMissedBytes) | " +
			"SMB rotation drain timed out after 5s (logs_config.close_timeout): 7 bytes of SMB file smb://acct.file.core.windows.net/share/app.log (last read as app.log) were not read and are lost",
		"2026-10-06 18:30:02 UTC | CORE | INFO | (pkg/logs/tailers/smb/tailer.go:290 in Stop) | Closed SMB tailer for smb://acct.file.core.windows.net/share/app.log",
		"2026-10-06 18:30:03 UTC | CORE | WARN | (pkg/logs/tailers/file/tailer.go:388 in func1) | After the rotation close timeout (5s), there were 10 bytes remaining unread",
	}, "\n")
	reports := parseMissedBytesReports(log)
	require.Len(t, reports, 2)
	assert.Equal(t, missedBytesReport{
		At: time.Date(2026, 10, 6, 18, 29, 7, 123456789, time.UTC), Reason: "Rotated SMB file is no longer listed", Bytes: 1234,
		Identifier: "smb://acct.file.core.windows.net/share/svc-1/app.log", ReadPath: "svc-1/app.log.06102026_182900",
	}, reports[0])
	assert.Equal(t, time.Date(2026, 10, 6, 18, 30, 1, 0, time.UTC), reports[1].At)
	assert.Equal(t, "SMB rotation drain timed out after 5s (logs_config.close_timeout)", reports[1].Reason)
	assert.Equal(t, int64(1241), sumReportedBytes(reports))

	c := cell{accountName: "acct", shareName: "share"}
	assert.Equal(t, "smb://acct.file.core.windows.net/share/svc-1/app.log", smbIdentifier(c, "svc-1"))
	assert.Len(t, reportsOfCell(c, reports), 2)
	assert.Empty(t, reportsOfCell(cell{accountName: "other", shareName: "share"}, reports))

	assert.NoError(t, checkMissedBytesTotal(reports[:1], reports, 1241))
	assert.NoError(t, checkMissedBytesTotal(reports[:1], reports, 1234))
	assert.ErrorContains(t, checkMissedBytesTotal(reports, reports, 2000), "some missed bytes have no warning")
	assert.Error(t, checkMissedBytesTotal(reports, reports, 1000))
}

// lossFixture is a file of ten 100-byte records, run r, rotated at 12:00:00,
// by a writer that writes 20000 B/s.
func lossFixture(mode rotationMode) (cell, []ledgerEntry, map[recordKey]struct{}) {
	c := cell{name: "smb-" + string(mode), reader: smbReader, accountName: "acct", shareName: "share",
		writer: writerOptions{mode: mode, periodMs: 60000, rateBytesPerSec: 20000}}
	ledger := []ledgerEntry{
		{RunID: "r", Period: "p1", File: "app.log.1", FirstSequence: 1, LastSequence: 10, Bytes: 1000, RotatedAt: "2026-10-06T12:00:00.000Z"},
		{RunID: "r", Period: "p2", File: "app.log.2", FirstSequence: 11, LastSequence: 12, Bytes: 200, RotatedAt: "2026-10-06T12:01:00.000Z"},
	}
	if mode == deleteRecreateRotation {
		ledger[0].File, ledger[1].File = activeLogName, activeLogName
	}
	return c, ledger, expectedRecords(ledger[:1])
}

func collectedSequences(sequences ...int64) (map[recordKey]int, map[recordKey]int64) {
	counts, sizes := map[recordKey]int{}, map[recordKey]int64{}
	for _, sequence := range sequences {
		counts[recordKey{"r", sequence}]++
		sizes[recordKey{"r", sequence}] = 100
	}
	return counts, sizes
}

func TestLossesMustBeTheEndOfAFileAndReported(t *testing.T) {
	at := func(clock string) time.Time {
		parsed, err := time.Parse(time.RFC3339, "2026-10-06T"+clock+"Z")
		require.NoError(t, err)
		return parsed
	}
	for _, mode := range []rotationMode{gzipRotation, deleteRecreateRotation} {
		c, ledger, expected := lossFixture(mode)
		readPath := "app.log.1"
		if mode == deleteRecreateRotation {
			readPath = activeLogName
		}
		// After the compression, 5s after the rotation, for gzip.
		report := func(bytes int64, when string) missedBytesReport {
			return missedBytesReport{At: at(when), Reason: "Rotated SMB file is no longer listed", Bytes: bytes, Identifier: smbIdentifier(c, ""), ReadPath: readPath}
		}
		counts, sizes := collectedSequences(1, 2, 3, 4, 5, 6, 7)
		check := checkRecords(expected, nil, counts)
		require.Len(t, check.missing, 3, mode)

		// The last three records, 300 bytes, reported as missed.
		losses := explainLosses(c, ledger, ledger[:1], check, sizes, []missedBytesReport{report(300, "12:00:06")})
		require.Len(t, losses, 1, mode)
		assert.True(t, losses[0].Suffix, mode)
		assert.Equal(t, int64(300), losses[0].UnreadBytes, mode)
		assert.Equal(t, int64(300), losses[0].ReportedBytes, mode)
		assert.Equal(t, "r:8-10", losses[0].MissingRanges, mode)
		assert.Equal(t, 7, losses[0].CollectedRecords, mode)
		// 20000 B/s over a poll interval and a scan, in 1 KiB payloads, and
		// one 64 KiB write.
		assert.Equal(t, 40+64, losses[0].AllowedRecords, mode)
		explained := new(recordingT)
		assertLossesExplained(explained, c, losses)
		assert.Empty(t, explained.failures, mode)

		// Nothing reported, or for another file: a silent loss.
		for name, reports := range map[string][]missedBytesReport{
			"none":         nil,
			"other stream": {{At: at("12:00:06"), Bytes: 300, Identifier: smbIdentifier(c, "svc-2"), ReadPath: readPath}},
		} {
			silent := new(recordingT)
			assertLossesExplained(silent, c, explainLosses(c, ledger, ledger[:1], check, sizes, reports))
			require.Len(t, silent.failures, 1, "%s %s", mode, name)
			assert.Contains(t, silent.failures[0], "the Agent reported no missed bytes for it", mode)
		}

		// A report short of the loss by more than what can be written after
		// the last listing is partly silent; within that, it is not.
		shortfall := fileLoss{Suffix: true, MissingRecords: 3, AllowedRecords: 104, CollectedRecords: 7,
			UnreadBytes: 50000, ReportedBytes: 300, ShortfallBytes: 49700, AllowedShortfallBytes: lossAccountingToleranceBytes}
		under := new(recordingT)
		assertLossesExplained(under, c, []fileLoss{shortfall})
		require.Len(t, under.failures, 1, mode)
		assert.Contains(t, under.failures[0], "partly silent loss", mode)
		shortfall.AllowedShortfallBytes = maxReportShortfallBytes(c.writer)
		within := new(recordingT)
		assertLossesExplained(within, c, []fileLoss{shortfall})
		assert.Empty(t, within.failures, mode)
		over := new(recordingT)
		assertLossesExplained(over, c, []fileLoss{{Suffix: true, MissingRecords: 3, AllowedRecords: 104, CollectedRecords: 7, UnreadBytes: 300, ReportedBytes: 50000}})
		require.Len(t, over.failures, 1, mode)
		assert.Contains(t, over.failures[0], "attributed to the wrong file", mode)

		// A hole before collected records is never a drain's loss.
		holed, holedSizes := collectedSequences(1, 2, 3, 5, 6, 7, 8, 9, 10)
		inside := new(recordingT)
		assertLossesExplained(inside, c, explainLosses(c, ledger, ledger[:1], checkRecords(expected, nil, holed), holedSizes, []missedBytesReport{report(100, "12:00:06")}))
		require.Len(t, inside.failures, 1, mode)
		assert.Contains(t, inside.failures[0], "not its last ones (r:4)", mode)

		// A whole file lost is never a drain's loss, even reported.
		none, noneSizes := collectedSequences()
		whole := new(recordingT)
		assertLossesExplained(whole, c, explainLosses(c, ledger, ledger[:1], checkRecords(expected, nil, none), noneSizes, []missedBytesReport{report(1000, "12:00:06")}))
		require.Len(t, whole.failures, 1, mode)
		assert.Contains(t, whole.failures[0], "lost every one of its 10 records", mode)

		// The Java schedule leaves nothing to lose.
		unpaced := c
		unpaced.writer.rateBytesPerSec = 0
		strict := new(recordingT)
		assertLossesExplained(strict, unpaced, explainLosses(unpaced, ledger, ledger[:1], check, sizes, []missedBytesReport{report(300, "12:00:06")}))
		require.Len(t, strict.failures, 1, mode)
		assert.Contains(t, strict.failures[0], "more than the 0 a "+string(mode)+" rotation can take", mode)
	}

	// A gzip drain that ends while the rotated file is still there, before
	// the compression, has no excuse to leave records unread; one that timed
	// out has, and so has one when the run compresses at once.
	c, ledger, expected := lossFixture(gzipRotation)
	counts, sizes := collectedSequences(1, 2, 3, 4, 5, 6, 7)
	check := checkRecords(expected, nil, counts)
	early := []missedBytesReport{{At: at("12:00:02"), Reason: "Rotated SMB file is no longer listed", Bytes: 300, Identifier: smbIdentifier(c, ""), ReadPath: "app.log.1"}}
	premature := new(recordingT)
	assertLossesExplained(premature, c, explainLosses(c, ledger, ledger[:1], check, sizes, early))
	require.Len(t, premature.failures, 1)
	assert.Contains(t, premature.failures[0], "was reported lost (\"Rotated SMB file is no longer listed\") 2s after its rotation, before the writer compressed it away 5s after the rotation")
	timedOut := slices.Clone(early)
	timedOut[0].Reason = "SMB rotation drain timed out after 5s (logs_config.close_timeout)"
	slow := new(recordingT)
	assertLossesExplained(slow, c, explainLosses(c, ledger, ledger[:1], check, sizes, timedOut))
	assert.Empty(t, slow.failures)
	forced := c
	forced.writer.forceLoss = true
	immediate := new(recordingT)
	assertLossesExplained(immediate, forced, explainLosses(forced, ledger, ledger[:1], check, sizes, early))
	assert.Empty(t, immediate.failures)

	// A deleted file's report is the one made before the next rotation.
	c, ledger, expected = lossFixture(deleteRecreateRotation)
	late := []missedBytesReport{{At: at("12:01:30"), Bytes: 300, Identifier: smbIdentifier(c, ""), ReadPath: activeLogName}}
	losses := explainLosses(c, ledger, ledger[:1], checkRecords(expected, nil, counts), sizes, late)
	require.Len(t, losses, 1)
	assert.Zero(t, losses[0].ReportedBytes)

	// A gzip report names the rotated file.
	c, ledger, expected = lossFixture(gzipRotation)
	other := []missedBytesReport{{At: at("12:00:06"), Bytes: 300, Identifier: smbIdentifier(c, ""), ReadPath: "app.log.2"}}
	losses = explainLosses(c, ledger, ledger[:1], checkRecords(expected, nil, counts), sizes, other)
	assert.Zero(t, losses[0].ReportedBytes)
	// Unless the file was gone before the source saw the rotation: then the
	// drain never found it under its rotated name and reports app.log.
	unseen := []missedBytesReport{{At: at("12:00:07"), Bytes: 300, Identifier: smbIdentifier(c, ""), ReadPath: activeLogName}}
	losses = explainLosses(c, ledger, ledger[:1], checkRecords(expected, nil, counts), sizes, unseen)
	assert.Equal(t, int64(300), losses[0].ReportedBytes)

	// Lines are counted with their newline.
	lines := recordLineBytes([]string{"x run_id=r period=p sequence=1 record=1", "noise"}, map[recordKey]struct{}{{"r", 1}: {}})
	assert.Equal(t, map[recordKey]int64{{"r", 1}: 40}, lines)
}

func TestRestartDuplicatesAreBounded(t *testing.T) {
	paced := writerOptions{mode: renameRotation, periodMs: 120000, rateBytesPerSec: 20000}
	assert.Zero(t, restartDuplicateBound(paced, true))
	// 20000 B/s of 1 KiB payloads over a flush period, a batch wait and a
	// second, plus one 64 KiB write.
	assert.Equal(t, int(math.Ceil(20000.0/1024*7))+64, restartDuplicateBound(paced, false))
	assert.Zero(t, restartDuplicateBound(defaultWriterOptions(), false))

	expected := map[recordKey]struct{}{}
	for sequence := int64(1); sequence <= 10; sequence++ {
		expected[recordKey{"r", sequence}] = struct{}{}
	}
	all := func(extra map[int64]int) map[recordKey]int {
		counts := map[recordKey]int{}
		for key := range expected {
			counts[key] = 1 + extra[key.sequence]
		}
		return counts
	}
	c := cell{name: "smb"}
	resent := all(map[int64]int{5: 1, 6: 1, 7: 1})
	ok := new(recordingT)
	assertRestartRecords(ok, c, checkRecords(expected, nil, resent), resent, restartRule{bound: 3})
	assert.Empty(t, ok.failures)

	tooMany := new(recordingT)
	assertRestartRecords(tooMany, c, checkRecords(expected, nil, resent), resent, restartRule{bound: 2, graceful: true})
	require.Len(t, tooMany.failures, 1)
	assert.Contains(t, tooMany.failures[0], "more than the 2 the Agent can resend after a graceful restart")

	scattered := all(map[int64]int{2: 1, 7: 2})
	problems := restartDuplicateProblems(checkRecords(expected, nil, scattered), scattered, restartRule{bound: 10})
	require.Len(t, problems, 2)
	assert.Contains(t, problems[0], "more than twice")
	assert.Contains(t, problems[1], "not one run of sequences")

	lost := all(nil)
	delete(lost, recordKey{"r", 10})
	missing := new(recordingT)
	assertRestartRecords(missing, c, checkRecords(expected, nil, lost), lost, restartRule{bound: 3})
	require.Len(t, missing.failures, 1)
	assert.Contains(t, missing.failures[0], "never collected across the Agent restart: r:10")

	assert.Equal(t, gracefulLogsStop, classifyLogsStop("... | INFO | Stopping logs-agent\n... | INFO | logs-agent stopped\n", nil))
	assert.Equal(t, forcedLogsStop, classifyLogsStop("Timed out when stopping logs-agent, forcing it to stop now\nlogs-agent stopped", nil))
	// Killed before it could log how the logs agent stopped.
	killed := &k8scorev1.ContainerStateTerminated{ExitCode: 137, Reason: "Error"}
	assert.Equal(t, forcedLogsStop, classifyLogsStop("... | INFO | Stopping logs-agent\n", killed))
	// No evidence either way is inconclusive, never the forced bound.
	assert.Equal(t, unknownLogsStop, classifyLogsStop("", nil))
	assert.Equal(t, unknownLogsStop, classifyLogsStop("... | INFO | Stopping logs-agent\n", &k8scorev1.ContainerStateTerminated{ExitCode: 0}))
}

func TestRegistryMustOutliveTheAgentPod(t *testing.T) {
	pod := func(volume k8scorev1.VolumeSource) k8scorev1.Pod {
		return k8scorev1.Pod{Spec: k8scorev1.PodSpec{
			Containers: []k8scorev1.Container{{Name: agentContainer, VolumeMounts: []k8scorev1.VolumeMount{{Name: "pointerdir", MountPath: logsRunPath}}}},
			Volumes:    []k8scorev1.Volume{{Name: "pointerdir", VolumeSource: volume}},
		}}
	}
	// What chart 3.245.0 renders with datadog.logs.enabled.
	assert.NoError(t, checkRegistryPersists(pod(k8scorev1.VolumeSource{HostPath: &k8scorev1.HostPathVolumeSource{Path: "/var/lib/datadog-agent/logs"}})))
	assert.ErrorContains(t, checkRegistryPersists(pod(k8scorev1.VolumeSource{EmptyDir: &k8scorev1.EmptyDirVolumeSource{}})), "not a hostPath")
	assert.ErrorContains(t, checkRegistryPersists(k8scorev1.Pod{}), "mounts nothing at /opt/datadog-agent/run")
}

func TestDisruptionsAreTimedInsideOnePeriod(t *testing.T) {
	at := func(clock string) time.Time {
		parsed, err := time.Parse(time.RFC3339, "2026-10-06T"+clock+"Z")
		require.NoError(t, err)
		return parsed
	}
	// Periods start at multiples of the period since the epoch.
	assert.Equal(t, at("12:02:00"), periodStart(at("12:03:59"), 120000))
	assert.Equal(t, at("12:00:00"), periodStart(at("12:04:59"), 300000))

	start, end := disruptionTiming(at("12:02:05"), 120000, 30*time.Second)
	assert.Equal(t, at("12:02:30"), start)
	assert.Equal(t, at("12:04:00"), end)
	// Too late in the period: the next one.
	start, end = disruptionTiming(at("12:02:31"), 120000, 30*time.Second)
	assert.Equal(t, at("12:04:30"), start)
	assert.Equal(t, at("12:06:00"), end)

	// The drop is centred on the end of the period.
	block, rotation := networkDropTiming(at("12:03:05"), 180000, 90)
	assert.Equal(t, at("12:05:15"), block)
	assert.Equal(t, at("12:06:00"), rotation)
	block, rotation = networkDropTiming(at("12:05:20"), 180000, 90)
	assert.Equal(t, at("12:08:15"), block)
	assert.Equal(t, at("12:09:00"), rotation)
}

func TestNetworkHelperOnlyTouchesTheAgentNamespace(t *testing.T) {
	pod := networkHelperPod("network-helper-1", "aks-node-0", "registry-1.docker.io/"+stockWorkloadImage,
		[]k8scorev1.LocalObjectReference{{Name: "pull"}}, testRunID)
	assert.Equal(t, e2eNamespace, pod.Namespace)
	assert.Equal(t, "aks-node-0", pod.Spec.NodeName)
	assert.True(t, pod.Spec.HostPID)
	assert.False(t, pod.Spec.HostNetwork, "the helper enters the Agent's network namespace, it does not need the node's")
	assert.Equal(t, k8scorev1.RestartPolicyNever, pod.Spec.RestartPolicy)
	require.Len(t, pod.Spec.Containers, 1)
	container := pod.Spec.Containers[0]
	assert.True(t, *container.SecurityContext.Privileged)
	assert.Zero(t, *container.SecurityContext.RunAsUser)
	// Pinned by digest: the writers' stock image.
	assert.Regexp(t, `@sha256:[0-9a-f]{64}$`, container.Image)
	// It is no writer pod: findWriterPod selects on the cell label.
	assert.NotContains(t, pod.Labels, cellLabel)
	assert.Equal(t, networkHelperApp, pod.Labels[helperRoleLabel])
	// A helper whose test process died stops by itself.
	require.NotNil(t, pod.Spec.ActiveDeadlineSeconds)
	assert.Equal(t, int64(networkHelperDeadlineSeconds), *pod.Spec.ActiveDeadlineSeconds)
	assert.Equal(t, []string{"sleep", strconv.Itoa(networkHelperDeadlineSeconds)}, container.Command)
	// It outlives the longest drop, a period to wait for it, and its start.
	assert.Greater(t, networkHelperDeadlineSeconds, maxNetworkDropSeconds+maxWriterPeriodMs/1000+120)

	assert.Equal(t, []string{"nsenter", "--mount=/proc/1/ns/mnt", "--net=/proc/4242/ns/net", "--", "iptables", "-w", "5",
		"-I", "OUTPUT", "1", "-p", "tcp", "-d", "20.60.1.2", "--dport", "445", "-m", "comment", "--comment", "tag", "-j", "DROP"},
		nsenterIptables("4242", dropRuleArgs("-I", "20.60.1.2", "tag")...))
	assert.Equal(t, []string{"-D", "OUTPUT", "-p", "tcp", "-d", "20.60.1.2", "--dport", "445", "-m", "comment", "--comment", "tag", "-j", "DROP"},
		dropRuleArgs("-D", "20.60.1.2", "tag"))

	assert.Equal(t, []string{"20.60.1.2", "20.60.1.3"}, parseAddresses("20.60.1.2      STREAM file.core.windows.net\n20.60.1.2      DGRAM\n20.60.1.3      STREAM\n"))
	tcp := `  sl  local_address rem_address   st tx_queue rx_queue
   0: 0A00000A:A1B2 0201033C:01BD 01 00000000:00000000 00:00000000 00000000     0
   1: 0A00000A:A1B3 0201033C:01BD 06 00000000:00000000 00:00000000 00000000     0
   2: 0A00000A:A1B4 0201033C:1F90 01 00000000:00000000 00:00000000 00000000     0`
	assert.Equal(t, 1, countSMBConnections(tcp))
	listing := `Chain OUTPUT (policy ACCEPT 10 packets, 600 bytes)
    pkts      bytes target     prot opt in     out     source               destination
      37     2220 DROP       tcp  --  *      *       0.0.0.0/0            20.60.1.2            tcp dpt:445 /* tag */
       5      300 DROP       tcp  --  *      *       0.0.0.0/0            20.60.1.3            tcp dpt:445 /* tag */
       9      540 DROP       tcp  --  *      *       0.0.0.0/0            20.60.1.4            tcp dpt:445 /* other */`
	assert.Equal(t, int64(42), droppedPacketCount(listing, "tag"))

	id, err := agentContainerID(k8scorev1.Pod{Status: k8scorev1.PodStatus{ContainerStatuses: []k8scorev1.ContainerStatus{
		{Name: "trace-agent", ContainerID: "containerd://other"}, {Name: agentContainer, ContainerID: "containerd://abc123"},
	}}})
	require.NoError(t, err)
	assert.Equal(t, "abc123", id)
	_, err = agentContainerID(k8scorev1.Pod{})
	assert.Error(t, err)
}

func TestSourceStatusesAreClassified(t *testing.T) {
	assert.Equal(t, sourceOK, classifySourceStatus("OK"))
	assert.Equal(t, sourcePending, classifySourceStatus("Pending"))
	assert.Equal(t, sourceAuthError, classifySourceStatus("Error: cannot read smb://h/s: the server rejected the credentials or denied access. Check the username"))
	assert.Equal(t, sourceUnreachable, classifySourceStatus("Error: cannot reach smb://h/s, retrying: i/o timeout"))
	assert.Equal(t, sourceOtherError, classifySourceStatus("Error: not found on smb://h/s"))

	start := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	obs := []statusObservation{
		{At: start, State: sourceOK},
		{At: start.Add(5 * time.Second), State: sourceUnreachable},
		{At: start.Add(10 * time.Second), State: sourceAuthError},
		{At: start.Add(15 * time.Second), State: sourceOK},
	}
	assert.Equal(t, 2, firstObservation(obs, 0, start, sourceAuthError))
	assert.Equal(t, 3, firstObservation(obs, 3, time.Time{}, sourceOK))
	assert.Equal(t, 3, firstObservation(obs, 0, start.Add(time.Second), sourceOK))
	assert.Equal(t, -1, firstObservation(obs, 0, start.Add(20*time.Second), sourceOK))
}

func TestLoadReportMeasuresThroughputAndLag(t *testing.T) {
	c := cell{name: "smb-glob-load", writer: writerOptions{mode: renameRotation, periodMs: 10000, rateBytesPerSec: 1000, streams: 2}}
	asserted := []ledgerEntry{
		{RunID: "s1", Period: "p1", FirstSequence: 1, LastSequence: 1, Bytes: 5000, RotatedAt: "2026-10-06T12:00:10.000Z"},
		{RunID: "s1", Period: "p2", FirstSequence: 2, LastSequence: 2, Bytes: 10000, RotatedAt: "2026-10-06T12:00:20.000Z"},
		{RunID: "s2", Period: "p2", FirstSequence: 1, LastSequence: 1, Bytes: 10000, RotatedAt: "2026-10-06T12:00:20.000Z"},
	}
	expected := expectedRecords(asserted)
	record := func(written string, runID string, sequence int) string {
		return written + "  INFO  1 --- [        scheduling-1] c.d.e.l.LogWriterService                 : run_id=" + runID +
			" period=p sequence=" + strconv.Itoa(sequence) + " record=1 phase=fill"
	}
	arrived := time.Date(2026, 10, 6, 12, 0, 21, 0, time.UTC)
	logs := []collectedLog{
		{message: record("2026-10-06 12:00:19.000", "s1", 2), timestamp: time.Date(2026, 10, 6, 12, 0, 19, 500e6, time.UTC).UnixMilli(), arrived: arrived},
		{message: record("2026-10-06 12:00:09.000", "s1", 1), timestamp: time.Date(2026, 10, 6, 12, 0, 9, 500e6, time.UTC).UnixMilli(), arrived: arrived.Add(-10 * time.Second)},
		{message: record("2026-10-06 12:00:18.000", "s2", 1), timestamp: time.Date(2026, 10, 6, 12, 0, 18, 500e6, time.UTC).UnixMilli(), arrived: arrived},
		{message: "post_rotation_marker run_id=s1", arrived: arrived},
	}
	report := buildLoadReport(c, asserted, expected, logs)
	assert.Equal(t, 3, report.CollectedRecords)
	assert.Equal(t, int64(25000), report.WrittenBytes)
	// Each stream's first file is left out: s1's second, s2 has none.
	assert.InDelta(t, 10000.0/((1.0/2)*10), report.WriterBytesPerSec, 0.001)
	assert.InDelta(t, 10, report.ArrivalWindowSeconds, 0.001)
	assert.Equal(t, latencySummary{Count: 3, P50: 2000, P95: 3000, P99: 3000, Max: 3000}, report.PipelineLag)
	assert.Equal(t, 500.0, report.AgentReadLag.Max)
	assert.Equal(t, 2500.0, report.DeliveryLag.Max)
	// Each file's last record arrived 1s after its rotation was journalled.
	assert.Equal(t, latencySummary{Count: 3, P50: 1000, P95: 1000, P99: 1000, Max: 1000}, report.FileCompletionLag)
	assert.Equal(t, 3, report.SMBOpens.ListingsPerScan)

	_, ok := recordWriteTime("short")
	assert.False(t, ok)
	assert.Equal(t, latencySummary{}, summarizeLatencies(nil))
}

func TestMultiNodeScenarioRunsTheSMBCellOnSeveralNodes(t *testing.T) {
	spec := testRunSpec(t, runOptions{scenario: " multi-node ", smbEnabled: true})
	assert.Equal(t, multiNodeScenario, spec.scenario.kind)
	assert.Equal(t, defaultMultiNodeNodes, spec.nodes)
	require.Len(t, spec.cells, 1)
	c := spec.cells[0]
	assert.Equal(t, scenarioCellName, c.name)
	// Nothing disrupts the Agents, so the markers still calibrate each of
	// them, and the writer keeps its defaults.
	assert.False(t, multiNodeScenario.disrupts())
	assert.Equal(t, markerDelaysFor(smbReader, writerOptions{mode: renameRotation}), c.markers)
	assert.Equal(t, defaultWriterOptions(), c.writer)
	assert.Zero(t, spec.scenario.minPeriodMs())
	assert.Equal(t, 2, spec.scenarioMetadata()["nodes"])
	assert.Contains(t, knownScenarios, multiNodeScenario)
	assert.NotContains(t, disruptionScenarios, multiNodeScenario)

	// The run's writer options apply, but for another rotation mode.
	paced := testRunSpec(t, runOptions{scenario: "multi-node", smbEnabled: true, nodes: "3", periodMs: "10000", rateBytesPerSec: "100000", streams: "2"})
	assert.Equal(t, 3, paced.nodes)
	assert.Equal(t, writerOptions{mode: renameRotation, periodMs: 10000, rateBytesPerSec: 100000, streams: 2}, paced.cells[0].writer)

	for name, opts := range map[string]runOptions{
		"only the multi-node scenario runs more than one node": {nodes: "2"},
		"2 nodes run 2 Agents":                                 {scenario: "agent-restart", nodes: "2"},
		"needs at least 2 nodes, not 1":                        {scenario: "multi-node", nodes: "1"},
		"node count 6 must be between 1 and 5":                 {scenario: "multi-node", nodes: "6"},
		`node count "two" is not a number`:                     {scenario: "multi-node", nodes: "two"},
		"runs with the smb cell alone, not with file-line":     {scenario: "multi-node", cells: "smb,file-line"},
		"runs with the smb cell alone, not with smb-gzip":      {scenario: "multi-node", cells: "smb,smb-gzip"},
		"rotates by rename":                                    {scenario: "multi-node", rotationMode: "copytruncate"},
	} {
		opts.runID, opts.smbEnabled = testRunID, true
		_, err := newRunSpec(opts)
		assert.ErrorContains(t, err, name)
	}
	// One node is the default of every other run, and may be asked for.
	assert.Equal(t, 1, testRunSpec(t, runOptions{}).nodes)
	assert.Equal(t, 1, testRunSpec(t, runOptions{nodes: "1"}).nodes)
	assert.Equal(t, 1, testRunSpec(t, runOptions{scenario: "agent-restart", smbEnabled: true}).nodes)
	// The gate still applies.
	assert.Empty(t, testRunSpec(t, runOptions{scenario: "multi-node"}).cells)
}

func TestMultiNodeRunsGetAStackOfTheirOwn(t *testing.T) {
	one := testRunSpec(t, runOptions{stackName: "azure-files-dev", smbEnabled: true})
	two := testRunSpec(t, runOptions{stackName: "azure-files-dev", scenario: "multi-node", smbEnabled: true})
	assert.Equal(t, "azure-files-dev", one.stackName)
	// A reused one-node stack is never resized: the two-node run has a stack,
	// and so storage accounts, of its own.
	assert.Equal(t, "azure-files-dev-n2", two.stackName)
	smb, ok := one.cellNamed("smb")
	require.True(t, ok)
	assert.NotEqual(t, smb.accountName, two.cells[0].accountName)
	assert.Regexp(t, `^azure-files-[0-9a-f]{8}-n3$`, testRunSpec(t, runOptions{scenario: "multi-node", nodes: "3", smbEnabled: true}).stackName)

	// The suffix is the suite's to add.
	for _, nodes := range []string{"", "2"} {
		opts := runOptions{runID: testRunID, stackName: "azure-files-dev-n2", smbEnabled: true, nodes: nodes}
		if nodes != "" {
			opts.scenario = "multi-node"
		}
		_, err := newRunSpec(opts)
		assert.ErrorContains(t, err, "-n<nodes> suffix the suite adds itself", nodes)
	}
}

func TestEveryAgentMustCollectEveryRecordOnce(t *testing.T) {
	c := cell{name: "smb", reader: smbReader, markers: markerDelaysFor(smbReader, writerOptions{mode: renameRotation})}
	expected := map[recordKey]struct{}{{"r", 1}: {}, {"r", 2}: {}}
	markers := []markerEntry{
		{MarkerID: "r-r1-m500", MarkerAgeMs: 500, RotatedFile: "app.log.1"},
		{MarkerID: "r-r1-m45000", MarkerAgeMs: 45000, RotatedFile: "app.log.1"},
	}
	record := func(host string, sequence int) collectedLog {
		return collectedLog{hostname: host, message: fmt.Sprintf("x run_id=r period=p sequence=%d record=%d", sequence, sequence)}
	}
	marker := func(host, id string) collectedLog {
		return collectedLog{hostname: host, message: "pppp post_rotation_marker run_id=r marker_id=" + id}
	}
	hosts := []string{"node-a", "node-b"}
	both := []collectedLog{
		record("node-a", 1), record("node-a", 2), marker("node-a", "r-r1-m500"),
		record("node-b", 1), record("node-b", 2), marker("node-b", "r-r1-m500"),
	}
	judge := func(logs []collectedLog, calibrate bool) ([]string, string) {
		recorded := new(recordingT)
		summary := assertCollectedOncePerHost(recorded, c, expected, nil, markersToAssert(c, markers, calibrate), logs, hosts)
		return recorded.failures, summary
	}

	failures, summary := judge(both, false)
	assert.Empty(t, failures)
	assert.Contains(t, summary, "each of the 2 Agents collected every record once, 2 copies of each in all; node-a: 2 records expected")
	assert.Contains(t, summary, "; node-b: 2 records expected")

	// Exactly once in all, what a single elected reader would give, fails:
	// every Agent has the source, so each must read every record.
	failures, _ = judge([]collectedLog{record("node-a", 1), record("node-b", 2), marker("node-a", "r-r1-m500")}, false)
	require.Len(t, failures, 3)
	assert.Contains(t, failures[0], "smb@node-a: 1 of 2 expected records were never collected")
	assert.Contains(t, failures[0], "r:2")
	assert.Contains(t, failures[1], "smb@node-b: 1 of 2 expected records were never collected")
	assert.Contains(t, failures[2], "smb@node-b: marker r-r1-m500")

	// An Agent collecting a record twice, a host that runs no Agent, and the
	// calibration marker surviving on one Agent.
	failures, _ = judge(append(slices.Clone(both), record("node-a", 2)), false)
	require.Len(t, failures, 1)
	assert.Contains(t, failures[0], "smb@node-a: 1 of 2 expected records were collected more than once")
	failures, _ = judge(append(slices.Clone(both), record("node-c", 1)), false)
	require.Len(t, failures, 1)
	assert.Contains(t, failures[0], "sent by hosts that run none of the 2 Agents (node-a, node-b)")
	failures, _ = judge(append(slices.Clone(both), marker("node-b", "r-r1-m45000")), false)
	require.Len(t, failures, 1)
	assert.Contains(t, failures[0], "smb@node-b: marker r-r1-m45000")
	// Calibration only relaxes a probe marker, which the smb cell has not.
	failures, _ = judge(append(slices.Clone(both), marker("node-b", "r-r1-m45000")), true)
	require.Len(t, failures, 1)
	assert.Contains(t, failures[0], "smb@node-b: marker r-r1-m45000")

	byHost := messagesByHost(append(slices.Clone(both), record("node-c", 1)), hosts)
	assert.Len(t, byHost, 2)
	assert.Len(t, byHost["node-a"], 3)
	assert.Equal(t, "aks-system-1-vmss000000", lastLine("2026-10-06 12:00:00 UTC | CORE | WARN | something\naks-system-1-vmss000000\n\n"))
}

func TestWindowsCellIsGatedByItsOwnOptIn(t *testing.T) {
	for _, opts := range []runOptions{
		{cells: windowsCellName},
		{cells: windowsCellName, smbEnabled: true},
		{cells: windowsCellName, windowsEnabled: true},
	} {
		spec := testRunSpec(t, opts)
		assert.Empty(t, spec.cells)
		require.Len(t, spec.gatedCells, 1)
		reason := gateReason(spec.gatedCells[0])
		assert.Contains(t, reason, smbOptIn+"=1")
		assert.Contains(t, reason, windowsOptIn+"=1")
		_, ok := spec.windowsCell()
		assert.False(t, ok)
	}
	// Not in the default matrix, even with both opt-ins: it only runs when named.
	_, ok := testRunSpec(t, runOptions{smbEnabled: true, windowsEnabled: true}).windowsCell()
	assert.False(t, ok)
	assert.NotContains(t, gateReason(cell{name: "smb", reader: smbReader}), windowsOptIn)
}

// windowsTestSpec is a run with the smb and smb-windows cells, after the
// storage pass gave the Windows file server's address.
func windowsTestSpec(t *testing.T) (runSpec, cell) {
	t.Helper()
	spec := testRunSpec(t, runOptions{cells: "smb," + windowsCellName, smbEnabled: true, windowsEnabled: true})
	spec.setWindowsServerHost("10.1.2.3")
	c, ok := spec.windowsCell()
	require.True(t, ok)
	return spec, c
}

func TestWindowsCellReadsTheShareOfTheVM(t *testing.T) {
	spec := testRunSpec(t, runOptions{cells: "smb," + windowsCellName, smbEnabled: true, windowsEnabled: true})
	c, ok := spec.windowsCell()
	require.True(t, ok)
	assert.True(t, c.onWindows())
	assert.Equal(t, smbReader, c.reader)
	assert.Empty(t, c.accountName, "the share is on the VM, not in a storage account")
	assert.Regexp(t, `^smbwin-[0-9a-f]{12}$`, c.shareName)
	assert.Equal(t, markerDelaysFor(smbReader, writerOptions{mode: renameRotation}), c.markers)
	assert.Equal(t, defaultWriterOptions(), c.writer)
	assert.Empty(t, c.host(), "the address comes from the storage pass")

	spec, c = windowsTestSpec(t)
	assert.Equal(t, "10.1.2.3", c.host())
	assert.Equal(t, windowsReaderUser, c.smbUsername())
	smb, _ := spec.cellNamed("smb")
	assert.Empty(t, smb.serverHost)
	assert.Equal(t, smb.accountName, smb.smbUsername())
	assert.Equal(t, smb.accountName+".file.core.windows.net", smb.host())

	values := spec.agentHelmValues()
	assert.Contains(t, values, "      - type: smb\n        path: app.log\n        service: azure-files-smb-windows\n")
	assert.Contains(t, values, "        smb:\n          host: 10.1.2.3\n          share: "+c.shareName+"\n          username: ddlogreader\n"+
		"          password: \"ENC[file@/etc/azure-files-secrets/smb-windows/azurestorageaccountkey]\"\n")
	// The password reaches the Agent like a storage account key: a file of
	// the cell's Secret, copied into the Agent namespace.
	assert.Contains(t, values, "    - name: azure-files-smb-windows-key\n      secret:\n        secretName: azure-files-storage-smb-windows\n")
	assert.Contains(t, values, "    - name: azure-files-smb-windows-key\n      mountPath: /etc/azure-files-secrets/smb-windows\n      readOnly: true\n")
	assert.Equal(t, "smb://10.1.2.3/"+c.shareName+"/app.log", smbIdentifier(c, ""))
	assert.Equal(t, map[string]any{
		"address": "10.1.2.3", "share_dir": `C:\azure-files-e2e\shares\` + c.shareName, "username": windowsReaderUser,
		"signing": "required", "python_url": windowsPythonURL(), "python_hash": "MD5:" + windowsPythonHash, "scheduled_tasks": windowsTasks,
	}, windowsServerMetadata(c))

	// The writer runs its default options only, and no scenario runs with it.
	_, err := newRunSpec(runOptions{runID: testRunID, cells: windowsCellName, smbEnabled: true, windowsEnabled: true, periodMs: "10000"})
	assert.ErrorContains(t, err, "cell smb-windows: the Windows file server only runs the writer's default options")
	for _, scenario := range knownScenarios {
		_, err := newRunSpec(runOptions{runID: testRunID, scenario: string(scenario), cells: "smb," + windowsCellName, smbEnabled: true, windowsEnabled: true})
		assert.ErrorContains(t, err, windowsCellName, scenario)
	}
	// It can share a run with any other cell.
	all := testRunSpec(t, runOptions{cells: "file-line,smb,smb-gzip," + windowsCellName, smbEnabled: true, windowsEnabled: true})
	assert.Len(t, all.cells, 4)
}

func TestWindowsTasksRunTheStockWorkloadOnTheVM(t *testing.T) {
	spec, c := windowsTestSpec(t)
	files, err := spec.windowsFiles(c)
	require.NoError(t, err)
	assert.Len(t, files, 6)
	assert.Equal(t, pythonWriterSource, files[`C:\azure-files-e2e\workload\logwriter.py`])
	assert.Equal(t, sidecarsSource, files[`C:\azure-files-e2e\workload\sidecars.py`])
	assert.Equal(t, fileServerSource, files[`C:\azure-files-e2e\workload\fileserver.ps1`])

	shareDir := `C:\azure-files-e2e\shares\` + c.shareName
	python := `"C:\azure-files-e2e\python-3.12.10\python.exe" -u `
	writer := files[`C:\azure-files-e2e\run\writer.cmd`]
	// The writer pod's configuration, on the VM's paths.
	for _, v := range spec.writerEnv(c) {
		if v.name == "LOGWRITER_LOG_DIR" {
			v.value = shareDir
		}
		assert.Contains(t, writer, "set \""+v.name+"="+v.value+"\"\r\n")
	}
	assert.Contains(t, writer, "set \"HOSTNAME="+c.writerName+"\"\r\n")
	assert.True(t, strings.HasSuffix(writer, python+`"C:\azure-files-e2e\workload\logwriter.py" >> "C:\azure-files-e2e\logs\`+c.shareName+"-writer.log\" 2>&1\r\n"), writer)
	assert.NotContains(t, writer, logMountPath)

	ledger := files[`C:\azure-files-e2e\run\ledger.cmd`]
	assert.Contains(t, ledger, "set \"LOGWRITER_LOG_DIR="+shareDir+"\"\r\n")
	assert.Contains(t, ledger, "set \"LOGWRITER_LEDGER_SOURCE=journal\"\r\n")
	assert.True(t, strings.HasSuffix(ledger, python+`"C:\azure-files-e2e\workload\sidecars.py" ledger >> "C:\azure-files-e2e\logs\`+c.shareName+"-ledger.log\" 2>&1\r\n"), ledger)

	appender := files[`C:\azure-files-e2e\run\appender.cmd`]
	assert.Contains(t, appender, "set \"LOGWRITER_RUN_ID="+spec.writerRunID(c)+"\"\r\n")
	assert.Contains(t, appender, "set \"LOGWRITER_MARKER_JOURNAL_PATH="+shareDir+"\\markers.jsonl\"\r\n")
	assert.Contains(t, appender, "set \"LOGWRITER_APPEND_DELAYS_MS=500,45000\"\r\n")
	assert.True(t, strings.HasSuffix(appender, python+`"C:\azure-files-e2e\workload\sidecars.py" appender >> "C:\azure-files-e2e\logs\`+c.shareName+"-appender.log\" 2>&1\r\n"), appender)

	for name, script := range files {
		if !strings.HasSuffix(name, ".cmd") {
			continue
		}
		assert.True(t, strings.HasPrefix(script, "@echo off\r\n"), name)
		for _, line := range strings.SplitAfter(script, "\n") {
			if line != "" {
				assert.True(t, strings.HasSuffix(line, "\r\n"), "%s: %q", name, line)
			}
		}
	}

	// A value cmd.exe would expand or split is refused.
	bad := c
	bad.writerName = "writer-%PATH%"
	_, err = spec.windowsTaskScript(bad, writerContainerName)
	assert.ErrorContains(t, err, "the writer task's HOSTNAME cannot be set from cmd.exe")
	assert.Equal(t, "https://www.python.org/ftp/python/3.12.10/python-3.12.10-embed-amd64.zip", windowsPythonURL())
}

func TestFileServerCommandsCarryNoSecret(t *testing.T) {
	_, c := windowsTestSpec(t)
	script := `& 'C:\azure-files-e2e\workload\fileserver.ps1'`
	// The password goes in a file, which the script deletes once read.
	assert.Equal(t, script+` prepare -Root 'C:\azure-files-e2e' -ShareName '`+c.shareName+`' -UserName 'ddlogreader'`+
		` -PasswordFile 'C:\azure-files-e2e\secrets\reader-password' -PythonUrl '`+windowsPythonURL()+`'`+
		` -PythonHashAlgorithm 'MD5' -PythonHash 'fe8ef205f2e9c3ba44d0cf9954e1abd3' -PythonDll 'python312.dll'`+
		` -PythonDir 'C:\azure-files-e2e\python-3.12.10' -TaskPrefix 'azure-files-e2e'`, windowsPrepareCommand(c))
	assert.Equal(t, script+` protect -Root 'C:\azure-files-e2e'`, windowsProtectCommand())
	assert.Equal(t, `Remove-Item -LiteralPath 'C:\azure-files-e2e\secrets\reader-password' -Force -ErrorAction SilentlyContinue`, windowsForgetPasswordCommand())
	assert.Equal(t, script+` read -Root 'C:\azure-files-e2e' -Paths 'C:\azure-files-e2e\shares\`+c.shareName+`\ledger.jsonl',`+
		`'C:\azure-files-e2e\shares\`+c.shareName+`\markers.jsonl'`, windowsReadCommand(c, ledgerName, postRotationMarkerJournalName))
	assert.Equal(t, script+` read -Root 'C:\azure-files-e2e' -Paths 'C:\azure-files-e2e\logs\`+c.shareName+`-ledger.log'`,
		windowsLogsCommand(c, ledgerContainerName))

	assert.Contains(t, fileServerSource, "Remove-Item -LiteralPath $PasswordFile -Force")
	for number, line := range strings.Split(fileServerSource, "\n") {
		if strings.Contains(line, "$plain") || strings.Contains(line, "$password") {
			assert.NotRegexp(t, `Write-|Out-|\[Console\]|throw`, line, "fileserver.ps1 line %d may print the password", number+1)
		}
	}
	for _, action := range []string{"protect", "prepare", "start", "read", "sessions", "describe"} {
		assert.Contains(t, fileServerSource, "    '"+action+"' {\n", action)
	}
	// Signing is required of every session, not only offered.
	assert.Contains(t, fileServerSource, "Set-SmbServerConfiguration -RequireSecuritySignature $true")
	// The share is the reader's to read, nothing more.
	assert.Contains(t, fileServerSource, "New-SmbShare -Name $ShareName -Path $dir -ReadAccess $account")
	// The zip is refused before it is expanded unless it has the pinned
	// digest, then python.org's signature is required on the interpreter and
	// its DLLs.
	hashAt := strings.Index(fileServerSource, "Get-FileHash -LiteralPath $zip -Algorithm $PythonHashAlgorithm")
	expandAt := strings.Index(fileServerSource, "Expand-Archive")
	require.Positive(t, hashAt)
	assert.Less(t, hashAt, expandAt, "the digest is checked before the zip is expanded")
	assert.Contains(t, fileServerSource, "if ($digest -ne $PythonHash) {")
	assert.Contains(t, fileServerSource, "foreach ($name in @('python.exe', 'python3.dll', $PythonDll)) {")
	assert.Contains(t, fileServerSource, "Get-AuthenticodeSignature -FilePath $path")
	assert.Equal(t, "python"+strings.ReplaceAll(windowsPythonVersion[:strings.LastIndex(windowsPythonVersion, ".")], ".", "")+".dll", windowsPythonDLL)
	assert.Regexp(t, `^[0-9a-f]{32}$`, windowsPythonHash, "an MD5 digest, as python.org publishes it")
	// prepare reads and deletes the password before anything that can fail,
	// and deletes the file again whatever happens.
	prepare := fileServerSource[strings.Index(fileServerSource, "    'prepare' {\n"):]
	assert.Less(t, strings.Index(prepare, "$password = Read-ReaderPassword"), strings.Index(prepare, "Stop-Workload"))
	assert.Contains(t, prepare, "} finally {\n            Remove-Item -LiteralPath $PasswordFile -Force -ErrorAction SilentlyContinue")
	// Only the local subnet, where the AKS nodes are, reaches the share,
	// and a kept VM's rule is narrowed too.
	assert.Equal(t, 2, strings.Count(fileServerSource, "-RemoteAddress LocalSubnet"))
	assert.NotContains(t, fileServerSource, "-RemoteAddress Any")
	assert.Contains(t, fileServerSource, "Set-NetFirewallRule -Name $rule")
	// The secrets directory inherits nothing before the password goes there.
	assert.Contains(t, fileServerSource, "/inheritance:r")
}

func TestWindowsServerStateIsDecoded(t *testing.T) {
	single, err := parseWindowsServerState(`{"require_security_signature":true,"encrypt_data":false,"sessions":` +
		`{"ClientComputerName":"10.224.0.4","ClientUserName":"AZ-SMBWIN\\ddlogreader","Dialect":"3.1.1","NumOpens":2,"SecondsExists":40}}`)
	require.NoError(t, err)
	assert.True(t, single.RequireSecuritySignature)
	assert.Equal(t, []windowsSession{{ClientComputerName: "10.224.0.4", ClientUserName: `AZ-SMBWIN\ddlogreader`, Dialect: "3.1.1", NumOpens: 2, SecondsExists: 40}}, single.Sessions)

	several, err := parseWindowsServerState("\r\n" + `{"require_security_signature":true,"encrypt_data":false,"sessions":[{"Dialect":"3.1.1"},{"Dialect":"3.0.2"}]}` + "\r\n")
	require.NoError(t, err)
	assert.Len(t, several.Sessions, 2)
	for _, empty := range []string{`[]`, `null`} {
		none, err := parseWindowsServerState(`{"require_security_signature":false,"encrypt_data":false,"sessions":` + empty + `}`)
		require.NoError(t, err)
		assert.False(t, none.RequireSecuritySignature)
		assert.Empty(t, none.Sessions)
	}
	_, err = parseWindowsServerState("not json")
	assert.Error(t, err)
}

func TestCalibrationOnlyRelaxesTheProbeMarker(t *testing.T) {
	// Left set for a run without a probe cell, it is refused rather than
	// silently turning marker assertions off.
	for _, cells := range []string{"", "smb", "smb,smb-gzip"} {
		_, err := newRunSpec(runOptions{runID: testRunID, cells: cells, smbEnabled: true, calibrate: true})
		assert.ErrorContains(t, err, "only records the probe marker of the smb-late-<age> cells", cells)
	}
	spec := testRunSpec(t, runOptions{cells: "smb,smb-late-2000", smbEnabled: true, calibrate: true})
	smb, probe := spec.cells[0], spec.cells[1]
	markers := []markerEntry{
		{MarkerID: "probe", MarkerAgeMs: 2000, RotatedFile: "app.log.1"},
		{MarkerID: "lost", MarkerAgeMs: postRotationMarkerLostDelayMs, RotatedFile: "app.log.1"},
	}
	assert.Equal(t, markers[1:], markersToAssert(probe, markers, true))
	assert.Equal(t, markers, markersToAssert(probe, markers, false))
	assert.Equal(t, markers, markersToAssert(smb, markers, true))

	// The 45s marker still fails a calibration run when it survives.
	survived := new(recordingT)
	assertMarkerOutcome(survived, probe, markersToAssert(probe, markers, true), map[string]int{"probe": 1, "lost": 1})
	require.Len(t, survived.failures, 1)
	assert.Contains(t, survived.failures[0], "marker lost was appended")
}

func TestForcedLossesNeedALossAccountedPacedCell(t *testing.T) {
	_, err := newRunSpec(runOptions{runID: testRunID, cells: "smb-gzip", smbEnabled: true, forceLoss: true})
	assert.ErrorContains(t, err, "cell smb-gzip: forced losses need a paced writer")
	_, err = newRunSpec(runOptions{runID: testRunID, cells: "smb", smbEnabled: true, forceLoss: true, rateBytesPerSec: "200000", periodMs: "10000"})
	assert.ErrorContains(t, err, "only applies to the loss-accounted cells")

	spec := testRunSpec(t, runOptions{cells: "smb-gzip,smb-delete-recreate,smb", smbEnabled: true, forceLoss: true, rateBytesPerSec: "200000", periodMs: "10000"})
	assert.True(t, spec.forceLoss())
	gz, dr, smb := spec.cells[0], spec.cells[1], spec.cells[2]
	assert.True(t, gz.writer.forceLoss)
	assert.True(t, dr.writer.forceLoss)
	assert.False(t, smb.writer.forceLoss, "the smb cell is not loss-accounted")
	assert.Contains(t, spec.writerEnv(gz), envVar{"LOGWRITER_GZIP_DELAY_MS", "0"})
	assert.Contains(t, spec.writerEnv(dr), envVar{"LOGWRITER_DELETE_PAUSE_MS", "0"})
	assert.Equal(t, true, gz.writer.metadata()["force_loss"])
	// The rotated file is compressed away before any marker could land.
	assert.Equal(t, markerDelays{}, gz.markers)
	assert.Contains(t, spec.appenderEnv(gz), envVar{"LOGWRITER_APPEND_DELAYS_MS", "none"})

	// Without it, the pauses are the usual ones.
	usual := testRunSpec(t, runOptions{cells: "smb-gzip,smb-delete-recreate", smbEnabled: true})
	assert.False(t, usual.forceLoss())
	assert.Contains(t, usual.writerEnv(usual.cells[0]), envVar{"LOGWRITER_GZIP_DELAY_MS", "5000"})
	assert.Contains(t, usual.writerEnv(usual.cells[1]), envVar{"LOGWRITER_DELETE_PAUSE_MS", "1000"})
}

func TestLossBoundsFollowTheWriter(t *testing.T) {
	// The Java schedule leaves nothing to lose, and a report must match.
	assert.Zero(t, maxLostRecords(defaultWriterOptions()))
	assert.Equal(t, int64(lossAccountingToleranceBytes), maxReportShortfallBytes(defaultWriterOptions()))
	// 200 kB/s shared by two streams: 100 kB/s each over a poll interval
	// and a scan.
	paced := writerOptions{mode: gzipRotation, periodMs: 10000, rateBytesPerSec: 200000, streams: 2}
	assert.Equal(t, int64(200000), lossWindowBytes(paced))
	assert.Equal(t, int(math.Ceil(200000.0/1024))+64, maxLostRecords(paced))
	assert.Equal(t, int64(lossAccountingToleranceBytes+200000+pacedWriterBufferBytes), maxReportShortfallBytes(paced))
	// A drain ends at its second idle poll, long before a gzip rotation
	// compresses the file.
	assert.Greater(t, gzipDelayMs, (smbDrainIdlePolls+lossScanMarginSeconds)*smbPollIntervalSeconds*1000)
}

func TestLedgerGapsAreFound(t *testing.T) {
	entry := func(stream, period string, first, last int64) ledgerEntry {
		return ledgerEntry{RunID: stream, Period: period, File: "app.log." + period, FirstSequence: first, LastSequence: last}
	}
	assert.Empty(t, ledgerGaps([]ledgerEntry{
		entry("s1", "p2", 11, 20), entry("s1", "p1", 1, 10), entry("s2", "p1", 1, 5), entry("s1", "p3", 21, 30),
	}))
	gaps := ledgerGaps([]ledgerEntry{entry("s1", "p2", 11, 20), entry("s1", "p4", 31, 40), entry("s2", "p2", 6, 9)})
	require.Len(t, gaps, 3)
	assert.Equal(t, "s1: its first file, app.log.p2 of period p2, starts at sequence 11, not at the writer's first sequence 1: sequences 1-10 are in no asserted file", gaps[0])
	assert.Equal(t, "s1: app.log.p4 of period p4 starts at sequence 31, but the previous file ended at 20: sequences 21-30 are in no asserted file", gaps[1])
	assert.Contains(t, gaps[2], "s2: its first file")
	restarted := ledgerGaps([]ledgerEntry{entry("s1", "p1", 1, 10), entry("s1", "p2", 1, 8)})
	require.Len(t, restarted, 1)
	assert.Contains(t, restarted[0], "inside the previous file, which ended at 10: the writer restarted")
}

func TestUnwrittenRecordsFailTheCellExceptAfterARecreate(t *testing.T) {
	entry := func(unwritten ...[2]int64) ledgerEntry {
		return ledgerEntry{RunID: "r", Period: "p2", File: activeLogName, FirstSequence: 11, LastSequence: 20, UnwrittenSequences: unwritten}
	}
	renamed := cell{name: "smb", reader: smbReader, writer: writerOptions{mode: renameRotation, periodMs: 60000}}
	recreated := cell{name: "smb-delete-recreate", reader: smbReader, writer: writerOptions{mode: deleteRecreateRotation, periodMs: 60000}}
	spec := testRunSpec(t, runOptions{})
	delay := spec.deleteRecreateDelaySeconds(recreated)
	assert.Equal(t, 1, delay)

	assert.Empty(t, unwrittenProblems(renamed, []ledgerEntry{entry()}, delay))
	problems := unwrittenProblems(renamed, []ledgerEntry{entry([2]int64{15, 15})}, delay)
	require.Len(t, problems, 1)
	assert.Contains(t, problems[0], "failed to write 1 records of app.log of r (period p2, sequences 11-20) (15-15); only a delete-recreate rotation")

	// The head record and a few ticks after a recreate are allowed.
	assert.Equal(t, int64(5), deleteRecreateUnwrittenBound(recreated.writer, delay))
	assert.Empty(t, unwrittenProblems(recreated, []ledgerEntry{entry([2]int64{11, 13})}, delay))
	tooMany := unwrittenProblems(recreated, []ledgerEntry{entry([2]int64{11, 16})}, delay)
	require.Len(t, tooMany, 1)
	assert.Contains(t, tooMany[0], "the first 6 records of app.log of r (period p2, sequences 11-20), more than the 5 it can lose in the 1s its smb reader may keep the deleted file open")
	inside := unwrittenProblems(recreated, []ledgerEntry{entry([2]int64{14, 15})}, delay)
	require.Len(t, inside, 1)
	assert.Contains(t, inside[0], "not one run at its start")
	// A file with no record written fails in every mode.
	for _, c := range []cell{renamed, recreated} {
		none := unwrittenProblems(c, []ledgerEntry{entry([2]int64{11, 20})}, delay)
		require.Len(t, none, 1, c.name)
		assert.Contains(t, none[0], "the writer wrote none of the 10 records", c.name)
	}
	// A paced stream loses what it writes in that time, and one write.
	paced := writerOptions{mode: deleteRecreateRotation, periodMs: 10000, rateBytesPerSec: 20000}
	assert.Equal(t, int64(1+int(math.Ceil(20000.0/1024))+64), deleteRecreateUnwrittenBound(paced, 1))
	// A file source keeps the deleted file open until its rotated tailer
	// closes, longer with unreliable_mount.
	file := cell{name: "file-line", reader: fileReader}
	assert.Equal(t, fileScanPeriodSeconds+closeTimeoutSeconds+1, spec.deleteRecreateDelaySeconds(file))
	unreliable := testRunSpec(t, runOptions{profile: "unreliable-mount"})
	assert.Equal(t, fileScanPeriodSeconds+fileHandoffQuietSeconds+1, unreliable.deleteRecreateDelaySeconds(file))
	assert.Equal(t, 1, unreliable.deleteRecreateDelaySeconds(recreated))
}

func TestNetworkDropFitsTheLongestPeriod(t *testing.T) {
	assert.Equal(t, 510, maxNetworkDropSeconds)
	spec := testRunSpec(t, runOptions{scenario: "network-drop", networkDropSeconds: strconv.Itoa(maxNetworkDropSeconds), smbEnabled: true})
	assert.Equal(t, maxWriterPeriodMs, spec.writer.periodMs)
	assert.NoError(t, spec.writer.validate())
	// A default that would leave the range is refused, not run.
	assert.ErrorContains(t, writerOptions{mode: renameRotation, periodMs: maxWriterPeriodMs + 1000}.validate(), "must be between 5000 and 600000")
}

func TestCopyTruncateAtRiskWindowFollowsTheReader(t *testing.T) {
	smb := testRunSpec(t, runOptions{cells: "smb-copytruncate", smbEnabled: true})
	c := smb.cells[0]
	assert.Contains(t, smb.writerEnv(c), envVar{"LOGWRITER_COPYTRUNCATE_AT_RISK_MS", "1500"})
	assert.Equal(t, copyTruncateSMBAtRiskMs, c.copyTruncateAtRiskMs())
	// Longer than a poll, so a source that keeps up reads every record
	// written before it; shorter than the hold, so the rest is checked.
	assert.Greater(t, copyTruncateSMBAtRiskMs, smbPollIntervalSeconds*1000)
	assert.Less(t, copyTruncateSMBAtRiskMs, copyTruncateHoldMs)

	file := testRunSpec(t, runOptions{cells: "file-line", rotationMode: "copytruncate"})
	assert.Equal(t, copyTruncateHoldMs, file.cells[0].copyTruncateAtRiskMs())
	for _, v := range file.writerEnv(file.cells[0]) {
		assert.NotEqual(t, "LOGWRITER_COPYTRUNCATE_AT_RISK_MS", v.name)
	}
}

func TestPacedWritersIdleAfterTheirPeriods(t *testing.T) {
	// The files a cell waits for, the period that rotates the last of them,
	// and two to spare.
	assert.Equal(t, completedFiles+3, pacedWriterMaxPeriods)
	paced := testRunSpec(t, runOptions{cells: "smb", smbEnabled: true, rateBytesPerSec: "20000"})
	assert.Contains(t, paced.writerEnv(paced.cells[0]), envVar{"LOGWRITER_MAX_PERIODS", "7"})
	assert.Equal(t, pacedWriterMaxPeriods, paced.cells[0].writer.metadata()["max_periods"])
	java := testRunSpec(t, runOptions{cells: "smb", smbEnabled: true})
	for _, v := range java.writerEnv(java.cells[0]) {
		assert.NotEqual(t, "LOGWRITER_MAX_PERIODS", v.name)
	}
}
