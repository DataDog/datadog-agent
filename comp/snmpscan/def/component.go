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

// team: ndm-core

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
}
