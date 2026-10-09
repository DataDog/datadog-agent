// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package azurefiles

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// Scenarios disrupt the Agent while the smb cell's writer runs, then hold the
// cell to the usual exactly-once check, with what the disruption allows. Each
// is its own test method, selected with AZURE_FILES_E2E_SCENARIO; the others
// skip. See the scenario constants in provisioner.go for their timings.

const (
	// logsRunPath is logs_config.run_path, where the registry lives.
	logsRunPath      = "/opt/datadog-agent/run"
	agentContainer   = "agent"
	helperRoleLabel  = "e2e.datadoghq.com/role"
	networkHelperApp = "network-helper"
	// statusPollInterval is how often a scenario records the source status.
	statusPollInterval = 5 * time.Second
	// smbPort is the port the SMB source connects to (smb.port's default).
	smbPort = 445
)

// scenarioMetadata describes the run's scenario for the run metadata.
func (spec runSpec) scenarioMetadata() map[string]any {
	if spec.scenario.kind == noScenario {
		return nil
	}
	metadata := map[string]any{"name": spec.scenario.kind, "cell": scenarioCellName, "min_period_ms": spec.scenario.minPeriodMs()}
	switch spec.scenario.kind {
	case agentRestartScenario:
		metadata["delete_after_ms"] = agentRestartDeleteAfterMs
	case keyRotationScenario:
		metadata["renew_after_ms"] = keyRotationStartAfterMs
		metadata["secret_refresh_interval_seconds"] = secretRefreshIntervalSeconds
		metadata["forced_drop_seconds"] = keyRotationForcedDropSeconds
	case networkDropScenario:
		metadata["drop_seconds"] = spec.scenario.dropSeconds
	}
	return metadata
}

// scenarioCell skips unless the run selected the scenario, and returns the
// cell it disrupts.
func (suite *azureFilesSuite) scenarioCell(kind scenarioKind) cell {
	suite.T().Helper()
	if suite.spec.scenario.kind != kind {
		suite.T().Skipf("set %s=%s to run this scenario", runScenario, kind)
	}
	c, ok := suite.spec.cellNamed(scenarioCellName)
	require.True(suite.T(), ok, "the %s scenario needs the %s cell", kind, scenarioCellName)
	return c
}

// sleepUntil sleeps until at, if it is still ahead.
func sleepUntil(at time.Time) {
	if wait := time.Until(at); wait > 0 {
		time.Sleep(wait)
	}
}

// periodStart is the start of the writer period that at falls in. Periods
// start at multiples of the period since the epoch, as logwriter.py aligns
// them.
func periodStart(at time.Time, periodMs int) time.Time {
	ms := at.UnixMilli()
	return time.UnixMilli(ms - ms%int64(periodMs)).UTC()
}

// disruptionTiming returns when a disruption that starts offset into a period
// should start, and when that period ends: in the current period if there is
// still time, else in the next one.
func disruptionTiming(now time.Time, periodMs int, offset time.Duration) (start, periodEnd time.Time) {
	period := time.Duration(periodMs) * time.Millisecond
	begin := periodStart(now, periodMs)
	if now.After(begin.Add(offset)) {
		begin = begin.Add(period)
	}
	return begin.Add(offset), begin.Add(period)
}

// networkDropTiming centres a drop of dropSeconds on the end of the period, so
// the file rotates while the source cannot reach the share.
func networkDropTiming(now time.Time, periodMs, dropSeconds int) (block, rotation time.Time) {
	half := time.Duration(dropSeconds) * time.Second / 2
	period := time.Duration(periodMs) * time.Millisecond
	rotation = periodStart(now, periodMs).Add(period)
	if now.After(rotation.Add(-half)) {
		rotation = rotation.Add(period)
	}
	return rotation.Add(-half), rotation
}

// writeScenarioEvidence keeps a scenario's record, which holds no secret.
func (suite *azureFilesSuite) writeScenarioEvidence(name string, value any) {
	if evidence, err := suite.evidenceDir(); err == nil {
		evidence.writeJSON(string(suite.spec.scenario.kind)+"-"+name+".json", value)
	}
}

// Agent restart.

func (suite *azureFilesSuite) TestAgentRestartScenario() {
	c := suite.scenarioCell(agentRestartScenario)
	suite.installAgent()
	defer suite.captureEvidence()
	suite.requireAgentReady()
	rule := suite.restartAgentMidPeriod(c)
	suite.checkCell(c, recordRules{restart: &rule})
}

