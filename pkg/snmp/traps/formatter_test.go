// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package traps

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type (
	// NoOpOIDResolver is a dummy OIDResolver implementation that is unable to get any Trap or Variable metadata.
	NoOpOIDResolver struct{}
)

// GetTrapMetadata always return an error in this OIDResolver implementation
func (or NoOpOIDResolver) GetTrapMetadata(trapOID string) (TrapMetadata, error) {
	return TrapMetadata{}, fmt.Errorf("trap OID %s is not defined", trapOID)
}

// GetVariableMetadata always return an error in this OIDResolver implementation
func (or NoOpOIDResolver) GetVariableMetadata(trapOID string, varOID string) (VariableMetadata, error) {
	return VariableMetadata{}, fmt.Errorf("trap OID %s is not defined", trapOID)
}

var (
	defaultFormatter, _ = NewJSONFormatter(NoOpOIDResolver{}, "totoro")

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

	// LinkUp Example Trap with injected BITS value V2+
	BitsValueExampleV2Trap = gosnmp.SnmpTrap{
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
			// pwCepSonetConfigErrorOrStatus
			// This translates to binary 1100 0000 0000 0000
			// this means bits 0 and 1 are set
			{Name: "1.3.6.1.2.1.200.1.1.1.3", Type: gosnmp.OctetString, Value: []byte{0xc0, 0x00}},
		},
	}

	// LinkUp Example Trap with injected BITS value V2+
	BitsMissingValueExampleV2Trap = gosnmp.SnmpTrap{
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
			// myFakeVarType
			// Bits 0, 1, 2, 3, 12, 13, 14, 15, 88, and 130 are set
			{Name: "1.3.6.1.2.1.200.1.3.1.5", Type: gosnmp.OctetString, Value: []byte{0xf0, 0x0f, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x80, 0, 0, 0, 0, 0x20}},
		},
	}

	// LinkUp Example Trap with injected BITS value V2+
	BitsZeroedOutValueExampleV2Trap = gosnmp.SnmpTrap{
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
			// myFakeVarType
			// Bits 0, 1, 2, 3, 12, 13, 14, 15, 88, and 130 are set
			{Name: "1.3.6.1.2.1.200.1.3.1.5", Type: gosnmp.OctetString, Value: []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
		},
	}

	// LinkUp Example Trap with OID of malformed trap variable
	// containing mappings for both integer and bits
	InvalidTrapDefinitionExampleV2Trap = gosnmp.SnmpTrap{
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
			// myBadVarType
			// Bits 0, 1, 2, 3, 12, 13, 14, 15, 88, and 130 are set
			{Name: "1.3.6.1.2.1.200.1.3.1.6", Type: gosnmp.OctetString, Value: []byte{0xf0, 0x0f, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x80, 0, 0, 0, 0, 0x20}},
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

	// LinkUp Example Trap with bad BITS values v2+
	BadBitsValueExampleV2Trap = gosnmp.SnmpTrap{
		Variables: []gosnmp.SnmpPDU{
			// sysUpTimeInstance
			{Name: "1.3.6.1.2.1.1.3.0", Type: gosnmp.TimeTicks, Value: uint32(1000)},
			// snmpTrapOID
			{Name: "1.3.6.1.6.3.1.1.4.1.0", Type: gosnmp.OctetString, Value: "1.3.6.1.6.3.1.1.5.4"},
			// ifIndex
			{Name: "1.3.6.1.2.1.2.2.1.1", Type: gosnmp.Integer, Value: 9001},
			// myFakeVarType
			{Name: "1.3.6.1.2.1.200.1.3.1.5", Type: gosnmp.OctetString, Value: 1}, // type is set to octet string, but value is integer
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
	trapContent := data["trap"].(map[string]interface{})

	assert.Equal(t, "snmp-traps", trapContent["ddsource"])
	assert.Equal(t, "snmp_version:1,device_namespace:totoro,snmp_device:127.0.0.1", trapContent["ddtags"])

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
	assert.Equal(t, "snmp_version:1,device_namespace:totoro,snmp_device:127.0.0.1", trapContent["ddtags"])

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
	packet := createTestPacket(NetSNMPExampleHeartbeatNotification)

	formattedPacket, err := defaultFormatter.FormatPacket(packet)
	require.NoError(t, err)
	data := make(map[string]interface{})
	err = json.Unmarshal(formattedPacket, &data)
	require.NoError(t, err)
	trapContent := data["trap"].(map[string]interface{})

	assert.Equal(t, "snmp-traps", trapContent["ddsource"])
	assert.Equal(t, "snmp_version:2,device_namespace:totoro,snmp_device:127.0.0.1", trapContent["ddtags"])

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
	assert.Equal(t, defaultFormatter.getTags(packet), []string{
		"snmp_version:2",
		"device_namespace:totoro",
		"snmp_device:127.0.0.1",
	})
}

func TestGetTagsForUnsupportedVersionShouldStillSucceed(t *testing.T) {
	packet := createTestPacket(NetSNMPExampleHeartbeatNotification)
	packet.Content.Version = 12
	assert.Equal(t, defaultFormatter.getTags(packet), []string{
		"snmp_version:unknown",
		"device_namespace:totoro",
		"snmp_device:127.0.0.1",
	})
}

func TestNewJSONFormatterWithNilStillWorks(t *testing.T) {
	var formatter, err = NewJSONFormatter(NoOpOIDResolver{}, "mononoke")
	require.NoError(t, err)
	packet := createTestPacket(NetSNMPExampleHeartbeatNotification)
	_, err = formatter.FormatPacket(packet)
	require.NoError(t, err)
	tags := formatter.getTags(packet)
	assert.Equal(t, tags, []string{
		"snmp_version:2",
		"device_namespace:mononoke",
		"snmp_device:127.0.0.1",
	})
}

func TestFormatterWithResolverAndTrapV2(t *testing.T) {
	data := []struct {
		description     string
		trap            gosnmp.SnmpTrap
		resolver        *MockedResolver
		namespace       string
		expectedContent map[string]interface{}
		expectedTags    []string
	}{
		{
			description: "test no enum variable resolution with netSnmpExampleHeartbeatNotification",
			trap:        NetSNMPExampleHeartbeatNotification,
			resolver:    resolverWithData,
			namespace:   "totoro",
			expectedContent: map[string]interface{}{
				"ddsource":                    "snmp-traps",
				"ddtags":                      "snmp_version:2,device_namespace:totoro,snmp_device:127.0.0.1",
				"timestamp":                   0.,
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
				"device_namespace:totoro",
				"snmp_device:127.0.0.1",
			},
		},
		{
			description: "test enum variable resolution with linkDown",
			trap:        LinkUpExampleV2Trap,
			resolver:    resolverWithData,
			namespace:   "mononoke",
			expectedContent: map[string]interface{}{
				"ddsource":      "snmp-traps",
				"ddtags":        "snmp_version:2,device_namespace:mononoke,snmp_device:127.0.0.1",
				"timestamp":     0.,
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
						"oid":   "1.3.6.1.2.1.2.2.1.7",
						"type":  "integer",
						"value": float64(2),
					},
					map[string]interface{}{
						"oid":   "1.3.6.1.2.1.2.2.1.8",
						"type":  "integer",
						"value": float64(7),
					},
				},
			},
			expectedTags: []string{
				"snmp_version:2",
				"device_namespace:mononoke",
				"snmp_device:127.0.0.1",
			},
		},
		{
			description: "test enum variable resolution with bad variable",
			trap:        BadValueExampleV2Trap,
			resolver:    resolverWithData,
			namespace:   "sosuke",
			expectedContent: map[string]interface{}{
				"ddsource":      "snmp-traps",
				"ddtags":        "snmp_version:2,device_namespace:sosuke,snmp_device:127.0.0.1",
				"timestamp":     0.,
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
						"oid":   "1.3.6.1.2.1.2.2.1.7",
						"type":  "integer",
						"value": "test",
					},
					map[string]interface{}{
						"oid":   "1.3.6.1.2.1.2.2.1.8",
						"type":  "integer",
						"value": float64(7),
					},
				},
			},
			expectedTags: []string{
				"snmp_version:2",
				"device_namespace:sosuke",
				"snmp_device:127.0.0.1",
			},
		},
		{
			description: "test enum variable resolution when mapping absent",
			trap:        NoEnumMappingExampleV2Trap,
			resolver:    resolverWithData,
			namespace:   "nausicaa",
			expectedContent: map[string]interface{}{
				"ddsource":      "snmp-traps",
				"ddtags":        "snmp_version:2,device_namespace:nausicaa,snmp_device:127.0.0.1",
				"timestamp":     0.,
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
						"oid":   "1.3.6.1.2.1.2.2.1.7",
						"type":  "integer",
						"value": float64(8),
					},
					map[string]interface{}{
						"oid":   "1.3.6.1.2.1.2.2.1.8",
						"type":  "integer",
						"value": float64(7),
					},
				},
			},
			expectedTags: []string{
				"snmp_version:2",
				"device_namespace:nausicaa",
				"snmp_device:127.0.0.1",
			},
		},
		{
			description: "test enum variable resolution with BITS enum",
			trap:        BitsValueExampleV2Trap,
			resolver:    resolverWithData,
			namespace:   "mononoke",
			expectedContent: map[string]interface{}{
				"ddsource":                      "snmp-traps",
				"ddtags":                        "snmp_version:2,device_namespace:mononoke,snmp_device:127.0.0.1",
				"timestamp":                     0.,
				"snmpTrapName":                  "linkUp",
				"snmpTrapMIB":                   "IF-MIB",
				"snmpTrapOID":                   "1.3.6.1.6.3.1.1.5.4",
				"ifIndex":                       float64(9001),
				"ifAdminStatus":                 "down",
				"ifOperStatus":                  "lowerLayerDown",
				"pwCepSonetConfigErrorOrStatus": []interface{}{string("other"), string("timeslotInUse")},
				"uptime":                        float64(1000),
				"variables": []interface{}{
					map[string]interface{}{
						"oid":   "1.3.6.1.2.1.2.2.1.1",
						"type":  "integer",
						"value": float64(9001),
					},
					map[string]interface{}{
						"oid":   "1.3.6.1.2.1.2.2.1.7",
						"type":  "integer",
						"value": float64(2),
					},
					map[string]interface{}{
						"oid":   "1.3.6.1.2.1.2.2.1.8",
						"type":  "integer",
						"value": float64(7),
					},
					map[string]interface{}{
						"oid":   "1.3.6.1.2.1.200.1.1.1.3",
						"type":  "string",
						"value": base64.StdEncoding.EncodeToString([]byte{0xc0, 0x00}),
					},
				},
			},
			expectedTags: []string{
				"snmp_version:2",
				"device_namespace:mononoke",
				"snmp_device:127.0.0.1",
			},
		},
		{
			description: "test enum variable resolution with BITS enum and some missing bits definitions",
			trap:        BitsMissingValueExampleV2Trap,
			resolver:    resolverWithData,
			namespace:   "mononoke",
			expectedContent: map[string]interface{}{
				"ddsource":      "snmp-traps",
				"ddtags":        "snmp_version:2,device_namespace:mononoke,snmp_device:127.0.0.1",
				"timestamp":     0.,
				"snmpTrapName":  "linkUp",
				"snmpTrapMIB":   "IF-MIB",
				"snmpTrapOID":   "1.3.6.1.6.3.1.1.5.4",
				"ifIndex":       float64(9001),
				"ifAdminStatus": "down",
				"ifOperStatus":  "lowerLayerDown",
				"myFakeVarType": []interface{}{
					string("test0"),
					string("test1"),
					float64(2),
					string("test3"),
					string("test12"),
					float64(13),
					float64(14),
					string("test15"),
					float64(88),
					string("test130"),
				},
				"uptime": float64(1000),
				"variables": []interface{}{
					map[string]interface{}{
						"oid":   "1.3.6.1.2.1.2.2.1.1",
						"type":  "integer",
						"value": float64(9001),
					},
					map[string]interface{}{
						"oid":   "1.3.6.1.2.1.2.2.1.7",
						"type":  "integer",
						"value": float64(2),
					},
					map[string]interface{}{
						"oid":   "1.3.6.1.2.1.2.2.1.8",
						"type":  "integer",
						"value": float64(7),
					},
					map[string]interface{}{
						"oid":   "1.3.6.1.2.1.200.1.3.1.5",
						"type":  "string",
						"value": base64.StdEncoding.EncodeToString([]byte{0xf0, 0x0f, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x80, 0, 0, 0, 0, 0x20}),
					},
				},
			},
			expectedTags: []string{
				"snmp_version:2",
				"device_namespace:mononoke",
				"snmp_device:127.0.0.1",
			},
		},
		{
			description: "test BITS variable resolution with bad variable",
			trap:        BadBitsValueExampleV2Trap,
			resolver:    resolverWithData,
			namespace:   "sosuke",
			expectedContent: map[string]interface{}{
				"ddsource":      "snmp-traps",
				"ddtags":        "snmp_version:2,device_namespace:sosuke,snmp_device:127.0.0.1",
				"timestamp":     0.,
				"snmpTrapName":  "linkUp",
				"snmpTrapMIB":   "IF-MIB",
				"snmpTrapOID":   "1.3.6.1.6.3.1.1.5.4",
				"ifIndex":       float64(9001),
				"myFakeVarType": float64(1),
				"uptime":        float64(1000),
				"variables": []interface{}{
					map[string]interface{}{
						"oid":   "1.3.6.1.2.1.2.2.1.1",
						"type":  "integer",
						"value": float64(9001),
					},
					map[string]interface{}{
						"oid":   "1.3.6.1.2.1.200.1.3.1.5",
						"type":  "string",
						"value": float64(1),
					},
				},
			},
			expectedTags: []string{
				"snmp_version:2",
				"device_namespace:sosuke",
				"snmp_device:127.0.0.1",
			},
		},
		{
			description: "values returned unenriched when var definition contains both enum and bits",
			trap:        InvalidTrapDefinitionExampleV2Trap,
			resolver:    resolverWithData,
			namespace:   "mononoke",
			expectedContent: map[string]interface{}{
				"ddsource":      "snmp-traps",
				"ddtags":        "snmp_version:2,device_namespace:mononoke,snmp_device:127.0.0.1",
				"timestamp":     0.,
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
						"oid":   "1.3.6.1.2.1.2.2.1.7",
						"type":  "integer",
						"value": float64(2),
					},
					map[string]interface{}{
						"oid":   "1.3.6.1.2.1.2.2.1.8",
						"type":  "integer",
						"value": float64(7),
					},
					map[string]interface{}{
						"oid":   "1.3.6.1.2.1.200.1.3.1.6",
						"type":  "string",
						"value": base64.StdEncoding.EncodeToString([]byte{0xf0, 0x0f, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x80, 0, 0, 0, 0, 0x20}),
					},
				},
			},
			expectedTags: []string{
				"snmp_version:2",
				"device_namespace:mononoke",
				"snmp_device:127.0.0.1",
			},
		},
		{
			description: "test enum variable resolution with zeroed out BITS",
			trap:        BitsZeroedOutValueExampleV2Trap,
			resolver:    resolverWithData,
			namespace:   "mononoke",
			expectedContent: map[string]interface{}{
				"ddsource":      "snmp-traps",
				"ddtags":        "snmp_version:2,device_namespace:mononoke,snmp_device:127.0.0.1",
				"timestamp":     0.,
				"snmpTrapName":  "linkUp",
				"snmpTrapMIB":   "IF-MIB",
				"snmpTrapOID":   "1.3.6.1.6.3.1.1.5.4",
				"ifIndex":       float64(9001),
				"ifAdminStatus": "down",
				"ifOperStatus":  "lowerLayerDown",
				"myFakeVarType": []interface{}{},
				"uptime":        float64(1000),
				"variables": []interface{}{
					map[string]interface{}{
						"oid":   "1.3.6.1.2.1.2.2.1.1",
						"type":  "integer",
						"value": float64(9001),
					},
					map[string]interface{}{
						"oid":   "1.3.6.1.2.1.2.2.1.7",
						"type":  "integer",
						"value": float64(2),
					},
					map[string]interface{}{
						"oid":   "1.3.6.1.2.1.2.2.1.8",
						"type":  "integer",
						"value": float64(7),
					},
					map[string]interface{}{
						"oid":   "1.3.6.1.2.1.200.1.3.1.5",
						"type":  "string",
						"value": base64.StdEncoding.EncodeToString([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}),
					},
				},
			},
			expectedTags: []string{
				"snmp_version:2",
				"device_namespace:mononoke",
				"snmp_device:127.0.0.1",
			},
		},
	}

	for _, d := range data {
		t.Run(d.description, func(t *testing.T) {
			formatter, err := NewJSONFormatter(d.resolver, d.namespace)
			require.NoError(t, err)
			packet := createTestPacket(d.trap)
			data, err := formatter.FormatPacket(packet)
			require.NoError(t, err)
			content := make(map[string]interface{})
			err = json.Unmarshal(data, &content)
			require.NoError(t, err)
			trapContent := content["trap"].(map[string]interface{})
			// map comparisons shouldn't be reliant on ordering with this lib
			// however variables are a slice, they must be sorted
			if diff := cmp.Diff(trapContent, d.expectedContent); diff != "" {
				t.Error(diff)
			}

			// sort strings lexographically as comparison is order sensitive
			tags := formatter.getTags(packet)
			sort.Strings(tags)
			sort.Strings(d.expectedTags)
			if diff := cmp.Diff(tags, d.expectedTags); diff != "" {
				t.Error(diff)
			}
		})
	}
}

