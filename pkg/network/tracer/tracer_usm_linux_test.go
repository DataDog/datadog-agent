// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package tracer

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"math/rand"
	"net"
	nethttp "net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/DataDog/gopsutil/host"
	krpretty "github.com/kr/pretty"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netlink "github.com/DataDog/datadog-agent/pkg/network/netlink/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	gotlstestutil "github.com/DataDog/datadog-agent/pkg/network/protocols/tls/gotls/testutil"
	javatestutil "github.com/DataDog/datadog-agent/pkg/network/protocols/tls/java/testutil"
	prototls "github.com/DataDog/datadog-agent/pkg/network/protocols/tls/openssl"
	nettestutil "github.com/DataDog/datadog-agent/pkg/network/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/kprobe"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/testutil/grpc"
)

func httpSupported() bool {
	if isFentry() {
		return false
	}
	// kv is declared in `tracer_linux_test.go`.
	return kv >= http.MinimumKernelVersion
}

func httpsSupported() bool {
	if isFentry() {
		return false
	}
	return http.HTTPSSupported(testConfig())
}

func goTLSSupported() bool {
	if !httpsSupported() {
		return false
	}
	cfg := config.New()
	return cfg.EnableRuntimeCompiler || cfg.EnableCORE
}

func classificationSupported(config *config.Config) bool {
	return kprobe.ClassificationSupported(config)
}

type USMSuite struct {
	suite.Suite
}

func TestUSMSuite(t *testing.T) {
	ebpftest.TestBuildModes(t, []ebpftest.BuildMode{ebpftest.Prebuilt, ebpftest.RuntimeCompiled, ebpftest.CORE}, "", func(t *testing.T) {
		suite.Run(t, new(USMSuite))
	})
}

func (s *USMSuite) TestEnableHTTPMonitoring() {
	t := s.T()
	if !httpSupported() {
		t.Skip("HTTP monitoring not supported")
	}

	cfg := testConfig()
	cfg.EnableHTTPMonitoring = true
	_ = setupTracer(t, cfg)
}

func (s *USMSuite) TestHTTPStats() {
	t := s.T()
	t.Run("status code", func(t *testing.T) {
		testHTTPStats(t, true)
	})
	t.Run("status class", func(t *testing.T) {
		testHTTPStats(t, false)
	})
}

func testHTTPStats(t *testing.T, aggregateByStatusCode bool) {
	if !httpSupported() {
		t.Skip("HTTP monitoring feature not available")
		return
	}

	cfg := testConfig()
	cfg.EnableHTTPMonitoring = true
	cfg.EnableHTTPStatsByStatusCode = aggregateByStatusCode
	tr := setupTracer(t, cfg)

	// Start an HTTP server on localhost:8080
	serverAddr := "127.0.0.1:8080"
	srv := &nethttp.Server{
		Addr: serverAddr,
		Handler: nethttp.HandlerFunc(func(w nethttp.ResponseWriter, req *nethttp.Request) {
			io.Copy(io.Discard, req.Body)
			w.WriteHeader(204)
		}),
		ReadTimeout:  time.Second,
		WriteTimeout: time.Second,
	}
	srv.SetKeepAlivesEnabled(false)
	go func() { _ = srv.ListenAndServe() }()
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })

	// Allow the HTTP server time to get set up
	time.Sleep(time.Millisecond * 500)

	// Send a series of HTTP requests to the test server
	resp, err := nethttp.Get("http://" + serverAddr + "/test")
	require.NoError(t, err)
	_ = resp.Body.Close()
	// Iterate through active connections until we find connection created above
	require.Eventuallyf(t, func() bool {
		payload := getConnections(t, tr)
		for key, stats := range payload.HTTP {
			if key.Method == http.MethodGet && key.Path.Content == "/test" && (key.SrcPort == 8080 || key.DstPort == 8080) {
				currentStats := stats.Data[stats.NormalizeStatusCode(204)]
				if currentStats != nil && currentStats.Count == 1 {
					return true
				}
			}
		}

		return false
	}, 3*time.Second, 10*time.Millisecond, "couldn't find http connection matching: %s", serverAddr)
}