// restartAgentMidPeriod deletes the Agent pod once the smb source has read
// part of the second period's file, and waits for the DaemonSet's
// replacement to run the source again before the period ends. It returns
// what the restart may resend.
func (suite *azureFilesSuite) restartAgentMidPeriod(c cell) restartRule {
	t := suite.T()
	pods, err := suite.agentPods()
	require.NoError(t, err)
	require.Len(t, pods, 1, "the restart scenario expects the one Agent pod of the one AKS node")
	old := pods[0]
	// The SMB source resumes from the registry: if a pod deletion emptied it,
	// every file would be read again from its start.
	require.NoError(t, checkRegistryPersists(old))
	suite.requireSMBSourceRunning(c)
	writer := suite.writerPod(c)
	suite.waitForLedger(c, writer, 1)

	deleteAt, periodEnd := disruptionTiming(time.Now(), c.writer.periodMs, agentRestartDeleteAfterMs*time.Millisecond)
	sleepUntil(deleteAt)
	keys, err := suite.cellKeys(c)
	require.NoError(t, err)
	var bytesRead int64
	suite.EventuallyWithT(func(collect *assert.CollectT) {
		var err error
		bytesRead, err = suite.activeTailerBytesRead(c, old, keys)
		require.NoError(collect, err)
		assert.Positive(collect, bytesRead, "the smb source has not read the active file yet")
	}, time.Minute, 2*time.Second)
	registry, _, registryErr := suite.Env().KubernetesCluster.KubernetesClient.PodExec(agentNamespace, old.Name, agentContainer,
		[]string{"cat", logsRunPath + "/registry.json"})
	require.NoError(t, registryErr, "read the registry of %s", old.Name)
	require.Contains(t, registry, smbIdentifier(c, ""), "the registry of %s has no offset for the active file yet", old.Name)

	// The old pod's log ends with its shutdown, which says whether the logs
	// agent stopped within its grace period.
	shutdownLog := suite.followAgentLog(old)
	deletedAt := time.Now()
	require.NoError(t, suite.Env().KubernetesCluster.Client().CoreV1().Pods(agentNamespace).
		Delete(context.Background(), old.Name, metav1.DeleteOptions{}))
	log, logErr := shutdownLog(3 * time.Minute)
	replacement := suite.waitForReplacementAgent(old.UID)
	suite.requireSMBSourceRunning(c)
	resumedAt := time.Now()

	graceful := logErr == nil && gracefulLogsStop(log)
	rule := restartRule{bound: restartDuplicateBound(c.writer, graceful), graceful: graceful}
	record := map[string]any{
		"old_pod": old.Name, "new_pod": replacement.Name,
		"deleted_at": deletedAt, "source_running_again_at": resumedAt, "period_end": periodEnd,
		"bytes_read_by_active_tailer_before_delete": bytesRead,
		"graceful_logs_stop":                        graceful, "duplicate_bound_records": rule.bound,
	}
	if logErr != nil {
		record["shutdown_log_error"] = logErr.Error()
	}
	suite.writeScenarioEvidence("restart", record)
	if evidence, err := suite.evidenceDir(); err == nil && evidence.redactsEverySecret() {
		evidence.write(old.Name+"-agent-shutdown.log", []byte(log))
	}
	t.Logf("%s: deleted %s %s into its period after the source read %d bytes of the active file; %s ran the source again %s later; graceful logs-agent stop: %t, so up to %d records may be collected twice",
		c.name, old.Name, deletedAt.Sub(periodStart(deletedAt, c.writer.periodMs)).Round(time.Second), bytesRead,
		replacement.Name, resumedAt.Sub(deletedAt).Round(time.Second), graceful, rule.bound)
	// A file that rotates while no Agent runs is never read past its
	// registry offset (see README.md), which says nothing about the restart.
	require.True(t, resumedAt.Before(periodEnd),
		"the replacement Agent ran the smb source again only %s after the period ended, so the file rotated while no Agent ran; raise %s",
		resumedAt.Sub(periodEnd).Round(time.Second), runPeriodMs)
	return rule
}

// checkRegistryPersists requires the core Agent's registry directory to be a
// hostPath volume, which outlives the pod. The Datadog chart mounts its
// pointerdir volume, <hostMountRoot>/logs on the node (/var/lib/datadog-agent/logs),
// there whenever datadog.logs.enabled is set.
func checkRegistryPersists(pod corev1.Pod) error {
	var volumeName string
	for _, container := range pod.Spec.Containers {
		if container.Name != agentContainer {
			continue
		}
		for _, mount := range container.VolumeMounts {
			if mount.MountPath == logsRunPath {
				volumeName = mount.Name
			}
		}
	}
	if volumeName == "" {
		return fmt.Errorf("the agent container of %s mounts nothing at %s, so its registry does not outlive the pod; mount a hostPath there", pod.Name, logsRunPath)
	}
	for _, volume := range pod.Spec.Volumes {
		if volume.Name != volumeName {
			continue
		}
		if volume.HostPath == nil {
			return fmt.Errorf("the registry of %s is on volume %s, which is not a hostPath and does not outlive the pod; mount a hostPath at %s (the chart does it when datadog.logs.enabled is set)", pod.Name, volumeName, logsRunPath)
		}
		return nil
	}
	return fmt.Errorf("the agent container of %s mounts volume %s, which its pod does not define", pod.Name, volumeName)
}

// gracefulLogsStop reports whether an Agent's log shows its logs agent stopped
// without hitting logs_config.stop_grace_period (comp/logs/agent/impl/agent.go).
func gracefulLogsStop(log string) bool {
	return strings.Contains(log, "logs-agent stopped") && !strings.Contains(log, "Timed out when stopping logs-agent")
}

// activeTailerBytesRead returns what the tailer of the cell's active app.log
// has read, from the verbose status.
func (suite *azureFilesSuite) activeTailerBytesRead(c cell, pod corev1.Pod, keys []string) (int64, error) {
	stdout, stderr, err := suite.Env().KubernetesCluster.KubernetesClient.PodExec(agentNamespace, pod.Name, agentContainer,
		strings.Fields(verboseStatusJSONCommand))
	if err != nil {
		return 0, fmt.Errorf("%s on %s: %w (stderr: %s)", verboseStatusJSONCommand, pod.Name, err, redactSecrets(strings.TrimSpace(stderr), keys...))
	}
	status, err := decodeAgentStatus(stdout)
	if err != nil {
		return 0, err
	}
	for _, tailer := range status.smbTailers(c) {
		if tailer.ID != smbIdentifier(c, "") {
			continue // a draining rotated file
		}
		values := tailer.Info[bytesReadInfoKey]
		if len(values) == 0 {
			return 0, fmt.Errorf("the tailer of %s has no %s", tailer.ID, bytesReadInfoKey)
		}
		return strconv.ParseInt(values[0], 10, 64)
	}
	return 0, fmt.Errorf("the status of %s lists no tailer for %s", pod.Name, smbIdentifier(c, ""))
}

