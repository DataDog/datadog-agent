// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package snmpscanmanager is a component that can be used to manage SNMP scans
package snmpscanmanager

// team: ndm-core

// Component is the component type for snmpscanmanager
type Component interface {
	TestPrint()
}
