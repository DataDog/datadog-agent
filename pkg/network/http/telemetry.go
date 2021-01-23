// +build linux_bpf

package http

import (
	"time"
)

type telemetry struct {
	then   time.Time
	hits   map[int]int
	misses int
}

func newTelemetry() *telemetry {
	t := new(telemetry)
	t.reset()
	return t
}

func (t *telemetry) aggregate(txs []httpTX, err error) {
	for _, tx := range txs {
		t.hits[tx.StatusClass()]++
	}

	if err == errLostBatch {
		t.misses++
	}
}

func (t *telemetry) getStats() (time.Time, map[string]int64) {
	now := time.Now()
	delta := float64(now.Sub(t.then).Seconds())
	data := map[string]int64{
		"1XX_request_count":     int64(t.hits[100]),
		"2XX_request_count":     int64(t.hits[200]),
		"3XX_request_count":     int64(t.hits[300]),
		"4XX_request_count":     int64(t.hits[400]),
		"5XX_request_count":     int64(t.hits[500]),
		"1XX_request_rate":      int64(float64(t.hits[100]) / delta),
		"2XX_request_rate":      int64(float64(t.hits[200]) / delta),
		"3XX_request_rate":      int64(float64(t.hits[300]) / delta),
		"4XX_request_rate":      int64(float64(t.hits[400]) / delta),
		"5XX_request_rate":      int64(float64(t.hits[500]) / delta),
		"requests_missed_count": int64(t.misses * HTTPBatchSize),
		"requests_missed_rate":  int64(float64(t.misses*HTTPBatchSize) / delta),
	}
	t.reset()
	return now, data
}

func (t *telemetry) reset() {
	t.then = time.Now()
	t.hits = make(map[int]int)
	t.misses = 0
}
