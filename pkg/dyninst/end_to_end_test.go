// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2025 Datadog, Inc.

//go:build linux_bpf

package dyninst_test

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/dyninsttest"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/module"
	"github.com/DataDog/datadog-agent/pkg/dyninst/module/tombstone"
	"github.com/DataDog/datadog-agent/pkg/dyninst/process"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procsubscribe"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcjson"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
	"github.com/DataDog/datadog-agent/pkg/dyninst/uploader"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/backoff"
)

type testState struct {
	tmpDir string

	rc       *dyninsttest.MockAgentRCServer
	rcServer *httptest.Server

	backend       *mockBackend
	backendServer *httptest.Server
	// A mock backend for uploading SymDB data to.
	symdbServer *httptest.Server
	symdbURL    string

	module     *module.Module
	serviceCmd *exec.Cmd
	servicePID uint32

	useDocker bool
}

func dockerIsEnabled(t *testing.T) bool {
	_, err := exec.LookPath("docker")
	if err != nil {
		return false
	}
	cmd := exec.Command("docker", "system", "info")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("docker system info: %s", string(out))
		return false
	}
	return true
}

const expectationsDir = "testdata/e2e"

const e2eTmpDirEnv = "E2E_TMP_DIR"

//go:embed testdata/e2e/rc_tester.json testdata/e2e/rc_tester_v1.json
var expectations embed.FS

func TestEndToEnd(t *testing.T) {
	t.Parallel()
	dyninsttest.SkipIfKernelNotSupported(t)
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	cfgs := testprogs.MustGetCommonConfigs(t)
	idx := slices.IndexFunc(cfgs, func(c testprogs.Config) bool {
		return c.GOARCH == runtime.GOARCH
	})
	require.NotEqual(t, -1, idx)
	cfg := cfgs[idx]

	rewrite, _ := strconv.ParseBool(os.Getenv("REWRITE"))
	useDocker := dockerIsEnabled(t)
	testCases := []struct {
		program       string
		supportsSymDB bool
	}{
		{"rc_tester", true},
		{"rc_tester_v1", false},
	}
	for _, tc := range testCases {
		t.Run(tc.program, func(t *testing.T) {
			t.Parallel()
			t.Run("docker", func(t *testing.T) {
				if rewrite {
					t.Skip("rewrite is enabled, skipping docker test")
				}
				if !useDocker {
					t.Skip("docker is not enabled")
				}
				if testing.Short() {
					t.Skip("skipping docker test in short mode")
				}
				t.Parallel()
				runE2ETest(t, e2eTestConfig{
					cfg:       cfg,
					binary:    tc.program,
					rewrite:   rewrite,
					useDocker: true,
					addSymdb:  tc.supportsSymDB,
				})
			})
			t.Run("direct", func(t *testing.T) {
				t.Parallel()
				runE2ETest(t, e2eTestConfig{
					cfg:       cfg,
					binary:    tc.program,
					rewrite:   rewrite,
					useDocker: false,
					addSymdb:  tc.supportsSymDB,
				})
			})
		})
	}
}

type e2eTestConfig struct {
	cfg       testprogs.Config
	binary    string
	useDocker bool
	rewrite   bool

	// This binary supports subscribing to the symdb rc product, and we should
	// test that.
	addSymdb bool
}

type fakeSubscriber struct {
	subscribeFunc func(func(process.ProcessesUpdate))
	startFunc     func()
}

func (f *fakeSubscriber) Subscribe(cb func(process.ProcessesUpdate)) {
	f.subscribeFunc(cb)
}

func (f *fakeSubscriber) Start() {
	f.startFunc()
}

type agentServiceImpl struct {
	*dyninsttest.MockAgentRCServer

	unimplementedAgentSecureServer
}

type unimplementedAgentSecureServer struct {
	pbgo.UnimplementedAgentSecureServer
}