// followAgentLog streams the core Agent's log of pod until the container
// exits. The returned function waits up to timeout for the end of the stream
// and returns what it read.
func (suite *azureFilesSuite) followAgentLog(pod corev1.Pod) func(time.Duration) (string, error) {
	ctx, cancel := context.WithCancel(context.Background())
	since := int64(10)
	stream, err := suite.Env().KubernetesCluster.Client().CoreV1().Pods(agentNamespace).
		GetLogs(pod.Name, &corev1.PodLogOptions{Container: agentContainer, Follow: true, SinceSeconds: &since}).Stream(ctx)
	if err != nil {
		cancel()
		return func(time.Duration) (string, error) { return "", fmt.Errorf("follow the log of %s: %w", pod.Name, err) }
	}
	var buffer bytes.Buffer
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(&buffer, stream)
		_ = stream.Close()
		done <- err
	}()
	return func(timeout time.Duration) (string, error) {
		var err error
		select {
		case err = <-done:
		case <-time.After(timeout):
			cancel()
			<-done
			err = fmt.Errorf("the log of %s did not end within %s", pod.Name, timeout)
		}
		cancel()
		return buffer.String(), err
	}
}

// waitForReplacementAgent waits for an Agent pod other than the deleted one
// to be ready.
func (suite *azureFilesSuite) waitForReplacementAgent(deleted types.UID) corev1.Pod {
	suite.T().Helper()
	var replacement corev1.Pod
	suite.EventuallyWithT(func(collect *assert.CollectT) {
		pods, err := suite.agentPods()
		require.NoError(collect, err)
		found := false
		for _, pod := range pods {
			// Every later check reads every Agent pod, so the deleted one
			// must be gone, not only terminating.
			if pod.UID == deleted {
				assert.Fail(collect, "the deleted Agent pod is still there", "%s is still listed (%s)", pod.Name, pod.Status.Phase)
				continue
			}
			found = true
			replacement = pod
			assert.True(collect, agentContainerReady(pod), "the replacement Agent %s is not ready: %s", pod.Name, suite.podProblems(pod))
		}
		assert.True(collect, found, "the DaemonSet has not replaced the deleted Agent pod yet")
	}, 5*time.Minute, 5*time.Second)
	return replacement
}

// Key rotation.

func (suite *azureFilesSuite) TestKeyRotationScenario() {
	c := suite.scenarioCell(keyRotationScenario)
	suite.installAgent()
	defer suite.captureEvidence()
	suite.requireAgentReady()
	suite.rotateAgentKey(c)
	suite.checkCell(c, recordRules{})
}

// rotateAgentKey renews key2, which the Agent reads the share with, while the
// writer keeps writing with key1. The Agent must fail to authenticate, then
// recover once its Secret holds the new key2 and the secret refresh picked it
// up, within the period.
func (suite *azureFilesSuite) rotateAgentKey(c cell) {
	t := suite.T()
	writerKey, err := suite.accountKey(c)
	require.NoError(t, err)
	agentKey, err := suite.agentAccountKey(c)
	require.NoError(t, err)
	// Compared without testify's equality helpers, which print both values.
	require.True(t, writerKey != agentKey, "the Agent's %s Secret must hold key2 while the writer mounts with key1; it holds the writer's key", c.secretName())
	account, err := suite.storageAccount(c)
	require.NoError(t, err)
	_, err = exec.LookPath("az")
	require.NoError(t, err, "the key-rotation scenario renews key2 with the Azure CLI, which must be on PATH and logged in (az login)")
	suite.requireSMBSourceRunning(c)
	suite.waitForLedger(c, suite.writerPod(c), 1)

	renewAt, periodEnd := disruptionTiming(time.Now(), c.writer.periodMs, keyRotationStartAfterMs*time.Millisecond)
	sleepUntil(renewAt)
	watcher := suite.watchSMBStatus(c)
	defer func() { suite.writeScenarioEvidence("status-timeline", watcher.stop()) }()

	// The old key2 leaves the Secret below; it stays a key to redact and
	// to look for in the Agent's output.
	suite.addKnownKey(agentKey)
	renewedAt := time.Now()
	newKey := suite.renewSecondaryKey(account)
	require.True(t, newKey != agentKey, "the Azure CLI returned the same key2 after renewing it")

	// Azure may keep a session that authenticated with the old key2. If it
	// does, nothing fails until the client has to authenticate again, so the
	// scenario then drops the Agent's SMB traffic for longer than an
	// operation timeout, which makes the client replace its session.
	authSince := func(obs []statusObservation) int { return firstObservation(obs, 0, renewedAt, sourceAuthError) }
	forced := !watcher.waitFor(func(obs []statusObservation) bool { return authSince(obs) >= 0 }, keyRotationNaturalAuthWait)
	if forced {
		t.Logf("%s: no authentication error %s after key2 was renewed: Azure kept the session; dropping the Agent's SMB traffic for %ds so the client authenticates again",
			c.name, keyRotationNaturalAuthWait, keyRotationForcedDropSeconds)
		helper := suite.startNetworkHelper(c)
		defer helper.cleanup()
		helper.block()
		time.Sleep(keyRotationForcedDropSeconds * time.Second)
		helper.unblock()
	}
	require.True(t, watcher.waitFor(func(obs []statusObservation) bool { return authSince(obs) >= 0 }, 2*time.Minute),
		"%s: the source never reported an authentication error after key2 was renewed (status: %s)", c.name, watcher.last())
	authIndex := authSince(watcher.snapshot())
	authAt := watcher.snapshot()[authIndex].At

	suite.updateAgentKey(c, newKey)
	updatedAt := time.Now()
	suite.waitForMountedKey(c, newKey)
	mountedAt := time.Now()
	recovered := func(obs []statusObservation) int { return firstObservation(obs, authIndex+1, time.Time{}, sourceOK) }
	require.True(t, watcher.waitFor(func(obs []statusObservation) bool { return recovered(obs) >= 0 },
		time.Duration(3*secretRefreshIntervalSeconds)*time.Second+time.Minute),
		"%s: the source did not recover after the Agent's Secret got the new key2 (status: %s)", c.name, watcher.last())
	recoveredAt := watcher.snapshot()[recovered(watcher.snapshot())].At

	suite.writeScenarioEvidence("rotation", map[string]any{
		"renewed_at": renewedAt, "auth_error_at": authAt, "forced_reconnect": forced,
		"secret_updated_at": updatedAt, "secret_mounted_at": mountedAt, "recovered_at": recoveredAt, "period_end": periodEnd,
	})
	t.Logf("%s: key2 renewed; authentication error after %s (forced: %t); Secret updated, mounted %s later; source OK %s after the update",
		c.name, authAt.Sub(renewedAt).Round(time.Second), forced, mountedAt.Sub(updatedAt).Round(time.Second), recoveredAt.Sub(updatedAt).Round(time.Second))
	require.True(t, recoveredAt.Before(periodEnd),
		"the source recovered %s after the period ended, so the file rotated while it could not read; raise %s",
		recoveredAt.Sub(periodEnd).Round(time.Second), runPeriodMs)
}

