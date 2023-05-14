// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gosnmplib

import "github.com/gosnmp/gosnmp"

// MockValidReachableGetNextPacket valid reachable packet
var MockValidReachableGetNextPacket = gosnmp.SnmpPacket{
	Variables: []gosnmp.SnmpPDU{
		{
			Name:  "1.3.6.1.2.1.1.2.0",
			Type:  gosnmp.ObjectIdentifier,
			Value: "1.3.6.1.4.1.3375.2.1.3.4.1",
		},
	},
}
