package testutil

import (
	"math"
	"sync"
)

// StatsClientGaugeArgs represents arguments to a StatsClient Gauge method call.
type StatsClientGaugeArgs struct {
	Name  string
	Value float64
	Tags  []string
	Rate  float64
}

// StatsClientCountArgs represents arguments to a StatsClient Count method call.
type StatsClientCountArgs struct {
	Name  string
	Value int64
	Tags  []string
	Rate  float64
}

// StatsClientHistogramArgs represents arguments to a StatsClient Histogram method call.
type StatsClientHistogramArgs struct {
	Name  string
	Value float64
	Tags  []string
	Rate  float64
}

// CountSummary contains a summary of all Count method calls to a particular StatsClient for a particular key.
type CountSummary struct {
	Calls []StatsClientCountArgs
	Sum   int64
}

// GaugeSummary contains a summary of all Gauge method calls to a particular StatsClient for a particular key.
type GaugeSummary struct {
	Calls []StatsClientGaugeArgs
	Last  float64
	Max   float64
}

// TestStatsClient is a mocked StatsClient that records all calls and replies with configurable error return values.
type TestStatsClient struct {
	mu sync.RWMutex

	GaugeErr       error
	GaugeCalls     []StatsClientGaugeArgs
	CountErr       error
	CountCalls     []StatsClientCountArgs
	HistogramErr   error
	HistogramCalls []StatsClientHistogramArgs
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
}

// Gauge records a call to a Gauge operation and replies with GaugeErr
func (c *TestStatsClient) Gauge(name string, value float64, tags []string, rate float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.GaugeCalls = append(c.GaugeCalls, StatsClientGaugeArgs{Name: name, Value: value, Tags: tags, Rate: rate})
	return c.GaugeErr
}

// Count records a call to a Count operation and replies with CountErr
func (c *TestStatsClient) Count(name string, value int64, tags []string, rate float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.CountCalls = append(c.CountCalls, StatsClientCountArgs{Name: name, Value: value, Tags: tags, Rate: rate})
	return c.CountErr
}

// Histogram records a call to a Histogram operation and replies with HistogramErr
func (c *TestStatsClient) Histogram(name string, value float64, tags []string, rate float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.HistogramCalls = append(c.HistogramCalls, StatsClientHistogramArgs{Name: name, Value: value, Tags: tags, Rate: rate})
	return c.HistogramErr
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
		summary.Sum += countCall.Value
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
