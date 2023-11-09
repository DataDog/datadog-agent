// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package http

import (
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/network/protocols"

	"github.com/DataDog/datadog-agent/pkg/network/types"
)

// Path returns the URL from the request fragment captured in eBPF with
// GET variables excluded.
// Example:
// For a request fragment "GET /foo?var=bar HTTP/1.1", this method will return "/foo"
func (e *EbpfEvent) Path(buffer []byte) ([]byte, bool) {
	return computePath(buffer, e.Http.Request_fragment[:])
}

// RequestLatency returns the latency of the request in nanoseconds
func (e *EbpfEvent) RequestLatency() float64 {
	if uint64(e.Http.Request_started) == 0 || uint64(e.Http.Response_last_seen) == 0 {
		return 0
	}
	return protocols.NSTimestampToFloat(e.Http.Response_last_seen - e.Http.Request_started)
}

// Incomplete returns true if the transaction contains only the request or response information
// This happens in the context of localhost with NAT, in which case we join the two parts in userspace
func (e *EbpfEvent) Incomplete() bool {
	return e.Http.Request_started == 0 || e.Http.Response_status_code == 0
}

// ConnTuple returns a `types.ConnectionKey` for the transaction
func (e *EbpfEvent) ConnTuple() types.ConnectionKey {
	return types.ConnectionKey{
		SrcIPHigh: e.Tuple.Saddr_h,
		SrcIPLow:  e.Tuple.Saddr_l,
		DstIPHigh: e.Tuple.Daddr_h,
		DstIPLow:  e.Tuple.Daddr_l,
		SrcPort:   e.Tuple.Sport,
		DstPort:   e.Tuple.Dport,
	}
}

// Method returns the HTTP method of the HTTP transaction
func (e *EbpfEvent) Method() Method {
	return Method(e.Http.Request_method)
}

// StatusCode returns the status code of the HTTP transaction
func (e *EbpfEvent) StatusCode() uint16 {
	return e.Http.Response_status_code
}

// SetStatusCode of the underlying HTTP transaction
func (e *EbpfEvent) SetStatusCode(code uint16) {
	e.Http.Response_status_code = code
}

// ResponseLastSeen returns the timestamp of the last captured segment of the HTTP transaction
func (e *EbpfEvent) ResponseLastSeen() uint64 {
	return e.Http.Response_last_seen
}

// SetResponseLastSeen of the HTTP transaction
func (e *EbpfEvent) SetResponseLastSeen(lastSeen uint64) {
	e.Http.Response_last_seen = lastSeen
}

// RequestStarted returns the timestamp of the first segment of the HTTP transaction
func (e *EbpfEvent) RequestStarted() uint64 {
	return e.Http.Request_started
}

// SetRequestMethod of the underlying HTTP transaction
func (e *EbpfEvent) SetRequestMethod(m Method) {
	e.Http.Request_method = uint8(m)
}

// StaticTags returns an uint64 representing the tags bitfields
// Tags are defined here : pkg/network/ebpf/kprobe_types.go
func (e *EbpfEvent) StaticTags() uint64 {
	return e.Http.Tags
}

// DynamicTags returns the dynamic tags associated to the HTTP trasnaction
func (e *EbpfEvent) DynamicTags() []string {
	return nil
}

// String returns a string representation of the underlying event
func (e *EbpfEvent) String() string {
	var output strings.Builder
	output.WriteString("ebpfTx{")
	output.WriteString("Method: '" + Method(e.Http.Request_method).String() + "', ")
	output.WriteString("Tags: '0x" + strconv.FormatUint(e.Http.Tags, 16) + "', ")
	output.WriteString("Fragment: '" + hex.EncodeToString(e.Http.Request_fragment[:]) + "', ")
	output.WriteString("}")
	return output.String()
}

func requestFragment(fragment []byte) [BufferSize]byte {
	if len(fragment) >= BufferSize {
		return *(*[BufferSize]byte)(fragment)
	}
	var b [BufferSize]byte
	copy(b[:], fragment)
	return b
}
