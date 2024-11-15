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
	"net"
	"net/http"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"golang.org/x/net/http2/hpack"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/ebpf/prebuilt"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	usmhttp "github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	usmhttp2 "github.com/DataDog/datadog-agent/pkg/network/protocols/http2"
	gotlsutils "github.com/DataDog/datadog-agent/pkg/network/protocols/tls/gotls/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/testutil/proxy"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

type pathType uint8

const (
	pathDefault pathType = iota
	pathLiteralNeverIndexed
	pathLiteralWithoutIndexing
	pathTooLarge
	pathOverride
)

const (
	srvPort              = 8082
	unixPath             = "/tmp/transparent.sock"
	http2DefaultTestPath = "/aaa"
	defaultMethod        = http.MethodPost
	defaultContentLength = 4
)

var (
	authority    = net.JoinHostPort("127.0.0.1", strconv.Itoa(srvPort))
	http2SrvAddr = "http://" + authority
)

type usmHTTP2Suite struct {
	suite.Suite
	isTLS bool
}

func (s *usmHTTP2Suite) getCfg() *config.Config {
	cfg := config.New()
	cfg.EnableHTTP2Monitoring = true
	cfg.EnableGoTLSSupport = s.isTLS
	cfg.GoTLSExcludeSelf = s.isTLS
	return cfg
}

func skipIfKernelNotSupported(t *testing.T) {
	currKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	if currKernelVersion < usmhttp2.MinimumKernelVersion {
		t.Skipf("HTTP2 monitoring can not run on kernel before %v", usmhttp2.MinimumKernelVersion)
	}
}

func TestHTTP2Scenarios(t *testing.T) {
	skipIfKernelNotSupported(t)
	modes := []ebpftest.BuildMode{ebpftest.RuntimeCompiled, ebpftest.CORE}
	if !prebuilt.IsDeprecated() {
		modes = append(modes, ebpftest.Prebuilt)
	}

	ebpftest.TestBuildModes(t, modes, "", func(t *testing.T) {
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
				suite.Run(t, &usmHTTP2Suite{isTLS: tc.isTLS})
			})
		}
	})
}

func (s *usmHTTP2Suite) TestLoadHTTP2Binary() {
	t := s.T()

	cfg := s.getCfg()

	for _, debug := range map[string]bool{"enabled": true, "disabled": false} {
		t.Run(fmt.Sprintf("debug %v", debug), func(t *testing.T) {
			cfg.BPFDebug = debug
			setupUSMTLSMonitor(t, cfg)
		})
	}
}

func (s *usmHTTP2Suite) TestHTTP2DynamicTableCleanup() {
	t := s.T()
	cfg := s.getCfg()
	cfg.HTTP2DynamicTableMapCleanerInterval = 5 * time.Second

	// Start local server and register its cleanup.
	t.Cleanup(startH2CServer(t, authority, s.isTLS))

	// Start the proxy server.
	proxyProcess, cancel := proxy.NewExternalUnixTransparentProxyServer(t, unixPath, authority, s.isTLS)
	t.Cleanup(cancel)
	require.NoError(t, proxy.WaitForConnectionReady(unixPath))

	monitor := setupUSMTLSMonitor(t, cfg)
	if s.isTLS {
		utils.WaitForProgramsToBeTraced(t, GoTLSAttacherName, proxyProcess.Process.Pid, utils.ManualTracingFallbackEnabled)
	}

	clients := getHTTP2UnixClientArray(2, unixPath)
	for i := 0; i < usmhttp2.HTTP2TerminatedBatchSize; i++ {
		req, err := clients[i%2].Post(fmt.Sprintf("%s/test-%d", http2SrvAddr, i+1), "application/json", bytes.NewReader([]byte("test")))
		require.NoError(t, err, "could not make request")
		_ = req.Body.Close()
	}

	matches := PrintableInt(0)

	require.Eventuallyf(t, func() bool {
		for key, stat := range getHTTPLikeProtocolStats(monitor, protocols.HTTP2) {
			if (key.DstPort == srvPort || key.SrcPort == srvPort) && key.Method == usmhttp.MethodPost && strings.HasPrefix(key.Path.Content.Get(), "/test") {
				matches.Add(stat.Data[200].Count)
			}
		}

		return matches.Load() == usmhttp2.HTTP2TerminatedBatchSize
	}, time.Second*10, time.Millisecond*100, "%v != %v", &matches, usmhttp2.HTTP2TerminatedBatchSize)

	for _, client := range clients {
		client.CloseIdleConnections()
	}

	dynamicTableMap, _, err := monitor.ebpfProgram.GetMap("http2_dynamic_table")
	require.NoError(t, err)
	iterator := dynamicTableMap.Iterate()
	key := make([]byte, dynamicTableMap.KeySize())
	value := make([]byte, dynamicTableMap.ValueSize())
	count := 0
	for iterator.Next(&key, &value) {
		count++
	}
	require.GreaterOrEqual(t, count, 0)

	require.Eventually(t, func() bool {
		iterator = dynamicTableMap.Iterate()
		count = 0
		for iterator.Next(&key, &value) {
			count++
		}

		return count == 0
	}, cfg.HTTP2DynamicTableMapCleanerInterval*4, time.Millisecond*100)
}

