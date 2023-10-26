// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package http2

import (
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
)

// Path returns the URL from the request fragment captured in eBPF.
func (tx *EbpfTx) Path(buffer []byte) ([]byte, bool) {
	if tx.Path_size == 0 || int(tx.Path_size) > len(tx.Request_path) {
		return nil, false
	}

	// trim null byte + after
	str, err := hpack.HuffmanDecodeToString(tx.Request_path[:tx.Path_size])
	if err != nil {
		return nil, false
	}

	// ensure we found a '/' in the beginning of the path
	if len(str) == 0 || str[0] != '/' {
		return nil, false
	}

	n := copy(buffer, str)
	// indicate if we knowingly captured the entire path
	return buffer[:n], true
}

// RequestLatency returns the latency of the request in nanoseconds
func (tx *EbpfTx) RequestLatency() float64 {
	if uint64(tx.Request_started) == 0 || uint64(tx.Response_last_seen) == 0 {
		return 0
	}
	return protocols.NSTimestampToFloat(tx.Response_last_seen - tx.Request_started)
}

// Incomplete returns true if the transaction contains only the request or response information
// This happens in the context of localhost with NAT, in which case we join the two parts in userspace
func (tx *EbpfTx) Incomplete() bool {
	return tx.Request_started == 0 || tx.Response_last_seen == 0 || tx.StatusCode() == 0 || tx.Path_size == 0 || tx.Method() == http.MethodUnknown
}

func (tx *EbpfTx) ConnTuple() types.ConnectionKey {
	return types.ConnectionKey{
		SrcIPHigh: tx.Tup.Saddr_h,
		SrcIPLow:  tx.Tup.Saddr_l,
		DstIPHigh: tx.Tup.Daddr_h,
		DstIPLow:  tx.Tup.Daddr_l,
		SrcPort:   tx.Tup.Sport,
		DstPort:   tx.Tup.Dport,
	}
}

func (tx *EbpfTx) Method() http.Method {
	switch tx.Request_method {
	case GetValue:
		return http.MethodGet
	case PostValue:
		return http.MethodPost
	default:
		return http.MethodUnknown
	}
}

func (tx *EbpfTx) StatusCode() uint16 {
	switch tx.Response_status_code {
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

func (tx *EbpfTx) SetStatusCode(code uint16) {
	tx.Response_status_code = code
}

func (tx *EbpfTx) ResponseLastSeen() uint64 {
	return tx.Response_last_seen
}

func (tx *EbpfTx) SetResponseLastSeen(lastSeen uint64) {
	tx.Response_last_seen = lastSeen

}
func (tx *EbpfTx) RequestStarted() uint64 {
	return tx.Request_started
}

func (tx *EbpfTx) SetRequestMethod(m http.Method) {
	tx.Request_method = uint8(m)
}

func (tx *EbpfTx) StaticTags() uint64 {
	return 0
}

func (tx *EbpfTx) DynamicTags() []string {
	return nil
}

func (tx *EbpfTx) String() string {
	var output strings.Builder
	output.WriteString("http2.ebpfTx{")
	output.WriteString("Method: '" + tx.Method().String() + "', ")
	buf := make([]byte, len(tx.Request_path))
	path, ok := tx.Path(buf)
	if ok {
		output.WriteString("Path: '" + string(path) + "'")
	}
	output.WriteString("}")
	return output.String()
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

func (t http2StreamKey) String() string {
	return fmt.Sprintf(
		"[%s] [%s ⇄ %s] (stream id %d)",
		t.family(),
		t.sourceEndpoint(),
		t.destEndpoint(),
		t.Id,
	)
}

func (t http2DynamicTableEntry) String() string {
	if t.Len == 0 {
		return ""
	}

	b := make([]byte, t.Len)
	for i := uint8(0); i < t.Len; i++ {
		b[i] = byte(t.Buffer[i])
	}
	// trim null byte + after
	str, err := hpack.HuffmanDecodeToString(b)
	if err != nil {
		return fmt.Sprintf("FAILED: %s", err)
	}

	return str
}
