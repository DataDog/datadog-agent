// +build linux_bpf

package http

import (
	"unsafe"
)

/*
#include "../ebpf/c/tracer.h"
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

// Path returns the URL from the request fragment captured in eBPF with
// GET variables excluded.
// Example:
// For a request fragment "GET /foo?var=bar HTTP/1.1", this method will return "/foo"
func (tx *httpTX) Path(buffer []byte) []byte {
	b := *(*[HTTPBufferSize]byte)(unsafe.Pointer(&tx.request_fragment))

	var i, j int
	for i = 0; i < len(b) && b[i] != ' '; i++ {
	}

	i++

	for j = i; j < len(b) && b[j] != ' ' && b[j] != '?'; j++ {
	}

	if i < j && j <= len(b) {
		n := copy(buffer, b[i:j])
		return buffer[:n]
	}

	return nil
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
