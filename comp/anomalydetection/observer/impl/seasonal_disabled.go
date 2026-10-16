// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !anomalydetectiontestbench

package observerimpl

import observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"

// Seasonal detection remains an opt-in testbench experiment.
func wrapSeasonalResearchDetector(d observer.Detector, _ func() observer.Detector) observer.Detector {
	return d
}