func (s *USMSuite) TestHTTPSViaLibraryIntegration() {
	t := s.T()
	if !httpsSupported() {
		t.Skip("HTTPS feature not available/supported for this setup")
	}

	buildPrefetchFileBin(t)

	ldd, err := exec.LookPath("ldd")
	lddFound := err == nil

	tlsLibs := []*regexp.Regexp{
		regexp.MustCompile(`/[^\ ]+libssl.so[^\ ]*`),
		regexp.MustCompile(`/[^\ ]+libgnutls.so[^\ ]*`),
	}
	tests := []struct {
		name         string
		fetchCmd     []string
		prefetchLibs []string
		commandFound bool
	}{
		{
			name:     "wget",
			fetchCmd: []string{"wget", "--no-check-certificate", "-O/dev/null"},
		},
		{
			name:     "curl",
			fetchCmd: []string{"curl", "--http1.1", "-k", "-o/dev/null"},
		},
	}

	if lddFound {
		for index := range tests {
			fetch, err := exec.LookPath(tests[index].fetchCmd[0])
			tests[index].commandFound = err == nil
			if !tests[index].commandFound {
				continue
			}
			linked, _ := exec.Command(ldd, fetch).Output()

			for _, lib := range tlsLibs {
				libSSLPath := lib.FindString(string(linked))
				if _, err := os.Stat(libSSLPath); err == nil {
					tests[index].prefetchLibs = append(tests[index].prefetchLibs, libSSLPath)
				}
			}
		}
	}

	for _, keepAlive := range []struct {
		name  string
		value bool
	}{
		{
			name:  "without keep-alive",
			value: false,
		},
		{
			name:  "with keep-alive",
			value: true,
		},
	} {
		t.Run(keepAlive.name, func(t *testing.T) {
			// Spin-up HTTPS server
			serverDoneFn := testutil.HTTPServer(t, "127.0.0.1:443", testutil.Options{
				EnableTLS:       true,
				EnableKeepAlive: keepAlive.value,
			})
			t.Cleanup(serverDoneFn)

			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					// The 2 checks below, could be done outside the loops, but it wouldn't mark the specific tests
					// as skipped. So we're checking it here.
					if !lddFound {
						t.Skip("ldd not found; skipping test.")
					}
					if !test.commandFound {
						t.Skipf("%s not found; skipping test.", test.fetchCmd)
					}
					if len(test.prefetchLibs) == 0 {
						t.Fatalf("%s not linked with any of these libs %v", test.name, tlsLibs)
					}
					testHTTPSLibrary(t, test.fetchCmd, test.prefetchLibs)
				})
			}
		})
	}
}

func buildPrefetchFileBin(t *testing.T) string {
	const srcPath = "prefetch_file"
	const binaryPath = "testutil/prefetch_file/prefetch_file"

	t.Helper()

	cur, err := testutil.CurDir()
	require.NoError(t, err)

	binary := fmt.Sprintf("%s/%s", cur, binaryPath)
	// If there is a compiled binary already, skip the compilation.
	// Meant for the CI.
	if _, err = os.Stat(binary); err == nil {
		return binary
	}

	srcDir := fmt.Sprintf("%s/testutil/%s", cur, srcPath)

	c := exec.Command("go", "build", "-buildvcs=false", "-a", "-ldflags=-extldflags '-static'", "-o", binary, srcDir)
	out, err := c.CombinedOutput()
	t.Log(c, string(out))
	require.NoError(t, err, "could not build test binary: %s\noutput: %s", err, string(out))

	return binary
}

func prefetchLib(t *testing.T, filename string) {
	prefetchBin := buildPrefetchFileBin(t)
	cmd := exec.Command(prefetchBin, filename, "3s")
	require.NoError(t, cmd.Start())
}

func testHTTPSLibrary(t *testing.T, fetchCmd []string, prefetchLibs []string) {
	// Start tracer with HTTPS support
	cfg := testConfig()
	cfg.EnableHTTPMonitoring = true
	cfg.EnableNativeTLSMonitoring = true
	/* enable protocol classification : TLS */
	cfg.ProtocolClassificationEnabled = true
	cfg.CollectTCPv4Conns = true
	cfg.CollectTCPv6Conns = true
	tr := setupTracer(t, cfg)

	// not ideal but, short process are hard to catch
	for _, lib := range prefetchLibs {
		prefetchLib(t, lib)
	}
	time.Sleep(2 * time.Second)

	// Issue request using fetchCmd (wget, curl, ...)
	// This is necessary (as opposed to using net/http) because we want to
	// test a HTTP client linked to OpenSSL or GnuTLS
	const targetURL = "https://127.0.0.1:443/200/foobar"
	cmd := append(fetchCmd, targetURL)

	t.Log("run 3 clients request as we can have a race between the closing tcp socket and the http response")
	fetchPids := make(map[uint32]struct{})
	for i := 0; i < 3; i++ {
		requestCmd := exec.Command(cmd[0], cmd[1:]...)
		out, err := requestCmd.CombinedOutput()
		require.NoErrorf(t, err, "failed to issue request via %s: %s\n%s", fetchCmd, err, string(out))
		fetchPid := uint32(requestCmd.Process.Pid)
		fetchPids[fetchPid] = struct{}{}
		t.Logf("%s pid %d", cmd[0], fetchPid)
	}

	var allConnections []network.ConnectionStats
	httpKeys := make(map[uint16]http.Key)
	require.Eventuallyf(t, func() bool {
		payload := getConnections(t, tr)
		allConnections = append(allConnections, payload.Conns...)
		found := false
		for key, stats := range payload.HTTP {
			if key.Path.Content != "/200/foobar" {
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
				httpKeys[key.SrcPort] = key
				found = true
				continue
			} else {
				s, _ := tr.getStats(allStats...)
				t.Logf("==== %# v\n%# v", krpretty.Formatter(req), krpretty.Formatter(s))
			}
			if len(httpKeys) == 3 {
				return true
			}
			t.Logf("HTTP stat didn't match criteria %v tags 0x%x\n", key, statsTags)
		}

		if !found {
			s, _ := tr.getStats(allStats...)
			t.Logf("=====loop= %# v", krpretty.Formatter(s))
		}
		return found
	}, 15*time.Second, 5*time.Second, "couldn't find USM HTTPS stats")

	// check NPM static TLS tag
	found := false
	for _, c := range allConnections {
		httpKey, foundKey := httpKeys[c.SPort]
		if !foundKey {
			continue
		}
		_, foundPid := fetchPids[c.Pid]
		if foundPid && c.DPort == httpKey.DstPort && c.ProtocolStack.Contains(protocols.TLS) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("NPM TLS tag not found")
		for _, c := range allConnections {
			httpKey, foundKey := httpKeys[c.SPort]
			if !foundKey {
				continue
			}
			_, foundPid := fetchPids[c.Pid]
			if foundPid {
				t.Logf("pid %d connection %# v \nhttp %# v\n", c.Pid, krpretty.Formatter(c), krpretty.Formatter(httpKey))
			}
		}
	}
}

