// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"math/rand"
	nethttp "net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	krpretty "github.com/kr/pretty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/ebpf/prebuilt"
	eventmonitortestutil "github.com/DataDog/datadog-agent/pkg/eventmonitor/testutil"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http2"
	protocolsUtils "github.com/DataDog/datadog-agent/pkg/network/protocols/testutil"
	gotlstestutil "github.com/DataDog/datadog-agent/pkg/network/protocols/tls/gotls/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/tls/nodejs"
	usmconfig "github.com/DataDog/datadog-agent/pkg/network/usm/config"
	usmtestutil "github.com/DataDog/datadog-agent/pkg/network/usm/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	procmontestutil "github.com/DataDog/datadog-agent/pkg/process/monitor/testutil"
)

type tlsSuite struct {
	suite.Suite
}

func TestTLSSuite(t *testing.T) {
	modes := []ebpftest.BuildMode{ebpftest.RuntimeCompiled, ebpftest.CORE}
	if !prebuilt.IsDeprecated() {
		modes = append(modes, ebpftest.Prebuilt)
	}
	ebpftest.TestBuildModes(t, modes, "", func(t *testing.T) {
		if !usmconfig.TLSSupported(config.New()) {
			t.Skip("TLS not supported for this setup")
		}
		suite.Run(t, new(tlsSuite))
	})
}

func (s *tlsSuite) TestHTTPSViaLibraryIntegration() {
	t := s.T()

	cfg := config.New()
	cfg.EnableHTTPMonitoring = true
	cfg.EnableNativeTLSMonitoring = true
	/* enable protocol classification : TLS */
	cfg.ProtocolClassificationEnabled = true
	cfg.CollectTCPv4Conns = true
	cfg.CollectTCPv6Conns = true

	buildPrefetchFileBin(t)

	ldd, err := exec.LookPath("ldd")
	lddFound := err == nil

	tempFile := generateTemporaryFile(t)

	tlsLibs := []*regexp.Regexp{
		regexp.MustCompile(`/[^ ]+libssl.so[^ ]*`),
		regexp.MustCompile(`/[^ ]+libgnutls.so[^ ]*`),
	}
	tests := []struct {
		name                string
		fetchCmd            []string
		getBinaryAndCommand func(*testing.T) (string, []string, []string)
	}{
		{
			name:     "wget",
			fetchCmd: []string{"wget", "--no-check-certificate", "-O/dev/null", "--post-data", tempFile},
		},
		{
			name:     "curl",
			fetchCmd: []string{"curl", "--http1.1", "-k", "-o/dev/null", "-d", tempFile},
		},
		{
			// musl (used in, for example, Alpine Linux) uses the open(2) system
			// call to open shared libraries, unlike glibc (default in most
			// other distributions) which uses openat(2) or openat2(2).
			name:     "curl (musl)",
			fetchCmd: []string{"chroot"},
			getBinaryAndCommand: func(t *testing.T) (string, []string, []string) {
				dir, err := testutil.CurDir()
				require.NoError(t, err)

				dir = path.Join(dir, "testdata", "musl")
				protocolsUtils.RunDockerServer(t, "musl-alpine", path.Join(dir, "/docker-compose.yml"),
					nil, regexp.MustCompile("started"), protocolsUtils.DefaultTimeout, 3)

				rawout, err := exec.Command("docker", "inspect", "-f", "{{.State.Pid}}", "musl-alpine-1").Output()
				require.NoError(t, err)
				containerPid := strings.TrimSpace(string(rawout))
				containerRoot := fmt.Sprintf("/proc/%s/root", containerPid)

				// We start curl with chroot instead of via docker run since
				// docker run forks and so `testHTTPSLibrary` woudn't have the
				// PID of curl which it needs to wait for the shared library
				// monitoring to happen.
				return containerRoot, []string{"chroot", containerRoot, "ldd", "/usr/bin/curl"}, []string{"chroot", containerRoot,
					"curl", "--http1.1", "-k", "-o/dev/null", "-d", tempFile}
			},
		},
	}

	// Spin-up HTTPS server
	serverDoneFn := testutil.HTTPServer(t, "127.0.0.1:8443", testutil.Options{
		EnableTLS:       true,
		EnableKeepAlive: true,
		// Having some sleep in the response, to allow us to ensure we hooked the process.
		SlowResponse: time.Millisecond * 200,
	})
	t.Cleanup(serverDoneFn)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// The 2 checks below, could be done outside the loops, but it wouldn't mark the specific tests
			// as skipped. So we're checking it here.
			if !lddFound {
				t.Skip("ldd not found; skipping test.")
			}

			fetch, err := exec.LookPath(test.fetchCmd[0])
			commandFound := err == nil
			if !commandFound {
				t.Skipf("%s not found; skipping test.", test.fetchCmd)
			}

			root := ""
			lddCommand := []string{ldd, fetch}
			command := test.fetchCmd
			if test.getBinaryAndCommand != nil {
				root, lddCommand, command = test.getBinaryAndCommand(t)
			}

			linked, _ := exec.Command(lddCommand[0], lddCommand[1:]...).Output()

			var prefetchLibs []string
			for _, lib := range tlsLibs {
				libSSLPath := lib.FindString(string(linked))
				if libSSLPath == "" {
					continue
				}
				libSSLPath = path.Join(root, libSSLPath)
				if _, err := os.Stat(libSSLPath); err == nil {
					prefetchLibs = append(prefetchLibs, libSSLPath)
				}

			}

			if len(prefetchLibs) == 0 {
				t.Fatalf("%s not linked with any of these libs %v", test.name, tlsLibs)
			}
			testHTTPSLibrary(t, cfg, command, prefetchLibs)
		})
	}
}

