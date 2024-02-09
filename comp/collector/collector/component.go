// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package collector defines the collector component.
package collector

import (
	"github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
)

// team: agent-metrics-logs

// Component is the component type.
type Component interface {
	// Start begins the collector's operation.  The scheduler will not run any checks until this has been called.
	Start()
	// Stop halts any component involved in running a Check
	Stop()
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
	// ReloadAllCheckInstances completely restarts a check with a new configuration
	ReloadAllCheckInstances(name string, newInstances []check.Check) ([]checkid.ID, error)
	// AddEventReceiver adds a callback to the collector to be called each time a check is added or removed.
	AddEventReceiver(cb collector.EventReceiver)
}
