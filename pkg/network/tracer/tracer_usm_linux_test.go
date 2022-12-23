// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

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
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netlink "github.com/DataDog/datadog-agent/pkg/network/netlink/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	nettestutil "github.com/DataDog/datadog-agent/pkg/network/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/kprobe"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/testutil/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type connTag = uint64

const (
	tagGnuTLS  connTag = 1 // netebpf.GnuTLS
	tagOpenSSL connTag = 2 // netebpf.OpenSSL
)

var (
	staticTags = map[connTag]string{
		tagGnuTLS:  "tls.library:gnutls",
		tagOpenSSL: "tls.library:openssl",
	}
)

func httpSupported(t *testing.T) bool {
	currKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	return currKernelVersion >= http.MinimumKernelVersion
}

func httpsSupported(t *testing.T) bool {
	return http.HTTPSSupported(testConfig())
}

func goTLSSupported() bool {
	return runtime.GOARCH == "amd64"
}

func classificationSupported(config *config.Config) bool {
	return kprobe.ClassificationSupported(config)
}

func TestEnableHTTPMonitoring(t *testing.T) {
	if !httpSupported(t) {
		t.Skip("HTTP monitoring not supported")
	}

	cfg := testConfig()
	cfg.EnableHTTPMonitoring = true
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	defer tr.Stop()
}

func TestHTTPStats(t *testing.T) {
	if !httpSupported(t) {
		t.Skip("HTTP monitoring feature not available")
		return
	}

	cfg := testConfig()
	cfg.EnableHTTPMonitoring = true
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	defer tr.Stop()

	initTracerState(t, tr)

	// Start an HTTP server on localhost:8080
	serverAddr := "127.0.0.1:8080"
	srv := &nethttp.Server{
		Addr: serverAddr,
		Handler: nethttp.HandlerFunc(func(w nethttp.ResponseWriter, req *nethttp.Request) {
			io.Copy(io.Discard, req.Body)
			w.WriteHeader(200)
		}),
		ReadTimeout:  time.Second,
		WriteTimeout: time.Second,
	}
	srv.SetKeepAlivesEnabled(false)
	go func() {
		_ = srv.ListenAndServe()
	}()
	defer srv.Shutdown(context.Background())

	// Allow the HTTP server time to get set up
	time.Sleep(time.Millisecond * 500)

	// Send a series of HTTP requests to the test server
	client := new(nethttp.Client)
	resp, err := client.Get("http://" + serverAddr + "/test")
	require.NoError(t, err)
	resp.Body.Close()

	// Iterate through active connections until we find connection created above
	var httpReqStats *http.RequestStats
	require.Eventuallyf(t, func() bool {
		payload, err := tr.GetActiveConnections("1")
		if err != nil {
			t.Fatal(err)
		}

		for key, stats := range payload.HTTP {
			if key.Path.Content == "/test" {
				httpReqStats = stats
				return true
			}
		}

		return false
	}, 3*time.Second, 10*time.Millisecond, "couldn't find http connection matching: %s", serverAddr)

	// Verify HTTP stats
	require.NotNil(t, httpReqStats)
	assert.Nil(t, httpReqStats.Stats(100), "100s")            // number of requests with response status 100
	assert.Equal(t, 1, httpReqStats.Stats(200).Count, "200s") // 200
	assert.Nil(t, httpReqStats.Stats(300), "300s")            // 300
	assert.Nil(t, httpReqStats.Stats(400), "400s")            // 400
	assert.Nil(t, httpReqStats.Stats(500), "500s")            // 500
}

