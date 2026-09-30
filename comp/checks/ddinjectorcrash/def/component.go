// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

// Package ddinjectorcrash defines the DDInjector crash telemetry component.
package ddinjectorcrash

// team: windows-products

// Component listens for DDInjector crash events and reports them through Agent Telemetry.
type Component interface {
}
