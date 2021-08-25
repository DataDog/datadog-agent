// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package traps

import (
	"net"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestPacket() *SnmpPacket {
	return &SnmpPacket{
		Content: &gosnmp.SnmpPacket{
			Version:   gosnmp.Version2c,
			Community: "public",
			Variables: NetSNMPExampleHeartbeatNotificationVariables,
		},
		Addr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 13156},
	}
}

func TestFormatPacketToJSON(t *testing.T) {
	packet := createTestPacket()

	data, err := FormatPacketToJSON(packet)
	require.NoError(t, err)

	assert.Equal(t, "1.3.6.1.4.1.8072.2.3.0.1", data["oid"])
	assert.NotNil(t, data["uptime"])

	variables, ok := data["variables"].([]map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, len(variables), 2)

	heartBeatRate := variables[0]
	assert.Equal(t, heartBeatRate["oid"], "1.3.6.1.4.1.8072.2.3.2.1")
	assert.Equal(t, heartBeatRate["type"], "integer")
	assert.Equal(t, heartBeatRate["value"], 1024)

	heartBeatName := variables[1]
	assert.Equal(t, heartBeatName["oid"], "1.3.6.1.4.1.8072.2.3.2.2")
	assert.Equal(t, heartBeatName["type"], "string")
	assert.Equal(t, heartBeatName["value"], "test")
}

func TestFormatPacketToJSONShouldFailIfNotEnoughVariables(t *testing.T) {
	packet := createTestPacket()

	packet.Content.Variables = []gosnmp.SnmpPDU{
		// No variables at all.
	}
	_, err := FormatPacketToJSON(packet)
	require.Error(t, err)

	packet.Content.Variables = []gosnmp.SnmpPDU{
		// sysUpTimeInstance and data, but no snmpTrapOID
		{Name: "1.3.6.1.2.1.1.3.0", Type: gosnmp.TimeTicks, Value: uint32(1000)},
		{Name: "1.3.6.1.4.1.8072.2.3.2.1", Type: gosnmp.Integer, Value: 1024},
		{Name: "1.3.6.1.4.1.8072.2.3.2.2", Type: gosnmp.OctetString, Value: "test"},
	}
	_, err = FormatPacketToJSON(packet)
	require.Error(t, err)

	packet.Content.Variables = []gosnmp.SnmpPDU{
		// snmpTrapOID and data, but no sysUpTimeInstance
		{Name: "1.3.6.1.6.3.1.1.4.1.0", Type: gosnmp.OctetString, Value: "1.3.6.1.4.1.8072.2.3.0.1"},
		{Name: "1.3.6.1.4.1.8072.2.3.2.1", Type: gosnmp.Integer, Value: 1024},
		{Name: "1.3.6.1.4.1.8072.2.3.2.2", Type: gosnmp.OctetString, Value: "test"},
	}
	_, err = FormatPacketToJSON(packet)
	require.Error(t, err)
}

func TestGetTags(t *testing.T) {
	packet := createTestPacket()
	tags := GetTags(packet)
	assert.Equal(t, tags, []string{
		"snmp_version:2",
		"snmp_device:127.0.0.1",
	})
}

func TestGetTagsForUnsupportedVersionShouldStillSucceed(t *testing.T) {
	packet := createTestPacket()
	packet.Content.Version = gosnmp.Version3
	packet.Content.Community = ""
	tags := GetTags(packet)
	assert.Equal(t, tags, []string{
		"snmp_version:unknown",
		"snmp_device:127.0.0.1",
	})
}
