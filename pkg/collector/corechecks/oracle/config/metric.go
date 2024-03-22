// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package config

// MetricConfig holds the config for a metric.
type MetricConfig struct {
	Query   string `yaml:"query"`
	Columns string `yaml:"columns"`
	// Tags    string `yaml:"tags"`
}