func (s *usmHTTP2Suite) TestSimpleHTTP2() {
	t := s.T()
	cfg := s.getCfg()

	// Start local server and register its cleanup.
	t.Cleanup(startH2CServer(t, authority, s.isTLS))

	// Start the proxy server.
	proxyProcess, cancel := proxy.NewExternalUnixTransparentProxyServer(t, unixPath, authority, s.isTLS)
	t.Cleanup(cancel)
	require.NoError(t, proxy.WaitForConnectionReady(unixPath))

	monitor := setupUSMTLSMonitor(t, cfg)
	if s.isTLS {
		utils.WaitForProgramsToBeTraced(t, GoTLSAttacherName, proxyProcess.Process.Pid, utils.ManualTracingFallbackEnabled)
	}

	tests := []struct {
		name              string
		runClients        func(t *testing.T, clientsCount int)
		expectedEndpoints map[usmhttp.Key]captureRange
	}{
		{
			name: " / path",
			runClients: func(t *testing.T, clientsCount int) {
				clients := getHTTP2UnixClientArray(clientsCount, unixPath)

				for i := 0; i < 1000; i++ {
					client := clients[getClientsIndex(i, clientsCount)]
					req, err := client.Post(http2SrvAddr+"/", "application/json", bytes.NewReader([]byte("test")))
					require.NoError(t, err, "could not make request")
					_ = req.Body.Close()
				}
			},
			expectedEndpoints: map[usmhttp.Key]captureRange{
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString("/")},
					Method: usmhttp.MethodPost,
				}: {
					lower: 999,
					upper: 1001,
				},
			},
		},
		{
			name: " /index.html path",
			runClients: func(t *testing.T, clientsCount int) {
				clients := getHTTP2UnixClientArray(clientsCount, unixPath)

				for i := 0; i < 1000; i++ {
					client := clients[getClientsIndex(i, clientsCount)]
					req, err := client.Post(http2SrvAddr+"/index.html", "application/json", bytes.NewReader([]byte("test")))
					require.NoError(t, err, "could not make request")
					_ = req.Body.Close()
				}
			},
			expectedEndpoints: map[usmhttp.Key]captureRange{
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString("/index.html")},
					Method: usmhttp.MethodPost,
				}: {
					lower: 999,
					upper: 1001,
				},
			},
		},
		{
			name: "path with repeated string",
			runClients: func(t *testing.T, clientsCount int) {
				clients := getHTTP2UnixClientArray(clientsCount, unixPath)

				for i := 1; i < 100; i++ {
					path := strings.Repeat("a", i)
					client := clients[getClientsIndex(i, clientsCount)]
					req, err := client.Post(http2SrvAddr+"/"+path, "application/json", bytes.NewReader([]byte("test")))
					require.NoError(t, err, "could not make request")
					_ = req.Body.Close()
				}
			},
			expectedEndpoints: getExpectedOutcomeForPathWithRepeatedChars(),
		},
	}
	for _, tt := range tests {
		for _, clientCount := range []int{1, 2, 5} {
			testNameSuffix := fmt.Sprintf("-different clients - %v", clientCount)
			t.Run(tt.name+testNameSuffix, func(t *testing.T) {
				t.Cleanup(func() { cleanProtocolMaps(t, "http2", monitor.ebpfProgram.Manager.Manager) })
				tt.runClients(t, clientCount)

				res := make(map[usmhttp.Key]int)
				assert.Eventually(t, func() bool {
					for key, stat := range getHTTPLikeProtocolStats(monitor, protocols.HTTP2) {
						if key.DstPort == srvPort || key.SrcPort == srvPort {
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
						valRange, ok := tt.expectedEndpoints[key]
						if !ok {
							return false
						}
						if count < valRange.lower || count > valRange.upper {
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
					ebpftest.DumpMapsTestHelper(t, monitor.DumpMaps, usmhttp2.InFlightMap)
					dumpTelemetry(t, monitor, s.isTLS)
				}
			})
		}
	}
}

var (
	// pathExceedingMaxSize is path with size 166, which is exceeding the maximum path size in the kernel (HTTP2_MAX_PATH_LEN).
	pathExceedingMaxSize = "X2YRUwfeNEmYWkk0bACThVya8MoSUkR7ZKANCPYkIGHvF9CWGA0rxXKsGogQag7HsJfmgaar3TiOTRUb3ynbmiOz3As9rXYjRGNRdCWGgdBPL8nGa6WheGlJLNtIVsUcxSerNQKmoQqqDLjGftbKXjqdMJLVY6UyECeXOKrrFU9aHx2fjlk2qMNDUptYWuzPPCWAnKOV7Ph"

	http2UniquePaths = []string{
		// size 82 bucket 0
		"C9ZaSMOpthT9XaRh9yc6AKqfIjT43M8gOz3p9ASKCNRIcLbc3PTqEoms2SDwt6Q90QM7DxjWKlmZUfRU1eOx5DjQOOhLaIJQke4N",
		// size 127 bucket 1
		"ZtZuUQeVB7BOl3F45oFicOOOJl21ePFwunMBvBh3bXPMBZqdEZepVsemYA0frZb5M83VHLDWq68KFELDHu0Xo28lzpzO3L7kDXuYuClivgEgURUn47kfwfUfW1PKjfsV6HaYpAZxly48lTGiRIXRINVC8b9",
		// size 137, bucket 2
		"RDBVk5COXAz52GzvuHVWRawNoKhmfxhBiTuyj5QZ6qR1DMsNOn4sWFLnaGXVzrqA8NLr2CaW1IDupzh9AzJlIvgYSf6OYIafIOsImL5O9M3AHzUHGMJ0KhjYGJAzXeTgvwl2qYWmlD9UYGELFpBSzJpykoriocvl3RRoYt4l",
		// size 147, bucket 3
		"T5r8QcP8qCiKVwhWlaxWjYCX8IrTmPrt2HRjfQJP2PxbWjLm8dP4BTDxUAmXJJWNyv4HIIaR3Fj6n8Tu6vSoDcBtKFuMqIPAdYEJt0qo2aaYDKomIJv74z7SiN96GrOufPTm6Eutl3JGeAKW2b0dZ4VYUsIOO8aheEOGmyhyWBymgCtBcXeki1",
		// size 158, bucket 4
		"VP4zOrIPiGhLDLSJYSVU78yUcb8CkU0dVDIZqPq98gVoenX5p1zS6cRX4LtrfSYKCQFX6MquluhDD2GPjZYFIraDLIHCno3yipQBLPGcPbPTgv9SD6jOlHMuLjmsGxyC3y2Hk61bWA6Af4D2SYS0q3BS7ahJ0vjddYYBRIpwMOOIez2jaR56rPcGCRW2eq0T1x",
		// size 166, bucket 5
		pathExceedingMaxSize,
		// size 172, bucket 6
		"bq5bcpUgiW1CpKgwdRVIulFMkwRenJWYdW8aek69anIV8w3br0pjGNtfnoPCyj4HUMD5MxWB2xM4XGp7fZ1JRHvskRZEgmoM7ag9BeuigmH05p7dzMwKsD76MqKyPmfhwBUZHLKtJ52ia3mOuMvyYiQNwA6KAU509bwuy4NCREVUAP76WFeAzr0jBvqMFXLg3eQQERIW0tKTcjQg8m9Jse",
		// size 247, bucket 7
		"LUhWUWPMztVFuEs83i7RmoxRiV1KzOq0NsZmGXVyW49BbBaL63m8H5vDwiewrrKbldXBuctplDxB28QekDclM6cO9BIsRqvzS3a802aOkRHTEruotA8Xh5K9GOMv9DzdoOL9P3GFPsUPgBy0mzFyyRJGk3JXpIH290Bj2FIRnIIpIjjKE1akeaimsuGEheA4D95axRpGmz4cm2s74UiksfBi4JnVX2cBzZN3oQaMt7zrWofwyzcZeF5W1n6BAQWxPPWe4Jyoc34jQ2fiEXQO0NnXe1RFbBD1E33a0OycziXZH9hEP23xvh",
	}
)

func (s *usmHTTP2Suite) TestHTTP2KernelTelemetry() {
	t := s.T()
	cfg := s.getCfg()

	// Start local server and register its cleanup.
	t.Cleanup(startH2CServer(t, authority, s.isTLS))

	// Start the proxy server.
	proxyProcess, cancel := proxy.NewExternalUnixTransparentProxyServer(t, unixPath, authority, s.isTLS)
	t.Cleanup(cancel)
	require.NoError(t, proxy.WaitForConnectionReady(unixPath))

	tests := []struct {
		name              string
		runClients        func(t *testing.T, clientsCount int)
		expectedTelemetry *usmhttp2.HTTP2Telemetry
	}{
		{
			name: "Fill each bucket",
			runClients: func(t *testing.T, clientsCount int) {
				clients := getHTTP2UnixClientArray(clientsCount, unixPath)
				for _, path := range http2UniquePaths {
					client := clients[getClientsIndex(1, clientsCount)]
					req, err := client.Post(http2SrvAddr+"/"+path, "application/json", bytes.NewReader([]byte("test")))
					require.NoError(t, err, "could not make request")
					_ = req.Body.Close()
				}
			},

			expectedTelemetry: &usmhttp2.HTTP2Telemetry{
				Request_seen:      8,
				Response_seen:     8,
				End_of_stream:     16,
				End_of_stream_rst: 0,
				Path_size_bucket:  [8]uint64{1, 1, 1, 1, 1, 1, 1, 1},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			monitor := setupUSMTLSMonitor(t, cfg)
			if s.isTLS {
				utils.WaitForProgramsToBeTraced(t, GoTLSAttacherName, proxyProcess.Process.Pid, utils.ManualTracingFallbackEnabled)
			}

			tt.runClients(t, 1)

			// We cannot predict if the client will send an RST frame or not, thus we cannot predict the number of
			// frames with EOS or RST frames, which leads into a flaking test. Therefore, we are asserting that the
			// gotten number of EOS or RST frames is at least the number of expected EOS frames.
			expectedEOSOrRST := tt.expectedTelemetry.End_of_stream + tt.expectedTelemetry.End_of_stream_rst
			var telemetry *usmhttp2.HTTP2Telemetry
			var err error
			assert.Eventually(t, func() bool {
				telemetry, err = getHTTP2KernelTelemetry(monitor, s.isTLS)
				require.NoError(t, err)
				if telemetry.Request_seen != tt.expectedTelemetry.Request_seen {
					return false
				}
				if telemetry.Response_seen != tt.expectedTelemetry.Response_seen {
					return false
				}
				if telemetry.Literal_value_exceeds_frame != tt.expectedTelemetry.Literal_value_exceeds_frame {
					return false
				}
				if telemetry.Exceeding_max_interesting_frames != tt.expectedTelemetry.Exceeding_max_interesting_frames {
					return false
				}
				if telemetry.Exceeding_max_frames_to_filter != tt.expectedTelemetry.Exceeding_max_frames_to_filter {
					return false
				}
				if telemetry.End_of_stream+telemetry.End_of_stream_rst < expectedEOSOrRST {
					return false
				}
				return reflect.DeepEqual(telemetry.Path_size_bucket, tt.expectedTelemetry.Path_size_bucket)
			}, time.Second*5, time.Millisecond*100)
			if t.Failed() {
				t.Logf("expected telemetry: %+v;\ngot: %+v", tt.expectedTelemetry, telemetry)
			}
		})
	}
}

func (s *usmHTTP2Suite) TestHTTP2ManyDifferentPaths() {
	t := s.T()
	cfg := s.getCfg()

	// Start local server and register its cleanup.
	t.Cleanup(startH2CServer(t, authority, s.isTLS))

	// Start the proxy server.
	proxyProcess, cancel := proxy.NewExternalUnixTransparentProxyServer(t, unixPath, authority, s.isTLS)
	t.Cleanup(cancel)
	require.NoError(t, proxy.WaitForConnectionReady(unixPath))

	monitor := setupUSMTLSMonitor(t, cfg)
	if s.isTLS {
		utils.WaitForProgramsToBeTraced(t, GoTLSAttacherName, proxyProcess.Process.Pid, utils.ManualTracingFallbackEnabled)
	}

	const (
		repetitionsPerRequest = 2
		// Should be bigger than the length of the http2_dynamic_table which is 1024
		numberOfRequests         = 1500
		expectedNumberOfRequests = numberOfRequests * repetitionsPerRequest
	)
	clients := getHTTP2UnixClientArray(1, unixPath)
	for i := 0; i < numberOfRequests; i++ {
		for j := 0; j < repetitionsPerRequest; j++ {
			req, err := clients[0].Post(fmt.Sprintf("%s/test-%d", http2SrvAddr, i+1), "application/json", bytes.NewReader([]byte("test")))
			require.NoError(t, err, "could not make request")
			_ = req.Body.Close()
		}
	}

	matches := PrintableInt(0)

	seenRequests := map[string]int{}
	assert.Eventuallyf(t, func() bool {
		for key, stat := range getHTTPLikeProtocolStats(monitor, protocols.HTTP2) {
			if (key.DstPort == srvPort || key.SrcPort == srvPort) && key.Method == usmhttp.MethodPost && strings.HasPrefix(key.Path.Content.Get(), "/test") {
				if _, ok := seenRequests[key.Path.Content.Get()]; !ok {
					seenRequests[key.Path.Content.Get()] = 0
				}
				seenRequests[key.Path.Content.Get()] += stat.Data[200].Count
				matches.Add(stat.Data[200].Count)
			}
		}

		// Due to a known issue in http2, we might consider an RST packet as a response to a request and therefore
		// we might capture a request twice. This is why we are expecting to see 2*numberOfRequests instead of
		return (expectedNumberOfRequests-1) <= matches.Load() && matches.Load() <= (expectedNumberOfRequests+1)
	}, time.Second*10, time.Millisecond*100, "%v != %v", &matches, expectedNumberOfRequests)

	for i := 0; i < numberOfRequests; i++ {
		if v, ok := seenRequests[fmt.Sprintf("/test-%d", i+1)]; !ok || v != repetitionsPerRequest {
			t.Logf("path: /test-%d should have %d occurrences but instead has %d", i+1, repetitionsPerRequest, v)
		}
	}
}

// DynamicTableSize is the size of the dynamic table used in the HPACK encoder.
const (
	defaultDynamicTableSize = 100
	endStream               = true
	endHeaders              = true
)

func (s *usmHTTP2Suite) TestRawTraffic() {
	t := s.T()
	cfg := s.getCfg()

	usmMonitor := setupUSMTLSMonitor(t, cfg)

	// Start local server and register its cleanup.
	t.Cleanup(startH2CServer(t, authority, s.isTLS))

	// Start the proxy server.
	proxyProcess, cancel := proxy.NewExternalUnixTransparentProxyServer(t, unixPath, authority, s.isTLS)
	t.Cleanup(cancel)
	require.NoError(t, proxy.WaitForConnectionReady(unixPath))

	if s.isTLS {
		utils.WaitForProgramsToBeTraced(t, GoTLSAttacherName, proxyProcess.Process.Pid, utils.ManualTracingFallbackEnabled)
	}
	tests := []struct {
		name              string
		skip              bool
		messageBuilder    func() [][]byte
		expectedEndpoints map[usmhttp.Key]int
	}{
		{
			name: "parse_frames tail call using 1 program",
			// The objective of this test is to verify that we accurately perform the parsing of frames within
			// a single program.
			messageBuilder: func() [][]byte {
				const settingsFramesCount = 238
				framer := newFramer()
				return [][]byte{
					framer.
						writeMultiMessage(t, settingsFramesCount, framer.writeSettings).
						writeHeaders(t, 1, usmhttp2.HeadersFrameOptions{Headers: testHeaders()}).
						writeData(t, 1, endStream, emptyBody).
						bytes(),
				}
			},
			expectedEndpoints: map[usmhttp.Key]int{
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
					Method: usmhttp.MethodPost,
				}: 1,
			},
		},
		{
			name: "validate frames_filter tail calls limit",
			// The purpose of this test is to validate that when we do not surpass
			// the tail call limit of HTTP2_MAX_TAIL_CALLS_FOR_FRAMES_FILTER.
			messageBuilder: func() [][]byte {
				const settingsFramesCount = 241
				framer := newFramer()
				return [][]byte{
					framer.
						writeMultiMessage(t, settingsFramesCount, framer.writeSettings).
						writeHeaders(t, 1, usmhttp2.HeadersFrameOptions{Headers: testHeaders()}).
						writeData(t, 1, endStream, emptyBody).
						bytes(),
				}
			},
			expectedEndpoints: nil,
		},
		{
			name: "validate max interesting frames limit",
			// The purpose of this test is to verify our ability to reach almost to the limit set by HTTP2_MAX_FRAMES_ITERATIONS, which
			// determines the maximum number of "interesting frames" we can process.
			// Not testing the max number of frame (240), as the server response can contain other frames, so we might miss EOS
			// coming after a server side frame, and by that miss a request.
			messageBuilder: func() [][]byte {
				// Generates 119 (requests) * 2 (number of frames per request) = 238 frames
				const iterations = 119
				framer := newFramer()
				for i := 0; i < iterations; i++ {
					streamID := getStreamID(i)
					framer.
						writeHeaders(t, streamID, usmhttp2.HeadersFrameOptions{Headers: testHeaders()}).
						writeData(t, streamID, endStream, emptyBody)
				}
				return [][]byte{framer.bytes()}
			},
			expectedEndpoints: map[usmhttp.Key]int{
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
					Method: usmhttp.MethodPost,
				}: 119,
			},
		},
		{
			name: "validate literal header field without indexing",
			// The purpose of this test is to verify our ability the case:
			// Literal Header Field without Indexing (0b0000xxxx: top four bits are 0000)
			// https://httpwg.org/specs/rfc7541.html#rfc.section.C.2.2
			messageBuilder: func() [][]byte {
				const iterations = 5
				framer := newFramer()
				for i := 0; i < iterations; i++ {
					streamID := getStreamID(i)
					framer.
						writeHeaders(t, streamID, usmhttp2.HeadersFrameOptions{
							Headers:                generateTestHeaderFields(headersGenerationOptions{pathTypeValue: pathLiteralWithoutIndexing}),
							DynamicTableUpdateSize: defaultDynamicTableSize}).
						writeData(t, streamID, endStream, emptyBody)
				}
				return [][]byte{framer.bytes()}
			},
			expectedEndpoints: map[usmhttp.Key]int{
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString("/" + strings.Repeat("a", defaultDynamicTableSize))},
					Method: usmhttp.MethodPost,
				}: 5,
			},
		},
		{
			name: "validate literal header field never indexed",
			// The purpose of this test is to verify our ability the case:
			// Literal Header Field never Indexed (0b0001xxxx: top four bits are 0001)
			// https://httpwg.org/specs/rfc7541.html#rfc.section.6.2.3
			messageBuilder: func() [][]byte {
				const iterations = 5
				framer := newFramer()
				for i := 0; i < iterations; i++ {
					streamID := getStreamID(i)
					framer.
						writeHeaders(t, streamID, usmhttp2.HeadersFrameOptions{
							Headers: generateTestHeaderFields(headersGenerationOptions{pathTypeValue: pathLiteralNeverIndexed})}).
						writeData(t, streamID, endStream, emptyBody)
				}
				return [][]byte{framer.bytes()}
			},
			expectedEndpoints: map[usmhttp.Key]int{
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
					Method: usmhttp.MethodPost,
				}: 5,
			},
		},
		{
			name: "validate path with index 4",
			// The purpose of this test is to verify our ability to identify paths with index 4.
			messageBuilder: func() [][]byte {
				const iterations = 5
				// pathHeaderField is the hex representation of the path /aaa with index 4.
				pathHeaderField := []byte{0x44, 0x83, 0x60, 0x63, 0x1f}
				headerFields := removeHeaderFieldByKey(testHeaders(), ":path")
				headersFrame, err := usmhttp2.NewHeadersFrameMessage(usmhttp2.HeadersFrameOptions{
					Headers: headerFields,
				})
				require.NoError(t, err, "could not create headers frame")

				// we are adding the path header field with index 4, we need to do it on the byte slice and not on the headerFields
				// due to the fact that when we create a header field it would be with index 5.
				headersFrame = append(pathHeaderField, headersFrame...)
				framer := newFramer()
				for i := 0; i < iterations; i++ {
					streamID := getStreamID(i)
					framer.
						writeRawHeaders(t, streamID, endHeaders, headersFrame).
						writeData(t, streamID, endStream, emptyBody)
				}
				return [][]byte{framer.bytes()}
			},
			expectedEndpoints: map[usmhttp.Key]int{
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
					Method: usmhttp.MethodPost,
				}: 5,
			},
		},
		{
			name: "validate PING and WINDOWS_UPDATE frames between HEADERS and DATA",
			// The purpose of this test is to verify our ability to process PING and WINDOWS_UPDATE frames between HEADERS and DATA.
			messageBuilder: func() [][]byte {
				const iterations = 5
				framer := newFramer()
				for i := 0; i < iterations; i++ {
					streamID := getStreamID(i)
					framer.
						writeHeaders(t, streamID, usmhttp2.HeadersFrameOptions{
							Headers: testHeaders(),
						}).
						writePing(t).
						writeWindowUpdate(t, streamID, 1).
						writeData(t, streamID, endStream, emptyBody)
				}

				return [][]byte{framer.bytes()}
			},
			expectedEndpoints: map[usmhttp.Key]int{
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
					Method: usmhttp.MethodPost,
				}: 5,
			},
		},
		{
			name: "validate RST_STREAM cancel err code",
			// The purpose of this test is to validate that when a cancel error code is sent, we will not count the request.
			// We are sending 10 requests, and 5 of them will contain RST_STREAM with a cancel error code.Therefore, we expect to
			// capture five valid requests.
			messageBuilder: func() [][]byte {
				const iterations = 10
				rstFramesCount := 5
				framer := newFramer()
				for i := 0; i < iterations; i++ {
					streamID := getStreamID(i)
					framer.
						writeHeaders(t, streamID, usmhttp2.HeadersFrameOptions{Headers: testHeaders()}).
						writeData(t, streamID, endStream, emptyBody)
					if rstFramesCount > 0 {
						framer.writeRSTStream(t, streamID, http2.ErrCodeCancel)
						rstFramesCount--
					}
				}
				return [][]byte{framer.bytes()}
			},
			expectedEndpoints: map[usmhttp.Key]int{
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
					Method: usmhttp.MethodPost,
				}: 5,
			},
		},
		{
			name: "validate RST_STREAM before server status ok",
			// The purpose of this test is to validate that when we see RST before DATA frame with status ok,
			// we will not count the requests.
			messageBuilder: func() [][]byte {
				const iterations = 10
				framer := newFramer()
				for i := 0; i < iterations; i++ {
					streamID := getStreamID(i)
					framer.
						writeHeaders(t, streamID, usmhttp2.HeadersFrameOptions{Headers: testHeaders()}).
						writeData(t, streamID, endStream, emptyBody).
						writeRSTStream(t, streamID, http2.ErrCodeNo)
				}
				return [][]byte{framer.bytes()}
			},
			expectedEndpoints: nil,
		},
		{
			name: "validate various status codes",
			// The purpose of this test is to verify that we support status codes that both do and
			// do not appear in the static table.
			messageBuilder: func() [][]byte {
				statusCodes := []int{200, 201, 204, 206, 300, 304, 400, 401, 404, 500, 504}
				const iterationsPerStatusCode = 3
				messages := make([][]byte, 0, len(statusCodes)*iterationsPerStatusCode)
				for statusCodeIteration, statusCode := range statusCodes {
					for i := 0; i < iterationsPerStatusCode; i++ {
						streamID := getStreamID(statusCodeIteration*iterationsPerStatusCode + i)
						messages = append(messages, newFramer().
							writeHeaders(t, streamID, usmhttp2.HeadersFrameOptions{Headers: headersWithGivenEndpoint(fmt.Sprintf("/status/%d", statusCode))}).
							writeData(t, streamID, endStream, emptyBody).
							bytes())
					}
				}
				return messages
			},
			expectedEndpoints: map[usmhttp.Key]int{
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString("/status/200")},
					Method: usmhttp.MethodPost,
				}: 3,
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString("/status/201")},
					Method: usmhttp.MethodPost,
				}: 3,
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString("/status/204")},
					Method: usmhttp.MethodPost,
				}: 3,
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString("/status/206")},
					Method: usmhttp.MethodPost,
				}: 3,
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString("/status/300")},
					Method: usmhttp.MethodPost,
				}: 3,
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString("/status/304")},
					Method: usmhttp.MethodPost,
				}: 3,
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString("/status/400")},
					Method: usmhttp.MethodPost,
				}: 3,
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString("/status/401")},
					Method: usmhttp.MethodPost,
				}: 3,
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString("/status/404")},
					Method: usmhttp.MethodPost,
				}: 3,
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString("/status/500")},
					Method: usmhttp.MethodPost,
				}: 3,
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString("/status/504")},
					Method: usmhttp.MethodPost,
				}: 3,
			},
		},
		{
			name: "validate http methods",
			// The purpose of this test is to validate that we are able to capture all http methods.
			messageBuilder: func() [][]byte {
				httpMethods := []string{http.MethodGet, http.MethodPost, http.MethodHead, http.MethodDelete,
					http.MethodPut, http.MethodPatch, http.MethodOptions, http.MethodTrace, http.MethodConnect}
				// Duplicating the list to have each method appearing twice
				httpMethods = append(httpMethods, httpMethods...)
				// Currently, the methods TRACE and CONNECT are not supported by the http.Method package.
				// Therefore, we mark those requests as incomplete and not expected to be captured.
				framer := newFramer()
				var buf bytes.Buffer
				encoder := hpack.NewEncoder(&buf)
				for i, method := range httpMethods {
					streamID := getStreamID(i)
					framer.
						writeHeadersWithEncoder(t, streamID, usmhttp2.HeadersFrameOptions{Headers: generateTestHeaderFields(headersGenerationOptions{overrideMethod: method})}, encoder, &buf).
						writeData(t, streamID, endStream, emptyBody)
				}
				return [][]byte{framer.bytes()}
			},
			expectedEndpoints: map[usmhttp.Key]int{
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
					Method: usmhttp.MethodGet,
				}: 2,
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
					Method: usmhttp.MethodPost,
				}: 2,
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
					Method: usmhttp.MethodHead,
				}: 2,
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
					Method: usmhttp.MethodDelete,
				}: 2,
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
					Method: usmhttp.MethodPut,
				}: 2,
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
					Method: usmhttp.MethodPatch,
				}: 2,
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
					Method: usmhttp.MethodOptions,
				}: 2,
			},
		},
		{
			name: "validate http methods huffman encoded",
			// The purpose of this test is to validate that we are able to capture all http methods.
			messageBuilder: func() [][]byte {
				httpMethods = []string{http.MethodGet, http.MethodPost, http.MethodHead, http.MethodDelete,
					http.MethodPut, http.MethodPatch, http.MethodOptions, http.MethodTrace, http.MethodConnect}
				// Currently, the methods TRACE and CONNECT are not supported by the http.Method package.
				// Therefore, we mark those requests as incomplete and not expected to be captured.
				framer := newFramer()
				headerFields := removeHeaderFieldByKey(testHeaders(), ":method")
				headersFrame, err := usmhttp2.NewHeadersFrameMessage(usmhttp2.HeadersFrameOptions{
					Headers: headerFields,
				})
				require.NoError(t, err, "could not create headers frame")
				for i, method := range httpMethods {
					huffMethod := hpack.AppendHuffmanString([]byte{}, method)
					// we are adding 128 to the length of the huffman encoded method,
					// as it is the representation of the huffman encoding (MSB ofo the octet is on).
					rawMethod := append([]byte{0x43}, byte(0x80|len(huffMethod)))
					rawMethod = append(rawMethod, huffMethod...)
					headersFrameWithRawMethod := append(rawMethod, headersFrame...)
					streamID := getStreamID(i)
					framer.
						writeRawHeaders(t, streamID, endHeaders, headersFrameWithRawMethod).
						writeData(t, streamID, endStream, emptyBody).bytes()
				}
				return [][]byte{framer.bytes()}
			},
			expectedEndpoints: map[usmhttp.Key]int{
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
					Method: usmhttp.MethodGet,
				}: 1,
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
					Method: usmhttp.MethodPost,
				}: 1,
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
					Method: usmhttp.MethodHead,
				}: 1,
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
					Method: usmhttp.MethodDelete,
				}: 1,
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
					Method: usmhttp.MethodPut,
				}: 1,
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
					Method: usmhttp.MethodPatch,
				}: 1,
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
					Method: usmhttp.MethodOptions,
				}: 1,
			},
		},
		{
			name: "validate http methods POST and GET methods as literal",
			// The purpose of this test is to validate that when the methods POST and GET are sent as literal fields
			// and not indexed, we are still able to capture and process them correctly.
			messageBuilder: func() [][]byte {
				getMethodRaw := append([]byte{0x43, 0x03}, []byte("GET")...)
				postMethodRaw := append([]byte{0x43, 0x04}, []byte("POST")...)

				headerFields := removeHeaderFieldByKey(testHeaders(), ":method")
				headersFrame, err := usmhttp2.NewHeadersFrameMessage(usmhttp2.HeadersFrameOptions{
					Headers: headerFields,
				})
				require.NoError(t, err, "could not create headers frame")
				headersFrameWithGET := append(getMethodRaw, headersFrame...)
				headersFrameWithPOST := append(postMethodRaw, headersFrame...)

				framer := newFramer()
				framer.
					writeRawHeaders(t, 1, endHeaders, headersFrameWithGET).
					writeData(t, 1, endStream, emptyBody).
					writeRawHeaders(t, 3, endHeaders, headersFrameWithPOST).
					writeData(t, 3, endStream, emptyBody)
				return [][]byte{framer.bytes()}
			},
			expectedEndpoints: map[usmhttp.Key]int{
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
					Method: usmhttp.MethodGet,
				}: 1,
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
					Method: usmhttp.MethodPost,
				}: 1,
			},
		},
		{
			name: "validate max path length",
			// The purpose of this test is to validate that we are not able to process a path longer than HTTP2_MAX_PATH_LEN.
			messageBuilder: func() [][]byte {
				framer := newFramer()
				return [][]byte{
					framer.
						writeHeaders(t, 1, usmhttp2.HeadersFrameOptions{Headers: generateTestHeaderFields(headersGenerationOptions{pathTypeValue: pathTooLarge})}).
						writeData(t, 1, endStream, emptyBody).
						bytes(),
				}
			},
			expectedEndpoints: nil,
		},
		{
			name: "validate path sent by value (:path)",
			// The purpose of this test is to verify our ability to identify paths which were sent with a key that
			// sent by value (:path).
			messageBuilder: func() [][]byte {
				headerFields := removeHeaderFieldByKey(testHeaders(), ":path")
				headersFrame, err := usmhttp2.NewHeadersFrameMessage(usmhttp2.HeadersFrameOptions{Headers: headerFields})
				require.NoError(t, err, "could not create headers frame")
				// pathHeaderField is created with a key that sent by value (:path) and
				// the value (of the path) is /aaa.
				pathHeaderField := []byte{0x40, 0x84, 0xb9, 0x58, 0xd3, 0x3f, 0x83, 0x60, 0x63, 0x1f}
				headersFrame = append(pathHeaderField, headersFrame...)
				framer := newFramer()
				return [][]byte{
					framer.
						writeRawHeaders(t, 1, endHeaders, headersFrame).
						writeData(t, 1, endStream, emptyBody).
						bytes(),
				}
			},
			expectedEndpoints: nil,
		},
		{
			name: "Interesting frame header sent separately from frame payload",
			// Testing the scenario in which the frame header (of an interesting type) is sent separately from the frame payload.
			messageBuilder: func() [][]byte {
				headersFrame := newFramer().writeHeaders(t, 1, usmhttp2.HeadersFrameOptions{Headers: testHeaders()}).bytes()
				dataFrame := newFramer().writeData(t, 1, endStream, emptyBody).bytes()
				headersFrameHeader := headersFrame[:9]
				secondMessage := append(headersFrame[9:], dataFrame...)
				return [][]byte{
					headersFrameHeader,
					secondMessage,
				}
			},
			expectedEndpoints: map[usmhttp.Key]int{
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
					Method: usmhttp.MethodPost,
				}: 1,
			},
		},
		{
			name: "Not interesting frame header sent separately from frame payload",
			// Testing the scenario in which the frame header (of a not interesting type) is sent separately from the frame payload.
			messageBuilder: func() [][]byte {
				pingFrame := newFramer().writePing(t).bytes()
				fullFrame := newFramer().writeHeaders(t, 1, usmhttp2.HeadersFrameOptions{Headers: testHeaders()}).
					writeData(t, 1, endStream, emptyBody).bytes()
				pingFrameHeader := pingFrame[:9]
				secondMessage := append(pingFrame[9:], fullFrame...)
				return [][]byte{
					pingFrameHeader,
					secondMessage,
				}
			},
			expectedEndpoints: map[usmhttp.Key]int{
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
					Method: usmhttp.MethodPost,
				}: 1,
			},
		},
		{
			name: "validate dynamic table update with indexed header field",
			// The purpose of this test is to verify our ability to support dynamic table update
			// while using a path with indexed header field.
			messageBuilder: func() [][]byte {
				const iterations = 5
				framer := newFramer()
				for i := 0; i < iterations; i++ {
					streamID := getStreamID(i)
					framer.
						writeHeaders(t, streamID, usmhttp2.HeadersFrameOptions{
							Headers:                headersWithGivenEndpoint("/"),
							DynamicTableUpdateSize: defaultDynamicTableSize}).
						writeData(t, streamID, endStream, emptyBody)
				}
				return [][]byte{framer.bytes()}
			},
			expectedEndpoints: map[usmhttp.Key]int{
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString("/")},
					Method: usmhttp.MethodPost,
				}: 5,
			},
		},
		{
			name: "validate dynamic table update with high value and indexed header field",
			// The purpose of this test is to verify our ability to support dynamic table update
			// with size exceeds one octet, to validate the iteration for parsing the dynamic table update.
			messageBuilder: func() [][]byte {
				const iterations = 5
				const dynamicTableSize = 4000
				framer := newFramer()
				for i := 0; i < iterations; i++ {
					streamID := getStreamID(i)
					framer.
						writeHeaders(t, streamID, usmhttp2.HeadersFrameOptions{
							Headers:                headersWithGivenEndpoint("/"),
							DynamicTableUpdateSize: dynamicTableSize,
						}).
						writeData(t, streamID, endStream, emptyBody)
				}
				return [][]byte{framer.bytes()}
			},
			expectedEndpoints: map[usmhttp.Key]int{
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString("/")},
					Method: usmhttp.MethodPost,
				}: 5,
			},
		},
		{
			name: "Data frame header sent separately from frame payload",
			// Testing the scenario in which the data frame header is sent separately from the frame payload.
			messageBuilder: func() [][]byte {
				payload := []byte("test")
				headersFrame := newFramer().writeHeaders(t, 1, usmhttp2.HeadersFrameOptions{Headers: testHeaders()}).bytes()
				dataFrame := newFramer().writeData(t, 1, endStream, payload).bytes()
				// We are creating a header frame with a content-length header field that contains the payload size.
				secondMessageHeadersFrame := newFramer().writeHeaders(t, 3, usmhttp2.HeadersFrameOptions{
					Headers: generateTestHeaderFields(headersGenerationOptions{
						overrideContentLength: len(payload)})}).writeData(t, 3, endStream, payload).bytes()

				headersFrameHeader := append(headersFrame, dataFrame[:9]...)
				secondMessage := append(dataFrame[9:], secondMessageHeadersFrame...)
				return [][]byte{
					headersFrameHeader,
					secondMessage,
				}
			},
			expectedEndpoints: map[usmhttp.Key]int{
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
					Method: usmhttp.MethodPost,
				}: 2,
			},
		},
		{
			name: "Data frame header sent separately from frame payload with PING between them",
			// Testing the scenario in which the data frame header is sent separately from the frame payload.,
			// including a PING frame in the second message between the data frame.
			messageBuilder: func() [][]byte {
				payload := []byte("test")
				// We are creating a header frame with a content-length header field that contains the payload size.
				headersFrame := newFramer().writeHeaders(t, 1, usmhttp2.HeadersFrameOptions{
					Headers: generateTestHeaderFields(headersGenerationOptions{
						overrideContentLength: len(payload)})}).bytes()
				dataFrame := newFramer().writeData(t, 1, endStream, payload).bytes()
				pingFrame := newFramer().writePing(t).bytes()

				headersFrameHeader := append(headersFrame, dataFrame[:9]...)
				secondMessage := append(pingFrame, dataFrame[9:]...)
				return [][]byte{
					headersFrameHeader,
					secondMessage,
				}
			},
			expectedEndpoints: map[usmhttp.Key]int{
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
					Method: usmhttp.MethodPost,
				}: 1,
			},
		},
		{
			name: "Payload data frame header sent separately",
			// Testing the scenario in which the data frame header is sent separately from the frame payload.
			messageBuilder: func() [][]byte {
				payload := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9}
				// We are creating a header frame with a content-length header field that contains the payload size.
				headersFrame := newFramer().writeHeaders(t, 1, usmhttp2.HeadersFrameOptions{
					Headers: generateTestHeaderFields(headersGenerationOptions{
						overrideContentLength: len(payload)})}).bytes()
				dataFrame := newFramer().writeData(t, 1, endStream, payload).bytes()
				secondMessageHeadersFrame := newFramer().writeHeaders(t, 3, usmhttp2.HeadersFrameOptions{Headers: testHeaders()}).bytes()
				secondMessageDataFrame := newFramer().writeData(t, 3, endStream, emptyBody).bytes()

				// We are cutting in the middle of the payload
				firstMessage := append(headersFrame, dataFrame[:9+3]...)
				secondMessage := append(dataFrame[9+3:], secondMessageHeadersFrame...)
				secondMessage = append(secondMessage, secondMessageDataFrame...)
				return [][]byte{
					firstMessage,
					secondMessage,
				}
			},
			expectedEndpoints: map[usmhttp.Key]int{
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
					Method: usmhttp.MethodPost,
				}: 2,
			},
		},
		{
			name: "validate CONTINUATION frame support",
			// Testing the scenario in which part of the data frame header is sent using CONTINUATION frame.
			// Currently, we do not support CONTINUATION frames, therefore, we expect to capture only the first
			// part of the message.
			messageBuilder: func() [][]byte {
				const headersFrameEndHeaders = false
				fullHeaders := generateTestHeaderFields(headersGenerationOptions{})
				prefixHeadersFrame, err := usmhttp2.NewHeadersFrameMessage(usmhttp2.HeadersFrameOptions{
					Headers: fullHeaders[:2],
				})
				require.NoError(t, err, "could not create prefix headers frame")

				suffixHeadersFrame, err := usmhttp2.NewHeadersFrameMessage(usmhttp2.HeadersFrameOptions{
					Headers: fullHeaders[2:],
				})
				require.NoError(t, err, "could not create suffix headers frame")

				return [][]byte{
					newFramer().writeRawHeaders(t, 1, headersFrameEndHeaders, prefixHeadersFrame).
						writeRawContinuation(t, 1, endHeaders, suffixHeadersFrame).
						writeData(t, 1, endStream, emptyBody).bytes(),
				}
			},
			expectedEndpoints: nil,
		},
		{
			name: "validate message split into 2 tcp segments",
			// The purpose of this test is to validate that we cannot handle reassembled tcp segments.
			messageBuilder: func() [][]byte {
				a := newFramer().
					writeHeaders(t, 1, usmhttp2.HeadersFrameOptions{Headers: testHeaders()}).
					writeData(t, 1, true, emptyBody).bytes()
				return [][]byte{
					a[:10],
					a[10:],
				}

			},
			expectedEndpoints: nil,
		},
		{
			name: "remainder + header remainder",
			// Testing the scenario where we have both a remainder (a frame's payload split over 2 packets) and in the
			// second packet, we have the remainder and a partial frame header of a new request. We're testing that we
			// can capture the 2 requests in this scenario.
			messageBuilder: func() [][]byte {
				data := []byte("testcontent")
				request1 := newFramer().
					writeHeaders(t, 1, usmhttp2.HeadersFrameOptions{Headers: generateTestHeaderFields(headersGenerationOptions{overrideContentLength: len(data)})}).
					writeData(t, 1, true, data).bytes()
				request2 := newFramer().
					writeHeaders(t, 3, usmhttp2.HeadersFrameOptions{Headers: headersWithGivenEndpoint("/bbb")}).
					writeData(t, 3, true, emptyBody).bytes()
				firstPacket := request1[:len(request1)-6]
				secondPacket := append(request1[len(request1)-6:], request2[:5]...)
				return [][]byte{
					firstPacket,
					secondPacket,
					request2[5:],
				}
			},
			expectedEndpoints: map[usmhttp.Key]int{
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
					Method: usmhttp.MethodPost,
				}: 1,
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString("/bbb")},
					Method: usmhttp.MethodPost,
				}: 1,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip {
				t.Skip("skipping test")
			}

			t.Cleanup(func() { cleanProtocolMaps(t, "http2", usmMonitor.ebpfProgram.Manager.Manager) })
			c := dialHTTP2Server(t)

			// Composing a message with the number of setting frames we want to send.
			require.NoError(t, writeInput(c, 500*time.Millisecond, tt.messageBuilder()...))

			res := make(map[usmhttp.Key]int)
			assert.Eventually(t, func() bool {
				return validateStats(usmMonitor, res, tt.expectedEndpoints, s.isTLS)
			}, time.Second*5, time.Millisecond*100, "%v != %v", res, tt.expectedEndpoints)
			if t.Failed() {
				for key := range tt.expectedEndpoints {
					if _, ok := res[key]; !ok {
						t.Logf("key: %v was not found in res", key.Path.Content.Get())
					}
				}
				ebpftest.DumpMapsTestHelper(t, usmMonitor.DumpMaps, usmhttp2.InFlightMap, "http2_dynamic_table")
				dumpTelemetry(t, usmMonitor, s.isTLS)
			}
		})
	}
}