const (
	numberOfRequests = 100
)

// TestOpenSSLVersions setups a HTTPs python server, and makes sure we are able to capture all traffic.
func (s *USMSuite) TestOpenSSLVersions() {
	t := s.T()
	if !httpsSupported() {
		t.Skip("HTTPS feature not available/supported for this setup")
	}

	cfg := testConfig()
	cfg.EnableNativeTLSMonitoring = true
	cfg.EnableHTTPMonitoring = true
	tr := setupTracer(t, cfg)

	addressOfHTTPPythonServer := "127.0.0.1:8001"
	closer, err := testutil.HTTPPythonServer(t, addressOfHTTPPythonServer, testutil.Options{
		EnableTLS: true,
	})
	require.NoError(t, err)
	defer closer()

	// Giving the tracer time to install the hooks
	time.Sleep(time.Second)
	client, requestFn := simpleGetRequestsGenerator(t, addressOfHTTPPythonServer)
	var requests []*nethttp.Request
	for i := 0; i < numberOfRequests; i++ {
		requests = append(requests, requestFn())
	}

	client.CloseIdleConnections()
	requestsExist := make([]bool, len(requests))

	require.Eventually(t, func() bool {
		conns := getConnections(t, tr)

		if len(conns.HTTP) == 0 {
			return false
		}

		for reqIndex, req := range requests {
			if !requestsExist[reqIndex] {
				requestsExist[reqIndex] = isRequestIncluded(conns.HTTP, req)
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
	}, 3*time.Second, time.Second, "connection not found")
}

// TestOpenSSLVersionsSlowStart check we are able to capture TLS traffic even if we haven't captured the TLS handshake.
// It can happen if the agent starts after connections have been made, or agent restart (OOM/upgrade).
// Unfortunately, this is only a best-effort mechanism and it relies on some assumptions that are not always necessarily true
// such as having SSL_read/SSL_write calls in the same call-stack/execution-context as the kernel function tcp_sendmsg. Force
// this is reason the fallback behavior may require a few warmup requests before we start capturing traffic.
func (s *USMSuite) TestOpenSSLVersionsSlowStart() {
	t := s.T()
	if !httpsSupported() {
		t.Skip("HTTPS feature not available/supported for this setup")
	}

	cfg := testConfig()
	cfg.EnableNativeTLSMonitoring = true
	cfg.EnableHTTPMonitoring = true

	addressOfHTTPPythonServer := "127.0.0.1:8001"
	closer, err := testutil.HTTPPythonServer(t, addressOfHTTPPythonServer, testutil.Options{
		EnableTLS: true,
	})
	require.NoError(t, err)
	t.Cleanup(closer)

	client, requestFn := simpleGetRequestsGenerator(t, addressOfHTTPPythonServer)
	// Send a couple of requests we won't capture.
	var missedRequests []*nethttp.Request
	for i := 0; i < 5; i++ {
		missedRequests = append(missedRequests, requestFn())
	}

	tr := setupTracer(t, cfg)

	// Giving the tracer time to install the hooks
	time.Sleep(time.Second)

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
		conns := getConnections(t, tr)

		if len(conns.HTTP) == 0 {
			return false
		}

		for reqIndex, req := range requests {
			if !requestsExist[reqIndex] {
				requestsExist[reqIndex] = isRequestIncluded(conns.HTTP, req)
			}
		}

		for reqIndex, req := range missedRequests {
			if !expectedMissingRequestsCaught[reqIndex] {
				expectedMissingRequestsCaught[reqIndex] = isRequestIncluded(conns.HTTP, req)
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
	}, 3*time.Second, time.Second, "connection not found")

	// Here we intend to check if we catch requests we should not have caught
	// Thus, if an expected missing requests - exists, thus there is a problem.
	for reqIndex, exist := range expectedMissingRequestsCaught {
		require.Falsef(t, exist, "request %d was not meant to be captured found (req %v) but we captured it", reqIndex+1, requests[reqIndex])
	}
}

var (
	statusCodes = []int{nethttp.StatusOK, nethttp.StatusMultipleChoices, nethttp.StatusBadRequest, nethttp.StatusInternalServerError}
)

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
		req, err := nethttp.NewRequest(nethttp.MethodGet, fmt.Sprintf("https://%s/%d/request-%d", targetAddr, status, idx), nil)
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.NoError(t, err)
		require.Equal(t, status, resp.StatusCode)
		io.ReadAll(resp.Body)
		resp.Body.Close()
		return req
	}
}

func isRequestIncluded(allStats map[http.Key]*http.RequestStats, req *nethttp.Request) bool {
	expectedStatus := testutil.StatusFromPath(req.URL.Path)
	for key, stats := range allStats {
		if key.Path.Content != req.URL.Path {
			continue
		}
		if requests, exists := stats.Data[expectedStatus]; exists && requests.Count > 0 {
			return true
		}
	}

	return false
}

