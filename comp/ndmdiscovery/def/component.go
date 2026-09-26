// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package ndmdiscovery sweeps the IP ranges configured over Remote
// Configuration and reports the devices it finds to Network Device
// Monitoring. It never schedules a check: discovery is report-only.
package ndmdiscovery

// team: network-device-monitoring-core

// Component is the component type.
type Component interface {
	// RangeCount is the number of autodiscovery ranges currently configured
	// from Remote Configuration.
	RangeCount() int
}