func runE2ETest(t *testing.T, cfg e2eTestConfig) {
	tmpDir, cleanup := dyninsttest.PrepTmpDir(t, strings.ReplaceAll(t.Name(), "/", "_"))
	defer cleanup()
	ts := &testState{tmpDir: tmpDir, useDocker: cfg.useDocker}

	diagCh := make(chan []byte, 10)
	ts.backend = &mockBackend{diagPayloadCh: diagCh}
	ts.backendServer = httptest.NewServer(ts.backend)
	t.Cleanup(ts.backendServer.Close)

	ts.rc = dyninsttest.NewMockAgentRCServer()
	ts.rcServer = httptest.NewServer(ts.rc)
	t.Cleanup(ts.rcServer.Close)
	t.Cleanup(ts.rc.Close)
	bufConn := bufconn.Listen(1024)
	s := grpc.NewServer(
		grpc.Creds(insecure.NewCredentials()),
	)
	pbgo.RegisterAgentSecureServer(s, &agentServiceImpl{MockAgentRCServer: ts.rc})
	go func() { _ = s.Serve(bufConn) }()
	t.Cleanup(func() { s.Stop() })
	client, err := grpc.NewClient(
		"passthrough://bufnet",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return bufConn.Dial()
		}),
	)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })
	rcSubscriberClient := pbgo.NewAgentSecureClient(client)

	symDBRequests := atomic.Uint64{}
	ts.symdbServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		symDBRequests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(ts.symdbServer.Close)
	ts.symdbURL = ts.symdbServer.URL

	probes := testprogs.MustGetProbeDefinitions(t, cfg.binary)
	rcs := makeRemoteConfigUpdate(t, probes, cfg.addSymdb)
	ts.rc.UpdateRemoteConfig(rcs)
	sampleServicePath := testprogs.MustGetBinary(t, cfg.binary, cfg.cfg)
	serverPort := ts.startSampleService(t, sampleServicePath)

	modCfg, err := module.NewConfig(nil)
	require.NoError(t, err)
	modCfg.ActuatorConfig.RecompilationRateLimit = -1

	modCfg.SymDBUploadEnabled = true
	modCfg.LogUploaderURL = ts.backendServer.URL + "/logs"
	modCfg.DiagsUploaderURL = ts.backendServer.URL + "/diags"
	modCfg.SymDBUploaderURL = ts.symdbURL

	started := make(chan struct{})
	symdbProcStates := make(map[process.ID]bool)
	modCfg.TestingKnobs.ProcessSubscriberOverride = func(
		subscriber module.ProcessSubscriber,
	) module.ProcessSubscriber {
		return &fakeSubscriber{
			subscribeFunc: func(callback func(process.ProcessesUpdate)) {
				subscriber.Subscribe(func(update process.ProcessesUpdate) {
					if cfg.addSymdb && len(update.Updates) > 0 {
						u := update.Updates[0]
						symdbProcStates[u.ProcessID] = u.ShouldUploadSymDB
					}
					callback(update)
				})
			},
			startFunc: func() {
				subscriber.Start()
				close(started)
			},
		}
	}

	ts.module, err = module.NewModule(modCfg, rcSubscriberClient)
	require.NoError(t, err)
	t.Cleanup(ts.module.Close)
	<-started

	expectedProbeIDs := []string{"look_at_the_request", "http_handler"}
	waitForProbeStatus(
		t, ts.backend.diagPayloadCh,
		makeTargetStatus(uploader.StatusInstalled, expectedProbeIDs...),
	)

	assertSymdb := func(c *assert.CollectT, expEnabled bool) {
		assert.Len(c, symdbProcStates, 1)
		var procID process.ID
		var enabled bool
		for procID, enabled = range symdbProcStates {
			break
		}
		assert.Equal(c, ts.servicePID, uint32(procID.PID))
		assert.Equal(c, expEnabled, enabled)
	}

	// If we added symdb, make sure we detect that it's enabled.
	if cfg.addSymdb {
		require.EventuallyWithT(t, func(c *assert.CollectT) {
			assertSymdb(c, true)
			assert.Greater(c, symDBRequests.Load(), uint64(0))
		}, 10*time.Second, 100*time.Millisecond, "SymDB server should be hit")
	}

	const numRequests = 3
	sendTestRequests(t, serverPort, numRequests)
	waitForLogMessages(
		t, ts.backend, numRequests*len(expectedProbeIDs),
		path.Join(expectationsDir, cfg.binary+".json"), cfg.rewrite,
	)
	waitForProbeStatus(
		t, ts.backend.diagPayloadCh,
		makeTargetStatus(uploader.StatusEmitting, expectedProbeIDs...),
	)

	assertModuleStats := func(t require.TestingT, expected actuator.Metrics) {
		stats := ts.module.GetStats()["actuator"].(map[string]any)
		exp := expected.AsStats()
		gotKeys := slices.Sorted(maps.Keys(stats))
		expectedKeys := slices.Sorted(maps.Keys(exp))
		require.Equal(t, gotKeys, expectedKeys)
		for _, key := range gotKeys {
			assert.Equal(t, exp[key], stats[key], "key %s", key)
		}
	}

	assertModuleStats(t, actuator.Metrics{
		NumProcesses: 1,
		NumPrograms:  1,
		NumAttached:  1,
		Loaded:       1,
		Attached:     1,
	})

	// Ensure that the diagnostics states are as expected.
	require.Equal(t,
		[]map[string][]string{
			{
				"look_at_the_request": {"received", "installed", "emitted"},
				"http_handler":        {"received", "installed", "emitted"},
			},
		},
		slices.Collect(maps.Values(ts.module.DiagnosticsStates())),
	)

	// Clear the remote config.
	ts.rc.UpdateRemoteConfig(nil)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		// If we previously added the SymDB key, make sure we detect that it's
		// gone.
		if cfg.addSymdb {
			assertSymdb(c, false)
		}
		assertModuleStats(c, actuator.Metrics{
			NumProcesses: 0,
			NumPrograms:  0,
			NumAttached:  0,

			Loaded:   1,
			Attached: 1,
			Detached: 1,
			Unloaded: 1,
		})
	}, 10*time.Second, 100*time.Millisecond, "probes should be removed")

	// Ensure that the diagnostics states have been cleared.
	require.Empty(t, ts.module.DiagnosticsStates())

	require.NoError(t, ts.serviceCmd.Process.Signal(os.Interrupt))
	require.NoError(t, ts.serviceCmd.Wait())
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.Empty(c, ts.module.DiagnosticsStates())
	}, 10*time.Second, 100*time.Millisecond, "diagnostics states should be empty")
}