func testHTTPSLibrary(t *testing.T, cfg *config.Config, fetchCmd, prefetchLibs []string) {
	usmMonitor := setupUSMTLSMonitor(t, cfg)
	// not ideal but, short process are hard to catch
	utils.WaitForProgramsToBeTraced(t, "shared_libraries", prefetchLib(t, prefetchLibs...).Process.Pid, utils.ManualTracingFallbackDisabled)

	// Issue request using fetchCmd (wget, curl, ...)
	// This is necessary (as opposed to using net/http) because we want to
	// test a HTTP client linked to OpenSSL or GnuTLS
	const targetURL = "https://127.0.0.1:8443/200/foobar"
	// Sending 3 requests to ensure we have enough time to hook the process.
	cmd := append(fetchCmd, targetURL, targetURL, targetURL)

	requestCmd := exec.Command(cmd[0], cmd[1:]...)
	stdout, err := requestCmd.StdoutPipe()
	require.NoError(t, err)
	requestCmd.Stderr = requestCmd.Stdout
	require.NoError(t, requestCmd.Start())

	utils.WaitForProgramsToBeTraced(t, "shared_libraries", requestCmd.Process.Pid, utils.ManualTracingFallbackDisabled)

	if err := requestCmd.Wait(); err != nil {
		output, err := io.ReadAll(stdout)
		if err == nil {
			t.Logf("output: %s", string(output))
		}
		t.FailNow()
	}

	fetchPid := uint32(requestCmd.Process.Pid)
	t.Logf("%s pid %d", cmd[0], fetchPid)
	assert.Eventuallyf(t, func() bool {
		stats := getHTTPLikeProtocolStats(usmMonitor, protocols.HTTP)
		if stats == nil {
			return false
		}
		for key, stats := range stats {
			if key.Path.Content.Get() != "/200/foobar" {
				continue
			}
			req, exists := stats.Data[200]
			if !exists {
				t.Errorf("http %# v stats %# v", krpretty.Formatter(key), krpretty.Formatter(stats))
				return false
			}

			statsTags := req.StaticTags
			// debian 10 have curl binary linked with openssl and gnutls but use only openssl during tls query (there no runtime flag available)
			// this make harder to map lib and tags, one set of tag should match but not both
			if statsTags == network.ConnTagGnuTLS || statsTags == network.ConnTagOpenSSL {
				t.Logf("found tag 0x%x %s", statsTags, network.GetStaticTags(statsTags))
				return true
			}
			t.Logf("HTTP stat didn't match criteria %v tags 0x%x\n", key, statsTags)
		}
		return false
	}, 5*time.Second, 100*time.Millisecond, "couldn't find USM HTTPS stats")

	if t.Failed() {
		ebpftest.DumpMapsTestHelper(t, usmMonitor.DumpMaps, "http_in_flight")
	}
}

