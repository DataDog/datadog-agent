// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metric_history

// HistoryReader provides read access to the metric history cache.
type HistoryReader interface {
	// ListSeries returns all tracked series keys.
	ListSeries() []SeriesKey

	// GetRecent returns data points from the Recent tier for a series.
	GetRecent(key SeriesKey) []DataPoint

	// GetMedium returns data points from the Medium tier for a series.
	GetMedium(key SeriesKey) []DataPoint

	// GetLong returns data points from the Long tier for a series.
	GetLong(key SeriesKey) []DataPoint

	// GetScalarSeries extracts a scalar time series from the stats.
	// tier: TierRecent, TierMedium, or TierLong
	// aspect: "mean", "min", "max", "count", "sum"
	GetScalarSeries(key SeriesKey, tier Tier, aspect string) []TimestampedValue

	// Scan iterates over all series, calling fn for each.
	// Return false from fn to stop iteration.
	Scan(fn func(SeriesKey, *MetricHistory) bool)
}

// Ensure MetricHistoryCache implements HistoryReader
var _ HistoryReader = (*MetricHistoryCache)(nil)

// ListSeries returns all tracked series keys.
func (c *MetricHistoryCache) ListSeries() []SeriesKey {
	keys := make([]SeriesKey, 0, len(c.series))
	for _, history := range c.series {
		keys = append(keys, history.Key)
	}
	return keys
}

// GetRecent returns data points from the Recent tier for a series.
func (c *MetricHistoryCache) GetRecent(key SeriesKey) []DataPoint {
	history := c.series[key.ContextKey]
	if history == nil {
		return nil
	}
	return history.Recent.ToSlice()
}

// GetMedium returns data points from the Medium tier for a series.
func (c *MetricHistoryCache) GetMedium(key SeriesKey) []DataPoint {
	history := c.series[key.ContextKey]
	if history == nil {
		return nil
	}
	return history.Medium.ToSlice()
}

// GetLong returns data points from the Long tier for a series.
func (c *MetricHistoryCache) GetLong(key SeriesKey) []DataPoint {
	history := c.series[key.ContextKey]
	if history == nil {
		return nil
	}
	return history.Long.ToSlice()
}

// GetScalarSeries extracts a scalar time series from the stats.
// tier: TierRecent, TierMedium, or TierLong
// aspect: "mean", "min", "max", "count", "sum"
func (c *MetricHistoryCache) GetScalarSeries(key SeriesKey, tier Tier, aspect string) []TimestampedValue {
	history := c.series[key.ContextKey]
	if history == nil {
		return nil
	}

	// Select the appropriate buffer based on tier
	var buffer *RingBuffer[DataPoint]
	switch tier {
	case TierRecent:
		buffer = history.Recent
	case TierMedium:
		buffer = history.Medium
	case TierLong:
		buffer = history.Long
	default:
		return nil
	}

	// Extract the data points
	dataPoints := buffer.ToSlice()
	if len(dataPoints) == 0 {
		return nil
	}

	// Convert to timestamped values based on the requested aspect
	result := make([]TimestampedValue, len(dataPoints))
	for i, dp := range dataPoints {
		result[i].Timestamp = dp.Timestamp

		switch aspect {
		case "mean":
			result[i].Value = dp.Stats.Mean()
		case "min":
			result[i].Value = dp.Stats.Min
		case "max":
			result[i].Value = dp.Stats.Max
		case "count":
			result[i].Value = float64(dp.Stats.Count)
		case "sum":
			result[i].Value = dp.Stats.Sum
		default:
			// Default to mean if aspect is not recognized
			result[i].Value = dp.Stats.Mean()
		}
	}

	return result
}

// Scan iterates over all series, calling fn for each.
// Return false from fn to stop iteration.
func (c *MetricHistoryCache) Scan(fn func(SeriesKey, *MetricHistory) bool) {
	for _, history := range c.series {
		if !fn(history.Key, history) {
			return
		}
	}
}
