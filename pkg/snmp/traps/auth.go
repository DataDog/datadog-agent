// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020 Datadog, Inc.

package traps

import "github.com/soniah/gosnmp"

func validateCredentials(p *gosnmp.SnmpPacket, c *Config) bool {
	if p.Version != gosnmp.Version2c {
		// Unsupported.
		return false
	}

	// At least one of the known community strings must match.
	for _, community := range c.CommunityStrings {
		if community == p.Community {
			return true
		}
	}

	return false
}