func (s *usmHTTP2Suite) TestDynamicTable() {
	t := s.T()
	cfg := s.getCfg()

	// Start local server and register its cleanup.
	t.Cleanup(startH2CServer(t, authority, s.isTLS))

	// Start the proxy server.
	proxyProcess, cancel := proxy.NewExternalUnixTransparentProxyServer(t, unixPath, authority, s.isTLS)
	t.Cleanup(cancel)
	require.NoError(t, proxy.WaitForConnectionReady(unixPath))

	tests := []struct {
		name                            string
		skip                            bool
		messageBuilder                  func() []byte
		expectedEndpoints               map[usmhttp.Key]int
		expectedDynamicTablePathIndexes []int
	}{
		{
			name: "dynamic table contains only index of path",
			// The purpose of this test is to validate that the dynamic table contains only paths indexes.
			messageBuilder: func() []byte {
				const iterations = 10
				framer := newFramer()

				for i := 0; i < iterations; i++ {
					streamID := getStreamID(i)
					framer.
						writeHeaders(t, streamID, usmhttp2.HeadersFrameOptions{Headers: testHeaders()}).
						writeData(t, streamID, endStream, emptyBody)
				}

				return framer.bytes()
			},
			expectedEndpoints: map[usmhttp.Key]int{
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
					Method: usmhttp.MethodPost,
				}: 10,
			},
			expectedDynamicTablePathIndexes: []int{1, 7, 13, 19, 25, 31, 37, 43, 49, 55},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			usmMonitor := setupUSMTLSMonitor(t, cfg)
			if s.isTLS {
				utils.WaitForProgramsToBeTraced(t, GoTLSAttacherName, proxyProcess.Process.Pid, utils.ManualTracingFallbackEnabled)
			}

			c := dialHTTP2Server(t)

			// Composing a message with the number of setting frames we want to send.
			require.NoError(t, writeInput(c, 500*time.Millisecond, tt.messageBuilder()))

			res := make(map[usmhttp.Key]int)
			assert.Eventually(t, func() bool {
				// validate the stats we get
				require.True(t, validateStats(usmMonitor, res, tt.expectedEndpoints, s.isTLS))

				validateDynamicTableMap(t, usmMonitor.ebpfProgram, tt.expectedDynamicTablePathIndexes)

				return true
			}, time.Second*5, time.Millisecond*100, "%v != %v", res, tt.expectedEndpoints)
			if t.Failed() {
				for key := range tt.expectedEndpoints {
					if _, ok := res[key]; !ok {
						t.Logf("key: %v was not found in res", key.Path.Content.Get())
					}
				}
				ebpftest.DumpMapsTestHelper(t, usmMonitor.DumpMaps, usmhttp2.InFlightMap)
				dumpTelemetry(t, usmMonitor, s.isTLS)
			}
		})
	}
}

