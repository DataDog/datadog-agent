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
)

type httpTX C.http_transaction_t
type httpNotification C.http_batch_notification_t
type httpBatch C.http_batch_t
type httpBatchKey C.http_batch_key_t
type httpBatchState C.http_batch_state_t

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

// IsDirty detects whether the batch page we're supposed to read from is still
// valid.  A "dirty" page here means that between the time the
// http_notification_t message was sent to userspace and the time we performed
// the batch lookup the page was overridden.
func (batch *httpBatch) IsDirty(notification httpNotification) bool {
	return batch.idx != notification.batch_idx
}

// GetTransactions extracts the HTTP transactions from the batch according to the
// httpNotification received from the Kernel.
func (batch *httpBatch) GetTransactions(notif httpNotification) *[HTTPBatchSize]httpTX {
	return (*[HTTPBatchSize]httpTX)(unsafe.Pointer(&batch.txs))
}