func generateTemporaryFile(t *testing.T) string {
	tmpFile, err := os.CreateTemp("", "example")
	require.NoError(t, err)
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	_, err = tmpFile.Write(bytes.Repeat([]byte("a"), 1024*4))
	require.NoError(t, err)
	return tmpFile.Name()
}

func buildPrefetchFileBin(t *testing.T) string {
	curDir, err := testutil.CurDir()
	require.NoError(t, err)
	serverBin, err := usmtestutil.BuildGoBinaryWrapper(filepath.Join(curDir, "testutil"), "prefetch_file")
	require.NoError(t, err)
	return serverBin
}

func prefetchLib(t *testing.T, filenames ...string) *exec.Cmd {
	prefetchBin := buildPrefetchFileBin(t)
	cmd := exec.Command(prefetchBin, filenames...)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})
	return cmd
}

// TestOpenSSLVersions setups a HTTPs python server, and makes sure we are able to capture all traffic.
func (s *tlsSuite) TestOpenSSLVersions() {
	t := s.T()

	cfg := config.New()
	cfg.EnableNativeTLSMonitoring = true
	cfg.EnableHTTPMonitoring = true
	usmMonitor := setupUSMTLSMonitor(t, cfg)

	addressOfHTTPPythonServer := "127.0.0.1:8001"
	cmd := testutil.HTTPPythonServer(t, addressOfHTTPPythonServer, testutil.Options{
		EnableTLS: true,
	})

	utils.WaitForProgramsToBeTraced(t, "shared_libraries", cmd.Process.Pid, utils.ManualTracingFallbackEnabled)

	client, requestFn := simpleGetRequestsGenerator(t, addressOfHTTPPythonServer)
	var requests []*nethttp.Request
	for i := 0; i < numberOfRequests; i++ {
		requests = append(requests, requestFn())
	}

	client.CloseIdleConnections()
	requestsExist := make([]bool, len(requests))

	require.Eventually(t, func() bool {
		stats := getHTTPLikeProtocolStats(usmMonitor, protocols.HTTP)
		if stats == nil {
			return false
		}

		if len(stats) == 0 {
			return false
		}

		for reqIndex, req := range requests {
			if !requestsExist[reqIndex] {
				requestsExist[reqIndex] = isRequestIncluded(stats, req)
			}
		}

		// Slight optimization here, if one is missing, then go into another cycle of checking the new connections.
		// otherwise, if all present, abort.
		for reqIndex, exists := range requestsExist {
			if !exists {
				// reqIndex is 0 based, while the number is requests[reqIndex] is 1 based.
				t.Logf("request %d was not found (req %v)", reqIndex+1, requests[reqIndex])
				return false
			}
		}

		return true
	}, 3*time.Second, 100*time.Millisecond, "connection not found")
}

