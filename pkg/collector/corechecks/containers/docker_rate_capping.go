// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package containers

import (
	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

// cappedSender wraps around the standard Sender and overrides
// the Rate method to implement rate capping
type cappedSender struct {
	aggregator.Sender
	previousRateValues map[string]float64
	rateCaps           map[string]float64
}

// Rate checks the rate value against the `capped_metrics` configuration
// to filter out buggy spikes coming for cgroup cpu accounting
func (s *cappedSender) Rate(metric string, value float64, hostname string, tags []string) {
	capValue, found := s.rateCaps[metric]
	if !found { // Metric not capped
		s.Sender.Rate(metric, value, hostname, tags)
		return
	}
	previousValue, found := s.previousRateValues[metric]
	if !found { // First submit of the rate
		s.previousRateValues[metric] = value
		s.Sender.Rate(metric, value, hostname, tags)
		return
	}
	delta := value - previousValue
	if delta < capValue { // Under cap
		s.previousRateValues[metric] = value
		s.Sender.Rate(metric, value, hostname, tags)
		return
	}
	// Over cap
	log.Debugf("Dropped latest value %.0f (raw sample: %.0f) of metric %s as it was above the cap for this metric.", delta, value, metric)
	s.previousRateValues[metric] = value

	return
}

func (d *DockerCheck) updateCappedSender() (aggregator.Sender, error) {
	sender, err := aggregator.GetSender(d.ID())
	if err != nil {
		return sender, err
	}

	if d.cappedSender == nil {
		d.cappedSender = &cappedSender{
			Sender:             sender,
			previousRateValues: make(map[string]float64),
			rateCaps:           d.instance.CappedMetrics,
		}
	} else {
		// always refresh the base sender reference (not guaranteed to be constant)
		d.cappedSender.Sender = sender
	}

	return d.cappedSender, nil
}
