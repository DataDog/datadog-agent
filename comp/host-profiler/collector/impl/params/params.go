// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package params defines the interface for collector configuration parameters.
package params

// CollectorParams provides access to collector configuration parameters.
type CollectorParams interface {
	GetGoRuntimeMetrics() bool
}
