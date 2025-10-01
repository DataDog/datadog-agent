// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package http2

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/http2/hpack"

	"github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/types"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type eventWrapper struct {
	EbpfTx
	dt *DynamicTable
}

var oversizedLogLimit = log.NewLogLimit(10, time.Minute*10)

// validatePath validates the given path.
func validatePath(str []byte) error {
	if len(str) == 0 {
		return errors.New("decoded path is empty")
	}
	// ensure we found a '/' at the beginning of the path
	if str[0] != '/' {
		return fmt.Errorf("decoded path (%#v) doesn't start with '/'", str)
	}
	return nil
}

// validatePathSize validates the given path size.
func validatePathSize(size uint8) error {
	if size == 0 {
		return errors.New("empty path")
	}
	if size > maxHTTP2Path {
		return fmt.Errorf("path size has exceeded the maximum limit: %d", size)
	}
	return nil
}

// Buffer pool to be used for decoding HTTP2 paths.
// This is used to avoid allocating a new buffer for each path decoding.
var bufPool = sync.Pool{
	New: func() interface{} { return new(bytes.Buffer) },
}

// decodeHTTP2Path tries to decode (Huffman) the path from the given buffer.
// Possible errors:
// - If the given pathSize is 0.
// - If the given pathSize is larger than the buffer size.
// - If the Huffman decoding fails.
// - If the decoded path doesn't start with a '/'.
func decodeHTTP2Path(buf [maxHTTP2Path]byte, pathSize uint8, output []byte) ([]byte, error) {
	if err := validatePathSize(pathSize); err != nil {
		return nil, err
	}

	tmpBuffer := bufPool.Get().(*bytes.Buffer)
	tmpBuffer.Reset()
	defer bufPool.Put(tmpBuffer)

	n, err := hpack.HuffmanDecode(tmpBuffer, buf[:pathSize])
	if err != nil {
		return nil, err
	}

	if err = validatePath(tmpBuffer.Bytes()); err != nil {
		return nil, err
	}

	if n > len(output) {
		n = len(output)
	}
	copy(output[:n], tmpBuffer.Bytes())
	return output[:n], nil
}

// Path returns the URL from the request fragment captured in eBPF.
func (ev *eventWrapper) Path(buffer []byte) ([]byte, bool) {
	if ev.EbpfTx.Stream.Path.Static_table_index != 0 {
		value, ok := pathStaticTable[ev.EbpfTx.Stream.Path.Static_table_index]
		if !ok {
			return nil, false
		}
		return []byte(value.Get()), true
	}

	value, ok := ev.dt.resolveDynamicEntry(HTTP2DynamicTableIndex{
		Index: ev.EbpfTx.Stream.Path.Dynamic_table_index,
		Tup:   ev.Tuple,
	})
	if !ok {
		if oversizedLogLimit.ShouldLog() {
			log.Warn("unknown path key")
		}
		return nil, false
	}

	strValue := value.Get()
	n := len(strValue)
	if n > len(buffer) {
		n = len(buffer)
	}
	copy(buffer[:n], strValue)
	return buffer, true
}

// RequestLatency returns the latency of the request in nanoseconds
func (ev *eventWrapper) RequestLatency() float64 {
	if ev.EbpfTx.Stream.Request_started == 0 || ev.EbpfTx.Stream.Response_last_seen == 0 {
		return 0
	}
	if ev.EbpfTx.Stream.Response_last_seen < ev.EbpfTx.Stream.Request_started {
		return 0
	}
	return protocols.NSTimestampToFloat(ev.EbpfTx.Stream.Response_last_seen - ev.EbpfTx.Stream.Request_started)
}

// Incomplete returns true if the transaction contains only the request or response information
// This happens in the context of localhost with NAT, in which case we join the two parts in userspace
func (ev *eventWrapper) Incomplete() bool {
	return ev.EbpfTx.Stream.Request_started == 0 || ev.EbpfTx.Stream.Response_last_seen == 0 || ev.StatusCode() == 0 || !ev.EbpfTx.Stream.Path.Finalized || ev.Method() == http.MethodUnknown
}

// ConnTuple returns the connections tuple of the transaction.
func (ev *eventWrapper) ConnTuple() types.ConnectionKey {
	return types.ConnectionKey{
		SrcIPHigh: ev.EbpfTx.Tuple.Saddr_h,
		SrcIPLow:  ev.EbpfTx.Tuple.Saddr_l,
		DstIPHigh: ev.EbpfTx.Tuple.Daddr_h,
		DstIPLow:  ev.EbpfTx.Tuple.Daddr_l,
		SrcPort:   ev.EbpfTx.Tuple.Sport,
		DstPort:   ev.EbpfTx.Tuple.Dport,
	}
}

