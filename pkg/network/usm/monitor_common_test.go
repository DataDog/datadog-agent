// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux_bpf || (windows && npm)

package usm

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"math/rand"
	"net"
	nethttp "net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
)

// TestMonitor is an interface for testing monitors across platforms.
// Both Linux and Windows monitors implement this interface for unified testing.
type TestMonitor interface {
	// GetHTTPStats returns HTTP protocol statistics.
	GetHTTPStats() map[http.Key]*http.RequestStats
}

// statusCodeCount holds the expected status code and count for validation.
type statusCodeCount struct {
	statusCode uint16
	count      int
}

// getHTTPStats extracts HTTP protocol stats from any monitor implementing TestMonitor.
func getHTTPStats(t *testing.T, monitor TestMonitor) map[http.Key]*http.RequestStats {
	t.Helper()
	return monitor.GetHTTPStats()
}

// verifyHTTPStats fetches HTTP stats from the monitor, accumulates them into the accumulated map
// across multiple calls (since getHTTPStats drains stats), and checks whether all expected endpoints
// have reached the expected counts.
// expectedOccurrences multiplies the expected count (1 on Linux, 2 on Windows where DDNPM sees both sides).
// additionalValidator is optional - if provided, performs custom validation on each RequestStat.
// Returns true if all expected endpoints are found with matching status codes and counts.
func verifyHTTPStats(t *testing.T, monitor TestMonitor, accumulated map[http.Key]statusCodeCount, expectedEndpoints map[http.Key]statusCodeCount, serverPort int, expectedOccurrences int, additionalValidator func(*testing.T, *http.RequestStat) bool) bool {
	t.Helper()

	stats := getHTTPStats(t, monitor)
	for key, reqStats := range stats {
		// Only check stats matching the server port
		if key.SrcPort != uint16(serverPort) && key.DstPort != uint16(serverPort) {
			continue
		}

		// Iterate through all status codes in the stats
		for statusCode, stat := range reqStats.Data {
			if stat == nil || stat.Count == 0 {
				continue
			}

			// Run additional validation if provided
			if additionalValidator != nil && !additionalValidator(t, stat) {
				continue
			}

			// Create a simplified key for comparison (normalize path and method only)
			simpleKey := http.Key{
				Method: key.Method,
				Path: http.Path{
					Content: key.Path.Content,
				},
			}

			// Accumulate counts for the same path/method/status across polls
			existing := accumulated[simpleKey]
			if existing.statusCode == 0 || existing.statusCode == statusCode {
				accumulated[simpleKey] = statusCodeCount{
					statusCode: statusCode,
					count:      existing.count + stat.Count,
				}
			}
		}
	}

	// Compare accumulated with expected endpoints
	for key, expected := range expectedEndpoints {
		actual, ok := accumulated[key]
		if !ok {
			return false
		}
		if actual.statusCode != expected.statusCode {
			return false
		}
		if actual.count < expected.count*expectedOccurrences {
			return false
		}
	}

	return true
}

// makeExpectedEndpoint creates an http.Key for expected endpoint verification.
func makeExpectedEndpoint(method http.Method, path string) http.Key {
	return http.Key{
		Path:   http.Path{Content: http.Interner.GetString(path)},
		Method: method,
	}
}

// commonTestParams holds shared parameters for common test functions.
type commonTestParams struct {
	// serverPort is the port the test server will listen on
	serverPort int
	// setupMonitor is a platform-specific function to set up the monitor
	setupMonitor func(t *testing.T) TestMonitor
	// expectedOccurrences is the exact number of times each request should be captured.
	// On Windows this should be 2, on Linux this should be 1.
	//ddnpm.sys hooks at the Application Layer Enforcement layer,
	//which fires per-process, per-socket. For same-host traffic between Process A (client) and Process B (server):
	//1. Process A's socket creates an outbound connection → ALE fires for Process A → ddnpm creates flow 1 and captures the HTTP transaction from A's perspective
	//2. Process B's socket accepts an inbound connection → ALE fires for Process B → ddnpm creates flow 2 and captures the same HTTP transaction from B's perspective
	//Two ALE events, two flows, two HTTP transactions — because the hook is at the application/socket boundary, not the packet level.
	//That's the architectural reason: packet-level (Linux) = 1 capture, per-process-flow-level (Windows) = 2 captures.
	expectedOccurrences int
}

