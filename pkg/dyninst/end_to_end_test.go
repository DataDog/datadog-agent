// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2025 Datadog, Inc.

//go:build linux_bpf

package dyninst_test

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-json-experiment/json/jsontext"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/dyninst/dyninsttest"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	di_module "github.com/DataDog/datadog-agent/pkg/dyninst/module"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcjson"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
	"github.com/DataDog/datadog-agent/pkg/dyninst/uploader"
)

type testState struct {
	tmpDir string

	rc       *dyninsttest.MockAgentRCServer
	rcServer *httptest.Server

	backend       *mockBackend
	backendServer *httptest.Server

	module     *di_module.Module
	subscriber *mockSubscriber
	serviceCmd *exec.Cmd
}

const expectationsPath = "testdata/e2e/rc_tester.json"

//go:embed testdata/e2e/rc_tester.json
var expectations embed.FS

func TestEndToEnd(t *testing.T) {
	dyninsttest.SkipIfKernelNotSupported(t)
	cfgs := testprogs.MustGetCommonConfigs(t)

	tmpDir, cleanup := dyninsttest.PrepTmpDir(t, t.Name())
	defer cleanup()
	ts := &testState{tmpDir: tmpDir}

	diagCh := make(chan []byte, 10)
	ts.backend = &mockBackend{diagPayloadCh: diagCh}
	ts.backendServer = httptest.NewServer(ts.backend)
	t.Cleanup(ts.backendServer.Close)

	ts.rc = dyninsttest.NewMockAgentRCServer()
	ts.rcServer = httptest.NewServer(ts.rc)
	t.Cleanup(ts.rcServer.Close)
	t.Cleanup(ts.rc.Close)

	idx := slices.IndexFunc(cfgs, func(c testprogs.Config) bool {
		return c.GOARCH == runtime.GOARCH
	})
	require.NotEqual(t, -1, idx)
	cfg := cfgs[idx]

	sampleServicePath := testprogs.MustGetBinary(t, "rc_tester", cfg)
	ts.setupRemoteConfig(t)
	serverPort := ts.startSampleService(t, sampleServicePath)

	ts.initializeModule(t)

	ts.subscriber.NotifyExec(uint32(ts.serviceCmd.Process.Pid))

	expectedProbeIDs := []string{"look_at_the_request", "http_handler"}
	waitForProbeStatus(t, ts.backend.diagPayloadCh, uploader.StatusInstalled, expectedProbeIDs)

	const numRequests = 3
	sendTestRequests(t, serverPort, numRequests)
	waitForLogMessages(t, ts.backend, numRequests*len(expectedProbeIDs), expectationsPath)
	waitForProbeStatus(t, ts.backend.diagPayloadCh, uploader.StatusEmitting, expectedProbeIDs)
}

func (ts *testState) setupRemoteConfig(t *testing.T) {
	probes := testprogs.MustGetProbeDefinitions(t, "rc_tester")
	rcs := make(map[string][]byte)
	for _, probe := range probes {
		rcProbe := setSnapshotsPerSecond(t, probe, 100)
		path, content := createProbeEntry(t, rcProbe)
		rcs[path] = content
	}
	ts.rc.UpdateRemoteConfig(rcs)
}

func setSnapshotsPerSecond(
	t *testing.T, probe ir.ProbeDefinition, snapshotsPerSecond float64,
) *rcjson.SnapshotProbe {
	rcProbe, ok := probe.(*rcjson.SnapshotProbe)
	require.True(t, ok)
	rcProbe.Sampling = &rcjson.Sampling{
		SnapshotsPerSecond: snapshotsPerSecond,
	}
	return rcProbe
}

func createProbeEntry(t *testing.T, probe rcjson.Probe) (string, []byte) {
	const fakeOrgID = 1234
	encoded, err := json.Marshal(probe)
	require.NoError(t, err)
	hash := sha256.Sum256(encoded)
	path := fmt.Sprintf(
		"datadog/%d/%s/%s/%s",
		fakeOrgID,
		data.ProductLiveDebugging,
		probe.GetID(),
		hex.EncodeToString(hash[:]),
	)
	return path, encoded
}

func getRcTesterEnv(rcHost string, rcPort int) []string {
	return []string{
		fmt.Sprintf("DD_AGENT_HOST=%s", rcHost),
		fmt.Sprintf("DD_AGENT_PORT=%d", rcPort),
		"DD_DYNAMIC_INSTRUMENTATION_ENABLED=true",
		"DD_REMOTE_CONFIGURATION_ENABLED=true",
		"DD_REMOTE_CONFIG_POLL_INTERVAL_SECONDS=.01",
		"DD_SERVICE=rc_tester",
		"DD_REMOTE_CONFIG_TUF_NO_VERIFICATION=true",
	}
}

func (ts *testState) startSampleService(t *testing.T, sampleServicePath string) int {
	rcHost, rcPort, err := hostPortFromURL(ts.rcServer.URL)
	require.NoError(t, err)
	cfg := sampleServiceConfig{
		rcHost:     rcHost,
		rcPort:     rcPort,
		binaryPath: sampleServicePath,
		tmpDir:     ts.tmpDir,
	}
	cmd, serverPort, err := startSampleService(t, cfg)
	require.NoError(t, err)
	ts.serviceCmd = cmd
	return serverPort
}

