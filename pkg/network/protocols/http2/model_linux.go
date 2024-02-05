// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package http2

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/http2/hpack"

	"github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/types"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var oversizedLogLimit = util.NewLogLimit(10, time.Minute*10)

// validatePath validates the given path.
func validatePath(str string) error {
	if len(str) == 0 {
		return errors.New("decoded path is empty")
	}
	// ensure we found a '/' at the beginning of the path
	if str[0] != '/' {
		return fmt.Errorf("decoded path '%s' doesn't start with '/'", str)
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

// decodeHTTP2Path tries to decode (Huffman) the path from the given buffer.
// Possible errors:
// - If the given pathSize is 0.
// - If the given pathSize is larger than the buffer size.
// - If the Huffman decoding fails.
// - If the decoded path doesn't start with a '/'.
func decodeHTTP2Path(buf [maxHTTP2Path]byte, pathSize uint8) ([]byte, error) {
	if err := validatePathSize(pathSize); err != nil {
		return nil, err
	}

	str, err := hpack.HuffmanDecodeToString(buf[:pathSize])
	if err != nil {
		return nil, err
	}

	if err = validatePath(str); err != nil {
		return nil, err
	}

	return []byte(str), nil
}

type interestingValue[V any] struct {
	value     V
	malformed bool
}

// ebpfTXWrapper is a wrapper around the eBPF transaction.
// It extends the basic type with a pointer to an interned string, which will be filled by processHTTP2 method.
type ebpfTXWrapper struct {
	*EbpfTx
	dynamicTable *DynamicTable
	path         interestingValue[string]
	statusCode   interestingValue[uint16]
	method       interestingValue[http.Method]
}

// Path returns the URL from the request fragment captured in eBPF.
func (tx *ebpfTXWrapper) Path(buffer []byte) ([]byte, bool) {
	if tx.resolvePath() {
		n := copy(buffer, tx.path.value)
		return buffer[:n], true
	}
	return nil, false

}

// RequestLatency returns the latency of the request in nanoseconds
func (tx *ebpfTXWrapper) RequestLatency() float64 {
	if uint64(tx.Stream.Request_started) == 0 || uint64(tx.Stream.Response_last_seen) == 0 {
		return 0
	}
	return protocols.NSTimestampToFloat(tx.Stream.Response_last_seen - tx.Stream.Request_started)
}

// Incomplete returns true if the transaction contains only the request or response information
// This happens in the context of localhost with NAT, in which case we join the two parts in userspace
func (tx *ebpfTXWrapper) Incomplete() bool {
	return tx.Stream.Request_started == 0 || tx.Stream.Response_last_seen == 0 || !tx.resolveStatusCode() || !tx.resolvePath() || !tx.resolveMethod()
}

// ConnTuple returns the connections tuple of the transaction.
func (tx *ebpfTXWrapper) ConnTuple() types.ConnectionKey {
	return types.ConnectionKey{
		SrcIPHigh: tx.Tuple.Saddr_h,
		SrcIPLow:  tx.Tuple.Saddr_l,
		DstIPHigh: tx.Tuple.Daddr_h,
		DstIPLow:  tx.Tuple.Daddr_l,
		SrcPort:   tx.Tuple.Sport,
		DstPort:   tx.Tuple.Dport,
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
func (tx *ebpfTXWrapper) Method() http.Method {
	if tx.resolveMethod() {
		return tx.method.value
	}
	return http.MethodUnknown
}
func (tx *ebpfTXWrapper) resolvePath() bool {
	if tx.path.malformed {
		return false
	}
	if tx.path.value != "" {
		return true
	}

	if tx.Stream.Path.Static_table_entry > 0 {
		switch tx.Stream.Path.Static_table_entry {
		case EmptyPathValue:
			tx.path.value = "/"
		case IndexPathValue:
			tx.path.value = "/index.html"
		default:
			tx.path.malformed = true
		}
		return !tx.path.malformed
	}

	tup := tx.Tuple
	if tx.Stream.Path.Tuple_flipped {
		tup = connTuple{
			Saddr_h:  tup.Daddr_h,
			Saddr_l:  tup.Daddr_l,
			Daddr_h:  tup.Saddr_h,
			Daddr_l:  tup.Saddr_l,
			Sport:    tup.Dport,
			Dport:    tup.Sport,
			Netns:    tup.Netns,
			Pid:      tup.Pid,
			Metadata: tup.Metadata,
		}
	}
	path, exists := tx.dynamicTable.resolveValue(tup, uint64(tx.Stream.Path.Dynamic_table_entry))
	if !exists {
		return false
	}
	if err := validatePath(path); err != nil {
		if oversizedLogLimit.ShouldLog() {
			log.Warnf("path %s is invalid due to: %s", path, err)
		}
		tx.path.malformed = true
	} else {
		tx.path.value = path
	}
	return !tx.path.malformed
}

func (tx *ebpfTXWrapper) resolveStatusCode() bool {
	if tx.statusCode.malformed {
		return false
	}
	if tx.statusCode.value != 0 {
		return true
	}

	if tx.Stream.Status_code.Static_table_entry > 0 {
		switch tx.Stream.Status_code.Static_table_entry {
		case K200Value:
			tx.statusCode.value = 200
		case K204Value:
			tx.statusCode.value = 204
		case K206Value:
			tx.statusCode.value = 206
		case K400Value:
			tx.statusCode.value = 400
		case K500Value:
			tx.statusCode.value = 500
		default:
			tx.statusCode.malformed = true
		}
		return !tx.statusCode.malformed
	}

	tup := tx.Tuple
	if tx.Stream.Status_code.Tuple_flipped {
		tup = connTuple{
			Saddr_h:  tup.Daddr_h,
			Saddr_l:  tup.Daddr_l,
			Daddr_h:  tup.Saddr_h,
			Daddr_l:  tup.Saddr_l,
			Sport:    tup.Dport,
			Dport:    tup.Sport,
			Netns:    tup.Netns,
			Pid:      tup.Pid,
			Metadata: tup.Metadata,
		}
	}
	stringStatusCode, exists := tx.dynamicTable.resolveValue(tup, uint64(tx.Stream.Status_code.Dynamic_table_entry))
	if !exists {
		return false
	}
	code, err := strconv.Atoi(stringStatusCode)
	if err != nil {
		tx.statusCode.malformed = true
		return false
	}
	tx.statusCode.value = uint16(code)
	return true
}

func (tx *ebpfTXWrapper) resolveMethod() bool {
	if tx.method.malformed {
		return false
	}
	if tx.method.value != 0 {
		return true
	}

	if tx.Stream.Request_method.Static_table_entry > 0 {
		switch tx.Stream.Request_method.Static_table_entry {
		case GetValue:
			tx.method.value = http.MethodGet
		case PostValue:
			tx.method.value = http.MethodPost
		default:
			tx.method.malformed = true
		}
		return !tx.method.malformed
	}

	tup := tx.Tuple
	if tx.Stream.Request_method.Tuple_flipped {
		tup = connTuple{
			Saddr_h:  tup.Daddr_h,
			Saddr_l:  tup.Daddr_l,
			Daddr_h:  tup.Saddr_h,
			Daddr_l:  tup.Saddr_l,
			Sport:    tup.Dport,
			Dport:    tup.Sport,
			Netns:    tup.Netns,
			Pid:      tup.Pid,
			Metadata: tup.Metadata,
		}
	}
	stringMethod, exists := tx.dynamicTable.resolveValue(tup, uint64(tx.Stream.Request_method.Dynamic_table_entry))
	if !exists {
		return false
	}
	method, err := stringToHTTPMethod(stringMethod)
	if err != nil {
		tx.method.malformed = true
		return false
	}
	tx.method.value = method
	return true
}

// StatusCode returns the status code of the transaction.
// If the status code is indexed, then we return the corresponding value.
// Otherwise, f the status code is huffman encoded, then we decode it and convert it from string to int.
// Otherwise, we convert the status code from byte array to int.
func (tx *ebpfTXWrapper) StatusCode() uint16 {
	if tx.resolveStatusCode() {
		return tx.statusCode.value
	}
	return 0
}

// SetStatusCode sets the HTTP status code of the transaction.
func (tx *ebpfTXWrapper) SetStatusCode(code uint16) {
	tx.statusCode.value = code
}

// ResponseLastSeen returns the last seen response.
func (tx *ebpfTXWrapper) ResponseLastSeen() uint64 {
	return tx.Stream.Response_last_seen
}

// SetResponseLastSeen sets the last seen response.
func (tx *ebpfTXWrapper) SetResponseLastSeen(lastSeen uint64) {
	tx.Stream.Response_last_seen = lastSeen

}

// RequestStarted returns the timestamp of the request start.
func (tx *ebpfTXWrapper) RequestStarted() uint64 {
	return tx.Stream.Request_started
}

// SetRequestMethod sets the HTTP method of the transaction.
func (tx *ebpfTXWrapper) SetRequestMethod(_ http.Method) {
	// if we set Static_table_entry to be different from 0, and no indexed value, it will default to 0 which is "UNKNOWN"
	tx.Stream.Request_method.Static_table_entry = 1
}

// StaticTags returns the static tags of the transaction.
func (tx *ebpfTXWrapper) StaticTags() uint64 {
	return 0
}

// DynamicTags returns the dynamic tags of the transaction.
func (tx *ebpfTXWrapper) DynamicTags() []string {
	return nil
}

// String returns a string representation of the transaction.
func (tx *ebpfTXWrapper) String() string {
	var output strings.Builder
	output.WriteString("http2.ebpfTx{")
	output.WriteString(fmt.Sprintf("[%s] [%s ⇄ %s] ", tx.family(), tx.sourceEndpoint(), tx.destEndpoint()))
	output.WriteString(" Method: '" + tx.Method().String() + "', ")
	if tx.resolvePath() {
		output.WriteString("Path: '" + tx.path.value + "'")
	}
	output.WriteString("}")
	return output.String()
}

func (tx *ebpfTXWrapper) family() ebpf.ConnFamily {
	if tx.Tuple.Metadata&uint32(ebpf.IPv6) != 0 {
		return ebpf.IPv6
	}
	return ebpf.IPv4
}

func (tx *ebpfTXWrapper) sourceAddress() util.Address {
	if tx.family() == ebpf.IPv4 {
		return util.V4Address(uint32(tx.Tuple.Saddr_l))
	}
	return util.V6Address(tx.Tuple.Saddr_l, tx.Tuple.Saddr_h)
}

func (tx *ebpfTXWrapper) sourceEndpoint() string {
	return net.JoinHostPort(tx.sourceAddress().String(), strconv.Itoa(int(tx.Tuple.Sport)))
}

func (tx *ebpfTXWrapper) destAddress() util.Address {
	if tx.family() == ebpf.IPv4 {
		return util.V4Address(uint32(tx.Tuple.Daddr_l))
	}
	return util.V6Address(tx.Tuple.Daddr_l, tx.Tuple.Daddr_h)
}

func (tx *ebpfTXWrapper) destEndpoint() string {
	return net.JoinHostPort(tx.destAddress().String(), strconv.Itoa(int(tx.Tuple.Dport)))
}

func (t http2StreamKey) family() ebpf.ConnFamily {
	if t.Tup.Metadata&uint32(ebpf.IPv6) != 0 {
		return ebpf.IPv6
	}
	return ebpf.IPv4
}

func (t http2StreamKey) sourceAddress() util.Address {
	if t.family() == ebpf.IPv4 {
		return util.V4Address(uint32(t.Tup.Saddr_l))
	}
	return util.V6Address(t.Tup.Saddr_l, t.Tup.Saddr_h)
}

func (t http2StreamKey) sourceEndpoint() string {
	return net.JoinHostPort(t.sourceAddress().String(), strconv.Itoa(int(t.Tup.Sport)))
}

func (t http2StreamKey) destAddress() util.Address {
	if t.family() == ebpf.IPv4 {
		return util.V4Address(uint32(t.Tup.Daddr_l))
	}
	return util.V6Address(t.Tup.Daddr_l, t.Tup.Daddr_h)
}

func (t http2StreamKey) destEndpoint() string {
	return net.JoinHostPort(t.destAddress().String(), strconv.Itoa(int(t.Tup.Dport)))
}

// String returns a string representation of the http2 stream key.
func (t http2StreamKey) String() string {
	return fmt.Sprintf(
		"[%s] [%s ⇄ %s] (stream id %d)",
		t.family(),
		t.sourceEndpoint(),
		t.destEndpoint(),
		t.Id,
	)
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
		t.Exceeding_max_frames_to_filter, t.Exceeding_max_interesting_frames, t.Path_size_bucket[0], t.Path_size_bucket[1],
		t.Path_size_bucket[2], t.Path_size_bucket[3], t.Path_size_bucket[4], t.Path_size_bucket[5], t.Path_size_bucket[6],
		t.Path_size_bucket[7])
}