// TestRemainderTable tests the remainder table map.
// We would like to make sure that the remainder table map is being updated correctly.
func (s *usmHTTP2Suite) TestRemainderTable() {
	t := s.T()
	cfg := s.getCfg()

	// Start local server and register its cleanup.
	t.Cleanup(startH2CServer(t, authority, s.isTLS))

	// Start the proxy server.
	proxyProcess, cancel := proxy.NewExternalUnixTransparentProxyServer(t, unixPath, authority, s.isTLS)
	t.Cleanup(cancel)
	require.NoError(t, proxy.WaitForConnectionReady(unixPath))

	tests := []struct {
		name           string
		messageBuilder func() [][]byte
		mapSize        int
	}{
		{
			name: "validate clean with remainder and header zero",
			// The purpose of this test is to validate that we cannot handle reassembled tcp segments.
			messageBuilder: func() [][]byte {
				data := []byte("test12345")
				a := newFramer().
					writeHeaders(t, 1, usmhttp2.HeadersFrameOptions{Headers: generateTestHeaderFields(headersGenerationOptions{overrideContentLength: len(data)})}).bytes()
				b := newFramer().writeData(t, 1, true, data).bytes()
				message := append(a, b[:11]...)
				return [][]byte{
					// we split it in 11 bytes in order to split the payload itself.
					message,
					b[11:],
				}
			},
			mapSize: 0,
		},
		{
			name: "validate remainder in map",
			// The purpose of this test is to validate that we cannot handle reassembled tcp segments.
			messageBuilder: func() [][]byte {
				a := newFramer().
					writeHeaders(t, 1, usmhttp2.HeadersFrameOptions{Headers: testHeaders()}).
					writeData(t, 1, true, emptyBody).bytes()
				return [][]byte{
					// we split it in 10 bytes in order to split the payload itself.
					a[:10],
					a[10:],
				}
			},
			mapSize: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usmMonitor := setupUSMTLSMonitor(t, cfg)
			if s.isTLS {
				utils.WaitForProgramsToBeTraced(t, GoTLSAttacherName, proxyProcess.Process.Pid, utils.ManualTracingFallbackEnabled)
			}

			c := dialHTTP2Server(t)

			// Composing a message with the number of setting frames we want to send.
			require.NoError(t, writeInput(c, 500*time.Millisecond, tt.messageBuilder()...))

			assert.Eventually(t, func() bool {
				require.Len(t, getRemainderTableMapKeys(t, usmMonitor.ebpfProgram), tt.mapSize)
				return true
			}, time.Second*5, time.Millisecond*100, "")
			if t.Failed() {
				ebpftest.DumpMapsTestHelper(t, usmMonitor.DumpMaps, usmhttp2.InFlightMap)
				dumpTelemetry(t, usmMonitor, s.isTLS)
			}
		})
	}
}

