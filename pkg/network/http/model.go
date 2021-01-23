// +build linux_bpf

package http

import (
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

/*
#include "../ebpf/c/tracer.h"
*/
import "C"

const (
	HTTPBatchSize  = int(C.HTTP_BATCH_SIZE)
	HTTPBatchPages = int(C.HTTP_BATCH_PAGES)
)

type httpTX C.http_transaction_t
type httpNotification C.http_batch_notification_t
type httpBatch C.http_batch_t
type httpBatchKey C.http_batch_key_t

const (
	CONN_V4 uint = 0 << 0
	CONN_V6 uint = 1 << 1
)

func toHTTPNotification(data []byte) httpNotification {
	return *(*httpNotification)(unsafe.Pointer(&data[0]))
}

// Prepare the httpBatchKey for a map lookup
func (k *httpBatchKey) Prepare(n httpNotification) {
	k.cpu = n.cpu
	k.page_num = C.uint(int(n.batch_idx) % HTTPBatchPages)
}

// Path returns the URL from the request fragment captured in eBPF
// Usually the request fragment will look like
// GET /foo HTTP/1.1
func (tx *httpTX) Path() string {
	b := C.GoBytes(unsafe.Pointer(&tx.request_fragment), C.int(C.HTTP_BUFFER_SIZE))

	var i, j int
	for i = 0; i < len(b) && b[i] != ' '; i++ {
	}

	i++

	for j = i; j < len(b) && b[j] != ' '; j++ {
	}

	if i < j && j <= len(b) {
		return string(b[i:j])
	}

	return ""
}

// StatusClass returns an integer representing the status code class
// Example: a 404 would return 400
func (tx *httpTX) StatusClass() int {
	return (int(tx.response_status_code) / 100) * 100
}

// Method returns a string representing the HTTP method of the request
func (tx *httpTX) Method() string {
	switch tx.request_method {
	case C.HTTP_GET:
		return "GET"
	case C.HTTP_POST:
		return "POST"
	case C.HTTP_PUT:
		return "PUT"
	case C.HTTP_HEAD:
		return "HEAD"
	case C.HTTP_DELETE:
		return "DELETE"
	case C.HTTP_OPTIONS:
		return "OPTIONS"
	case C.HTTP_PATCH:
		return "PATCH"
	default:
		return ""
	}
}

func (tx *httpTX) SourceIP() util.Address {
	// Second bit of metadata indicates if the connection is V6 (1) or V4 (0)
	metadata := uint(tx.tup.metadata)
	if metadata&CONN_V6 == 1 {
		return util.V6Address(uint64(tx.tup.saddr_l), uint64(tx.tup.saddr_h))
	}
	return util.V4Address(uint32(tx.tup.saddr_l))
}

func (tx *httpTX) DestIP() util.Address {
	// Second bit of metadata indicates if the connection is V6 (1) or V4 (0)
	metadata := uint(tx.tup.metadata)
	if metadata&CONN_V6 == 1 {
		return util.V6Address(uint64(tx.tup.daddr_l), uint64(tx.tup.daddr_h))
	}
	return util.V4Address(uint32(tx.tup.daddr_l))
}

func (tx *httpTX) SourcePort() uint16 {
	return uint16(tx.tup.sport)
}

func (tx *httpTX) DestPort() uint16 {
	return uint16(tx.tup.dport)
}

// RequestLatency returns the latency of the request in ms
func (tx *httpTX) RequestLatency() float64 {
	return float64((tx.response_last_seen - tx.request_started) / (1000000))
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