type sampleServiceConfig struct {
	rcHost     string
	rcPort     int
	binaryPath string
	tmpDir     string
}

func hostPortFromURL(urlStr string) (host string, port int, err error) {
	rcURL, err := url.Parse(urlStr)
	if err != nil {
		return "", 0, err
	}
	host, portStr, err := net.SplitHostPort(rcURL.Host)
	if err != nil {
		return "", 0, err
	}
	port, err = strconv.Atoi(portStr)
	if err != nil {
		return "", 0, err
	}
	return host, port, nil
}

func startSampleService(t *testing.T, cfg sampleServiceConfig) (sampleServiceCmd *exec.Cmd, serverPort int, err error) {
	cmd := exec.Command(cfg.binaryPath)
	cmd.Env = getRcTesterEnv(cfg.rcHost, cfg.rcPort)

	stderrFile, err := os.Create(filepath.Join(cfg.tmpDir, "rc_tester.stderr"))
	require.NoError(t, err)
	cmd.Stderr = stderrFile
	t.Cleanup(func() {
		if t.Failed() {
			stderrFile.Seek(0, 0)
			stderr, err := io.ReadAll(stderrFile)
			if err == nil {
				t.Logf("rc_tester stderr:\n%s", stderr)
			}
		}
		stderrFile.Close()
	})

	stdoutPath := filepath.Join(cfg.tmpDir, "rc_tester.stdout")
	stdoutFile, err := os.Create(stdoutPath)
	require.NoError(t, err)
	defer stdoutFile.Close()
	cmd.Stdout = stdoutFile

	t.Log("Starting sample service...")
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	serverPort = waitForServicePort(t, stdoutPath)
	t.Logf("rc_tester listening on port %d", serverPort)

	return cmd, serverPort, nil
}

func waitForServicePort(t *testing.T, stdoutPath string) int {
	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatal("timed out waiting for server port")
		case <-ticker.C:
			content, err := os.ReadFile(stdoutPath)
			if err != nil {
				// File might not have been created yet, continue polling
				continue
			}

			scanner := bufio.NewScanner(bytes.NewReader(content))
			for scanner.Scan() {
				const msg = "Listening on port "
				line := scanner.Text()
				if !strings.Contains(line, msg) {
					continue
				}
				portStr := strings.TrimPrefix(line, msg)
				if port, err := strconv.Atoi(portStr); err == nil {
					return port
				}
			}
		}
	}
}

func (ts *testState) initializeModule(t *testing.T) {
	ts.subscriber = &mockSubscriber{}
	cfg, err := di_module.NewConfig(nil)
	require.NoError(t, err)

	cfg.LogUploaderURL = ts.backendServer.URL + "/logs"
	cfg.DiagsUploaderURL = ts.backendServer.URL + "/diags"

	ts.module, err = di_module.NewModule(cfg, ts.subscriber)
	require.NoError(t, err)
}