func (s *usmHTTP2Suite) TestRawHuffmanEncoding() {
	t := s.T()
	cfg := s.getCfg()

	// Start local server and register its cleanup.
	t.Cleanup(startH2CServer(t, authority, s.isTLS))

	// Start the proxy server.
	proxyProcess, cancel := proxy.NewExternalUnixTransparentProxyServer(t, unixPath, authority, s.isTLS)
	t.Cleanup(cancel)
	require.NoError(t, proxy.WaitForConnectionReady(unixPath))

	tests := []struct {
		name                   string
		skip                   bool
		messageBuilder         func() []byte
		expectedEndpoints      map[usmhttp.Key]int
		expectedHuffmanEncoded map[int]bool
	}{
		{
			name: "validate huffman encoding",
			// The purpose of this test is to verify that we are able to identify if the path is huffman encoded.
			messageBuilder: func() []byte {
				framer := newFramer()
				return framer.writeHeaders(t, 1, usmhttp2.HeadersFrameOptions{Headers: testHeaders()}).
					writeData(t, 1, endStream, emptyBody).
					writeHeaders(t, 3, usmhttp2.HeadersFrameOptions{Headers: headersWithGivenEndpoint("/a")}).
					writeData(t, 3, endStream, emptyBody).bytes()
			},
			expectedEndpoints: map[usmhttp.Key]int{
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString(http2DefaultTestPath)},
					Method: usmhttp.MethodPost,
				}: 1,
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString("/a")},
					Method: usmhttp.MethodPost,
				}: 1,
			},
			// the key is the path size, and the value is if it should be huffman encoded or not.
			expectedHuffmanEncoded: map[int]bool{
				2: false,
				3: true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usmMonitor := setupUSMTLSMonitor(t, cfg)
			if s.isTLS {
				utils.WaitForProgramsToBeTraced(t, GoTLSAttacherName, proxyProcess.Process.Pid, utils.ManualTracingFallbackEnabled)
			}

			c := dialHTTP2Server(t)

			// Composing a message with the number of setting frames we want to send.
			require.NoError(t, writeInput(c, 500*time.Millisecond, tt.messageBuilder()))

			res := make(map[usmhttp.Key]int)
			assert.Eventually(t, func() bool {
				// validate the stats we get
				if !validateStats(usmMonitor, res, tt.expectedEndpoints, s.isTLS) {
					return false
				}

				return validateHuffmanEncoded(t, usmMonitor.ebpfProgram, tt.expectedHuffmanEncoded)
			}, time.Second*5, time.Millisecond*100, "%v != %v", res, tt.expectedEndpoints)
			if t.Failed() {
				for key := range tt.expectedEndpoints {
					if _, ok := res[key]; !ok {
						t.Logf("key: %v was not found in res", key.Path.Content.Get())
					}
				}
				ebpftest.DumpMapsTestHelper(t, usmMonitor.DumpMaps, usmhttp2.InFlightMap)
				dumpTelemetry(t, usmMonitor, s.isTLS)
			}
		})
	}
}

