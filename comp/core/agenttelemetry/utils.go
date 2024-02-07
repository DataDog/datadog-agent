// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttelemetry

import (
	"fmt"
	"strings"

	"github.com/itchyny/gojq"
	dto "github.com/prometheus/client_model/go"
)

type jBuilder struct {
	jqSource    *gojq.Query
	jpathTarget []string
}

// compileJBuilders compiles a map of JSON path and JQ query into a slice of jBuilder
func compileJBuilders(params map[string]string) ([]jBuilder, error) {
	var builders []jBuilder
	for jpath, query := range params {
		// Validate JSON/attribute path
		jpaths := strings.Split(jpath, ".")
		if len(jpaths) < 2 {
			return nil, fmt.Errorf("jpath `%s` should contain at leat two elements", jpath)
		}

		// Compile JQ expression
		q, err := gojq.Parse(query)
		if err != nil {
			return nil, fmt.Errorf("failed to parse jq query %s for jpath '%s': %s", query, jpath, err.Error())
		}

		builder := jBuilder{jqSource: q, jpathTarget: jpaths}
		builders = append(builders, builder)
	}

	return builders, nil
}

// select only supported metric types
func isSupportedMetricType(mt dto.MetricType) bool {
	return mt == dto.MetricType_COUNTER || mt == dto.MetricType_GAUGE || mt == dto.MetricType_HISTOGRAM
}

// isZeroValueMetric checks if a metric is a zero value metric
func isZeroValueMetric(mt dto.MetricType, m *dto.Metric) bool {
	switch mt {
	case dto.MetricType_COUNTER:
		if m.GetCounter().GetValue() == 0 {
			return true
		}
	case dto.MetricType_GAUGE:
		if m.GetGauge().GetValue() == 0 {
			return true
		}
	case dto.MetricType_HISTOGRAM:
		// makes sure that all buckets are not zero (sufficient to check the last one)
		h := m.GetHistogram()
		c := len(h.GetBucket())
		if c == 0 || h.GetBucket()[c-1].GetCumulativeCount() == 0 {
			return true
		}
	}

	return false
}

// areTagsMatching checks if the metric tags match the given set of tags
// It is currently used to filter (exclude) metrics based on tags.
func areTagsMatching(metricTags []*dto.LabelPair, matchTags map[string]interface{}) bool {
	if len(metricTags) == 0 || len(matchTags) == 0 {
		return false
	}

	for _, tv := range metricTags {
		if valToMatch, ok := matchTags[tv.GetName()]; ok {
			// If matching tag value is not specified then there is match if the tag exists
			if _, ok := valToMatch.(struct{}); ok {
				return true
			}

			// valToMatch is a map of tag values, check if we now have a match
			if valsToMatch, ok := valToMatch.(map[string]struct{}); ok {
				if _, ok := valsToMatch[tv.GetValue()]; ok {
					return true
				}
			}
		}
	}

	return false
}
