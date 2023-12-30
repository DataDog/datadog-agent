// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

// A MatchOpt to filter fakeintake payloads
type MatchOpt[P aggregator.PayloadItem] func(payload P) (bool, error)

// WithTags filters by `tags`
func WithTags[P aggregator.PayloadItem](tags []string) MatchOpt[P] {
	return func(payload P) (bool, error) {
		if aggregator.AreTagsSubsetOfOtherTags(tags, payload.GetTags()) {
			return true, nil
		}
		// TODO return similarity error score
		return false, nil
	}
}

// WithMetricValueInRange filters metrics with values in range `minValue < value < maxValue`
func WithMetricValueInRange(minValue float64, maxValue float64) MatchOpt[*aggregator.MetricSeries] {
	return func(metric *aggregator.MetricSeries) (bool, error) {
		isMatch, err := WithMetricValueHigherThan(minValue)(metric)
		if !isMatch || err != nil {
			return isMatch, err
		}
		return WithMetricValueLowerThan(maxValue)(metric)
	}
}

// WithMetricValueLowerThan filters metrics with values lower than `maxValue`
func WithMetricValueLowerThan(maxValue float64) MatchOpt[*aggregator.MetricSeries] {
	return func(metric *aggregator.MetricSeries) (bool, error) {
		for _, point := range metric.Points {
			if point.Value < maxValue {
				return true, nil
			}
		}
		// TODO return similarity error score
		return false, nil
	}
}

// WithMetricValueLowerThan filters metrics with values higher than `minValue`
//
//nolint:revive // TODO(APL) Fix revive linter
func WithMetricValueHigherThan(minValue float64) MatchOpt[*aggregator.MetricSeries] {
	return func(metric *aggregator.MetricSeries) (bool, error) {
		for _, point := range metric.Points {
			if point.Value > minValue {
				return true, nil
			}
		}
		// TODO return similarity error score
		return false, nil
	}
}

// WithMessageContaining filters logs by message containing `content`
func WithMessageContaining(content string) MatchOpt[*aggregator.Log] {
	return func(log *aggregator.Log) (bool, error) {
		if strings.Contains(log.Message, content) {
			return true, nil
		}
		// TODO return similarity score in error
		return false, nil
	}
}

// WithMessageMatching filters logs by message matching [regexp](https://pkg.go.dev/regexp) `pattern`
func WithMessageMatching(pattern string) MatchOpt[*aggregator.Log] {
	return func(log *aggregator.Log) (bool, error) {
		matched, err := regexp.MatchString(pattern, log.Message)
		if err != nil {
			return false, err
		}
		if matched {
			return true, nil
		}
		// TODO return similarity score in error
		return false, nil
	}
}