func TestHTTP2InFlightMapCleaner(t *testing.T) {
	skipIfKernelNotSupported(t)
	cfg := config.New()
	cfg.EnableHTTP2Monitoring = true
	cfg.HTTP2DynamicTableMapCleanerInterval = 5 * time.Second
	cfg.HTTPIdleConnectionTTL = time.Second
	monitor := setupUSMTLSMonitor(t, cfg)
	ebpfNow, err := ddebpf.NowNanoseconds()
	require.NoError(t, err)
	http2InFLightMap, _, err := monitor.ebpfProgram.GetMap(usmhttp2.InFlightMap)
	require.NoError(t, err)
	key := usmhttp2.HTTP2StreamKey{
		Id: 1,
	}
	val := usmhttp2.HTTP2Stream{
		Request_started: uint64(ebpfNow - (time.Second * 3).Nanoseconds()),
	}
	require.NoError(t, http2InFLightMap.Update(unsafe.Pointer(&key), unsafe.Pointer(&val), ebpf.UpdateAny))

	var newVal usmhttp2.HTTP2Stream
	require.NoError(t, http2InFLightMap.Lookup(unsafe.Pointer(&key), unsafe.Pointer(&newVal)))
	require.Equal(t, val, newVal)

	require.Eventually(t, func() bool {
		err := http2InFLightMap.Lookup(unsafe.Pointer(&key), unsafe.Pointer(&newVal))
		return errors.Is(err, ebpf.ErrKeyNotExist)
	}, 3*cfg.HTTP2DynamicTableMapCleanerInterval, time.Millisecond*100)
}

