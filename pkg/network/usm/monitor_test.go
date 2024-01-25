// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	nethttp "net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netlink "github.com/DataDog/datadog-agent/pkg/network/netlink/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	libtelemetry "github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	gotlsutils "github.com/DataDog/datadog-agent/pkg/network/protocols/tls/gotls/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/testutil/proxy"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func TestMain(m *testing.M) {
	logLevel := os.Getenv("DD_LOG_LEVEL")
	if logLevel == "" {
		logLevel = "warn"
	}
	log.SetupLogger(seelog.Default, logLevel)
	os.Exit(m.Run())
}

const (
	kb = 1024
	mb = 1024 * kb
)

var (
	emptyBody = []byte(nil)
	kv        = kernel.MustHostVersion()
)

func TestMonitorProtocolFail(t *testing.T) {
	failingStartupMock := func(_ *manager.Manager) error {
		return fmt.Errorf("mock error")
	}

	testCases := []struct {
		name string
		spec protocolMockSpec
	}{
		{name: "PreStart fails", spec: protocolMockSpec{preStartFn: failingStartupMock}},
		{name: "PostStart fails", spec: protocolMockSpec{postStartFn: failingStartupMock}},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			// Replace the HTTP protocol with a Mock
			patchProtocolMock(t, tt.spec)

			cfg := config.New()
			cfg.EnableHTTPMonitoring = true
			monitor, err := NewMonitor(cfg, nil, nil, nil)
			skipIfNotSupported(t, err)
			require.NoError(t, err)
			t.Cleanup(monitor.Stop)

			err = monitor.Start()
			require.ErrorIs(t, err, errNoProtocols)
		})
	}
}

type httpTestSuite struct {
	suite.Suite
	isTLS bool
}

func (s *httpTestSuite) getCfg() *config.Config {
	cfg := config.New()
	cfg.EnableHTTPMonitoring = true
	cfg.EnableGoTLSSupport = s.isTLS
	cfg.GoTLSExcludeSelf = s.isTLS
	return cfg
}

func TestHTTP(t *testing.T) {
	if kv < http.MinimumKernelVersion {
		t.Skipf("USM is not supported on %v", kv)
	}
	ebpftest.TestBuildModes(t, []ebpftest.BuildMode{ebpftest.Prebuilt, ebpftest.RuntimeCompiled, ebpftest.CORE}, "", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			isTLS bool
		}{
			{
				name:  "without TLS",
				isTLS: false,
			},
			{
				name:  "with TLS",
				isTLS: true,
			},
		} {
			if tc.isTLS && !gotlsutils.GoTLSSupported(t, config.New()) {
				t.Skip("GoTLS not supported for this setup")
			}
			t.Run(tc.name, func(t *testing.T) {
				suite.Run(t, &httpTestSuite{isTLS: tc.isTLS})
			})
		}
	})
}

func (s *httpTestSuite) TestHTTPStats() {
	t := s.T()

	const serverAddr = "127.0.0.1:8080"
	// Start the proxy server.
	proxyProcess, cancel := proxy.NewExternalUnixTransparentProxyServer(t, unixPath, serverAddr, s.isTLS)
	t.Cleanup(cancel)

	testCases := []struct {
		name                  string
		aggregateByStatusCode bool
	}{
		{
			name:                  "status code",
			aggregateByStatusCode: true,
		},
		{
			name:                  "status class",
			aggregateByStatusCode: false,
		},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			// Start an HTTP/HTTPS Server
			srvDoneFn := testutil.HTTPServer(t, serverAddr, testutil.Options{
				EnableKeepAlive: true,
				EnableTLS:       s.isTLS,
			})
			t.Cleanup(srvDoneFn)

			// Wait for the proxy server to be ready.
			require.NoError(t, proxy.WaitForConnectionReady(unixPath))
			cfg := s.getCfg()
			cfg.EnableHTTPStatsByStatusCode = tt.aggregateByStatusCode
			monitor := setupUSMTLSMonitor(t, cfg)
			if s.isTLS {
				utils.WaitForProgramsToBeTraced(t, "go-tls", proxyProcess.Process.Pid)
			}

			client := getHTTPUnixClientArray(1, unixPath)[0]
			resp, err := client.Get(fmt.Sprintf("http://unix/%d/test", nethttp.StatusNoContent))
			require.NoError(t, err)
			_ = resp.Body.Close()
			srvDoneFn()

			// Iterate through active connections until we find connection created above
			require.Eventuallyf(t, func() bool {
				stats := getHTTPLikeProtocolStats(monitor, protocols.HTTP)

				for key, reqStats := range stats {
					if key.Method == http.MethodGet && strings.HasSuffix(key.Path.Content.Get(), "/test") && (key.SrcPort == 8080 || key.DstPort == 8080) {
						currentStats := reqStats.Data[reqStats.NormalizeStatusCode(nethttp.StatusNoContent)]
						if currentStats != nil && currentStats.Count == 1 {
							return true
						}
					}
				}

				return false
			}, 3*time.Second, 100*time.Millisecond, "couldn't find http connection matching: %s", serverAddr)
		})
	}
}