func TestFormatterWithResolverAndTrapV1Generic(t *testing.T) {
	myFakeVarTypeExpected := []interface{}{
		"test0",
		"test1",
		float64(2),
		"test3",
		"test12",
		float64(13),
		float64(14),
		"test15",
		float64(95),
		"test130",
	}

	formatter, err := NewJSONFormatter(resolverWithData, "porco_rosso")
	require.NoError(t, err)
	packet := createTestV1GenericPacket()
	data, err := formatter.FormatPacket(packet)
	require.NoError(t, err)
	content := make(map[string]interface{})
	err = json.Unmarshal(data, &content)
	require.NoError(t, err)
	trapContent := content["trap"].(map[string]interface{})

	assert.Equal(t, "snmp-traps", trapContent["ddsource"])
	assert.Equal(t, "snmp_version:1,device_namespace:porco_rosso,snmp_device:127.0.0.1", trapContent["ddtags"])

	assert.EqualValues(t, "ifDown", trapContent["snmpTrapName"])
	assert.EqualValues(t, "IF-MIB", trapContent["snmpTrapMIB"])
	assert.EqualValues(t, 2, trapContent["ifIndex"])
	assert.EqualValues(t, "up", trapContent["ifAdminStatus"])
	assert.EqualValues(t, "down", trapContent["ifOperStatus"])
	assert.EqualValues(t, myFakeVarTypeExpected, trapContent["myFakeVarType"])

	tags := formatter.getTags(packet)
	assert.Equal(t, tags, []string{
		"snmp_version:1",
		"device_namespace:porco_rosso",
		"snmp_device:127.0.0.1",
	})
}