func makeRemoteConfigUpdate(t *testing.T, probes []ir.ProbeDefinition, addSymdb bool) map[string][]byte {
	rcs := make(map[string][]byte)
	for _, probe := range probes {
		rcProbe := setSnapshotsPerSecond(t, probe, 100)
		path, content := createProbeEntry(t, rcProbe)
		rcs[path] = content
	}
	if addSymdb {
		payload := []byte(`{"upload_symbols": true}`)
		p := createRemoteConfigPath("LIVE_DEBUGGING_SYMBOL_DB", "symDb", payload)
		rcs[p] = payload
	}
	return rcs
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
	encoded, err := json.Marshal(probe)
	require.NoError(t, err)
	path := createRemoteConfigPath(data.ProductLiveDebugging, probe.GetID(), encoded)
	return path, encoded
}

func createRemoteConfigPath(product data.Product, id string, data []byte) string {
	const fakeOrgID = 1234
	hash := sha256.Sum256(data)
	return fmt.Sprintf(
		"datadog/%d/%s/%s/%s",
		fakeOrgID,
		product,
		id,
		hex.EncodeToString(hash[:]),
	)
}

func getRcTesterEnv(rcHost string, rcPort int, tmpDir string) []string {
	return []string{
		"DD_AGENT_HOST=" + rcHost,
		fmt.Sprintf("DD_AGENT_PORT=%d", rcPort),
		"DD_DYNAMIC_INSTRUMENTATION_ENABLED=true",
		"DD_REMOTE_CONFIGURATION_ENABLED=true",
		"DD_REMOTE_CONFIG_POLL_INTERVAL_SECONDS=.01",
		"DD_SERVICE=rc_tester",
		"DD_ENV=test",
		"DD_VERSION=1.0.0",
		"DD_REMOTE_CONFIG_TUF_NO_VERIFICATION=true",
		fmt.Sprintf("%s=%s", e2eTmpDirEnv, tmpDir),
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
		useDocker:  ts.useDocker,
	}
	cmd, sampleServicePID, serverPort, err := startSampleService(t, cfg)
	require.NoError(t, err)
	ts.serviceCmd = cmd
	ts.servicePID = sampleServicePID
	return serverPort
}

