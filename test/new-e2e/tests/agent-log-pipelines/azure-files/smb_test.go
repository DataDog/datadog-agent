// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package azurefiles

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// bytesReadInfoKey is the per-source counter every log source registers
	// in its status info (pkg/logs/sources).
	bytesReadInfoKey = "Bytes Read"
	sourceStatusOK   = "OK"

	verboseStatusJSONCommand = "agent status --json --verbose"
	secretInfoCommand        = "agent secret"
)

var (
	// leakCheckedCommands are the Agent commands whose output must never
	// contain an SMB cell's storage account key.
	leakCheckedCommands = [][]string{
		{"agent", "status", "--verbose"},
		strings.Fields(verboseStatusJSONCommand),
		{"agent", "configcheck"},
		// Lists the secret handles the Agent resolved and where it used them.
		strings.Fields(secretInfoCommand),
	}

	// agent flare prints the archive path before it asks for confirmation.
	flareArchivePathPattern = regexp.MustCompile(`(/[^\s"\x1b]+\.zip)`)
	// The flare stores each expvar map as YAML; numbers went through a JSON
	// float on the way, so large values can be written in exponent form.
	bytesMissedPattern = regexp.MustCompile(`(?m)^BytesMissed:\s*(\S+)\s*$`)
)

// agentStatus is the part of `agent status --json` this suite reads.
type agentStatus struct {
	LogsStats struct {
		Integrations []struct {
			Name    string            `json:"name"`
			Sources []logSourceStatus `json:"sources"`
		} `json:"integrations"`
		// Tailers is only filled with --verbose.
		Tailers []logTailerStatus `json:"tailers"`
	} `json:"logsStats"`
}

// logSourceStatus is one log source as `agent status --json` reports it.
type logSourceStatus struct {
	Type          string              `json:"type"`
	Configuration map[string]any      `json:"configuration"`
	Status        string              `json:"status"`
	Inputs        []string            `json:"inputs"`
	Messages      []string            `json:"messages"`
	Info          map[string][]string `json:"info"`
}

// logTailerStatus is one tailer as `agent status --json --verbose` reports it.
type logTailerStatus struct {
	ID   string              `json:"id"`
	Type string              `json:"type"`
	Info map[string][]string `json:"info"`
}

// smbSourceRecord is what the evidence keeps about an SMB source, per Agent
// pod, at the end of its cell.
type smbSourceRecord struct {
	Pod       string              `json:"pod"`
	Service   string              `json:"service"`
	Status    string              `json:"status"`
	Inputs    []string            `json:"inputs"`
	Messages  []string            `json:"messages"`
	Info      map[string][]string `json:"info"`
	BytesRead string              `json:"bytes_read"`
	// Tailers are the source's tailers when the cell ends: the active file's,
	// and those of rotated files still draining, each with its own Bytes Read.
	Tailers []logTailerStatus `json:"tailers"`
	// SecretHandleResolved says whether `agent secret` lists the password
	// handle among the resolved ones.
	SecretHandleResolved bool `json:"secret_handle_resolved"`
	// BytesMissedAgentTotal is the Agent-wide logs-agent BytesMissed expvar,
	// read from a flare. It also counts the file sources of the other cells
	// of the run.
	BytesMissedAgentTotal *int64 `json:"bytes_missed_agent_total,omitempty"`
}

// requireSMBSourceRunning fails the cell early when the Agent does not run its
// SMB source. Without this, an Agent built without the native SMB source would
// only show up as every sequence missing.
func (suite *azureFilesSuite) requireSMBSourceRunning(c cell) {
	suite.T().Helper()
	keys, err := suite.cellKeys(c)
	require.NoError(suite.T(), err)
	pods, err := suite.agentPods()
	require.NoError(suite.T(), err)
	require.NotEmpty(suite.T(), pods, "no Agent pod to run the %s source", c.name)

	for _, pod := range pods {
		suite.EventuallyWithT(func(collect *assert.CollectT) {
			statusJSON, err := suite.agentStatusJSON(pod, keys...)
			require.NoError(collect, err)
			source, found, err := findLogSource(statusJSON, string(smbReader), c.service)
			require.NoError(collect, err)
			require.True(collect, found,
				"the Agent on %s lists no %s source for service %s", pod.Name, smbReader, c.service)
			// testify prints both values on a mismatch, so the status is
			// compared redacted: an error status may quote the password.
			assert.Equal(collect, sourceStatusOK, redactSecrets(source.Status, keys...),
				"the %s source on %s is not running; Pending means no launcher handles type: %s, which is what an Agent built without the native SMB source does",
				c.name, pod.Name, smbReader)
		}, 2*time.Minute, 10*time.Second)
	}
}

