// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package check

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
	telemetry_utils "github.com/DataDog/datadog-agent/pkg/telemetry/utils"
)

const (
	runCheckFailureTag = "fail"
	runCheckSuccessTag = "ok"
)

var (
	tlmRuns = telemetry.NewCounter("checks", "runs",
		[]string{"check_name", "state"}, "Check runs")
	tlmWarnings = telemetry.NewCounter("checks", "warnings",
		[]string{"check_name"}, "Check warnings")
	tlmMetricsSamples = telemetry.NewCounter("checks", "metrics_samples",
		[]string{"check_name"}, "Metrics count")
	tlmEvents = telemetry.NewCounter("checks", "events",
		[]string{"check_name"}, "Events count")
	tlmServices = telemetry.NewCounter("checks", "services_checks",
		[]string{"check_name"}, "Service checks count")
	tlmExecutionTime = telemetry.NewGauge("checks", "execution_time",
		[]string{"check_name"}, "Check execution time")
)

// Stats holds basic runtime statistics about check instances
type Stats struct {
	CheckName            string
	CheckVersion         string
	CheckConfigSource    string
	CheckID              ID
	TotalRuns            uint64
	TotalErrors          uint64
	TotalWarnings        uint64
	MetricSamples        int64
	Events               int64
	ServiceChecks        int64
	TotalMetricSamples   uint64
	TotalEvents          uint64
	TotalServiceChecks   uint64
	ExecutionTimes       [32]int64 // circular buffer of recent run durations, most recent at [(TotalRuns+31) % 32]
	AverageExecutionTime int64     // average run duration
	LastExecutionTime    int64     // most recent run duration, provided for convenience
	LastSuccessDate      int64     // most recent successful execution date, unix timestamp in seconds
	LastError            string    // error that occurred in the last run, if any
	LastWarnings         []string  // warnings that occurred in the last run, if any
	UpdateTimestamp      int64     // latest update to this instance, unix timestamp in seconds
	m                    sync.Mutex
	telemetry            bool // do we want telemetry on this Check
}

// NewStats returns a new check stats instance
func NewStats(c Check) *Stats {
	stats := Stats{
		CheckID:           c.ID(),
		CheckName:         c.String(),
		CheckVersion:      c.Version(),
		CheckConfigSource: c.ConfigSource(),
		telemetry:         telemetry_utils.IsCheckEnabled(c.String()),
	}

	// We are interested in a check's run state values even when they are 0 so we
	// initialize them here explicitly
	if stats.telemetry && telemetry_utils.IsEnabled() {
		tlmRuns.Initialize(stats.CheckName, runCheckFailureTag)
		tlmRuns.Initialize(stats.CheckName, runCheckSuccessTag)
	}

	return &stats
}

// Add tracks a new execution time
func (cs *Stats) Add(t time.Duration, err error, warnings []error, metricStats map[string]int64) {
	cs.m.Lock()
	defer cs.m.Unlock()

	// store execution times in Milliseconds
	tms := t.Nanoseconds() / 1e6
	cs.LastExecutionTime = tms
	cs.ExecutionTimes[cs.TotalRuns%uint64(len(cs.ExecutionTimes))] = tms
	cs.TotalRuns++
	if cs.telemetry {
		tlmExecutionTime.Set(float64(tms), cs.CheckName)
	}
	var totalExecutionTime int64
	ringSize := cs.TotalRuns
	if ringSize > uint64(len(cs.ExecutionTimes)) {
		ringSize = uint64(len(cs.ExecutionTimes))
	}
	for i := uint64(0); i < ringSize; i++ {
		totalExecutionTime += cs.ExecutionTimes[i]
	}
	cs.AverageExecutionTime = totalExecutionTime / int64(ringSize)
	if err != nil {
		cs.TotalErrors++
		if cs.telemetry {
			tlmRuns.Inc(cs.CheckName, runCheckFailureTag)
		}
		cs.LastError = err.Error()
	} else {
		if cs.telemetry {
			tlmRuns.Inc(cs.CheckName, runCheckSuccessTag)
		}
		cs.LastError = ""
		cs.LastSuccessDate = time.Now().Unix()
	}
	cs.LastWarnings = []string{}
	if len(warnings) != 0 {
		if cs.telemetry {
			tlmWarnings.Add(float64(len(warnings)), cs.CheckName)
		}
		for _, w := range warnings {
			cs.TotalWarnings++
			cs.LastWarnings = append(cs.LastWarnings, w.Error())
		}
	}
	cs.UpdateTimestamp = time.Now().Unix()

	if m, ok := metricStats["MetricSamples"]; ok {
		cs.MetricSamples = m
		cs.TotalMetricSamples += uint64(m)
		if cs.telemetry {
			tlmMetricsSamples.Add(float64(m), cs.CheckName)
		}
	}
	if ev, ok := metricStats["Events"]; ok {
		cs.Events = ev
		cs.TotalEvents += uint64(ev)
		if cs.telemetry {
			tlmEvents.Add(float64(ev), cs.CheckName)
		}
	}
	if sc, ok := metricStats["ServiceChecks"]; ok {
		cs.ServiceChecks = sc
		cs.TotalServiceChecks += uint64(sc)
		if cs.telemetry {
			tlmServices.Add(float64(sc), cs.CheckName)
		}
	}
}