func TestHTTPSViaLibraryIntegration(t *testing.T) {
	if !httpSupported(t) {
		t.Skip("HTTPS feature not available on pre 4.14.0 kernels")
	}
	if !httpsSupported(t) {
		t.Skip("HTTPS feature not available/supported for this setup")
	}
	t.Skip("we don't support fast process start/stop yet as /proc/pid/root pass likely doesn't exist when we try to add hook")

	tlsLibs := []*regexp.Regexp{
		regexp.MustCompile(`/[^\ ]+libssl.so[^\ ]*`),
		regexp.MustCompile(`/[^\ ]+libgnutls.so[^\ ]*`),
	}
	tests := []struct {
		name     string
		fetchCmd []string
	}{
		{name: "wget", fetchCmd: []string{"wget", "--no-check-certificate", "-O/dev/null"}},
		{name: "curl", fetchCmd: []string{"curl", "--http1.1", "-k", "-o/dev/null"}},
	}

	for _, keepAlives := range []struct {
		name  string
		value bool
	}{
		{name: "without keep-alives", value: false},
		{name: "with keep-alives", value: true},
	} {
		t.Run(keepAlives.name, func(t *testing.T) {
			// Spin-up HTTPS server
			serverDoneFn := testutil.HTTPServer(t, "127.0.0.1:443", testutil.Options{
				EnableTLS:        true,
				EnableKeepAlives: keepAlives.value,
			})
			t.Cleanup(serverDoneFn)

			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					fetch, err := exec.LookPath(test.fetchCmd[0])
					if err != nil {
						t.Skipf("%s not found; skipping test.", test.fetchCmd)
					}
					ldd, err := exec.LookPath("ldd")
					if err != nil {
						t.Skip("ldd not found; skipping test.")
					}
					linked, _ := exec.Command(ldd, fetch).Output()

					foundSSLLib := false
					for _, lib := range tlsLibs {
						libSSLPath := lib.FindString(string(linked))
						if _, err := os.Stat(libSSLPath); err == nil {
							foundSSLLib = true
							break
						}
					}
					if !foundSSLLib {
						t.Fatalf("%s not linked with any of these libs %v", test.name, tlsLibs)
					}

					testHTTPSLibrary(t, test.fetchCmd)

				})
			}
		})
	}
}

func testHTTPSLibrary(t *testing.T, fetchCmd []string) {
	// Start tracer with HTTPS support
	cfg := testConfig()
	cfg.EnableHTTPMonitoring = true
	cfg.EnableHTTPSMonitoring = true
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	defer tr.Stop()
	err = tr.RegisterClient("1")
	require.NoError(t, err)

	// Run fetchCmd once to make sure the OpenSSL is detected and uprobes are attached
	exec.Command(fetchCmd[0]).Run()
	time.Sleep(2 * time.Second)

	// Issue request using fetchCmd (wget, curl, ...)
	// This is necessary (as opposed to using net/http) because we want to
	// test a HTTP client linked to OpenSSL or GnuTLS
	const targetURL = "https://127.0.0.1:443/200/foobar"
	cmd := append(fetchCmd, targetURL)
	requestCmd := exec.Command(cmd[0], cmd[1:]...)
	var out []byte
	out, err = requestCmd.CombinedOutput()
	require.NoErrorf(t, err, "failed to issue request via %s: %s\n%s", fetchCmd, err, string(out))

	require.Eventuallyf(t, func() bool {
		payload, err := tr.GetActiveConnections("1")
		if err != nil {
			t.Fatal(err)
		}

		for key, stats := range payload.HTTP {
			if !stats.HasStats(200) {
				continue
			}

			statsTags := stats.Stats(200).StaticTags
			// debian 10 have curl binary linked with openssl and gnutls but use only openssl during tls query (there no runtime flag available)
			// this make harder to map lib and tags, one set of tag should match but not both
			foundPathAndHTTPTag := false
			if key.Path.Content == "/200/foobar" && (statsTags == tagGnuTLS || statsTags == tagOpenSSL) {
				foundPathAndHTTPTag = true
				t.Logf("found tag 0x%x %s", statsTags, staticTags[statsTags])
			}
			if foundPathAndHTTPTag {
				return true
			}
			t.Logf("HTTP stat didn't match criteria %v tags 0x%x\n", key, statsTags)
			for _, c := range payload.Conns {
				possibleKeyTuples := network.HTTPKeyTuplesFromConn(c)
				t.Logf("conn sport %d dport %d tags %x connKey [%v] or [%v]\n", c.SPort, c.DPort, c.Tags, possibleKeyTuples[0], possibleKeyTuples[1])
			}
		}

		return false
	}, 10*time.Second, 1*time.Second, "couldn't find HTTPS stats")
}

const (
	numberOfRequests = 100
)