// storageAccountRef addresses a storage account for the Azure CLI.
type storageAccountRef struct {
	subscription  string
	resourceGroup string
	name          string
}

var storageAccountIDPattern = regexp.MustCompile(`(?i)^/subscriptions/([^/]+)/resourceGroups/([^/]+)/providers/Microsoft\.Storage/storageAccounts/([^/]+)$`)

func parseStorageAccountID(id string) (storageAccountRef, error) {
	match := storageAccountIDPattern.FindStringSubmatch(strings.TrimSpace(id))
	if match == nil {
		return storageAccountRef{}, fmt.Errorf("%q is not a storage account ID", id)
	}
	return storageAccountRef{subscription: match[1], resourceGroup: match[2], name: match[3]}, nil
}

// renewKeyArgs renew key2 and print nothing: the command's output lists the
// keys.
func (a storageAccountRef) renewKeyArgs() []string {
	return []string{"storage", "account", "keys", "renew",
		"--subscription", a.subscription, "--resource-group", a.resourceGroup, "--account-name", a.name,
		"--key", "secondary", "--output", "none"}
}

// secondaryKeyArgs print key2 alone, which the test reads and never logs.
func (a storageAccountRef) secondaryKeyArgs() []string {
	return []string{"storage", "account", "keys", "list",
		"--subscription", a.subscription, "--resource-group", a.resourceGroup, "--account-name", a.name,
		"--query", "[?keyName=='key2'].value | [0]", "--output", "tsv"}
}

// storageAccount reads the cell's storage account from the storage pass's
// ConfigMap, which names its subscription and resource group.
func (suite *azureFilesSuite) storageAccount(c cell) (storageAccountRef, error) {
	configMap, err := suite.Env().KubernetesCluster.Client().CoreV1().ConfigMaps(e2eNamespace).
		Get(context.Background(), storageAccountsConfigMapName, metav1.GetOptions{})
	if err != nil {
		return storageAccountRef{}, fmt.Errorf("read ConfigMap %s/%s: %w", e2eNamespace, storageAccountsConfigMapName, err)
	}
	account, err := parseStorageAccountID(configMap.Data[c.name])
	if err != nil {
		return storageAccountRef{}, err
	}
	if !strings.EqualFold(account.name, c.accountName) {
		return storageAccountRef{}, fmt.Errorf("the stack's %s account is %s, not %s", c.name, account.name, c.accountName)
	}
	return account, nil
}

// renewSecondaryKey renews key2 and returns the new one, which every later
// redaction removes. Neither command's output reaches a log or a failure.
func (suite *azureFilesSuite) renewSecondaryKey(account storageAccountRef) string {
	t := suite.T()
	run := func(args []string) (string, error) {
		var stdout, stderr bytes.Buffer
		cmd := exec.Command("az", args...)
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("az %s: %w (stderr: %s)", strings.Join(args[:4], " "), err,
				redactSecrets(strings.TrimSpace(stderr.String()), append(suite.knownSecrets(), strings.TrimSpace(stdout.String()))...))
		}
		return strings.TrimSpace(stdout.String()), nil
	}
	_, err := run(account.renewKeyArgs())
	require.NoError(t, err)
	key, err := run(account.secondaryKeyArgs())
	require.NoError(t, err)
	require.True(t, key != "" && !strings.ContainsAny(key, " \n"), "the Azure CLI did not print one key2")
	suite.addKnownKey(key)
	return key
}

