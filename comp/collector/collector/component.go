// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package collector defines the collector component.
package collector

import (
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
)

// team: agent-metrics-logs

// EventType represents the type of events emitted by the collector
type EventType uint32

const (
	// CheckRun is emitted when a check is added to the collector
	CheckRun EventType = iota
	// CheckStop is emitted when a check is stopped and removed from the collector
	CheckStop
)

// EventReceiver represents a function to receive notification from the collector when running or stopping checks.
type EventReceiver func(checkid.ID, EventType)

// Component is the component type.
type Component interface {
	// RunCheck sends a Check in the execution queue
	RunCheck(inner check.Check) (checkid.ID, error)
	// StopCheck halts a check and remove the instance
	StopCheck(id checkid.ID) error
	// MapOverChecks call the callback with the list of checks locked.
	MapOverChecks(cb func([]check.Info))
	// GetChecks copies checks
	GetChecks() []check.Check
	// GetAllInstanceIDs returns the ID's of all instances of a check
	GetAllInstanceIDs(checkName string) []checkid.ID
	// ReloadAllCheckInstances completely restarts a check with a new configuration and returns a list of killed check IDs
	ReloadAllCheckInstances(name string, newInstances []check.Check) ([]checkid.ID, error)
	// AddEventReceiver adds a callback to the collector to be called each time a check is added or removed.
	AddEventReceiver(cb EventReceiver)
}
