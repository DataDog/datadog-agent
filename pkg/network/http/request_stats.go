// +build linux_bpf

package http

import (
	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/sketches-go/ddsketch"
)

// DDSketch uses a relative error guarantee, meaning that quantiles in the sketch are accurate to within
// RelativeAccuracy percent (ie if the actual value at p50 is 100, with a relative accuracy of 0.01 the
// value calculated will be between 99 and 101)
const RelativeAccuracy = 0.01

// RequestStats stores stats for HTTP requests to a particular path, organized by the class
// of the response code (1XX, 2XX, 3XX, 4XX, 5XX)
type RequestStats [5]struct {
	count     int
	latencies *ddsketch.DDSketch
}

func (r *RequestStats) addRequest(statusClass int, latency float64) {
	i := statusClass/100 - 1
	r[i].count++

	if r[i].latencies == nil {
		var err error
		r[i].latencies, err = ddsketch.NewDefaultDDSketch(RelativeAccuracy)
		if err != nil {
			log.Debugf("Error recording HTTP transaction latency: could not create new ddsketch: %v", err)
			return
		}
	}

	err := r[i].latencies.Add(latency)
	log.Debugf("Error recording HTTP transaction latency: could not add latency to ddsketch: %v", err)
}

// CombineWith merges the data in 2 RequestStats objects
func (r *RequestStats) CombineWith(newStats RequestStats) {
	for i := 0; i < 5; i++ {
		r[i].count += newStats[i].count

		if r[i].latencies == nil {
			r[i].latencies = newStats[i].latencies
		} else if newStats[i].latencies != nil {
			r[i].latencies.MergeWith(newStats[i].latencies)
		}
	}
}

// Count returns the number of requests made which received a response of status class s
func (r *RequestStats) Count(s model.HTTPResponseStatus) int {
	return r[s].count
}

// Latencies returns a sketch of the latencies of the requests made which received
// a response of status class s
func (r *RequestStats) Latencies(s model.HTTPResponseStatus) *ddsketch.DDSketch {
	return r[s].latencies
}