// updateAgentKey replaces the key of the cell's Secret copy in the Agent
// namespace, which the Agent pod mounts.
func (suite *azureFilesSuite) updateAgentKey(c cell, key string) {
	secrets := suite.Env().KubernetesCluster.Client().CoreV1().Secrets(agentNamespace)
	secret, err := secrets.Get(context.Background(), c.secretName(), metav1.GetOptions{})
	require.NoError(suite.T(), err)
	secret.Data[accountKeySecretKey] = []byte(key)
	_, err = secrets.Update(context.Background(), secret, metav1.UpdateOptions{})
	require.NoError(suite.T(), err, "update Secret %s/%s", agentNamespace, c.secretName())
}

// waitForMountedKey waits until the kubelet has written the new key into the
// Agent pod's Secret volume. It compares digests, so the key is never read
// out of the pod.
func (suite *azureFilesSuite) waitForMountedKey(c cell, key string) {
	sum := sha256.Sum256([]byte(key))
	want := hex.EncodeToString(sum[:])
	file := agentKeyDir(c) + "/" + accountKeySecretKey
	suite.EventuallyWithT(func(collect *assert.CollectT) {
		pods, err := suite.agentPods()
		require.NoError(collect, err)
		for _, pod := range pods {
			stdout, _, err := suite.Env().KubernetesCluster.KubernetesClient.PodExec(agentNamespace, pod.Name, agentContainer,
				[]string{"sha256sum", file})
			require.NoError(collect, err)
			fields := strings.Fields(stdout)
			assert.True(collect, len(fields) > 0 && fields[0] == want, "the kubelet has not updated %s in %s yet", file, pod.Name)
		}
	}, 3*time.Minute, 5*time.Second)
}

// Network drop.

func (suite *azureFilesSuite) TestNetworkDropScenario() {
	c := suite.scenarioCell(networkDropScenario)
	suite.installAgent()
	defer suite.captureEvidence()
	suite.requireAgentReady()
	window := suite.dropSMBAcrossRotation(c)
	suite.Run(c.name, func() {
		suite.checkCell(c, recordRules{})
		suite.requireWriterRotatedDuring(c, window)
	})
	// The file cells' kernel mounts are outside the blocked namespace.
	for _, other := range suite.spec.cells {
		if other.name == c.name {
			continue
		}
		suite.Run(other.name, func() {
			suite.checkCell(other, recordRules{})
		})
	}
}

// dropWindow is when the Agent's SMB traffic was dropped.
type dropWindow struct {
	blocked, unblocked time.Time
}

// dropSMBAcrossRotation drops the Agent pod's traffic to the smb cell's
// storage endpoint around the end of the second period, and requires the
// source to report an error during the drop and to recover after it.
func (suite *azureFilesSuite) dropSMBAcrossRotation(c cell) dropWindow {
	t := suite.T()
	suite.requireSMBSourceRunning(c)
	suite.waitForLedger(c, suite.writerPod(c), 1)
	// The helper takes a while to start, so it starts before the drop is
	// timed.
	helper := suite.startNetworkHelper(c)
	defer helper.cleanup()
	watcher := suite.watchSMBStatus(c)
	defer func() { suite.writeScenarioEvidence("status-timeline", watcher.stop()) }()
	blockAt, rotation := networkDropTiming(time.Now(), c.writer.periodMs, suite.spec.scenario.dropSeconds)
	sleepUntil(blockAt)
	helper.block()
	window := dropWindow{blocked: time.Now()}
	time.Sleep(time.Duration(suite.spec.scenario.dropSeconds) * time.Second)
	dropped := helper.droppedPackets()
	helper.unblock()
	window.unblocked = time.Now()

	require.Positive(t, dropped, "the DROP rule matched no packet: the Agent's SMB traffic did not go through it")
	failed := func(obs []statusObservation) int {
		return firstObservation(obs, 0, window.blocked, sourceUnreachable, sourceOtherError, sourceAuthError)
	}
	require.True(t, failed(watcher.snapshot()) >= 0,
		"%s: the source never reported an error while its traffic was dropped for %ds (status: %s)", c.name, suite.spec.scenario.dropSeconds, watcher.last())
	errorAt := watcher.snapshot()[failed(watcher.snapshot())].At
	recovered := func(obs []statusObservation) int {
		return firstObservation(obs, 0, window.unblocked, sourceOK)
	}
	require.True(t, watcher.waitFor(func(obs []statusObservation) bool { return recovered(obs) >= 0 },
		time.Duration(smbMaxBackoffSeconds+smbDialTimeoutSeconds+smbOpTimeoutSeconds)*time.Second+30*time.Second),
		"%s: the source did not recover after the drop (status: %s)", c.name, watcher.last())
	recoveredAt := watcher.snapshot()[recovered(watcher.snapshot())].At

	suite.writeScenarioEvidence("drop", map[string]any{
		"blocked_at": window.blocked, "unblocked_at": window.unblocked, "rotation": rotation,
		"error_at": errorAt, "recovered_at": recoveredAt, "dropped_packets": dropped,
		"addresses": helper.addresses, "agent_connections_before": helper.connectionsBefore,
	})
	t.Logf("%s: dropped %d packets to %s:%d for %s around the %s rotation; error after %s, recovered %s after the drop",
		c.name, dropped, strings.Join(helper.addresses, ","), smbPort, window.unblocked.Sub(window.blocked).Round(time.Second),
		rotation.Format(time.TimeOnly), errorAt.Sub(window.blocked).Round(time.Second), recoveredAt.Sub(window.unblocked).Round(time.Second))
	nextRotation := rotation.Add(time.Duration(c.writer.periodMs) * time.Millisecond)
	require.True(t, recoveredAt.Before(nextRotation),
		"the source recovered after the next rotation too, so a file rotated in and out while it could not read; raise %s", runPeriodMs)
	return window
}

