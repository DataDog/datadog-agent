// +build linux_bpf

package http

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
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

func (t *telemetry) report() {
	delta := float64(time.Now().Sub(t.then).Seconds())
	log.Infof(
		"http report: 100(%d reqs, %.2f/s) 200(%d reqs, %.2f/s) 300(%d reqs, %.2f/s), 400(%d reqs, %.2f/s) 500(%d reqs, %.2f/s), misses(%d reqs, %.2f/s)",
		t.hits[100], float64(t.hits[100])/delta,
		t.hits[200], float64(t.hits[200])/delta,
		t.hits[300], float64(t.hits[300])/delta,
		t.hits[400], float64(t.hits[400])/delta,
		t.hits[500], float64(t.hits[500])/delta,
		t.misses*HTTPBatchSize,
		float64(t.misses*HTTPBatchSize)/delta,
	)

	t.reset()
}

func (t *telemetry) reset() {
	t.then = time.Now()
	t.hits = make(map[int]int)
	t.misses = 0
}
