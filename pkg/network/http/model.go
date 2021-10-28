// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package http

import (
	"encoding/binary"
	"fmt"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/process/util"
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

type httpTX C.http_transaction_t
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

// ReqFragment returns a byte slice containing the first HTTPBufferSize bytes of the request
func (tx *httpTX) ReqFragment() []byte {
	b := *(*[HTTPBufferSize]byte)(unsafe.Pointer(&tx.request_fragment))
	return b[:]
}

// StatusClass returns an integer representing the status code class
// Example: a 404 would return 400
func (tx *httpTX) StatusClass() int {
	return (int(tx.response_status_code) / 100) * 100
}

// RequestLatency returns the latency of the request in nanoseconds
func (tx *httpTX) RequestLatency() float64 {
	return nsTimestampToFloat(uint64(tx.response_last_seen - tx.request_started))
}

// Incomplete returns true if the transaction contains only the request or response information
// This happens in the context of localhost with NAT, in which case we join the two parts in userspace
func (tx *httpTX) Incomplete() bool {
	return tx.request_started == 0 || tx.response_status_code == 0
}

func (tx *httpTX) SrcIPHigh() uint64 {
	return uint64(tx.tup.saddr_h)
}

func (tx *httpTX) SrcIPLow() uint64 {
	return uint64(tx.tup.saddr_l)
}

func (tx *httpTX) SrcPort() uint16 {
	return uint16(tx.tup.sport)
}

func (tx *httpTX) DstIPHigh() uint64 {
	return uint64(tx.tup.daddr_h)
}

func (tx *httpTX) DstIPLow() uint64 {
	return uint64(tx.tup.daddr_l)
}

func (tx *httpTX) DstPort() uint16 {
	return uint16(tx.tup.dport)
}

func (tx *httpTX) Method() Method {
	return Method(tx.request_method)
}

func (tx *httpTX) StatusCode() uint16 {
	return uint16(tx.response_status_code)
}

// Tags returns an uint64 representing the tags bitfields
// Tags are defined here : pkg/network/ebpf/kprobe_types.go
func (tx *httpTX) Tags() uint64 {
	return uint64(tx.tags)
}

// IsDirty detects whether the batch page we're supposed to read from is still
// valid.  A "dirty" page here means that between the time the
// http_notification_t message was sent to userspace and the time we performed
// the batch lookup the page was overridden.
func (batch *httpBatch) IsDirty(notification httpNotification) bool {
	return batch.idx != notification.batch_idx
}

// Transactions returns the slice of HTTP transactions embedded in the batch
func (batch *httpBatch) Transactions() []httpTX {
	return (*(*[HTTPBatchSize]httpTX)(unsafe.Pointer(&batch.txs)))[:]
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

func generateIPv4HTTPTransaction(source util.Address, dest util.Address, sourcePort int, destPort int, path string, code int, latency time.Duration) httpTX {
	var tx httpTX

	reqFragment := fmt.Sprintf("GET %s HTTP/1.1\nHost: example.com\nUser-Agent: example-browser/1.0", path)
	latencyNS := C.ulonglong(uint64(latency))

	tx.request_started = 1
	tx.response_last_seen = tx.request_started + latencyNS
	tx.response_status_code = C.ushort(code)
	for i := 0; i < len(tx.request_fragment) && i < len(reqFragment); i++ {
		tx.request_fragment[i] = C.char(reqFragment[i])
	}
	tx.tup.saddr_l = C.ulonglong(binary.LittleEndian.Uint32(source.Bytes()))
	tx.tup.sport = C.ushort(sourcePort)
	tx.tup.daddr_l = C.ulonglong(binary.LittleEndian.Uint32(dest.Bytes()))
	tx.tup.dport = C.ushort(destPort)

	return tx
}