// requireWriterRotatedDuring requires a rotation of the writer inside the
// drop, with every record written: its CIFS mount was not affected.
func (suite *azureFilesSuite) requireWriterRotatedDuring(c cell, window dropWindow) {
	ledger, err := suite.readLedger(c, suite.writerPod(c))
	require.NoError(suite.T(), err)
	for _, entry := range ledger {
		at, ok := entry.rotatedAt()
		if ok && at.After(window.blocked) && at.Before(window.unblocked) {
			assert.Empty(suite.T(), entry.UnwrittenSequences,
				"%s: the writer failed to write records of %s during the drop, so its mount was affected", c.name, entry.File)
			return
		}
	}
	assert.Fail(suite.T(), "no rotation during the drop",
		"%s: the writer journalled no rotation between %s and %s, so the drop did not cover a rotation", c.name, window.blocked, window.unblocked)
}

// networkHelper is a privileged pod on the Agent's node that adds and removes
// an iptables rule inside the Agent pod's network namespace. It runs the
// stock workload image, already on the node and pinned by digest, and uses
// the node's own iptables: its commands enter the node's mount namespace
// (PID 1's) and the Agent's network namespace (the PID of a process of the
// agent container) with nsenter. The rule drops only TCP to the smb cell's
// storage endpoint on port 445. The writer's CIFS mount is unaffected: the
// kernel opened its connection from the network namespace of the CSI node
// plugin that mounted it, the node's, which the rule is not in.
type networkHelper struct {
	suite *azureFilesSuite
	pod   string
	// agentPID is a process of the agent container, in the node's PID
	// namespace.
	agentPID          string
	addresses         []string
	comment           string
	blocked           bool
	connectionsBefore int
}

// networkHelperPod is the helper's pod: privileged, in the node's PID
// namespace, on the Agent's node.
func networkHelperPod(name, node, image string, pullSecrets []corev1.LocalObjectReference, runID string) *corev1.Pod {
	privileged := true
	root := int64(0)
	grace := int64(0)
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: e2eNamespace,
			Labels: map[string]string{
				partOfLabel:     partOfLabelValue,
				runIDLabel:      runID,
				helperRoleLabel: networkHelperApp,
			},
		},
		Spec: corev1.PodSpec{
			NodeName:                      node,
			HostPID:                       true,
			RestartPolicy:                 corev1.RestartPolicyNever,
			TerminationGracePeriodSeconds: &grace,
			ImagePullSecrets:              pullSecrets,
			Containers: []corev1.Container{{
				Name:            networkHelperApp,
				Image:           image,
				ImagePullPolicy: corev1.PullIfNotPresent,
				Command:         []string{"sleep", "3600"},
				SecurityContext: &corev1.SecurityContext{Privileged: &privileged, RunAsUser: &root},
			}},
		},
	}
}

// agentPIDScript prints the lowest PID whose cgroup names the container: the
// container's first process.
const agentPIDScript = `pid=; for f in /proc/[0-9]*/cgroup; do if grep -q -- "$1" "$f" 2>/dev/null; then p=${f#/proc/}; p=${p%/cgroup}; if [ -z "$pid" ] || [ "$p" -lt "$pid" ]; then pid=$p; fi; fi; done; test -n "$pid" && echo "$pid"`

// nsenterIptables runs the node's iptables in the network namespace of pid.
func nsenterIptables(pid string, args ...string) []string {
	return append([]string{"nsenter", "--mount=/proc/1/ns/mnt", "--net=/proc/" + pid + "/ns/net", "--", "iptables", "-w", "5"}, args...)
}

// dropRuleArgs add (-I) or delete (-D) the rule dropping TCP to address:445.
func dropRuleArgs(op, address, comment string) []string {
	args := []string{op, "OUTPUT"}
	if op == "-I" {
		args = append(args, "1")
	}
	return append(args, "-p", "tcp", "-d", address, "--dport", strconv.Itoa(smbPort),
		"-m", "comment", "--comment", comment, "-j", "DROP")
}

// parseAddresses reads the IPv4 addresses of getent ahostsv4.
func parseAddresses(getent string) []string {
	seen := make(map[string]bool)
	var addresses []string
	for _, line := range strings.Split(getent, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || seen[fields[0]] {
			continue
		}
		seen[fields[0]] = true
		addresses = append(addresses, fields[0])
	}
	return addresses
}

// countSMBConnections counts the established TCP connections to port 445 in
// a /proc/<pid>/net/tcp listing.
func countSMBConnections(procNetTCP string) int {
	port := fmt.Sprintf(":%04X", smbPort)
	count := 0
	for _, line := range strings.Split(procNetTCP, "\n") {
		fields := strings.Fields(line)
		// sl local_address rem_address st ...; 01 is ESTABLISHED.
		if len(fields) > 3 && strings.HasSuffix(fields[2], port) && fields[3] == "01" {
			count++
		}
	}
	return count
}

