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

// MetricFamily represents a metric family that is returned by a prometheus endpoint
type MetricFamily struct {
	Name    string
	Type    string
	Samples model.Vector
}

// trimMetricSuffix extracts the base metric name by removing known suffixes.
// Strips histogram/summary suffixes (_bucket, _sum, _count)
func trimMetricSuffix(name string) string {
	for _, suffix := range []string{"_bucket", "_sum", "_count"} {
		if trimmed := strings.TrimSuffix(name, suffix); trimmed != name {
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
func ParseMetricsWithFilter(data []byte, filter []string) ([]*MetricFamily, error) {
	data = preprocessData(data, filter)

	st := labels.NewSymbolTable()
	parser := textparse.NewPromParser(data, st, false)

	families := make(map[string]*MetricFamily)

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
			name, typ := parser.Type()
			baseName := trimMetricSuffix(string(name))
			if fam, ok := families[baseName]; ok {
				fam.Type = strings.ToUpper(string(typ))
			} else {
				families[baseName] = &MetricFamily{
					Name:    baseName,
					Type:    strings.ToUpper(string(typ)),
					Samples: make(model.Vector, 0, 8),
				}
			}

		case textparse.EntrySeries:
			_, ts, value := parser.Series()
			parser.Labels(&lbls)

			// Get metric name from __name__ label
			fullName := lbls.Get(labels.MetricName)

			// Convert labels to model.Metric
			metric := make(model.Metric, lbls.Len())
			lbls.Range(func(l labels.Label) {
				metric[model.LabelName(l.Name)] = model.LabelValue(l.Value)
			})

			// Create sample
			sample := &model.Sample{
				Metric: metric,
				Value:  model.SampleValue(value),
			}
			if ts != nil {
				sample.Timestamp = model.TimeFromUnixNano(*ts * 1e6)
			}

			// Group by base metric name
			baseName := trimMetricSuffix(fullName)

			family, exists := families[baseName]
			if !exists {
				family = &MetricFamily{
					Name:    baseName,
					Type:    "UNTYPED",
					Samples: make(model.Vector, 0, 8),
				}
				families[baseName] = family
			}
			family.Samples = append(family.Samples, sample)
		}
	}

	result := make([]*MetricFamily, 0, len(families))
	for _, fam := range families {
		if len(fam.Samples) > 0 {
			result = append(result, fam)
		}
	}

	return result, nil
}

// ParseMetrics parses prometheus-formatted metrics from the input data.
func ParseMetrics(data []byte) ([]*MetricFamily, error) {
	return ParseMetricsWithFilter(data, nil)
}
