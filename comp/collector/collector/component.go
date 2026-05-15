// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package collector defines the collector component.
//
// Deprecated: use comp/collector/collector/def instead.
package collector

import (
	collector "github.com/DataDog/datadog-agent/comp/collector/collector/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-runtimes

// EventType represents the type of events emitted by the collector.
//
// Deprecated: use comp/collector/collector/def.EventType instead.
type EventType = collector.EventType

const (
	// CheckRun is emitted when a check is added to the collector.
	//
	// Deprecated: use comp/collector/collector/def.CheckRun instead.
	CheckRun = collector.CheckRun
	// CheckStop is emitted when a check is stopped and removed from the collector.
	//
	// Deprecated: use comp/collector/collector/def.CheckStop instead.
	CheckStop = collector.CheckStop
)

// EventReceiver represents a function to receive notification from the collector when running or stopping checks.
//
// Deprecated: use comp/collector/collector/def.EventReceiver instead.
type EventReceiver = collector.EventReceiver

// Component is the component type.
//
// Deprecated: use comp/collector/collector/def.Component instead.
type Component = collector.Component

// NoneModule return a None optional type for Component.
//
// This helper allows code that needs a disabled Optional type for the collector to get it. The helper is split from
// the implementation to avoid linking with the implementation.
//
// Deprecated: use comp/collector/collector/def.NoneModule instead.
func NoneModule() fxutil.Module {
	return collector.NoneModule()
}
