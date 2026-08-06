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
	RunSnmpWalkAll(snmpConnection *gosnmp.GoSNMP, firstOid string) ([]gosnmp.SnmpPDU, error)
	ScanDeviceAndSendData(ctx context.Context, connParams *snmpparse.SNMPConfig, namespace string, scanParams ScanParams) error
}

// ScanMethod selects the SNMP walk method used for a device scan.
type ScanMethod string

const (
	// ScanMethodGetBulk walks the device using SNMP GetBulk requests.
	ScanMethodGetBulk ScanMethod = "getbulk"
	// ScanMethodGetNext walks the device using SNMP GetNext requests.
	ScanMethodGetNext ScanMethod = "getnext"
)

// ScanParams contains options for a device scan
type ScanParams struct {
	ScanType metadata.ScanType

	// CallInterval specifies how long to wait between consecutive SNMP calls.
	// A value of 0 means calls are made without any delay.
	CallInterval time.Duration

	// MaxCallCount limits the total number of SNMP calls in a scan.
	// A value of 0 means there is no limit.
	MaxCallCount int

	// ScanMethod selects the SNMP walk method. The zero value defaults to
	// GetBulk; GetBulk is automatically downgraded to GetNext for SNMPv1
	// devices, which do not support GetBulk.
	ScanMethod ScanMethod

	// BulkBatchSize sets the starting number of values requested per GetBulk
	// call (the SNMP max-repetitions). A value of 0 uses the default. No effect
	// when walking with GetNext.
	BulkBatchSize uint32

	// FlushEveryNOIDs reports partial scan results once this many OIDs have been
	// collected. A value of 0 uses the default batch size.
	FlushEveryNOIDs int

	// FlushInterval reports partial scan results when this much time has elapsed
	// since the last report. A value of 0 uses the default interval.
	FlushInterval time.Duration
}
