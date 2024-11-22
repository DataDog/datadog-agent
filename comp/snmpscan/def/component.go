// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package snmpscan is a light component that can be used to perform a scan or a walk of a particular device
package snmpscan

import (
	"github.com/gosnmp/gosnmp"
)

// team: ndm-core

// Component is the component type.
type Component interface {
	// Triggers a device scan
	RunDeviceScan(snmpConection *gosnmp.GoSNMP, deviceNamespace string, deviceIPAddress string) error
	RunSnmpWalk(snmpConection *gosnmp.GoSNMP, firstOid string) error
}
