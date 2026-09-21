// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package gnmistatus provides the gNMI device status registry shared between
// the gNMI check instances and the agent status output.
package gnmistatus

// team: network-device-monitoring-core

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/status"
)

// Component is the component type.
type Component interface {
	// RegisterDevice records a configured gNMI device in the status registry.
	RegisterDevice(address string, port int, profile string, collectTopology bool, subscriptionPaths []string)

	// UnregisterDevice removes a device from the status registry.
	UnregisterDevice(address string, port int)

	// UpdateDevice mutates the runtime fields of a registered device.
	UpdateDevice(address string, port int, update func(*status.DeviceState))
}
