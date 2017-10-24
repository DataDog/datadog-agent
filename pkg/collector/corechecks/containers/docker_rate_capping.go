// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package containers

import (
	"time"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

// cappedSender wraps around the standard Sender and overrides
// the Rate method to implement rate capping
type cappedSender struct {
	aggregator.Sender
	previousRateValues map[string]float64
	previousTimes      map[string]time.Time
	rateCaps           map[string]float64
	timestamp          time.Time // Current time at check Run()
}

// Rate checks the rate value against the `capped_metrics` configuration
// to filter out buggy spikes coming for cgroup cpu accounting
func (s *cappedSender) Rate(metric string, value float64, hostname string, tags []string) {
	capValue, found := s.rateCaps[metric]
	if !found { // Metric not capped, skip capping system
		s.Sender.Rate(metric, value, hostname, tags)
		return
	}
	previousValue, found := s.previousRateValues[metric]
	if !found { // First submit of the rate
		s.doRate(metric, value, hostname, tags)
		return
	}
	timeDelta := s.timestamp.Sub(s.previousTimes[metric]).Seconds()
	if timeDelta == 0 {
		s.doRate(metric, value, hostname, tags)
		return
	}
	rate := (value - previousValue) / timeDelta
	if rate < capValue { // Under cap
		s.doRate(metric, value, hostname, tags)
		return
	}
	// Over cap, skipping
	log.Debugf("Dropped latest value %.0f (raw sample: %.0f) of metric %s as it was above the cap for this metric.", rate, value, metric)
	s.previousRateValues[metric] = value
	s.previousTimes[metric] = s.timestamp

	return
}

func (s *cappedSender) doRate(metric string, value float64, hostname string, tags []string) {
	s.previousRateValues[metric] = value
	s.previousTimes[metric] = s.timestamp
	s.Sender.Rate(metric, value, hostname, tags)
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
			Sender:             sender,
			previousRateValues: make(map[string]float64),
			previousTimes:      make(map[string]time.Time),
			rateCaps:           d.instance.CappedMetrics,
			timestamp:          time.Now(),
		}
	} else {
		d.cappedSender.timestamp = time.Now()
		// always refresh the base sender reference (not guaranteed to be constant)
		d.cappedSender.Sender = sender
	}

	return d.cappedSender, nil
}