// TestHTTPMonitorLoadWithIncompleteBuffers sends thousands of requests without getting responses for them, in parallel
// we send another request. We expect to capture the another request but not the incomplete requests.
func (s *httpTestSuite) TestHTTPMonitorLoadWithIncompleteBuffers() {
	t := s.T()

	slowServerAddr := "localhost:8080"
	fastServerAddr := "localhost:8081"

	monitor := newHTTPMonitorWithCfg(t, config.New())
	slowSrvDoneFn := testutil.HTTPServer(t, slowServerAddr, testutil.Options{
		SlowResponse: time.Millisecond * 500, // Half a second.
		WriteTimeout: time.Millisecond * 200,
		ReadTimeout:  time.Millisecond * 200,
	})

	fastSrvDoneFn := testutil.HTTPServer(t, fastServerAddr, testutil.Options{})
	abortedRequestFn := requestGenerator(t, nil, fmt.Sprintf("%s/ignore", slowServerAddr), emptyBody)
	wg := sync.WaitGroup{}
	abortedRequests := make(chan *nethttp.Request, 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := abortedRequestFn()
			abortedRequests <- req
		}()
	}
	fastReq := requestGenerator(t, nil, fastServerAddr, emptyBody)()
	wg.Wait()
	close(abortedRequests)
	slowSrvDoneFn()
	fastSrvDoneFn()

	foundFastReq := false
	// We are iterating for a couple of iterations and making sure the aborted requests will never be found.
	// Since the every call for monitor.GetHTTPStats will delete the pop all entries, and we want to find fastReq
	// then we are using a variable to check if "we ever found it" among the iterations.
	for i := 0; i < 10; i++ {
		time.Sleep(10 * time.Millisecond)
		stats := getHTTPLikeProtocolStats(monitor, protocols.HTTP)
		for req := range abortedRequests {
			checkRequestIncluded(t, stats, req, false)
		}

		included, err := isRequestIncludedOnce(stats, fastReq)
		require.NoError(t, err)
		foundFastReq = foundFastReq || included
	}

	require.True(t, foundFastReq)
}

func (s *httpTestSuite) TestHTTPMonitorIntegrationWithResponseBody() {
	t := s.T()
	const serverAddr = "127.0.0.1:8080"
	// Start the proxy server.
	proxyProcess, cancel := proxy.NewExternalUnixTransparentProxyServer(t, unixPath, serverAddr, s.isTLS)
	t.Cleanup(cancel)

	tests := []struct {
		name            string
		requestBodySize int
	}{
		{
			name:            "no body",
			requestBodySize: 0,
		},
		{
			name:            "1kb body",
			requestBodySize: 1 * kb,
		},
		{
			name:            "10kb body",
			requestBodySize: 10 * kb,
		},
		{
			name:            "500kb body",
			requestBodySize: 500 * kb,
		},
		{
			name:            "10mb body",
			requestBodySize: 10 * mb,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvDoneFn := testutil.HTTPServer(t, serverAddr, testutil.Options{
				EnableKeepAlive: true,
				EnableTLS:       s.isTLS,
			})
			t.Cleanup(srvDoneFn)
			// Wait for the proxy server to be ready.
			require.NoError(t, proxy.WaitForConnectionReady(unixPath))
			monitor := setupUSMTLSMonitor(t, s.getCfg())
			if s.isTLS {
				utils.WaitForProgramsToBeTraced(t, "go-tls", proxyProcess.Process.Pid)
			}
			requestFn := requestGenerator(t, getHTTPUnixClientArray(1, unixPath)[0], serverAddr, bytes.Repeat([]byte("a"), tt.requestBodySize))
			var requests []*nethttp.Request
			for i := 0; i < 100; i++ {
				requests = append(requests, requestFn())
			}
			srvDoneFn()

			assertAllRequestsExists(t, monitor, requests)
		})
	}
}

