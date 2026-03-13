// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

// FanoutCorrelator produces one ActiveCorrelation per unique anomaly source,
// without requiring multi-signal co-occurrence. Every anomaly triggers its own
// correlation, making it useful when any single anomaly should produce a
// notification event.
//
// Each source's correlation expires after windowSeconds with no new anomaly
// for that source. windowSeconds == 0 disables eviction.
type FanoutCorrelator struct {
	windowSeconds   int64
	active          map[observerdef.MetricName]*observerdef.ActiveCorrelation
	currentDataTime int64
}

// Name returns the correlator name.
func (c *FanoutCorrelator) Name() string {
	return "fanout_correlator"
}

// ProcessAnomaly creates or updates the ActiveCorrelation for the anomaly's source.
func (c *FanoutCorrelator) ProcessAnomaly(a observerdef.Anomaly) {
	if a.Timestamp > c.currentDataTime {
		c.currentDataTime = a.Timestamp
	}
	if existing, ok := c.active[a.Source]; ok {
		existing.Anomalies = append(existing.Anomalies, a)
		if a.Timestamp > existing.LastUpdated {
			existing.LastUpdated = a.Timestamp
		}
	} else {
		c.active[a.Source] = &observerdef.ActiveCorrelation{
			Pattern:         "fanout:" + string(a.Source),
			Title:           "Anomaly: " + string(a.Source),
			MemberSeriesIDs: []observerdef.SeriesID{a.SourceSeriesID},
			MetricNames:     []observerdef.MetricName{a.Source},
			Anomalies:       []observerdef.Anomaly{a},
			FirstSeen:       a.Timestamp,
			LastUpdated:     a.Timestamp,
		}
	}
}

// Advance evicts correlations whose last anomaly is older than windowSeconds.
func (c *FanoutCorrelator) Advance(dataTime int64) {
	if dataTime > c.currentDataTime {
		c.currentDataTime = dataTime
	}
	if c.windowSeconds == 0 {
		return
	}
	cutoff := c.currentDataTime - c.windowSeconds
	for src, ac := range c.active {
		if ac.LastUpdated < cutoff {
			delete(c.active, src)
		}
	}
}

// ActiveCorrelations returns a copy of all currently active per-source correlations.
func (c *FanoutCorrelator) ActiveCorrelations() []observerdef.ActiveCorrelation {
	result := make([]observerdef.ActiveCorrelation, 0, len(c.active))
	for _, ac := range c.active {
		result = append(result, observerdef.ActiveCorrelation{
			Pattern:         ac.Pattern,
			Title:           ac.Title,
			MemberSeriesIDs: append([]observerdef.SeriesID(nil), ac.MemberSeriesIDs...),
			MetricNames:     append([]observerdef.MetricName(nil), ac.MetricNames...),
			Anomalies:       append([]observerdef.Anomaly(nil), ac.Anomalies...),
			FirstSeen:       ac.FirstSeen,
			LastUpdated:     ac.LastUpdated,
		})
	}
	return result
}

// Reset clears all internal state.
func (c *FanoutCorrelator) Reset() {
	c.active = make(map[observerdef.MetricName]*observerdef.ActiveCorrelation)
	c.currentDataTime = 0
}
