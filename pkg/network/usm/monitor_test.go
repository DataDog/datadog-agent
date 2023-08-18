// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"bytes"
	"crypto/tls"
	"errors"
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
	"github.com/stretchr/testify/suite"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	networkconfig "github.com/DataDog/datadog-agent/pkg/network/config"
	netlink "github.com/DataDog/datadog-agent/pkg/network/netlink/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	libtelemetry "github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

const (
	kb = 1024
	mb = 1024 * kb
)

var (
	emptyBody = []byte(nil)
)

func TestMonitorProtocolFail(t *testing.T) {
	failingPreStartupMock := func(_ *manager.Manager, _ protocols.BuildMode) error {
		return fmt.Errorf("mock error")
	}
	failingPostStartupMock := func(_ *manager.Manager) error {
		return fmt.Errorf("mock error")
	}

	testCases := []struct {
		name string
		spec protocolMockSpec
	}{
		{name: "PreStart fails", spec: protocolMockSpec{preStartFn: failingPreStartupMock}},
		{name: "PostStart fails", spec: protocolMockSpec{postStartFn: failingPostStartupMock}},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			// Replace the HTTP protocol with a Mock
			patchProtocolMock(t, tt.spec)

			cfg := networkconfig.New()
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

type HTTPTestSuite struct {
	suite.Suite
}

func TestHTTP(t *testing.T) {
	ebpftest.TestBuildModes(t, []ebpftest.BuildMode{ebpftest.Prebuilt, ebpftest.RuntimeCompiled, ebpftest.CORE}, "", func(t *testing.T) {
		suite.Run(t, new(HTTPTestSuite))
	})
}

func (s *HTTPTestSuite) TestHTTPMonitorCaptureRequestMultipleTimes() {
	t := s.T()

	for _, TCPTimestamp := range []struct {
		name  string
		value bool
	}{
		{name: "without TCP timestamp option", value: false},
		{name: "with TCP timestamp option", value: true},
	} {
		t.Run(TCPTimestamp.name, func(t *testing.T) {

			monitor := newHTTPMonitor(t)

			serverAddr := "localhost:8081"
			srvDoneFn := testutil.HTTPServer(t, serverAddr, testutil.Options{
				EnableTCPTimestamp: &TCPTimestamp.value,
			})

			client := nethttp.Client{}

			req, err := nethttp.NewRequest(httpMethods[0], fmt.Sprintf("http://%s/%d/request", serverAddr, nethttp.StatusOK), nil)
			require.NoError(t, err)

			expectedOccurrences := 10
			for i := 0; i < expectedOccurrences; i++ {
				resp, err := client.Do(req)
				require.NoError(t, err)
				// Have to read the response body to ensure the client will be able to properly close the connection.
				io.ReadAll(resp.Body)
				resp.Body.Close()
			}
			srvDoneFn()

			occurrences := 0
			require.Eventually(t, func() bool {
				stats := getHttpStats(t, monitor)
				occurrences += countRequestOccurrences(stats, req)
				return occurrences == expectedOccurrences
			}, time.Second*3, time.Millisecond*100, "Expected to find a request %d times, instead captured %d", occurrences, expectedOccurrences)
		})
	}
}

// TestHTTPMonitorLoadWithIncompleteBuffers sends thousands of requests without getting responses for them, in parallel
// we send another request. We expect to capture the another request but not the incomplete requests.
func (s *HTTPTestSuite) TestHTTPMonitorLoadWithIncompleteBuffers() {
	t := s.T()

	slowServerAddr := "localhost:8080"
	fastServerAddr := "localhost:8081"

	for _, TCPTimestamp := range []struct {
		name  string
		value bool
	}{
		{name: "without TCP timestamp option", value: false},
		{name: "with TCP timestamp option", value: true},
	} {
		t.Run(TCPTimestamp.name, func(t *testing.T) {
			monitor := newHTTPMonitor(t)
			slowSrvDoneFn := testutil.HTTPServer(t, slowServerAddr, testutil.Options{
				SlowResponse:       time.Millisecond * 500, // Half a second.
				WriteTimeout:       time.Millisecond * 200,
				ReadTimeout:        time.Millisecond * 200,
				EnableTCPTimestamp: &TCPTimestamp.value,
			})

			fastSrvDoneFn := testutil.HTTPServer(t, fastServerAddr, testutil.Options{})
			abortedRequestFn := requestGenerator(t, fmt.Sprintf("%s/ignore", slowServerAddr), emptyBody)
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
			// Since the every call for monitor.GetHTTPStats will delete the pop all entries, and we want to find fastReq
			// then we are using a variable to check if "we ever found it" among the iterations.
			for i := 0; i < 10; i++ {
				time.Sleep(10 * time.Millisecond)
				stats := getHttpStats(t, monitor)
				for req := range abortedRequests {
					requestNotIncluded(t, stats, req)
				}

				included, err := isRequestIncludedOnce(stats, fastReq)
				require.NoError(t, err)
				foundFastReq = foundFastReq || included
			}

			require.True(t, foundFastReq)
		})
	}
}

func (s *HTTPTestSuite) TestHTTPMonitorIntegrationWithResponseBody() {
	t := s.T()
	targetAddr := "localhost:8080"
	serverAddr := "localhost:8080"

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
			name:            "100kb body",
			requestBodySize: 100 * kb,
		},
		{
			name:            "500kb body",
			requestBodySize: 500 * kb,
		},
		{
			name:            "2mb body",
			requestBodySize: 2 * mb,
		},
		{
			name:            "10mb body",
			requestBodySize: 10 * mb,
		},
	}
	for _, TCPTimestamp := range []struct {
		name  string
		value bool
	}{
		{name: "without TCP timestamp option", value: false},
		{name: "with TCP timestamp option", value: true},
	} {
		t.Run(TCPTimestamp.name, func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					monitor := newHTTPMonitor(t)
					srvDoneFn := testutil.HTTPServer(t, serverAddr, testutil.Options{
						EnableKeepAlive:    true,
						EnableTCPTimestamp: &TCPTimestamp.value,
					})
					t.Cleanup(srvDoneFn)

					requestFn := requestGenerator(t, targetAddr, bytes.Repeat([]byte("a"), tt.requestBodySize))
					var requests []*nethttp.Request
					for i := 0; i < 100; i++ {
						requests = append(requests, requestFn())
					}
					srvDoneFn()

					assertAllRequestsExists(t, monitor, requests)
				})
			}
		})
	}
}

