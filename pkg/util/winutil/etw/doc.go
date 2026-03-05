// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

// Package etw provides utilities for Event Tracing for Windows (ETW):
// - StopETWSession: stop an ETW trace session by name
// - ProcessETLFile: read and process events from an ETL trace file
// - Event property parsing via GetEventPropertyString
package etw