// checkSMBSource records how the SMB source read the cell, and checks that the
// storage account key it resolves through the secret backend appears nowhere
// the Agent prints, logs, packs into a flare or ships. Failure messages say
// where the key was found and never include it.
func (suite *azureFilesSuite) checkSMBSource(c cell) {
	suite.T().Helper()
	keys, err := suite.cellKeys(c)
	require.NoError(suite.T(), err)

	pods, err := suite.agentPods()
	require.NoError(suite.T(), err)
	require.NotEmpty(suite.T(), pods, "no Agent pod to check for the %s account key", c.name)
	cluster := suite.Env().KubernetesCluster
	for _, pod := range pods {
		// The verbose status also lists every tailer. The JSON status is not
		// scrubbed by the CLI, unlike the text one, so both are checked.
		outputs := make(map[string]string, len(leakCheckedCommands))
		for _, command := range leakCheckedCommands {
			name := strings.Join(command, " ")
			stdout, stderr, execErr := cluster.KubernetesClient.PodExec(agentNamespace, pod.Name, "agent", command)
			require.NoError(suite.T(), execErr, "%s on %s (stderr: %s)",
				name, pod.Name, redactSecrets(strings.TrimSpace(stderr), keys...))
			assert.False(suite.T(), containsAny(stdout+stderr, keys),
				"%s on %s prints a %s storage account key", name, pod.Name, c.name)
			outputs[name] = stdout
		}

		flare, err := suite.agentFlare(pod, keys...)
		require.NoError(suite.T(), err)
		for _, key := range keys {
			for _, entry := range flare.entriesContaining(key) {
				assert.Fail(suite.T(), "flare exposes a storage account key",
					"the flare of %s contains a %s storage account key in %s", pod.Name, c.name, entry)
			}
		}

		// Every container mounts the key file, so every container log is
		// checked, not only the core Agent's.
		for _, container := range pod.Spec.Containers {
			logs, logErr := cluster.Client().CoreV1().Pods(agentNamespace).
				GetLogs(pod.Name, &corev1.PodLogOptions{Container: container.Name}).DoRaw(context.Background())
			require.NoError(suite.T(), logErr, "read the %s container log of %s", container.Name, pod.Name)
			assert.False(suite.T(), containsAny(string(logs), keys),
				"the %s container log of %s contains a %s storage account key", container.Name, pod.Name, c.name)
		}

		suite.recordSMBSource(c, pod, outputs[verboseStatusJSONCommand], outputs[secretInfoCommand], flare, keys)
	}

	services, err := suite.Env().FakeIntake.Client().GetLogServiceNames()
	require.NoError(suite.T(), err)
	for _, service := range services {
		messages, err := suite.collectedMessages(service)
		require.NoError(suite.T(), err)
		for _, message := range messages {
			if containsAny(message, keys) {
				assert.Fail(suite.T(), "shipped log exposes a storage account key",
					"a log of service %s collected by Fakeintake contains a %s storage account key", service, c.name)
				break
			}
		}
	}
}

// recordSMBSource writes the SMB source's status, its tailers and the Agent's
// missed-bytes total to the evidence, and logs a one-line summary with key
// redacted. It records rather than asserts: the exactly-once checks already
// decide the cell, and the leak checks already cover the status.
func (suite *azureFilesSuite) recordSMBSource(c cell, pod corev1.Pod, statusJSON, secretInfo string, flare flareArchive, keys []string) {
	suite.T().Helper()
	record := smbSourceRecord{Pod: pod.Name, Service: c.service}
	status, err := decodeAgentStatus(statusJSON)
	if err != nil {
		suite.T().Logf("%s: cannot read the status of %s: %v", c.name, pod.Name, err)
	} else if source, found := status.logSource(string(smbReader), c.service); !found {
		suite.T().Logf("%s: the status of %s lists no %s source for service %s", c.name, pod.Name, smbReader, c.service)
	} else {
		record.Status = source.Status
		record.Inputs = source.Inputs
		record.Messages = source.Messages
		record.Info = source.Info
		record.BytesRead = strings.Join(source.Info[bytesReadInfoKey], ",")
		record.Tailers = status.smbTailers(c)
	}
	record.SecretHandleResolved = secretHandleResolved(secretInfo, smbPasswordHandle(c))
	bytesMissed := "unknown"
	if missed, ok := flare.logsAgentBytesMissed(); ok {
		record.BytesMissedAgentTotal = &missed
		bytesMissed = strconv.FormatInt(missed, 10)
	}

	suite.T().Logf("%s on %s: status=%s inputs=%d tailers=%d bytes_read=%s bytes_missed_agent_total=%s secret_handle_resolved=%t",
		c.name, pod.Name, redactSecrets(record.Status, keys...), len(record.Inputs), len(record.Tailers),
		record.BytesRead, bytesMissed, record.SecretHandleResolved)
	evidence, err := suite.evidenceDir()
	if err != nil {
		suite.T().Logf("cannot create evidence directory: %v", err)
		return
	}
	if !evidence.redactsEverySecret() {
		return
	}
	evidence.writeJSON(c.name+"-"+pod.Name+"-source.json", record)
}

