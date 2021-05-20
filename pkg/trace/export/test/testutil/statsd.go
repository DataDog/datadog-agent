// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutil

import (
	"math"
	"sync"
	"time"
)

// MetricsArgs represents arguments to a StatsClient Gauge method call.
type MetricsArgs struct {
	Name  string
	Value float64
	Tags  []string
	Rate  float64
}

// CountSummary contains a summary of all Count method calls to a particular StatsClient for a particular key.
type CountSummary struct {
	Calls []MetricsArgs
	Sum   int64
}

// GaugeSummary contains a summary of all Gauge method calls to a particular StatsClient for a particular key.
type GaugeSummary struct {
	Calls []MetricsArgs
	Last  float64
	Max   float64
}

// TestStatsClient is a mocked StatsClient that records all calls and replies with configurable error return values.
type TestStatsClient struct {
	mu sync.RWMutex

	GaugeErr       error
	GaugeCalls     []MetricsArgs
	CountErr       error
	CountCalls     []MetricsArgs
	HistogramErr   error
	HistogramCalls []MetricsArgs
	TimingErr      error
	TimingCalls    []MetricsArgs
}

// Reset resets client's internal records.
func (c *TestStatsClient) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.GaugeErr = nil
	c.GaugeCalls = c.GaugeCalls[:0]
	c.CountErr = nil
	c.CountCalls = c.CountCalls[:0]
	c.HistogramErr = nil
	c.HistogramCalls = c.HistogramCalls[:0]
	c.TimingErr = nil
	c.TimingCalls = c.TimingCalls[:0]
}

// Gauge records a call to a Gauge operation and replies with GaugeErr
func (c *TestStatsClient) Gauge(name string, value float64, tags []string, rate float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.GaugeCalls = append(c.GaugeCalls, MetricsArgs{Name: name, Value: value, Tags: tags, Rate: rate})
	return c.GaugeErr
}

// Flush implements metrics.StatsClient
func (c *TestStatsClient) Flush() error {
	// TODO
	return nil
}

// Count records a call to a Count operation and replies with CountErr
func (c *TestStatsClient) Count(name string, value int64, tags []string, rate float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.CountCalls = append(c.CountCalls, MetricsArgs{Name: name, Value: float64(value), Tags: tags, Rate: rate})
	return c.CountErr
}

// Histogram records a call to a Histogram operation and replies with HistogramErr
func (c *TestStatsClient) Histogram(name string, value float64, tags []string, rate float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.HistogramCalls = append(c.HistogramCalls, MetricsArgs{Name: name, Value: value, Tags: tags, Rate: rate})
	return c.HistogramErr
}

// Timing records a call to a Timing operation.
func (c *TestStatsClient) Timing(name string, value time.Duration, tags []string, rate float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.TimingCalls = append(c.TimingCalls, MetricsArgs{Name: name, Value: float64(value), Tags: tags, Rate: rate})
	return c.TimingErr
}

// GetCountSummaries computes summaries for all names supplied as parameters to Count calls.
func (c *TestStatsClient) GetCountSummaries() map[string]*CountSummary {
	result := map[string]*CountSummary{}

	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, countCall := range c.CountCalls {
		name := countCall.Name
		summary, ok := result[name]

		if !ok {
			summary = &CountSummary{}
			result[name] = summary
		}

		summary.Calls = append(summary.Calls, countCall)
		summary.Sum += int64(countCall.Value)
	}

	return result
}

// GetGaugeSummaries computes summaries for all names supplied as parameters to Gauge calls.
func (c *TestStatsClient) GetGaugeSummaries() map[string]*GaugeSummary {
	result := map[string]*GaugeSummary{}

	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, gaugeCall := range c.GaugeCalls {
		name := gaugeCall.Name
		summary, ok := result[name]

		if !ok {
			summary = &GaugeSummary{}
			summary.Max = math.MinInt64
			result[name] = summary
		}

		summary.Calls = append(summary.Calls, gaugeCall)
		summary.Last = gaugeCall.Value

		if gaugeCall.Value > summary.Max {
			summary.Max = gaugeCall.Value
		}
	}

	return result
}