// TestOpenSSLVersionsSlowStart check we are able to capture TLS traffic even if we haven't captured the TLS handshake.
// It can happen if the agent starts after connections have been made, or agent restart (OOM/upgrade).
// Unfortunately, this is only a best-effort mechanism and it relies on some assumptions that are not always necessarily true
// such as having SSL_read/SSL_write calls in the same call-stack/execution-context as the kernel function tcp_sendmsg. Force
// this is reason the fallback behavior may require a few warmup requests before we start capturing traffic.
func (s *tlsSuite) TestOpenSSLVersionsSlowStart() {
	t := s.T()

	cfg := config.New()
	cfg.EnableNativeTLSMonitoring = true
	cfg.EnableHTTPMonitoring = true

	addressOfHTTPPythonServer := "127.0.0.1:8001"
	cmd := testutil.HTTPPythonServer(t, addressOfHTTPPythonServer, testutil.Options{
		EnableTLS: true,
	})

	client, requestFn := simpleGetRequestsGenerator(t, addressOfHTTPPythonServer)
	// Send a couple of requests we won't capture.
	var missedRequests []*nethttp.Request
	for i := 0; i < 5; i++ {
		missedRequests = append(missedRequests, requestFn())
	}

	usmMonitor := setupUSMTLSMonitor(t, cfg)
	// Giving the tracer time to install the hooks
	utils.WaitForProgramsToBeTraced(t, "shared_libraries", cmd.Process.Pid, utils.ManualTracingFallbackEnabled)

	// Send a warmup batch of requests to trigger the fallback behavior
	for i := 0; i < numberOfRequests; i++ {
		requestFn()
	}

	var requests []*nethttp.Request
	for i := 0; i < numberOfRequests; i++ {
		requests = append(requests, requestFn())
	}

	client.CloseIdleConnections()
	requestsExist := make([]bool, len(requests))
	expectedMissingRequestsCaught := make([]bool, len(missedRequests))

	require.Eventually(t, func() bool {
		stats := getHTTPLikeProtocolStats(usmMonitor, protocols.HTTP)
		if stats == nil {
			return false
		}

		if len(stats) == 0 {
			return false
		}

		for reqIndex, req := range requests {
			if !requestsExist[reqIndex] {
				requestsExist[reqIndex] = isRequestIncluded(stats, req)
			}
		}

		for reqIndex, req := range missedRequests {
			if !expectedMissingRequestsCaught[reqIndex] {
				expectedMissingRequestsCaught[reqIndex] = isRequestIncluded(stats, req)
			}
		}

		// Slight optimization here, if one is missing, then go into another cycle of checking the new connections.
		// otherwise, if all present, abort.
		for reqIndex, exists := range requestsExist {
			if !exists {
				// reqIndex is 0 based, while the number is requests[reqIndex] is 1 based.
				t.Logf("request %d was not found (req %v)", reqIndex+1, requests[reqIndex])
				return false
			}
		}

		return true
	}, 3*time.Second, 100*time.Millisecond, "connection not found")

	// Here we intend to check if we catch requests we should not have caught
	// Thus, if an expected missing requests - exists, thus there is a problem.
	for reqIndex, exist := range expectedMissingRequestsCaught {
		require.Falsef(t, exist, "request %d was not meant to be captured found (req %v) but we captured it", reqIndex+1, requests[reqIndex])
	}
}

const (
	numberOfRequests = 100
)

// TODO: Get rid of it, in favor of `requestGenerator`
func simpleGetRequestsGenerator(t *testing.T, targetAddr string) (*nethttp.Client, func() *nethttp.Request) {
	var (
		random = rand.New(rand.NewSource(time.Now().Unix()))
		idx    = 0
		client = &nethttp.Client{
			Transport: &nethttp.Transport{
				TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
				DisableKeepAlives: false,
			},
		}
	)

	return client, func() *nethttp.Request {
		idx++
		status := statusCodes[random.Intn(len(statusCodes))]
		req, err := nethttp.NewRequest(nethttp.MethodGet, fmt.Sprintf("https://%s/status/%d/request-%d", targetAddr, status, idx), nil)
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.NoError(t, err)
		require.Equal(t, status, resp.StatusCode)
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return req
	}
}

// TODO: Get rid of it, in favor of `includesRequest`
func isRequestIncluded(allStats map[http.Key]*http.RequestStats, req *nethttp.Request) bool {
	expectedStatus := testutil.StatusFromPath(req.URL.Path)
	for key, stats := range allStats {
		if key.Path.Content.Get() != req.URL.Path {
			continue
		}
		if requests, exists := stats.Data[expectedStatus]; exists && requests.Count > 0 {
			return true
		}
	}

	return false
}

func TestHTTPGoTLSAttachProbes(t *testing.T) {
	t.Skip("skipping GoTLS tests while we investigate their flakiness")

	modes := []ebpftest.BuildMode{ebpftest.RuntimeCompiled, ebpftest.CORE}
	ebpftest.TestBuildModes(t, modes, "", func(t *testing.T) {
		if !gotlstestutil.GoTLSSupported(t, config.New()) {
			t.Skip("GoTLS not supported for this setup")
		}

		t.Run("new process", func(t *testing.T) {
			testHTTPGoTLSCaptureNewProcess(t, config.New(), false)
		})
		t.Run("already running process", func(t *testing.T) {
			testHTTPGoTLSCaptureAlreadyRunning(t, config.New(), false)
		})
	})
}

