// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package snmpscan ...
package snmpscan

import (
	"github.com/DataDog/datadog-agent/pkg/snmp/snmpparse"
	"github.com/gosnmp/gosnmp"
)

// team: network-device-monitoring

type SnmpConnectionParams struct {
	// embed a SNMPConfig because it's all the same fields anyway
	snmpparse.SNMPConfig
	// fields that aren't part of parse.SNMPConfig
	SecurityLevel           string
	UseUnconnectedUDPSocket bool
}

// Component is the component type.
type Component interface {
	// Triggers a device scan
	RunDeviceScan(snmpConection *gosnmp.GoSNMP, deviceNamespace string) error
	RunSnmpWalk(snmpConection *gosnmp.GoSNMP, firstOid string) error
}
