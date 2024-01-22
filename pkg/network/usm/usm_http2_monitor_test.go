// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"bytes"
	"encoding/binary"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"

	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	usmhttp2 "github.com/DataDog/datadog-agent/pkg/network/protocols/http2"
	gotlsutils "github.com/DataDog/datadog-agent/pkg/network/protocols/tls/gotls/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/testutil/proxy"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

const (
	srvPort  = 8082
	unixPath = "/tmp/transparent.sock"
)

var (
	authority = net.JoinHostPort("127.0.0.1", strconv.Itoa(srvPort))
)

type usmHTTP2Suite struct {
	suite.Suite
	isTLS bool
}

func TestHTTP2Scenarios(t *testing.T) {
	currKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	if currKernelVersion < usmhttp2.MinimumKernelVersion {
		t.Skipf("HTTP2 monitoring can not run on kernel before %v", usmhttp2.MinimumKernelVersion)
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
				suite.Run(t, &usmHTTP2Suite{isTLS: tc.isTLS})
			})
		}
	})
}

func (s *usmHTTP2Suite) TestRawTraffic() {
	t := s.T()
	cfg := config.New()
	cfg.EnableHTTP2Monitoring = true
	cfg.EnableGoTLSSupport = s.isTLS
	cfg.GoTLSExcludeSelf = s.isTLS

	// Start local server
	cleanup, err := startH2CServer(authority, s.isTLS)
	require.NoError(t, err)
	t.Cleanup(cleanup)

	// Start the proxy server.
	proxyProcess, cancel := proxy.NewExternalUnixTransparentProxyServer(t, unixPath, authority, s.isTLS)
	t.Cleanup(cancel)
	require.NoError(t, proxy.WaitForConnectionReady(unixPath))

	getTLSNumber := func(numberWithoutTLS, numberWithTLS int, isTLS bool) int {
		if isTLS {
			return numberWithTLS
		}
		return numberWithoutTLS
	}

	tests := []struct {
		name              string
		skip              bool
		messageBuilder    func() []byte
		expectedEndpoints map[http.Key]int
	}{
		{
			name: "parse_frames tail call using 1 program",
			// The objective of this test is to verify that we accurately perform the parsing of frames within
			// a single program.
			messageBuilder: func() []byte {
				const settingsFramesCount = 100
				framer := newFramer()
				return framer.
					writeMultiMessage(t, settingsFramesCount, framer.writeSettings).
					writeHeaders(t, getStreamID(1), testHeaders(), false, false).
					writeData(t, getStreamID(1), true, []byte{}).
					Bytes()
			},
			expectedEndpoints: map[http.Key]int{
				{
					Path:   http.Path{Content: http.Interner.GetString("/aaa")},
					Method: http.MethodPost,
				}: 1,
			},
		},
		{
			name: "parse_frames tail call using 2 programs",
			// The purpose of this test is to validate that when we surpass the limit of HTTP2_MAX_FRAMES_ITERATIONS,
			// the filtering of subsequent frames will continue using tail calls.
			messageBuilder: func() []byte {
				const settingsFramesCount = 130
				framer := newFramer()
				return framer.
					writeMultiMessage(t, settingsFramesCount, framer.writeSettings).
					writeHeaders(t, getStreamID(1), testHeaders(), false, false).
					writeData(t, getStreamID(1), true, []byte{}).
					Bytes()
			},
			expectedEndpoints: map[http.Key]int{
				{
					Path:   http.Path{Content: http.Interner.GetString("/aaa")},
					Method: http.MethodPost,
				}: 1,
			},
			// Currently we don't have a way to test it as TLS version does not have the ability to run 2 tail calls
			// for filtering frames (will be fixed in USMON-684)
			skip: s.isTLS,
		},
		{
			name: "validate frames_filter tail calls limit",
			// The purpose of this test is to validate that when we surpass the limit of HTTP2_MAX_TAIL_CALLS_FOR_FRAMES_FILTER,
			// for 2 filter_frames we do not use more than two tail calls.
			messageBuilder: func() []byte {
				settingsFramesCount := getTLSNumber(241, 121, s.isTLS)
				framer := newFramer()
				return framer.
					writeMultiMessage(t, settingsFramesCount, framer.writeSettings).
					writeHeaders(t, getStreamID(1), testHeaders(), false, false).
					writeData(t, getStreamID(1), true, []byte{}).
					Bytes()
			},
			expectedEndpoints: nil,
		},
		{
			name: "validate max interesting frames limit",
			// The purpose of this test is to verify our ability to reach the limit set by HTTP2_MAX_FRAMES_ITERATIONS, which
			// determines the maximum number of "interesting frames" we can process.
			messageBuilder: func() []byte {
				iterations := getTLSNumber(120, 60, s.isTLS)
				framer := newFramer()
				for i := 0; i < iterations; i++ {
					streamID := getStreamID(i)
					framer.
						writeHeaders(t, streamID, testHeaders(), false, false).
						writeData(t, streamID, true, []byte{})
				}
				return framer.Bytes()
			},
			expectedEndpoints: map[http.Key]int{
				{
					Path:   http.Path{Content: http.Interner.GetString("/aaa")},
					Method: http.MethodPost,
				}: getTLSNumber(120, 60, s.isTLS),
			},
		},
		{
			name: "validate literal header field without indexing",
			// The purpose of this test is to verify our ability the case:
			// Literal Header Field without Indexing (0b0000xxxx: top four bits are 0000)
			// https://httpwg.org/specs/rfc7541.html#rfc.section.C.2.2
			messageBuilder: func() []byte {
				const iterations = 5
				const setDynamicTableSize = true
				framer := newFramer()
				for i := 0; i < iterations; i++ {
					streamID := getStreamID(i)
					framer.
						writeHeaders(t, streamID, headersWithoutIndexingPath(), false, setDynamicTableSize).
						writeData(t, streamID, true, []byte{})
				}
				return framer.Bytes()
			},
			expectedEndpoints: map[http.Key]int{
				{
					Path:   http.Path{Content: http.Interner.GetString("/" + strings.Repeat("a", usmhttp2.DynamicTableSize))},
					Method: http.MethodPost,
				}: 5,
			},
		},
		{
			name: "validate literal header field never indexed",
			// The purpose of this test is to verify our ability the case:
			// Literal Header Field never Indexed (0b0001xxxx: top four bits are 0001)
			// https://httpwg.org/specs/rfc7541.html#rfc.section.6.2.3
			messageBuilder: func() []byte {
				const iterations = 5
				framer := newFramer()
				for i := 0; i < iterations; i++ {
					streamID := getStreamID(i)
					framer.
						writeHeaders(t, streamID, headersWithNeverIndexedPath(), false, false).
						writeData(t, streamID, true, []byte{})
				}
				return framer.Bytes()

			},
			expectedEndpoints: map[http.Key]int{
				{
					Path:   http.Path{Content: http.Interner.GetString("/aaa")},
					Method: http.MethodPost,
				}: 5,
			},
		},
		{
			name: "validate path with index 4",
			// The purpose of this test is to verify our ability to identify paths with index 4.
			messageBuilder: func() []byte {
				const iterations = 5
				// pathHeaderField is the hex representation of the path /aaa with index 4.
				pathHeaderField := []byte{0x44, 0x83, 0x60, 0x63, 0x1f}
				headerFields := removeHeaderFieldByKey(testHeaders(), ":path")
				headersFrame, err := usmhttp2.NewHeadersFrameMessage(headerFields)
				require.NoError(t, err, "could not create headers frame")

				// we are adding the path header field with index 4, we need to do it on the byte slice and not on the headerFields
				// due to the fact that when we create a header field it would be with index 5.
				headersFrame = append(pathHeaderField, headersFrame...)
				framer := newFramer()
				for i := 0; i < iterations; i++ {
					streamID := getStreamID(i)
					framer.
						writeHeaders(t, streamID, testHeaders(), false, false).
						writeData(t, streamID, true, []byte{})
				}
				return framer.Bytes()
			},
			expectedEndpoints: map[http.Key]int{
				{
					Path:   http.Path{Content: http.Interner.GetString("/aaa")},
					Method: http.MethodPost,
				}: 5,
			},
		},
		{
			name: "validate PING and WINDOWS_UPDATE frames between HEADERS and DATA",
			// The purpose of this test is to verify our ability to process PING and WINDOWS_UPDATE frames between HEADERS and DATA.
			messageBuilder: func() []byte {
				const iterations = 5
				framer := newFramer()

				for i := 0; i < iterations; i++ {
					streamID := getStreamID(i)
					framer.
						writeHeaders(t, streamID, testHeaders(), false, false).
						writePing(t).
						writeWindowUpdate(t, streamID, 1).
						writeData(t, streamID, true, []byte{})
				}

				return framer.Bytes()
			},
			expectedEndpoints: map[http.Key]int{
				{
					Path:   http.Path{Content: http.Interner.GetString("/aaa")},
					Method: http.MethodPost,
				}: 5,
			},
		},
		{
			name: "validate RST_STREAM cancel err code",
			// The purpose of this test is to validate that when a cancel error code is sent, we will not count the request.
			// We are sending 10 requests, and 5 of them will contain RST_STREAM with a cancel error code.Therefore, we expect to
			// capture five valid requests.
			messageBuilder: func() []byte {
				const iterations = 10
				rstFramesCount := 5
				framer := newFramer()
				for i := 0; i < iterations; i++ {
					streamID := getStreamID(i)
					framer.
						writeHeaders(t, streamID, testHeaders(), false, false).
						writeData(t, streamID, true, []byte{})
					if rstFramesCount > 0 {
						framer.writeRSTStream(t, streamID, http2.ErrCodeCancel)
						rstFramesCount--
					}
				}
				return framer.Bytes()
			},
			expectedEndpoints: map[http.Key]int{
				{
					Path:   http.Path{Content: http.Interner.GetString("/aaa")},
					Method: http.MethodPost,
				}: 5,
			},
		},
		{
			name: "validate RST_STREAM before server status ok",
			// The purpose of this test is to validate that when we see RST before DATA frame with status ok,
			// we will not count the requests.
			messageBuilder: func() []byte {
				const iterations = 10
				framer := newFramer()
				for i := 0; i < iterations; i++ {
					streamID := getStreamID(i)
					framer.
						writeHeaders(t, streamID, testHeaders(), false, false).
						writeData(t, streamID, true, []byte{}).writeRSTStream(t, streamID, http2.ErrCodeNo)
				}
				return framer.Bytes()
			},
			expectedEndpoints: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip {
				t.Skip("skipping test")
			}

			usmMonitor, err := NewMonitor(cfg, nil, nil, nil)
			require.NoError(t, err)
			require.NoError(t, usmMonitor.Start())
			t.Cleanup(usmMonitor.Stop)
			t.Cleanup(utils.ResetDebugger)
			if s.isTLS {
				utils.WaitForProgramsToBeTraced(t, "go-tls", proxyProcess.Process.Pid)
			}

			c, err := net.Dial("unix", unixPath)
			require.NoError(t, err, "could not dial")
			defer c.Close()

			// Create a buffer to write the frame into.
			var buf bytes.Buffer
			framer := http2.NewFramer(&buf, nil)
			// Write the empty SettingsFrame to the buffer using the Framer
			require.NoError(t, framer.WriteSettings(http2.Setting{}), "could not write settings frame")

			// Writing a magic and the settings in the same packet to socket.
			require.NoError(t, writeInput(c, usmhttp2.ComposeMessage([]byte(http2.ClientPreface), buf.Bytes()), time.Second))

			// Composing a message with the number of setting frames we want to send.
			require.NoError(t, writeInput(c, tt.messageBuilder(), time.Second))

			res := make(map[http.Key]int)
			assert.Eventually(t, func() bool {
				stats := usmMonitor.GetProtocolStats()
				http2Stats, ok := stats[protocols.HTTP2]
				if !ok {
					return false
				}
				http2StatsTyped := http2Stats.(map[http.Key]*http.RequestStats)
				for key, stat := range http2StatsTyped {
					if key.DstPort == srvPort || key.SrcPort == srvPort {
						count := stat.Data[200].Count
						newKey := http.Key{
							Path:   http.Path{Content: key.Path.Content},
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

				for key, endpointCount := range res {
					_, ok := tt.expectedEndpoints[key]
					if !ok {
						return false
					}
					if endpointCount != tt.expectedEndpoints[key] {
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
				ebpftest.DumpMapsTestHelper(t, usmMonitor.DumpMaps, "http2_in_flight")
			}
		})
	}
}

// writeInput writes the given input to the socket and reads the response.
// Presently, the timeout is configured to one second for all readings.
// In case of encountered issues, increasing this duration might be necessary.
func writeInput(c net.Conn, input []byte, timeout time.Duration) error {
	_, err := c.Write(input)
	if err != nil {
		return err
	}
	frame := make([]byte, 9)
	// Since we don't know when to stop reading from the socket, we set a timeout.
	c.SetReadDeadline(time.Now().Add(timeout))
	for {
		// Read the frame header.
		_, err := c.Read(frame)
		if err != nil {
			// we want to stop reading from the socket when we encounter an i/o timeout.
			if strings.Contains(err.Error(), "i/o timeout") {
				return nil
			}
			return err
		}
		// Calculate frame length.
		frameLength := int(binary.BigEndian.Uint32(append([]byte{0}, frame[:3]...)))
		if frameLength == 0 {
			continue
		}
		// Read the frame payload.
		payload := make([]byte, frameLength)
		_, err = c.Read(payload)
		if err != nil {
			// we want to stop reading from the socket when we encounter an i/o timeout.
			if strings.Contains(err.Error(), "i/o timeout") {
				return nil
			}
			return err
		}
	}
}

// headersWithNeverIndexedPath returns a set of header fields with path that never indexed.
func headersWithNeverIndexedPath() []hpack.HeaderField { return generateTestHeaderFields(true, false) }

// headersWithoutIndexingPath returns a set of header fields with path without-indexing.
func headersWithoutIndexingPath() []hpack.HeaderField { return generateTestHeaderFields(false, true) }

// testHeaders returns a set of header fields.
func testHeaders() []hpack.HeaderField { return generateTestHeaderFields(false, false) }

// generateTestHeaderFields generates a set of header fields that will be used for the tests.
func generateTestHeaderFields(pathNeverIndexed, withoutIndexing bool) []hpack.HeaderField {
	pathHeaderField := hpack.HeaderField{Name: ":path", Value: "/aaa", Sensitive: false}

	// If we want to create a case without indexing, we need to make sure that the path is longer than 100 characters.
	// The indexing is determined by the dynamic table size (which we set to dynamicTableSize) and the size of the path.
	// ref: https://github.com/golang/net/blob/07e05fd6e95ab445ebe48840c81a027dbace3b8e/http2/hpack/encode.go#L140
	// Therefore, we want to make sure that the path is longer or equal to 100 characters so that the path will not be indexed.
	if withoutIndexing {
		pathHeaderField = hpack.HeaderField{Name: ":path", Value: "/" + strings.Repeat("a", usmhttp2.DynamicTableSize), Sensitive: true}
	}

	// If the path is sensitive, we are in a case where a path is never indexed
	if pathNeverIndexed {
		pathHeaderField = hpack.HeaderField{Name: ":path", Value: "/aaa", Sensitive: true}
	}

	return []hpack.HeaderField{
		{Name: ":authority", Value: authority},
		{Name: ":method", Value: "POST"},
		pathHeaderField,
		{Name: ":scheme", Value: "http"},
		{Name: "content-type", Value: "application/json"},
		{Name: "content-length", Value: "4"},
		{Name: "accept-encoding", Value: "gzip"},
		{Name: "user-agent", Value: "Go-http-client/2.0"},
	}
}

// removeHeaderFieldByKey removes the header field with the given key from the given header fields.
func removeHeaderFieldByKey(headerFields []hpack.HeaderField, keyToRemove string) []hpack.HeaderField {
	for i := 0; i < len(headerFields); i++ {
		if headerFields[i].Name == keyToRemove {
			// Remove the element by modifying the slice in-place
			headerFields = append(headerFields[:i], headerFields[i+1:]...)
			break
		}
	}

	return headerFields
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

func (f *framer) Bytes() []byte {
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

func (f *framer) writeWindowUpdate(t *testing.T, streamID uint32, increment uint32) *framer {
	require.NoError(t, f.framer.WriteWindowUpdate(streamID, increment), "could not write window update frame")
	return f
}

func (f *framer) writeHeaders(t *testing.T, streamID uint32, headerFields []hpack.HeaderField, endStream, setDynamicTableSize bool) *framer {
	headersFrame, err := usmhttp2.NewHeadersFrameMessage(headerFields, setDynamicTableSize)
	require.NoError(t, err, "could not create headers frame")

	require.NoError(t, f.framer.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      streamID,
		BlockFragment: headersFrame,
		EndStream:     endStream,
		EndHeaders:    true,
	}), "could not write header frames")
	return f
}
