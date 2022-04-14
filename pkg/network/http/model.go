// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package http

import (
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

var (
	HTTPVerbs = map[string]struct{}{
		"OPTIONS": {},
		"GET":     {},
		"HEAD":    {},
		"POST":    {},
		"PUT":     {},
		"PATCH":   {},
		"DELETE":  {},
		"TRACE":   {},
		"CONNECT": {},
	}
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

func (tx *httpTX) validateFragment(validateHTTPVerb bool, outPath []byte) (bool, []byte) {
	b := *(*[HTTPBufferSize]byte)(unsafe.Pointer(&tx.request_fragment))

	// b might contain a null terminator in the middle
	bLen := strlen(b[:])

	var i, j int
	for i = 0; i < bLen && b[i] != ' '; i++ {
	}

	if validateHTTPVerb {
		if _, ok := HTTPVerbs[string(b[:i])]; !ok {
			return false, nil
		}
	}

	i++

	if i >= bLen || (b[i] != '/' && b[i] != '*') {
		return false, nil
	}

	if outPath != nil {
		var j int
		for j = i; j < bLen && b[j] != ' ' && b[j] != '?'; j++ {
		}

		// no bound check necessary here as we know we at least have '/' character
		n := copy(outPath, b[i:j])
		return true, outPath[:n]
	}

	return true, nil
}

// Path returns the URL from the request fragment captured in eBPF with
// GET variables excluded. The transaction must be validated with `IsValid`
// before making this call.
// Example:
// For a request fragment "GET /foo?var=bar HTTP/1.1", this method will return "/foo"
func (tx *httpTX) Path(buffer []byte) []byte {
	if ok, match := tx.validateFragment(false, buffer); ok {
		return match
	}
	return nil
}

// IsValid returns true if the transaction looks like a valid transaction
func (tx *httpTX) IsValid() bool {
	ok, _ := tx.validateFragment(true, nil)
	return ok
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

// strlen returns the length of a null-terminated string
func strlen(str []byte) int {
	for i := 0; i < len(str); i++ {
		if str[i] == 0 {
			return i
		}
	}
	return len(str)
}