func (s *USMSuite) TestProtocolClassification() {
	t := s.T()
	cfg := testConfig()
	if !classificationSupported(cfg) {
		t.Skip("Classification is not supported")
	}

	cfg.EnableGoTLSSupport = true
	cfg.EnableNativeTLSMonitoring = true
	cfg.EnableHTTPMonitoring = true
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	t.Cleanup(tr.Stop)

	t.Run("with dnat", func(t *testing.T) {
		// SetupDNAT sets up a NAT translation from 2.2.2.2 to 1.1.1.1
		netlink.SetupDNAT(t)
		testProtocolClassification(t, tr, "localhost", "2.2.2.2", "1.1.1.1")
		testHTTPSClassification(t, tr, "localhost", "2.2.2.2", "1.1.1.1")
		testProtocolConnectionProtocolMapCleanup(t, tr, "localhost", "2.2.2.2", "1.1.1.1:0")
	})

	t.Run("with snat", func(t *testing.T) {
		// SetupDNAT sets up a NAT translation from 6.6.6.6 to 7.7.7.7
		netlink.SetupSNAT(t)
		testProtocolClassification(t, tr, "6.6.6.6", "127.0.0.1", "127.0.0.1")
		testHTTPSClassification(t, tr, "6.6.6.6", "127.0.0.1", "127.0.0.1")
		testProtocolConnectionProtocolMapCleanup(t, tr, "6.6.6.6", "127.0.0.1", "127.0.0.1:0")
	})

	t.Run("without nat", func(t *testing.T) {
		testProtocolClassification(t, tr, "localhost", "127.0.0.1", "127.0.0.1")
		testHTTPSClassification(t, tr, "localhost", "127.0.0.1", "127.0.0.1")
		testProtocolConnectionProtocolMapCleanup(t, tr, "localhost", "127.0.0.1", "127.0.0.1:0")
	})
}

func testProtocolConnectionProtocolMapCleanup(t *testing.T, tr *Tracer, clientHost, targetHost, serverHost string) {
	t.Run("protocol cleanup", func(t *testing.T) {
		if tr.ebpfTracer.Type() == connection.TracerTypeFentry {
			t.Skip("protocol classification not supported for fentry tracer")
		}
		t.Cleanup(func() { tr.ebpfTracer.Pause() })

		dialer := &net.Dialer{
			LocalAddr: &net.TCPAddr{
				IP:   net.ParseIP(clientHost),
				Port: 0,
			},
			Control: func(network, address string, c syscall.RawConn) error {
				var opErr error
				err := c.Control(func(fd uintptr) {
					opErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
					opErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
				})
				if err != nil {
					return err
				}
				return opErr
			},
		}

		initTracerState(t, tr)
		require.NoError(t, tr.ebpfTracer.Resume())

		HTTPServer := NewTCPServerOnAddress(serverHost, func(c net.Conn) {
			r := bufio.NewReader(c)
			input, err := r.ReadBytes(byte('\n'))
			if err == nil {
				c.Write(input)
			}
			c.Close()
		})
		t.Cleanup(HTTPServer.Shutdown)
		require.NoError(t, HTTPServer.Run())
		_, port, err := net.SplitHostPort(HTTPServer.address)
		require.NoError(t, err)
		targetAddr := net.JoinHostPort(targetHost, port)

		// Running a HTTP client
		client := nethttp.Client{
			Transport: &nethttp.Transport{
				DialContext: dialer.DialContext,
			},
		}
		resp, err := client.Get("http://" + targetAddr + "/test")
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}

		client.CloseIdleConnections()
		waitForConnectionsWithProtocol(t, tr, targetAddr, HTTPServer.address, &protocols.Stack{Application: protocols.HTTP})
		HTTPServer.Shutdown()

		gRPCServer, err := grpc.NewServer(HTTPServer.address)
		require.NoError(t, err)
		gRPCServer.Run()

		grpcClient, err := grpc.NewClient(targetAddr, grpc.Options{
			CustomDialer: dialer,
		})
		require.NoError(t, err)
		defer grpcClient.Close()
		_ = grpcClient.HandleUnary(context.Background(), "test")
		gRPCServer.Stop()
		waitForConnectionsWithProtocol(t, tr, targetAddr, gRPCServer.Address, &protocols.Stack{Api: protocols.GRPC, Application: protocols.HTTP2})
	})
}

// Java Injection and TLS tests
func createJavaTempFile(t *testing.T, dir string) string {
	tempfile, err := os.CreateTemp(dir, "TestAgentLoaded.agentmain.*")
	require.NoError(t, err)
	tempfile.Close()
	os.Remove(tempfile.Name())
	t.Cleanup(func() { os.Remove(tempfile.Name()) })

	return tempfile.Name()
}

