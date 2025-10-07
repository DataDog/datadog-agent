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
	"github.com/DataDog/datadog-agent/pkg/util/intern"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var oversizedLogLimit = log.NewLogLimit(10, time.Minute*10)
var interner = intern.NewStringInterner()

// EventWrapper wraps an ebpf event and provides additional methods to extract information from it.
// We use this wrapper to avoid recomputing the same values (path/method/status code) multiple times.
type EventWrapper struct {
	*EbpfTx

	pathSet bool
	path    *intern.StringValue

	methodSet bool
	method    http.Method

	statusCodeSet bool
	statusCode    uint16
}

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
func (ew *EventWrapper) Path(buffer []byte) ([]byte, bool) {
	if ew.pathSet {
		n := copy(buffer, ew.path.Get())
		return buffer[:n], true
	}

	var err error

	if ew.Stream.Path.Static_table_entry != 0 {
		value, ok := pathStaticTable[ew.Stream.Path.Static_table_entry]
		if !ok {
			return nil, false
		}
		ew.path = value
		ew.pathSet = true
		n := copy(buffer, ew.path.Get())
		return buffer[:n], true
	}

	if ew.Stream.Path.Is_huffman_encoded {
		buffer, err = decodeHTTP2Path(ew.Stream.Path.Raw_buffer, ew.Stream.Path.Length, buffer)
		if err != nil {
			if oversizedLogLimit.ShouldLog() {
				log.Warnf("unable to decode HTTP2 path (%#v) due to: %s", ew.Stream.Path.Raw_buffer[:ew.Stream.Path.Length], err)
			}
			return nil, false
		}
	} else {
		if ew.Stream.Path.Length == 0 {
			if oversizedLogLimit.ShouldLog() {
				log.Warn("path size: 0 is invalid")
			}
			return nil, false
		} else if int(ew.Stream.Path.Length) > len(ew.Stream.Path.Raw_buffer) {
			if oversizedLogLimit.ShouldLog() {
				log.Warnf("Truncating as path size: %d is greater than the buffer size: %d", ew.Stream.Path.Length, len(buffer))
			}
			ew.Stream.Path.Length = uint8(len(ew.Stream.Path.Raw_buffer))
		}
		n := copy(buffer, ew.Stream.Path.Raw_buffer[:ew.Stream.Path.Length])
		// Truncating exceeding nulls.
		buffer = buffer[:n]
		if err = validatePath(buffer); err != nil {
			if oversizedLogLimit.ShouldLog() {
				// The error already contains the path, so we don't need to log it again.
				log.Warn(err)
			}
			return nil, false
		}
	}

	// Ignore query parameters
	queryStart := bytes.IndexByte(buffer, byte('?'))
	if queryStart == -1 {
		queryStart = len(buffer)
	}
	ew.path = interner.Get(buffer[:queryStart])
	ew.pathSet = true
	return buffer[:queryStart], true
}

// RequestLatency returns the latency of the request in nanoseconds
func (ew *EventWrapper) RequestLatency() float64 {
	if ew.Stream.Request_started == 0 || ew.Stream.Response_last_seen == 0 {
		return 0
	}
	if ew.Stream.Response_last_seen < ew.Stream.Request_started {
		return 0
	}
	return protocols.NSTimestampToFloat(ew.Stream.Response_last_seen - ew.Stream.Request_started)
}

// Incomplete returns true if the transaction contains only the request or response information
// This happens in the context of localhost with NAT, in which case we join the two parts in userspace
func (ew *EventWrapper) Incomplete() bool {
	return ew.Stream.Request_started == 0 || ew.Stream.Response_last_seen == 0 || ew.StatusCode() == 0 || !ew.Stream.Path.Finalized || ew.Method() == http.MethodUnknown
}

// ConnTuple returns the connections tuple of the transaction.
func (ew *EventWrapper) ConnTuple() types.ConnectionKey {
	return types.ConnectionKey{
		SrcIPHigh: ew.Tuple.Saddr_h,
		SrcIPLow:  ew.Tuple.Saddr_l,
		DstIPHigh: ew.Tuple.Daddr_h,
		DstIPLow:  ew.Tuple.Daddr_l,
		SrcPort:   ew.Tuple.Sport,
		DstPort:   ew.Tuple.Dport,
	}
}

var (
	unsupportedMethods = map[http.Method]struct{}{
		http.MethodConnect: {},
		http.MethodTrace:   {},
	}
)

// Method returns the HTTP method of the transaction.
func (ew *EventWrapper) Method() http.Method {
	var method string
	var err error

	if ew.methodSet {
		return ew.method
	}

	if ew.Stream.Request_method.Static_table_entry != 0 {
		value, ok := methodStaticTable[ew.Stream.Request_method.Static_table_entry]
		if ok {
			ew.SetRequestMethod(value)
			return value
		}
		return http.MethodUnknown
	}

	// if the length of the method is greater than the buffer, then we return 0.
	if int(ew.Stream.Request_method.Length) > len(ew.Stream.Request_method.Raw_buffer) || ew.Stream.Request_method.Length == 0 {
		if oversizedLogLimit.ShouldLog() {
			log.Warnf("method length %d is longer than the size buffer: %v and is huffman encoded: %v",
				ew.Stream.Request_method.Length, ew.Stream.Request_method.Raw_buffer, ew.Stream.Request_method.Is_huffman_encoded)
		}
		return http.MethodUnknown
	}

	// Case which the method is literal.
	if ew.Stream.Request_method.Is_huffman_encoded {
		method, err = hpack.HuffmanDecodeToString(ew.Stream.Request_method.Raw_buffer[:ew.Stream.Request_method.Length])
		if err != nil {
			return http.MethodUnknown
		}
	} else {
		method = string(ew.Stream.Request_method.Raw_buffer[:ew.Stream.Request_method.Length])
	}

	http2Method, ok := http.StringToMethod[strings.ToUpper(method)]
	if !ok {
		return http.MethodUnknown
	}
	if _, exists := unsupportedMethods[http2Method]; exists {
		return http.MethodUnknown
	}

	ew.SetRequestMethod(http2Method)
	return ew.method
}