func TestIsBitEnabled(t *testing.T) {
	data := []struct {
		description string
		input       byte
		position    int
		expected    bool
		errMsg      string
	}{
		{
			description: "negative position should error",
			input:       0xff,
			position:    -1,
			expected:    false,
			errMsg:      "invalid position",
		},
		{
			description: "position >7 should error",
			input:       0xff,
			position:    8,
			expected:    false,
			errMsg:      "invalid position",
		},
		{
			description: "position 7 unset should return false",
			input:       0xfe, // 1111 1110
			position:    7,
			expected:    false,
			errMsg:      "",
		},
		{
			description: "position 7 set should return true",
			input:       0x01, // 0000 0001
			position:    7,
			expected:    true,
			errMsg:      "",
		},
		{
			description: "position 0 unset should return false",
			input:       0x7f, // 0111 1111
			position:    0,
			expected:    false,
			errMsg:      "",
		},
		{
			description: "position 0 set should return true",
			input:       0x80, // 1000 0000
			position:    0,
			expected:    true,
			errMsg:      "",
		},
		{
			description: "position 3 unset should return false",
			input:       0xef, // 1110 1111
			position:    3,
			expected:    false,
			errMsg:      "",
		},
		{
			description: "position 3 set should return true",
			input:       0x10, // 0001 0000
			position:    3,
			expected:    true,
			errMsg:      "",
		},
		{
			description: "position 4 unset should return false",
			input:       0xf7, // 1110 1111
			position:    4,
			expected:    false,
			errMsg:      "",
		},
		{
			description: "position 4 set should return true",
			input:       0x08, // 0000 1000
			position:    4,
			expected:    true,
			errMsg:      "",
		},
	}

	for _, d := range data {
		t.Run(d.description, func(t *testing.T) {
			actual, err := isBitEnabled(d.input, d.position)
			var errMsg string
			if err != nil {
				errMsg = err.Error()
			}
			if !strings.Contains(errMsg, d.errMsg) {
				t.Errorf("error message mismatch, wanted %q, got %q", d.errMsg, errMsg)
			}

			if actual != d.expected {
				t.Errorf("result mismatch, wanted %t, got %t", d.expected, actual)
			}
		})
	}
}