// TestOpenSSLVersions setups a HTTPs python server, and makes sure we are able to capture all traffic.
func TestOpenSSLVersions(t *testing.T) {
	if !httpSupported(t) {
		t.Skip("HTTPS feature not available on pre 4.14.0 kernels")
	}

	if !httpsSupported(t) {
		t.Skip("HTTPS feature not available/supported for this setup")
	}

	cfg := testConfig()
	cfg.EnableHTTPSMonitoring = true
	cfg.EnableHTTPMonitoring = true
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	initTracerState(t, tr)
	defer tr.Stop()

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
	// At the moment, there is a bug in the SSL hooks which cause us to miss (statistically) the last request.
	// So I'm sending another request and not expecting to capture it.
	requestFn()

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
func TestOpenSSLVersionsSlowStart(t *testing.T) {
	if !httpsSupported(t) {
		t.Skip("HTTPS feature not available/supported for this setup")
	}

	cfg := testConfig()
	cfg.EnableHTTPSMonitoring = true
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

	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	defer tr.Stop()

	initTracerState(t, tr)

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

	// At the moment, there is a bug in the SSL hooks which cause us to miss (statistically) the last request.
	// So I'm sending another request and not expecting to capture it.
	requestFn()

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
		if key.Path.Content == req.URL.Path && stats.HasStats(expectedStatus) {
			return true
		}
	}

	return false
}

func TestProtocolClassification(t *testing.T) {
	cfg := testConfig()
	if !classificationSupported(cfg) {
		t.Skip("Classification is not supported")
	}

	t.Run("with dnat", func(t *testing.T) {
		// SetupDNAT sets up a NAT translation from 2.2.2.2 to 1.1.1.1
		netlink.SetupDNAT(t)
		testProtocolClassification(t, cfg, "localhost", "2.2.2.2", "1.1.1.1")
		testProtocolClassificationMapCleanup(t, cfg, "localhost", "2.2.2.2", "1.1.1.1:0")
	})

	t.Run("with snat", func(t *testing.T) {
		// SetupDNAT sets up a NAT translation from 6.6.6.6 to 7.7.7.7
		netlink.SetupSNAT(t)
		testProtocolClassification(t, cfg, "6.6.6.6", "127.0.0.1", "127.0.0.1")
		testProtocolClassificationMapCleanup(t, cfg, "6.6.6.6", "127.0.0.1", "127.0.0.1:0")
	})

	t.Run("without nat", func(t *testing.T) {
		testProtocolClassification(t, cfg, "localhost", "127.0.0.1", "127.0.0.1")
		testProtocolClassificationMapCleanup(t, cfg, "localhost", "127.0.0.1", "127.0.0.1:0")
	})
}