// TestHTTPMonitorIntegrationSlowResponse sends a request and getting a slow response.
// The test checks multiple scenarios regarding USM's internal timeouts and cleaning intervals, and based on the values
// we check if we captured a request (and if we should have), or we didn't capture (and if we shouldn't have).
func (s *httpTestSuite) TestHTTPMonitorIntegrationSlowResponse() {
	t := s.T()
	const serverAddr = "localhost:8080"
	// Start the proxy server.
	proxyProcess, cancel := proxy.NewExternalUnixTransparentProxyServer(t, unixPath, serverAddr, s.isTLS)
	t.Cleanup(cancel)

	tests := []struct {
		name                         string
		mapCleanerIntervalSeconds    int
		httpIdleConnectionTTLSeconds int
		slowResponseTime             int
		shouldCapture                bool
	}{
		{
			name:                         "response reaching after cleanup",
			mapCleanerIntervalSeconds:    1,
			httpIdleConnectionTTLSeconds: 1,
			slowResponseTime:             3,
			shouldCapture:                false,
		},
		{
			name:                         "response reaching before cleanup",
			mapCleanerIntervalSeconds:    1,
			httpIdleConnectionTTLSeconds: 3,
			slowResponseTime:             1,
			shouldCapture:                true,
		},
		{
			name:                         "slow response reaching after ttl but cleaner not running",
			mapCleanerIntervalSeconds:    3,
			httpIdleConnectionTTLSeconds: 1,
			slowResponseTime:             2,
			shouldCapture:                true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := s.getCfg()
			cfg.HTTPMapCleanerInterval = time.Duration(tt.mapCleanerIntervalSeconds) * time.Second
			cfg.HTTPIdleConnectionTTL = time.Duration(tt.httpIdleConnectionTTLSeconds) * time.Second
			monitor := setupUSMTLSMonitor(t, cfg)
			if s.isTLS {
				utils.WaitForProgramsToBeTraced(t, "go-tls", proxyProcess.Process.Pid)
			}

			slowResponseTimeout := time.Duration(tt.slowResponseTime) * time.Second
			serverTimeout := slowResponseTimeout + time.Second
			srvDoneFn := testutil.HTTPServer(t, serverAddr, testutil.Options{
				WriteTimeout: serverTimeout,
				ReadTimeout:  serverTimeout,
				SlowResponse: slowResponseTimeout,
				EnableTLS:    s.isTLS,
			})
			t.Cleanup(srvDoneFn)
			require.NoError(t, proxy.WaitForConnectionReady(unixPath))

			// Create a request generator `requestGenerator(t, nil, serverAddr, emptyBody)`, and runs it once. We save
			// the request for a later comparison.
			req := requestGenerator(t, getHTTPUnixClientArray(1, unixPath)[0], serverAddr, emptyBody)()
			srvDoneFn()

			// Ensure all captured transactions get sent to user-space
			time.Sleep(10 * time.Millisecond)
			checkRequestIncluded(t, getHTTPLikeProtocolStats(monitor, protocols.HTTP), req, tt.shouldCapture)
		})
	}
}

func testNameHelper(optionTrue, optionFalse string, value bool) string {
	if value {
		return optionTrue
	}
	return optionFalse
}

