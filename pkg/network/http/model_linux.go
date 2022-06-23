// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package http

import (
	"encoding/hex"
	"strings"
	"unsafe"
)

/*
#include "../ebpf/c/http-types.h"
*/
import "C"

const (
	HTTPBatchSize  = int(C.HTTP_BATCH_SIZE)
	HTTPBatchPages = int(C.HTTP_BATCH_PAGES)
	HTTPBufferSize = int(C.HTTP_BUFFER_SIZE)
)

type ebpfHttpTx C.http_transaction_t
type httpNotification C.http_batch_notification_t
type httpBatch C.http_batch_t
type httpBatchKey C.http_batch_key_t

func toHTTPNotification(data []byte) httpNotification {
	return *(*httpNotification)(unsafe.Pointer(&data[0]))
}

// Prepare the httpBatchKey for a map lookup
func (k *httpBatchKey) Prepare(n httpNotification) {
	k.cpu = n.cpu
	k.page_num = C.uint(int(n.batch_idx) % HTTPBatchPages)
}

// Path returns the URL from the request fragment captured in eBPF with
// GET variables excluded.
// Example:
// For a request fragment "GET /foo?var=bar HTTP/1.1", this method will return "/foo"
func (tx *ebpfHttpTx) Path(buffer []byte) ([]byte, bool) {
	b := *(*[HTTPBufferSize]byte)(unsafe.Pointer(&tx.request_fragment))

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
func (tx *ebpfHttpTx) StatusClass() int {
	return (int(tx.response_status_code) / 100) * 100
}

// RequestLatency returns the latency of the request in nanoseconds
func (tx *ebpfHttpTx) RequestLatency() float64 {
	if uint64(tx.request_started) == 0 || uint64(tx.response_last_seen) == 0 {
		return 0
	}
	return nsTimestampToFloat(uint64(tx.response_last_seen - tx.request_started))
}

// Incomplete returns true if the transaction contains only the request or response information
// This happens in the context of localhost with NAT, in which case we join the two parts in userspace
func (tx *ebpfHttpTx) Incomplete() bool {
	return tx.request_started == 0 || tx.response_status_code == 0
}

func (tx *ebpfHttpTx) ReqFragment() []byte {
	asslice := (*[1 << 30]byte)(unsafe.Pointer(&tx.request_fragment))[:int(C.HTTP_BUFFER_SIZE):int(C.HTTP_BUFFER_SIZE)]
	return asslice
}

func (tx *ebpfHttpTx) isIPV4() bool {
	return true
}

func (tx *ebpfHttpTx) SrcIPHigh() uint64 {
	return uint64(tx.tup.saddr_h)
}

func (tx *ebpfHttpTx) SrcIPLow() uint64 {
	return uint64(tx.tup.saddr_l)
}

func (tx *ebpfHttpTx) SrcPort() uint16 {
	return uint16(tx.tup.sport)
}

func (tx *ebpfHttpTx) DstIPHigh() uint64 {
	return uint64(tx.tup.daddr_h)
}

func (tx *ebpfHttpTx) DstIPLow() uint64 {
	return uint64(tx.tup.daddr_l)
}

func (tx *ebpfHttpTx) DstPort() uint16 {
	return uint16(tx.tup.dport)
}

func (tx *ebpfHttpTx) Method() Method {
	return Method(tx.request_method)
}

func (tx *ebpfHttpTx) StatusCode() uint16 {
	return uint16(tx.response_status_code)
}

func (tx *ebpfHttpTx) SetStatusCode(code uint16) {
	tx.response_status_code = C.ushort(code)
}

func (tx *ebpfHttpTx) ResponseLastSeen() uint64 {
	return uint64(tx.response_last_seen)
}

func (tx *ebpfHttpTx) SetResponseLastSeen(lastSeen uint64) {
	tx.response_last_seen = C.ulonglong(lastSeen)

}
func (tx *ebpfHttpTx) RequestStarted() uint64 {
	return uint64(tx.request_started)
}

func (tx *ebpfHttpTx) RequestMethod() uint32 {
	return uint32(tx.request_method)
}

func (tx *ebpfHttpTx) SetRequestMethod(m uint32) {
	tx.request_method = m
}

// Tags returns an uint64 representing the tags bitfields
// Tags are defined here : pkg/network/ebpf/kprobe_types.go
func (tx *ebpfHttpTx) StaticTags() uint64 {
	return uint64(tx.tags)
}

func (tx *ebpfHttpTx) DynamicTags() []string {
	return nil
}

func (tx *ebpfHttpTx) String() string {
	var output strings.Builder
	fragment := *(*[HTTPBufferSize]byte)(unsafe.Pointer(&tx.request_fragment))
	output.WriteString("ebpfHttpTx{")
	output.WriteString("Method: '" + Method(tx.request_method).String() + "', ")
	output.WriteString("Fragment: '" + hex.EncodeToString(fragment[:]) + "', ")
	output.WriteString("}")
	return output.String()
}

// IsDirty detects whether the batch page we're supposed to read from is still
// valid.  A "dirty" page here means that between the time the
// http_notification_t message was sent to userspace and the time we performed
// the batch lookup the page was overridden.
func (batch *httpBatch) IsDirty(notification httpNotification) bool {
	return batch.idx != notification.batch_idx
}

// Transactions returns the slice of HTTP transactions embedded in the batch
func (batch *httpBatch) Transactions() []ebpfHttpTx {
	return (*(*[HTTPBatchSize]ebpfHttpTx)(unsafe.Pointer(&batch.txs)))[:]
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