func testHTTP2GoTLSAttachProbes(t *testing.T, cfg *config.Config) {
	modes := []ebpftest.BuildMode{ebpftest.RuntimeCompiled, ebpftest.CORE}
	ebpftest.TestBuildModes(t, modes, "", func(t *testing.T) {
		if !http2.Supported() {
			t.Skip("HTTP2 not supported for this setup")
		}
		if !gotlstestutil.GoTLSSupported(t, cfg) {
			t.Skip("GoTLS not supported for this setup")
		}

		t.Run("new process", func(tt *testing.T) {
			testHTTPGoTLSCaptureNewProcess(tt, cfg, true)
		})
		t.Run("already running process", func(tt *testing.T) {
			testHTTPGoTLSCaptureAlreadyRunning(tt, cfg, true)
		})
	})
}

func TestHTTP2GoTLSAttachProbes(t *testing.T) {
	t.Run("netlink",
		func(tt *testing.T) {
			cfg := config.New()
			cfg.EnableUSMEventStream = false
			testHTTP2GoTLSAttachProbes(tt, cfg)
		})
	t.Run("event stream",
		func(tt *testing.T) {
			cfg := config.New()
			cfg.EnableUSMEventStream = true
			testHTTP2GoTLSAttachProbes(tt, cfg)
		})
}

func TestHTTPSGoTLSAttachProbesOnContainer(t *testing.T) {
	t.Skip("Skipping a flaky test")
	modes := []ebpftest.BuildMode{ebpftest.RuntimeCompiled, ebpftest.CORE}
	ebpftest.TestBuildModes(t, modes, "", func(t *testing.T) {
		if !gotlstestutil.GoTLSSupported(t, config.New()) {
			t.Skip("GoTLS not supported for this setup")
		}

		t.Run("new process", func(t *testing.T) {
			testHTTPSGoTLSCaptureNewProcessContainer(t, config.New())
		})
		t.Run("already running process", func(t *testing.T) {
			testHTTPSGoTLSCaptureAlreadyRunningContainer(t, config.New())
		})
	})
}

func TestOldConnectionRegression(t *testing.T) {
	t.Skip("skipping this test for now while we investigate the errors on debian-10-x86 and ubuntu-18.04-x86")

	modes := []ebpftest.BuildMode{ebpftest.RuntimeCompiled, ebpftest.CORE}
	ebpftest.TestBuildModes(t, modes, "", func(t *testing.T) {
		if !gotlstestutil.GoTLSSupported(t, config.New()) {
			t.Skip("GoTLS not supported for this setup")
		}

		// Spin up HTTP server
		const serverAddr = "127.0.0.1:8081"
		const httpPath = "/200/foobar"
		closeServer := testutil.HTTPServer(t, serverAddr, testutil.Options{
			EnableTLS:       true,
			EnableKeepAlive: true,
		})
		t.Cleanup(closeServer)

		// Create a TLS connection *before* starting the USM monitor
		// This is the main purpose of this test: verifying that GoTLS
		// monitoring works for connections initiated prior to USM monitor.
		tlsConfig := &tls.Config{InsecureSkipVerify: true}
		conn, err := tls.Dial("tcp", serverAddr, tlsConfig)
		require.NoError(t, err)
		defer conn.Close()

		// Start USM monitor
		cfg := config.New()
		cfg.EnableHTTPMonitoring = true
		cfg.EnableGoTLSSupport = true
		cfg.GoTLSExcludeSelf = false
		usmMonitor := setupUSMTLSMonitor(t, cfg)

		// Ensure this test program is being traced
		utils.WaitForProgramsToBeTraced(t, GoTLSAttacherName, os.Getpid(), utils.ManualTracingFallbackEnabled)

		// The HTTPServer used here effectively works as an "echo" servers and
		// returns back in the response whatever it received in the request
		// body. Here we add a `$` to the request body as a way to delimit the
		// end of the http response since we can't rely on EOFs for the code
		// below because we're sending multiple requests over the same socket.
		requestBody := fmt.Sprintf("GET %s HTTP/1.1\nHost: %s\n\n$", httpPath, serverAddr)

		// Create a bufio.Reader to help with reading until the delimiter
		// mentioned above.
		reader := bufio.NewReader(conn)

		// Issue multiple HTTP requests
		// NOTE: This is a temporary hack to avoid test flakiness because
		// currently the TLS.Close() codepath may fail due to a race condition
		// in which the `protocol_stack` object is deleted before the
		// termination code runs. By issuing a multiple requests on the same socket
		// we force the previous ones to be flushed.
		for i := 0; i < 10; i++ {
			conn.Write([]byte(requestBody))
			_, err := reader.ReadBytes('$')
			if err != nil {
				break
			}
		}

		// Ensure we have captured a request
		stats, ok := usmMonitor.GetProtocolStats()[protocols.HTTP]
		require.True(t, ok)
		httpStats, ok := stats.(map[http.Key]*http.RequestStats)
		require.True(t, ok)
		assert.Condition(t, func() bool {
			for key := range httpStats {
				if key.Path.Content.Get() == httpPath {
					return true
				}
			}
			return false
		})
	})
}