// TestSanity checks that USM capture a random generated 100 requests send to a local HTTP server under the following
// conditions:
// 1. Server and client support keep alive, and there is no NAT.
// 2. Server and client do not support keep alive, and there is no NAT.
// 3. Server and client support keep alive, and there is DNAT.
// 4. Server and client do not support keep alive, and there is DNAT.
func (s *httpTestSuite) TestSanity() {
	t := s.T()
	serverAddrWithoutNAT := "localhost:8080"
	targetAddrWithNAT := "2.2.2.2:8080"
	serverAddrWithNAT := "1.1.1.1:8080"
	// SetupDNAT sets up a NAT translation from 2.2.2.2 to 1.1.1.1
	netlink.SetupDNAT(t)

	testCases := []struct {
		name          string
		serverAddress string
		targetAddress string
	}{
		{
			name:          "with dnat",
			serverAddress: serverAddrWithNAT,
			targetAddress: targetAddrWithNAT,
		},
		{
			name:          "without dnat",
			serverAddress: serverAddrWithoutNAT,
			targetAddress: serverAddrWithoutNAT,
		},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			// Start the proxy server.
			proxyProcess, cancel := proxy.NewExternalUnixTransparentProxyServer(t, unixPath, tt.targetAddress, s.isTLS)
			t.Cleanup(cancel)

			for _, keepAliveEnabled := range []bool{true, false} {
				t.Run(testNameHelper("with keep alive", "without keep alive", keepAliveEnabled), func(t *testing.T) {
					monitor := setupUSMTLSMonitor(t, s.getCfg())
					if s.isTLS {
						utils.WaitForProgramsToBeTraced(t, "go-tls", proxyProcess.Process.Pid)
					}

					srvDoneFn := testutil.HTTPServer(t, tt.serverAddress, testutil.Options{
						EnableKeepAlive: keepAliveEnabled,
						EnableTLS:       s.isTLS,
					})
					t.Cleanup(srvDoneFn)

					client := getHTTPUnixClientArray(1, unixPath)[0]
					// Create a request generator that will be used to randomly generate requests and send them to the server.
					requestFn := requestGenerator(t, client, "unix", emptyBody)
					var requests []*nethttp.Request
					for i := 0; i < 100; i++ {
						// Send a request to the server and save it for later comparison.
						requests = append(requests, requestFn())
					}
					srvDoneFn()
					client.CloseIdleConnections()

					// Ensure USM captured all requests.
					assertAllRequestsExists(t, monitor, requests)
				})
			}
		})
	}
}

// TestRSTPacketRegression checks that USM captures a request that was forcefully terminated by a RST packet.
func (s *httpTestSuite) TestRSTPacketRegression() {
	t := s.T()
	if s.isTLS {
		t.Skip("TLS not supported for this setup")
	}

	monitor := newHTTPMonitorWithCfg(t, config.New())

	serverAddr := "127.0.0.1:8080"
	srvDoneFn := testutil.HTTPServer(t, serverAddr, testutil.Options{
		EnableKeepAlive: true,
	})
	t.Cleanup(srvDoneFn)

	// Create a "raw" TCP socket that will serve as our HTTP client
	// We do this in order to configure the socket option SO_LINGER
	// so we can force a RST packet to be sent during termination
	c, err := net.DialTimeout("tcp", serverAddr, 5*time.Second)
	require.NoError(t, err)

	// Issue HTTP request
	c.Write([]byte("GET /200/foobar HTTP/1.1\nHost: 127.0.0.1:8080\n\n"))
	io.Copy(io.Discard, c)

	// Configure SO_LINGER to 0 so that triggers an RST when the socket is terminated
	require.NoError(t, c.(*net.TCPConn).SetLinger(0))
	c.Close()
	time.Sleep(100 * time.Millisecond)

	url, err := url.Parse("http://127.0.0.1:8080/200/foobar")
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		// Assert that the HTTP request was correctly handled despite its forceful termination
		stats := getHTTPLikeProtocolStats(monitor, protocols.HTTP)
		included, err := isRequestIncludedOnce(stats, &nethttp.Request{URL: url})
		return err == nil && included
	}, 3*time.Second, 100*time.Millisecond, "connection not found")
}

