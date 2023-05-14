// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package traps

import (
	"errors"

	"github.com/gosnmp/gosnmp"
)

func validatePacket(p *gosnmp.SnmpPacket, c Config) error {
	if p.Version == gosnmp.Version3 {
		// v3 Packets are already decrypted and validated by gosnmp
		return nil
	}

	// At least one of the known community strings must match.
	for _, community := range c.CommunityStrings {
		if community == p.Community {
			return nil
		}
	}

	return errors.New("unknown community string")
}
