// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package healthprobe implements the health check server
package healthprobe

// team: agent-shared-components

// Component is the component type.
type Component interface {
}

// Options holds the different healthprobe options
type Options struct {
	Port           int
	LogsGoroutines bool
}
