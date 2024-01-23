// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package stats

import (
	"sync"
	"time"

	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

const (
	runCheckFailureTag = "fail"
	runCheckSuccessTag = "ok"
)

// EventPlatformNameTranslations contains human readable translations for event platform event types
var EventPlatformNameTranslations = map[string]string{
	"dbm-samples":                "Database Monitoring Query Samples",
	"dbm-metrics":                "Database Monitoring Query Metrics",
	"dbm-activity":               "Database Monitoring Activity Samples",
	"dbm-metadata":               "Database Monitoring Metadata Samples",
	"network-devices-metadata":   "Network Devices Metadata",
	"network-devices-netflow":    "Network Devices NetFlow",
	"network-devices-snmp-traps": "SNMP Traps",
}

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
	tlmHistogramBuckets = telemetry.NewCounter("checks", "histogram_buckets",
		[]string{"check_name"}, "Histogram buckets count")
	tlmExecutionTime = telemetry.NewGauge("checks", "execution_time",
		[]string{"check_name"}, "Check execution time")
	tlmCheckDelay = telemetry.NewGauge("checks",
		"delay",
		[]string{"check_name"},
		"Check start time delay relative to the previous check run")
)

// SenderStats contains statistics showing the count of various types of telemetry sent by a check sender
type SenderStats struct {
	MetricSamples    int64
	Events           int64
	ServiceChecks    int64
	HistogramBuckets int64
	// EventPlatformEvents tracks the number of events submitted for each eventType
	EventPlatformEvents map[string]int64
}

// NewSenderStats creates a new SenderStats
func NewSenderStats() SenderStats {
	panic("not called")
}

// Copy creates a copy of the current SenderStats
func (s SenderStats) Copy() (result SenderStats) {
	panic("not called")
}

// Stats holds basic runtime statistics about check instances
type Stats struct {
	CheckName                string
	CheckVersion             string
	CheckConfigSource        string
	CheckID                  checkid.ID
	Interval                 time.Duration
	TotalRuns                uint64
	TotalErrors              uint64
	TotalWarnings            uint64
	MetricSamples            int64
	Events                   int64
	ServiceChecks            int64
	HistogramBuckets         int64
	TotalMetricSamples       uint64
	TotalEvents              uint64
	TotalServiceChecks       uint64
	TotalHistogramBuckets    uint64
	EventPlatformEvents      map[string]int64
	TotalEventPlatformEvents map[string]int64
	ExecutionTimes           [32]int64 // circular buffer of recent run durations, most recent at [(TotalRuns+31) % 32]
	AverageExecutionTime     int64     // average run duration
	LastExecutionTime        int64     // most recent run duration, provided for convenience
	LastSuccessDate          int64     // most recent successful execution date, unix timestamp in seconds
	LastError                string    // error that occurred in the last run, if any
	LastDelay                int64     // most recent check start time delay relative to the previous check run, in seconds
	LastWarnings             []string  // warnings that occurred in the last run, if any
	UpdateTimestamp          int64     // latest update to this instance, unix timestamp in seconds
	m                        sync.Mutex
	telemetry                bool // do we want telemetry on this Check
}

//nolint:revive // TODO(AML) Fix revive linter
type StatsCheck interface {
	// String provides a printable version of the check name
	String() string
	// ID provides a unique identifier for every check instance
	ID() checkid.ID
	// Version returns the version of the check if available
	Version() string
	//Interval returns the interval time for the check
	Interval() time.Duration
	// ConfigSource returns the configuration source of the check
	ConfigSource() string
}

// NewStats returns a new check stats instance
func NewStats(c StatsCheck) *Stats {
	panic("not called")
}

// Add tracks a new execution time
func (cs *Stats) Add(t time.Duration, err error, warnings []error, metricStats SenderStats) {
	panic("not called")
}

type aggStats struct {
	EventPlatformEvents       map[string]interface{}
	EventPlatformEventsErrors map[string]interface{}
	Other                     map[string]interface{} `mapstructure:",remain"`
}

func translateEventTypes(original map[string]interface{}) map[string]interface{} {
	panic("not called")
}

// TranslateEventPlatformEventTypes translates the event platform event types in aggregator stats to human readable names
func TranslateEventPlatformEventTypes(aggregatorStats interface{}) (interface{}, error) {
	panic("not called")
}
