// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"github.com/DataDog/datadog-agent/pkg/util/stats_tracker"
)

// StatsTracker Keeps track of simple stats over its lifetime and a configurable time range.
// StatsTracker is designed to be memory efficient by aggregating data into buckets. For example
// a time frame of 24 hours with a bucketFrame of 1 hour will ensure that only 24 points are ever
// kept in memory. New data is considered in the stats immediately while old data is removed by
// dropping expired aggregated buckets.
type StatsTracker = stats_tracker.StatsTracker

// NewStatsTracker Creates a new StatsTracker instance
var NewStatsTracker = stats_tracker.NewStatsTracker

// NewStatsTrackerWithTimeProvider Creates a new StatsTracker instance with a time provider closure (mostly for testing)
var NewStatsTrackerWithTimeProvider = stats_tracker.NewStatsTrackerWithTimeProvider

// Add Records a new value to the stats tracker
var Add = (*stats_tracker.StatsTracker).Add

// AllTimeAvg Gets the all time average of values seen so far
var AllTimeAvg = (*stats_tracker.StatsTracker).AllTimeAvg

// MovingAvg Gets the moving average of values within the time frame
var MovingAvg = (*stats_tracker.StatsTracker).MovingAvg

// AllTimePeak Gets the largest value seen so far
var AllTimePeak = (*stats_tracker.StatsTracker).AllTimePeak

// MovingPeak Gets the largest value seen within the time frame
var MovingPeak = (*stats_tracker.StatsTracker).MovingPeak

var InfoKey = (*stats_tracker.StatsTracker).InfoKey

var Info = (*stats_tracker.StatsTracker).Info
