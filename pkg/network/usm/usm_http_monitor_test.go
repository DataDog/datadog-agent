// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"context"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	usmhttp "github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	gotlsutils "github.com/DataDog/datadog-agent/pkg/network/protocols/tls/gotls/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/testutil/proxy"
	"github.com/DataDog/datadog-agent/pkg/network/usm/consts"
	usmtestutil "github.com/DataDog/datadog-agent/pkg/network/usm/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"
)

const (
	httpSrvPort = 8080
)

var (
	serverAddr = net.JoinHostPort("127.0.0.1", strconv.Itoa(httpSrvPort))
)

type usmHTTPSuite struct {
	suite.Suite
	isTLS bool
}

func (s *usmHTTPSuite) getCfg() *config.Config {
	cfg := utils.NewUSMEmptyConfig()
	cfg.EnableHTTPMonitoring = true
	cfg.EnableGoTLSSupport = s.isTLS
	cfg.GoTLSExcludeSelf = s.isTLS
	return cfg
}

func TestHTTPScenarios(t *testing.T) {
	ebpftest.TestBuildModes(t, usmtestutil.SupportedBuildModes(), "", func(t *testing.T) {
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
			t.Run(tc.name, func(t *testing.T) {
				if tc.isTLS && !gotlsutils.GoTLSSupported(t, config.New()) {
					t.Skip("GoTLS not supported for this setup")
				}
				suite.Run(t, &usmHTTPSuite{isTLS: tc.isTLS})
			})
		}
	})
}

func (s *usmHTTPSuite) TestLoadHTTPBinary() {
	t := s.T()

	cfg := s.getCfg()

	for _, debug := range map[string]bool{"enabled": true, "disabled": false} {
		t.Run(fmt.Sprintf("debug %v", debug), func(t *testing.T) {
			cfg.BPFDebug = debug
			setupUSMTLSMonitor(t, cfg, useExistingConsumer)
		})
	}
}

func (s *usmHTTPSuite) TestSimpleHTTP() {
	t := s.T()
	cfg := s.getCfg()

	srvDoneFn := testutil.HTTPServer(t, serverAddr, testutil.Options{
		EnableTLS:       s.isTLS,
		EnableKeepAlive: true,
	})
	t.Cleanup(srvDoneFn)

	// Start the proxy server.
	proxyProcess, cancel := proxy.NewExternalUnixTransparentProxyServer(t, unixPath, serverAddr, s.isTLS)
	t.Cleanup(cancel)
	require.NoError(t, proxy.WaitForConnectionReady(unixPath))

	monitor := setupUSMTLSMonitor(t, cfg, useExistingConsumer)
	if s.isTLS {
		utils.WaitForProgramsToBeTraced(t, consts.USMModuleName, GoTLSAttacherName, proxyProcess.Process.Pid, utils.ManualTracingFallbackEnabled)
	}

	tests := []struct {
		name              string
		runClients        func(t *testing.T, clientsCount int)
		expectedEndpoints map[usmhttp.Key]int
	}{
		{
			name: "GET /hello",
			runClients: func(t *testing.T, clientsCount int) {
				clients := getHTTPUnixClientArray(clientsCount, unixPath)

				for i := 0; i < 1; i++ {
					client := clients[getClientsIndex(i, clientsCount)]
					req, err := client.Get("http://" + serverAddr + "/200/hello")
					require.NoError(t, err, "could not make request")
					_ = req.Body.Close()
				}
			},
			expectedEndpoints: map[usmhttp.Key]int{
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString("/200/hello")},
					Method: usmhttp.MethodGet,
				}: 1,
			},
		},
	}
	for _, tt := range tests {
		for _, clientCount := range []int{1} {
			testNameSuffix := fmt.Sprintf("-different clients - %v", clientCount)
			t.Run(tt.name+testNameSuffix, func(t *testing.T) {
				t.Cleanup(func() { cleanProtocolMaps(t, "http", monitor.ebpfProgram.Manager.Manager) })
				tt.runClients(t, clientCount)

				res := make(map[usmhttp.Key]int)
				assert.Eventually(t, func() bool {
					for key, stat := range getHTTPLikeProtocolStats(t, monitor, protocols.HTTP) {
						if key.DstPort == httpSrvPort || key.SrcPort == httpSrvPort {
							count := stat.Data[200].Count
							newKey := usmhttp.Key{
								Path:   usmhttp.Path{Content: key.Path.Content},
								Method: key.Method,
							}
							if _, ok := res[newKey]; !ok {
								res[newKey] = count
							} else {
								res[newKey] += count
							}
						}
					}

					if len(res) != len(tt.expectedEndpoints) {
						return false
					}

					for key, count := range res {
						value, ok := tt.expectedEndpoints[key]
						if !ok {
							return false
						}
						if count != value {
							return false
						}
					}

					return true
				}, time.Second*5, time.Millisecond*100, "%v != %v", res, tt.expectedEndpoints)
				if t.Failed() {
					for key := range tt.expectedEndpoints {
						if _, ok := res[key]; !ok {
							t.Logf("key: %v was not found in res", key.Path.Content.Get())
						}
					}
					ebpftest.DumpMapsTestHelper(t, monitor.DumpMaps, "http_in_flight")
					dumpTelemetry(t, monitor, s.isTLS)
				}
			})
		}
	}
}