func TestLimitListenerRegression(t *testing.T) {
	modes := []ebpftest.BuildMode{ebpftest.RuntimeCompiled, ebpftest.CORE}
	ebpftest.TestBuildModes(t, modes, "", func(t *testing.T) {
		if !gotlstestutil.GoTLSSupported(t, config.New()) {
			t.Skip("GoTLS not supported for this setup")
		}

		// Spin up HTTP server
		const serverAddr = "127.0.0.1:8081"
		const httpPath = "/200/foobar"
		closeServer := testutil.HTTPServer(t, serverAddr, testutil.Options{
			EnableTLS:           true,
			EnableLimitListener: true,
		})
		t.Cleanup(closeServer)

		// Start USM monitor
		cfg := config.New()
		cfg.EnableHTTPMonitoring = true
		cfg.EnableGoTLSSupport = true
		cfg.GoTLSExcludeSelf = false
		// This one is particularly important for this test so we ensure we
		// don't accidentally report a false positive based on client (`curl`)
		// data as opposed to the GoTLS server with `netutils.LimitListener`
		cfg.EnableNativeTLSMonitoring = false
		usmMonitor := setupUSMTLSMonitor(t, cfg)

		// Ensure this test program is being traced
		utils.WaitForProgramsToBeTraced(t, GoTLSAttacherName, os.Getpid(), utils.ManualTracingFallbackEnabled)

		// Issue multiple HTTP requests
		for i := 0; i < 10; i++ {
			cmd := exec.Command("curl", "-k", "--http1.1", fmt.Sprintf("https://%s%s", serverAddr, httpPath))
			err := cmd.Run()
			assert.NoError(t, err)
		}

		// Ensure we have captured a request
		stats, ok := usmMonitor.GetProtocolStats()[protocols.HTTP]
		require.True(t, ok)
		httpStats, ok := stats.(map[http.Key]*http.RequestStats)
		require.True(t, ok)
		assert.Condition(t, func() bool {
			for key := range httpStats {
				if key.Path.Content.Get() == httpPath {
					return true
				}
			}
			return false
		})
	})
}

// Test that we can capture HTTPS traffic from Go processes started after the
// tracer.
func testHTTPGoTLSCaptureNewProcess(t *testing.T, cfg *config.Config, isHTTP2 bool) {
	const (
		serverAddr          = "localhost:8081"
		expectedOccurrences = 10
	)

	// Setup
	closeServer := testutil.HTTPServer(t, serverAddr, testutil.Options{
		EnableTLS:       true,
		EnableKeepAlive: false,
		EnableHTTP2:     isHTTP2,
	})
	t.Cleanup(closeServer)

	cfg.EnableGoTLSSupport = true
	if isHTTP2 {
		cfg.EnableHTTP2Monitoring = true
	} else {
		cfg.EnableHTTPMonitoring = true
	}

	usmMonitor := setupUSMTLSMonitor(t, cfg)

	// This maps will keep track of whether the tracer saw this request already or not
	reqs := make(requestsMap)
	for i := 0; i < expectedOccurrences; i++ {
		req, err := nethttp.NewRequest(nethttp.MethodGet, fmt.Sprintf("https://%s/%d/request-%d", serverAddr, nethttp.StatusOK, i), nil)
		require.NoError(t, err)
		reqs[req] = false
	}

	// spin-up goTLS client and issue requests after initialization
	command, runRequests := gotlstestutil.NewGoTLSClient(t, serverAddr, expectedOccurrences, isHTTP2)
	utils.WaitForProgramsToBeTraced(t, GoTLSAttacherName, command.Process.Pid, utils.ManualTracingFallbackEnabled)
	runRequests()
	checkRequests(t, usmMonitor, expectedOccurrences, reqs, isHTTP2)
}

