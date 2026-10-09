// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

// Package ddinjectorlogs defines the DDInjector ETW logs forwarding component.
package ddinjectorlogs

// team: windows-products

// Component forwards selected DDInjector ETW events as Agent Telemetry logs.
type Component interface {
}
