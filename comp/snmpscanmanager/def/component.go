// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package snmpscanmanager is a component that is used to manage SNMP device scans
package snmpscanmanager

// team: ndm-core

// Component is the component type
type Component interface {
	RequestScan(req ScanRequest, forceQueue bool)
}

// ScanRequest represents a device scan request
type ScanRequest struct {
	DeviceIP string
}