func (s *HTTPTestSuite) TestHTTPMonitorIntegrationSlowResponse() {
	t := s.T()
	targetAddr := "localhost:8080"
	serverAddr := "localhost:8080"

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
		for _, TCPTimestamp := range []struct {
			name  string
			value bool
		}{
			{name: "without TCP timestamp option", value: false},
			{name: "with TCP timestamp option", value: true},
		} {
			t.Run(TCPTimestamp.name, func(t *testing.T) {
				t.Run(tt.name, func(t *testing.T) {
					config.ResetSystemProbeConfig(t)
					t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_MAP_CLEANER_INTERVAL_IN_S", strconv.Itoa(tt.mapCleanerIntervalSeconds))
					t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_IDLE_CONNECTION_TTL_IN_S", strconv.Itoa(tt.httpIdleConnectionTTLSeconds))
					monitor := newHTTPMonitor(t)

					slowResponseTimeout := time.Duration(tt.slowResponseTime) * time.Second
					serverTimeout := slowResponseTimeout + time.Second
					srvDoneFn := testutil.HTTPServer(t, serverAddr, testutil.Options{
						WriteTimeout:       serverTimeout,
						ReadTimeout:        serverTimeout,
						SlowResponse:       slowResponseTimeout,
						EnableTCPTimestamp: &TCPTimestamp.value,
					})
					t.Cleanup(srvDoneFn)

					// Perform a number of random requests
					req := requestGenerator(t, targetAddr, emptyBody)()
					srvDoneFn()

					// Ensure all captured transactions get sent to user-space
					time.Sleep(10 * time.Millisecond)
					stats := getHttpStats(t, monitor)

					if tt.shouldCapture {
						includesRequest(t, stats, req)
					} else {
						requestNotIncluded(t, stats, req)
					}
				})
			})
		}
	}
}