type sampleServiceConfig struct {
	rcHost     string
	rcPort     int
	binaryPath string
	tmpDir     string
	useDocker  bool
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

func startSampleServiceWithDocker(
	t *testing.T,
	cfg sampleServiceConfig,
) (sampleServiceCmd *exec.Cmd) {
	// Copy the binary to a tar file in the tmp dir.
	tarPath := filepath.Join(cfg.tmpDir, "rc_tester.tar")
	tarFile, err := os.Create(tarPath)
	require.NoError(t, err)
	defer tarFile.Close()
	binaryFile, err := os.Open(cfg.binaryPath)
	require.NoError(t, err)
	defer binaryFile.Close()
	stat, err := binaryFile.Stat()
	require.NoError(t, err)
	tarWriter := tar.NewWriter(tarFile)
	binName := filepath.Base(cfg.binaryPath)
	tarWriter.WriteHeader(&tar.Header{
		Name: binName,
		Mode: 0755, //rwxr-xr-x
		Size: stat.Size(),
	})
	_, err = io.Copy(tarWriter, binaryFile)
	require.NoError(t, err)
	require.NoError(t, tarWriter.Close())
	require.NoError(t, tarFile.Close())

	containerTag := strings.ReplaceAll(strings.ReplaceAll(cfg.tmpDir, "/", "_"), ":", "_")
	containerName := "dyninst-e2e:" + containerTag
	// Build the docker image.
	dockerBuildCmd := exec.Command("docker", "image", "import", tarPath, containerName)
	out, err := dockerBuildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build docker image: %s", string(out))
	}
	t.Logf("built docker image %s", containerName)
	t.Cleanup(func() {
		if err := exec.Command("docker", "image", "rm", containerName).Run(); err != nil {
			t.Logf("failed to remove docker image %s: %v", containerName, err)
		}
	})

	args := []string{"run", "--rm", "--network", "host"}
	for _, env := range getRcTesterEnv(cfg.rcHost, cfg.rcPort, cfg.tmpDir) {
		args = append(args, "--env", env)
	}
	args = append(args, containerName, "/"+binName)
	dockerCmd := exec.Command("docker", args...)
	return dockerCmd
}

func newDirectCommand(cfg sampleServiceConfig) *exec.Cmd {
	cmd := exec.Command(cfg.binaryPath)
	cmd.Env = getRcTesterEnv(cfg.rcHost, cfg.rcPort, cfg.tmpDir)
	return cmd
}

func startSampleService(
	t *testing.T, cfg sampleServiceConfig,
) (
	sampleServiceCmd *exec.Cmd,
	sampleServicePID uint32,
	serverPort int,
	err error,
) {
	var cmd *exec.Cmd
	if cfg.useDocker {
		cmd = startSampleServiceWithDocker(t, cfg)
	} else {
		cmd = newDirectCommand(cfg)
	}

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
		_ = cmd.Process.Signal(os.Interrupt)
		_ = cmd.Wait()
	})
	serverPort = waitForServicePort(t, stdoutPath)
	t.Logf("rc_tester listening on port %d", serverPort)
	// Now we want to find the relevant process ID because we might be
	// underneath docker.
	sampleServicePID = findProcessID(t, processPredicate{
		exeContains:     path.Base(cfg.binaryPath),
		environContains: fmt.Sprintf("%s=%s", e2eTmpDirEnv, cfg.tmpDir),
	})
	t.Logf("found sample service PID %d", sampleServicePID)

	return cmd, sampleServicePID, serverPort, nil
}

type processPredicate struct {
	exeContains     string
	environContains string
}

