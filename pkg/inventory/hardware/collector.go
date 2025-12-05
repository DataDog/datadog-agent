// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package hardware provides cross-platform hardware collection.
// This package defines the interfaces and types for collecting host hardware
// information from various sources on the host system, including manufacturer, model,
// serial number, enclosure type, enclosure type name, and host type.
package hardware

// SystemHardwareInfo represents the hardware information of the host system
type SystemHardwareInfo struct {
	Manufacturer      string
	Model             string
	SerialNumber      string
	EnclosureType     string
	EnclosureTypeName string
	HostType          string
}

// Collect gathers hardware information from the system
// Platform-specific implementations are in collector_windows.go and collector_nix.go
func Collect() (*SystemHardwareInfo, error) {
	return collect()
}
