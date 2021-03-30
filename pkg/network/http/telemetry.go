// +build linux_bpf

package http

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type telemetry struct {
	then   int64
	hits   [5]int64
	misses int64 // this happens when we can't cope with the rate of events
	drops  int64 // this happens when httpStatKeeper reaches capacity
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

func (t *telemetry) dropped(count int) {
	atomic.AddInt64(&t.drops, int64(count))
}

func (t *telemetry) get() (time.Time, map[string]int64) {
	var (
		now        = time.Now()
		then       = atomic.SwapInt64(&t.then, now.Unix())
		misses     = atomic.SwapInt64(&t.misses, 0)
		drops      = atomic.SwapInt64(&t.drops, 0)
		data       = make(map[string]int64)
		elapsed    = now.Unix() - then
		totalCount = int64(0)
	)

	if elapsed == 0 {
		return now, nil
	}

	for i := range t.hits {
		count := atomic.SwapInt64(&t.hits[i], 0)
		totalCount += count

		data[fmt.Sprintf("%dXX_request_count", i+1)] = count
		data[fmt.Sprintf("%dXX_request_rate", i+1)] = count / elapsed
	}

	data["requests_dropped_count"] = drops
	data["requests_dropped_rate"] = drops / elapsed
	data["requests_missed_count"] = misses
	data["requests_missed_rate"] = misses / elapsed

	log.Debugf(
		"http stats summary. requests_processed=%d(%.2f/s) requests_missed=%d(%.2f/s) requests_dropped=%d(%.2f/s)",
		totalCount,
		float64(totalCount)/float64(elapsed),
		misses,
		float64(misses)/float64(elapsed),
		drops,
		float64(drops)/float64(elapsed),
	)

	return now, data
}
