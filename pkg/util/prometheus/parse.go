// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

/*
Package prometheus provides utility functions to deal with prometheus endpoints
*/
package prometheus

import (
	"bytes"
	"errors"
	"io"
	"strings"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/textparse"
)

// Metric is a set of labels for a sample.
type Metric map[string]string

// Sample represents a single metric data point.
type Sample struct {
	Metric    Metric
	Value     float64
	Timestamp int64 // milliseconds since epoch, 0 if not set
}

// MetricFamily represents a metric family that is returned by a prometheus endpoint.
type MetricFamily struct {
	Name    string
	Type    string
	Samples []Sample
}

// trimHistogramSuffix removes histogram-specific suffixes (_bucket, _sum, _count).
func trimHistogramSuffix(name string) string {
	for _, suffix := range []string{"_bucket", "_sum", "_count"} {
		if trimmed, ok := strings.CutSuffix(name, suffix); ok {
			return trimmed
		}
	}
	return name
}

// trimSummarySuffix removes summary-specific suffixes (_sum, _count).
func trimSummarySuffix(name string) string {
	for _, suffix := range []string{"_sum", "_count"} {
		if trimmed, ok := strings.CutSuffix(name, suffix); ok {
			return trimmed
		}
	}
	return name
}

// preprocessData normalizes lines and filters out lines matching any filter string.
func preprocessData(data []byte, filter []string) []byte {
	lines := bytes.Split(data, []byte{'\n'})
	filteredLines := make([][]byte, 0, len(lines))
	for _, line := range lines {
		line = bytes.TrimRight(bytes.TrimLeft(line, " \t"), "\r")
		// Filter lines containing any filter string
		skip := false
		for _, f := range filter {
			if bytes.Contains(line, []byte(f)) {
				skip = true
				break
			}
		}
		if !skip {
			filteredLines = append(filteredLines, line)
		}
	}
	return bytes.Join(filteredLines, []byte{'\n'})
}

// ParseMetricsWithFilter parses prometheus-formatted metrics from the input data, ignoring lines which contain
// text that matches the passed in filter.
func ParseMetricsWithFilter(data []byte, filter []string) ([]MetricFamily, error) {
	data = preprocessData(data, filter)

	st := labels.NewSymbolTable()
	parser := textparse.NewPromParser(data, st, false)

	var result []MetricFamily
	var lbls labels.Labels

	for {
		entry, err := parser.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}

		switch entry {
		case textparse.EntryType:
			// Discard previous family if it has no samples
			if len(result) > 0 && len(result[len(result)-1].Samples) == 0 {
				result = result[:len(result)-1]
			}
			name, typ := parser.Type()
			result = append(result, MetricFamily{
				Name:    string(name),
				Type:    strings.ToUpper(string(typ)),
				Samples: make([]Sample, 0, 8),
			})

		case textparse.EntrySeries:
			_, ts, value := parser.Series()
			parser.Labels(&lbls)

			rawName := lbls.Get(model.MetricNameLabel)

			// Fast path: check if raw name matches current family (common for COUNTER/GAUGE)
			if len(result) == 0 || result[len(result)-1].Name != rawName {
				// Slow path: try trimming suffix based on current family type
				name := rawName
				if len(result) > 0 {
					switch result[len(result)-1].Type {
					case "HISTOGRAM":
						name = trimHistogramSuffix(rawName)
					case "SUMMARY":
						name = trimSummarySuffix(rawName)
					}
				}

				// If still no match, create a new UNTYPED family
				if len(result) == 0 || result[len(result)-1].Name != name {
					// Discard previous family if it has no samples
					if len(result) > 0 && len(result[len(result)-1].Samples) == 0 {
						result = result[:len(result)-1]
					}
					result = append(result, MetricFamily{
						Name:    name,
						Type:    "UNTYPED",
						Samples: make([]Sample, 0, 8),
					})
				}
			}

			// Convert labels to Metric
			metric := make(Metric, lbls.Len())
			lbls.Range(func(l labels.Label) {
				metric[l.Name] = l.Value
			})

			// Create sample
			sample := Sample{
				Metric: metric,
				Value:  value,
			}
			if ts != nil {
				sample.Timestamp = *ts
			}

			result[len(result)-1].Samples = append(result[len(result)-1].Samples, sample)
		}
	}

	// Discard last family if it has no samples
	if len(result) > 0 && len(result[len(result)-1].Samples) == 0 {
		result = result[:len(result)-1]
	}

	return result, nil
}

// ParseMetrics parses prometheus-formatted metrics from the input data.
func ParseMetrics(data []byte) ([]MetricFamily, error) {
	return ParseMetricsWithFilter(data, nil)
}
