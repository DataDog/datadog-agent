// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package traps

import (
	"encoding/json"
	"fmt"
	"net"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// NoOpOIDResolver is a dummy OIDResolver implementation that is unable to get any Trap or Variable metadata.
type NoOpOIDResolver struct{}

// GetTrapMetadata always return an error in this OIDResolver implementation
func (or NoOpOIDResolver) GetTrapMetadata(trapOID string) (TrapMetadata, error) {
	return TrapMetadata{}, fmt.Errorf("trap OID %s is not defined", trapOID)
}

// GetVariableMetadata always return an error in this OIDResolver implementation
func (or NoOpOIDResolver) GetVariableMetadata(trapOID string, varOID string) (VariableMetadata, error) {
	return VariableMetadata{}, fmt.Errorf("trap OID %s is not defined", trapOID)
}

var defaultFormatter, _ = NewJSONFormatter(NoOpOIDResolver{})

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
	formattedPacket, err := defaultFormatter.FormatPacket(packet)
	require.NoError(t, err)
	data := make(map[string]interface{})
	err = json.Unmarshal(formattedPacket, &data)
	require.NoError(t, err)
	trapContent := data["trap"].(map[string]interface{})

	assert.Equal(t, "snmp-traps", trapContent["ddsource"])
	assert.Equal(t, "snmp_version:1,device_namespace:default,snmp_device:127.0.0.1", trapContent["ddtags"])

	assert.Equal(t, "1.3.6.1.6.3.1.1.5.3", trapContent["snmpTrapOID"])
	assert.NotNil(t, trapContent["uptime"])
	assert.NotNil(t, trapContent["enterpriseOID"])
	assert.NotNil(t, trapContent["genericTrap"])
	assert.NotNil(t, trapContent["specificTrap"])

	variables := make([]map[string]interface{}, 3)
	for i := 0; i < 3; i++ {
		variables[i] = trapContent["variables"].([]interface{})[i].(map[string]interface{})
	}

	assert.Equal(t, "1.3.6.1.6.3.1.1.5", trapContent["enterpriseOID"])
	assert.EqualValues(t, 2, trapContent["genericTrap"])
	assert.EqualValues(t, 0, trapContent["specificTrap"])

	ifIndex := variables[0]
	assert.EqualValues(t, ifIndex["oid"], "1.3.6.1.2.1.2.2.1.1")
	assert.EqualValues(t, ifIndex["type"], "integer")
	assert.EqualValues(t, ifIndex["value"], 2)

	ifAdminStatus := variables[1]
	assert.EqualValues(t, ifAdminStatus["oid"], "1.3.6.1.2.1.2.2.1.7")
	assert.EqualValues(t, ifAdminStatus["type"], "integer")
	assert.EqualValues(t, ifAdminStatus["value"], 1)

	ifOperStatus := variables[2]
	assert.EqualValues(t, ifOperStatus["oid"], "1.3.6.1.2.1.2.2.1.8")
	assert.EqualValues(t, ifOperStatus["type"], "integer")
	assert.EqualValues(t, ifOperStatus["value"], 2)
}