// validateStats validates that the stats we get from the monitor are as expected.
func validateStats(usmMonitor *Monitor, res, expectedEndpoints map[usmhttp.Key]int, isTLS bool) bool {
	for key, stat := range getHTTPLikeProtocolStats(usmMonitor, protocols.HTTP2) {
		if key.DstPort == srvPort || key.SrcPort == srvPort {
			statusCode := testutil.StatusFromPath(key.Path.Content.Get())
			// statusCode 0 represents an error returned from the function, which means the URL is not in the special
			// form which contains the expected status code (form - `/status/{statusCode}`). So by default we use
			// 200 as the status code.
			if statusCode == 0 {
				statusCode = 200
			}
			hasTag := stat.Data[statusCode].StaticTags == network.ConnTagGo
			if hasTag != isTLS {
				continue
			}
			count := stat.Data[statusCode].Count
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

	if len(res) != len(expectedEndpoints) {
		return false
	}

	for key, endpointCount := range res {
		_, ok := expectedEndpoints[key]
		if !ok {
			return false
		}
		if endpointCount != expectedEndpoints[key] {
			return false
		}
	}
	return true
}

// getHTTP2UnixClientArray creates an array of http2 clients over a unix socket.
func getHTTP2UnixClientArray(size int, unixPath string) []*http.Client {
	res := make([]*http.Client, size)
	for i := 0; i < size; i++ {
		res[i] = &http.Client{
			Transport: &http2.Transport{
				AllowHTTP: true,
				DialTLSContext: func(context.Context, string, string, *tls.Config) (net.Conn, error) {
					return net.Dial("unix", unixPath)
				},
			},
		}
	}

	return res
}

// writeInput writes the given input to the socket and reads the response.
// Presently, the timeout is configured to one second for all readings.
// In case of encountered issues, increasing this duration might be necessary.
func writeInput(c net.Conn, timeout time.Duration, inputs ...[]byte) error {
	for i, input := range inputs {
		if _, err := c.Write(input); err != nil {
			return err
		}
		if i != len(inputs)-1 {
			// As long as we're not at the last message, we want to wait a bit before sending the next message.
			// as we've seen that Go's implementation can "merge" messages together, so having a small delay
			// between messages help us avoid this issue.
			// There is no purpose for the delay after the last message, as the use case of "merging" messages cannot
			// happen in this case.
			time.Sleep(time.Millisecond * 10)
		}
	}
	// Since we don't know when to stop reading from the socket, we set a timeout.
	if err := c.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return err
	}
	http2Framer := http2.NewFramer(io.Discard, c)
	for {
		_, err := http2Framer.ReadFrame()
		if err != nil {
			// we want to stop reading from the socket when we encounter an i/o timeout.
			if strings.Contains(err.Error(), "i/o timeout") {
				return nil
			}
			return err
		}
	}
}

// headersWithGivenEndpoint returns a set of header fields with the given path.
func headersWithGivenEndpoint(path string) []hpack.HeaderField {
	return generateTestHeaderFields(headersGenerationOptions{pathTypeValue: pathOverride, overrideEndpoint: path})
}

// testHeaders returns a set of header fields.
func testHeaders() []hpack.HeaderField { return generateTestHeaderFields(headersGenerationOptions{}) }

type headersGenerationOptions struct {
	pathTypeValue         pathType
	overrideMethod        string
	overrideEndpoint      string
	overrideContentLength int
}

// generateTestHeaderFields generates a set of header fields that will be used for the tests.
func generateTestHeaderFields(options headersGenerationOptions) []hpack.HeaderField {
	method := defaultMethod
	if options.overrideMethod != "" {
		method = options.overrideMethod
	}
	pathHeaderField := hpack.HeaderField{Name: ":path"}
	switch options.pathTypeValue {
	case pathDefault:
		pathHeaderField.Value = http2DefaultTestPath
	case pathLiteralNeverIndexed:
		pathHeaderField.Value = http2DefaultTestPath
		pathHeaderField.Sensitive = true
	case pathLiteralWithoutIndexing:
		// If we want to create a case without indexing, we need to make sure that the path is longer than 100 characters.
		// The indexing is determined by the dynamic table size (which we set to dynamicTableSize) and the size of the path.
		// ref: https://github.com/golang/net/blob/07e05fd6e95ab445ebe48840c81a027dbace3b8e/http2/hpack/encode.go#L140
		// Therefore, we want to make sure that the path is longer or equal to 100 characters so that the path will not be indexed.
		pathHeaderField.Value = "/" + strings.Repeat("a", defaultDynamicTableSize)
	case pathTooLarge:
		pathHeaderField.Value = "/" + pathExceedingMaxSize
	case pathOverride:
		pathHeaderField.Value = options.overrideEndpoint
	}

	contentLength := defaultContentLength
	if options.overrideContentLength != 0 {
		contentLength = options.overrideContentLength
	}

	return []hpack.HeaderField{
		{Name: ":authority", Value: authority},
		{Name: ":method", Value: method},
		pathHeaderField,
		{Name: ":scheme", Value: "http"},
		{Name: "content-type", Value: "application/json"},
		{Name: "content-length", Value: strconv.Itoa(contentLength)},
		{Name: "accept-encoding", Value: "gzip"},
		{Name: "user-agent", Value: "Go-http-client/2.0"},
	}
}

// removeHeaderFieldByKey removes the header field with the given key from the given header fields.
func removeHeaderFieldByKey(headerFields []hpack.HeaderField, keyToRemove string) []hpack.HeaderField {
	return slices.DeleteFunc(headerFields, func(value hpack.HeaderField) bool {
		return value.Name == keyToRemove
	})
}

func getStreamID(streamID int) uint32 {
	return uint32(streamID*2 + 1)
}

type framer struct {
	buf    *bytes.Buffer
	framer *http2.Framer
}

func newFramer() *framer {
	buf := &bytes.Buffer{}
	return &framer{
		buf:    buf,
		framer: http2.NewFramer(buf, nil),
	}
}

func (f *framer) writeMultiMessage(t *testing.T, count int, cb func(t *testing.T) *framer) *framer {
	for i := 0; i < count; i++ {
		cb(t)
	}
	return f
}

func (f *framer) bytes() []byte {
	return f.buf.Bytes()
}

func (f *framer) writeSettings(t *testing.T) *framer {
	require.NoError(t, f.framer.WriteSettings(http2.Setting{}), "could not write settings frame")
	return f
}

func (f *framer) writePing(t *testing.T) *framer {
	require.NoError(t, f.framer.WritePing(true, [8]byte{}), "could not write ping frame")
	return f
}

func (f *framer) writeRSTStream(t *testing.T, streamID uint32, errCode http2.ErrCode) *framer {
	require.NoError(t, f.framer.WriteRSTStream(streamID, errCode), "could not write RST_STREAM")
	return f
}

func (f *framer) writeData(t *testing.T, streamID uint32, endStream bool, buf []byte) *framer {
	require.NoError(t, f.framer.WriteData(streamID, endStream, buf), "could not write data frame")
	return f
}

func (f *framer) writeRawContinuation(t *testing.T, streamID uint32, endHeaders bool, buf []byte) *framer {
	require.NoError(t, f.framer.WriteContinuation(streamID, endHeaders, buf), "could not write raw continuation frame")
	return f
}

func (f *framer) writeWindowUpdate(t *testing.T, streamID uint32, increment uint32) *framer {
	require.NoError(t, f.framer.WriteWindowUpdate(streamID, increment), "could not write window update frame")
	return f
}

func (f *framer) writeRawHeaders(t *testing.T, streamID uint32, endHeaders bool, headerFrames []byte) *framer {
	require.NoError(t, f.framer.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      streamID,
		BlockFragment: headerFrames,
		EndHeaders:    endHeaders,
	}), "could not write header frames")
	return f
}

