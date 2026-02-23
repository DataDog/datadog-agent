// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package systeminfo provides cross-platform system information collection.
// This package defines the interfaces and types for collecting host system
// information from various sources on the host system, including manufacturer, model,
// serial number, enclosure type, enclosure type name, and host type.
package systeminfo

// SystemInfo represents the system information of the host system
type SystemInfo struct {
	Manufacturer string
	ModelNumber  string
	SerialNumber string
	ModelName    string
	ChassisType  string
	Identifier   string
}

// Collect gathers system information from the system
// Platform-specific implementations are in collector_windows.go and collector_nix.go
func Collect() (*SystemInfo, error) {
	return collect()
}