// accountKey reads the storage account key a cell's writer mounts the share
// with, from its Kubernetes Secret.
func (suite *azureFilesSuite) accountKey(c cell) (string, error) {
	return suite.secretKey(e2eNamespace, c)
}

// agentAccountKey reads the key of the cell's Secret copy in the Agent
// namespace, which the SMB source authenticates with. It is the writer's key
// except in the key-rotation scenario.
func (suite *azureFilesSuite) agentAccountKey(c cell) (string, error) {
	return suite.secretKey(agentNamespace, c)
}

func (suite *azureFilesSuite) secretKey(namespace string, c cell) (string, error) {
	secret, err := suite.Env().KubernetesCluster.Client().CoreV1().Secrets(namespace).
		Get(context.Background(), c.secretName(), metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("read Secret %s/%s: %w", namespace, c.secretName(), err)
	}
	key := string(secret.Data[accountKeySecretKey])
	if key == "" {
		return "", fmt.Errorf("Secret %s/%s has no %s", namespace, c.secretName(), accountKeySecretKey)
	}
	return key, nil
}

// containsAny reports whether text contains any of the non-empty secrets.
func containsAny(text string, secrets []string) bool {
	for _, secret := range secrets {
		if secret != "" && strings.Contains(text, secret) {
			return true
		}
	}
	return false
}

func (suite *azureFilesSuite) agentStatusJSON(pod corev1.Pod, secrets ...string) (string, error) {
	stdout, stderr, err := suite.Env().KubernetesCluster.KubernetesClient.PodExec(
		agentNamespace, pod.Name, "agent", []string{"agent", "status", "--json"},
	)
	if err != nil {
		return "", fmt.Errorf("agent status --json on %s: %w (stderr: %s)", pod.Name, err, redactSecrets(strings.TrimSpace(stderr), secrets...))
	}
	return stdout, nil
}

// agentFlare builds a flare in an Agent pod without sending it, copies the
// archive out and deletes it from the pod. Without --send, agent flare asks for
// confirmation on stdin; the exec has no stdin, so the answer is no and the
// archive stays where the command printed it.
func (suite *azureFilesSuite) agentFlare(pod corev1.Pod, secrets ...string) (flareArchive, error) {
	client := suite.Env().KubernetesCluster.KubernetesClient
	stdout, stderr, err := client.PodExec(agentNamespace, pod.Name, "agent", []string{"agent", "flare"})
	if err != nil {
		return nil, fmt.Errorf("agent flare on %s: %w (stderr: %s)", pod.Name, err, redactSecrets(strings.TrimSpace(stderr), secrets...))
	}
	archivePath, err := flareArchivePath(stdout)
	if err != nil {
		return nil, fmt.Errorf("agent flare on %s: %w", pod.Name, err)
	}
	defer func() {
		_, _, _ = client.PodExec(agentNamespace, pod.Name, "agent", []string{"rm", "-f", archivePath})
	}()

	content, stderr, err := client.PodExec(agentNamespace, pod.Name, "agent", []string{"cat", archivePath})
	if err != nil {
		return nil, fmt.Errorf("copy %s out of %s: %w (stderr: %s)", archivePath, pod.Name, err, redactSecrets(strings.TrimSpace(stderr), secrets...))
	}
	return readFlareArchive([]byte(content))
}

