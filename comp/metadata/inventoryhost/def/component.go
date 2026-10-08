// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package inventoryhost exposes the interface for the component to generate the 'host_metadata' metadata payload for inventory.
package inventoryhost

// team: fleet-automation

// Component is the component type.
type Component interface {
	// Refresh trigger a new payload to be send while still respecting the minimal interval between two updates.
	Refresh()
	// SendNow builds and submits the payload immediately, without waiting for the next collection. It returns an
	// error if the payload is disabled, during the first run delay after startup, or if the submission fails. A nil
	// error means the payload was queued for sending.
	SendNow() error
}
