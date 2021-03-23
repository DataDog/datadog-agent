package http

import (
	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/sketches-go/ddsketch"
)

// Key is an identifier for a group of HTTP transactions
type Key struct {
	SrcIPHigh uint64
	SrcIPLow  uint64
	SrcPort   uint16

	DstIPHigh uint64
	DstIPLow  uint64
	DstPort   uint16
}

func NewKey(saddr, daddr util.Address, sport, dport uint16) Key {
	saddrl, saddrh := util.ToLowHigh(saddr)
	daddrl, daddrh := util.ToLowHigh(daddr)
	return Key{
		SrcIPHigh: saddrh,
		SrcIPLow:  saddrl,
		SrcPort:   sport,
		DstIPHigh: daddrh,
		DstIPLow:  daddrl,
		DstPort:   dport,
	}
}

// RelativeAccuracy defines the acceptable error in quantile values calculated by DDSketch.
// For example, if the actual value at p50 is 100, with a relative accuracy of 0.01 the value calculated
// will be between 99 and 101
const RelativeAccuracy = 0.01

// RequestStats stores stats for HTTP requests to a particular path, organized by the class
// of the response code (1XX, 2XX, 3XX, 4XX, 5XX)
type RequestStats [5]struct {
	// Note: every time we add a latency value to the DDSketch below, it's possible for the sketch to discard that value
	// (ie if it is outside the range that is tracked by the sketch). For that reason, in order to keep an accurate count
	// the number of http transactions processed, we have our own count field (rather than relying on DDSketch.GetCount())
	count     int
	latencies *ddsketch.DDSketch

	firstLatency float64
}

// CombineWith merges the data in 2 RequestStats objects
func (r *RequestStats) CombineWith(newStats RequestStats) {
	for i := 0; i < 5; i++ {
		statusClass := 100 * (i + 1)

		if newStats[i].count == 0 {
			continue
		}

		if newStats[i].count == 1 {
			r.AddRequest(statusClass, newStats[i].firstLatency)
			continue
		}

		if r[i].latencies == nil {
			if err := r.initSketch(i); err != nil {
				continue
			}

			if r[i].count == 1 {
				r[i].latencies.Add(r[i].firstLatency)
			}
		}

		r[i].count += newStats[i].count
		err := r[i].latencies.MergeWith(newStats[i].latencies)
		if err != nil {
			log.Debugf("error merging http transactions: %v", err)
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

// AddRequest takes information about a HTTP transaction and adds it to the request stats
func (r *RequestStats) AddRequest(statusClass int, latency float64) {
	i := statusClass/100 - 1
	r[i].count++

	// We postpone the creation of histograms when we have only one latency sample
	if r[i].count == 1 {
		r[i].firstLatency = latency
		return
	}

	if r[i].latencies == nil {
		if err := r.initSketch(i); err != nil {
			return
		}

		// Add the defered latency sample
		r[i].latencies.Add(r[i].firstLatency)
	}

	err := r[i].latencies.Add(latency)
	if err != nil {
		log.Debugf("error recording http transaction latency: could not add latency to ddsketch: %v", err)
	}
}

func (r *RequestStats) initSketch(i int) (err error) {
	r[i].latencies, err = ddsketch.NewDefaultDDSketch(RelativeAccuracy)
	if err != nil {
		log.Debugf("error recording http transaction latency: could not create new ddsketch: %v", err)
	}
	return
}
