// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package postgres

import (
	"sync"

	"github.com/DataDog/sketches-go/ddsketch"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// RelativeAccuracy defines the acceptable error in quantile values calculated by DDSketch.
// For example, if the actual value at p50 is 100, with a relative accuracy of 0.01 the value calculated
// will be between 99 and 101
const RelativeAccuracy = 0.01

// Key is an identifier for a group of Kafka transactions
type Key struct {
	Operation string
	TableName string
	types.ConnectionKey
}

// RequestStat stores statskeeper for Kafka requests to a particular key
type RequestStat struct {
	// this field order is intentional to help the GC pointer tracking
	Latencies          *ddsketch.DDSketch
	FirstLatencySample float64
	Count              int
}

func (r *RequestStat) initSketch() (err error) {
	r.Latencies, err = ddsketch.NewDefaultDDSketch(RelativeAccuracy)
	if err != nil {
		log.Debugf("error recording postgres transaction latency: could not create new ddsketch: %v", err)
	}
	return
}

// CombineWith merges the data in 2 RequestStats objects
// newStats is kept as it is, while the method receiver gets mutated
func (r *RequestStat) CombineWith(newStats *RequestStat) {
	r.Count += newStats.Count
	if r.FirstLatencySample == 0 && newStats.FirstLatencySample != 0 {
		r.FirstLatencySample = newStats.FirstLatencySample
	}
	if newStats.Latencies == nil {
		return
	}
	if r.Latencies == nil {
		r.Latencies = newStats.Latencies.Copy()
	} else if newStats.Latencies != nil {
		if err := r.Latencies.MergeWith(newStats.Latencies); err != nil {
			log.Debugf("could not add request latency to ddsketch: %v", err)
		}
	}
}

// StatKeeper is a struct to hold the statskeeper for the postgres protocol
type StatKeeper struct {
	stats      map[Key]*RequestStat
	statsMutex sync.RWMutex
	maxEntries int
}

// NewStatkeeper creates a new StatKeeper
func NewStatkeeper(c *config.Config) *StatKeeper {
	return &StatKeeper{
		stats:      make(map[Key]*RequestStat),
		maxEntries: c.MaxPostgresStatsBuffered,
	}
}

// Process processes the kafka transaction
func (statKeeper *StatKeeper) Process(tx *EbpfEvent) {
	statKeeper.statsMutex.Lock()
	defer statKeeper.statsMutex.Unlock()

	key := Key{
		Operation:     tx.Operation(),
		TableName:     tx.TableName(),
		ConnectionKey: tx.ConnTuple(),
	}
	requestStats, ok := statKeeper.stats[key]
	if !ok {
		if len(statKeeper.stats) >= statKeeper.maxEntries {
			return
		}
		requestStats = new(RequestStat)
		if err := requestStats.initSketch(); err != nil {
			return
		}
		requestStats.FirstLatencySample = tx.RequestLatency()
		statKeeper.stats[key] = requestStats
	}
	requestStats.Count++
	if requestStats.Count == 1 {
		return
	}
	if err := requestStats.Latencies.Add(tx.RequestLatency()); err != nil {
		log.Debugf("could not add request latency to ddsketch: %v", err)
	}
}

// GetAndResetAllStats returns all the statskeeper and resets the statskeeper
func (statKeeper *StatKeeper) GetAndResetAllStats() map[Key]*RequestStat {
	statKeeper.statsMutex.RLock()
	defer statKeeper.statsMutex.RUnlock()
	ret := statKeeper.stats // No deep copy needed since `statKeeper.statskeeper` gets reset
	statKeeper.stats = make(map[Key]*RequestStat)
	return ret
}