func sendTestRequests(t *testing.T, serverPort int, numRequests int) {
	testPaths := make([]string, numRequests)
	for i := range numRequests {
		testPaths[i] = fmt.Sprintf("/%d", i)
	}

	t.Log("Sending requests to trigger probes...")
	client := http.Client{Timeout: 1 * time.Second}
	for _, path := range testPaths {
		t.Logf("sending request to %s", path)
		resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d%s", serverPort, path))
		if err == nil {
			resp.Body.Close()
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func waitForProbeStatus(
	t *testing.T,
	diagPayloadCh <-chan []byte,
	targetStatus uploader.Status,
	expectedProbeIDs []string,
) {
	t.Logf("Waiting for probes to be %s...", targetStatus)
	const timeout = 10 * time.Second

	probeStatus := make(map[string]uploader.Status)
	allInStatus := func() bool {
		for _, probeID := range expectedProbeIDs {
			status, ok := probeStatus[probeID]
			if !ok {
				return false
			}
			if status != targetStatus {
				return false
			}
		}
		return true
	}

	timeoutCh := time.After(timeout)
	for !allInStatus() {
		select {
		case p := <-diagPayloadCh:
			processDiagnosticPayload(t, p, probeStatus)
		case <-timeoutCh:
			t.Fatalf(
				"timed out waiting for probes to be %s. Current statuses: %v",
				targetStatus,
				probeStatus,
			)
		}
	}
}

func processDiagnosticPayload(
	t *testing.T,
	payload []byte,
	probeStatus map[string]uploader.Status,
) {
	var diags []*uploader.DiagnosticMessage
	if err := json.Unmarshal(payload, &diags); err != nil {
		t.Logf("failed to unmarshal diag payload: %v", err)
		return
	}

	for _, diag := range diags {
		if diag.Debugger.ProbeID != "" && diag.Debugger.Status != "" {
			probeStatus[diag.Debugger.ProbeID] = diag.Debugger.Status
			t.Logf("Probe %s status: %s", diag.Debugger.ProbeID, diag.Debugger.Status)
		}
	}
}

func waitForLogMessages(
	t *testing.T,
	backend *mockBackend,
	expectedLogs int,
	expectationsPath string,
) {
	t.Log("Waiting for log messages...")

	var processedLogs []json.RawMessage

	logProcessingTimeout := time.After(5 * time.Second)
	checkTicker := time.NewTicker(100 * time.Millisecond)
	defer checkTicker.Stop()

	for len(processedLogs) < expectedLogs {
		select {
		case <-logProcessingTimeout:
			t.Fatalf(
				"timed out waiting for log messages. Currently have %d logs (expected %d)",
				len(processedLogs),
				expectedLogs,
			)
		case <-checkTicker.C:
			payloads := backend.getLogPayloads()
			for _, p := range payloads {
				var logs []json.RawMessage
				require.NoError(t, json.Unmarshal(p, &logs))
				processedLogs = append(processedLogs, logs...)
			}
		}
	}

	var content []byte
	{
		redactors := append(make([]jsonRedactor, 0, len(defaultRedactors)), defaultRedactors...)
		redactors = append(redactors, redactor(
			prefixSuffixMatcher{
				"/debugger/snapshot/captures/",
				"/RemoteAddr/value",
			},
			replacement(`"[remote network address]"`),
		))
		redactors = append(redactors, redactor(
			prefixSuffixMatcher{
				"/debugger/snapshot/captures/",
				"/Host/value",
			},
			replacement(`"[host network address]"`),
		))
		// These source files can have different path prefixes, so let's redact
		// them.
		redactors = append(redactors, redactor(
			prefixSuffixMatcher{
				"/debugger/snapshot/captures/",
				"/pat/fields/loc/value",
			},
			replacerFunc(func(v jsontext.Value) jsontext.Value {
				t, err := jsontext.NewDecoder(bytes.NewReader(v)).ReadToken()
				if err != nil {
					return v
				}
				s := t.String()
				idx := strings.Index(s, "pkg/dyninst")
				if idx == -1 {
					return v
				}
				s = "[datadog-agent]/" + s[idx:]
				var buf bytes.Buffer
				_ = jsontext.NewEncoder(&buf).WriteToken(jsontext.String(s))
				return jsontext.Value(buf.Bytes())
			}),
		))
		var allRedacted []json.RawMessage
		for _, log := range processedLogs {
			allRedacted = append(allRedacted, redactJSON(t, log, redactors))
		}
		var err error
		content, err = json.MarshalIndent(allRedacted, "", "  ")
		require.NoError(t, err)
	}
	if expectationsPath == "" {
		return
	}

	rewrite, _ := strconv.ParseBool(os.Getenv("REWRITE"))
	if rewrite {
		saveExpectations(t, content, expectationsPath)
		return
	}

	golden, err := expectations.ReadFile(expectationsPath)
	require.NoError(t, err)
	require.Equal(t, string(golden), string(content))
}

func saveExpectations(t *testing.T, content []byte, expectationsPath string) {
	err := os.MkdirAll(filepath.Dir(expectationsPath), 0755)
	require.NoError(t, err)
	tmpFile, err := os.CreateTemp(filepath.Dir(expectationsPath), ".tmp.rc_tester.*.json")
	require.NoError(t, err)
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	_, err = io.Copy(tmpFile, bytes.NewReader(content))
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())
	require.NoError(t, os.Rename(tmpFile.Name(), expectationsPath))
	t.Logf("golden file saved to %s", tmpFile.Name())
}

type mockSubscriber struct {
	mu   sync.Mutex
	exec func(uint32)
	exit func(uint32)
}

func (m *mockSubscriber) SubscribeExec(cb func(uint32)) func() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.exec = cb
	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.exec = nil
	}
}

func (m *mockSubscriber) SubscribeExit(cb func(uint32)) func() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.exit = cb
	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.exit = nil
	}
}

func (m *mockSubscriber) NotifyExec(pid uint32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.exec != nil {
		m.exec(pid)
	}
}

func (m *mockSubscriber) Sync() error {
	return nil
}

type mockBackend struct {
	mu            sync.Mutex
	logPayloads   [][]byte
	diagPayloadCh chan []byte
}

func (m *mockBackend) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/logs":
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}
		m.mu.Lock()
		m.logPayloads = append(m.logPayloads, body)
		m.mu.Unlock()
	case "/diags":
		err := r.ParseMultipartForm(10 << 20) // 10 MiB
		if err != nil {
			http.Error(w, "failed to parse multipart form", http.StatusBadRequest)
			return
		}
		file, _, err := r.FormFile("event")
		if err != nil {
			http.Error(w, "failed to get event file", http.StatusBadRequest)
			return
		}
		defer file.Close()
		body, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, "failed to read event file", http.StatusBadRequest)
			return
		}
		m.diagPayloadCh <- body
	default:
		http.NotFound(w, r)
	}

	w.WriteHeader(http.StatusOK)
}

func (m *mockBackend) getLogPayloads() [][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	ret := m.logPayloads
	m.logPayloads = nil
	return ret
}