// runHTTPStatsTest runs the common HTTP stats test logic.
func runHTTPStatsTest(t *testing.T, params commonTestParams) {
	serverAddr := fmt.Sprintf("127.0.0.1:%d", params.serverPort)
	t.Logf("Using server address: %s (port: %d)", serverAddr, params.serverPort)

	srvDoneFn := testutil.HTTPServer(t, serverAddr, testutil.Options{
		EnableKeepAlive: true,
	})
	t.Cleanup(srvDoneFn)

	monitor := params.setupMonitor(t)

	// Define test endpoints with status codes in path
	testEndpointPath := "/" + strconv.Itoa(nethttp.StatusNoContent) + "/test"
	healthEndpointPath := "/" + strconv.Itoa(nethttp.StatusOK) + "/api/health"

	// Make first request: GET /204/test with 204 status
	resp1, err := nethttp.Get("http://" + serverAddr + testEndpointPath)
	require.NoError(t, err)
	defer resp1.Body.Close()

	// Make second request: GET /200/api/health with 200 status
	resp2, err := nethttp.Get("http://" + serverAddr + healthEndpointPath)
	require.NoError(t, err)
	defer resp2.Body.Close()

	srvDoneFn()

	// Define expected endpoints
	expectedEndpoints := map[http.Key]statusCodeCount{
		makeExpectedEndpoint(http.MethodGet, testEndpointPath): {
			statusCode: 204,
			count:      1,
		},
		makeExpectedEndpoint(http.MethodGet, healthEndpointPath): {
			statusCode: 200,
			count:      1,
		},
	}

	// Verify both endpoints were captured by the monitor
	accumulated := make(map[http.Key]statusCodeCount)
	require.Eventuallyf(t, func() bool {
		return verifyHTTPStats(t, monitor, accumulated, expectedEndpoints, params.serverPort, params.expectedOccurrences, nil)
	}, 3*time.Second, 100*time.Millisecond, "HTTP connections not found for %s", serverAddr)
}

// Size constants for body tests
const (
	kb = 1024
	mb = 1024 * kb
)

var (
	// httpMethods and httpMethodsWithBody are defined in platform-specific files
	// (monitor_common_linux_test.go and monitor_common_windows_test.go) because
	// Windows maps some HTTP methods (TRACE, PATCH, CONNECT) to MethodUnknown,
	// which causes them to be silently dropped by the statkeeper.
	statusCodes = []int{nethttp.StatusOK, nethttp.StatusMultipleChoices, nethttp.StatusBadRequest, nethttp.StatusInternalServerError}
)

// requestGenerator creates a function that generates HTTP requests with random methods and status codes.
// If reqBody is non-empty, it will use methods that support request bodies.
func requestGenerator(t *testing.T, targetAddr string, reqBody []byte) func() *nethttp.Request {
	var (
		random  = rand.New(rand.NewSource(time.Now().Unix()))
		idx     = 0
		client  = new(nethttp.Client)
		reqBuf  = make([]byte, 0, len(reqBody))
		respBuf = make([]byte, 512)
	)

	// Disabling http2
	tr := nethttp.DefaultTransport.(*nethttp.Transport).Clone()
	tr.ForceAttemptHTTP2 = false
	tr.TLSNextProto = make(map[string]func(authority string, c *tls.Conn) nethttp.RoundTripper)

	client.Transport = tr

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

// countRequestOccurrences counts how many times a request appears in the stats.
func countRequestOccurrences(allStats map[http.Key]*http.RequestStats, req *nethttp.Request) int {
	expectedStatus := testutil.StatusFromPath(req.URL.Path)
	occurrences := 0
	for key, stats := range allStats {
		if key.Method.String() != req.Method {
			continue
		}
		if key.Path.Content.Get() != req.URL.Path {
			continue
		}
		if requests, exists := stats.Data[expectedStatus]; exists && requests.Count > 0 {
			occurrences++
		}
	}

	return occurrences
}

// isRequestIncludedOnce checks if a request appears exactly once in the stats.
func isRequestIncludedOnce(allStats map[http.Key]*http.RequestStats, req *nethttp.Request) (bool, error) {
	occurrences := countRequestOccurrences(allStats, req)

	if occurrences == 1 {
		return true, nil
	}
	if occurrences == 0 {
		return false, nil
	}
	return false, fmt.Errorf("expected to find 1 occurrence of %v, but found %d instead", req, occurrences)
}

// runHTTPMonitorIntegrationWithResponseBodyTest runs the test for various HTTP body sizes.
// It uses requestGenerator to create varied requests (random methods, status codes) and verifies
// that the monitor captures all of them.
func runHTTPMonitorIntegrationWithResponseBodyTest(t *testing.T, params commonTestParams) {
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
			serverAddr := fmt.Sprintf("127.0.0.1:%d", params.serverPort)

			monitor := params.setupMonitor(t)
			srvDoneFn := testutil.HTTPServer(t, serverAddr, testutil.Options{
				EnableKeepAlive: true,
			})
			t.Cleanup(srvDoneFn)

			requestFn := requestGenerator(t, serverAddr, bytes.Repeat([]byte("a"), tt.requestBodySize))
			var requests []*nethttp.Request
			for i := 0; i < 100; i++ {
				requests = append(requests, requestFn())
			}
			srvDoneFn()

			assertAllRequestsExist(t, monitor, requests, params.expectedOccurrences)
		})
	}
}

