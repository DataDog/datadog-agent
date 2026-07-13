// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package exampletaskmanager provides a minimal RC-to-check task bridge.
//
// The component subscribes to the DEBUG remote-config product. Each payload
// may contain a task named "trigger"; when present, it schedules a one-shot
// example shared-library check via autodiscovery.
package exampletaskmanager

// team: sensitive-data-scanner

// Component is the example task manager interface.
type Component interface{}
