// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package redis

import (
	"github.com/DataDog/datadog-agent/pkg/process/util"

	"github.com/DataDog/sketches-go/ddsketch"

	"github.com/DataDog/datadog-agent/pkg/network/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Key is an identifier for a group of Redis transactions
type Key struct {
	types.ConnectionKey
	Command   CommandType
	KeyName   string
	Truncated bool
}

// NewKey creates a new redis key
func NewKey(saddr, daddr util.Address, sport, dport uint16, command CommandType, keyName string, truncated bool) Key {
	return Key{
		ConnectionKey: types.NewConnectionKey(saddr, daddr, sport, dport),
		Command:       command,
		KeyName:       keyName,
		Truncated:     truncated,
	}
}

// RequestStat represents a group of Redis transactions that has a shared key.
type RequestStats struct {
	ErrorsToStats map[bool]*RequestStat
}

type RequestStat struct {
	// this field order is intentional to help the GC pointer tracking
	Latencies          *ddsketch.DDSketch
	FirstLatencySample float64
	Count              int
	StaticTags         uint64
}

func NewRequestStats() *RequestStats {
	return &RequestStats{
		ErrorsToStats: make(map[bool]*RequestStat),
	}
}

// CombineWith merges the data in 2 RequestStats objects
// newStats is kept as it is, while the method receiver gets mutated
func (r *RequestStats) CombineWith(newStats *RequestStats) {
	for isErr, newRequests := range newStats.ErrorsToStats {
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

func (r *RequestStats) mergeRequests(isErr bool, newStats *RequestStat) {
	stats, exists := r.ErrorsToStats[isErr]
	if !exists {
		stats = &RequestStat{}
		r.ErrorsToStats[isErr] = stats
	}
	// The other bucket (newStats) has a DDSketch object
	// We first ensure that the bucket we're merging to have a DDSketch object
	if stats.Latencies == nil {
		stats.Latencies = newStats.Latencies.Copy()

		// If we have a latency sample in this bucket we now add it to the DDSketch
		if stats.FirstLatencySample != 0 {
			err := stats.Latencies.AddWithCount(stats.FirstLatencySample, float64(stats.Count))
			if err != nil {
				log.Debugf("could not add kafka request latency to ddsketch: %v", err)
			}
		}
	} else {
		err := stats.Latencies.MergeWith(newStats.Latencies)
		if err != nil {
			log.Debugf("error merging kafka transactions: %v", err)
		}
	}
	stats.Count += newStats.Count
}

func (r *RequestStats) AddRequest(isError bool, count int, staticTags uint64, latency float64) {
	stats, exists := r.ErrorsToStats[isError]
	if !exists {
		stats = &RequestStat{}
		r.ErrorsToStats[isError] = stats
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

func (r *RequestStats) Close() {
	for _, stats := range r.ErrorsToStats {
		if stats != nil {
			stats.close()
		}
	}
}
