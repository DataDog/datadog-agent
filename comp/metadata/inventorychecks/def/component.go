// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package inventorychecks implements a component to generate the 'check_metadata' metadata payload for inventory.
package inventorychecks

// team: fleet-automation

// Component is the component type.
//
// TODO: (components) - Once the collector is migrated to a component it might make more sense for this metadata provider
// to be part of the collector.
type Component interface {
	// Set sets a metadata value for one check instance
	Set(instanceID string, key string, value interface{})
	// GetInstanceMetadata returns metadata for a specific check instance
	GetInstanceMetadata(instanceID string) map[string]interface{}
	// Refresh trigger a new payload to be send while still respecting the minimal interval between two updates.
	Refresh()
	// SendNow builds and submits the payload immediately, without waiting for the next collection. It returns an
	// error if the payload is disabled, during the first run delay after startup, or if the submission fails. A nil
	// error means the payload was queued for sending.
	SendNow() error
}
