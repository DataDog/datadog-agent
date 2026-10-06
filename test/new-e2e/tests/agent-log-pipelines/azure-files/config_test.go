// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package azurefiles

import (
	"archive/zip"
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"testing"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		assert.Equal(t, markerDelaysFor(c.reader), c.markers, c.name)
		// The appender appends inline, so it must be done before the next
		// rotation.
		assert.Less(t, c.markers.lostMs, writerRotationSeconds*1000, c.name)
	}

	// A file source reads the rotated file for at least close_timeout after it
	// sees the rotation, which cannot come before the rename.
	file := markerDelaysFor(fileReader)
	assert.Less(t, file.survivingMs, closeTimeoutSeconds*1000)

	// The longest file source drain at actimeo=1 is the unreliable-mount
	// handoff: it sees the rotation after a scan and an attribute cache
	// refresh, reads the surviving marker, then waits out its quiet period.
	longestFileDrainMs := (fileScanPeriodSeconds+1)*1000 + file.survivingMs + fileHandoffQuietSeconds*1000
	assert.Greater(t, file.lostMs, longestFileDrainMs)
	assert.Greater(t, file.lostMs, (fileScanPeriodSeconds+1+closeTimeoutSeconds)*1000)

	// The SMB source can drop an idle rotated file at the first poll after the
	// scan that saw the rotation, and that scan can come right after the
	// rename. The appender notices the rename up to one of its polls late, so
	// the surviving marker must land within a poll interval even then.
	smb := markerDelaysFor(smbReader)
	assert.Less(t, smb.survivingMs+postRotationMarkerPollMs, (smbDrainIdlePolls-1)*smbPollIntervalSeconds*1000)
	// It sees the rotation within a poll of the rename and stops at
	// close_timeout at the latest, however often new data restarts its idle
	// polls.
	assert.Greater(t, smb.lostMs, (smbPollIntervalSeconds+closeTimeoutSeconds)*1000)

	assert.Equal(t, "1500,45000", file.appenderValue())
	assert.Equal(t, "500,45000", smb.appenderValue())
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
	assert.Equal(t, []envVar{
		{"LOGWRITER_LOG_DIR", logMountPath},
		{"LOGWRITER_CRC64_COMMAND", "python3 /app/logwriter.py crc64"},
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
	assert.Equal(t, map[string]any{"workload": customWorkload, "image": testWriterImage}, custom)
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
	for _, reader := range []readerKind{fileReader, smbReader} {
		c := cell{name: string(reader), reader: reader, markers: markerDelaysFor(reader)}
		markers := []markerEntry{
			{MarkerID: "early", MarkerAgeMs: c.markers.survivingMs, RotatedFile: "app.log.a"},
			{MarkerID: "late", MarkerAgeMs: c.markers.lostMs, RotatedFile: "app.log.a"},
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
	smb := cell{name: "smb", reader: smbReader, markers: markerDelaysFor(smbReader)}
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