// assertAllRequestsExist verifies that all requests are found in the monitor stats
func assertAllRequestsExist(t *testing.T, monitor TestMonitor, requests []*nethttp.Request, expectedOccurrences int) {
	t.Helper()
	requestsExist := make([]bool, len(requests))
	requestsCapturedCount := make([]int, len(requests))

	assert.Eventually(t, func() bool {
		stats := getHTTPStats(t, monitor)
		if len(stats) == 0 {
			return false
		}

		for reqIndex, req := range requests {
			count := countRequestOccurrences(stats, req)
			requestsCapturedCount[reqIndex] += count
			if !requestsExist[reqIndex] {
				requestsExist[reqIndex] = requestsCapturedCount[reqIndex] >= expectedOccurrences
			}
		}

		for _, exists := range requestsExist {
			if !exists {
				return false
			}
		}
		return true
	}, 3*time.Second, 100*time.Millisecond, "not all requests were captured the expected number of times")

	for reqIndex := range requests {
		req := requests[reqIndex]
		assert.Equalf(t, expectedOccurrences, requestsCapturedCount[reqIndex],
			"request %d captured %d times, expected %d (req %v)", reqIndex+1, requestsCapturedCount[reqIndex], expectedOccurrences, req)
	}
}

// testNameHelper returns optionTrue if value is true, otherwise optionFalse.
func testNameHelper(optionTrue, optionFalse string, value bool) string {
	if value {
		return optionTrue
	}
	return optionFalse
}

// checkRequestIncluded verifies if a request is included (or not) in the stats.
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

// httpLoadTestParams holds parameters for the HTTP load with incomplete buffers test.
type httpLoadTestParams struct {
	// slowServerPort is the port for the slow server
	slowServerPort int
	// fastServerPort is the port for the fast server
	fastServerPort int
	// setupMonitor is a platform-specific function to set up the monitor
	setupMonitor func(t *testing.T) TestMonitor
	// expectedOccurrences is the exact number of times the fast request should be captured.

	expectedOccurrences int
}

// runHTTPMonitorLoadWithIncompleteBuffersTest sends thousands of requests without getting responses for them,
// in parallel we send another request. We expect to capture the other request but not the incomplete requests.
func runHTTPMonitorLoadWithIncompleteBuffersTest(t *testing.T, params httpLoadTestParams) {
	slowServerAddr := fmt.Sprintf("127.0.0.1:%d", params.slowServerPort)
	fastServerAddr := fmt.Sprintf("127.0.0.1:%d", params.fastServerPort)

	monitor := params.setupMonitor(t)
	slowSrvDoneFn := testutil.HTTPServer(t, slowServerAddr, testutil.Options{
		SlowResponse: time.Millisecond * 500, // Half a second.
		WriteTimeout: time.Millisecond * 200,
		ReadTimeout:  time.Millisecond * 200,
	})

	fastSrvDoneFn := testutil.HTTPServer(t, fastServerAddr, testutil.Options{})
	abortedRequestFn := requestGenerator(t, slowServerAddr+"/ignore", nil)
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
	fastReq := requestGenerator(t, fastServerAddr, nil)()
	wg.Wait()
	close(abortedRequests)
	slowSrvDoneFn()
	fastSrvDoneFn()

	totalFastReqOccurrences := 0
	// We are iterating for a couple of iterations and making sure the aborted requests will never be found.
	// Since every call for monitor.GetHTTPStats will delete/pop all entries, and we want to find fastReq
	// then we are accumulating the count across iterations.
	for i := 0; i < 10; i++ {
		time.Sleep(10 * time.Millisecond)
		stats := getHTTPStats(t, monitor)
		for req := range abortedRequests {
			checkRequestIncluded(t, stats, req, false)
		}

		totalFastReqOccurrences += countRequestOccurrences(stats, fastReq)
	}

	require.Equal(t, params.expectedOccurrences, totalFastReqOccurrences, "expected fast request to be captured exactly %d times, got %d", params.expectedOccurrences, totalFastReqOccurrences)
}