func (s *HTTPTestSuite) TestHTTPMonitorIntegration() {
	t := s.T()
	targetAddr := "localhost:8080"
	serverAddr := "localhost:8080"

	for _, TCPTimestamp := range []struct {
		name  string
		value bool
	}{
		{name: "without TCP timestamp option", value: false},
		{name: "with TCP timestamp option", value: true},
	} {
		t.Run(TCPTimestamp.name, func(t *testing.T) {
			t.Run("with keep-alives", func(t *testing.T) {
				testHTTPMonitor(t, targetAddr, serverAddr, 100, testutil.Options{
					EnableKeepAlive:    true,
					EnableTCPTimestamp: &TCPTimestamp.value,
				})
			})
			t.Run("without keep-alives", func(t *testing.T) {
				testHTTPMonitor(t, targetAddr, serverAddr, 100, testutil.Options{
					EnableKeepAlive:    false,
					EnableTCPTimestamp: &TCPTimestamp.value,
				})
			})
		})
	}
}

func (s *HTTPTestSuite) TestHTTPMonitorIntegrationWithNAT() {
	t := s.T()
	// SetupDNAT sets up a NAT translation from 2.2.2.2 to 1.1.1.1
	netlink.SetupDNAT(t)

	targetAddr := "2.2.2.2:8080"
	serverAddr := "1.1.1.1:8080"

	for _, TCPTimestamp := range []struct {
		name  string
		value bool
	}{
		{name: "without TCP timestamp option", value: false},
		{name: "with TCP timestamp option", value: true},
	} {
		t.Run(TCPTimestamp.name, func(t *testing.T) {
			t.Run("with keep-alives", func(t *testing.T) {
				testHTTPMonitor(t, targetAddr, serverAddr, 100, testutil.Options{
					EnableKeepAlive:    true,
					EnableTCPTimestamp: &TCPTimestamp.value,
				})
			})
			t.Run("without keep-alives", func(t *testing.T) {
				testHTTPMonitor(t, targetAddr, serverAddr, 100, testutil.Options{
					EnableKeepAlive:    false,
					EnableTCPTimestamp: &TCPTimestamp.value,
				})
			})
		})
	}
}

func (s *HTTPTestSuite) TestUnknownMethodRegression() {
	t := s.T()

	// SetupDNAT sets up a NAT translation from 2.2.2.2 to 1.1.1.1
	netlink.SetupDNAT(t)

	for _, TCPTimestamp := range []struct {
		name  string
		value bool
	}{
		{name: "without TCP timestamp option", value: false},
		{name: "with TCP timestamp option", value: true},
	} {
		t.Run(TCPTimestamp.name, func(t *testing.T) {
			monitor := newHTTPMonitor(t)
			targetAddr := "2.2.2.2:8080"
			serverAddr := "1.1.1.1:8080"
			serverAddrIP := util.AddressFromString("1.1.1.1")
			srvDoneFn := testutil.HTTPServer(t, serverAddr, testutil.Options{
				EnableTLS:          false,
				EnableKeepAlive:    true,
				EnableTCPTimestamp: &TCPTimestamp.value,
			})
			t.Cleanup(srvDoneFn)

			requestFn := requestGenerator(t, targetAddr, emptyBody)
			for i := 0; i < 100; i++ {
				requestFn()
			}

			time.Sleep(5 * time.Second)
			stats := getHttpStats(t, monitor)
			tel := telemetry.ReportPayloadTelemetry("1")
			requestsSum := 0
			for key := range stats {
				if key.Method == http.MethodUnknown {
					t.Error("detected HTTP request with method unknown")
				}
				// we just want our requests
				if strings.Contains(key.Path.Content, "/request-") &&
					key.DstPort == 8080 &&
					util.FromLowHigh(key.DstIPLow, key.DstIPHigh) == serverAddrIP {
					requestsSum++
				}
			}

			require.Equal(t, int64(0), tel["usm.http.dropped"])
			require.Equal(t, int64(0), tel["usm.http.rejected"])
			require.Equal(t, int64(0), tel["usm.http.malformed"])
			// requestGenerator() doesn't query 100 responses
			require.Equal(t, int64(0), tel["usm.http.hits1XX"])

			require.Equal(t, int(100), requestsSum)
		})
	}
}

