// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package redis

import (
	"errors"

	"github.com/DataDog/sketches-go/ddsketch"

	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/types"
	"github.com/DataDog/datadog-agent/pkg/util/intern"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Interner is used to intern strings to save memory allocations.
var Interner = intern.NewStringInterner()

// Key is an identifier for a group of Redis transactions
type Key struct {
	types.ConnectionKey
	Command   CommandType
	KeyName   *intern.StringValue
	Truncated bool
}

// RequestStats stores Redis request statistics, grouped by whether an error occurred.
// We include the error here and not in the Key to avoid duplicating keys when we observe both successful and
// erroneous transactions for the same key.
type RequestStats struct {
	ErrorToStats map[bool]*RequestStat
}

// RequestStat represents a group of Redis transactions stats.
type RequestStat struct {
	// this field order is intentional to help the GC pointer tracking
	Latencies          *ddsketch.DDSketch
	FirstLatencySample float64
	Count              int
	StaticTags         uint64
}

// NewRequestStats creates a new RequestStats object.
func NewRequestStats() *RequestStats {
	return &RequestStats{
		ErrorToStats: make(map[bool]*RequestStat),
	}
}

func (r *RequestStat) initSketch() error {
	latencies := protocols.SketchesPool.Get()
	if latencies == nil {
		return errors.New("error recording redis transaction latency: could not create new ddsketch")
	}
	r.Latencies = latencies
	return nil
}

func (r *RequestStat) close() {
	if r.Latencies != nil {
		r.Latencies.Clear()
		protocols.SketchesPool.Put(r.Latencies)
	}
}

// CombineWith merges the data in 2 RequestStats objects
// newStats is kept as it is, while the method receiver gets mutated
func (r *RequestStats) CombineWith(newStats *RequestStats) {
	for isErr, newRequests := range newStats.ErrorToStats {
		if newRequests.Count == 0 {
			continue
		}
		if newRequests.Latencies == nil {
			r.AddRequest(isErr, newRequests.Count, newRequests.StaticTags, newRequests.FirstLatencySample)
		} else {
			r.mergeRequests(isErr, newRequests)
		}
	}
}

// mergeRequests adds a RequestStat to the given RequestStats. Only called when newStats has Latencies.
func (r *RequestStats) mergeRequests(isErr bool, newStats *RequestStat) {
	stats, exists := r.ErrorToStats[isErr]
	if !exists {
		stats = &RequestStat{}
		r.ErrorToStats[isErr] = stats
	}
	// The other bucket (newStats) has a DDSketch object
	// We first ensure that the bucket we're merging to have a DDSketch object
	if stats.Latencies == nil {
		stats.Latencies = newStats.Latencies.Copy()

		// If we have a latency sample in this bucket we now add it to the DDSketch
		if stats.Count == 1 {
			err := stats.Latencies.Add(stats.FirstLatencySample)
			if err != nil {
				log.Debugf("could not add redis request latency to ddsketch: %v", err)
			}
		}
	} else {
		err := stats.Latencies.MergeWith(newStats.Latencies)
		if err != nil {
			log.Debugf("error merging redis transactions: %v", err)
		}
	}
	stats.StaticTags |= newStats.StaticTags
	stats.Count += newStats.Count
}

// AddRequest adds information about a Redis transaction to the request stats
func (r *RequestStats) AddRequest(isError bool, count int, staticTags uint64, latency float64) {
	stats, exists := r.ErrorToStats[isError]
	if !exists {
		stats = &RequestStat{}
		r.ErrorToStats[isError] = stats
	}
	originalCount := stats.Count
	stats.Count += count
	stats.StaticTags |= staticTags
	// If the receiver has no latency sample, use the newStat sample
	if stats.FirstLatencySample == 0 {
		stats.FirstLatencySample = latency
		return
	}
	// If the receiver has no ddsketch latency, use the newStat latency
	if stats.Latencies == nil {
		if err := stats.initSketch(); err != nil {
			log.Warnf("could not add request latency to ddsketch: %v", err)
			return
		}
		// If we have a latency sample in this bucket we now add it to the DDSketch
		if stats.FirstLatencySample != 0 {
			err := stats.Latencies.AddWithCount(stats.FirstLatencySample, float64(originalCount))
			if err != nil {
				log.Debugf("could not add redis request latency to ddsketch: %v", err)
			}
		}
	}
	if err := stats.Latencies.AddWithCount(latency, float64(count)); err != nil {
		log.Debugf("could not add request latency to ddsketch: %v", err)
	}
}

// Close releases internal stats resources.
func (r *RequestStats) Close() {
	for _, stats := range r.ErrorToStats {
		if stats != nil {
			stats.close()
		}
	}
}