func (f *framer) writeHeaders(t *testing.T, streamID uint32, headersFramesOptions usmhttp2.HeadersFrameOptions) *framer {
	headersFrame, err := usmhttp2.NewHeadersFrameMessage(headersFramesOptions)
	require.NoError(t, err, "could not create headers frame")

	if headersFramesOptions.EndStream {
		require.NoError(t, f.framer.WriteHeaders(http2.HeadersFrameParam{
			StreamID:      streamID,
			BlockFragment: headersFrame,
			EndHeaders:    endHeaders,
			EndStream:     true,
		}), "could not write header frames")
		return f
	}

	return f.writeRawHeaders(t, streamID, true, headersFrame)
}

func (f *framer) writeHeadersWithEncoder(t *testing.T, streamID uint32, headersFramesOptions usmhttp2.HeadersFrameOptions, encoder *hpack.Encoder, buf *bytes.Buffer) *framer {
	require.NoError(t, usmhttp2.NewHeadersFrameMessageWithEncoder(encoder, headersFramesOptions), "could not create headers frame")
	headersFrame := buf.Bytes()
	buf.Reset()
	if headersFramesOptions.EndStream {
		require.NoError(t, f.framer.WriteHeaders(http2.HeadersFrameParam{
			StreamID:      streamID,
			BlockFragment: headersFrame,
			EndHeaders:    endHeaders,
			EndStream:     true,
		}), "could not write header frames")
		return f
	}

	return f.writeRawHeaders(t, streamID, true, headersFrame)
}

type captureRange struct {
	lower int
	upper int
}

func getExpectedOutcomeForPathWithRepeatedChars() map[usmhttp.Key]captureRange {
	expected := make(map[usmhttp.Key]captureRange)
	for i := 1; i < 100; i++ {
		expected[usmhttp.Key{
			Path: usmhttp.Path{
				Content: usmhttp.Interner.GetString(fmt.Sprintf("/%s", strings.Repeat("a", i))),
			},
			Method: usmhttp.MethodPost,
		}] = captureRange{
			lower: 1,
			upper: 1,
		}
	}
	return expected
}

func startH2CServer(t *testing.T, address string, isTLS bool) func() {
	srv := &http.Server{
		Addr: authority,
		Handler: h2c.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			statusCode := testutil.StatusFromPath(r.URL.Path)
			if statusCode == 0 {
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(int(statusCode))
			}
			defer func() { _ = r.Body.Close() }()
			_, _ = io.Copy(w, r.Body)
		}), &http2.Server{}),
		IdleTimeout: 2 * time.Second,
	}

	require.NoError(t, http2.ConfigureServer(srv, nil), "could not configure server")

	l, err := net.Listen("tcp", address)
	require.NoError(t, err, "could not listen")

	if isTLS {
		cert, key, err := testutil.GetCertsPaths()
		require.NoError(t, err, "could not get certs paths")
		go func() {
			if err := srv.ServeTLS(l, cert, key); err != http.ErrServerClosed {
				require.NoError(t, err, "could not serve TLS")
			}
		}()
	} else {
		go func() {
			if err := srv.Serve(l); err != http.ErrServerClosed {
				require.NoError(t, err, "could not serve")
			}
		}()
	}

	return func() { _ = srv.Shutdown(context.Background()) }
}

func getClientsIndex(index, totalCount int) int {
	return index % totalCount
}

// validateDynamicTableMap validates that the dynamic table map contains the expected indexes.
func validateDynamicTableMap(t *testing.T, ebpfProgram *ebpfProgram, expectedDynamicTablePathIndexes []int) {
	dynamicTableMap, _, err := ebpfProgram.GetMap("http2_dynamic_table")
	require.NoError(t, err)
	resultIndexes := make([]int, 0)
	var key usmhttp2.HTTP2DynamicTableIndex
	var value usmhttp2.HTTP2DynamicTableEntry
	iterator := dynamicTableMap.Iterate()

	for iterator.Next(&key, &value) {
		resultIndexes = append(resultIndexes, int(key.Index))
	}
	sort.Ints(resultIndexes)
	require.EqualValues(t, expectedDynamicTablePathIndexes, resultIndexes)
}

// getRemainderTableMapKeys returns the keys of the remainder table map.
func getRemainderTableMapKeys(t *testing.T, ebpfProgram *ebpfProgram) []usmhttp.ConnTuple {
	remainderMap, _, err := ebpfProgram.GetMap("http2_remainder")
	require.NoError(t, err)
	resultIndexes := make([]usmhttp.ConnTuple, 0)
	var key usmhttp2.ConnTuple
	var value usmhttp2.HTTP2RemainderEntry
	iterator := remainderMap.Iterate()

	for iterator.Next(&key, &value) {
		resultIndexes = append(resultIndexes, key)
	}
	return resultIndexes
}

// validateHuffmanEncoded validates that the dynamic table map contains the expected huffman encoded paths.
func validateHuffmanEncoded(t *testing.T, ebpfProgram *ebpfProgram, expectedHuffmanEncoded map[int]bool) bool {
	dynamicTableMap, _, err := ebpfProgram.GetMap("http2_dynamic_table")
	if err != nil {
		t.Logf("could not get dynamic table map: %v", err)
		return false
	}
	resultEncodedPaths := make(map[int]bool, 0)

	var key usmhttp2.HTTP2DynamicTableIndex
	var value usmhttp2.HTTP2DynamicTableEntry
	iterator := dynamicTableMap.Iterate()
	for iterator.Next(&key, &value) {
		resultEncodedPaths[int(value.String_len)] = value.Is_huffman_encoded
	}
	// we compare the size of the path and if it is huffman encoded.
	return reflect.DeepEqual(expectedHuffmanEncoded, resultEncodedPaths)
}

// dialHTTP2Server dials the http2 server and performs the initial handshake
func dialHTTP2Server(t *testing.T) net.Conn {
	c, err := net.Dial("unix", unixPath)
	require.NoError(t, err, "failed to dial")
	t.Cleanup(func() { _ = c.Close() })

	// Writing a magic and the settings in the same packet to socket.
	require.NoError(t, writeInput(c, time.Millisecond*200, usmhttp2.ComposeMessage([]byte(http2.ClientPreface), newFramer().writeSettings(t).bytes())))
	return c
}

// getHTTP2KernelTelemetry returns the HTTP2 kernel telemetry
func getHTTP2KernelTelemetry(monitor *Monitor, isTLS bool) (*usmhttp2.HTTP2Telemetry, error) {
	http2Telemetry := &usmhttp2.HTTP2Telemetry{}
	var zero uint32

	mapName := usmhttp2.TelemetryMap
	if isTLS {
		mapName = usmhttp2.TLSTelemetryMap
	}

	mp, _, err := monitor.ebpfProgram.GetMap(mapName)
	if err != nil {
		return nil, fmt.Errorf("unable to get %q map: %s", mapName, err)
	}

	if err := mp.Lookup(unsafe.Pointer(&zero), unsafe.Pointer(http2Telemetry)); err != nil {
		return nil, fmt.Errorf("unable to lookup %q map: %s", mapName, err)
	}
	return http2Telemetry, nil
}

func dumpTelemetry(t *testing.T, usmMonitor *Monitor, isTLS bool) {
	if telemetry, err := getHTTP2KernelTelemetry(usmMonitor, isTLS); err == nil {
		tlsMarker := ""
		if isTLS {
			tlsMarker = "tls "
		}
		t.Logf("http2 eBPF %stelemetry: %v", tlsMarker, telemetry)
	}
}
