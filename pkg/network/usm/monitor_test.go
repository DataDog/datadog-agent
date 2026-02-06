// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"bytes"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	nethttp "net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"golang.org/x/net/http2"

	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	networkConfig "github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	netlink "github.com/DataDog/datadog-agent/pkg/network/netlink/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	usmhttp2 "github.com/DataDog/datadog-agent/pkg/network/protocols/http2"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/redis"
	usmconfig "github.com/DataDog/datadog-agent/pkg/network/usm/config"
	usmtestutil "github.com/DataDog/datadog-agent/pkg/network/usm/testutil"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
)

func TestMain(m *testing.M) {
	logLevel := os.Getenv("DD_LOG_LEVEL")
	if logLevel == "" {
		logLevel = "warn"
	}
	log.SetupLogger(log.Default(), logLevel)
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
	failingStartupMock := func() error {
		return errors.New("mock error")
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

			cfg := NewUSMEmptyConfig()
			cfg.EnableHTTPMonitoring = true

			monitor, err := NewMonitor(cfg, nil, nil)
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
	if kv < usmconfig.MinimumKernelVersion {
		t.Skipf("USM is not supported on %v", kv)
	}
	ebpftest.TestBuildModes(t, usmtestutil.SupportedBuildModes(), "", func(t *testing.T) {
		suite.Run(t, new(HTTPTestSuite))
	})
}

func (s *HTTPTestSuite) TestHTTPStats() {
	t := s.T()

	// Start an HTTP server on localhost:8080
	serverAddr := "127.0.0.1:8080"
	srvDoneFn := testutil.HTTPServer(t, serverAddr, testutil.Options{
		EnableKeepAlive: true,
	})
	t.Cleanup(srvDoneFn)

	monitor := setupUSMTLSMonitor(t, getHTTPCfg(), useExistingConsumer)

	resp, err := nethttp.Get("http://" + serverAddr + "/" + strconv.Itoa(nethttp.StatusNoContent) + "/test")
	require.NoError(t, err)
	_ = resp.Body.Close()
	srvDoneFn()

	// Iterate through active connections until we find connection created above
	require.Eventuallyf(t, func() bool {
		stats := getHTTPLikeProtocolStats(t, monitor, protocols.HTTP)

		for key, reqStats := range stats {
			if key.Method == http.MethodGet && strings.HasSuffix(key.Path.Content.Get(), "/test") && (key.SrcPort == 8080 || key.DstPort == 8080) {
				currentStats := reqStats.Data[204]
				if currentStats != nil && currentStats.Count == 1 {
					return true
				}
			}
		}

		return false
	}, 3*time.Second, 100*time.Millisecond, "couldn't find http connection matching: %s", serverAddr)
}

// TestHTTPMonitorLoadWithIncompleteBuffers sends thousands of requests without getting responses for them, in parallel
// we send another request. We expect to capture the another request but not the incomplete requests.
func (s *HTTPTestSuite) TestHTTPMonitorLoadWithIncompleteBuffers() {
	t := s.T()

	slowServerAddr := "localhost:8080"
	fastServerAddr := "localhost:8081"

	monitor := setupUSMTLSMonitor(t, getHTTPCfg(), useExistingConsumer)
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
	// Since the every call for monitor.GetHTTPStats will delete the pop all entries, and we want to find fastReq
	// then we are using a variable to check if "we ever found it" among the iterations.
	for i := 0; i < 10; i++ {
		time.Sleep(10 * time.Millisecond)
		stats := getHTTPLikeProtocolStats(t, monitor, protocols.HTTP)
		for req := range abortedRequests {
			checkRequestIncluded(t, stats, req, false)
		}

		included, err := isRequestIncludedOnce(stats, fastReq)
		require.NoError(t, err)
		foundFastReq = foundFastReq || included
	}

	require.True(t, foundFastReq)
}

func (s *HTTPTestSuite) TestHTTPMonitorIntegrationWithResponseBody() {
	t := s.T()
	flake.MarkOnJobName(t, "ubuntu_25.10")
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
			monitor := setupUSMTLSMonitor(t, getHTTPCfg(), useExistingConsumer)
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

			assertAllRequestsExists(t, monitor, requests)
		})
	}
}