func testProtocolClassificationMapCleanup(t *testing.T, cfg *config.Config, clientHost, targetHost, serverHost string) {
	t.Run("protocol cleanup", func(t *testing.T) {
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

		tr, err := NewTracer(cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer tr.Stop()

		initTracerState(t, tr)
		require.NoError(t, err)
		done := make(chan struct{})
		HTTPServer := NewTCPServerOnAddress(serverHost, func(c net.Conn) {
			r := bufio.NewReader(c)
			input, err := r.ReadBytes(byte('\n'))
			if err == nil {
				c.Write(input)
			}
			c.Close()
		})
		require.NoError(t, HTTPServer.Run(done))
		_, port, err := net.SplitHostPort(HTTPServer.address)
		require.NoError(t, err)
		targetAddr := net.JoinHostPort(targetHost, port)

		// Letting the server time to start
		time.Sleep(500 * time.Millisecond)

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
		waitForConnectionsWithProtocol(t, tr, targetAddr, HTTPServer.address, network.ProtocolHTTP)
		close(done)

		time.Sleep(2 * time.Second)

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
		waitForConnectionsWithProtocol(t, tr, targetAddr, gRPCServer.Address, network.ProtocolHTTP2)
	})
}

// GoTLS test

func TestHTTPGoTLSAttachProbes(t *testing.T) {
	clientBin := buildGoTLSClientBin(t)

	if !goTLSSupported() {
		t.Skip("GoTLS support not available on non amd64 architectures")
	}

	if !httpSupported(t) {
		t.Skip("HTTPS feature not available on pre 4.14.0 kernels")
	}

	if !httpsSupported(t) {
		t.Skip("HTTPS feature not available supported for this setup")
	}

	t.Run("New Process", testHTTPGoTLSCaptureNewProcess(clientBin))
	t.Run("Already running process", testHTTPGoTLSCaptureAlreadyRunning(clientBin))
}

// Test that we can capture HTTPS traffic from Go processes started after the
// tracer.
func testHTTPGoTLSCaptureNewProcess(clientBin string) func(t *testing.T) {
	return func(t *testing.T) {
		const (
			serverAddr          = "localhost:8081"
			expectedOccurrences = 10
		)

		// Setup
		closeServer := testutil.HTTPServer(t, serverAddr, testutil.Options{
			EnableTLS: true,
		})
		defer closeServer()

		cfg := testConfig()
		cfg.EnableGoTLSSupport = true
		cfg.EnableHTTPMonitoring = true
		cfg.EnableHTTPSMonitoring = true
		cfg.EnableRuntimeCompiler = true

		tr, err := NewTracer(cfg)
		require.NoError(t, err)
		defer tr.Stop()
		require.NoError(t, tr.RegisterClient("1"))

		// This maps will keep track of whether or not the tracer saw this request already or not
		reqs := make(requestsMap)
		for i := 0; i < expectedOccurrences; i++ {
			req, err := nethttp.NewRequest(nethttp.MethodGet, fmt.Sprintf("https://%s/%d/request-%d", serverAddr, nethttp.StatusOK, i), nil)
			require.NoError(t, err)
			reqs[req] = false
		}

		// Test
		clientCmd := fmt.Sprintf("%s %s %d", clientBin, serverAddr, expectedOccurrences)
		c, clientInput, err := nettestutil.StartCommand(clientCmd)
		require.NoError(t, err)
		_, err = clientInput.Write([]byte{1})
		require.NoError(t, err)
		err = c.Wait()
		require.NoError(t, err)

		checkRequests(t, tr, expectedOccurrences, reqs)
	}
}

func testHTTPGoTLSCaptureAlreadyRunning(clientBin string) func(t *testing.T) {
	return func(t *testing.T) {
		const (
			serverAddr          = "localhost:8081"
			expectedOccurrences = 10
		)

		// Setup
		var closeServer func() = testutil.HTTPServer(t, serverAddr, testutil.Options{
			EnableTLS: true,
		})

		done := make(chan struct{})

		cfg := config.New()
		cfg.EnableGoTLSSupport = true
		cfg.EnableHTTPMonitoring = true
		cfg.EnableHTTPSMonitoring = true
		cfg.EnableRuntimeCompiler = true

		// Test

		// This maps will keep track of whether or not the tracer saw this request already or not
		reqs := make(requestsMap)
		for i := 0; i < expectedOccurrences; i++ {
			req, err := nethttp.NewRequest(nethttp.MethodGet, fmt.Sprintf("https://%s/%d/request-%d", serverAddr, nethttp.StatusOK, i), nil)
			require.NoError(t, err)
			reqs[req] = false
		}

		clientCmd := fmt.Sprintf("%s %s %d", clientBin, serverAddr, expectedOccurrences)
		c, clientInput, err := nettestutil.StartCommand(clientCmd)
		require.NoError(t, err)

		go func() {
			defer close(done)
			err = c.Wait()
			require.NoError(t, err)

			closeServer()
		}()

		tr, err := NewTracer(cfg)
		require.NoError(t, err)
		defer tr.Stop()
		require.NoError(t, tr.RegisterClient("1"))

		_, err = clientInput.Write([]byte{1})
		require.NoError(t, err)

		<-done

		checkRequests(t, tr, expectedOccurrences, reqs)
	}
}

func buildGoTLSClientBin(t *testing.T) string {
	const ClientSrcPath = "testutil/gotls_client"
	const ClientBinaryPath = "testutil/gotls_client/gotls_client"

	t.Helper()

	cur, err := testutil.CurDir()
	require.NoError(t, err)

	clientBinary := fmt.Sprintf("%s/%s", cur, ClientBinaryPath)

	// If there is a compiled binary already, skip the compilation.
	// Meant for the CI.
	if _, err = os.Stat(clientBinary); err == nil {
		return clientBinary
	}

	clientSrcDir := fmt.Sprintf("%s/%s", cur, ClientSrcPath)
	clientBuildDir, err := os.MkdirTemp("/tmp", "gotls_client_build-")
	require.NoError(t, err)

	t.Cleanup(func() {
		os.RemoveAll(clientBuildDir)
	})

	clientBinPath := fmt.Sprintf("%s/gotls_client", clientBuildDir)

	c := exec.Command("go", "build", "-buildvcs=false", "-a", "-ldflags=-extldflags '-static'", "-o", clientBinPath, clientSrcDir)
	out, err := c.CombinedOutput()
	require.NoError(t, err, "could not build client test binary: %s\noutput: %s", err, string(out))

	return clientBinPath
}

func checkRequests(t *testing.T, tr *Tracer, expectedOccurrences int, reqs requestsMap) {
	t.Helper()

	occurrences := PrintableInt(0)
	require.Eventually(t, func() bool {
		stats, err := tr.GetActiveConnections("1")
		require.NoError(t, err)
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
			if key.Path.Content == req.URL.Path && stats.HasStats(expectedStatus) {
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