func (s *USMSuite) TestJavaInjection() {
	t := s.T()
	if !httpsSupported() {
		t.Skip("java TLS not supported on the current platform")
	}

	cfg := testConfig()
	cfg.EnableHTTPMonitoring = true
	cfg.EnableJavaTLSSupport = true
	defaultCfg := cfg

	dir, _ := testutil.CurDir()
	testdataDir := filepath.Join(dir, "../protocols/tls/java/testdata")
	legacyJavaDir := cfg.JavaDir
	// create a fake agent-usm.jar based on TestAgentLoaded.jar by forcing cfg.JavaDir
	fakeAgentDir, err := os.MkdirTemp("", "fake.agent-usm.jar.")
	require.NoError(t, err)
	defer os.RemoveAll(fakeAgentDir)
	_, err = nettestutil.RunCommand("install -m444 " + filepath.Join(testdataDir, "TestAgentLoaded.jar") + " " + filepath.Join(fakeAgentDir, "agent-usm.jar"))
	require.NoError(t, err)

	// testContext shares the context of a given test.
	// It contains common variable used by all tests, and allows extending the context dynamically by setting more
	// attributes to the `extras` map.
	type testContext struct {
		// A dynamic map that allows extending the context easily between phases of the test.
		extras map[string]interface{}
	}

	commonTearDown := func(t *testing.T, ctx testContext) {
		cfg.JavaAgentArgs = ctx.extras["JavaAgentArgs"].(string)

		testfile := ctx.extras["testfile"].(string)
		_, err := os.Stat(testfile)
		if err == nil {
			os.Remove(testfile)
		}
	}

	commonValidation := func(t *testing.T, ctx testContext, tr *Tracer) {
		testfile := ctx.extras["testfile"].(string)
		_, err := os.Stat(testfile)
		require.NoError(t, err)
	}

	tests := []struct {
		name            string
		context         testContext
		preTracerSetup  func(t *testing.T, ctx testContext)
		postTracerSetup func(t *testing.T, ctx testContext)
		validation      func(t *testing.T, ctx testContext, tr *Tracer)
		teardown        func(t *testing.T, ctx testContext)
	}{
		{
			// Test the java hotspot injection is working
			name: "java_hotspot_injection_8u151",
			context: testContext{
				extras: make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				cfg.JavaDir = fakeAgentDir
				ctx.extras["JavaAgentArgs"] = cfg.JavaAgentArgs

				ctx.extras["testfile"] = createJavaTempFile(t, testdataDir)
				cfg.JavaAgentArgs += ",testfile=/v/" + filepath.Base(ctx.extras["testfile"].(string))
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				// if RunJavaVersion failing to start it's probably because the java process has not been injected
				require.NoError(t, javatestutil.RunJavaVersion(t, "openjdk:8u151-jre", "JustWait"), "Failed running Java version")
			},
			validation: commonValidation,
			teardown:   commonTearDown,
		},
		{
			// Test the java hotspot injection is working
			name: "java_hotspot_injection_21_allow_only",
			context: testContext{
				extras: make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				cfg.JavaDir = fakeAgentDir
				ctx.extras["JavaAgentArgs"] = cfg.JavaAgentArgs

				ctx.extras["testfile"] = createJavaTempFile(t, testdataDir)
				cfg.JavaAgentArgs += ",testfile=/v/" + filepath.Base(ctx.extras["testfile"].(string))

				// testing allow/block list, as Allow list have higher priority
				// this test will pass normally
				cfg.JavaAgentAllowRegex = ".*JustWait.*"
				cfg.JavaAgentBlockRegex = ""
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				// if RunJavaVersion failing to start it's probably because the java process has not been injected
				require.NoError(t, javatestutil.RunJavaVersion(t, "openjdk:21-oraclelinux8", "JustWait"), "Failed running Java version")
				require.Error(t, javatestutil.RunJavaVersion(t, "openjdk:21-oraclelinux8", "AnotherWait"), "AnotherWait should not be attached")
			},
			validation: commonValidation,
			teardown:   commonTearDown,
		},
		{
			// Test the java hotspot injection is working
			name: "java_hotspot_injection_21_block_only",
			context: testContext{
				extras: make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				ctx.extras["JavaAgentArgs"] = cfg.JavaAgentArgs

				ctx.extras["testfile"] = createJavaTempFile(t, testdataDir)
				cfg.JavaAgentArgs += ",testfile=/v/" + filepath.Base(ctx.extras["testfile"].(string))

				// block the agent attachment
				cfg.JavaAgentAllowRegex = ""
				cfg.JavaAgentBlockRegex = ".*JustWait.*"
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				// if RunJavaVersion failing to start it's probably because the java process has not been injected
				require.NoError(t, javatestutil.RunJavaVersion(t, "openjdk:21-oraclelinux8", "AnotherWait"), "Failed running Java version")
				require.Error(t, javatestutil.RunJavaVersion(t, "openjdk:21-oraclelinux8", "JustWait"), "JustWait should not be attached")
			},
			validation: commonValidation,
			teardown:   commonTearDown,
		},
		{
			name: "java_hotspot_injection_21_allowblock",
			context: testContext{
				extras: make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				ctx.extras["JavaAgentArgs"] = cfg.JavaAgentArgs

				ctx.extras["testfile"] = createJavaTempFile(t, testdataDir)
				cfg.JavaAgentArgs += ",testfile=/v/" + filepath.Base(ctx.extras["testfile"].(string))

				// block the agent attachment
				cfg.JavaAgentAllowRegex = ".*JustWait.*"
				cfg.JavaAgentBlockRegex = ".*AnotherWait.*"
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				require.NoError(t, javatestutil.RunJavaVersion(t, "openjdk:21-oraclelinux8", "JustWait"), "Failed running Java version")
				require.Error(t, javatestutil.RunJavaVersion(t, "openjdk:21-oraclelinux8", "AnotherWait"), "AnotherWait should not be attached")
			},
			validation: commonValidation,
			teardown:   commonTearDown,
		},
		{
			name: "java_hotspot_injection_21_allow_higher_priority",
			context: testContext{
				extras: make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				ctx.extras["JavaAgentArgs"] = cfg.JavaAgentArgs

				ctx.extras["testfile"] = createJavaTempFile(t, testdataDir)
				cfg.JavaAgentArgs += ",testfile=/v/" + filepath.Base(ctx.extras["testfile"].(string))

				// allow has a higher priority
				cfg.JavaAgentAllowRegex = ".*JustWait.*"
				cfg.JavaAgentBlockRegex = ".*JustWait.*"
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				require.NoError(t, javatestutil.RunJavaVersion(t, "openjdk:21-oraclelinux8", "JustWait"), "Failed running Java version")
			},
			validation: commonValidation,
			teardown:   commonTearDown,
		},
		{
			// Test the java jdk client https request is working
			name: "java_jdk_client_httpbin_docker_withTLSClassification_java15",
			preTracerSetup: func(t *testing.T, ctx testContext) {
				cfg.JavaDir = legacyJavaDir
				cfg.ProtocolClassificationEnabled = true
				cfg.CollectTCPv4Conns = true
				cfg.CollectTCPv6Conns = true

				serverDoneFn := testutil.HTTPServer(t, "0.0.0.0:5443", testutil.Options{
					EnableTLS: true,
				})
				t.Cleanup(serverDoneFn)
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				require.NoError(t, javatestutil.RunJavaVersion(t, "openjdk:15-oraclelinux8", "Wget https://host.docker.internal:5443/200/anything/java-tls-request", regexp.MustCompile("Response code = .*")), "Failed running Java version")
			},
			validation: func(t *testing.T, ctx testContext, tr *Tracer) {
				// Iterate through active connections until we find connection created above
				require.Eventuallyf(t, func() bool {
					payload := getConnections(t, tr)
					for key, stats := range payload.HTTP {
						if key.Path.Content == "/200/anything/java-tls-request" {
							t.Log("path content found")
							// socket filter is not supported on fentry tracer
							if tr.ebpfTracer.Type() == connection.TracerTypeFentry {
								// so we return early if the test was successful until now
								return true
							}

							req, exists := stats.Data[200]
							if !exists {
								t.Logf("wrong response, not 200 : %#+v", key)
								continue
							}

							if req.StaticTags != network.ConnTagJava {
								t.Logf("tag not java : %#+v", key)
								continue
							}
							return true
							// Commented out, as it makes the test flaky
							//for _, c := range payload.Conns {
							//	if c.SPort == key.SrcPort && c.DPort == key.DstPort && c.ProtocolStack.Contains(protocols.TLS) {
							//		return true
							//	}
							//}
							//t.Logf("TLS connection tag not found : %#+v", key)
						}
					}

					return false
				}, 4*time.Second, time.Second, "couldn't find http connection matching: %s", "https://host.docker.internal:5443/200/anything/java-tls-request")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.teardown != nil {
				t.Cleanup(func() {
					tt.teardown(t, tt.context)
				})
			}
			cfg = defaultCfg
			if tt.preTracerSetup != nil {
				tt.preTracerSetup(t, tt.context)
			}
			tr := setupTracer(t, cfg)
			tt.postTracerSetup(t, tt.context)
			tt.validation(t, tt.context, tr)
		})
	}
}