// stringToHTTPMethod converts a string to an HTTP method.
func stringToHTTPMethod(method string) (http.Method, error) {
	switch strings.ToUpper(method) {
	case "PUT":
		return http.MethodPut, nil
	case "DELETE":
		return http.MethodDelete, nil
	case "HEAD":
		return http.MethodHead, nil
	case "OPTIONS":
		return http.MethodOptions, nil
	case "PATCH":
		return http.MethodPatch, nil
	case "GET":
		return http.MethodGet, nil
	case "POST":
		return http.MethodPost, nil
	// Currently unsupported methods due to lack of support in http.Method.
	case "CONNECT":
		return http.MethodUnknown, nil
	case "TRACE":
		return http.MethodUnknown, nil
	default:
		return 0, fmt.Errorf("unsupported HTTP method: %s", method)
	}
}

// Method returns the HTTP method of the transaction.
func (ev *eventWrapper) Method() http.Method {
	// Case which the method is indexed.
	if ev.EbpfTx.Stream.Request_method.Static_table_index != 0 {
		value, ok := methodStaticTable[ev.EbpfTx.Stream.Request_method.Static_table_index]
		if ok {
			return value
		}
		return http.MethodUnknown
	}

	value, ok := ev.dt.resolveDynamicEntry(HTTP2DynamicTableIndex{
		Index: ev.EbpfTx.Stream.Request_method.Dynamic_table_index,
		Tup:   ev.Tuple,
	})
	if !ok {
		if oversizedLogLimit.ShouldLog() {
			log.Warn("unknown method key")
		}
		return http.MethodUnknown
	}
	http2Method, err := stringToHTTPMethod(value.Get())
	if err != nil {
		return http.MethodUnknown
	}
	return http2Method
}

// StatusCode returns the status code of the transaction.
// If the status code is indexed, then we return the corresponding value.
// Otherwise, f the status code is huffman encoded, then we decode it and convert it from string to int.
// Otherwise, we convert the status code from byte array to int.
func (ev *eventWrapper) StatusCode() uint16 {
	if ev.EbpfTx.Stream.Status_code.Static_table_index != 0 {
		value, ok := statusStaticTable[ev.EbpfTx.Stream.Status_code.Static_table_index]
		if ok {
			return value
		}
		return 0
	}

	// TODO: Should be integer as well
	value, ok := ev.dt.resolveDynamicEntry(HTTP2DynamicTableIndex{
		Index: ev.EbpfTx.Stream.Status_code.Dynamic_table_index,
		Tup:   ev.Tuple,
	})
	if !ok {
		if oversizedLogLimit.ShouldLog() {
			log.Warn("unknown status code key")
		}
		return 0
	}

	code, err := strconv.Atoi(value.Get())
	if err != nil {
		return 0
	}
	return uint16(code)
}

// SetStatusCode sets the HTTP status code of the transaction.
func (ev *eventWrapper) SetStatusCode(code uint16) {
	val := strconv.Itoa(int(code))
	if len(val) > http2RawStatusCodeMaxLength {
		return
	}
	// copy(ev.EbpfTx.Stream.Status_code.Raw_buffer[:], val)
}

// ResponseLastSeen returns the last seen response.
func (ev *eventWrapper) ResponseLastSeen() uint64 {
	return ev.EbpfTx.Stream.Response_last_seen
}

// SetResponseLastSeen sets the last seen response.
func (ev *eventWrapper) SetResponseLastSeen(lastSeen uint64) {
	ev.EbpfTx.Stream.Response_last_seen = lastSeen

}

// RequestStarted returns the timestamp of the request start.
func (ev *eventWrapper) RequestStarted() uint64 {
	return ev.EbpfTx.Stream.Request_started
}

// SetRequestMethod sets the HTTP method of the transaction.
func (ev *eventWrapper) SetRequestMethod(_ http.Method) {
	// if we set Static_table_index to be different from 0, and no indexed value, it will default to 0 which is "UNKNOWN"
	ev.EbpfTx.Stream.Request_method.Static_table_index = 1
}

// StaticTags returns the static tags of the transaction.
func (ev *eventWrapper) StaticTags() uint64 {
	return uint64(ev.EbpfTx.Stream.Tags)
}

// DynamicTags returns the dynamic tags of the transaction.
func (ev *eventWrapper) DynamicTags() []string {
	return nil
}

// String returns a string representation of the transaction.
func (ev *eventWrapper) String() string {
	var output strings.Builder
	output.WriteString("http2.ebpfTx{")
	output.WriteString(fmt.Sprintf("[%s] [%s ⇄ %s] ", ev.family(), ev.sourceEndpoint(), ev.destEndpoint()))
	output.WriteString(" Method: '" + ev.Method().String() + "', ")
	// fullBufferSize := len(ev.EbpfTx.Stream.Path.Raw_buffer)
	// if ev.EbpfTx.Stream.Path.Is_huffman_encoded {
	// If the path is huffman encoded, then the path is compressed (with an upper bound to compressed size of maxHTTP2Path)
	// thus, we need more room for the decompressed path, therefore using 2*maxHTTP2Path.
	// fullBufferSize = 2 * maxHTTP2Path
	// }
	// buf := make([]byte, fullBufferSize)
	// path, ok := ev.Path(buf)
	// if ok {
	// 	output.WriteString("Path: '" + string(path) + "'")
	// }
	output.WriteString("}")
	return output.String()
}

