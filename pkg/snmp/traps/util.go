// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020 Datadog, Inc.

package traps

import (
	"github.com/DataDog/datadog-agent/pkg/config"

	"github.com/soniah/gosnmp"
)

// IsEnabled returns whether SNMP trap collection is enabled in the Agent configuration.
func IsEnabled() bool {
	return config.Datadog.GetBool("snmp_traps_enabled")
}

// VersionAsString converts a version value to a human-readable string.
func VersionAsString(value gosnmp.SnmpVersion) string {
	switch value {
	case gosnmp.Version1:
		return "1"
	case gosnmp.Version2c:
		return "2c"
	case gosnmp.Version3:
		return "3"
	default:
		return ""
	}
}