// TestKeepAliveWithIncompleteResponseRegression checks that USM captures a request, although we initially saw a
// response and then a request with its response.
func (s *httpTestSuite) TestKeepAliveWithIncompleteResponseRegression() {
	t := s.T()
	if s.isTLS {
		t.Skip("TLS not supported for this setup")
	}

	monitor := newHTTPMonitorWithCfg(t, config.New())

	const req = "GET /200/foobar HTTP/1.1\n"
	const rsp = "HTTP/1.1 200 OK\n"
	const serverAddr = "127.0.0.1:8080"

	srvFn := func(c net.Conn) {
		// emulates a half-transaction (beginning with a response)
		n, err := c.Write([]byte(rsp))
		require.NoError(t, err)
		require.Equal(t, len(rsp), n)

		// now we read the request from the client on the same connection
		b := make([]byte, len(req))
		n, err = c.Read(b)
		require.NoError(t, err)
		require.Equal(t, len(req), n)
		require.Equal(t, string(b), req)

		// and finally send the response completing a full HTTP transaction
		n, err = c.Write([]byte(rsp))
		require.NoError(t, err)
		require.Equal(t, len(rsp), n)
		c.Close()
	}
	srv := testutil.NewTCPServer(serverAddr, srvFn, false)
	done := make(chan struct{})
	srv.Run(done)
	t.Cleanup(func() { close(done) })

	c, err := net.DialTimeout("tcp", serverAddr, 5*time.Second)
	require.NoError(t, err)

	// ensure we're beginning the connection with a "headless" response from the
	// server. this emulates the case where system-probe started in the middle of
	// request/response cycle
	b := make([]byte, len(rsp))
	n, err := c.Read(b)
	require.NoError(t, err)
	require.Equal(t, len(rsp), n)
	require.Equal(t, string(b), rsp)

	// now perform a request
	n, err = c.Write([]byte(req))
	require.NoError(t, err)
	require.Equal(t, len(req), n)

	// and read the response completing a full transaction
	n, err = c.Read(b)
	require.NoError(t, err)
	require.Equal(t, len(rsp), n)
	require.Equal(t, string(b), rsp)

	// after this response, request, response cycle we should ensure that
	// we got a full HTTP transaction
	url, err := url.Parse("http://127.0.0.1:8080/200/foobar")
	require.NoError(t, err)
	assertAllRequestsExists(t, monitor, []*nethttp.Request{{URL: url, Method: "GET"}})
}

func assertAllRequestsExists(t *testing.T, monitor *Monitor, requests []*nethttp.Request) {
	requestsExist := make([]bool, len(requests))

	assert.Eventually(t, func() bool {
		stats := getHTTPLikeProtocolStats(monitor, protocols.HTTP)

		if len(stats) == 0 {
			return false
		}

		for reqIndex, req := range requests {
			if !requestsExist[reqIndex] {
				exists, err := isRequestIncludedOnce(stats, req)
				require.NoError(t, err)
				requestsExist[reqIndex] = exists
			}
		}

		// Slight optimization here, if one is missing, then go into another cycle of checking the new connections.
		// otherwise, if all present, abort.
		for _, exists := range requestsExist {
			if !exists {
				return false
			}
		}

		return true
	}, 3*time.Second, time.Millisecond*100, "connection not found")

	if t.Failed() {
		ebpftest.DumpMapsTestHelper(t, monitor.DumpMaps, "http_in_flight")

		for reqIndex, exists := range requestsExist {
			if !exists {
				// reqIndex is 0 based, while the number is requests[reqIndex] is 1 based.
				t.Logf("request %d was not found (req %v)", reqIndex+1, requests[reqIndex])
			}
		}
	}
}

var (
	httpMethods         = []string{nethttp.MethodGet, nethttp.MethodHead, nethttp.MethodPost, nethttp.MethodPut, nethttp.MethodPatch, nethttp.MethodDelete, nethttp.MethodOptions}
	httpMethodsWithBody = []string{nethttp.MethodPost, nethttp.MethodPut, nethttp.MethodPatch, nethttp.MethodDelete}
	statusCodes         = []int{nethttp.StatusOK, nethttp.StatusMultipleChoices, nethttp.StatusBadRequest, nethttp.StatusInternalServerError}
)

