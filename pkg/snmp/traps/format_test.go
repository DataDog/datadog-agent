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

func createTestV1GenericPacket() *SnmpPacket {
	examplePacket := &gosnmp.SnmpPacket{Version: gosnmp.Version1, SnmpTrap: LinkDownv1GenericTrap}
	examplePacket.Variables = examplePacket.SnmpTrap.Variables
	return &SnmpPacket{
		Content: examplePacket,
		Addr:    &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 13156},
	}
}

func createTestV1SpecificPacket() *SnmpPacket {
	examplePacket := &gosnmp.SnmpPacket{Version: gosnmp.Version1, SnmpTrap: AlarmActiveStatev1SpecificTrap}
	examplePacket.Variables = examplePacket.SnmpTrap.Variables
	return &SnmpPacket{
		Content: examplePacket,
		Addr:    &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 13156},
	}
}

func createTestPacket() *SnmpPacket {
	examplePacket := &gosnmp.SnmpPacket{
		Version:   gosnmp.Version2c,
		Community: "public",
		Variables: NetSNMPExampleHeartbeatNotification.Variables,
	}
	return &SnmpPacket{
		Content: examplePacket,
		Addr:    &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 13156},
	}
}

func TestFormatPacketV1Generic(t *testing.T) {
	packet := createTestV1GenericPacket()
	data, err := FormatPacketToJSON(packet)
	require.NoError(t, err)

	assert.Equal(t, "1.3.6.1.6.3.1.1.5.3", data["oid"])
	assert.NotNil(t, data["uptime"])
	assert.NotNil(t, data["enterprise_oid"])
	assert.NotNil(t, data["generic_trap"])
	assert.NotNil(t, data["specific_trap"])

	variables, ok := data["variables"].([]map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, len(variables), 3)

	assert.Equal(t, "1.3.6.1.6.3.1.1.5", data["enterprise_oid"])
	assert.Equal(t, 2, data["generic_trap"])
	assert.Equal(t, 0, data["specific_trap"])

	ifIndex := variables[0]
	assert.Equal(t, ifIndex["oid"], "1.3.6.1.2.1.2.2.1.1")
	assert.Equal(t, ifIndex["type"], "integer")
	assert.Equal(t, ifIndex["value"], 2)

	ifAdminStatus := variables[1]
	assert.Equal(t, ifAdminStatus["oid"], "1.3.6.1.2.1.2.2.1.7")
	assert.Equal(t, ifAdminStatus["type"], "integer")
	assert.Equal(t, ifAdminStatus["value"], 1)

	ifOperStatus := variables[2]
	assert.Equal(t, ifOperStatus["oid"], "1.3.6.1.2.1.2.2.1.8")
	assert.Equal(t, ifOperStatus["type"], "integer")
	assert.Equal(t, ifOperStatus["value"], 2)
}

func TestFormatPacketV1Specific(t *testing.T) {
	packet := createTestV1SpecificPacket()
	data, err := FormatPacketToJSON(packet)
	require.NoError(t, err)

	assert.Equal(t, "1.3.6.1.2.1.118.0.2", data["oid"])
	assert.NotNil(t, data["uptime"])
	assert.NotNil(t, data["enterprise_oid"])
	assert.NotNil(t, data["generic_trap"])
	assert.NotNil(t, data["specific_trap"])

	variables, ok := data["variables"].([]map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, len(variables), 2)

	assert.Equal(t, "1.3.6.1.2.1.118", data["enterprise_oid"])
	assert.Equal(t, 6, data["generic_trap"])
	assert.Equal(t, 2, data["specific_trap"])

	alarmActiveModelPointer := variables[0]
	assert.Equal(t, alarmActiveModelPointer["oid"], "1.3.6.1.2.1.118.1.2.2.1.13")
	assert.Equal(t, alarmActiveModelPointer["type"], "string")
	assert.Equal(t, alarmActiveModelPointer["value"], "foo")

	alarmActiveResourceID := variables[1]
	assert.Equal(t, alarmActiveResourceID["oid"], "1.3.6.1.2.1.118.1.2.2.1.10")
	assert.Equal(t, alarmActiveResourceID["type"], "string")
	assert.Equal(t, alarmActiveResourceID["value"], "bar")

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
		"device_namespace:default",
		"snmp_device:127.0.0.1",
	})
}

func TestGetTagsForUnsupportedVersionShouldStillSucceed(t *testing.T) {
	packet := createTestPacket()
	packet.Content.Version = 12
	tags := GetTags(packet)
	assert.Equal(t, tags, []string{
		"snmp_version:unknown",
		"device_namespace:default",
		"snmp_device:127.0.0.1",
	})
}
