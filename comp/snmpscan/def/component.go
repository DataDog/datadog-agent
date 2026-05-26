// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package snmpscan is a light component that can be used to perform a scan or a walk of a particular device
package snmpscan

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/DataDog/datadog-agent/pkg/snmp/snmpparse"

	"github.com/gosnmp/gosnmp"
)

// team: network-device-monitoring-core

// Component is the component type.
type Component interface {
	RunSnmpWalk(snmpConection *gosnmp.GoSNMP, firstOid string) error
	ScanDeviceAndSendData(ctx context.Context, connParams *snmpparse.SNMPConfig, namespace string, scanParams ScanParams) error
}

// ScanParams contains options for a device scan
type ScanParams struct {
	ScanType metadata.ScanType

	// CallInterval specifies how long to wait between consecutive SNMP calls.
	// A value of 0 means calls are made without any delay.
	CallInterval time.Duration

	// MaxCallCount limits the total number of SNMP calls in a scan.
	// A value of 0 means there is no limit.
	MaxCallCount int

	// UseGetBulk selects the GetBulk-based walk introduced in PR #49734.
	// When false (default), the scan uses the existing SkipOIDRowsNaive-based
	// gatherPDUs path. When true, the scan uses gatherPDUsWithBulk, which never
	// fabricates OIDs and is required to scan devices that previously caused
	// infinite loops or crashes.
	UseGetBulk bool

	// BulkMaxRepetitions is the starting max-repetitions value for GetBulk
	// when UseGetBulk is true. A value of 0 selects the default (10, matching
	// net-snmp's snmpbulkwalk -Cr). Halves on timeout, recovers on success.
	// Ignored when UseGetBulk is false.
	BulkMaxRepetitions int
}
