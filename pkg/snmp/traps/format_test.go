// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020 Datadog, Inc.

package traps

import (
	"net"
	"testing"

	"github.com/soniah/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestPacket() *SnmpPacket {
	return &SnmpPacket{
		Content: &gosnmp.SnmpPacket{
			Version:   gosnmp.Version2c,
			Community: "public",
			Variables: []gosnmp.SnmpPDU{
				// sysUpTime
				{Name: "1.3.6.1.2.1.1.3", Type: gosnmp.TimeTicks, Value: uint32(1000)},
				// snmpTrapOID
				{Name: "1.3.6.1.6.3.1.1.4.1", Type: gosnmp.OctetString, Value: "1.3.6.1.4.1.8072.2.3.0.1"},
				// heartBeatRate
				{Name: "1.3.6.1.4.1.8072.2.3.2.1", Type: gosnmp.Integer, Value: 1024},
				// heartBeatName
				{Name: "1.3.6.1.4.1.8072.2.3.2.2", Type: gosnmp.OctetString, Value: "test"},
			},
		},
		Addr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 13156},
	}
}

func TestFormatJSON(t *testing.T) {
	p := createTestPacket()

	data, err := FormatJSON(p)
	require.NoError(t, err)

	assert.Equal(t, "1.3.6.1.4.1.8072.2.3.0.1", data["oid"])
	assert.NotNil(t, data["uptime"])

	vars, ok := data["variables"].([]map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, len(vars), 2)

	heartBeatRate := vars[0]
	assert.Equal(t, heartBeatRate["oid"], "1.3.6.1.4.1.8072.2.3.2.1")
	assert.Equal(t, heartBeatRate["type"], "integer")
	assert.Equal(t, heartBeatRate["value"], 1024)

	heartBeatName := vars[1]
	assert.Equal(t, heartBeatName["oid"], "1.3.6.1.4.1.8072.2.3.2.2")
	assert.Equal(t, heartBeatName["type"], "string")
	assert.Equal(t, heartBeatName["value"], "test")
}

func TestGetTags(t *testing.T) {
	p := createTestPacket()
	tags := GetTags(p)
	assert.Equal(t, tags, []string{
		"community:public",
		"device_ip:127.0.0.1",
		"device_port:13156",
		"snmp_version:2",
	})
}

func TestFormatJSONShouldFailIfNotEnoughVariables(t *testing.T) {
	p := createTestPacket()

	p.Content.Variables = []gosnmp.SnmpPDU{
		// No variables at all.
	}
	_, err := FormatJSON(p)
	require.Error(t, err)

	p.Content.Variables = []gosnmp.SnmpPDU{
		// sysUpTime and data, but no snmpTrapOID
		{Name: "1.3.6.1.2.1.1.3", Type: gosnmp.TimeTicks, Value: uint32(1000)},
		{Name: "1.3.6.1.4.1.8072.2.3.2.1", Type: gosnmp.Integer, Value: 1024},
		{Name: "1.3.6.1.4.1.8072.2.3.2.2", Type: gosnmp.OctetString, Value: "test"},
	}
	_, err = FormatJSON(p)
	require.Error(t, err)

	p.Content.Variables = []gosnmp.SnmpPDU{
		// snmpTrapOID and data, but no sysUpTime
		{Name: "1.3.6.1.2.1.1.3", Type: gosnmp.TimeTicks, Value: uint32(1000)},
		{Name: "1.3.6.1.4.1.8072.2.3.2.1", Type: gosnmp.Integer, Value: 1024},
		{Name: "1.3.6.1.4.1.8072.2.3.2.2", Type: gosnmp.OctetString, Value: "test"},
	}
	_, err = FormatJSON(p)
	require.Error(t, err)
}
