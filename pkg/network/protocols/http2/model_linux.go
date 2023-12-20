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

	"golang.org/x/net/http2/hpack"

	"github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/types"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

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

// Path returns the URL from the request fragment captured in eBPF.
func (tx *EbpfTx) Path(buffer []byte) ([]byte, bool) {
	var res []byte
	var err error
	if tx.Stream.Is_huffman_encoded {
		res, err = decodeHTTP2Path(tx.Stream.Request_path, tx.Stream.Path_size)
		if err != nil {
			log.Errorf("unable to decode HTTP2 path due to: %s", err)
			return nil, false
		}
	} else {
		if err = validatePathSize(tx.Stream.Path_size); err != nil {
			log.Errorf("path size is invalid due to: %s", err)
			return nil, false
		}

		res = tx.Stream.Request_path[:tx.Stream.Path_size]
		if err = validatePath(string(res)); err != nil {
			log.Errorf("path is invalid due to: %s", err)
			return nil, false
		}

		res = tx.Stream.Request_path[:tx.Stream.Path_size]
	}

	n := copy(buffer, res)
	return buffer[:n], true
}

// RequestLatency returns the latency of the request in nanoseconds
func (tx *EbpfTx) RequestLatency() float64 {
	if uint64(tx.Stream.Request_started) == 0 || uint64(tx.Stream.Response_last_seen) == 0 {
		return 0
	}
	return protocols.NSTimestampToFloat(tx.Stream.Response_last_seen - tx.Stream.Request_started)
}

// Incomplete returns true if the transaction contains only the request or response information
// This happens in the context of localhost with NAT, in which case we join the two parts in userspace
func (tx *EbpfTx) Incomplete() bool {
	return tx.Stream.Request_started == 0 || tx.Stream.Response_last_seen == 0 || tx.StatusCode() == 0 || tx.Stream.Path_size == 0 || tx.Method() == http.MethodUnknown
}

// ConnTuple returns the connections tuple of the transaction.
func (tx *EbpfTx) ConnTuple() types.ConnectionKey {
	return types.ConnectionKey{
		SrcIPHigh: tx.Tuple.Saddr_h,
		SrcIPLow:  tx.Tuple.Saddr_l,
		DstIPHigh: tx.Tuple.Daddr_h,
		DstIPLow:  tx.Tuple.Daddr_l,
		SrcPort:   tx.Tuple.Sport,
		DstPort:   tx.Tuple.Dport,
	}
}

// Method returns the HTTP method of the transaction.
func (tx *EbpfTx) Method() http.Method {
	switch tx.Stream.Request_method {
	case GetValue:
		return http.MethodGet
	case PostValue:
		return http.MethodPost
	default:
		return http.MethodUnknown
	}
}

// StatusCode returns the HTTP status code of the transaction.
func (tx *EbpfTx) StatusCode() uint16 {
	switch tx.Stream.Response_status_code {
	case uint16(K200Value):
		return 200
	case uint16(K204Value):
		return 204
	case uint16(K206Value):
		return 206
	case uint16(K400Value):
		return 400
	case uint16(K500Value):
		return 500
	default:
		return 0
	}
}

// SetStatusCode sets the HTTP status code of the transaction.
func (tx *EbpfTx) SetStatusCode(code uint16) {
	tx.Stream.Response_status_code = code
}

// ResponseLastSeen returns the last seen response.
func (tx *EbpfTx) ResponseLastSeen() uint64 {
	return tx.Stream.Response_last_seen
}

// SetResponseLastSeen sets the last seen response.
func (tx *EbpfTx) SetResponseLastSeen(lastSeen uint64) {
	tx.Stream.Response_last_seen = lastSeen

}

// RequestStarted returns the timestamp of the request start.
func (tx *EbpfTx) RequestStarted() uint64 {
	return tx.Stream.Request_started
}

// SetRequestMethod sets the HTTP method of the transaction.
func (tx *EbpfTx) SetRequestMethod(m http.Method) {
	tx.Stream.Request_method = uint8(m)
}

// StaticTags returns the static tags of the transaction.
func (tx *EbpfTx) StaticTags() uint64 {
	return 0
}

// DynamicTags returns the dynamic tags of the transaction.
func (tx *EbpfTx) DynamicTags() []string {
	return nil
}

// String returns a string representation of the transaction.
func (tx *EbpfTx) String() string {
	var output strings.Builder
	output.WriteString("http2.ebpfTx{")
	output.WriteString(fmt.Sprintf("[%s] [%s ⇄ %s] ", tx.family(), tx.sourceEndpoint(), tx.destEndpoint()))
	output.WriteString(" Method: '" + tx.Method().String() + "', ")
	buf := make([]byte, len(tx.Stream.Request_path))
	path, ok := tx.Path(buf)
	if ok {
		output.WriteString("Path: '" + string(path) + "'")
	}
	output.WriteString("}")
	return output.String()
}

func (tx *EbpfTx) family() ebpf.ConnFamily {
	if tx.Tuple.Metadata&uint32(ebpf.IPv6) != 0 {
		return ebpf.IPv6
	}
	return ebpf.IPv4
}

func (tx *EbpfTx) sourceAddress() util.Address {
	if tx.family() == ebpf.IPv4 {
		return util.V4Address(uint32(tx.Tuple.Saddr_l))
	}
	return util.V6Address(tx.Tuple.Saddr_l, tx.Tuple.Saddr_h)
}

func (tx *EbpfTx) sourceEndpoint() string {
	return net.JoinHostPort(tx.sourceAddress().String(), strconv.Itoa(int(tx.Tuple.Sport)))
}

func (tx *EbpfTx) destAddress() util.Address {
	if tx.family() == ebpf.IPv4 {
		return util.V4Address(uint32(tx.Tuple.Daddr_l))
	}
	return util.V6Address(tx.Tuple.Daddr_l, tx.Tuple.Daddr_h)
}

func (tx *EbpfTx) destEndpoint() string {
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

// String returns a string representation of the http2 dynamic table.
func (t http2DynamicTableEntry) String() string {
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