func findProcessID(t *testing.T, p processPredicate) uint32 {
	procs, err := os.ReadDir("/proc")
	if err != nil {
		t.Fatalf("failed to read /proc: %v", err)
	}
	for _, proc := range procs {
		if !proc.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(proc.Name())
		if err != nil {
			continue
		}
		exePath := filepath.Join("/proc", proc.Name(), "exe")
		exe, err := os.Readlink(exePath)
		if err != nil {
			continue
		}
		if !strings.Contains(exe, p.exeContains) {
			continue
		}
		environPath := filepath.Join("/proc", proc.Name(), "environ")
		environ, err := os.ReadFile(environPath)
		if err != nil {
			continue
		}
		if !strings.Contains(string(environ), p.environContains) {
			continue
		}
		return uint32(pid)
	}
	t.Fatalf(
		"no process found with exe %s and environ %s",
		p.exeContains,
		p.environContains,
	)
	return 0
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

func makeTargetStatus(status uploader.Status, probeIDs ...string) map[string]uploader.Status {
	m := make(map[string]uploader.Status, len(probeIDs))
	for _, probeID := range probeIDs {
		m[probeID] = status
	}
	return m
}

func waitForProbeStatus(
	t *testing.T,
	diagPayloadCh <-chan []byte,
	targetStatus map[string]uploader.Status,
) {
	t.Logf("Waiting for probes to be %s...", targetStatus)
	const timeout = 10 * time.Second

	probeStatus := make(map[string]uploader.Status)
	allInStatus := func() bool {
		for probeID, expectedStatus := range targetStatus {
			status, ok := probeStatus[probeID]
			if !ok {
				return false
			}
			if status != expectedStatus {
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
	rewrite bool,
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
			prefixMatcher("/debugger/snapshot/stack"),
			replacement(`"[stack]"`),
		))
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
			newRegexpReplacer(`(?P<datadogagent>.*)pkg/dyninst/testprogs/progs/(?P<binary>[^/]+)/.*:(?P<line>[[:digit:]]+)`),
		))
		var allRedacted []json.RawMessage
		for _, log := range processedLogs {
			allRedacted = append(allRedacted, redactJSON(t, "", log, redactors))
		}
		var err error
		content, err = json.MarshalIndent(allRedacted, "", "  ")
		require.NoError(t, err)
	}
	if expectationsPath == "" {
		return
	}

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

// Test that starting the module waits on a tombstone file.
func TestWaitOnTombstone(t *testing.T) {
	dyninsttest.SkipIfKernelNotSupported(t)

	modCfg, err := module.NewConfig(nil)
	require.NoError(t, err)

	started := make(chan struct{})
	modCfg.TestingKnobs.ProcessSubscriberOverride = func(
		subscriber module.ProcessSubscriber,
	) module.ProcessSubscriber {
		if sub, ok := subscriber.(*procsubscribe.Subscriber); ok {
			sub.Close() // prevent start from doing anything
		}
		return &fakeSubscriber{
			subscribeFunc: func(func(process.ProcessesUpdate)) {},
			startFunc: func() {
				subscriber.Start()
				close(started)
			},
		}
	}
	unblockSleep := make(chan struct{})
	modCfg.TestingKnobs.TombstoneSleepKnobs = tombstone.WaitTestingKnobs{
		BackoffPolicy: &ShortWaitPolicy{},
		OnSleep: func() {
			<-unblockSleep
		},
	}

	// Write a tombstone file, simulating a previous crash.
	dir := t.TempDir()
	tombstonePath := filepath.Join(dir, "tombstone.json")
	modCfg.ProbeTombstoneFilePath = tombstonePath
	require.NoError(t, tombstone.WriteTombstoneFile(tombstonePath, 1 /* errorNumber */))

	// Instantiate the module. This will start the ProcessSubscriber
	// asynchronously; we'll check that the starting only happens after a sleep.
	m, err := module.NewModule(modCfg, nil)
	require.NoError(t, err)
	t.Cleanup(m.Close)

	// Check that ProcessSubscriber.Start is not called before the sleep is done.
	select {
	case <-started:
		t.Fatalf("unexpected start")
	case <-time.After(10 * time.Millisecond):
	}

	// Unblock the sleep.
	close(unblockSleep)

	// Check that ProcessSubscriber.Start is called soon after.
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatalf("ProcessSubscriber.Start not called in a timely manner")
	}
}

type ShortWaitPolicy struct{}

var _ backoff.Policy = &ShortWaitPolicy{}

func (s *ShortWaitPolicy) IncError(int) int {
	return 0
}

func (s *ShortWaitPolicy) DecError(int) int {
	return 0
}

func (s *ShortWaitPolicy) GetBackoffDuration(int) time.Duration {
	return time.Millisecond
}
