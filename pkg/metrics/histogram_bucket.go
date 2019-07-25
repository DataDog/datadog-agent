// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package metrics

// HistogramBucket represents a prometheus/openmetrics histogram bucket
type HistogramBucket struct {
	Name       string
	Value      int
	LowerBound float64
	UpperBound float64
	Monotonic  bool
	Tags       []string
	Host       string
	Timestamp  float64
}
