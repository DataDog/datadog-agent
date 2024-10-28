// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

// (only used in tests) reset the tagset telemetry to zeroes
func (t *tagsetTelemetry) reset() {
	for i := range t.sizeThresholds {
		t.hugeSeriesCount[i].Store(0)
		t.hugeSketchesCount[i].Store(0)
	}
}
