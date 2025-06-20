// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package otelcolfeatures provides constants for OpenTelemetry Collector features.
package otelcolfeatures

// DefaultConverterFeatures defines the default features enabled for the otel collector converter
var DefaultConverterFeatures = []string{"infraattributes", "prometheus", "pprof", "zpages", "health_check", "ddflare"}
