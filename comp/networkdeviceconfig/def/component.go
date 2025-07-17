// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package networkdeviceconfig provides the component for retrieving network device configurations.
package networkdeviceconfig

// team: network-device-monitoring

// Component is the component type.
type Component interface {
	RetrieveConfiguration(deviceID string) (string, error)
}
