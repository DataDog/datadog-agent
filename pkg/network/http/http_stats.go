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
}

// CombineWith merges the data in 2 RequestStats objects
func (r *RequestStats) CombineWith(newStats RequestStats) {
	for i := 0; i < 5; i++ {
		r[i].count += newStats[i].count

		if r[i].latencies == nil {
			r[i].latencies = newStats[i].latencies
		} else if newStats[i].latencies != nil {
			err := r[i].latencies.MergeWith(newStats[i].latencies)
			if err != nil {
				log.Debugf("Error merging HTTP transactions: %v", err)
			}
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

	if r[i].latencies == nil {
		var err error
		r[i].latencies, err = ddsketch.NewDefaultDDSketch(RelativeAccuracy)
		if err != nil {
			log.Debugf("Error recording HTTP transaction latency: could not create new ddsketch: %v", err)
			return
		}
	}

	err := r[i].latencies.Add(latency)
	if err != nil {
		log.Debugf("Error recording HTTP transaction latency: could not add latency to ddsketch: %v", err)
	}
}