func (s *HTTPTestSuite) TestRSTPacketRegression() {
	t := s.T()

	for _, TCPTimestamp := range []struct {
		name  string
		value bool
	}{
		{name: "without TCP timestamp option", value: false},
		{name: "with TCP timestamp option", value: true},
	} {
		t.Run(TCPTimestamp.name, func(t *testing.T) {

			monitor := newHTTPMonitor(t)

			serverAddr := "127.0.0.1:8080"
			srvDoneFn := testutil.HTTPServer(t, serverAddr, testutil.Options{
				EnableKeepAlive:    true,
				EnableTCPTimestamp: &TCPTimestamp.value,
			})
			t.Cleanup(srvDoneFn)

			// Create a "raw" TCP socket that will serve as our HTTP client
			// We do this in order to configure the socket option SO_LINGER
			// so we can force a RST packet to be sent during termination
			c, err := net.DialTimeout("tcp", serverAddr, 5*time.Second)
			if err != nil {
				t.Fatal(err)
			}

			// Issue HTTP request
			c.Write([]byte("GET /200/foobar HTTP/1.1\nHost: 127.0.0.1:8080\n\n"))
			io.Copy(io.Discard, c)

			// Configure SO_LINGER to 0 so that triggers an RST when the socket is terminated
			require.NoError(t, c.(*net.TCPConn).SetLinger(0))
			c.Close()
			time.Sleep(100 * time.Millisecond)

			// Assert that the HTTP request was correctly handled despite its forceful termination
			stats := getHttpStats(t, monitor)
			url, err := url.Parse("http://127.0.0.1:8080/200/foobar")
			require.NoError(t, err)
			includesRequest(t, stats, &nethttp.Request{URL: url})
		})
	}
}

func (s *HTTPTestSuite) TestKeepAliveWithIncompleteResponseRegression() {
	t := s.T()

	for _, TCPTimestamp := range []struct {
		name  string
		value bool
	}{
		{name: "without TCP timestamp option", value: false},
		{name: "with TCP timestamp option", value: true},
	} {
		t.Run(TCPTimestamp.name, func(t *testing.T) {
			testutil.SetupNetIPV4TCPTimestamp(t, TCPTimestamp.value)

			monitor := newHTTPMonitor(t)

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
			srv := testutil.NewTCPServer(serverAddr, srvFn)
			done := make(chan struct{})
			srv.Run(done)
			t.Cleanup(func() { close(done) })

			c, err := net.DialTimeout("tcp", serverAddr, 5*time.Second)
			require.NoError(t, err)

			// ensure we're beginning the connection with a "headless" response from the
			// server. this emulates the case where system-probe started in the middle of
			// request/response cyle
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
		})
	}
}

func assertAllRequestsExists(t *testing.T, monitor *Monitor, requests []*nethttp.Request) {
	requestsExist := make([]bool, len(requests))
	for i := 0; i < 10; i++ {
		time.Sleep(10 * time.Millisecond)
		stats := getHttpStats(t, monitor)
		for reqIndex, req := range requests {
			included, err := isRequestIncludedOnce(stats, req)
			require.NoError(t, err)
			requestsExist[reqIndex] = requestsExist[reqIndex] || included
		}
		if allTrue(requestsExist) {
			return
		}
	}

	for reqIndex, exists := range requestsExist {
		require.Truef(t, exists, "request %d was not found (req %v)", reqIndex, requests[reqIndex])
	}
}