func testHTTPGoTLSCaptureAlreadyRunning(t *testing.T, cfg *config.Config, isHTTP2 bool) {
	const (
		serverAddr          = "localhost:8081"
		expectedOccurrences = 10
	)

	// Setup
	closeServer := testutil.HTTPServer(t, serverAddr, testutil.Options{
		EnableTLS:   true,
		EnableHTTP2: isHTTP2,
	})
	t.Cleanup(closeServer)

	cfg.EnableGoTLSSupport = true
	if isHTTP2 {
		cfg.EnableHTTP2Monitoring = true
	} else {
		cfg.EnableHTTPMonitoring = true
	}
	// spin-up goTLS client but don't issue requests yet
	command, issueRequestsFn := gotlstestutil.NewGoTLSClient(t, serverAddr, expectedOccurrences, isHTTP2)

	usmMonitor := setupUSMTLSMonitor(t, cfg)

	// This maps will keep track of whether the tracer saw this request already or not
	reqs := make(requestsMap)
	for i := 0; i < expectedOccurrences; i++ {
		req, err := nethttp.NewRequest(nethttp.MethodGet, fmt.Sprintf("https://%s/%d/request-%d", serverAddr, nethttp.StatusOK, i), nil)
		require.NoError(t, err)
		reqs[req] = false
	}

	utils.WaitForProgramsToBeTraced(t, GoTLSAttacherName, command.Process.Pid, utils.ManualTracingFallbackEnabled)
	issueRequestsFn()
	checkRequests(t, usmMonitor, expectedOccurrences, reqs, isHTTP2)
}

func testHTTPSGoTLSCaptureNewProcessContainer(t *testing.T, cfg *config.Config) {
	const (
		serverPort          = "8443"
		expectedOccurrences = 10
	)

	// problems with aggregation
	client := &nethttp.Client{
		Transport: &nethttp.Transport{
			TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
			DisableKeepAlives: false,
		},
	}

	// Setup
	cfg.EnableGoTLSSupport = true
	cfg.EnableHTTPMonitoring = true

	usmMonitor := setupUSMTLSMonitor(t, cfg)

	require.NoError(t, gotlstestutil.RunServer(t, serverPort))
	reqs := make(requestsMap)
	for i := 0; i < expectedOccurrences; i++ {
		resp, err := client.Get(fmt.Sprintf("https://localhost:%s/status/%d", serverPort, 200+i))
		require.NoError(t, err)
		resp.Body.Close()
		reqs[resp.Request] = false
	}

	client.CloseIdleConnections()
	checkRequests(t, usmMonitor, expectedOccurrences, reqs, false)
}

func testHTTPSGoTLSCaptureAlreadyRunningContainer(t *testing.T, cfg *config.Config) {
	const (
		serverPort          = "8443"
		expectedOccurrences = 10
	)

	require.NoError(t, gotlstestutil.RunServer(t, serverPort))

	client := &nethttp.Client{
		Transport: &nethttp.Transport{
			TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
			DisableKeepAlives: false,
		},
	}

	// Setup
	cfg.EnableGoTLSSupport = true
	cfg.EnableHTTPMonitoring = true

	usmMonitor := setupUSMTLSMonitor(t, cfg)

	reqs := make(requestsMap)
	for i := 0; i < expectedOccurrences; i++ {
		resp, err := client.Get(fmt.Sprintf("https://localhost:%s/status/%d", serverPort, 200+i))
		require.NoError(t, err)
		resp.Body.Close()
		reqs[resp.Request] = false
	}

	client.CloseIdleConnections()
	checkRequests(t, usmMonitor, expectedOccurrences, reqs, false)
}

func checkRequests(t *testing.T, usmMonitor *Monitor, expectedOccurrences int, reqs requestsMap, isHTTP2 bool) {
	t.Helper()

	occurrences := PrintableInt(0)
	require.Eventually(t, func() bool {
		protocolType := protocols.HTTP
		if isHTTP2 {
			protocolType = protocols.HTTP2
		}
		stats := getHTTPLikeProtocolStats(usmMonitor, protocolType)
		occurrences += PrintableInt(countRequestsOccurrences(t, stats, reqs))
		return int(occurrences) == expectedOccurrences
	}, 3*time.Second, 100*time.Millisecond, "Expected to find the request %v times, got %v captured. Requests not found:\n%v", expectedOccurrences, &occurrences, reqs)
}

