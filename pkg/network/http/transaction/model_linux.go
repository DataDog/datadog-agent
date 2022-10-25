// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package transaction

import (
	"encoding/hex"
	"errors"
	"strings"
	"unsafe"
)

/*
#include "../../ebpf/c/http-types.h"
*/
import "C"

// export to match windows definition
var ErrLostBatch = errors.New("http batch lost (not consumed fast enough)")

// Path returns the URL from the request fragment captured in eBPF with
// GET variables excluded.
// Example:
// For a request fragment "GET /foo?var=bar HTTP/1.1", this method will return "/foo"
func (tx *EbpfHttpTx) Path(buffer []byte) ([]byte, bool) {
	b := *(*[HTTPBufferSize]byte)(unsafe.Pointer(&tx.Request_fragment))

	// b might contain a null terminator in the middle
	bLen := strlen(b[:])

	var i, j int
	for i = 0; i < bLen && b[i] != ' '; i++ {
	}

	i++

	if i >= bLen || (b[i] != '/' && b[i] != '*') {
		return nil, false
	}

	for j = i; j < bLen && b[j] != ' ' && b[j] != '?'; j++ {
	}

	// no bound check necessary here as we know we at least have '/' character
	n := copy(buffer, b[i:j])
	fullPath := j < bLen || (j == HTTPBufferSize-1 && b[j] == ' ')
	return buffer[:n], fullPath
}

// StatusClass returns an integer representing the status code class
// Example: a 404 would return 400
func (tx *EbpfHttpTx) StatusClass() int {
	return (int(tx.Response_status_code) / 100) * 100
}

// RequestLatency returns the latency of the request in nanoseconds
func (tx *EbpfHttpTx) RequestLatency() float64 {
	if uint64(tx.Request_started) == 0 || uint64(tx.Response_last_seen) == 0 {
		return 0
	}
	return nsTimestampToFloat(uint64(tx.Response_last_seen - tx.Request_started))
}

// Incomplete returns true if the transaction contains only the request or response information
// This happens in the context of localhost with NAT, in which case we join the two parts in userspace
func (tx *EbpfHttpTx) Incomplete() bool {
	return tx.Request_started == 0 || tx.Response_status_code == 0
}

func (tx *EbpfHttpTx) ReqFragment() []byte {
	asslice := (*[1 << 30]byte)(unsafe.Pointer(&tx.Request_fragment))[:int(HTTPBufferSize):int(HTTPBufferSize)]
	return asslice
}

func (tx *EbpfHttpTx) isIPV4() bool {
	return true
}

func (tx *EbpfHttpTx) Method() Method {
	return Method(tx.Request_method)
}

func (tx *EbpfHttpTx) StatusCode() uint16 {
	return uint16(tx.Response_status_code)
}

func (tx *EbpfHttpTx) SetStatusCode(code uint16) {
	tx.Response_status_code = code
}

func (tx *EbpfHttpTx) ResponseLastSeen() uint64 {
	return uint64(tx.Response_last_seen)
}

func (tx *EbpfHttpTx) SetResponseLastSeen(lastSeen uint64) {
	tx.Response_last_seen = uint64(lastSeen)

}
func (tx *EbpfHttpTx) RequestStarted() uint64 {
	return uint64(tx.Request_started)
}

func (tx *EbpfHttpTx) RequestMethod() uint32 {
	return uint32(tx.Request_method)
}

func (tx *EbpfHttpTx) SetRequestMethod(m uint32) {
	tx.Request_method = uint8(m)
}

// Tags returns an uint64 representing the tags bitfields
// Tags are defined here : pkg/network/ebpf/kprobe_types.go
func (tx *EbpfHttpTx) StaticTags() uint64 {
	return uint64(tx.Tags)
}

func (tx *EbpfHttpTx) DynamicTags() []string {
	return nil
}

func (tx *EbpfHttpTx) String() string {
	var output strings.Builder
	fragment := *(*[HTTPBufferSize]byte)(unsafe.Pointer(&tx.Request_fragment))
	output.WriteString("ebpf.EbpfHttpTx{")
	output.WriteString("Method: '" + Method(tx.Request_method).String() + "', ")
	output.WriteString("Fragment: '" + hex.EncodeToString(fragment[:]) + "', ")
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

func RequestFragment(fragment []byte) [HTTPBufferSize]byte {
	var b [HTTPBufferSize]byte
	for i := 0; i < len(b) && i < len(fragment); i++ {
		b[i] = byte(fragment[i])
	}
	return b
}

func (tx *EbpfHttpTx) NewKey(path string, fullPath bool) Key {
	return Key{
		KeyTuple: KeyTuple{
			SrcIPHigh: uint64(tx.Tup.Saddr_h),
			SrcIPLow:  uint64(tx.Tup.Saddr_l),
			SrcPort:   uint16(tx.Tup.Sport),
			DstIPHigh: uint64(tx.Tup.Daddr_h),
			DstIPLow:  uint64(tx.Tup.Daddr_l),
			DstPort:   uint16(tx.Tup.Dport),
		},
		Path: Path{
			Content:  path,
			FullPath: fullPath,
		},
		Method: tx.Method(),
	}
}
func (tx *EbpfHttpTx) NewKeyTuple() KeyTuple {
	return KeyTuple{
		SrcIPHigh: uint64(tx.Tup.Saddr_h),
		SrcIPLow:  uint64(tx.Tup.Saddr_l),
		SrcPort:   uint16(tx.Tup.Sport),
	}
}
