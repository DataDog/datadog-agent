// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package http

import (
	"github.com/DataDog/sketches-go/ddsketch"

	"github.com/DataDog/datadog-agent/pkg/network/types"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/intern"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Interner is used to intern strings to save memory allocations.
var Interner = intern.NewStringInterner()

// Method is the type used to represent HTTP request methods
type Method uint8

// RelativeAccuracy defines the acceptable error in quantile values calculated by DDSketch.
// For example, if the actual value at p50 is 100, with a relative accuracy of 0.01 the value calculated
// will be between 99 and 101
const RelativeAccuracy = 0.01

const (
	// MethodUnknown represents an unknown request method
	MethodUnknown Method = iota
	// MethodGet represents the GET request method
	MethodGet
	// MethodPost represents the POST request method
	MethodPost
	// MethodPut represents the PUT request method
	MethodPut
	// MethodDelete represents the DELETE request method
	MethodDelete
	// MethodHead represents the HEAD request method
	MethodHead
	// MethodOptions represents the OPTIONS request method
	MethodOptions
	// MethodPatch represents the PATCH request method
	MethodPatch
	// MethodTrace represents the TRACE request method
	MethodTrace
)

// Method returns a string representing the HTTP method of the request
func (m Method) String() string {
	switch m {
	case MethodGet:
		return "GET"
	case MethodPost:
		return "POST"
	case MethodPut:
		return "PUT"
	case MethodHead:
		return "HEAD"
	case MethodDelete:
		return "DELETE"
	case MethodOptions:
		return "OPTIONS"
	case MethodPatch:
		return "PATCH"
	case MethodTrace:
		return "TRACE"
	default:
		return "UNKNOWN"
	}
}

// Path represents the HTTP path
type Path struct {
	Content  *intern.StringValue
	FullPath bool
}

// Key is an identifier for a group of HTTP transactions
type Key struct {
	// this field order is intentional to help the GC pointer tracking
	Path Path
	types.ConnectionKey
	Method Method
}

// String returns a string representation of the Key
func (k Key) String() string {
	return "{IP: " + k.ConnectionKey.String() + ", Method: " + k.Method.String() + ", Path: " + k.Path.Content.Get() + "}"
}

// NewKey generates a new Key
func NewKey(saddr, daddr util.Address, sport, dport uint16, path []byte, fullPath bool, method Method) Key {
	return NewKeyWithConnection(types.NewConnectionKey(saddr, daddr, sport, dport), path, fullPath, method)
}

// NewKeyWithConnection generates a new Key with a given connection tuple
func NewKeyWithConnection(connKey types.ConnectionKey, path []byte, fullPath bool, method Method) Key {
	return Key{
		ConnectionKey: connKey,
		Path: Path{
			Content:  Interner.Get(path),
			FullPath: fullPath,
		},
		Method: method,
	}
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

func (r *RequestStat) initSketch() (err error) {
	r.Latencies, err = ddsketch.NewDefaultDDSketch(RelativeAccuracy)
	if err != nil {
		log.Debugf("error recording http transaction latency: could not create new ddsketch: %v", err)
	}
	return
}

// RequestStats stores HTTP request statistics.
type RequestStats struct {
	Data map[uint16]*RequestStat
}

// NewRequestStats creates a new RequestStats object.
func NewRequestStats() *RequestStats {
	return &RequestStats{
		Data: make(map[uint16]*RequestStat),
	}
}

// isValid checks is the status code is in the range of valid HTTP responses.
func (r *RequestStats) isValid(status uint16) bool {
	return status >= 100 && status < 600
}

// CombineWith merges the data in 2 RequestStats objects
// newStats is kept as it is, while the method receiver gets mutated
func (r *RequestStats) CombineWith(newStats *RequestStats) {
	for statusCode, newRequests := range newStats.Data {
		if newRequests.Count == 0 {
			// Nothing to do in this case
			continue
		}

		if newRequests.Count == 1 {
			// The other bucket has a single latency sample, so we "manually" add it
			r.AddRequest(statusCode, newRequests.FirstLatencySample, newRequests.StaticTags, newRequests.DynamicTags)
			continue
		}

		stats, exists := r.Data[statusCode]
		if !exists {
			stats = &RequestStat{}
			r.Data[statusCode] = stats
		}

		// The other bucket (newStats) has multiple samples and therefore a DDSketch object
		// We first ensure that the bucket we're merging to have a DDSketch object
		if stats.Latencies == nil {
			stats.Latencies = newRequests.Latencies.Copy()

			// If we have a latency sample in this bucket we now add it to the DDSketch
			if stats.Count == 1 {
				err := stats.Latencies.Add(stats.FirstLatencySample)
				if err != nil {
					log.Debugf("could not add request latency to ddsketch: %v", err)
				}
			}
		} else {
			err := stats.Latencies.MergeWith(newRequests.Latencies)
			if err != nil {
				log.Debugf("error merging http transactions: %v", err)
			}
		}
		stats.Count += newRequests.Count
	}
}

// AddRequest takes information about a HTTP transaction and adds it to the request stats
func (r *RequestStats) AddRequest(statusCode uint16, latency float64, staticTags uint64, dynamicTags []string) {
	if !r.isValid(statusCode) {
		return
	}

	stats, exists := r.Data[statusCode]
	if !exists {
		stats = &RequestStat{}
		r.Data[statusCode] = stats
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
		if err := stats.Latencies.Add(stats.FirstLatencySample); err != nil {
			log.Debugf("could not add request latency to ddsketch: %v", err)
		}
	}

	if err := stats.Latencies.Add(latency); err != nil {
		log.Debugf("could not add request latency to ddsketch: %v", err)
	}
}

// HalfAllCounts sets the count of all stats for each status class to half their current value.
// This is used to remove duplicates from the count in the context of Windows localhost traffic.
func (r *RequestStats) HalfAllCounts() {
	for _, stats := range r.Data {
		if stats != nil {
			stats.Count = stats.Count / 2
		}
	}
}