func skipFedora(t *testing.T) bool {
	info, err := host.Info()
	require.NoError(t, err)

	return info.Platform == "fedora" && (info.PlatformVersion == "35" || info.PlatformVersion == "36" || info.PlatformVersion == "37" || info.PlatformVersion == "38")
}

func TestHTTPGoTLSAttachProbes(t *testing.T) {
	modes := []ebpftest.BuildMode{ebpftest.RuntimeCompiled, ebpftest.CORE}
	ebpftest.TestBuildModes(t, modes, "", func(t *testing.T) {
		if !goTLSSupported() {
			t.Skip("GoTLS not supported for this setup")
		}

		// TODO fix TestHTTPGoTLSAttachProbes on these Fedora versions
		if skipFedora(t) {
			// TestHTTPGoTLSAttachProbes fails consistently in CI on Fedora 36,37
			t.Skip("TestHTTPGoTLSAttachProbes fails on this OS consistently")
		}

		t.Run("new process", func(t *testing.T) {
			testHTTPGoTLSCaptureNewProcess(t, config.New())
		})
		t.Run("already running process", func(t *testing.T) {
			testHTTPGoTLSCaptureAlreadyRunning(t, config.New())
		})
	})
}

func TestHTTPSGoTLSAttachProbesOnContainer(t *testing.T) {
	t.Skip("Skipping a flaky test")
	modes := []ebpftest.BuildMode{ebpftest.RuntimeCompiled, ebpftest.CORE}
	ebpftest.TestBuildModes(t, modes, "", func(t *testing.T) {
		if !goTLSSupported() {
			t.Skip("GoTLS not supported for this setup")
		}

		t.Run("new process", func(t *testing.T) {
			testHTTPsGoTLSCaptureNewProcessContainer(t, config.New())
		})
		t.Run("already running process", func(t *testing.T) {
			testHTTPsGoTLSCaptureAlreadyRunningContainer(t, config.New())
		})
	})
}

