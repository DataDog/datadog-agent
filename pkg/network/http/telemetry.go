// +build linux_bpf

package http

import (
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type telemetry struct {
	then    int64
	elapsed int64

	hits         [5]int64
	misses       int64 // this happens when we can't cope with the rate of events
	dropped      int64 // this happens when httpStatKeeper reaches capacity
	aggregations int64
}

func newTelemetry() *telemetry {
	return &telemetry{
		then: time.Now().Unix(),
	}
}

func (t *telemetry) aggregate(txs []httpTX, err error) {
	for _, tx := range txs {
		if i := tx.StatusClass()/100 - 1; i >= 0 && i < len(t.hits) {
			atomic.AddInt64(&t.hits[i], 1)
		}
	}

	if err == errLostBatch {
		atomic.AddInt64(&t.misses, int64(HTTPBatchSize))
	}
}

func (t *telemetry) reset() telemetry {
	now := time.Now()
	then := atomic.SwapInt64(&t.then, now.Unix())

	delta := telemetry{
		misses:       atomic.SwapInt64(&t.misses, 0),
		dropped:      atomic.SwapInt64(&t.dropped, 0),
		aggregations: atomic.SwapInt64(&t.aggregations, 0),
		elapsed:      now.Unix() - then,
	}

	for i := range t.hits {
		delta.hits[i] = atomic.SwapInt64(&t.hits[i], 0)
	}

	return delta
}

func (t *telemetry) report() {
	var totalRequests int64
	for _, n := range t.hits {
		totalRequests += n
	}

	log.Debugf(
		"http stats summary: requests_processed=%d(%.2f/s) requests_missed=%d(%.2f/s) requests_dropped=%d(%.2f/s) aggregations=%d",
		totalRequests,
		float64(totalRequests)/float64(t.elapsed),
		t.misses,
		float64(t.misses)/float64(t.elapsed),
		t.dropped,
		float64(t.dropped)/float64(t.elapsed),
		t.aggregations,
	)
}
