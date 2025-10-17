// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package implnoop

import telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"

// Prometheus implements histograms using Prometheus.
type simpleNoOpHistogram struct {
}

// Observe the value to the Histogram value.
func (s *simpleNoOpHistogram) Observe(float64) {}

func (s *simpleNoOpHistogram) Get() telemetry.HistogramValue {
	return telemetry.HistogramValue{}
}
