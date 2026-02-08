// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (linux_bpf || (windows && npm)) && test

package usm

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"math/rand"
	nethttp "net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
)

// TestMonitor is an interface for testing monitors across platforms.
// Both Linux and Windows monitors implement this interface for unified testing.
type TestMonitor interface {
	// GetHTTPStats returns HTTP protocol statistics.
	GetHTTPStats() map[protocols.ProtocolType]interface{}
}

// statusCodeCount holds the expected status code and count for validation.
type statusCodeCount struct {
	statusCode uint16
	count      int
}

// getHTTPLikeProtocolStatsGeneric extracts HTTP protocol stats from any monitor implementing TestMonitor.
func getHTTPLikeProtocolStatsGeneric(t *testing.T, monitor TestMonitor, protocolType protocols.ProtocolType) map[http.Key]*http.RequestStats {
	t.Helper()

	allStats := monitor.GetHTTPStats()
	if allStats == nil {
		return nil
	}

	statsObj, ok := allStats[protocolType]
	if !ok {
		return nil
	}

	stats, ok := statsObj.(map[http.Key]*http.RequestStats)
	if !ok {
		return nil
	}

	return stats
}

// verifyHTTPStats validates that the expected HTTP endpoints are present in the stats.
// expectedEndpoints maps http.Key (without connection details) to expected status code and count.
// serverPort is used to filter stats to only those matching the server port.
// additionalValidator is optional - if provided, performs custom validation on each RequestStat.
// Returns true if all expected endpoints are found with matching status codes and counts.
func verifyHTTPStats(t *testing.T, monitor TestMonitor, expectedEndpoints map[http.Key]statusCodeCount, serverPort int, additionalValidator func(*testing.T, *http.RequestStat) bool) bool {
	t.Helper()

	stats := getHTTPLikeProtocolStatsGeneric(t, monitor, protocols.HTTP)
	if len(stats) == 0 {
		return false
	}

	// Build result map from actual stats
	result := make(map[http.Key]statusCodeCount)

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

			// Store in result map
			result[simpleKey] = statusCodeCount{
				statusCode: statusCode,
				count:      stat.Count,
			}
		}
	}

	// Compare result with expected endpoints
	if len(result) != len(expectedEndpoints) {
		return false
	}

	for key, expected := range expectedEndpoints {
		actual, ok := result[key]
		if !ok {
			return false
		}
		if actual.statusCode != expected.statusCode || actual.count != expected.count {
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

// httpStatsTestParams holds parameters for the common HTTP stats test.
type httpStatsTestParams struct {
	// serverPort is the port the test server will listen on
	serverPort int
	// setupMonitor is a platform-specific function to set up the monitor
	setupMonitor func(t *testing.T) TestMonitor
}

// runHTTPStatsTest runs the common HTTP stats test logic.
func runHTTPStatsTest(t *testing.T, params httpStatsTestParams) {
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
	require.Eventuallyf(t, func() bool {
		return verifyHTTPStats(t, monitor, expectedEndpoints, params.serverPort, nil)
	}, 3*time.Second, 100*time.Millisecond, "HTTP connections not found for %s", serverAddr)
}

// Size constants for body tests
const (
	kb = 1024
	mb = 1024 * kb
)

var (
	emptyBody = []byte(nil)
)

var (
	httpMethods         = []string{nethttp.MethodGet, nethttp.MethodHead, nethttp.MethodPost, nethttp.MethodPut, nethttp.MethodPatch, nethttp.MethodDelete, nethttp.MethodOptions, nethttp.MethodTrace}
	httpMethodsWithBody = []string{nethttp.MethodPost, nethttp.MethodPut, nethttp.MethodPatch, nethttp.MethodDelete}
	statusCodes         = []int{nethttp.StatusOK, nethttp.StatusMultipleChoices, nethttp.StatusBadRequest, nethttp.StatusInternalServerError}
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
	} else if occurrences == 0 {
		return false, nil
	}
	return false, fmt.Errorf("expected to find 1 occurrence of %v, but found %d instead", req, occurrences)
}

// assertAllRequestsExist verifies that all given requests were captured by the monitor.
func assertAllRequestsExist(t *testing.T, monitor TestMonitor, requests []*nethttp.Request) {
	requestsExist := make([]bool, len(requests))

	assert.Eventually(t, func() bool {
		stats := getHTTPLikeProtocolStatsGeneric(t, monitor, protocols.HTTP)

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
		for reqIndex, exists := range requestsExist {
			if !exists {
				// reqIndex is 0 based, while the number is requests[reqIndex] is 1 based.
				t.Logf("request %d was not found (req %v)", reqIndex+1, requests[reqIndex])
			}
		}
	}
}

// httpBodySizeTestParams holds parameters for the HTTP body size test.
type httpBodySizeTestParams struct {
	// serverPort is the port the test server will listen on
	serverPort int
	// setupMonitor is a platform-specific function to set up the monitor
	setupMonitor func(t *testing.T) TestMonitor
}

// runHTTPMonitorIntegrationWithResponseBodyTest runs the test for various HTTP body sizes.
func runHTTPMonitorIntegrationWithResponseBodyTest(t *testing.T, params httpBodySizeTestParams) {
	serverAddr := fmt.Sprintf("localhost:%d", params.serverPort)

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

			assertAllRequestsExist(t, monitor, requests)
		})
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
}

// runHTTPMonitorLoadWithIncompleteBuffersTest sends thousands of requests without getting responses for them,
// in parallel we send another request. We expect to capture the other request but not the incomplete requests.
func runHTTPMonitorLoadWithIncompleteBuffersTest(t *testing.T, params httpLoadTestParams) {
	slowServerAddr := fmt.Sprintf("localhost:%d", params.slowServerPort)
	fastServerAddr := fmt.Sprintf("localhost:%d", params.fastServerPort)

	monitor := params.setupMonitor(t)
	slowSrvDoneFn := testutil.HTTPServer(t, slowServerAddr, testutil.Options{
		SlowResponse: time.Millisecond * 500, // Half a second.
		WriteTimeout: time.Millisecond * 200,
		ReadTimeout:  time.Millisecond * 200,
	})

	fastSrvDoneFn := testutil.HTTPServer(t, fastServerAddr, testutil.Options{})
	abortedRequestFn := requestGenerator(t, slowServerAddr+"/ignore", emptyBody)
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
	fastReq := requestGenerator(t, fastServerAddr, emptyBody)()
	wg.Wait()
	close(abortedRequests)
	slowSrvDoneFn()
	fastSrvDoneFn()

	foundFastReq := false
	// We are iterating for a couple of iterations and making sure the aborted requests will never be found.
	// Since every call for monitor.GetHTTPStats will delete/pop all entries, and we want to find fastReq
	// then we are using a variable to check if "we ever found it" among the iterations.
	for i := 0; i < 10; i++ {
		time.Sleep(10 * time.Millisecond)
		stats := getHTTPLikeProtocolStatsGeneric(t, monitor, protocols.HTTP)
		for req := range abortedRequests {
			checkRequestIncluded(t, stats, req, false)
		}

		included, err := isRequestIncludedOnce(stats, fastReq)
		require.NoError(t, err)
		foundFastReq = foundFastReq || included
	}

	require.True(t, foundFastReq)
}