func TestEnrichBits(t *testing.T) {
	data := []struct {
		description string
		variable    trapVariable
		varMetadata VariableMetadata
		expected    interface{}
	}{
		{
			description: "all bits are enrichable and are enriched",
			variable:    trapVariable{OID: ".1.4.3.6.7.3.4.1.4.7", VarType: "string", Value: []byte{0b11000100, 0b10000001}}, // made up OID, bits 0, 1, 5, 8, and 15 set
			varMetadata: VariableMetadata{
				Name: "myDummyVariable",
				Bits: map[int]string{
					0:  "test0",
					1:  "test1",
					2:  "test2",
					5:  "test5",
					8:  "test8",
					15: "test15",
				},
			},
			expected: []interface{}{
				"test0",
				"test1",
				"test5",
				"test8",
				"test15",
			},
		},
		{
			description: "no bits are enrichable are returned unenriched",
			variable:    trapVariable{OID: ".1.4.3.6.7.3.4.1.4.7", VarType: "string", Value: []byte{0b11000100, 0b10000001}}, // made up OID, bits 0, 1, 5, 8, and 15 set
			varMetadata: VariableMetadata{
				Name: "myDummyVariable",
				Bits: map[int]string{
					2:  "test2",
					4:  "test4",
					6:  "test6",
					14: "test14",
				},
			},
			expected: []interface{}{
				0,
				1,
				5,
				8,
				15,
			},
		},
		{
			description: "mix of enrichable and unenrichable bits are returned semi-enriched",
			variable:    trapVariable{OID: ".1.4.3.6.7.3.4.1.4.7", VarType: "string", Value: []byte{0b00111000, 0b01000010}}, // made up OID, bits 2, 3, 4, 9, 14 are set
			varMetadata: VariableMetadata{
				Name: "myDummyVariable",
				Bits: map[int]string{
					2:  "test2",
					4:  "test4",
					6:  "test6",
					14: "test14",
				},
			},
			expected: []interface{}{
				"test2",
				3,
				"test4",
				9,
				"test14",
			},
		},
		{
			description: "non-byte array value returns original value unchanged",
			variable:    trapVariable{OID: ".1.4.3.6.7.3.4.1.4.7", VarType: "string", Value: 42},
			varMetadata: VariableMetadata{
				Name: "myDummyVariable",
				Bits: map[int]string{
					2:  "test2",
					4:  "test4",
					6:  "test6",
					14: "test14",
				},
			},
			expected: 42,
		},
		{
			description: "completely zeroed out bits returns zeroed out bits",
			variable:    trapVariable{OID: ".1.4.3.6.7.3.4.1.4.7", VarType: "string", Value: []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
			varMetadata: VariableMetadata{
				Name: "myDummyVariable",
				Bits: map[int]string{
					2:  "test2",
					4:  "test4",
					6:  "test6",
					14: "test14",
				},
			},
			expected: []interface{}{},
		},
	}

	for _, d := range data {
		t.Run(d.description, func(t *testing.T) {
			actual := enrichBits(d.variable, d.varMetadata)
			if diff := cmp.Diff(d.expected, actual); diff != "" {
				t.Error(diff)
			}
		})
	}
}

func TestIsValidOID_PropertyBasedTesting(t *testing.T) {
	rand.Seed(time.Now().Unix())
	testSize := 100
	validOIDs := make([]string, testSize)
	for i := 0; i < testSize; i++ {
		// Valid cases
		oidLen := rand.Intn(100) + 2
		oidParts := make([]string, oidLen)
		for j := 0; j < oidLen; j++ {
			oidParts[j] = fmt.Sprint(rand.Intn(100000))
		}
		recreatedOID := strings.Join(oidParts, ".")
		if rand.Intn(2) == 0 {
			recreatedOID = "." + recreatedOID
		}
		validOIDs[i] = recreatedOID
		require.True(t, IsValidOID(validOIDs[i]), "OID: %s", validOIDs[i])
	}

	var invalidRunes = []rune(",?><|\\}{[]()*&^%$#@!abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	for i := 0; i < testSize; i++ {
		// Valid cases
		oid := validOIDs[i]
		x := 0
		switch x = rand.Intn(3); x {
		case 0:
			// Append a dot at the end, this is not possible
			oid = oid + "."
		case 1:
			// Append a random invalid character anywhere
			randomRune := invalidRunes[rand.Intn(len(invalidRunes))]
			randomIdx := rand.Intn(len(oid))
			oid = oid[:randomIdx] + string(randomRune) + oid[randomIdx:]
		case 2:
			// Put two dots next to each other
			oidParts := strings.Split(oid, ".")
			randomIdx := rand.Intn(len(oidParts)-1) + 1
			oidParts[randomIdx] = "." + oidParts[randomIdx]
			oid = strings.Join(oidParts, ".")
		}

		require.False(t, IsValidOID(oid), "OID: %s", oid)
	}
}

func TestIsValidOID_Unit(t *testing.T) {
	cases := map[string]bool{
		"1.3.6.1.4.1.4962.2.1.6.3":       true,
		".1.3.6.1.4.1.4962.2.1.6.999999": true,
		"1":                              true,
		"1.3.6.1.4.1.4962.2.1.-6.3":      false,
		"1.3.6.1.4.1..4962.2.1.6.3":      false,
		"1.3.6.1foo.4.1.4962.2.1.6.3":    false,
		"1.3.6.1foo.4.1.4962_2.1.6.3":    false,
		"1.3.6.1.4.1.4962.2.1.6.999999.": false,
	}

	for oid, expected := range cases {
		require.Equal(t, expected, IsValidOID(oid))
	}
}

func TestVariableValueFormat(t *testing.T) {
	data := []struct {
		description string
		variable    gosnmp.SnmpPDU
		expected    interface{}
	}{

		{
			description: "type integer is correctly formatted",
			variable:    gosnmp.SnmpPDU{Name: "1.3.6.1.2.1.200.1.3.1.7", Type: gosnmp.Integer, Value: 10},
			expected:    10,
		},
		{
			description: "type OID is normalized",
			variable:    gosnmp.SnmpPDU{Name: "1.3.6.1.2.1.200.1.3.1.7", Type: gosnmp.ObjectIdentifier, Value: ".1.3.6.1.6.3.1.1.5.4"},
			expected:    "1.3.6.1.6.3.1.1.5.4",
		},
		{
			description: "type OID is normalized only if necessary",
			variable:    gosnmp.SnmpPDU{Name: "1.3.6.1.2.1.200.1.3.1.7", Type: gosnmp.ObjectIdentifier, Value: "1.3.6.1.6.3.1.1.5.4"},
			expected:    "1.3.6.1.6.3.1.1.5.4",
		},
		{
			description: "type OID with incorrect value",
			variable:    gosnmp.SnmpPDU{Name: "1.3.6.1.2.1.200.1.3.1.7", Type: gosnmp.ObjectIdentifier, Value: 1},
			expected:    1,
		},
		{
			description: "[]byte values are converted to string",
			variable:    gosnmp.SnmpPDU{Name: "1.3.6.1.2.1.200.1.3.1.7", Type: gosnmp.OctetString, Value: []byte{0x74, 0x65, 0x73, 0x74}},
			expected:    "test",
		},
		{
			description: "[]byte value is not normalized",
			variable:    gosnmp.SnmpPDU{Name: "1.3.6.1.2.1.200.1.3.1.7", Type: gosnmp.OctetString, Value: []byte{0x2e, 0x74, 0x65, 0x73, 0x74}},
			expected:    ".test",
		},
	}

	for _, d := range data {
		require.Equal(t, d.expected, formatValue(d.variable), d.description)
	}
}