// StatusCode returns the status code of the transaction.
// If the status code is indexed, then we return the corresponding value.
// Otherwise, f the status code is huffman encoded, then we decode it and convert it from string to int.
// Otherwise, we convert the status code from byte array to int.
func (ew *EventWrapper) StatusCode() uint16 {
	if ew.statusCodeSet {
		return ew.statusCode
	}

	if ew.Stream.Status_code.Static_table_entry != 0 {
		value, ok := statusStaticTable[ew.Stream.Status_code.Static_table_entry]
		if ok {
			ew.SetStatusCode(value)
			return value
		}
		return 0
	}

	if ew.Stream.Status_code.Is_huffman_encoded {
		// The final form of the status code is 3 characters.
		statusCode, err := hpack.HuffmanDecodeToString(ew.Stream.Status_code.Raw_buffer[:http2RawStatusCodeMaxLength-1])
		if err != nil {
			return 0
		}
		code, err := strconv.Atoi(statusCode)
		if err != nil {
			return 0
		}

		ew.SetStatusCode(uint16(code))
		return ew.statusCode
	}

	code, err := strconv.Atoi(string(ew.Stream.Status_code.Raw_buffer[:]))
	if err != nil {
		return 0
	}
	ew.SetStatusCode(uint16(code))
	return ew.statusCode
}

// SetStatusCode sets the HTTP status code of the transaction.
func (ew *EventWrapper) SetStatusCode(code uint16) {
	ew.statusCode = code
	ew.statusCodeSet = true
}

// ResponseLastSeen returns the last seen response.
func (ew *EventWrapper) ResponseLastSeen() uint64 {
	return ew.Stream.Response_last_seen
}

// SetResponseLastSeen sets the last seen response.
func (ew *EventWrapper) SetResponseLastSeen(lastSeen uint64) {
	ew.Stream.Response_last_seen = lastSeen

}

// RequestStarted returns the timestamp of the request start.
func (ew *EventWrapper) RequestStarted() uint64 {
	return ew.Stream.Request_started
}

// SetRequestMethod sets the HTTP method of the transaction.
func (ew *EventWrapper) SetRequestMethod(method http.Method) {
	ew.method = method
	ew.methodSet = true
}

// StaticTags returns the static tags of the transaction.
func (ew *EventWrapper) StaticTags() uint64 {
	return uint64(ew.Stream.Tags)
}

// DynamicTags returns the dynamic tags of the transaction.
func (ew *EventWrapper) DynamicTags() []string {
	return nil
}

// String returns a string representation of the transaction.
func (ew *EventWrapper) String() string {
	var output strings.Builder
	output.WriteString("http2.ebpfTx{")
	output.WriteString(fmt.Sprintf("[%s] [%s ⇄ %s] ", ew.family(), ew.sourceEndpoint(), ew.destEndpoint()))
	output.WriteString(" Method: '" + ew.Method().String() + "', ")
	fullBufferSize := len(ew.Stream.Path.Raw_buffer)
	if ew.Stream.Path.Is_huffman_encoded {
		// If the path is huffman encoded, then the path is compressed (with an upper bound to compressed size of maxHTTP2Path)
		// thus, we need more room for the decompressed path, therefore using 2*maxHTTP2Path.
		fullBufferSize = 2 * maxHTTP2Path
	}
	buf := make([]byte, fullBufferSize)
	path, ok := ew.Path(buf)
	if ok {
		output.WriteString("Path: '" + string(path) + "'")
	}
	output.WriteString("}")
	return output.String()
}

func (ew *EventWrapper) family() ebpf.ConnFamily {
	if ew.Tuple.Metadata&uint32(ebpf.IPv6) != 0 {
		return ebpf.IPv6
	}
	return ebpf.IPv4
}

func (ew *EventWrapper) sourceAddress() util.Address {
	if ew.family() == ebpf.IPv4 {
		return util.V4Address(uint32(ew.Tuple.Saddr_l))
	}
	return util.V6Address(ew.Tuple.Saddr_l, ew.Tuple.Saddr_h)
}

func (ew *EventWrapper) sourceEndpoint() string {
	return net.JoinHostPort(ew.sourceAddress().String(), strconv.Itoa(int(ew.Tuple.Sport)))
}

func (ew *EventWrapper) destAddress() util.Address {
	if ew.family() == ebpf.IPv4 {
		return util.V4Address(uint32(ew.Tuple.Daddr_l))
	}
	return util.V6Address(ew.Tuple.Daddr_l, ew.Tuple.Daddr_h)
}

func (ew *EventWrapper) destEndpoint() string {
	return net.JoinHostPort(ew.destAddress().String(), strconv.Itoa(int(ew.Tuple.Dport)))
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
