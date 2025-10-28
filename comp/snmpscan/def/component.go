// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package snmpscan is a light component that can be used to perform a scan or a walk of a particular device
package snmpscan

import (
	"github.com/gosnmp/gosnmp"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/DataDog/datadog-agent/pkg/snmp/snmpparse"
)

// team: ndm-core

// Component is the component type.
type Component interface {
	RunSnmpWalk(snmpConection *gosnmp.GoSNMP, firstOid string) error
	ScanDeviceAndSendData(connParams *snmpparse.SNMPConfig, namespace string, scanType metadata.ScanType) error
}