func (ev *eventWrapper) family() ebpf.ConnFamily {
	if ev.EbpfTx.Tuple.Metadata&uint32(ebpf.IPv6) != 0 {
		return ebpf.IPv6
	}
	return ebpf.IPv4
}

func (ev *eventWrapper) sourceAddress() util.Address {
	if ev.family() == ebpf.IPv4 {
		return util.V4Address(uint32(ev.EbpfTx.Tuple.Saddr_l))
	}
	return util.V6Address(ev.EbpfTx.Tuple.Saddr_l, ev.EbpfTx.Tuple.Saddr_h)
}

func (ev *eventWrapper) sourceEndpoint() string {
	return net.JoinHostPort(ev.sourceAddress().String(), strconv.Itoa(int(ev.EbpfTx.Tuple.Sport)))
}

func (ev *eventWrapper) destAddress() util.Address {
	if ev.family() == ebpf.IPv4 {
		return util.V4Address(uint32(ev.EbpfTx.Tuple.Daddr_l))
	}
	return util.V6Address(ev.EbpfTx.Tuple.Daddr_l, ev.EbpfTx.Tuple.Daddr_h)
}

func (ev *eventWrapper) destEndpoint() string {
	return net.JoinHostPort(ev.destAddress().String(), strconv.Itoa(int(ev.EbpfTx.Tuple.Dport)))
}

func (t HTTP2StreamKey) family() ebpf.ConnFamily {
	if t.Tup.Metadata&uint32(ebpf.IPv6) != 0 {
		return ebpf.IPv6
	}
	return ebpf.IPv4
}

func (t HTTP2StreamKey) sourceAddress() util.Address {
	if t.family() == ebpf.IPv4 {
		return util.V4Address(uint32(t.Tup.Saddr_l))
	}
	return util.V6Address(t.Tup.Saddr_l, t.Tup.Saddr_h)
}

func (t HTTP2StreamKey) sourceEndpoint() string {
	return net.JoinHostPort(t.sourceAddress().String(), strconv.Itoa(int(t.Tup.Sport)))
}

func (t HTTP2StreamKey) destAddress() util.Address {
	if t.family() == ebpf.IPv4 {
		return util.V4Address(uint32(t.Tup.Daddr_l))
	}
	return util.V6Address(t.Tup.Daddr_l, t.Tup.Daddr_h)
}

func (t HTTP2StreamKey) destEndpoint() string {
	return net.JoinHostPort(t.destAddress().String(), strconv.Itoa(int(t.Tup.Dport)))
}

// String returns a string representation of the http2 stream key.
func (t HTTP2StreamKey) String() string {
	return fmt.Sprintf(
		"[%s] [%s ⇄ %s] (stream id %d)",
		t.family(),
		t.sourceEndpoint(),
		t.destEndpoint(),
		t.Id,
	)
}

// String returns a string representation of the http2 dynamic table.
func (t HTTP2DynamicTableEntry) String() string {
	if t.String_len == 0 {
		return ""
	}

	b := make([]byte, t.String_len)
	for i := uint8(0); i < t.String_len; i++ {
		b[i] = byte(t.Buffer[i])
	}
	// trim null byte + after
	str, err := hpack.HuffmanDecodeToString(b)
	if err != nil {
		return fmt.Sprintf("FAILED: %s", err)
	}

	return str
}

// String returns a string representation of the http2 eBPF telemetry.
func (t *HTTP2Telemetry) String() string {
	return fmt.Sprintf(`
HTTP2Telemetry{
	"requests seen": %d,
	"responses seen": %d,
	"end of stream seen": %d,
	"reset frames seen": %d,
	"literal values exceed message count": %d,
	"messages with more frames than we can filter": %d,
	"messages with more interesting frames than we can process": %d,
	"continuation frames" : %d,
	"path headers length distribution": {
		"in range [0, 120)": %d,
		"in range [120, 130)": %d,
		"in range [130, 140)": %d,
		"in range [140, 150)": %d,
		"in range [150, 160)": %d,
		"in range [160, 170)": %d,
		"in range [170, 180)": %d,
		"in range [180, infinity)": %d
	}
}`, t.Request_seen, t.Response_seen, t.End_of_stream, t.End_of_stream_rst, t.Literal_value_exceeds_frame,
		t.Exceeding_max_frames_to_filter, t.Exceeding_max_interesting_frames, t.Continuation_frames,
		t.Path_size_bucket[0], t.Path_size_bucket[1], t.Path_size_bucket[2], t.Path_size_bucket[3],
		t.Path_size_bucket[4], t.Path_size_bucket[5], t.Path_size_bucket[6], t.Path_size_bucket[7])
}