func countRequestsOccurrences(t *testing.T, conns map[http.Key]*http.RequestStats, reqs map[*nethttp.Request]bool) (occurrences int) {
	t.Helper()

	for key, stats := range conns {
		for req, found := range reqs {
			if found {
				continue
			}

			expectedStatus := testutil.StatusFromPath(req.URL.Path)
			if key.Path.Content.Get() != req.URL.Path {
				continue
			}
			if requests, exists := stats.Data[expectedStatus]; exists && requests.Count > 0 {
				occurrences++
				reqs[req] = true
				break
			}
		}
	}

	return
}

type requestsMap map[*nethttp.Request]bool

func (m requestsMap) String() string {
	var result strings.Builder

	for req, found := range m {
		if found {
			continue
		}
		result.WriteString(fmt.Sprintf("\t- %v\n", req.URL.Path))
	}

	return result.String()
}

func setupUSMTLSMonitor(t *testing.T, cfg *config.Config) *Monitor {
	usmMonitor, err := NewMonitor(cfg, nil)
	require.NoError(t, err)
	require.NoError(t, usmMonitor.Start())
	if cfg.EnableUSMEventStream {
		eventmonitortestutil.StartEventMonitor(t, procmontestutil.RegisterProcessMonitorEventConsumer)
	}
	t.Cleanup(usmMonitor.Stop)
	t.Cleanup(utils.ResetDebugger)
	return usmMonitor
}

// getHTTPLikeProtocolStats returns the stats for the protocols that store their stats in a map of http.Key and *http.RequestStats as values.
func getHTTPLikeProtocolStats(monitor *Monitor, protocolType protocols.ProtocolType) map[http.Key]*http.RequestStats {
	httpStats, ok := monitor.GetProtocolStats()[protocolType]
	if !ok {
		return nil
	}
	res, ok := httpStats.(map[http.Key]*http.RequestStats)
	if !ok {
		return nil
	}
	return res
}

func (s *tlsSuite) TestNodeJSTLS() {
	const (
		expectedOccurrences = 10
		serverPort          = "4444"
	)

	t := s.T()

	cert, key, err := testutil.GetCertsPaths()
	require.NoError(t, err)

	require.NoError(t, nodejs.RunServerNodeJS(t, key, cert, serverPort))
	nodeJSPID, err := nodejs.GetNodeJSDockerPID()
	require.NoError(t, err)

	cfg := config.New()
	cfg.EnableHTTPMonitoring = true
	cfg.EnableNodeJSMonitoring = true

	usmMonitor := setupUSMTLSMonitor(t, cfg)
	utils.WaitForProgramsToBeTraced(t, "nodejs", int(nodeJSPID), utils.ManualTracingFallbackEnabled)

	// This maps will keep track of whether the tracer saw this request already or not
	client, requestFn := simpleGetRequestsGenerator(t, fmt.Sprintf("localhost:%s", serverPort))
	var requests []*nethttp.Request
	for i := 0; i < expectedOccurrences; i++ {
		requests = append(requests, requestFn())
	}

	client.CloseIdleConnections()
	requestsExist := make([]bool, len(requests))

	assert.Eventually(t, func() bool {
		stats := getHTTPLikeProtocolStats(usmMonitor, protocols.HTTP)
		if stats == nil {
			return false
		}

		if len(stats) == 0 {
			return false
		}

		for reqIndex, req := range requests {
			if !requestsExist[reqIndex] {
				requestsExist[reqIndex] = isRequestIncluded(stats, req)
			}
		}

		// Slight optimization here, if one is missing, then go into another cycle of checking the new connections.
		// otherwise, if all present, abort.
		for reqIndex, exists := range requestsExist {
			if !exists {
				// reqIndex is 0 based, while the number is requests[reqIndex] is 1 based.
				t.Logf("request %d was not found (req %v)", reqIndex+1, requests[reqIndex])
				return false
			}
		}

		return true
	}, 3*time.Second, 100*time.Millisecond, "connection not found")

	for reqIndex, exists := range requestsExist {
		if !exists {
			// reqIndex is 0 based, while the number is requests[reqIndex] is 1 based.
			t.Logf("request %d was not found (req %v)", reqIndex+1, requests[reqIndex])
		}
	}
}
