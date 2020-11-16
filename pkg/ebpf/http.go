// +build linux_bpf

package ebpf

import (
	"fmt"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/ebpf"
	"github.com/DataDog/ebpf/manager"
)

/*
#include "c/tracer-ebpf.h"
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
	return (int(tx.status_code) / 100) * 100
}

// IsDirty detects whether the batch page we're supposed to read from is still
// valid.  A "dirty" page here means that between the time the
// http_notification_t message was sent to userspace and the time we performed
// the batch lookup the page was overriden.
func (batch *httpBatch) IsDirty(notification httpNotification) bool {
	return batch.idx != notification.batch_idx
}

// GetTransactions extracts the HTTP transactions from the batch acording to the
// httpNotification received from the Kernel.
func (batch *httpBatch) GetTransactions(notif httpNotification) *[HTTPBatchSize]httpTX {
	return (*[HTTPBatchSize]httpTX)(unsafe.Pointer(&batch.txs))
}

type httpMonitor struct {
	batchMap      *ebpf.Map
	perfMap       *manager.PerfMap
	perfHandler   *bytecode.PerfHandler
	closeFilterFn func()
}

func newHTTPMonitor(config *Config, m *manager.Manager, h *bytecode.PerfHandler) (*httpMonitor, error) {
	if !config.HTTPInspection {
		log.Infof("http monitoring disabled")
		return nil, nil
	}

	filter, _ := m.GetProbe(manager.ProbeIdentificationPair{Section: string(bytecode.SocketHTTPFilter)})
	if filter == nil {
		return nil, fmt.Errorf("error retrieving socket filter")
	}

	closeFilterFn, err := network.HeadlessSocketFilter(config.ProcRoot, filter)
	if err != nil {
		return nil, fmt.Errorf("error enabling HTTP traffic inspection: %s", err)
	}

	batchMap, _, err := m.GetMap(string(bytecode.HttpBatchesMap))
	if err != nil {
		return nil, err
	}

	batchStateMap, _, err := m.GetMap(string(bytecode.HttpBatchStateMap))
	if err != nil {
		return nil, err
	}

	notificationMap, _, _ := m.GetMap(string(bytecode.HttpNotificationsMap))
	numCPUs := int(notificationMap.ABI().MaxEntries)
	batch := new(httpBatch)
	batchState := new(httpBatchState)

	for i := 0; i < numCPUs; i++ {
		batchStateMap.Put(unsafe.Pointer(&i), unsafe.Pointer(batchState))
		for j := 0; j < HTTPBatchPages; j++ {
			key := &httpBatchKey{cpu: C.uint(i), page_num: C.uint(j)}
			batchMap.Put(unsafe.Pointer(key), unsafe.Pointer(batch))
		}
	}

	pm, found := m.GetPerfMap(string(bytecode.HttpNotificationsMap))
	if !found {
		return nil, fmt.Errorf("unable to find perf map %s", bytecode.HttpNotificationsMap)
	}

	log.Infof("http monitoring enabled")
	return &httpMonitor{
		batchMap:      batchMap,
		perfMap:       pm,
		perfHandler:   h,
		closeFilterFn: closeFilterFn,
	}, nil
}

// Start consuming HTTP events
// Please note the code below is merely an *example* of how to consume the HTTP
// transaction information sent from the eBPF program
func (http *httpMonitor) Start() error {
	if http == nil {
		return nil
	}

	if err := http.perfMap.Start(); err != nil {
		return fmt.Errorf("error starting perf map: %s", err)
	}

	go func() {
		var (
			misses int
			then   = time.Now()
			hits   = make(map[int]int)
			report = time.NewTicker(30 * time.Second)
			key    = new(httpBatchKey)
		)

		for {
			select {
			case data, ok := <-http.perfHandler.ClosedChannel:
				if !ok {
					return
				}

				// The notification we read off the perf ring tells us which HTTP batch of transactions is ready to be read
				notification := toHTTPNotification(data)
				key.Prepare(notification)
				batch := new(httpBatch)
				err := http.batchMap.Lookup(unsafe.Pointer(key), unsafe.Pointer(batch))
				if err != nil {
					log.Errorf("error retrieving http batch for cpu=%d", notification.cpu)
					continue
				}

				if batch.IsDirty(notification) {
					misses++
					continue
				}

				txs := batch.GetTransactions(notification)
				// This is where we would add code handling the HTTP data (eg., generating latency percentiles etc.)
				// Right now I'm just aggregating the hits per status code just as a placeholder to make sure everything
				// is working as expected
				for _, tx := range txs {
					hits[tx.StatusClass()]++
				}
			case _, ok := <-http.perfHandler.LostChannel:
				if !ok {
					return
				}
				misses++
			case now := <-report.C:
				delta := float64(now.Sub(then).Seconds())
				log.Infof("http report: 100(%d reqs, %.2f/s) 200(%d reqs, %.2f/s) 300(%d reqs, %.2f/s), 400(%d reqs, %.2f/s) 500(%d reqs, %.2f/s), misses(%d reqs, %.2f/s)",
					hits[100], float64(hits[100])/delta,
					hits[200], float64(hits[200])/delta,
					hits[300], float64(hits[300])/delta,
					hits[400], float64(hits[400])/delta,
					hits[500], float64(hits[500])/delta,
					misses*HTTPBatchSize,
					float64(misses*HTTPBatchSize)/delta,
				)
				hits = make(map[int]int)
				then = now
				misses = 0
			}
		}
	}()

	return nil
}

func (http *httpMonitor) Stop() {
	if http == nil {
		return
	}

	http.closeFilterFn()
	_ = http.perfMap.Stop(manager.CleanAll)
	http.perfHandler.Stop()
}
