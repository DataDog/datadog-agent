// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package integrations contains NDM integrations utils
package integrations

// Integration is an enum listing NDM integrations
type Integration string

const (
	// SNMP the SNMP integration
	SNMP Integration = "snmp"
	// CiscoSDWAN the Cisco SD-WAN integration
	CiscoSDWAN Integration = "cisco-sdwan"
	// Versa the Versa integration
	Versa Integration = "versa"
	// Netflow the Netflow integration
	Netflow Integration = "netflow"
)