func TestFormatPacketV1Specific(t *testing.T) {
	packet := createTestV1SpecificPacket()
	formattedPacket, err := defaultFormatter.FormatPacket(packet)
	require.NoError(t, err)
	data := make(map[string]interface{})
	err = json.Unmarshal(formattedPacket, &data)
	require.NoError(t, err)
	trapContent := data["trap"].(map[string]interface{})

	assert.Equal(t, "snmp-traps", trapContent["ddsource"])
	assert.Equal(t, "snmp_version:1,device_namespace:default,snmp_device:127.0.0.1", trapContent["ddtags"])

	assert.Equal(t, "1.3.6.1.2.1.118.0.2", trapContent["snmpTrapOID"])
	assert.NotNil(t, trapContent["uptime"])
	assert.NotNil(t, trapContent["enterpriseOID"])
	assert.NotNil(t, trapContent["genericTrap"])
	assert.NotNil(t, trapContent["specificTrap"])

	variables := make([]map[string]interface{}, 2)
	for i := 0; i < 2; i++ {
		variables[i] = trapContent["variables"].([]interface{})[i].(map[string]interface{})
	}

	assert.Equal(t, "1.3.6.1.2.1.118", trapContent["enterpriseOID"])
	assert.EqualValues(t, 6, trapContent["genericTrap"])
	assert.EqualValues(t, 2, trapContent["specificTrap"])

	alarmActiveModelPointer := variables[0]
	assert.Equal(t, alarmActiveModelPointer["oid"], "1.3.6.1.2.1.118.1.2.2.1.13")
	assert.EqualValues(t, alarmActiveModelPointer["type"], "string")
	assert.EqualValues(t, alarmActiveModelPointer["value"], "foo")

	alarmActiveResourceID := variables[1]
	assert.Equal(t, alarmActiveResourceID["oid"], "1.3.6.1.2.1.118.1.2.2.1.10")
	assert.EqualValues(t, alarmActiveResourceID["type"], "string")
	assert.EqualValues(t, alarmActiveResourceID["value"], "bar")

}

func TestFormatPacketToJSON(t *testing.T) {
	packet := createTestPacket()

	formattedPacket, err := defaultFormatter.FormatPacket(packet)
	require.NoError(t, err)
	data := make(map[string]interface{})
	err = json.Unmarshal(formattedPacket, &data)
	require.NoError(t, err)
	trapContent := data["trap"].(map[string]interface{})

	assert.Equal(t, "snmp-traps", trapContent["ddsource"])
	assert.Equal(t, "snmp_version:2,device_namespace:default,snmp_device:127.0.0.1", trapContent["ddtags"])

	assert.Equal(t, "1.3.6.1.4.1.8072.2.3.0.1", trapContent["snmpTrapOID"])
	assert.NotNil(t, trapContent["uptime"])

	variables := make([]map[string]interface{}, 2)
	for i := 0; i < 2; i++ {
		variables[i] = trapContent["variables"].([]interface{})[i].(map[string]interface{})
	}

	heartBeatRate := variables[0]
	assert.Equal(t, heartBeatRate["oid"], "1.3.6.1.4.1.8072.2.3.2.1")
	assert.EqualValues(t, heartBeatRate["type"], "integer")
	assert.EqualValues(t, heartBeatRate["value"], 1024)

	heartBeatName := variables[1]
	assert.Equal(t, heartBeatName["oid"], "1.3.6.1.4.1.8072.2.3.2.2")
	assert.EqualValues(t, heartBeatName["type"], "string")
	assert.EqualValues(t, heartBeatName["value"], "test")
}

func TestFormatPacketToJSONShouldFailIfNotEnoughVariables(t *testing.T) {
	packet := createTestPacket()

	packet.Content.Variables = []gosnmp.SnmpPDU{
		// No variables at all.
	}
	_, err := defaultFormatter.FormatPacket(packet)
	require.Error(t, err)

	packet.Content.Variables = []gosnmp.SnmpPDU{
		// sysUpTimeInstance and data, but no snmpsnmpTrapOID
		{Name: "1.3.6.1.2.1.1.3.0", Type: gosnmp.TimeTicks, Value: uint32(1000)},
		{Name: "1.3.6.1.4.1.8072.2.3.2.1", Type: gosnmp.Integer, Value: 1024},
		{Name: "1.3.6.1.4.1.8072.2.3.2.2", Type: gosnmp.OctetString, Value: "test"},
	}
	_, err = defaultFormatter.FormatPacket(packet)
	require.Error(t, err)

	packet.Content.Variables = []gosnmp.SnmpPDU{
		// snmpsnmpTrapOID and data, but no sysUpTimeInstance
		{Name: "1.3.6.1.6.3.1.1.4.1.0", Type: gosnmp.OctetString, Value: "1.3.6.1.4.1.8072.2.3.0.1"},
		{Name: "1.3.6.1.4.1.8072.2.3.2.1", Type: gosnmp.Integer, Value: 1024},
		{Name: "1.3.6.1.4.1.8072.2.3.2.2", Type: gosnmp.OctetString, Value: "test"},
	}
	_, err = defaultFormatter.FormatPacket(packet)
	require.Error(t, err)
}