func allTrue(x []bool) bool {
	for _, v := range x {
		if !v {
			return false
		}
	}
	return true
}

func testHTTPMonitor(t *testing.T, targetAddr, serverAddr string, numReqs int, o testutil.Options) {
	monitor := newHTTPMonitor(t)

	srvDoneFn := testutil.HTTPServer(t, serverAddr, o)
	t.Cleanup(srvDoneFn)

	// Perform a number of random requests
	requestFn := requestGenerator(t, targetAddr, emptyBody)
	var requests []*nethttp.Request
	for i := 0; i < numReqs; i++ {
		requests = append(requests, requestFn())
	}
	srvDoneFn()

	// Ensure all captured transactions get sent to user-space
	assertAllRequestsExists(t, monitor, requests)
}

var (
	httpMethods         = []string{nethttp.MethodGet, nethttp.MethodHead, nethttp.MethodPost, nethttp.MethodPut, nethttp.MethodPatch, nethttp.MethodDelete, nethttp.MethodOptions}
	httpMethodsWithBody = []string{nethttp.MethodPost, nethttp.MethodPut, nethttp.MethodPatch, nethttp.MethodDelete}
	statusCodes         = []int{nethttp.StatusOK, nethttp.StatusMultipleChoices, nethttp.StatusBadRequest, nethttp.StatusInternalServerError}
)

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

func includesRequest(t *testing.T, allStats map[http.Key]*http.RequestStats, req *nethttp.Request) {
	expectedStatus := testutil.StatusFromPath(req.URL.Path)
	included, err := isRequestIncludedOnce(allStats, req)
	require.NoError(t, err)
	if !included {
		t.Errorf(
			"could not find HTTP transaction matching the following criteria:\n path=%s method=%s status=%d",
			req.URL.Path,
			req.Method,
			expectedStatus,
		)
	}
}

func requestNotIncluded(t *testing.T, allStats map[http.Key]*http.RequestStats, req *nethttp.Request) {
	included, err := isRequestIncludedOnce(allStats, req)
	require.NoError(t, err)
	if included {
		expectedStatus := testutil.StatusFromPath(req.URL.Path)
		t.Errorf(
			"should not find HTTP transaction matching the following criteria:\n path=%s method=%s status=%d",
			req.URL.Path,
			req.Method,
			expectedStatus,
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

func getHttpStats(t *testing.T, mon *Monitor) map[http.Key]*http.RequestStats {
	t.Helper()

	allStats := mon.GetProtocolStats()
	require.NotNil(t, allStats)

	httpStats, ok := allStats[protocols.HTTP]
	require.True(t, ok)

	return httpStats.(map[http.Key]*http.RequestStats)
}

func countRequestOccurrences(allStats map[http.Key]*http.RequestStats, req *nethttp.Request) int {
	expectedStatus := testutil.StatusFromPath(req.URL.Path)
	occurrences := 0
	for key, stats := range allStats {
		if key.Path.Content != req.URL.Path {
			continue
		}
		if requests, exists := stats.Data[expectedStatus]; exists && requests.Count > 0 {
			occurrences++
		}
	}

	return occurrences
}

func newHTTPMonitor(t *testing.T) *Monitor {
	cfg := networkconfig.New()
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
	err = monitor.Start()
	skipIfNotSupported(t, err)
	require.NoError(t, err)
	return monitor
}

func skipIfNotSupported(t *testing.T, err error) {
	notSupported := new(errNotSupported)
	if errors.As(err, &notSupported) {
		t.Skipf("skipping test because this kernel is not supported: %s", notSupported)
	}
}