// Test that we can capture HTTPS traffic from Go processes started after the
// tracer.
func testHTTPGoTLSCaptureNewProcess(t *testing.T, cfg *config.Config) {
	const (
		serverAddr          = "localhost:8081"
		expectedOccurrences = 10
	)

	// Setup
	closeServer := testutil.HTTPServer(t, serverAddr, testutil.Options{
		EnableTLS: true,
	})
	t.Cleanup(closeServer)

	cfg.EnableGoTLSSupport = true
	cfg.EnableHTTPMonitoring = true

	tr := setupTracer(t, cfg)

	// This maps will keep track of whether the tracer saw this request already or not
	reqs := make(requestsMap)
	for i := 0; i < expectedOccurrences; i++ {
		req, err := nethttp.NewRequest(nethttp.MethodGet, fmt.Sprintf("https://%s/%d/request-%d", serverAddr, nethttp.StatusOK, i), nil)
		require.NoError(t, err)
		reqs[req] = false
	}

	// spin-up goTLS client and issue requests after initialization
	gotlstestutil.NewGoTLSClient(t, serverAddr, expectedOccurrences)()
	checkRequests(t, tr, expectedOccurrences, reqs)
}

func testHTTPGoTLSCaptureAlreadyRunning(t *testing.T, cfg *config.Config) {
	const (
		serverAddr          = "localhost:8081"
		expectedOccurrences = 10
	)

	// Setup
	closeServer := testutil.HTTPServer(t, serverAddr, testutil.Options{
		EnableTLS: true,
	})
	t.Cleanup(closeServer)

	// spin-up goTLS client but don't issue requests yet
	issueRequestsFn := gotlstestutil.NewGoTLSClient(t, serverAddr, expectedOccurrences)

	cfg.EnableGoTLSSupport = true
	cfg.EnableHTTPMonitoring = true

	tr := setupTracer(t, cfg)

	// This maps will keep track of whether the tracer saw this request already or not
	reqs := make(requestsMap)
	for i := 0; i < expectedOccurrences; i++ {
		req, err := nethttp.NewRequest(nethttp.MethodGet, fmt.Sprintf("https://%s/%d/request-%d", serverAddr, nethttp.StatusOK, i), nil)
		require.NoError(t, err)
		reqs[req] = false
	}

	issueRequestsFn()
	checkRequests(t, tr, expectedOccurrences, reqs)
}

// Test that we can capture HTTPS traffic from Go processes started after the
// tracer.
func testHTTPsGoTLSCaptureNewProcessContainer(t *testing.T, cfg *config.Config) {
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
	cfg.EnableHTTPStatsByStatusCode = true

	tr := setupTracer(t, cfg)

	require.NoError(t, gotlstestutil.RunServer(t, serverPort))
	reqs := make(requestsMap)
	for i := 0; i < expectedOccurrences; i++ {
		resp, err := client.Get(fmt.Sprintf("https://localhost:%s/status/%d", serverPort, 200+i))
		require.NoError(t, err)
		resp.Body.Close()
		reqs[resp.Request] = false
	}

	client.CloseIdleConnections()
	checkRequests(t, tr, expectedOccurrences, reqs)
}

func testHTTPsGoTLSCaptureAlreadyRunningContainer(t *testing.T, cfg *config.Config) {
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
	cfg.EnableHTTPStatsByStatusCode = true

	tr := setupTracer(t, cfg)

	reqs := make(requestsMap)
	for i := 0; i < expectedOccurrences; i++ {
		resp, err := client.Get(fmt.Sprintf("https://localhost:%s/status/%d", serverPort, 200+i))
		require.NoError(t, err)
		resp.Body.Close()
		reqs[resp.Request] = false
	}

	client.CloseIdleConnections()
	checkRequests(t, tr, expectedOccurrences, reqs)
}

type tlsTestCommand struct {
	version        string
	openSSLCommand string
}

// TLS classification tests
func (s *USMSuite) TestTLSClassification() {
	t := s.T()
	cfg := testConfig()
	cfg.ProtocolClassificationEnabled = true
	cfg.CollectTCPv4Conns = true
	cfg.CollectTCPv6Conns = true

	if !classificationSupported(cfg) {
		t.Skip("TLS classification platform not supported")
	}

	tr := setupTracer(t, cfg)

	type tlsTest struct {
		name            string
		postTracerSetup func(t *testing.T)
		validation      func(t *testing.T, tr *Tracer)
	}
	scenarios := []tlsTestCommand{
		{
			version:        "1.0",
			openSSLCommand: "-tls1",
		},
		{
			version:        "1.1",
			openSSLCommand: "-tls1_1",
		},
		{
			version:        "1.2",
			openSSLCommand: "-tls1_2",
		},
		{
			version:        "1.3",
			openSSLCommand: "-tls1_3",
		},
	}
	tests := make([]tlsTest, 0, len(scenarios))
	for _, scenario := range scenarios {
		tests = append(tests, tlsTest{
			name: "TLS-" + scenario.version + "_docker",
			postTracerSetup: func(t *testing.T) {
				clientSuccess := false
				var wg sync.WaitGroup
				wg.Add(1)
				go func() {
					defer wg.Done()
					time.Sleep(5 * time.Second)
					clientSuccess = prototls.RunClientOpenssl(t, "localhost", "44330", scenario.openSSLCommand)
				}()
				require.NoError(t, prototls.RunServerOpenssl(t, "44330", "-www"))
				wg.Wait()
				if !clientSuccess {
					t.Fatalf("openssl client failed")
				}
			},
			validation: func(t *testing.T, tr *Tracer) {
				// Iterate through active connections until we find connection created above
				require.Eventuallyf(t, func() bool {
					payload := getConnections(t, tr)
					for _, c := range payload.Conns {
						if c.DPort == 44330 && c.ProtocolStack.Contains(protocols.TLS) {
							return true
						}
					}
					return false
				}, 4*time.Second, time.Second, "couldn't find TLS connection matching: dst port 44330")
			},
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tr.ebpfTracer.Type() == connection.TracerTypeFentry {
				t.Skip("protocol classification not supported for fentry tracer")
			}
			t.Cleanup(func() { tr.removeClient(clientID) })
			t.Cleanup(func() { _ = tr.ebpfTracer.Pause() })

			tr.removeClient(clientID)
			initTracerState(t, tr)
			require.NoError(t, tr.ebpfTracer.Resume(), "enable probes - before post tracer")
			tt.postTracerSetup(t)
			require.NoError(t, tr.ebpfTracer.Pause(), "disable probes - after post tracer")
			tt.validation(t, tr)
		})
	}
}