//func (s *usmHTTPSuite) TestHTTPManyDifferentPaths() {
//	t := s.T()
//	cfg := s.getCfg()
//
//	// Start local server and register its cleanup.
//	t.Cleanup(usmhttp2.StartH2CServer(t, serverAddr, s.isTLS))
//
//	// Start the proxy server.
//	proxyProcess, cancel := proxy.NewExternalUnixTransparentProxyServer(t, unixPath, serverAddr, s.isTLS)
//	t.Cleanup(cancel)
//	require.NoError(t, proxy.WaitForConnectionReady(unixPath))
//
//	monitor := setupUSMTLSMonitor(t, cfg, useExistingConsumer)
//	if s.isTLS {
//		utils.WaitForProgramsToBeTraced(t, consts.USMModuleName, GoTLSAttacherName, proxyProcess.Process.Pid, utils.ManualTracingFallbackEnabled)
//	}
//
//	const (
//		repetitionsPerRequest = 2
//		// Should be bigger than the length of the http2_dynamic_table which is 1024
//		numberOfRequests         = 1500
//		expectedNumberOfRequests = numberOfRequests * repetitionsPerRequest
//	)
//	clients := getHTTP2UnixClientArray(1, unixPath)
//	for i := 0; i < numberOfRequests; i++ {
//		for j := 0; j < repetitionsPerRequest; j++ {
//			req, err := clients[0].Post(fmt.Sprintf("%s/test-%d", http2SrvAddr, i+1), "application/json", bytes.NewReader([]byte("test")))
//			require.NoError(t, err, "could not make request")
//			_ = req.Body.Close()
//		}
//	}
//
//	matches := PrintableInt(0)
//
//	seenRequests := map[string]int{}
//	assert.Eventuallyf(t, func() bool {
//		for key, stat := range getHTTPLikeProtocolStats(t, monitor, protocols.HTTP2) {
//			if (key.DstPort == httpSrvPort || key.SrcPort == httpSrvPort) && key.Method == usmhttp.MethodPost && strings.HasPrefix(key.Path.Content.Get(), "/test") {
//				if _, ok := seenRequests[key.Path.Content.Get()]; !ok {
//					seenRequests[key.Path.Content.Get()] = 0
//				}
//				seenRequests[key.Path.Content.Get()] += stat.Data[200].Count
//				matches.Add(stat.Data[200].Count)
//			}
//		}
//
//		// Due to a known issue in http2, we might consider an RST packet as a response to a request and therefore
//		// we might capture a request twice. This is why we are expecting to see 2*numberOfRequests instead of
//		return (expectedNumberOfRequests-1) <= matches.Load() && matches.Load() <= (expectedNumberOfRequests+1)
//	}, time.Second*10, time.Millisecond*100, "%v != %v", &matches, expectedNumberOfRequests)
//
//	for i := 0; i < numberOfRequests; i++ {
//		if v, ok := seenRequests[fmt.Sprintf("/test-%d", i+1)]; !ok || v != repetitionsPerRequest {
//			t.Logf("path: /test-%d should have %d occurrences but instead has %d", i+1, repetitionsPerRequest, v)
//		}
//	}
//}
//
//func (s *usmHTTPSuite) TestRawTraffic() {
//	t := s.T()
//	cfg := s.getCfg()
//
//	usmMonitor := setupUSMTLSMonitor(t, cfg, useExistingConsumer)
//
//	// Start local server and register its cleanup.
//	t.Cleanup(usmhttp2.StartH2CServer(t, serverAddr, s.isTLS))
//
//	// Start the proxy server.
//	proxyProcess, cancel := proxy.NewExternalUnixTransparentProxyServer(t, unixPath, serverAddr, s.isTLS)
//	t.Cleanup(cancel)
//	require.NoError(t, proxy.WaitForConnectionReady(unixPath))
//
//	if s.isTLS {
//		utils.WaitForProgramsToBeTraced(t, consts.USMModuleName, GoTLSAttacherName, proxyProcess.Process.Pid, utils.ManualTracingFallbackEnabled)
//	}
//	tests := []struct {
//		name              string
//		skip              bool
//		messageBuilder    func() [][]byte
//		expectedEndpoints map[usmhttp.Key]int
//	}{
//		{
//			name: "parse_frames tail call using 1 program",
//			// The objective of this test is to verify that we accurately perform the parsing of frames within
//			// a single program.
//			messageBuilder: func() [][]byte {
//				const settingsFramesCount = 238
//				framer := newFramer()
//				return [][]byte{
//					framer.
//						writeMultiMessage(t, settingsFramesCount, framer.writeSettings).
//						writeHeaders(t, 1, usmhttp2.HeadersFrameOptions{Headers: testHeaders()}).
//						writeData(t, 1, endStream, emptyBody).
//						bytes(),
//				}
//			},
//			expectedEndpoints: map[usmhttp.Key]int{
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
//					Method: usmhttp.MethodPost,
//				}: 1,
//			},
//		},
//		{
//			name: "validate frames_filter tail calls limit",
//			// The purpose of this test is to validate that when we do not surpass
//			// the tail call limit of HTTP2_MAX_TAIL_CALLS_FOR_FRAMES_FILTER.
//			messageBuilder: func() [][]byte {
//				const settingsFramesCount = 241
//				framer := newFramer()
//				return [][]byte{
//					framer.
//						writeMultiMessage(t, settingsFramesCount, framer.writeSettings).
//						writeHeaders(t, 1, usmhttp2.HeadersFrameOptions{Headers: testHeaders()}).
//						writeData(t, 1, endStream, emptyBody).
//						bytes(),
//				}
//			},
//			expectedEndpoints: nil,
//		},
//		{
//			name: "validate max interesting frames limit",
//			// The purpose of this test is to verify our ability to reach almost to the limit set by HTTP2_MAX_FRAMES_ITERATIONS, which
//			// determines the maximum number of "interesting frames" we can process.
//			// Not testing the max number of frame (240), as the server response can contain other frames, so we might miss EOS
//			// coming after a server side frame, and by that miss a request.
//			messageBuilder: func() [][]byte {
//				// Generates 119 (requests) * 2 (number of frames per request) = 238 frames
//				const iterations = 119
//				framer := newFramer()
//				for i := 0; i < iterations; i++ {
//					streamID := getStreamID(i)
//					framer.
//						writeHeaders(t, streamID, usmhttp2.HeadersFrameOptions{Headers: testHeaders()}).
//						writeData(t, streamID, endStream, emptyBody)
//				}
//				return [][]byte{framer.bytes()}
//			},
//			expectedEndpoints: map[usmhttp.Key]int{
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
//					Method: usmhttp.MethodPost,
//				}: 119,
//			},
//		},
//		{
//			name: "validate literal header field without indexing",
//			// The purpose of this test is to verify our ability the case:
//			// Literal Header Field without Indexing (0b0000xxxx: top four bits are 0000)
//			// https://httpwg.org/specs/rfc7541.html#rfc.section.C.2.2
//			messageBuilder: func() [][]byte {
//				const iterations = 5
//				framer := newFramer()
//				for i := 0; i < iterations; i++ {
//					streamID := getStreamID(i)
//					framer.
//						writeHeaders(t, streamID, usmhttp2.HeadersFrameOptions{
//							Headers:                generateTestHeaderFields(headersGenerationOptions{pathTypeValue: pathLiteralWithoutIndexing}),
//							DynamicTableUpdateSize: defaultDynamicTableSize}).
//						writeData(t, streamID, endStream, emptyBody)
//				}
//				return [][]byte{framer.bytes()}
//			},
//			expectedEndpoints: map[usmhttp.Key]int{
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString("/" + strings.Repeat("a", defaultDynamicTableSize))},
//					Method: usmhttp.MethodPost,
//				}: 5,
//			},
//		},
//		{
//			name: "validate literal header field never indexed",
//			// The purpose of this test is to verify our ability the case:
//			// Literal Header Field never Indexed (0b0001xxxx: top four bits are 0001)
//			// https://httpwg.org/specs/rfc7541.html#rfc.section.6.2.3
//			messageBuilder: func() [][]byte {
//				const iterations = 5
//				framer := newFramer()
//				for i := 0; i < iterations; i++ {
//					streamID := getStreamID(i)
//					framer.
//						writeHeaders(t, streamID, usmhttp2.HeadersFrameOptions{
//							Headers: generateTestHeaderFields(headersGenerationOptions{pathTypeValue: pathLiteralNeverIndexed})}).
//						writeData(t, streamID, endStream, emptyBody)
//				}
//				return [][]byte{framer.bytes()}
//			},
//			expectedEndpoints: map[usmhttp.Key]int{
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
//					Method: usmhttp.MethodPost,
//				}: 5,
//			},
//		},
//		{
//			name: "validate path with index 4",
//			// The purpose of this test is to verify our ability to identify paths with index 4.
//			messageBuilder: func() [][]byte {
//				const iterations = 5
//				// pathHeaderField is the hex representation of the path /aaa with index 4.
//				pathHeaderField := []byte{0x44, 0x83, 0x60, 0x63, 0x1f}
//				headerFields := removeHeaderFieldByKey(testHeaders(), ":path")
//				headersFrame, err := usmhttp2.NewHeadersFrameMessage(usmhttp2.HeadersFrameOptions{
//					Headers: headerFields,
//				})
//				require.NoError(t, err, "could not create headers frame")
//
//				// we are adding the path header field with index 4, we need to do it on the byte slice and not on the headerFields
//				// due to the fact that when we create a header field it would be with index 5.
//				headersFrame = append(pathHeaderField, headersFrame...)
//				framer := newFramer()
//				for i := 0; i < iterations; i++ {
//					streamID := getStreamID(i)
//					framer.
//						writeRawHeaders(t, streamID, endHeaders, headersFrame).
//						writeData(t, streamID, endStream, emptyBody)
//				}
//				return [][]byte{framer.bytes()}
//			},
//			expectedEndpoints: map[usmhttp.Key]int{
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
//					Method: usmhttp.MethodPost,
//				}: 5,
//			},
//		},
//		{
//			name: "validate PING and WINDOWS_UPDATE frames between HEADERS and DATA",
//			// The purpose of this test is to verify our ability to process PING and WINDOWS_UPDATE frames between HEADERS and DATA.
//			messageBuilder: func() [][]byte {
//				const iterations = 5
//				framer := newFramer()
//				for i := 0; i < iterations; i++ {
//					streamID := getStreamID(i)
//					framer.
//						writeHeaders(t, streamID, usmhttp2.HeadersFrameOptions{
//							Headers: testHeaders(),
//						}).
//						writePing(t).
//						writeWindowUpdate(t, streamID, 1).
//						writeData(t, streamID, endStream, emptyBody)
//				}
//
//				return [][]byte{framer.bytes()}
//			},
//			expectedEndpoints: map[usmhttp.Key]int{
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
//					Method: usmhttp.MethodPost,
//				}: 5,
//			},
//		},
//		{
//			name: "validate RST_STREAM cancel err code",
//			// The purpose of this test is to validate that when a cancel error code is sent, we will not count the request.
//			// We are sending 10 requests, and 5 of them will contain RST_STREAM with a cancel error code.Therefore, we expect to
//			// capture five valid requests.
//			messageBuilder: func() [][]byte {
//				const iterations = 10
//				rstFramesCount := 5
//				framer := newFramer()
//				for i := 0; i < iterations; i++ {
//					streamID := getStreamID(i)
//					framer.
//						writeHeaders(t, streamID, usmhttp2.HeadersFrameOptions{Headers: testHeaders()}).
//						writeData(t, streamID, endStream, emptyBody)
//					if rstFramesCount > 0 {
//						framer.writeRSTStream(t, streamID, http2.ErrCodeCancel)
//						rstFramesCount--
//					}
//				}
//				return [][]byte{framer.bytes()}
//			},
//			expectedEndpoints: map[usmhttp.Key]int{
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
//					Method: usmhttp.MethodPost,
//				}: 5,
//			},
//		},
//		{
//			name: "validate RST_STREAM before server status ok",
//			// The purpose of this test is to validate that when we see RST before DATA frame with status ok,
//			// we will not count the requests.
//			messageBuilder: func() [][]byte {
//				const iterations = 10
//				framer := newFramer()
//				for i := 0; i < iterations; i++ {
//					streamID := getStreamID(i)
//					framer.
//						writeHeaders(t, streamID, usmhttp2.HeadersFrameOptions{Headers: testHeaders()}).
//						writeData(t, streamID, endStream, emptyBody).
//						writeRSTStream(t, streamID, http2.ErrCodeNo)
//				}
//				return [][]byte{framer.bytes()}
//			},
//			expectedEndpoints: nil,
//		},
//		{
//			name: "validate various status codes",
//			// The purpose of this test is to verify that we support status codes that both do and
//			// do not appear in the static table.
//			messageBuilder: func() [][]byte {
//				statusCodes := []int{200, 201, 204, 206, 300, 304, 400, 401, 404, 500, 504}
//				const iterationsPerStatusCode = 3
//				messages := make([][]byte, 0, len(statusCodes)*iterationsPerStatusCode)
//				for statusCodeIteration, statusCode := range statusCodes {
//					for i := 0; i < iterationsPerStatusCode; i++ {
//						streamID := getStreamID(statusCodeIteration*iterationsPerStatusCode + i)
//						messages = append(messages, newFramer().
//							writeHeaders(t, streamID, usmhttp2.HeadersFrameOptions{Headers: headersWithGivenEndpoint(fmt.Sprintf("/status/%d", statusCode))}).
//							writeData(t, streamID, endStream, emptyBody).
//							bytes())
//					}
//				}
//				return messages
//			},
//			expectedEndpoints: map[usmhttp.Key]int{
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString("/status/200")},
//					Method: usmhttp.MethodPost,
//				}: 3,
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString("/status/201")},
//					Method: usmhttp.MethodPost,
//				}: 3,
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString("/status/204")},
//					Method: usmhttp.MethodPost,
//				}: 3,
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString("/status/206")},
//					Method: usmhttp.MethodPost,
//				}: 3,
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString("/status/300")},
//					Method: usmhttp.MethodPost,
//				}: 3,
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString("/status/304")},
//					Method: usmhttp.MethodPost,
//				}: 3,
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString("/status/400")},
//					Method: usmhttp.MethodPost,
//				}: 3,
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString("/status/401")},
//					Method: usmhttp.MethodPost,
//				}: 3,
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString("/status/404")},
//					Method: usmhttp.MethodPost,
//				}: 3,
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString("/status/500")},
//					Method: usmhttp.MethodPost,
//				}: 3,
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString("/status/504")},
//					Method: usmhttp.MethodPost,
//				}: 3,
//			},
//		},
//		{
//			name: "validate http methods",
//			// The purpose of this test is to validate that we are able to capture all http methods.
//			messageBuilder: func() [][]byte {
//				httpMethods := []string{http.MethodGet, http.MethodPost, http.MethodHead, http.MethodDelete,
//					http.MethodPut, http.MethodPatch, http.MethodOptions, http.MethodTrace, http.MethodConnect}
//				// Duplicating the list to have each method appearing twice
//				httpMethods = append(httpMethods, httpMethods...)
//				// Currently, the methods TRACE and CONNECT are not supported by the http.Method package.
//				// Therefore, we mark those requests as incomplete and not expected to be captured.
//				framer := newFramer()
//				var buf bytes.Buffer
//				encoder := hpack.NewEncoder(&buf)
//				for i, method := range httpMethods {
//					streamID := getStreamID(i)
//					framer.
//						writeHeadersWithEncoder(t, streamID, usmhttp2.HeadersFrameOptions{Headers: generateTestHeaderFields(headersGenerationOptions{overrideMethod: method})}, encoder, &buf).
//						writeData(t, streamID, endStream, emptyBody)
//				}
//				return [][]byte{framer.bytes()}
//			},
//			expectedEndpoints: map[usmhttp.Key]int{
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
//					Method: usmhttp.MethodGet,
//				}: 2,
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
//					Method: usmhttp.MethodPost,
//				}: 2,
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
//					Method: usmhttp.MethodHead,
//				}: 2,
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
//					Method: usmhttp.MethodDelete,
//				}: 2,
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
//					Method: usmhttp.MethodPut,
//				}: 2,
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
//					Method: usmhttp.MethodPatch,
//				}: 2,
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
//					Method: usmhttp.MethodOptions,
//				}: 2,
//			},
//		},
//		{
//			name: "validate http methods huffman encoded",
//			// The purpose of this test is to validate that we are able to capture all http methods.
//			messageBuilder: func() [][]byte {
//				httpMethods = []string{http.MethodGet, http.MethodPost, http.MethodHead, http.MethodDelete,
//					http.MethodPut, http.MethodPatch, http.MethodOptions, http.MethodTrace, http.MethodConnect}
//				// Currently, the methods TRACE and CONNECT are not supported by the http.Method package.
//				// Therefore, we mark those requests as incomplete and not expected to be captured.
//				framer := newFramer()
//				headerFields := removeHeaderFieldByKey(testHeaders(), ":method")
//				headersFrame, err := usmhttp2.NewHeadersFrameMessage(usmhttp2.HeadersFrameOptions{
//					Headers: headerFields,
//				})
//				require.NoError(t, err, "could not create headers frame")
//				for i, method := range httpMethods {
//					huffMethod := hpack.AppendHuffmanString([]byte{}, method)
//					// we are adding 128 to the length of the huffman encoded method,
//					// as it is the representation of the huffman encoding (MSB ofo the octet is on).
//					rawMethod := append([]byte{0x43}, byte(0x80|len(huffMethod)))
//					rawMethod = append(rawMethod, huffMethod...)
//					headersFrameWithRawMethod := append(rawMethod, headersFrame...)
//					streamID := getStreamID(i)
//					framer.
//						writeRawHeaders(t, streamID, endHeaders, headersFrameWithRawMethod).
//						writeData(t, streamID, endStream, emptyBody).bytes()
//				}
//				return [][]byte{framer.bytes()}
//			},
//			expectedEndpoints: map[usmhttp.Key]int{
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
//					Method: usmhttp.MethodGet,
//				}: 1,
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
//					Method: usmhttp.MethodPost,
//				}: 1,
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
//					Method: usmhttp.MethodHead,
//				}: 1,
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
//					Method: usmhttp.MethodDelete,
//				}: 1,
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
//					Method: usmhttp.MethodPut,
//				}: 1,
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
//					Method: usmhttp.MethodPatch,
//				}: 1,
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
//					Method: usmhttp.MethodOptions,
//				}: 1,
//			},
//		},
//		{
//			name: "validate http methods POST and GET methods as literal",
//			// The purpose of this test is to validate that when the methods POST and GET are sent as literal fields
//			// and not indexed, we are still able to capture and process them correctly.
//			messageBuilder: func() [][]byte {
//				getMethodRaw := append([]byte{0x43, 0x03}, []byte("GET")...)
//				postMethodRaw := append([]byte{0x43, 0x04}, []byte("POST")...)
//
//				headerFields := removeHeaderFieldByKey(testHeaders(), ":method")
//				headersFrame, err := usmhttp2.NewHeadersFrameMessage(usmhttp2.HeadersFrameOptions{
//					Headers: headerFields,
//				})
//				require.NoError(t, err, "could not create headers frame")
//				headersFrameWithGET := append(getMethodRaw, headersFrame...)
//				headersFrameWithPOST := append(postMethodRaw, headersFrame...)
//
//				framer := newFramer()
//				framer.
//					writeRawHeaders(t, 1, endHeaders, headersFrameWithGET).
//					writeData(t, 1, endStream, emptyBody).
//					writeRawHeaders(t, 3, endHeaders, headersFrameWithPOST).
//					writeData(t, 3, endStream, emptyBody)
//				return [][]byte{framer.bytes()}
//			},
//			expectedEndpoints: map[usmhttp.Key]int{
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
//					Method: usmhttp.MethodGet,
//				}: 1,
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
//					Method: usmhttp.MethodPost,
//				}: 1,
//			},
//		},
//		{
//			name: "validate max path length",
//			// The purpose of this test is to validate that we are not able to process a path longer than HTTP2_MAX_PATH_LEN.
//			messageBuilder: func() [][]byte {
//				framer := newFramer()
//				return [][]byte{
//					framer.
//						writeHeaders(t, 1, usmhttp2.HeadersFrameOptions{Headers: generateTestHeaderFields(headersGenerationOptions{pathTypeValue: pathTooLarge})}).
//						writeData(t, 1, endStream, emptyBody).
//						bytes(),
//				}
//			},
//			expectedEndpoints: nil,
//		},
//		{
//			name: "validate path sent by value (:path)",
//			// The purpose of this test is to verify our ability to identify paths which were sent with a key that
//			// sent by value (:path).
//			messageBuilder: func() [][]byte {
//				headerFields := removeHeaderFieldByKey(testHeaders(), ":path")
//				headersFrame, err := usmhttp2.NewHeadersFrameMessage(usmhttp2.HeadersFrameOptions{Headers: headerFields})
//				require.NoError(t, err, "could not create headers frame")
//				// pathHeaderField is created with a key that sent by value (:path) and
//				// the value (of the path) is /aaa.
//				pathHeaderField := []byte{0x40, 0x84, 0xb9, 0x58, 0xd3, 0x3f, 0x83, 0x60, 0x63, 0x1f}
//				headersFrame = append(pathHeaderField, headersFrame...)
//				framer := newFramer()
//				return [][]byte{
//					framer.
//						writeRawHeaders(t, 1, endHeaders, headersFrame).
//						writeData(t, 1, endStream, emptyBody).
//						bytes(),
//				}
//			},
//			expectedEndpoints: nil,
//		},
//		{
//			name: "Interesting frame header sent separately from frame payload",
//			// Testing the scenario in which the frame header (of an interesting type) is sent separately from the frame payload.
//			messageBuilder: func() [][]byte {
//				headersFrame := newFramer().writeHeaders(t, 1, usmhttp2.HeadersFrameOptions{Headers: testHeaders()}).bytes()
//				dataFrame := newFramer().writeData(t, 1, endStream, emptyBody).bytes()
//				headersFrameHeader := headersFrame[:9]
//				secondMessage := append(headersFrame[9:], dataFrame...)
//				return [][]byte{
//					headersFrameHeader,
//					secondMessage,
//				}
//			},
//			expectedEndpoints: map[usmhttp.Key]int{
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
//					Method: usmhttp.MethodPost,
//				}: 1,
//			},
//		},
//		{
//			name: "Not interesting frame header sent separately from frame payload",
//			// Testing the scenario in which the frame header (of a not interesting type) is sent separately from the frame payload.
//			messageBuilder: func() [][]byte {
//				pingFrame := newFramer().writePing(t).bytes()
//				fullFrame := newFramer().writeHeaders(t, 1, usmhttp2.HeadersFrameOptions{Headers: testHeaders()}).
//					writeData(t, 1, endStream, emptyBody).bytes()
//				pingFrameHeader := pingFrame[:9]
//				secondMessage := append(pingFrame[9:], fullFrame...)
//				return [][]byte{
//					pingFrameHeader,
//					secondMessage,
//				}
//			},
//			expectedEndpoints: map[usmhttp.Key]int{
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
//					Method: usmhttp.MethodPost,
//				}: 1,
//			},
//		},
//		{
//			name: "validate dynamic table update with indexed header field",
//			// The purpose of this test is to verify our ability to support dynamic table update
//			// while using a path with indexed header field.
//			messageBuilder: func() [][]byte {
//				const iterations = 5
//				framer := newFramer()
//				for i := 0; i < iterations; i++ {
//					streamID := getStreamID(i)
//					framer.
//						writeHeaders(t, streamID, usmhttp2.HeadersFrameOptions{
//							Headers:                headersWithGivenEndpoint("/"),
//							DynamicTableUpdateSize: defaultDynamicTableSize}).
//						writeData(t, streamID, endStream, emptyBody)
//				}
//				return [][]byte{framer.bytes()}
//			},
//			expectedEndpoints: map[usmhttp.Key]int{
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString("/")},
//					Method: usmhttp.MethodPost,
//				}: 5,
//			},
//		},
//		{
//			name: "validate dynamic table update with high value and indexed header field",
//			// The purpose of this test is to verify our ability to support dynamic table update
//			// with size exceeds one octet, to validate the iteration for parsing the dynamic table update.
//			messageBuilder: func() [][]byte {
//				const iterations = 5
//				const dynamicTableSize = 4000
//				framer := newFramer()
//				for i := 0; i < iterations; i++ {
//					streamID := getStreamID(i)
//					framer.
//						writeHeaders(t, streamID, usmhttp2.HeadersFrameOptions{
//							Headers:                headersWithGivenEndpoint("/"),
//							DynamicTableUpdateSize: dynamicTableSize,
//						}).
//						writeData(t, streamID, endStream, emptyBody)
//				}
//				return [][]byte{framer.bytes()}
//			},
//			expectedEndpoints: map[usmhttp.Key]int{
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString("/")},
//					Method: usmhttp.MethodPost,
//				}: 5,
//			},
//		},
//		{
//			name: "Data frame header sent separately from frame payload",
//			// Testing the scenario in which the data frame header is sent separately from the frame payload.
//			messageBuilder: func() [][]byte {
//				payload := []byte("test")
//				headersFrame := newFramer().writeHeaders(t, 1, usmhttp2.HeadersFrameOptions{Headers: testHeaders()}).bytes()
//				dataFrame := newFramer().writeData(t, 1, endStream, payload).bytes()
//				// We are creating a header frame with a content-length header field that contains the payload size.
//				secondMessageHeadersFrame := newFramer().writeHeaders(t, 3, usmhttp2.HeadersFrameOptions{
//					Headers: generateTestHeaderFields(headersGenerationOptions{
//						overrideContentLength: len(payload)})}).writeData(t, 3, endStream, payload).bytes()
//
//				headersFrameHeader := append(headersFrame, dataFrame[:9]...)
//				secondMessage := append(dataFrame[9:], secondMessageHeadersFrame...)
//				return [][]byte{
//					headersFrameHeader,
//					secondMessage,
//				}
//			},
//			expectedEndpoints: map[usmhttp.Key]int{
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
//					Method: usmhttp.MethodPost,
//				}: 2,
//			},
//		},
//		{
//			name: "Data frame header sent separately from frame payload with PING between them",
//			// Testing the scenario in which the data frame header is sent separately from the frame payload.,
//			// including a PING frame in the second message between the data frame.
//			messageBuilder: func() [][]byte {
//				payload := []byte("test")
//				// We are creating a header frame with a content-length header field that contains the payload size.
//				headersFrame := newFramer().writeHeaders(t, 1, usmhttp2.HeadersFrameOptions{
//					Headers: generateTestHeaderFields(headersGenerationOptions{
//						overrideContentLength: len(payload)})}).bytes()
//				dataFrame := newFramer().writeData(t, 1, endStream, payload).bytes()
//				pingFrame := newFramer().writePing(t).bytes()
//
//				headersFrameHeader := append(headersFrame, dataFrame[:9]...)
//				secondMessage := append(pingFrame, dataFrame[9:]...)
//				return [][]byte{
//					headersFrameHeader,
//					secondMessage,
//				}
//			},
//			expectedEndpoints: map[usmhttp.Key]int{
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
//					Method: usmhttp.MethodPost,
//				}: 1,
//			},
//		},
//		{
//			name: "Payload data frame header sent separately",
//			// Testing the scenario in which the data frame header is sent separately from the frame payload.
//			messageBuilder: func() [][]byte {
//				payload := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9}
//				// We are creating a header frame with a content-length header field that contains the payload size.
//				headersFrame := newFramer().writeHeaders(t, 1, usmhttp2.HeadersFrameOptions{
//					Headers: generateTestHeaderFields(headersGenerationOptions{
//						overrideContentLength: len(payload)})}).bytes()
//				dataFrame := newFramer().writeData(t, 1, endStream, payload).bytes()
//				secondMessageHeadersFrame := newFramer().writeHeaders(t, 3, usmhttp2.HeadersFrameOptions{Headers: testHeaders()}).bytes()
//				secondMessageDataFrame := newFramer().writeData(t, 3, endStream, emptyBody).bytes()
//
//				// We are cutting in the middle of the payload
//				firstMessage := append(headersFrame, dataFrame[:9+3]...)
//				secondMessage := append(dataFrame[9+3:], secondMessageHeadersFrame...)
//				secondMessage = append(secondMessage, secondMessageDataFrame...)
//				return [][]byte{
//					firstMessage,
//					secondMessage,
//				}
//			},
//			expectedEndpoints: map[usmhttp.Key]int{
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
//					Method: usmhttp.MethodPost,
//				}: 2,
//			},
//		},
//		{
//			name: "validate CONTINUATION frame support",
//			// Testing the scenario in which part of the data frame header is sent using CONTINUATION frame.
//			// Currently, we do not support CONTINUATION frames, therefore, we expect to capture only the first
//			// part of the message.
//			messageBuilder: func() [][]byte {
//				const headersFrameEndHeaders = false
//				fullHeaders := generateTestHeaderFields(headersGenerationOptions{})
//				prefixHeadersFrame, err := usmhttp2.NewHeadersFrameMessage(usmhttp2.HeadersFrameOptions{
//					Headers: fullHeaders[:2],
//				})
//				require.NoError(t, err, "could not create prefix headers frame")
//
//				suffixHeadersFrame, err := usmhttp2.NewHeadersFrameMessage(usmhttp2.HeadersFrameOptions{
//					Headers: fullHeaders[2:],
//				})
//				require.NoError(t, err, "could not create suffix headers frame")
//
//				return [][]byte{
//					newFramer().writeRawHeaders(t, 1, headersFrameEndHeaders, prefixHeadersFrame).
//						writeRawContinuation(t, 1, endHeaders, suffixHeadersFrame).
//						writeData(t, 1, endStream, emptyBody).bytes(),
//				}
//			},
//			expectedEndpoints: nil,
//		},
//		{
//			name: "validate message split into 2 tcp segments",
//			// The purpose of this test is to validate that we cannot handle reassembled tcp segments.
//			messageBuilder: func() [][]byte {
//				a := newFramer().
//					writeHeaders(t, 1, usmhttp2.HeadersFrameOptions{Headers: testHeaders()}).
//					writeData(t, 1, true, emptyBody).bytes()
//				return [][]byte{
//					a[:10],
//					a[10:],
//				}
//
//			},
//			expectedEndpoints: nil,
//		},
//		{
//			name: "remainder + header remainder",
//			// Testing the scenario where we have both a remainder (a frame's payload split over 2 packets) and in the
//			// second packet, we have the remainder and a partial frame header of a new request. We're testing that we
//			// can capture the 2 requests in this scenario.
//			messageBuilder: func() [][]byte {
//				data := []byte("testcontent")
//				request1 := newFramer().
//					writeHeaders(t, 1, usmhttp2.HeadersFrameOptions{Headers: generateTestHeaderFields(headersGenerationOptions{overrideContentLength: len(data)})}).
//					writeData(t, 1, true, data).bytes()
//				request2 := newFramer().
//					writeHeaders(t, 3, usmhttp2.HeadersFrameOptions{Headers: headersWithGivenEndpoint("/bbb")}).
//					writeData(t, 3, true, emptyBody).bytes()
//				firstPacket := request1[:len(request1)-6]
//				secondPacket := append(request1[len(request1)-6:], request2[:5]...)
//				return [][]byte{
//					firstPacket,
//					secondPacket,
//					request2[5:],
//				}
//			},
//			expectedEndpoints: map[usmhttp.Key]int{
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
//					Method: usmhttp.MethodPost,
//				}: 1,
//				{
//					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString("/bbb")},
//					Method: usmhttp.MethodPost,
//				}: 1,
//			},
//		},
//	}
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			if tt.skip {
//				t.Skip("skipping test")
//			}
//
//			t.Cleanup(func() { cleanProtocolMaps(t, "http2", usmMonitor.ebpfProgram.Manager.Manager) })
//			c := dialHTTP2Server(t)
//
//			// Composing a message with the number of setting frames we want to send.
//			require.NoError(t, writeInput(c, 500*time.Millisecond, tt.messageBuilder()...))
//
//			res := make(map[usmhttp.Key]int)
//			assert.Eventually(t, func() bool {
//				return validateStats(t, usmMonitor, res, tt.expectedEndpoints, s.isTLS)
//			}, time.Second*5, time.Millisecond*100, "%v != %v", res, tt.expectedEndpoints)
//			if t.Failed() {
//				for key := range tt.expectedEndpoints {
//					if _, ok := res[key]; !ok {
//						t.Logf("key: %v was not found in res", key.Path.Content.Get())
//					}
//				}
//				ebpftest.DumpMapsTestHelper(t, usmMonitor.DumpMaps, usmhttp2.InFlightMap, "http2_dynamic_table")
//				dumpTelemetry(t, usmMonitor, s.isTLS)
//			}
//		})
//	}
//}
//
//func TestHTTPInFlightMapCleaner(t *testing.T) {
//	skipIfKernelNotSupported(t)
//	cfg := utils.NewUSMEmptyConfig()
//	cfg.EnableHTTP2Monitoring = true
//	cfg.HTTP2DynamicTableMapCleanerInterval = 5 * time.Second
//	cfg.HTTPIdleConnectionTTL = time.Second
//	monitor := setupUSMTLSMonitor(t, cfg, useExistingConsumer)
//	ebpfNow, err := ddebpf.NowNanoseconds()
//	require.NoError(t, err)
//	http2InFLightMap, _, err := monitor.ebpfProgram.GetMap(usmhttp2.InFlightMap)
//	require.NoError(t, err)
//	key := usmhttp2.HTTP2StreamKey{
//		Id: 1,
//	}
//	val := usmhttp2.HTTP2Stream{
//		Request_started: uint64(ebpfNow - (time.Second * 3).Nanoseconds()),
//	}
//	require.NoError(t, http2InFLightMap.Update(unsafe.Pointer(&key), unsafe.Pointer(&val), ebpf.UpdateAny))
//
//	var newVal usmhttp2.HTTP2Stream
//	require.NoError(t, http2InFLightMap.Lookup(unsafe.Pointer(&key), unsafe.Pointer(&newVal)))
//	require.Equal(t, val, newVal)
//
//	require.Eventually(t, func() bool {
//		err := http2InFLightMap.Lookup(unsafe.Pointer(&key), unsafe.Pointer(&newVal))
//		return errors.Is(err, ebpf.ErrKeyNotExist)
//	}, 3*cfg.HTTP2DynamicTableMapCleanerInterval, time.Millisecond*100)
//}

//// validateStats validates that the stats we get from the monitor are as expected.
//func validateStats(t *testing.T, usmMonitor *Monitor, res, expectedEndpoints map[usmhttp.Key]int, isTLS bool) bool {
//	for key, stat := range getHTTPLikeProtocolStats(t, usmMonitor, protocols.HTTP2) {
//		if key.DstPort == httpSrvPort || key.SrcPort == httpSrvPort {
//			statusCode := testutil.StatusFromPath(key.Path.Content.Get())
//			// statusCode 0 represents an error returned from the function, which means the URL is not in the special
//			// form which contains the expected status code (form - `/status/{statusCode}`). So by default we use
//			// 200 as the status code.
//			if statusCode == 0 {
//				statusCode = 200
//			}
//			hasTag := stat.Data[statusCode].StaticTags == ebpftls.ConnTagGo
//			if hasTag != isTLS {
//				continue
//			}
//			count := stat.Data[statusCode].Count
//			newKey := usmhttp.Key{
//				Path:   usmhttp.Path{Content: key.Path.Content},
//				Method: key.Method,
//			}
//			if _, ok := res[newKey]; !ok {
//				res[newKey] = count
//			} else {
//				res[newKey] += count
//			}
//		}
//	}
//
//	if len(res) != len(expectedEndpoints) {
//		return false
//	}
//
//	for key, endpointCount := range res {
//		_, ok := expectedEndpoints[key]
//		if !ok {
//			return false
//		}
//		if endpointCount != expectedEndpoints[key] {
//			return false
//		}
//	}
//	return true
//}

// getHTTP2UnixClientArray creates an array of http2 clients over a unix socket.
func getHTTPUnixClientArray(size int, unixPath string) []*http.Client {
	res := make([]*http.Client, size)
	for i := 0; i < size; i++ {
		res[i] = &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return net.Dial("unix", unixPath)
				},
				DialTLSContext: func(context.Context, string, string) (net.Conn, error) {
					return net.Dial("unix", unixPath)
				},
			},
		}
	}

	return res
}

//// dialHTTPServer dials the http2 server and performs the initial handshake
//func dialHTTPServer(t *testing.T) net.Conn {
//	c, err := net.Dial("unix", unixPath)
//	require.NoError(t, err, "failed to dial")
//	t.Cleanup(func() { _ = c.Close() })
//
//	// Writing a magic and the settings in the same packet to socket.
//	require.NoError(t, writeInput(c, time.Millisecond*200, usmhttp2.ComposeMessage([]byte(http2.ClientPreface), newFramer().writeSettings(t).bytes())))
//	return c
//}