// runRSTPacketRegressionTest checks that USM captures a request that was forcefully terminated by a RST packet.
func runRSTPacketRegressionTest(t *testing.T, params commonTestParams) {
	serverAddr := fmt.Sprintf("127.0.0.1:%d", params.serverPort)

	srvDoneFn := testutil.HTTPServer(t, serverAddr, testutil.Options{
		EnableKeepAlive: true,
	})
	t.Cleanup(srvDoneFn)

	monitor := params.setupMonitor(t)

	// Create a "raw" TCP socket that will serve as our HTTP client
	// We do this in order to configure the socket option SO_LINGER
	// so we can force a RST packet to be sent during termination
	c, err := net.DialTimeout("tcp", serverAddr, 5*time.Second)
	require.NoError(t, err)

	// Issue HTTP request
	requestPath := "/200/foobar"
	c.Write([]byte(fmt.Sprintf("GET %s HTTP/1.1\nHost: %s\n\n", requestPath, serverAddr)))
	io.Copy(io.Discard, c)

	// Configure SO_LINGER to 0 so that triggers an RST when the socket is terminated
	require.NoError(t, c.(*net.TCPConn).SetLinger(0))
	c.Close()

	srvDoneFn()

	// Assert that the HTTP request was correctly handled despite its forceful termination
	reqURL, err := url.Parse(fmt.Sprintf("http://%s%s", serverAddr, requestPath))
	require.NoError(t, err)

	totalOccurrences := 0
	require.Eventually(t, func() bool {
		stats := getHTTPStats(t, monitor)
		totalOccurrences += countRequestOccurrences(stats, &nethttp.Request{URL: reqURL, Method: nethttp.MethodGet})
		return totalOccurrences >= params.expectedOccurrences
	}, 3*time.Second, 100*time.Millisecond, "HTTP request with RST termination not captured")
	require.Equal(t, params.expectedOccurrences, totalOccurrences, "expected exactly %d occurrences of RST request, got %d", params.expectedOccurrences, totalOccurrences)
}

// runKeepAliveWithIncompleteResponseRegressionTest checks that USM captures a request, although we initially saw a
// response and then a request with its response. This emulates the case where the monitor started in the middle of
// a request/response cycle.
func runKeepAliveWithIncompleteResponseRegressionTest(t *testing.T, params commonTestParams) {
	serverAddr := fmt.Sprintf("127.0.0.1:%d", params.serverPort)

	const req = "GET /200/foobar HTTP/1.1\nHost: 127.0.0.1\n\n"
	const rsp = "HTTP/1.1 200 OK\n"

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

	monitor := params.setupMonitor(t)

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

	c.Close()

	// after this response, request, response cycle we should ensure that
	// we got a full HTTP transaction
	reqURL, err := url.Parse(fmt.Sprintf("http://%s/200/foobar", serverAddr))
	require.NoError(t, err)

	totalOccurrences := 0
	require.Eventually(t, func() bool {
		stats := getHTTPStats(t, monitor)
		totalOccurrences += countRequestOccurrences(stats, &nethttp.Request{URL: reqURL, Method: nethttp.MethodGet})
		return totalOccurrences >= params.expectedOccurrences
	}, 3*time.Second, 100*time.Millisecond, "HTTP request with incomplete response not captured")
	require.Equal(t, params.expectedOccurrences, totalOccurrences, "expected exactly %d occurrences of keepalive request, got %d", params.expectedOccurrences, totalOccurrences)
}

// emptyConfigTestParams holds parameters for the empty config test.
type emptyConfigTestParams struct {
	// validateMonitorCreation is a platform-specific function to validate monitor creation with empty config.
	// On Linux: expects nil monitor with no error.
	// On Windows: expects non-nil monitor (Windows always creates a monitor).
	validateMonitorCreation func(t *testing.T)
}

// runEmptyConfigTest checks that NewUSMEmptyConfig returns a config with ServiceMonitoringEnabled=true
// and all protocols disabled. Platform-specific behavior for monitor creation is validated separately.
func runEmptyConfigTest(t *testing.T, params emptyConfigTestParams) {
	cfg := NewUSMEmptyConfig()

	// Verify ServiceMonitoringEnabled is true
	require.True(t, cfg.ServiceMonitoringEnabled, "ServiceMonitoringEnabled should be true in empty config")

	// Verify all protocols are disabled
	require.False(t, cfg.EnableHTTPMonitoring, "EnableHTTPMonitoring should be false")
	require.False(t, cfg.EnableHTTP2Monitoring, "EnableHTTP2Monitoring should be false")
	require.False(t, cfg.EnableKafkaMonitoring, "EnableKafkaMonitoring should be false")
	require.False(t, cfg.EnablePostgresMonitoring, "EnablePostgresMonitoring should be false")
	require.False(t, cfg.EnableRedisMonitoring, "EnableRedisMonitoring should be false")
	require.False(t, cfg.EnableNativeTLSMonitoring, "EnableNativeTLSMonitoring should be false")
	require.False(t, cfg.EnableGoTLSSupport, "EnableGoTLSSupport should be false")

	// Platform-specific monitor creation validation
	if params.validateMonitorCreation != nil {
		params.validateMonitorCreation(t)
	}
}