func checkRequests(t *testing.T, tr *Tracer, expectedOccurrences int, reqs requestsMap) {
	t.Helper()

	occurrences := PrintableInt(0)
	require.Eventually(t, func() bool {
		stats := getConnections(t, tr)
		occurrences += PrintableInt(countRequestsOccurrences(t, stats, reqs))
		return int(occurrences) == expectedOccurrences
	}, 3*time.Second, 100*time.Millisecond, "Expected to find the request %v times, got %v captured. Requests not found:\n%v", expectedOccurrences, &occurrences, reqs)
}

func countRequestsOccurrences(t *testing.T, conns *network.Connections, reqs map[*nethttp.Request]bool) (occurrences int) {
	t.Helper()

	for key, stats := range conns.HTTP {
		for req, found := range reqs {
			if found {
				continue
			}

			expectedStatus := testutil.StatusFromPath(req.URL.Path)
			if key.Path.Content != req.URL.Path {
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

type PrintableInt int

func (i *PrintableInt) String() string {
	if i == nil {
		return "nil"
	}

	return fmt.Sprintf("%d", *i)
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

func skipIfHTTPSNotSupported(t *testing.T, _ testContext) {
	if !httpsSupported() {
		t.Skip("https is not supported")
	}
}

func testHTTPSClassification(t *testing.T, tr *Tracer, clientHost, targetHost, serverHost string) {
	skipFunc := composeSkips(skipIfHTTPSNotSupported)
	skipFunc(t, testContext{
		serverAddress: serverHost,
		serverPort:    httpsPort,
		targetAddress: targetHost,
	})

	defaultDialer := &net.Dialer{
		LocalAddr: &net.TCPAddr{
			IP: net.ParseIP(clientHost),
		},
	}

	serverAddress := net.JoinHostPort(serverHost, httpsPort)
	targetAddress := net.JoinHostPort(targetHost, httpsPort)
	tests := []protocolClassificationAttributes{
		{
			name: "HTTPs request",
			context: testContext{
				serverPort:    httpsPort,
				serverAddress: serverAddress,
				targetAddress: targetAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				closer, err := testutil.HTTPPythonServer(t, ctx.serverAddress, testutil.Options{
					EnableKeepAlive: false,
					EnableTLS:       true,
				})
				require.NoError(t, err)
				t.Cleanup(closer)
			},
			validation: func(t *testing.T, ctx testContext, tr *Tracer) {
				client := nethttp.Client{
					Transport: &nethttp.Transport{
						TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
						DialContext:     defaultDialer.DialContext,
					},
				}

				// Ensure that we see HTTPS requests being traced *before* the actual test assertions
				// This is done to reduce test test flakiness due to uprobe attachment delays
				require.Eventually(t, func() bool {
					resp, err := client.Get(fmt.Sprintf("https://%s/200/warm-up", ctx.targetAddress))
					if err != nil {
						return false
					}
					_, _ = io.Copy(io.Discard, resp.Body)
					_ = resp.Body.Close()

					httpData := getConnections(t, tr).HTTP
					for httpKey := range httpData {
						if httpKey.Path.Content == resp.Request.URL.Path {
							return true
						}
					}

					return false
				}, 5*time.Second, 100*time.Millisecond, "couldn't detect HTTPS traffic being traced (test setup validation)")

				t.Log("run 3 clients request as we can have a race between the closing tcp socket and the http response")
				for i := 0; i < 3; i++ {
					resp, err := client.Get(fmt.Sprintf("https://%s/200/request-1", ctx.targetAddress))
					require.NoError(t, err)
					_, _ = io.Copy(io.Discard, resp.Body)
					_ = resp.Body.Close()
					client.CloseIdleConnections()
				}

				waitForConnectionsWithProtocol(t, tr, ctx.targetAddress, ctx.serverAddress, &protocols.Stack{Encryption: protocols.TLS, Application: protocols.HTTP})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testProtocolClassificationInner(t, tt, tr)
		})
	}
}