// droppedPacketCount reads the packet counter of the rules carrying comment
// in `iptables -L OUTPUT -v -n -x`.
func droppedPacketCount(listing, comment string) int64 {
	var total int64
	for _, line := range strings.Split(listing, "\n") {
		if !strings.Contains(line, "/* "+comment+" */") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if packets, err := strconv.ParseInt(fields[0], 10, 64); err == nil {
			total += packets
		}
	}
	return total
}

// agentContainerID returns the runtime ID of the agent container.
func agentContainerID(pod corev1.Pod) (string, error) {
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name == agentContainer && status.ContainerID != "" {
			_, id, found := strings.Cut(status.ContainerID, "://")
			if !found {
				id = status.ContainerID
			}
			return id, nil
		}
	}
	return "", fmt.Errorf("the agent container of %s has no container ID", pod.Name)
}

// startNetworkHelper starts the helper on the Agent's node and finds the
// Agent's network namespace and the smb cell's storage endpoint.
func (suite *azureFilesSuite) startNetworkHelper(c cell) *networkHelper {
	t := suite.T()
	pods, err := suite.agentPods()
	require.NoError(t, err)
	require.Len(t, pods, 1, "the network helper expects the one Agent pod of the one AKS node")
	agent := pods[0]
	containerID, err := agentContainerID(agent)
	require.NoError(t, err)
	// The writer pod runs the stock image the helper needs: nsenter and a
	// shell, pinned by digest and already pulled on the node.
	writer := suite.writerPod(c)
	helper := &networkHelper{
		suite:   suite,
		pod:     "network-helper-" + hexDigest(suite.spec.runID + time.Now().String())[:10],
		comment: "azure-files-e2e-" + hexDigest(suite.spec.runID)[:12],
	}
	helperPods := suite.Env().KubernetesCluster.Client().CoreV1().Pods(e2eNamespace)
	_, err = helperPods.Create(context.Background(),
		networkHelperPod(helper.pod, agent.Spec.NodeName, writer.Spec.Containers[0].Image, writer.Spec.ImagePullSecrets, suite.spec.runID),
		metav1.CreateOptions{})
	require.NoError(t, err, "create the network helper")
	suite.EventuallyWithT(func(collect *assert.CollectT) {
		pod, err := helperPods.Get(context.Background(), helper.pod, metav1.GetOptions{})
		require.NoError(collect, err)
		assert.Equal(collect, corev1.PodRunning, pod.Status.Phase, "the network helper is not running: %s", suite.podProblems(*pod))
	}, 2*time.Minute, 2*time.Second)

	stdout, err := helper.exec("/bin/sh", "-c", agentPIDScript, "sh", containerID)
	require.NoError(t, err, "find the agent container's process")
	helper.agentPID = strings.TrimSpace(stdout)
	stdout, err = helper.exec("getent", "ahostsv4", c.host())
	require.NoError(t, err, "resolve %s", c.host())
	helper.addresses = parseAddresses(stdout)
	require.NotEmpty(t, helper.addresses, "%s resolved to no IPv4 address", c.host())
	return helper
}

func (h *networkHelper) exec(command ...string) (string, error) {
	stdout, stderr, err := h.suite.Env().KubernetesCluster.KubernetesClient.PodExec(e2eNamespace, h.pod, networkHelperApp, command)
	if err != nil {
		return stdout, fmt.Errorf("%s: %w (stderr: %s)", strings.Join(command, " "), err, strings.TrimSpace(stderr))
	}
	return stdout, nil
}

// block adds the rule, then checks it is in the Agent's network namespace and
// not in the node's, where the writer's mount connects from.
func (h *networkHelper) block() {
	t := h.suite.T()
	tcp, err := h.exec("cat", "/proc/"+h.agentPID+"/net/tcp")
	require.NoError(t, err)
	h.connectionsBefore = countSMBConnections(tcp)
	assert.Positive(t, h.connectionsBefore, "the Agent's network namespace has no connection to port %d before the drop", smbPort)
	for _, address := range h.addresses {
		_, err := h.exec(nsenterIptables(h.agentPID, dropRuleArgs("-I", address, h.comment)...)...)
		require.NoError(t, err, "add the DROP rule for %s", address)
	}
	h.blocked = true
	agentRules, err := h.exec(nsenterIptables(h.agentPID, "-S", "OUTPUT")...)
	require.NoError(t, err)
	require.Contains(t, agentRules, h.comment, "the DROP rule is not in the Agent's network namespace")
	nodeRules, err := h.exec(nsenterIptables("1", "-S", "OUTPUT")...)
	require.NoError(t, err)
	require.NotContains(t, nodeRules, h.comment, "the DROP rule landed in the node's network namespace, where the writer's mount connects from")
}

func (h *networkHelper) droppedPackets() int64 {
	listing, err := h.exec(nsenterIptables(h.agentPID, "-L", "OUTPUT", "-v", "-n", "-x")...)
	if err != nil {
		h.suite.T().Logf("cannot read the DROP rule's counters: %v", err)
		return 0
	}
	return droppedPacketCount(listing, h.comment)
}

// unblock removes the rule.
func (h *networkHelper) unblock() {
	if !h.blocked {
		return
	}
	var failed []error
	for _, address := range h.addresses {
		if _, err := h.exec(nsenterIptables(h.agentPID, dropRuleArgs("-D", address, h.comment)...)...); err != nil {
			failed = append(failed, err)
		}
	}
	if len(failed) == 0 {
		h.blocked = false
	}
	require.NoError(h.suite.T(), errors.Join(failed...), "remove the DROP rule")
}

