// +build linux_bpf

package http

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/stats/quantile"
)

// RequestStats stores stats for HTTP requests to a particular path, organized by the class
// of the response code (1XX, 2XX, 3XX, 4XX, 5XX)
type RequestStats [5]struct {
	count     int
	latencies *quantile.SliceSummary
}

func (r RequestStats) addRequest(statusClass int, latency time.Duration) {
	i := statusClass/100 - 1
	r[i].count++

	if r[i].latencies == nil {
		r[i].latencies = quantile.NewSliceSummary()
	}
	r[i].latencies.Insert(float64(latency))
}

// CombineWith merges the data in 2 RequestStats objects
func (r RequestStats) CombineWith(newStats RequestStats) {
	for i := 0; i < 5; i++ {
		r[i].count += newStats[i].count

		if r[i].latencies == nil {
			r[i].latencies = newStats[i].latencies
		} else if newStats[i].latencies != nil {
			r[i].latencies.Merge(newStats[i].latencies)
		}
	}
}

// Count returns the count of requests of type "i" made
func (r RequestStats) Count(i int) int {
	return r[i].count
}

// GetLatencyQuantile returns the latency of requests of type "i" at quantile 'q' (0 <= q <= 1)
func (r RequestStats) GetLatencyQuantile(i int, q float64) float64 {
	if r[i].latencies == nil {
		return 0
	}
	return r[i].latencies.Quantile(q)
}
