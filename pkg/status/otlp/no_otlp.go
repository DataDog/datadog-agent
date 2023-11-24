// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !otlp

// Package otlp fetch information from otlp collector
package otlp

import "github.com/DataDog/datadog-agent/comp/otelcol/collector"

// SetOtelCollector sets the active OTEL Collector.
func SetOtelCollector(_ collector.Component) {}

// PopulateStatus populates stats with otlp information.
func PopulateStatus(_ map[string]interface{}) {}
