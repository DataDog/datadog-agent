// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gosnmplib

import (
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
)

func TestPacketToString(t *testing.T) {
	tests := []struct {
		name        string
		packet      *gosnmp.SnmpPacket
		expectedStr string
	}{
		{
			name: "to string",
			packet: &gosnmp.SnmpPacket{
				Variables: []gosnmp.SnmpPDU{
					{
						Name:  "1.3.6.1.2.1.1.2.0",
						Type:  gosnmp.ObjectIdentifier,
						Value: "1.3.6.1.4.1.3375.2.1.3.4.1",
					},
					{
						Name:  "1.3.6.1.2.1.1.3.0",
						Type:  gosnmp.Counter32,
						Value: 10,
					},
				},
			},
			expectedStr: "error=NoError(code:0, idx:0), values=[{\"oid\":\"1.3.6.1.2.1.1.2.0\",\"type\":\"ObjectIdentifier\",\"value\":\"1.3.6.1.4.1.3375.2.1.3.4.1\"},{\"oid\":\"1.3.6.1.2.1.1.3.0\",\"type\":\"Counter32\",\"value\":\"10\"}]",
		},
		{
			name: "invalid ipaddr",
			packet: &gosnmp.SnmpPacket{
				Variables: []gosnmp.SnmpPDU{
					{
						Name:  "1.3.6.1.2.1.1.2.0",
						Type:  gosnmp.IPAddress,
						Value: 10,
					},
				},
			},
			expectedStr: "error=NoError(code:0, idx:0), values=[{\"oid\":\"1.3.6.1.2.1.1.2.0\",\"type\":\"IPAddress\",\"value\":\"10\",\"parse_err\":\"`oid 1.3.6.1.2.1.1.2.0: IPAddress should be string type but got type `int` and value `10``\"}]",
		},
		{
			name:        "nil packet loglevel",
			expectedStr: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			str := PacketAsString(tt.packet)
			assert.Equal(t, tt.expectedStr, str)
		})
	}
}
