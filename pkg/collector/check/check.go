// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package check

import (
	"regexp"
	"sync"
	"time"
)

const (
	// DefaultCheckInterval is the interval in seconds the scheduler should apply
	// when no value was provided in Check configuration.
	DefaultCheckInterval time.Duration = 15 * time.Second
)

var (
	tplVarRegex = regexp.MustCompile(`%%.+?%%`)

	tplVars = []string{
		"host",
		"pid",
		"port",
		"container-name",
		"tags",
	}
)

// JMXChecks list of JMXFetch checks supported
var JMXChecks = []string{
	"activemq",
	"activemq_58",
	"cassandra",
	"jmx",
	"solr",
	"tomcat",
	"kafka",
}

// Check is an interface for types capable to run checks
type Check interface {
	Run() error                                    // run the check
	Stop()                                         // stop the check if it's running
	String() string                                // provide a printable version of the check name
	Configure(config, initConfig ConfigData) error // configure the check from the outside
	Interval() time.Duration                       // return the interval time for the check
	ID() ID                                        // provide a unique identifier for every check instance
	GetWarnings() []error                          // return the last warning registered by the check
	GetMetricStats() (map[string]int64, error)     // get metric stats from the sender
}

// Stats holds basic runtime statistics about check instances
type Stats struct {
	CheckName            string
	CheckID              ID
	TotalRuns            uint64
	TotalErrors          uint64
	TotalWarnings        uint64
	Metrics              int64
	Events               int64
	ServiceChecks        int64
	TotalMetrics         int64
	TotalEvents          int64
	TotalServiceChecks   int64
	ExecutionTimes       [32]int64 // circular buffer of recent run durations, most recent at [(TotalRuns+31) % 32]
	AverageExecutionTime int64     // average run duration
	LastExecutionTime    int64     // most recent run duration, provided for convenience
	LastError            string    // error that occurred in the last run, if any
	LastWarnings         []string  // warnings that occurred in the last run, if any
	UpdateTimestamp      int64     // latest update to this instance, unix timestamp in seconds
	m                    sync.Mutex
}

// NewStats returns a new check stats instance
func NewStats(c Check) *Stats {
	return &Stats{
		CheckID:   c.ID(),
		CheckName: c.String(),
	}
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
		cs.LastError = err.Error()
	} else {
		cs.LastError = ""
	}
	cs.LastWarnings = []string{}
	if len(warnings) != 0 {
		for _, w := range warnings {
			cs.TotalWarnings++
			cs.LastWarnings = append(cs.LastWarnings, w.Error())
		}
	}
	cs.UpdateTimestamp = time.Now().Unix()

	if m, ok := metricStats["Metrics"]; ok {
		cs.Metrics = m
		if cs.TotalMetrics <= 1000001 {
			cs.TotalMetrics += m
		}
	}
	if ev, ok := metricStats["Events"]; ok {
		cs.Events = ev
		if cs.TotalEvents <= 1000001 {
			cs.TotalEvents += ev
		}
	}
	if sc, ok := metricStats["ServiceChecks"]; ok {
		cs.ServiceChecks = sc
		if cs.TotalServiceChecks <= 1000001 {
			cs.TotalServiceChecks += sc
		}
	}
}
