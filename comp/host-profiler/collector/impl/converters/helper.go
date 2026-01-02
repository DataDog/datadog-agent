// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package converters implements the converters for the host profiler collector.
package converters

func ddprofilingName() string {
	return "ddprofiling/default"
}

func hpflareName() string {
	return "hpflare/default"
}
