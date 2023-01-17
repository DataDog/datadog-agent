// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package generic

import (
	"sort"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

const (
	rateCappingCacheKey = "rcap_prev_value"
	rateCachingDuration = time.Minute
)

// CappedSender wraps around the standard Sender and overrides
// the Rate method to implement rate capping
type CappedSender struct {
	aggregator.Sender
	rateCaps  map[string]float64
	timestamp time.Time // Current time at check Run()
}

type ratePoint struct {
	value float64
	time  time.Time
}

// NewCappedSender returns a capped sender
func NewCappedSender(cappedMetrics map[string]float64, sender aggregator.Sender) aggregator.Sender {
	return &CappedSender{
		Sender:    sender,
		rateCaps:  cappedMetrics,
		timestamp: time.Now(),
	}
}

// Rate checks the rate value against the `capped_metrics` configuration
// to filter out buggy spikes coming for cgroup cpu accounting
func (s *CappedSender) Rate(metric string, value float64, hostname string, tags []string) {
	capValue, found := s.rateCaps[metric]
	if !found { // Metric not capped, skip capping system
		s.Sender.Rate(metric, value, hostname, tags)
		return
	}

	// Previous value lookup
	sort.Strings(tags)
	cacheKeyParts := []string{rateCappingCacheKey, metric, hostname}
	cacheKeyParts = append(cacheKeyParts, tags...)
	cacheKey := cache.BuildAgentKey(cacheKeyParts...)
	previous, found := s.getPoint(cacheKey)

	if !found {
		// First submit of the rate for that context
		s.storePoint(cacheKey, value, s.timestamp)
		s.Sender.Rate(metric, value, hostname, tags)
		return
	}

	timeDelta := s.timestamp.Sub(previous.time).Seconds()
	if timeDelta == 0 {
		// Let's avoid a divide by zero and pass through, the aggregator will handle it
		s.storePoint(cacheKey, value, s.timestamp)
		s.Sender.Rate(metric, value, hostname, tags)
		return
	}
	rate := (value - previous.value) / timeDelta
	if rate < capValue {
		// Under cap, transmit
		s.storePoint(cacheKey, value, s.timestamp)
		s.Sender.Rate(metric, value, hostname, tags)
		return
	}

	// Over cap, store but don't transmit
	log.Debugf("Dropped latest value %.0f (raw sample: %.0f) of metric %s as it was above the cap for this metric.", rate, value, metric)
	s.storePoint(cacheKey, value, s.timestamp)
}

func (s *CappedSender) getPoint(cacheKey string) (*ratePoint, bool) {
	prev, found := cache.Cache.Get(cacheKey)
	if !found {
		return nil, false
	}
	prevPoint, ok := prev.(*ratePoint)
	return prevPoint, ok
}

func (s *CappedSender) storePoint(cacheKey string, value float64, timestamp time.Time) {
	point := &ratePoint{
		value: value,
		time:  timestamp,
	}
	cache.Cache.Set(cacheKey, point, rateCachingDuration)
}