func TestGetTags(t *testing.T) {
	packet := createTestPacket()
	assert.Equal(t, defaultFormatter.getTags(packet), []string{
		"snmp_version:2",
		"device_namespace:default",
		"snmp_device:127.0.0.1",
	})
}

func TestGetTagsForUnsupportedVersionShouldStillSucceed(t *testing.T) {
	packet := createTestPacket()
	packet.Content.Version = 12
	assert.Equal(t, defaultFormatter.getTags(packet), []string{
		"snmp_version:unknown",
		"device_namespace:default",
		"snmp_device:127.0.0.1",
	})
}

func TestNewJSONFormatterWithNilStillWorks(t *testing.T) {
	var formatter, err = NewJSONFormatter(NoOpOIDResolver{})
	require.NoError(t, err)
	packet := createTestPacket()
	_, err = formatter.FormatPacket(packet)
	require.NoError(t, err)
	tags := formatter.getTags(packet)
	assert.Equal(t, tags, []string{
		"snmp_version:2",
		"device_namespace:default",
		"snmp_device:127.0.0.1",
	})
}

func TestFormatterWithResolverAndTrapV2(t *testing.T) {
	formatter, err := NewJSONFormatter(resolverWithData)
	require.NoError(t, err)
	packet := createTestPacket()
	data, err := formatter.FormatPacket(packet)
	require.NoError(t, err)
	content := make(map[string]interface{})
	err = json.Unmarshal(data, &content)
	require.NoError(t, err)
	trapContent := content["trap"].(map[string]interface{})

	assert.Equal(t, "snmp-traps", trapContent["ddsource"])
	assert.Equal(t, "snmp_version:2,device_namespace:default,snmp_device:127.0.0.1", trapContent["ddtags"])

	assert.EqualValues(t, "netSnmpExampleHeartbeatNotification", trapContent["snmpTrapName"])
	assert.EqualValues(t, "NET-SNMP-EXAMPLES-MIB", trapContent["snmpTrapMIB"])
	assert.EqualValues(t, 1024, trapContent["netSnmpExampleHeartbeatRate"])

	tags := formatter.getTags(packet)
	assert.Equal(t, tags, []string{
		"snmp_version:2",
		"device_namespace:default",
		"snmp_device:127.0.0.1",
	})
}

func TestFormatterWithResolverAndTrapV1Generic(t *testing.T) {
	formatter, err := NewJSONFormatter(resolverWithData)
	require.NoError(t, err)
	packet := createTestV1GenericPacket()
	data, err := formatter.FormatPacket(packet)
	require.NoError(t, err)
	content := make(map[string]interface{})
	err = json.Unmarshal(data, &content)
	require.NoError(t, err)
	trapContent := content["trap"].(map[string]interface{})

	assert.Equal(t, "snmp-traps", trapContent["ddsource"])
	assert.Equal(t, "snmp_version:1,device_namespace:default,snmp_device:127.0.0.1", trapContent["ddtags"])

	assert.EqualValues(t, "ifDown", trapContent["snmpTrapName"])
	assert.EqualValues(t, "IF-MIB", trapContent["snmpTrapMIB"])
	assert.EqualValues(t, 2, trapContent["ifIndex"])
	assert.EqualValues(t, 1, trapContent["ifAdminStatus"])
	assert.EqualValues(t, 2, trapContent["ifOperStatus"])

	tags := formatter.getTags(packet)
	assert.Equal(t, tags, []string{
		"snmp_version:1",
		"device_namespace:default",
		"snmp_device:127.0.0.1",
	})
}
