// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package contexttop summarizes DogStatsD context dumps.
package contexttop

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/DataDog/zstd"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

// Result contains the metrics with the most active contexts.
type Result struct {
	Metrics       []Metric `json:"metrics"`
	OtherMetrics  int      `json:"other_metrics,omitempty"`
	OtherContexts uint     `json:"other_contexts,omitempty"`
}

// Metric contains context and tag cardinality information for a metric name.
type Metric struct {
	Name           string `json:"name"`
	Contexts       uint   `json:"contexts"`
	Tags           []Tag  `json:"tags"`
	OtherTags      int    `json:"other_tags,omitempty"`
	OtherTagValues uint   `json:"other_tag_values,omitempty"`
}

// Tag contains the number of unique values observed for a metric tag key.
type Tag struct {
	Key          string `json:"key"`
	UniqueValues uint   `json:"unique_values"`
}

type metricContexts struct {
	count uint
	tags  map[string]struct{}
}

// FromFile reads a context dump and returns its top metrics.
func FromFile(filePath string, numMetrics, numTags int) (Result, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return Result{}, err
	}
	defer f.Close()

	var r io.Reader = bufio.NewReader(f)
	if strings.HasSuffix(filePath, ".zstd") {
		d := zstd.NewReader(r)
		defer d.Close()
		r = d
	}

	return FromReader(r, numMetrics, numTags)
}

// FromReader summarizes a sequence of JSON-encoded context representations.
func FromReader(r io.Reader, numMetrics, numTags int) (Result, error) {
	if numMetrics < 0 {
		return Result{}, errors.New("number of metrics must not be negative")
	}
	if numTags < 0 {
		return Result{}, errors.New("number of tags must not be negative")
	}

	metrics := make(map[string]*metricContexts)
	dec := json.NewDecoder(r)
	for {
		var repr aggregator.ContextDebugRepr
		if err := dec.Decode(&repr); err != nil {
			if err == io.EOF {
				break
			}
			return Result{}, err
		}

		m := metrics[repr.Name]
		if m == nil {
			m = &metricContexts{tags: make(map[string]struct{}, len(repr.MetricTags))}
			metrics[repr.Name] = m
		}

		m.count++
		for _, tag := range repr.MetricTags {
			m.tags[tag] = struct{}{}
		}
	}

	names := make([]string, 0, len(metrics))
	for name := range metrics {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		left := metrics[names[i]].count
		right := metrics[names[j]].count
		if left == right {
			return names[i] < names[j]
		}
		return left > right
	})

	top, rest := splitTop(names, numMetrics)
	result := Result{Metrics: make([]Metric, 0, len(top))}
	for _, name := range top {
		m := metrics[name]
		tags, otherTags, otherTagValues := summarizeTags(m.tags, numTags)
		result.Metrics = append(result.Metrics, Metric{
			Name:           name,
			Contexts:       m.count,
			Tags:           tags,
			OtherTags:      otherTags,
			OtherTagValues: otherTagValues,
		})
	}

	result.OtherMetrics = len(rest)
	for _, name := range rest {
		result.OtherContexts += metrics[name].count
	}

	return result, nil
}

func summarizeTags(tags map[string]struct{}, limit int) ([]Tag, int, uint) {
	cardinalities := make(map[string]uint)
	for tag := range tags {
		key, _, _ := strings.Cut(tag, ":")
		cardinalities[key]++
	}

	keys := make([]string, 0, len(cardinalities))
	for key := range cardinalities {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left := cardinalities[keys[i]]
		right := cardinalities[keys[j]]
		if left == right {
			return keys[i] < keys[j]
		}
		return left > right
	})

	top, rest := splitTop(keys, limit)
	result := make([]Tag, 0, len(top))
	for _, key := range top {
		result = append(result, Tag{Key: key, UniqueValues: cardinalities[key]})
	}

	var otherValues uint
	for _, key := range rest {
		otherValues += cardinalities[key]
	}

	return result, len(rest), otherValues
}

func splitTop[T any](values []T, limit int) ([]T, []T) {
	// Avoid collapsing a single remaining value into an "other" row.
	if len(values) <= limit+1 {
		return values, nil
	}
	return values[:limit], values[limit:]
}