func flareArchivePath(output string) (string, error) {
	match := flareArchivePathPattern.FindStringSubmatch(output)
	if match == nil {
		return "", errors.New("the output names no flare archive")
	}
	return match[1], nil
}

// findLogSource returns the log source of the given type and service from the
// output of `agent status --json`.
func findLogSource(statusJSON, sourceType, service string) (logSourceStatus, bool, error) {
	status, err := decodeAgentStatus(statusJSON)
	if err != nil {
		return logSourceStatus{}, false, err
	}
	source, found := status.logSource(sourceType, service)
	return source, found, nil
}

// decodeAgentStatus decodes the output of `agent status --json`, which can be
// preceded by warnings and by Agent log lines. Those lines may contain braces
// (for example "[]interface {}"), so the JSON is looked for only where a line
// starts with '{'.
func decodeAgentStatus(statusJSON string) (agentStatus, error) {
	var lastErr error
	for start := 0; start < len(statusJSON); {
		if statusJSON[start] == '{' {
			var status agentStatus
			err := json.NewDecoder(strings.NewReader(statusJSON[start:])).Decode(&status)
			if err == nil {
				return status, nil
			}
			lastErr = err
		}
		next := strings.IndexByte(statusJSON[start:], '\n')
		if next < 0 {
			break
		}
		start += next + 1
	}
	if lastErr != nil {
		return agentStatus{}, fmt.Errorf("decode agent status: %w", lastErr)
	}
	return agentStatus{}, errors.New("agent status --json printed no JSON object")
}

func (s agentStatus) logSource(sourceType, service string) (logSourceStatus, bool) {
	for _, integration := range s.LogsStats.Integrations {
		for _, source := range integration.Sources {
			if source.Type == sourceType && source.Configuration["Service"] == service {
				return source, true
			}
		}
	}
	return logSourceStatus{}, false
}

// smbTailers returns the tailers of an SMB cell's share. Their ID is the
// smb://<host>/<share>/<path> registry identifier, which a rotated file keeps
// while it drains.
func (s agentStatus) smbTailers(c cell) []logTailerStatus {
	prefix := "smb://" + strings.ToLower(c.host()) + "/" + strings.ToLower(c.shareName) + "/"
	var tailers []logTailerStatus
	for _, tailer := range s.LogsStats.Tailers {
		if tailer.Type == string(smbReader) && strings.HasPrefix(tailer.ID, prefix) {
			tailers = append(tailers, tailer)
		}
	}
	return tailers
}

// secretHandleResolved reports whether the output of `agent secret` lists the
// ENC[] handle among the resolved secrets, which it prints without ENC[].
func secretHandleResolved(secretInfo, handle string) bool {
	handle = strings.TrimSuffix(strings.TrimPrefix(handle, "ENC["), "]")
	resolved, _, _ := strings.Cut(secretInfo, "Secrets not resolved:")
	return strings.Contains(resolved, "- '"+handle+"':")
}

// flareArchive is the content of an Agent flare by archive entry name.
type flareArchive map[string][]byte

func readFlareArchive(content []byte) (flareArchive, error) {
	reader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return nil, fmt.Errorf("open flare archive: %w", err)
	}
	archive := make(flareArchive, len(reader.File))
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		entry, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("open flare entry %s: %w", file.Name, err)
		}
		data, err := io.ReadAll(entry)
		if closeErr := entry.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return nil, fmt.Errorf("read flare entry %s: %w", file.Name, err)
		}
		archive[file.Name] = data
	}
	return archive, nil
}

// entriesContaining lists, in order, the entries that contain secret.
func (a flareArchive) entriesContaining(secret string) []string {
	var entries []string
	for name, data := range a {
		if bytes.Contains(data, []byte(secret)) {
			entries = append(entries, name)
		}
	}
	sort.Strings(entries)
	return entries
}

// logsAgentBytesMissed reads the BytesMissed counter of the logs-agent expvar,
// which the flare stores under <root>/expvar/logs-agent.
func (a flareArchive) logsAgentBytesMissed() (int64, bool) {
	for name, data := range a {
		if path.Base(name) != "logs-agent" || path.Base(path.Dir(name)) != "expvar" {
			continue
		}
		match := bytesMissedPattern.FindSubmatch(data)
		if match == nil {
			return 0, false
		}
		value, err := strconv.ParseFloat(string(match[1]), 64)
		if err != nil {
			return 0, false
		}
		return int64(value), true
	}
	return 0, false
}
