// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package containers

import (
	"sort"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

/*
 * Rate capping logic to work around buggy kernel versions reporting artificial
 * spikes in the cpuacct metrics. This logic is specific to this check and not
 * ported downstream in the aggregator as it's not meant as a "standard" feature.
 *
 * This is triggered by users uncommenting the `capped_metrics` section in docker.yaml
 */

const (
	dockerRateCacheKey        = "docker_rate_prev_value"
	dockerRateCachingDuration = time.Minute
)

// cappedSender wraps around the standard Sender and overrides
// the Rate method to implement rate capping
type cappedSender struct {
	aggregator.Sender
	rateCaps  map[string]float64
	timestamp time.Time // Current time at check Run()
}

type ratePoint struct {
	value float64
	time  time.Time
}

// Rate checks the rate value against the `capped_metrics` configuration
// to filter out buggy spikes coming for cgroup cpu accounting
func (s *cappedSender) Rate(metric string, value float64, hostname string, tags []string) {
	capValue, found := s.rateCaps[metric]
	if !found { // Metric not capped, skip capping system
		s.Sender.Rate(metric, value, hostname, tags)
		return
	}

	// Previous value lookup
	sort.Strings(tags)
	cacheKeyParts := []string{dockerRateCacheKey, metric, hostname}
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
	return
}

func (s *cappedSender) getPoint(cacheKey string) (*ratePoint, bool) {
	prev, found := cache.Cache.Get(cacheKey)
	if !found {
		return nil, false
	}
	prevPoint, ok := prev.(*ratePoint)
	return prevPoint, ok
}

func (s *cappedSender) storePoint(cacheKey string, value float64, timestamp time.Time) {
	point := &ratePoint{
		value: value,
		time:  timestamp,
	}
	cache.Cache.Set(cacheKey, point, dockerRateCachingDuration)
}

func (d *DockerCheck) GetSender() (aggregator.Sender, error) {
	sender, err := aggregator.GetSender(d.ID())
	if err != nil {
		return sender, err
	}
	if len(d.instance.CappedMetrics) == 0 {
		// No cap set, using a bare sender
		return sender, nil
	}

	if d.cappedSender == nil {
		d.cappedSender = &cappedSender{
			Sender:    sender,
			rateCaps:  d.instance.CappedMetrics,
			timestamp: time.Now(),
		}
	} else {
		d.cappedSender.timestamp = time.Now()
		// always refresh the base sender reference (not guaranteed to be constant)
		d.cappedSender.Sender = sender
	}

	return d.cappedSender, nil
}