// TestHTTPMonitorIntegrationSlowResponse sends a request and getting a slow response.
// The test checks multiple scenarios regarding USM's internal timeouts and cleaning intervals, and based on the values
// we check if we captured a request (and if we should have), or we didn't capture (and if we shouldn't have).
func (s *HTTPTestSuite) TestHTTPMonitorIntegrationSlowResponse() {
	t := s.T()
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
		t.Run(tt.name, func(t *testing.T) {
			cfg := NewUSMEmptyConfig()
			cfg.EnableHTTPMonitoring = true
			cfg.HTTPMapCleanerInterval = time.Duration(tt.mapCleanerIntervalSeconds) * time.Second
			cfg.HTTPIdleConnectionTTL = time.Duration(tt.httpIdleConnectionTTLSeconds) * time.Second
			monitor := setupUSMTLSMonitor(t, cfg, useExistingConsumer)

			slowResponseTimeout := time.Duration(tt.slowResponseTime) * time.Second
			serverTimeout := slowResponseTimeout + time.Second
			srvDoneFn := testutil.HTTPServer(t, serverAddr, testutil.Options{
				WriteTimeout: serverTimeout,
				ReadTimeout:  serverTimeout,
				SlowResponse: slowResponseTimeout,
			})
			t.Cleanup(srvDoneFn)

			// Create a request generator `requestGenerator(t, serverAddr, emptyBody)`, and runs it once. We save
			// the request for a later comparison.
			req := requestGenerator(t, serverAddr, emptyBody)()
			srvDoneFn()

			// Ensure all captured transactions get sent to user-space
			time.Sleep(10 * time.Millisecond)
			checkRequestIncluded(t, getHTTPLikeProtocolStats(t, monitor, protocols.HTTP), req, tt.shouldCapture)
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
func (s *HTTPTestSuite) TestSanity() {
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
			for _, keepAliveEnabled := range []bool{true, false} {
				t.Run(testNameHelper("with keep alive", "without keep alive", keepAliveEnabled), func(t *testing.T) {
					monitor := setupUSMTLSMonitor(t, getHTTPCfg(), useExistingConsumer)

					srvDoneFn := testutil.HTTPServer(t, tt.serverAddress, testutil.Options{EnableKeepAlive: keepAliveEnabled})
					t.Cleanup(srvDoneFn)

					// Create a request generator that will be used to randomly generate requests and send them to the server.
					requestFn := requestGenerator(t, tt.targetAddress, emptyBody)
					var requests []*nethttp.Request
					for i := 0; i < 100; i++ {
						// Send a request to the server and save it for later comparison.
						requests = append(requests, requestFn())
					}
					srvDoneFn()

					// Ensure USM captured all requests.
					// Patching the recent change by testify
					time.Sleep(time.Second)
					assertAllRequestsExists(t, monitor, requests)
				})
			}
		})
	}
}

// TestRSTPacketRegression checks that USM captures a request that was forcefully terminated by a RST packet.
func (s *HTTPTestSuite) TestRSTPacketRegression() {
	t := s.T()

	monitor := setupUSMTLSMonitor(t, getHTTPCfg(), useExistingConsumer)

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

	// Assert that the HTTP request was correctly handled despite its forceful termination
	stats := getHTTPLikeProtocolStats(t, monitor, protocols.HTTP)
	url, err := url.Parse("http://127.0.0.1:8080/200/foobar")
	require.NoError(t, err)
	checkRequestIncluded(t, stats, &nethttp.Request{URL: url, Method: nethttp.MethodGet}, true)
}

