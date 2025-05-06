// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless
// +build !serverless

package server

import "github.com/DataDog/datadog-agent/pkg/metrics"

// GetDefaultMetricSource returns the default metric source based on build tags
func GetDefaultMetricSource() metrics.MetricSource {
	return metrics.MetricSourceDogstatsd
}
