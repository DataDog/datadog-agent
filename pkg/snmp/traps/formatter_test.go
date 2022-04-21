// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package traps

import (
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type (
	// NoOpOIDResolver is a dummy OIDResolver implementation that is unable to get any Trap or Variable metadata.
	NoOpOIDResolver struct{}

	// ByOID is a wrapper to sort formatted variables by OID
	byOID []interface{}
)

// GetTrapMetadata always return an error in this OIDResolver implementation
func (or NoOpOIDResolver) GetTrapMetadata(trapOID string) (TrapMetadata, error) {
	return TrapMetadata{}, fmt.Errorf("trap OID %s is not defined", trapOID)
}

// GetVariableMetadata always return an error in this OIDResolver implementation
func (or NoOpOIDResolver) GetVariableMetadata(trapOID string, varOID string) (VariableMetadata, error) {
	return VariableMetadata{}, fmt.Errorf("trap OID %s is not defined", trapOID)
}

func (b byOID) Len() int {
	return len(b)
}

func (b byOID) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}

func (b byOID) Less(i, j int) bool {
	// TODO(ken): this is ugly
	return b[i].(map[string]interface{})["oid"].(string) < b[j].(map[string]interface{})["oid"].(string)
}

var (
	defaultFormatter, _ = NewJSONFormatter(NoOpOIDResolver{})

	// LinkUp Example Trap V2+
	LinkUpExampleV2Trap = gosnmp.SnmpTrap{
		Variables: []gosnmp.SnmpPDU{
			// sysUpTimeInstance
			{Name: "1.3.6.1.2.1.1.3.0", Type: gosnmp.TimeTicks, Value: uint32(1000)},
			// snmpTrapOID
			{Name: "1.3.6.1.6.3.1.1.4.1.0", Type: gosnmp.OctetString, Value: "1.3.6.1.6.3.1.1.5.4"},
			// ifIndex
			{Name: "1.3.6.1.2.1.2.2.1.1", Type: gosnmp.Integer, Value: 9001},
			// ifAdminStatus
			{Name: "1.3.6.1.2.1.2.2.1.7", Type: gosnmp.Integer, Value: 2},
			// ifOperStatus
			{Name: "1.3.6.1.2.1.2.2.1.8", Type: gosnmp.Integer, Value: 7},
		},
	}

	// LinkUp Example Trap with bad value V2+
	BadValueExampleV2Trap = gosnmp.SnmpTrap{
		Variables: []gosnmp.SnmpPDU{
			// sysUpTimeInstance
			{Name: "1.3.6.1.2.1.1.3.0", Type: gosnmp.TimeTicks, Value: uint32(1000)},
			// snmpTrapOID
			{Name: "1.3.6.1.6.3.1.1.4.1.0", Type: gosnmp.OctetString, Value: "1.3.6.1.6.3.1.1.5.4"},
			// ifIndex
			{Name: "1.3.6.1.2.1.2.2.1.1", Type: gosnmp.Integer, Value: 9001},
			// ifAdminStatus
			{Name: "1.3.6.1.2.1.2.2.1.7", Type: gosnmp.Integer, Value: "test"}, // type is set to integer, but we have a string
			// ifOperStatus
			{Name: "1.3.6.1.2.1.2.2.1.8", Type: gosnmp.Integer, Value: 7},
		},
	}

	// LinkUp Example Trap with value not found in enum V2+
	NoEnumMappingExampleV2Trap = gosnmp.SnmpTrap{
		Variables: []gosnmp.SnmpPDU{
			// sysUpTimeInstance
			{Name: "1.3.6.1.2.1.1.3.0", Type: gosnmp.TimeTicks, Value: uint32(1000)},
			// snmpTrapOID
			{Name: "1.3.6.1.6.3.1.1.4.1.0", Type: gosnmp.OctetString, Value: "1.3.6.1.6.3.1.1.5.4"},
			// ifIndex
			{Name: "1.3.6.1.2.1.2.2.1.1", Type: gosnmp.Integer, Value: 9001},
			// ifAdminStatus
			{Name: "1.3.6.1.2.1.2.2.1.7", Type: gosnmp.Integer, Value: 8}, // 8 does not exist in the ifAdminStatus enum
			// ifOperStatus
			{Name: "1.3.6.1.2.1.2.2.1.8", Type: gosnmp.Integer, Value: 7},
		},
	}
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

func createTestPacket(trap gosnmp.SnmpTrap) *SnmpPacket {
	examplePacket := &gosnmp.SnmpPacket{
		Version:   gosnmp.Version2c,
		Community: "public",
		Variables: trap.Variables,
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

	assert.Equal(t, "1.3.6.1.6.3.1.1.5.3", data["snmpTrapOID"])
	assert.NotNil(t, data["uptime"])
	assert.NotNil(t, data["enterpriseOID"])
	assert.NotNil(t, data["genericTrap"])
	assert.NotNil(t, data["specificTrap"])

	variables := make([]map[string]interface{}, 3)
	for i := 0; i < 3; i++ {
		variables[i] = data["variables"].([]interface{})[i].(map[string]interface{})
	}

	assert.Equal(t, "1.3.6.1.6.3.1.1.5", data["enterpriseOID"])
	assert.EqualValues(t, 2, data["genericTrap"])
	assert.EqualValues(t, 0, data["specificTrap"])

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

	assert.Equal(t, "1.3.6.1.2.1.118.0.2", data["snmpTrapOID"])
	assert.NotNil(t, data["uptime"])
	assert.NotNil(t, data["enterpriseOID"])
	assert.NotNil(t, data["genericTrap"])
	assert.NotNil(t, data["specificTrap"])

	variables := make([]map[string]interface{}, 2)
	for i := 0; i < 2; i++ {
		variables[i] = data["variables"].([]interface{})[i].(map[string]interface{})
	}

	assert.Equal(t, "1.3.6.1.2.1.118", data["enterpriseOID"])
	assert.EqualValues(t, 6, data["genericTrap"])
	assert.EqualValues(t, 2, data["specificTrap"])

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
	packet := createTestPacket(NetSNMPExampleHeartbeatNotification)

	formattedPacket, err := defaultFormatter.FormatPacket(packet)
	require.NoError(t, err)
	data := make(map[string]interface{})
	err = json.Unmarshal(formattedPacket, &data)
	require.NoError(t, err)

	assert.Equal(t, "1.3.6.1.4.1.8072.2.3.0.1", data["snmpTrapOID"])
	assert.NotNil(t, data["uptime"])

	variables := make([]map[string]interface{}, 2)
	for i := 0; i < 2; i++ {
		variables[i] = data["variables"].([]interface{})[i].(map[string]interface{})
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
	packet := createTestPacket(NetSNMPExampleHeartbeatNotification)

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
	packet := createTestPacket(NetSNMPExampleHeartbeatNotification)
	assert.Equal(t, defaultFormatter.GetTags(packet), []string{
		"snmp_version:2",
		"device_namespace:default",
		"snmp_device:127.0.0.1",
	})
}

func TestGetTagsForUnsupportedVersionShouldStillSucceed(t *testing.T) {
	packet := createTestPacket(NetSNMPExampleHeartbeatNotification)
	packet.Content.Version = 12
	assert.Equal(t, defaultFormatter.GetTags(packet), []string{
		"snmp_version:unknown",
		"device_namespace:default",
		"snmp_device:127.0.0.1",
	})
}

func TestNewJSONFormatterWithNilStillWorks(t *testing.T) {
	var formatter, err = NewJSONFormatter(NoOpOIDResolver{})
	require.NoError(t, err)
	packet := createTestPacket(NetSNMPExampleHeartbeatNotification)
	_, err = formatter.FormatPacket(packet)
	require.NoError(t, err)
	tags := formatter.GetTags(packet)
	assert.Equal(t, tags, []string{
		"snmp_version:2",
		"device_namespace:default",
		"snmp_device:127.0.0.1",
	})
}

func TestFormatterWithResolverAndTrapV2(t *testing.T) {
	data := []struct {
		description     string
		trap            gosnmp.SnmpTrap
		resolver        *MockedResolver
		expectedContent map[string]interface{}
		expectedTags    []string
	}{
		{
			description: "test no enum variable resolution with netSnmpExampleHeartbeatNotification",
			trap:        NetSNMPExampleHeartbeatNotification,
			resolver:    resolverWithData,
			expectedContent: map[string]interface{}{
				"uptime":                      float64(1000),
				"snmpTrapName":                "netSnmpExampleHeartbeatNotification",
				"snmpTrapMIB":                 "NET-SNMP-EXAMPLES-MIB",
				"snmpTrapOID":                 "1.3.6.1.4.1.8072.2.3.0.1",
				"netSnmpExampleHeartbeatRate": float64(1024),
				"variables": []interface{}{
					map[string]interface{}{
						"oid":   "1.3.6.1.4.1.8072.2.3.2.1",
						"type":  "integer",
						"value": float64(1024),
					},
					map[string]interface{}{
						"oid":   "1.3.6.1.4.1.8072.2.3.2.2",
						"type":  "string",
						"value": "test",
					},
				},
			},
			expectedTags: []string{
				"snmp_version:2",
				"device_namespace:default",
				"snmp_device:127.0.0.1",
			},
		},
		{
			description: "test enum variable resolution with linkDown",
			trap:        LinkUpExampleV2Trap,
			resolver:    resolverWithData,
			expectedContent: map[string]interface{}{
				"snmpTrapName":  "linkUp",
				"snmpTrapMIB":   "IF-MIB",
				"snmpTrapOID":   "1.3.6.1.6.3.1.1.5.4",
				"ifIndex":       float64(9001),
				"ifAdminStatus": "down",
				"ifOperStatus":  "lowerLayerDown",
				"uptime":        float64(1000),
				"variables": []interface{}{
					map[string]interface{}{
						"oid":   "1.3.6.1.2.1.2.2.1.1",
						"type":  "integer",
						"value": float64(9001),
					},
					map[string]interface{}{
						"oid":   "1.3.6.1.2.1.2.2.1.8",
						"type":  "integer",
						"value": float64(7),
					},
					map[string]interface{}{
						"oid":   "1.3.6.1.2.1.2.2.1.7",
						"type":  "integer",
						"value": float64(2),
					},
				},
			},
			expectedTags: []string{
				"snmp_version:2",
				"device_namespace:default",
				"snmp_device:127.0.0.1",
			},
		},
		{
			description: "test enum variable resolution with bad variable",
			trap:        BadValueExampleV2Trap,
			resolver:    resolverWithData,
			expectedContent: map[string]interface{}{
				"snmpTrapName":  "linkUp",
				"snmpTrapMIB":   "IF-MIB",
				"snmpTrapOID":   "1.3.6.1.6.3.1.1.5.4",
				"ifIndex":       float64(9001),
				"ifAdminStatus": "test",
				"ifOperStatus":  "lowerLayerDown",
				"uptime":        float64(1000),
				"variables": []interface{}{
					map[string]interface{}{
						"oid":   "1.3.6.1.2.1.2.2.1.1",
						"type":  "integer",
						"value": float64(9001),
					},
					map[string]interface{}{
						"oid":   "1.3.6.1.2.1.2.2.1.8",
						"type":  "integer",
						"value": float64(7),
					},
					map[string]interface{}{
						"oid":   "1.3.6.1.2.1.2.2.1.7",
						"type":  "integer",
						"value": "test",
					},
				},
			},
			expectedTags: []string{
				"snmp_version:2",
				"device_namespace:default",
				"snmp_device:127.0.0.1",
			},
		},
		{
			description: "test enum variable resolution when mapping absent",
			trap:        NoEnumMappingExampleV2Trap,
			resolver:    resolverWithData,
			expectedContent: map[string]interface{}{
				"snmpTrapName":  "linkUp",
				"snmpTrapMIB":   "IF-MIB",
				"snmpTrapOID":   "1.3.6.1.6.3.1.1.5.4",
				"ifIndex":       float64(9001),
				"ifAdminStatus": float64(8),
				"ifOperStatus":  "lowerLayerDown",
				"uptime":        float64(1000),
				"variables": []interface{}{
					map[string]interface{}{
						"oid":   "1.3.6.1.2.1.2.2.1.1",
						"type":  "integer",
						"value": float64(9001),
					},
					map[string]interface{}{
						"oid":   "1.3.6.1.2.1.2.2.1.8",
						"type":  "integer",
						"value": float64(7),
					},
					map[string]interface{}{
						"oid":   "1.3.6.1.2.1.2.2.1.7",
						"type":  "integer",
						"value": float64(8),
					},
				},
			},
			expectedTags: []string{
				"snmp_version:2",
				"device_namespace:default",
				"snmp_device:127.0.0.1",
			},
		},
	}

	for _, d := range data {
		t.Run(d.description, func(t *testing.T) {
			formatter, err := NewJSONFormatter(d.resolver)
			require.NoError(t, err)
			packet := createTestPacket(d.trap)
			data, err := formatter.FormatPacket(packet)
			require.NoError(t, err)
			content := make(map[string]interface{})
			json.Unmarshal(data, &content)

			// map comparisons shouldn't be reliant on ordering with this lib
			// however variables are a slice, they must be sorted
			sort.Stable(byOID(content["variables"].([]interface{})))
			sort.Stable(byOID(d.expectedContent["variables"].([]interface{})))
			if diff := cmp.Diff(content, d.expectedContent); diff != "" {
				t.Error(diff)
			}

			// sort strings lexographically as comparison is order sensitive
			tags := formatter.GetTags(packet)
			sort.Strings(tags)
			sort.Strings(d.expectedTags)
			if diff := cmp.Diff(tags, d.expectedTags); diff != "" {
				t.Error(diff)
			}
		})
	}
}

func TestFormatterWithResolverAndTrapV1Generic(t *testing.T) {
	formatter, err := NewJSONFormatter(resolverWithData)
	require.NoError(t, err)
	packet := createTestV1GenericPacket()
	data, err := formatter.FormatPacket(packet)
	require.NoError(t, err)
	content := make(map[string]interface{})
	json.Unmarshal(data, &content)

	assert.EqualValues(t, "ifDown", content["snmpTrapName"])
	assert.EqualValues(t, "IF-MIB", content["snmpTrapMIB"])
	assert.EqualValues(t, 2, content["ifIndex"])
	assert.EqualValues(t, "up", content["ifAdminStatus"])
	assert.EqualValues(t, "down", content["ifOperStatus"])

	tags := formatter.GetTags(packet)
	assert.Equal(t, tags, []string{
		"snmp_version:1",
		"device_namespace:default",
		"snmp_device:127.0.0.1",
	})
}
