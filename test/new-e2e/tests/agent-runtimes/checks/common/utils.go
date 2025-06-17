// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package checktests contains e2e tests for core checks
package common

import (
	"cmp"
	"math"
	"slices"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/testcommon/check"
)

// metric payload output comparison
// metric output comparison
// tag output comparison
// format config yaml from struct

func EqualMetrics(a, b check.Metric) bool {
	return a.Host == b.Host &&
		a.Interval == b.Interval &&
		a.Metric == b.Metric &&
		a.SourceTypeName == b.SourceTypeName &&
		a.Type == b.Type && gocmp.Equal(a.Tags, b.Tags, gocmpopts.SortSlices(cmp.Less[string]))
}

func CompareValuesWithRelativeMargin(a, b, p, fraction float64) bool {
	x := math.Round(a*p) / p
	y := math.Round(b*p) / p
	relMarg := fraction * math.Abs(x)
	return math.Abs(x-y) <= relMarg
}

func MetricPayloadCompare(a, b check.Metric) int {
	return cmp.Or(
		cmp.Compare(a.Host, b.Host),
		cmp.Compare(a.Metric, b.Metric),
		cmp.Compare(a.Type, b.Type),
		cmp.Compare(a.SourceTypeName, b.SourceTypeName),
		cmp.Compare(a.Interval, b.Interval),
		slices.Compare(a.Tags, b.Tags),
		slices.CompareFunc(a.Points, b.Points, func(a, b []float64) int {
			return slices.Compare(a, b)
		}),
	)
}
