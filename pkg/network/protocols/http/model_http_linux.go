// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package http

import (
	"bytes"
	"encoding/hex"
	"strconv"
	"strings"
)

// Path returns the URL from the request fragment captured in eBPF with
// GET variables excluded.
// Example:
// For a request fragment "GET /foo?var=bar HTTP/1.1", this method will return "/foo"
func (tx *ebpfHttpTx) Path(buffer []byte) ([]byte, bool) {
	bLen := bytes.IndexByte(tx.Request_fragment[:], 0)
	if bLen == -1 {
		bLen = len(tx.Request_fragment)
	}
	// trim null byte + after
	b := tx.Request_fragment[:bLen]
	// find first space after request method
	i := bytes.IndexByte(b, ' ')
	i++
	// ensure we found a space, it isn't at the end, and the next chars are '/' or '*'
	if i == 0 || i == len(b) || (b[i] != '/' && b[i] != '*') {
		return nil, false
	}
	// trim to start of path
	b = b[i:]
	// capture until we find the slice end, a space, or a question mark (we ignore the query parameters)
	var j int
	for j = 0; j < len(b) && b[j] != ' ' && b[j] != '?'; j++ {
	}
	n := copy(buffer, b[:j])
	// indicate if we knowingly captured the entire path
	fullPath := n < len(b)
	return buffer[:n], fullPath
}

// RequestLatency returns the latency of the request in nanoseconds
func (tx *ebpfHttpTx) RequestLatency() float64 {
	if uint64(tx.Request_started) == 0 || uint64(tx.Response_last_seen) == 0 {
		return 0
	}
	return nsTimestampToFloat(tx.Response_last_seen - tx.Request_started)
}

// Incomplete returns true if the transaction contains only the request or response information
// This happens in the context of localhost with NAT, in which case we join the two parts in userspace
func (tx *ebpfHttpTx) Incomplete() bool {
	return tx.Request_started == 0 || tx.Response_status_code == 0
}

func (tx *ebpfHttpTx) ConnTuple() KeyTuple {
	return KeyTuple{
		SrcIPHigh: tx.Tup.Saddr_h,
		SrcIPLow:  tx.Tup.Saddr_l,
		DstIPHigh: tx.Tup.Daddr_h,
		DstIPLow:  tx.Tup.Daddr_l,
		SrcPort:   tx.Tup.Sport,
		DstPort:   tx.Tup.Dport,
	}
}

func (tx *ebpfHttpTx) Method() Method {
	return Method(tx.Request_method)
}

func (tx *ebpfHttpTx) StatusCode() uint16 {
	return tx.Response_status_code
}

func (tx *ebpfHttpTx) SetStatusCode(code uint16) {
	tx.Response_status_code = code
}

func (tx *ebpfHttpTx) ResponseLastSeen() uint64 {
	return tx.Response_last_seen
}

func (tx *ebpfHttpTx) SetResponseLastSeen(lastSeen uint64) {
	tx.Response_last_seen = lastSeen

}
func (tx *ebpfHttpTx) RequestStarted() uint64 {
	return tx.Request_started
}

func (tx *ebpfHttpTx) SetRequestMethod(m Method) {
	tx.Request_method = uint8(m)
}

// StaticTags returns an uint64 representing the tags bitfields
// Tags are defined here : pkg/network/ebpf/kprobe_types.go
func (tx *ebpfHttpTx) StaticTags() uint64 {
	return tx.Tags
}

func (tx *ebpfHttpTx) DynamicTags() []string {
	return nil
}

func (tx *ebpfHttpTx) String() string {
	var output strings.Builder
	output.WriteString("ebpfHttpTx{")
	output.WriteString("Method: '" + Method(tx.Request_method).String() + "', ")
	output.WriteString("Tags: '0x" + strconv.FormatUint(tx.Tags, 16) + "', ")
	output.WriteString("Fragment: '" + hex.EncodeToString(tx.Request_fragment[:]) + "', ")
	output.WriteString("}")
	return output.String()
}

// below is copied from pkg/trace/stats/statsraw.go
// 10 bits precision (any value will be +/- 1/1024)
const roundMask uint64 = 1 << 10

// nsTimestampToFloat converts a nanosec timestamp into a float nanosecond timestamp truncated to a fixed precision
func nsTimestampToFloat(ns uint64) float64 {
	var shift uint
	for ns > roundMask {
		ns = ns >> 1
		shift++
	}
	return float64(ns << shift)
}

func requestFragment(fragment []byte) [HTTPBufferSize]byte {
	if len(fragment) >= HTTPBufferSize {
		return *(*[HTTPBufferSize]byte)(fragment)
	}
	var b [HTTPBufferSize]byte
	copy(b[:], fragment)
	return b
}
