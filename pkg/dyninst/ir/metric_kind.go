// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ir

//go:generate go run golang.org/x/tools/cmd/stringer -type=MetricKind -trimprefix=MetricKind -output metric_kind_string.go

// MetricKind is the kind of metric that a metric probe emits.
type MetricKind uint8

const (
	_ MetricKind = iota
	// MetricKindCount sums the probe's values, or counts the probe's hits
	// when the probe has no value expression.
	MetricKindCount
	// MetricKindGauge reports the most recent value.
	MetricKindGauge
	// MetricKindHistogram reports a histogram of the values.
	MetricKindHistogram
	// MetricKindDistribution reports a distribution of the values.
	MetricKindDistribution
)