// cleanup removes the rule if it is still there and deletes the helper. It
// runs deferred, so a failed assertion does not leave the Agent cut off.
func (h *networkHelper) cleanup() {
	if h.blocked {
		for _, address := range h.addresses {
			if _, err := h.exec(nsenterIptables(h.agentPID, dropRuleArgs("-D", address, h.comment)...)...); err != nil {
				h.suite.T().Logf("cannot remove the DROP rule for %s: %v", address, err)
			}
		}
		h.blocked = false
	}
	grace := int64(0)
	if err := h.suite.Env().KubernetesCluster.Client().CoreV1().Pods(e2eNamespace).
		Delete(context.Background(), h.pod, metav1.DeleteOptions{GracePeriodSeconds: &grace}); err != nil {
		h.suite.T().Logf("cannot delete the network helper %s: %v", h.pod, err)
	}
}

// Status watch.

// sourceState classifies an smb source's status.
type sourceState string

const (
	sourceOK          sourceState = "ok"
	sourceAuthError   sourceState = "auth_error"
	sourceUnreachable sourceState = "unreachable"
	sourceOtherError  sourceState = "error"
	sourcePending     sourceState = "pending"
	sourceNotListed   sourceState = "not_listed"
	agentUnavailable  sourceState = "agent_unavailable"
)

// classifySourceStatus reads the status the SMB scanner sets
// (scanner.statusError in pkg/logs/launchers/smb).
func classifySourceStatus(status string) sourceState {
	switch {
	case status == sourceStatusOK:
		return sourceOK
	case status == "Pending":
		return sourcePending
	case strings.Contains(status, "rejected the credentials or denied access"):
		return sourceAuthError
	case strings.Contains(status, "cannot reach"):
		return sourceUnreachable
	default:
		return sourceOtherError
	}
}

// statusObservation is one look at the smb source's status.
type statusObservation struct {
	At     time.Time   `json:"at"`
	Pod    string      `json:"pod,omitempty"`
	State  sourceState `json:"state"`
	Status string      `json:"status,omitempty"`
}

// firstObservation returns the index of the first observation from index from
// on, at or after since, in one of states, or -1.
func firstObservation(obs []statusObservation, from int, since time.Time, states ...sourceState) int {
	for i := max(0, from); i < len(obs); i++ {
		if obs[i].At.Before(since) {
			continue
		}
		for _, state := range states {
			if obs[i].State == state {
				return i
			}
		}
	}
	return -1
}

// statusWatcher records the smb source's status every statusPollInterval on
// a goroutine of its own. It never calls the test's assertions, and redacts
// every key it knows from what it records.
type statusWatcher struct {
	mu           sync.Mutex
	observations []statusObservation
	done         chan struct{}
	stopped      chan struct{}
	stopOnce     sync.Once
}

func (suite *azureFilesSuite) watchSMBStatus(c cell) *statusWatcher {
	w := &statusWatcher{done: make(chan struct{}), stopped: make(chan struct{})}
	go func() {
		defer close(w.stopped)
		ticker := time.NewTicker(statusPollInterval)
		defer ticker.Stop()
		for {
			w.add(suite.observeSMBStatus(c)...)
			select {
			case <-w.done:
				return
			case <-ticker.C:
			}
		}
	}()
	return w
}

// observeSMBStatus looks at the source on every Agent pod once.
func (suite *azureFilesSuite) observeSMBStatus(c cell) []statusObservation {
	now := time.Now()
	keys, err := suite.cellKeys(c)
	if err != nil {
		return []statusObservation{{At: now, State: agentUnavailable, Status: "cannot read the keys to redact: " + err.Error()}}
	}
	pods, err := suite.agentPods()
	if err != nil {
		return []statusObservation{{At: now, State: agentUnavailable, Status: err.Error()}}
	}
	observations := make([]statusObservation, 0, len(pods))
	for _, pod := range pods {
		obs := statusObservation{At: now, Pod: pod.Name}
		statusJSON, err := suite.agentStatusJSON(pod, keys...)
		switch {
		case err != nil:
			obs.State, obs.Status = agentUnavailable, err.Error()
		default:
			source, found, err := findLogSource(statusJSON, string(smbReader), c.service)
			switch {
			case err != nil:
				obs.State, obs.Status = agentUnavailable, err.Error()
			case !found:
				obs.State = sourceNotListed
			default:
				obs.State, obs.Status = classifySourceStatus(source.Status), source.Status
			}
		}
		obs.Status = redactSecrets(obs.Status, keys...)
		observations = append(observations, obs)
	}
	return observations
}

func (w *statusWatcher) add(observations ...statusObservation) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.observations = append(w.observations, observations...)
}

func (w *statusWatcher) snapshot() []statusObservation {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]statusObservation(nil), w.observations...)
}

// last describes the latest observation, for failure messages.
func (w *statusWatcher) last() string {
	obs := w.snapshot()
	if len(obs) == 0 {
		return "none recorded"
	}
	latest := obs[len(obs)-1]
	return fmt.Sprintf("%s %q at %s", latest.State, latest.Status, latest.At.Format(time.TimeOnly))
}

// waitFor polls the observations until done says so or timeout passes.
func (w *statusWatcher) waitFor(done func([]statusObservation) bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if done(w.snapshot()) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(time.Second)
	}
}

// stop ends the watch and returns every observation.
func (w *statusWatcher) stop() []statusObservation {
	w.stopOnce.Do(func() { close(w.done) })
	<-w.stopped
	return w.snapshot()
}