// TestKeepAliveWithIncompleteResponseRegression checks that USM captures a request, although we initially saw a
// response and then a request with its response.
func (s *HTTPTestSuite) TestKeepAliveWithIncompleteResponseRegression() {
	t := s.T()

	monitor := setupUSMTLSMonitor(t, getHTTPCfg(), useExistingConsumer)

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

// TestEmptyConfig checks the test helper indeed returns a config with no
// protocols enable, by checking it prevents USM from running.
// If this test fails after enabling a protocol by default, you MUST NOT change
// this test, and instead update `NewUSMEmptyConfig` to make sure it disables the
// new protocol.
func TestEmptyConfig(t *testing.T) {
	cfg := NewUSMEmptyConfig()
	require.True(t, cfg.ServiceMonitoringEnabled)

	// The monitor should not start, and not return an error when no protocols
	// are enabled.
	monitor, err := NewMonitor(cfg, nil, nil)
	require.Nil(t, monitor)
	require.NoError(t, err)
}

func assertAllRequestsExists(t *testing.T, monitor *Monitor, requests []*nethttp.Request) {
	requestsExist := make([]bool, len(requests))

	assert.Eventually(t, func() bool {
		stats := getHTTPLikeProtocolStats(t, monitor, protocols.HTTP)

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
	httpMethods         = []string{nethttp.MethodGet, nethttp.MethodHead, nethttp.MethodPost, nethttp.MethodPut, nethttp.MethodPatch, nethttp.MethodDelete, nethttp.MethodOptions, nethttp.MethodTrace}
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

func skipIfNotSupported(t *testing.T, err error) {
	notSupported := new(errNotSupported)
	if errors.As(err, &notSupported) {
		t.Skipf("skipping test because this kernel is not supported: %s", notSupported)
	}
}

func cleanProtocolMaps(t *testing.T, protocolName string, manager *manager.Manager) {
	maps, err := manager.GetMaps()
	if err != nil {
		t.Logf("failed to get maps: %v", err)
		return
	}
	cleanMaps(t, protocolName, maps)
}

func cleanMaps(t *testing.T, protocolName string, maps map[string]*ebpf.Map) {
	for name, m := range maps {
		if !strings.Contains(name, protocolName) || strings.Contains(name, protocolName+"_batch") {
			continue
		}
		cleanMapEntries(t, m)
	}
}

func cleanMapEntries(t *testing.T, m *ebpf.Map) {
	switch m.Type() {
	case ebpf.Hash, ebpf.Array, ebpf.PerCPUHash, ebpf.PerCPUArray:
	default:
		return
	}

	keys := getAllKeys(t, m)
	if len(keys) == 0 {
		return
	}
	switch {
	case isPercpu(m.Type()):
		emptyValue := make([][]byte, ebpf.MustPossibleCPU())
		for i := range emptyValue {
			emptyValue[i] = make([]byte, m.ValueSize())
		}
		updateEntries(t, m, keys, emptyValue)
	case m.Type() == ebpf.Array:
		emptyValue := make([]byte, m.ValueSize())
		updateEntries(t, m, keys, emptyValue)
	default:
		deleteEntries(t, m, keys)
	}
}

func getAllKeys(t *testing.T, m *ebpf.Map) [][]byte {
	var keys [][]byte
	key := make([]byte, m.KeySize())

	var value interface{}
	if isPercpu(m.Type()) {
		valueSlice := make([][]byte, ebpf.MustPossibleCPU())
		for i := range valueSlice {
			valueSlice[i] = make([]byte, m.ValueSize())
		}
		value = valueSlice

	} else {
		value = make([]byte, m.ValueSize())
	}

	it := m.Iterate()
	for it.Next(&key, value) {
		keys = append(keys, append([]byte{}, key...))
	}
	if it.Err() != nil {
		t.Logf("failed to iterate over map %q: %v", m.String(), it.Err())
	}
	return keys
}

func updateEntries(t *testing.T, m *ebpf.Map, keys [][]byte, value interface{}) {
	for _, key := range keys {
		if err := m.Put(&key, value); err != nil {
			t.Log("failed zeroing map entry; error: ", err)
		}
	}
}

func deleteEntries(t *testing.T, m *ebpf.Map, keys [][]byte) {
	for _, key := range keys {
		if err := m.Delete(&key); err != nil {
			t.Log("failed deleting map entry; error: ", err)
		}
	}
}

func isPercpu(mapType ebpf.MapType) bool {
	return mapType == ebpf.PerCPUArray || mapType == ebpf.PerCPUHash
}

func generateMockMap(t *testing.T, mapType ebpf.MapType) (string, *ebpf.Map) {
	name := "test_" + mapType.String()
	m, err := ebpf.NewMap(&ebpf.MapSpec{
		Name:       name,
		Type:       mapType,
		KeySize:    4,
		ValueSize:  1,
		MaxEntries: 10,
	})
	require.NoError(t, err)
	t.Cleanup(func() { m.Close() })

	populateMockMap(t, m, mapType)
	return name, m
}

func populateMockMap(t *testing.T, m *ebpf.Map, mapType ebpf.MapType) {
	for i := 0; i < int(m.MaxEntries()); i++ {
		key := make([]byte, m.KeySize())
		binary.LittleEndian.PutUint32(key, uint32(i))

		valueSize := m.ValueSize()
		if isPercpu(mapType) {
			valueSize = uint32(ebpf.MustPossibleCPU())
		}
		value := make([]byte, valueSize)
		for j := 0; j < int(valueSize); j++ {
			value[j] = byte(i + j)
		}
		require.NoError(t, m.Put(key, value))
	}
}

func checkMockMapEntriesExist(t *testing.T, m *ebpf.Map) {
	key := make([]byte, m.KeySize())
	value := make([]byte, m.ValueSize())
	for i := 0; i < int(m.MaxEntries()); i++ {
		binary.LittleEndian.PutUint32(key, uint32(i))
		require.NoError(t, m.Lookup(&key, &value))
	}
}

func checkMockMapIsClean(t *testing.T, m *ebpf.Map) {
	key := make([]byte, m.KeySize())
	value := make([]byte, m.ValueSize())
	for i := 0; i < int(m.MaxEntries()); i++ {
		binary.LittleEndian.PutUint32(key, uint32(i))
		if m.Type() == ebpf.Array || isPercpu(m.Type()) {
			require.NoError(t, m.Lookup(&key, &value))
			require.Equal(t, make([]byte, len(value)), value, "Array/PerCPU map %s should be zeroed", m.Type().String())
		} else {
			require.Error(t, m.Lookup(&key, &value))
		}
	}
}

func TestCleanProtocolMaps(t *testing.T) {
	skipTestIfKernelNotSupported(t)
	mapTypes := []ebpf.MapType{ebpf.Hash, ebpf.Array, ebpf.PerCPUHash, ebpf.PerCPUArray}

	for _, mapType := range mapTypes {
		t.Run(mapType.String(), func(t *testing.T) {
			name, mockMap := generateMockMap(t, mapType)
			checkMockMapEntriesExist(t, mockMap)
			cleanMaps(t, "test", map[string]*ebpf.Map{name: mockMap})
			checkMockMapIsClean(t, mockMap)
		})
	}
}

var (
	httpBuffer = []byte("GET / HTTP/1.1\r\nHost: localhost\r\n\r\n")
	// A dump taken from wireshark, representing a produce v2 to a topic called "franz-kafka-2" with a payload
	// of "Hello Kafka!"
	kafkaBuffer = []byte{0x0, 0x0, 0x0, 0x5d, 0x0, 0x0, 0x0, 0x2, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xff, 0xff, 0x0, 0x0, 0x27, 0x10, 0x0, 0x0, 0x0, 0x1, 0x0, 0xd, 0x66, 0x72, 0x61, 0x6e, 0x7a, 0x2d, 0x6b, 0x61, 0x66, 0x6b, 0x61, 0x2d, 0x32, 0x0, 0x0, 0x0, 0x1, 0x0, 0x0, 0x0, 0x1, 0x0, 0x0, 0x0, 0x2e, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x22, 0x73, 0xcb, 0x7b, 0xc4, 0x1, 0x0, 0x0, 0x0, 0x1, 0x96, 0x32, 0x2f, 0x5b, 0xcb, 0xff, 0xff, 0xff, 0xff, 0x0, 0x0, 0x0, 0xc, 0x48, 0x65, 0x6c, 0x6c, 0x6f, 0x20, 0x4b, 0x61, 0x66, 0x6b, 0x61, 0x21}
	// A dump taken from wireshark, representing a simple postgres query of "SELECT 1"
	postgresBuffer = []byte{0x51, 0x0, 0x0, 0x0, 0xd, 0x53, 0x45, 0x4c, 0x45, 0x43, 0x54, 0x20, 0x31, 0x0}
	// A buffer matching redis classification. The min length is 3 characters, the first character is a special marker
	// `+`, then a chain of valid characters, and finally a CRLF.
	redisBuffer  = []byte("+AAA\r\n")
	randomBuffer = []byte("random payload")
	amqpBuffer   = []byte("AMQP")
)

// skipIfHTTP2KernelNotSupported returns a skip function for HTTP2 kernel checks that matches func(*testing.T) signature
func skipIfHTTP2KernelNotSupported() func(*testing.T) {
	return func(t *testing.T) {
		t.Helper()
		skipIfKernelNotSupported(t, usmhttp2.MinimumKernelVersion, "HTTP2")
	}
}

// skipIfRedisKernelNotSupported returns a skip function for Redis kernel checks that matches func(*testing.T) signature
func skipIfRedisKernelNotSupported() func(*testing.T) {
	return func(t *testing.T) {
		t.Helper()
		skipIfKernelNotSupported(t, redis.MinimumKernelVersion, "Redis")
	}
}

func TestConnectionStatesMap(t *testing.T) {
	skipTestIfKernelNotSupported(t)

	httpEnabledConfig := withConfigChange(NewUSMEmptyConfig(), func(cfg *networkConfig.Config) { cfg.EnableHTTPMonitoring = true })
	http2EnabledConfig := withConfigChange(NewUSMEmptyConfig(), func(cfg *networkConfig.Config) { cfg.EnableHTTP2Monitoring = true })
	postgresEnabledConfig := withConfigChange(NewUSMEmptyConfig(), func(cfg *networkConfig.Config) { cfg.EnablePostgresMonitoring = true })
	redisEnabledConfig := withConfigChange(NewUSMEmptyConfig(), func(cfg *networkConfig.Config) { cfg.EnableRedisMonitoring = true })
	kafkaEnabledConfig := withConfigChange(NewUSMEmptyConfig(), func(cfg *networkConfig.Config) { cfg.EnableKafkaMonitoring = true })

	tests := make([]connectionStatesMapTestCase, 0)
	tests = append(tests, connectionStatesMapTestCase{
		name:                "HTTP protocol enabled",
		cfg:                 httpEnabledConfig,
		expectedResult:      shouldExists,
		sendRequestCallback: sendAndReadBuffer(httpBuffer),
	}, connectionStatesMapTestCase{
		name:                "HTTP protocol disabled",
		cfg:                 http2EnabledConfig, // Enabling any protocol other than HTTP to allow USM to run
		expectedResult:      shouldNotExists,
		sendRequestCallback: sendAndReadBuffer(httpBuffer),
		skipCondition:       skipIfHTTP2KernelNotSupported(),
	}, connectionStatesMapTestCase{
		name:           "HTTP protocol already classified",
		cfg:            httpEnabledConfig,
		expectedResult: shouldExists,
		preTestSetup:   markConnectionProtocol(protocols.HTTP),
	}, connectionStatesMapTestCase{
		name:           "HTTP protocol already classified but not enabled",
		cfg:            redisEnabledConfig, // Enabling any protocol other than HTTP to allow USM to run
		expectedResult: shouldNotExists,
		preTestSetup:   markConnectionProtocol(protocols.HTTP),
		skipCondition:  skipIfRedisKernelNotSupported(),
	}, connectionStatesMapTestCase{
		name:                "HTTP2 protocol enabled",
		cfg:                 http2EnabledConfig,
		expectedResult:      shouldExists,
		sendRequestCallback: sendAndReadBuffer([]byte(http2.ClientPreface)),
		skipCondition:       skipIfHTTP2KernelNotSupported(),
	}, connectionStatesMapTestCase{
		name:                "HTTP2 protocol disabled",
		cfg:                 httpEnabledConfig, // Enabling any protocol other than HTTP2 to allow USM to run
		expectedResult:      shouldNotExists,
		sendRequestCallback: sendAndReadBuffer([]byte(http2.ClientPreface)),
	}, connectionStatesMapTestCase{
		name:           "HTTP2 protocol already classified",
		cfg:            http2EnabledConfig,
		expectedResult: shouldExists,
		preTestSetup:   markConnectionProtocol(protocols.HTTP2),
		skipCondition:  skipIfHTTP2KernelNotSupported(),
	}, connectionStatesMapTestCase{
		name:           "HTTP2 protocol already classified but not enabled",
		cfg:            httpEnabledConfig, // Enabling any protocol other than HTTP2 to allow USM to run
		expectedResult: shouldNotExists,
		preTestSetup:   markConnectionProtocol(protocols.HTTP2),
	}, connectionStatesMapTestCase{
		name:                "Kafka protocol enabled",
		cfg:                 kafkaEnabledConfig,
		expectedResult:      shouldExists,
		sendRequestCallback: sendAndReadBuffer(kafkaBuffer),
	}, connectionStatesMapTestCase{
		name:                "Kafka protocol disabled",
		cfg:                 httpEnabledConfig, // Enabling any protocol other than Kafka to allow USM to run
		expectedResult:      shouldNotExists,
		sendRequestCallback: sendAndReadBuffer(kafkaBuffer),
	}, connectionStatesMapTestCase{
		name:           "Kafka protocol already classified",
		cfg:            kafkaEnabledConfig,
		expectedResult: shouldExists,
		preTestSetup:   markConnectionProtocol(protocols.Kafka),
	}, connectionStatesMapTestCase{
		name:           "Kafka protocol already classified but not enabled",
		cfg:            httpEnabledConfig, // Enabling any protocol other than Kafka to allow USM to run
		expectedResult: shouldNotExists,
		preTestSetup:   markConnectionProtocol(protocols.Kafka),
	}, connectionStatesMapTestCase{
		name:                "postgres protocol enabled",
		cfg:                 postgresEnabledConfig,
		expectedResult:      shouldExists,
		sendRequestCallback: sendAndReadBuffer(postgresBuffer),
	}, connectionStatesMapTestCase{
		name:                "postgres protocol disabled",
		cfg:                 httpEnabledConfig, // Enabling any protocol other than Postgres to allow USM to run
		expectedResult:      shouldNotExists,
		sendRequestCallback: sendAndReadBuffer(postgresBuffer),
	}, connectionStatesMapTestCase{
		name:           "Postgres protocol already classified",
		cfg:            postgresEnabledConfig,
		expectedResult: shouldExists,
		preTestSetup:   markConnectionProtocol(protocols.Postgres),
	}, connectionStatesMapTestCase{
		name:           "Postgres protocol already classified but not enabled",
		cfg:            httpEnabledConfig, // Enabling any protocol other than Postgres to allow USM to run
		expectedResult: shouldNotExists,
		preTestSetup:   markConnectionProtocol(protocols.Postgres),
	}, connectionStatesMapTestCase{
		name:                "redis protocol enabled",
		cfg:                 redisEnabledConfig,
		expectedResult:      shouldExists,
		sendRequestCallback: sendAndReadBuffer(redisBuffer),
		skipCondition:       skipIfRedisKernelNotSupported(),
	}, connectionStatesMapTestCase{
		name:                "redis protocol disabled",
		cfg:                 httpEnabledConfig, // Enabling any protocol other than Redis to allow USM to run
		expectedResult:      shouldNotExists,
		sendRequestCallback: sendAndReadBuffer(redisBuffer),
	}, connectionStatesMapTestCase{
		name:           "Redis protocol already classified",
		cfg:            redisEnabledConfig,
		expectedResult: shouldExists,
		preTestSetup:   markConnectionProtocol(protocols.Redis),
		skipCondition:  skipIfRedisKernelNotSupported(),
	}, connectionStatesMapTestCase{
		name:           "Redis protocol already classified but not enabled",
		cfg:            httpEnabledConfig, // Enabling any protocol other than Redis to allow USM to run
		expectedResult: shouldNotExists,
		preTestSetup:   markConnectionProtocol(protocols.Redis),
	}, connectionStatesMapTestCase{
		name:           "random protocol",
		cfg:            httpEnabledConfig,
		expectedResult: shouldNotExists,
	}, connectionStatesMapTestCase{
		name:                "protocol is classified but not supported for decoding",
		cfg:                 httpEnabledConfig,
		expectedResult:      shouldNotExists,
		sendRequestCallback: sendAndReadBuffer(amqpBuffer),
	}, connectionStatesMapTestCase{
		name:           "protocol is already classified but not supported for decoding",
		cfg:            httpEnabledConfig,
		expectedResult: shouldNotExists,
		preTestSetup:   markConnectionProtocol(protocols.AMQP),
	}, connectionStatesMapTestCase{
		name:                "encrypted",
		cfg:                 httpEnabledConfig,
		expectedResult:      shouldNotExists,
		useTLS:              true,
		sendRequestCallback: sendAndReadBuffer(httpBuffer),
	}, connectionStatesMapTestCase{
		name:           "encrypted and classified",
		cfg:            httpEnabledConfig,
		expectedResult: shouldNotExists,
		useTLS:         true,
		preTestSetup:   markEncryptedConnectionProtocol(protocols.HTTP),
	})

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			testConnectionStatesMap(t, testCase)
		})
	}
}

// markEncryptedConnectionProtocol marks the connection and its reverse connection with a given protocol in the
// connection_protocol map, and marks the connection as encrypted.
func markEncryptedConnectionProtocol(protocol protocols.ProtocolType) func(*testing.T, *Monitor, net.Conn) {
	return markConnectionProtocolHelper(netebpf.ProtocolStack{
		Application: protocols.FromProtocolType(protocol),
		Encryption:  protocols.FromProtocolType(protocols.TLS),
	})
}

// markConnectionProtocol marks the connection and its reverse connection with a given protocol in the
// connection_protocol map. Assuming the connection is not encrypted.
func markConnectionProtocol(protocol protocols.ProtocolType) func(*testing.T, *Monitor, net.Conn) {
	return markConnectionProtocolHelper(netebpf.ProtocolStack{
		Application: protocols.FromProtocolType(protocol),
	})
}

// markConnectionProtocolHelper marks the connection and its reverse connection with a given protocol in the
// connection_protocol map.
func markConnectionProtocolHelper(protocolStack netebpf.ProtocolStack) func(*testing.T, *Monitor, net.Conn) {
	return func(t *testing.T, monitor *Monitor, conn net.Conn) {
		connProtocolMap, _, err := monitor.ebpfProgram.GetMap(probes.ConnectionProtocolMap)
		require.NoError(t, err)

		localIP, localPortStr, err := net.SplitHostPort(conn.LocalAddr().String())
		require.NoError(t, err)
		localPort, err := strconv.Atoi(localPortStr)
		require.NoError(t, err)
		remoteIP, remotePortStr, err := net.SplitHostPort(conn.RemoteAddr().String())
		require.NoError(t, err)
		remotePort, err := strconv.Atoi(remotePortStr)
		require.NoError(t, err)
		sourceAddressLow, sourceAddressHigh := util.ToLowHigh(util.AddressFromString(localIP))
		destinationAddressLow, destinationAddressHigh := util.ToLowHigh(util.AddressFromString(remoteIP))
		key := netebpf.ConnTuple{
			Saddr_h:  sourceAddressHigh,
			Saddr_l:  sourceAddressLow,
			Daddr_h:  destinationAddressHigh,
			Daddr_l:  destinationAddressLow,
			Sport:    uint16(localPort),
			Dport:    uint16(remotePort),
			Netns:    0, // Netns is always 0 for socket filter
			Pid:      0, // PID is always 0 for socket filter
			Metadata: 1, // TCP v4
		}
		value := netebpf.ProtocolStackWrapper{
			Updated: 1,
			Stack:   protocolStack,
		}

		require.NoError(t, connProtocolMap.Put(unsafe.Pointer(&key), unsafe.Pointer(&value)))

		key.Saddr_l = destinationAddressLow
		key.Saddr_h = destinationAddressHigh
		key.Daddr_l = sourceAddressLow
		key.Daddr_h = destinationAddressHigh
		key.Sport = uint16(remotePort)
		key.Dport = uint16(localPort)
		require.NoError(t, connProtocolMap.Put(unsafe.Pointer(&key), unsafe.Pointer(&value)))
	}
}

func withConfigChange(cfg *networkConfig.Config, change func(*networkConfig.Config)) *networkConfig.Config {
	change(cfg)
	return cfg
}

func sendAndReadBuffer(buffer []byte) func(*testing.T, net.Conn) {
	return func(t *testing.T, conn net.Conn) {
		_, err := conn.Write(buffer)
		require.NoError(t, err)
		_, err = conn.Read(buffer)
		require.NoError(t, err)
	}
}

type connectionStatesMapTestCase struct {
	// name is the name of the test
	name string
	// cfg is the USM configuration to use for the test
	cfg *networkConfig.Config
	// preTestSetup is a function that will be called before sending requests to the server, but
	// after monitor and client initialization. It can allow us to manipulate other USM maps, such as
	// connection_protocol to influence the test
	preTestSetup func(*testing.T, *Monitor, net.Conn)
	// useTLS indicates whether to use TLS for the connection
	useTLS bool
	// expectedResult indicates whether the connection should exist in the connection_states map
	expectedResult existenceResult
	// sendRequestCallback is a function that will be called to send the request to the server.
	// the method is responsible for reading the response from the server.
	sendRequestCallback func(*testing.T, net.Conn)
	// skipCondition is a function that will be called to determine whether to skip the test
	skipCondition func(*testing.T)
}

func testConnectionStatesMap(t *testing.T, testParams connectionStatesMapTestCase) {
	if testParams.skipCondition != nil {
		testParams.skipCondition(t)
	}
	monitor := setupUSMTLSMonitor(t, testParams.cfg, useExistingConsumer)

	serverConns := make([]net.Conn, 0)
	t.Cleanup(func() {
		closeConnections(serverConns...)
	})
	// echo server
	srvFn := func(conn net.Conn) {
		// We intentionally don't close the connection, to ensure the connection won't be deleted
		// from the `connection_states` map
		serverConns = append(serverConns, conn)
		// Echo back whatever is received
		_, _ = io.Copy(conn, conn)
	}
	srv := testutil.NewTCPServer("127.0.0.1:0", srvFn, testParams.useTLS)
	done := make(chan struct{})
	require.NoError(t, srv.Run(done))
	t.Cleanup(func() { close(done) })

	var clientConn net.Conn
	var err error
	if testParams.useTLS {
		clientConn, err = tls.Dial("tcp", srv.Address(), &tls.Config{InsecureSkipVerify: true})
	} else {
		clientConn, err = net.DialTimeout("tcp", srv.Address(), 5*time.Second)
	}
	require.NoError(t, err)
	t.Cleanup(func() { closeConnections(clientConn) })

	if testParams.preTestSetup != nil {
		testParams.preTestSetup(t, monitor, clientConn)
	}

	if testParams.sendRequestCallback != nil {
		testParams.sendRequestCallback(t, clientConn)
	} else {
		sendAndReadBuffer(randomBuffer)(t, clientConn)
	}

	m, _, err := monitor.ebpfProgram.GetMap(connectionStatesMap)
	require.NoError(t, err)

	checkConnExistenceInConnectionStatesMap(t, m, clientConn, testParams.expectedResult)

	// Close the connection and expect it to be removed from the map
	closeConnections(serverConns...)
	closeConnections(clientConn)
	// Wait for the connection to be removed from the map
	time.Sleep(100 * time.Millisecond)
	checkConnExistenceInConnectionStatesMap(t, m, clientConn, shouldNotExists)
}

type existenceResult bool

const (
	shouldExists    existenceResult = true
	shouldNotExists existenceResult = false
)

func checkConnExistenceInConnectionStatesMap(t *testing.T, m *ebpf.Map, conn net.Conn, expectedResult existenceResult) {
	iter := m.Iterate()
	var key, localConn, remoteConn netebpf.ConnTuple
	var value uint32

	_, clientLocalPortStr, err := net.SplitHostPort(conn.LocalAddr().String())
	require.NoError(t, err)
	clientLocalPort, err := strconv.Atoi(clientLocalPortStr)
	require.NoError(t, err)
	_, clientRemotePortStr, err := net.SplitHostPort(conn.RemoteAddr().String())
	require.NoError(t, err)
	clientRemotePort, err := strconv.Atoi(clientRemotePortStr)
	require.NoError(t, err)

	for iter.Next(&key, &value) {
		if key.Sport == uint16(clientLocalPort) && key.Dport == uint16(clientRemotePort) {
			localConn = key
		} else if key.Sport == uint16(clientRemotePort) && key.Dport == uint16(clientLocalPort) {
			remoteConn = key
		}

		if localConn.Dport != 0 && remoteConn.Dport != 0 {
			break
		}
	}
	if expectedResult {
		require.NotZero(t, localConn.Dport, "Client connection not found in connection_states map")
		require.NotZero(t, remoteConn.Dport, "Server connection not found in connection_states map")
	} else {
		require.Zero(t, localConn.Dport, "Client connection should not be found in connection_states map")
		require.Zero(t, remoteConn.Dport, "Server connection should not be found in connection_states map")
	}
}

func closeConnections(conns ...net.Conn) {
	for _, conn := range conns {
		if conn != nil {
			_ = conn.Close()
			conn = nil
		}
	}
}