func requestGenerator(t *testing.T, outerClient *nethttp.Client, targetAddr string, reqBody []byte) func() *nethttp.Request {
	var (
		random  = rand.New(rand.NewSource(time.Now().Unix()))
		idx     = 0
		reqBuf  = make([]byte, 0, len(reqBody))
		respBuf = make([]byte, 512)
	)

	client := outerClient
	if client == nil {
		client = new(nethttp.Client)
		// Disabling http2
		tr := nethttp.DefaultTransport.(*nethttp.Transport).Clone()
		tr.ForceAttemptHTTP2 = false
		tr.TLSNextProto = make(map[string]func(authority string, c *tls.Conn) nethttp.RoundTripper)

		client.Transport = tr
	}

	return func() *nethttp.Request {
		idx++
		var method string
		var body io.Reader
		var finalBody []byte
		if len(reqBody) > 0 {
			finalBody = reqBuf[:0]
			finalBody = append(finalBody, []byte(strings.Repeat(" ", idx))...)
			finalBody = append(finalBody, reqBody...)
			body = bytes.NewReader(finalBody)

			// save resized-buffer
			reqBuf = finalBody

			method = httpMethodsWithBody[random.Intn(len(httpMethodsWithBody))]
		} else {
			method = httpMethods[random.Intn(len(httpMethods))]
		}
		status := statusCodes[random.Intn(len(statusCodes))]
		url := fmt.Sprintf("http://%s/%d/request-%d", targetAddr, status, idx)
		req, err := nethttp.NewRequest(method, url, body)
		require.NoError(t, err)

		resp, err := client.Do(req)
		if strings.Contains(targetAddr, "ignore") {
			return req
		}
		require.NoError(t, err)
		defer resp.Body.Close()
		if len(reqBody) > 0 {
			for {
				n, err := resp.Body.Read(respBuf)
				require.True(t, n <= len(finalBody))
				require.Equal(t, respBuf[:n], finalBody[:n])
				if err != nil {
					assert.Equal(t, io.EOF, err)
					break
				}
				finalBody = finalBody[n:]
			}
		}
		return req
	}
}

func checkRequestIncluded(t *testing.T, allStats map[http.Key]*http.RequestStats, req *nethttp.Request, expectedToBeIncluded bool) {
	included, err := isRequestIncludedOnce(allStats, req)
	require.NoError(t, err)
	if included != expectedToBeIncluded {
		t.Errorf(
			"%s not find HTTP transaction matching the following criteria:\n path=%s method=%s status=%d",
			testNameHelper("could", "should", expectedToBeIncluded),
			req.URL.Path,
			req.Method,
			testutil.StatusFromPath(req.URL.Path),
		)
	}
}

func isRequestIncludedOnce(allStats map[http.Key]*http.RequestStats, req *nethttp.Request) (bool, error) {
	occurrences := countRequestOccurrences(allStats, req)

	if occurrences == 1 {
		return true, nil
	} else if occurrences == 0 {
		return false, nil
	}
	return false, fmt.Errorf("expected to find 1 occurrence of %v, but found %d instead", req, occurrences)
}

func countRequestOccurrences(allStats map[http.Key]*http.RequestStats, req *nethttp.Request) int {
	expectedStatus := testutil.StatusFromPath(req.URL.Path)
	occurrences := 0
	for key, stats := range allStats {
		if key.Path.Content.Get() != req.URL.Path {
			continue
		}
		if requests, exists := stats.Data[expectedStatus]; exists && requests.Count > 0 {
			occurrences++
		}
	}

	return occurrences
}

func newHTTPMonitorWithCfg(t *testing.T, cfg *config.Config) *Monitor {
	cfg.EnableHTTPMonitoring = true

	monitor, err := NewMonitor(cfg, nil, nil, nil)
	skipIfNotSupported(t, err)
	require.NoError(t, err)
	t.Cleanup(func() {
		monitor.Stop()
		libtelemetry.Clear()
	})

	// at this stage the test can be legitimately skipped due to missing BTF information
	// in the context of CO-RE
	require.NoError(t, monitor.Start())
	return monitor
}

func skipIfNotSupported(t *testing.T, err error) {
	notSupported := new(errNotSupported)
	if errors.As(err, &notSupported) {
		t.Skipf("skipping test because this kernel is not supported: %s", notSupported)
	}
}

// getHTTPUnixClientArray creates an array of http clients over a unix socket.
func getHTTPUnixClientArray(size int, unixPath string) []*nethttp.Client {
	res := make([]*nethttp.Client, size)
	for i := 0; i < size; i++ {
		res[i] = &nethttp.Client{
			Transport: &nethttp.Transport{
				ForceAttemptHTTP2: false,
				TLSNextProto:      make(map[string]func(string, *tls.Conn) nethttp.RoundTripper),
				DialContext: func(context.Context, string, string) (net.Conn, error) {
					return net.Dial("unix", unixPath)
				},
			},
		}
	}
	return res
}
