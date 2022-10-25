// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package http

import (
	"github.com/DataDog/sketches-go/ddsketch"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Method is the type used to represent HTTP request methods
type Method uint8

// RelativeAccuracy defines the acceptable error in quantile values calculated by DDSketch.
// For example, if the actual value at p50 is 100, with a relative accuracy of 0.01 the value calculated
// will be between 99 and 101
const RelativeAccuracy = 0.01

// Path represents the HTTP path
type Path struct {
	Content  string
	FullPath bool
}

// KeyTuple represents the network tuple for a group of HTTP transactions
type KeyTuple struct {
	SrcIPHigh uint64
	SrcIPLow  uint64

	DstIPHigh uint64
	DstIPLow  uint64

	// ports separated for alignment/size optimization
	SrcPort uint16
	DstPort uint16
}

// Key is an identifier for a group of HTTP transactions
type Key struct {
	// this field order is intentional to help the GC pointer tracking
	Path Path
	KeyTuple
	Method Method
}

// NewKey generates a new Key
func NewKey(saddr, daddr util.Address, sport, dport uint16, path string, fullPath bool, method Method) Key {
	return Key{
		KeyTuple: NewKeyTuple(saddr, daddr, sport, dport),
		Path: Path{
			Content:  path,
			FullPath: fullPath,
		},
		Method: method,
	}
}

// NewKeyTuple generates a new KeyTuple
func NewKeyTuple(saddr, daddr util.Address, sport, dport uint16) KeyTuple {
	saddrl, saddrh := util.ToLowHigh(saddr)
	daddrl, daddrh := util.ToLowHigh(daddr)
	return KeyTuple{
		SrcIPHigh: saddrh,
		SrcIPLow:  saddrl,
		SrcPort:   sport,
		DstIPHigh: daddrh,
		DstIPLow:  daddrl,
		DstPort:   dport,
	}
}

// NumStatusClasses represents the number of HTTP status classes (1XX, 2XX, 3XX, 4XX, 5XX)
const NumStatusClasses = 5

// RequestStats stores stats for HTTP requests to a particular path, organized by the class
// of the response code (1XX, 2XX, 3XX, 4XX, 5XX)
type RequestStats struct {
	data [NumStatusClasses]*RequestStat
}

// RequestStat stores stats for HTTP requests to a particular path
type RequestStat struct {
	// this field order is intentional to help the GC pointer tracking
	Latencies *ddsketch.DDSketch
	// Note: every time we add a latency value to the DDSketch, it's possible for the sketch to discard that value
	// (ie if it is outside the range that is tracked by the sketch). For that reason, in order to keep an accurate count
	// the number of http transactions processed, we have our own count field (rather than relying on DDSketch.GetCount())
	Count int

	// This field holds the value (in nanoseconds) of the first HTTP request
	// in this bucket. We do this as optimization to avoid creating sketches with
	// a single value. This is quite common in the context of HTTP requests without
	// keep-alives where a short-lived TCP connection is used for a single request.
	FirstLatencySample float64

	// Tags bitfields from tags-types.h
	StaticTags uint64

	// Dynamic tags (if attached)
	DynamicTags []string
}

func (r *RequestStats) idx(status int) int {
	return status/100 - 1
}

func (r *RequestStats) isValid(status int) bool {
	i := r.idx(status)
	return i >= 0 && i < len(r.data)
}

func (r *RequestStats) init(status int) {
	r.data[r.idx(status)] = new(RequestStat)
}

// Stats returns the RequestStat object for the provided status.
// If no stats exist, or the status code is invalid, it will return nil.
func (r *RequestStats) Stats(status int) *RequestStat {
	i := r.idx(status)
	if i < 0 || i >= len(r.data) {
		return nil
	}
	return r.data[i]
}

// HasStats returns true if there is data for that status class
func (r *RequestStats) HasStats(status int) bool {
	i := r.idx(status)
	if i < 0 || i >= len(r.data) {
		return false
	}
	return r.data[i] != nil && r.data[i].Count > 0
}

// CombineWith merges the data in 2 RequestStats objects
// newStats is kept as it is, while the method receiver gets mutated
func (r *RequestStats) CombineWith(newStats *RequestStats) {
	for statusClass := 100; statusClass <= 500; statusClass += 100 {
		if !newStats.HasStats(statusClass) {
			// Nothing to do in this case
			continue
		}

		newStatsData := newStats.Stats(statusClass)
		if newStatsData.Count == 1 {
			// The other bucket has a single latency sample, so we "manually" add it
			r.AddRequest(statusClass, newStatsData.FirstLatencySample, newStatsData.StaticTags, newStatsData.DynamicTags)
			continue
		}

		stats := r.Stats(statusClass)
		if stats == nil {
			r.init(statusClass)
			stats = r.Stats(statusClass)
		}

		// The other bucket (newStats) has multiple samples and therefore a DDSketch object
		// We first ensure that the bucket we're merging to has a DDSketch object
		if stats.Latencies == nil {
			stats.Latencies = newStatsData.Latencies.Copy()

			// If we have a latency sample in this bucket we now add it to the DDSketch
			if stats.Count == 1 {
				err := stats.Latencies.Add(stats.FirstLatencySample)
				if err != nil {
					log.Debugf("could not add request latency to ddsketch: %v", err)
				}
			}
		} else {
			err := stats.Latencies.MergeWith(newStatsData.Latencies)
			if err != nil {
				log.Debugf("error merging http transactions: %v", err)
			}
		}
		stats.Count += newStatsData.Count
	}
}

// AddRequest takes information about a HTTP transaction and adds it to the request stats
func (r *RequestStats) AddRequest(statusClass int, latency float64, staticTags uint64, dynamicTags []string) {
	if !r.isValid(statusClass) {
		return
	}
	stats := r.Stats(statusClass)
	if stats == nil {
		r.init(statusClass)
		stats = r.Stats(statusClass)
	}

	stats.StaticTags |= staticTags
	if len(dynamicTags) != 0 {
		stats.DynamicTags = append(stats.DynamicTags, dynamicTags...)
	}

	stats.Count++
	if stats.Count == 1 {
		// We postpone the creation of histograms when we have only one latency sample
		stats.FirstLatencySample = latency
		return
	}

	if stats.Latencies == nil {
		if err := stats.initSketch(); err != nil {
			return
		}

		// Add the deferred latency sample
		err := stats.Latencies.Add(stats.FirstLatencySample)
		if err != nil {
			log.Debugf("could not add request latency to ddsketch: %v", err)
		}
	}

	err := stats.Latencies.Add(latency)
	if err != nil {
		log.Debugf("could not add request latency to ddsketch: %v", err)
	}
}

func (r *RequestStat) initSketch() (err error) {
	r.Latencies, err = ddsketch.NewDefaultDDSketch(RelativeAccuracy)
	if err != nil {
		log.Debugf("error recording http transaction latency: could not create new ddsketch: %v", err)
	}
	return
}

// HalfAllCounts sets the count of all stats for each status class to half their current value.
// This is used to remove duplicates from the count in the context of Windows localhost traffic.
func (r *RequestStats) HalfAllCounts() {
	for i := 0; i < NumStatusClasses; i++ {
		if r.data[i] != nil {
			r.data[i].Count = r.data[i].Count / 2
		}
	}
}
