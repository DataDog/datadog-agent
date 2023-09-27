// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package formatter

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/ndmtmp/sender"
	"github.com/DataDog/datadog-agent/comp/snmptraps/oidresolver"
	"github.com/DataDog/datadog-agent/comp/snmptraps/packet"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"

	"github.com/google/go-cmp/cmp"
	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
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
			// No bits are set
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

	// Example Trap with not enough variables
	NotEnoughVarsExampleV2Trap = gosnmp.SnmpTrap{
		Variables: []gosnmp.SnmpPDU{
			// snmpTrapOID
			{Name: "1.3.6.1.6.3.1.1.4.1.0", Type: gosnmp.OctetString, Value: "1.3.6.1.6.3.1.1.5.4"}},
	}

	// Example Trap missing sysUpTimeInstance
	MissingSysUpTimeInstanceExampleV2Trap = gosnmp.SnmpTrap{
		Variables: []gosnmp.SnmpPDU{
			// snmpTrapOID
			{Name: "1.3.6.1.6.3.1.1.4.1.0", Type: gosnmp.OctetString, Value: "1.3.6.1.6.3.1.1.5.4"},
			// ifIndex
			{Name: "1.3.6.1.2.1.2.2.1.1", Type: gosnmp.Integer, Value: 9001},
			// myFakeVarType
			{Name: "1.3.6.1.2.1.200.1.3.1.5", Type: gosnmp.Integer, Value: 1},
		},
	}

	// Example Trap missing trap OID
	MissingTrapOIDExampleV2Trap = gosnmp.SnmpTrap{
		Variables: []gosnmp.SnmpPDU{
			// sysUpTimeInstance
			{Name: "1.3.6.1.2.1.1.3.0", Type: gosnmp.TimeTicks, Value: uint32(1000)},
			// ifIndex
			{Name: "1.3.6.1.2.1.2.2.1.1", Type: gosnmp.Integer, Value: 9001},
			// myFakeVarType
			{Name: "1.3.6.1.2.1.200.1.3.1.5", Type: gosnmp.Integer, Value: 1},
		},
	}

	// Example Unknown Trap
	UnknownExampleV2Trap = gosnmp.SnmpTrap{
		Variables: []gosnmp.SnmpPDU{
			// sysUpTimeInstance
			{Name: "1.3.6.1.2.1.1.3.0", Type: gosnmp.TimeTicks, Value: uint32(1000)},
			// snmpTrapOID
			{Name: "1.3.6.1.6.3.1.1.4.1.0", Type: gosnmp.OctetString, Value: "1.3.6.1.6.3.1.1234.4321"},
			// Fake
			{Name: "1.3.6.1.6.3.1.1234.4321.1", Type: gosnmp.Integer, Value: 1},
			// Fake
			{Name: "1.3.6.1.6.3.1.1234.4321.2", Type: gosnmp.Integer, Value: 2},
		},
	}
)

var testOptions = fx.Options(
	log.MockModule,
	sender.MockModule,
	oidresolver.MockModule,
	Module,
)

func TestFormatPacketV1Generic(t *testing.T) {
	defaultFormatter := fxutil.Test[Component](t, testOptions)

	packet := packet.CreateTestV1GenericPacket()
	formattedPacket, err := defaultFormatter.FormatPacket(packet)
	require.NoError(t, err)
	data := make(map[string]interface{})
	err = json.Unmarshal(formattedPacket, &data)
	require.NoError(t, err)
	trapContent := data["trap"].(map[string]interface{})

	assert.Equal(t, "snmp-traps", trapContent["ddsource"])
	assert.Equal(t, "snmp_version:1,device_namespace:the_baron,snmp_device:127.0.0.1", trapContent["ddtags"])

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
	defaultFormatter := fxutil.Test[Component](t, testOptions)
	packet := packet.CreateTestV1SpecificPacket()
	formattedPacket, err := defaultFormatter.FormatPacket(packet)
	require.NoError(t, err)
	data := make(map[string]interface{})
	err = json.Unmarshal(formattedPacket, &data)
	require.NoError(t, err)
	trapContent := data["trap"].(map[string]interface{})

	assert.Equal(t, "snmp-traps", trapContent["ddsource"])
	assert.Equal(t, "snmp_version:1,device_namespace:catbus,snmp_device:127.0.0.1", trapContent["ddtags"])

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
	defaultFormatter := fxutil.Test[Component](t, testOptions)
	packet := packet.CreateTestPacket(packet.NetSNMPExampleHeartbeatNotification)

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
	defaultFormatter := fxutil.Test[Component](t, testOptions)
	packet := packet.CreateTestPacket(packet.NetSNMPExampleHeartbeatNotification)

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

func TestNewJSONFormatterWithNilStillWorks(t *testing.T) {
	formatter := fxutil.Test[Component](t, testOptions)
	packet := packet.CreateTestPacket(packet.NetSNMPExampleHeartbeatNotification)
	_, err := formatter.FormatPacket(packet)
	require.NoError(t, err)
}

func TestFormatterWithResolverAndTrapV2(t *testing.T) {
	data := []struct {
		description     string
		trap            gosnmp.SnmpTrap
		expectedContent map[string]interface{}
	}{
		{
			description: "test no enum variable resolution with netSnmpExampleHeartbeatNotification",
			trap:        packet.NetSNMPExampleHeartbeatNotification,
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
		},
		{
			description: "test enum variable resolution with linkDown",
			trap:        LinkUpExampleV2Trap,
			expectedContent: map[string]interface{}{
				"ddsource":      "snmp-traps",
				"ddtags":        "snmp_version:2,device_namespace:totoro,snmp_device:127.0.0.1",
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
		},
		{
			description: "test enum variable resolution with bad variable",
			trap:        BadValueExampleV2Trap,
			expectedContent: map[string]interface{}{
				"ddsource":      "snmp-traps",
				"ddtags":        "snmp_version:2,device_namespace:totoro,snmp_device:127.0.0.1",
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
		},
		{
			description: "test enum variable resolution when mapping absent",
			trap:        NoEnumMappingExampleV2Trap,
			expectedContent: map[string]interface{}{
				"ddsource":      "snmp-traps",
				"ddtags":        "snmp_version:2,device_namespace:totoro,snmp_device:127.0.0.1",
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
		},
		{
			description: "test enum variable resolution with BITS enum",
			trap:        BitsValueExampleV2Trap,
			expectedContent: map[string]interface{}{
				"ddsource":                      "snmp-traps",
				"ddtags":                        "snmp_version:2,device_namespace:totoro,snmp_device:127.0.0.1",
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
						"value": "0xC000",
					},
				},
			},
		},
		{
			description: "test enum variable resolution with BITS enum and some missing bits definitions",
			trap:        BitsMissingValueExampleV2Trap,
			expectedContent: map[string]interface{}{
				"ddsource":      "snmp-traps",
				"ddtags":        "snmp_version:2,device_namespace:totoro,snmp_device:127.0.0.1",
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
						"value": "0xF00F000000000000000000800000000020",
					},
				},
			},
		},
		{
			description: "test BITS variable resolution with bad variable",
			trap:        BadBitsValueExampleV2Trap,
			expectedContent: map[string]interface{}{
				"ddsource":      "snmp-traps",
				"ddtags":        "snmp_version:2,device_namespace:totoro,snmp_device:127.0.0.1",
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
		},
		{
			description: "values returned unenriched when var definition contains both enum and bits",
			trap:        InvalidTrapDefinitionExampleV2Trap,
			expectedContent: map[string]interface{}{
				"ddsource":      "snmp-traps",
				"ddtags":        "snmp_version:2,device_namespace:totoro,snmp_device:127.0.0.1",
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
		},
		{
			description: "test enum variable resolution with zeroed out BITS",
			trap:        BitsZeroedOutValueExampleV2Trap,
			expectedContent: map[string]interface{}{
				"ddsource":      "snmp-traps",
				"ddtags":        "snmp_version:2,device_namespace:totoro,snmp_device:127.0.0.1",
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
						"value": "0x0000000000000000000000000000000000",
					},
				},
			},
		},
	}

	formatter := fxutil.Test[Component](t, testOptions)
	for _, d := range data {
		t.Run(d.description, func(t *testing.T) {
			packet := packet.CreateTestPacket(d.trap)
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

	formatter := fxutil.Test[Component](t, testOptions)
	packet := packet.CreateTestV1GenericPacket()
	data, err := formatter.FormatPacket(packet)
	require.NoError(t, err)
	content := make(map[string]interface{})
	err = json.Unmarshal(data, &content)
	require.NoError(t, err)
	trapContent := content["trap"].(map[string]interface{})

	assert.Equal(t, "snmp-traps", trapContent["ddsource"])
	assert.Equal(t, "snmp_version:1,device_namespace:the_baron,snmp_device:127.0.0.1", trapContent["ddtags"])

	assert.EqualValues(t, "ifDown", trapContent["snmpTrapName"])
	assert.EqualValues(t, "IF-MIB", trapContent["snmpTrapMIB"])
	assert.EqualValues(t, 2, trapContent["ifIndex"])
	assert.EqualValues(t, "up", trapContent["ifAdminStatus"])
	assert.EqualValues(t, "down", trapContent["ifOperStatus"])
	assert.EqualValues(t, myFakeVarTypeExpected, trapContent["myFakeVarType"])
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
	logger := fxutil.Test[log.Component](t, log.MockModule)
	data := []struct {
		description     string
		variable        trapVariable
		varMetadata     oidresolver.VariableMetadata
		expectedMapping interface{}
		expectedHex     string
	}{
		{
			description: "all bits are enrichable and are enriched",
			variable:    trapVariable{OID: ".1.4.3.6.7.3.4.1.4.7", VarType: "string", Value: []byte{0b11000100, 0b10000001}}, // made up OID, bits 0, 1, 5, 8, and 15 set
			varMetadata: oidresolver.VariableMetadata{
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
			expectedMapping: []interface{}{
				"test0",
				"test1",
				"test5",
				"test8",
				"test15",
			},
			expectedHex: "0xC481",
		},
		{
			description: "no bits are enrichable are returned unenriched",
			variable:    trapVariable{OID: ".1.4.3.6.7.3.4.1.4.7", VarType: "string", Value: []byte{0b11000100, 0b10000001}}, // made up OID, bits 0, 1, 5, 8, and 15 set
			varMetadata: oidresolver.VariableMetadata{
				Name: "myDummyVariable",
				Bits: map[int]string{
					2:  "test2",
					4:  "test4",
					6:  "test6",
					14: "test14",
				},
			},
			expectedMapping: []interface{}{
				0,
				1,
				5,
				8,
				15,
			},
			expectedHex: "0xC481",
		},
		{
			description: "mix of enrichable and unenrichable bits are returned semi-enriched",
			variable:    trapVariable{OID: ".1.4.3.6.7.3.4.1.4.7", VarType: "string", Value: []byte{0b00111000, 0b01000010}}, // made up OID, bits 2, 3, 4, 9, 14 are set
			varMetadata: oidresolver.VariableMetadata{
				Name: "myDummyVariable",
				Bits: map[int]string{
					2:  "test2",
					4:  "test4",
					6:  "test6",
					14: "test14",
				},
			},
			expectedMapping: []interface{}{
				"test2",
				3,
				"test4",
				9,
				"test14",
			},
			expectedHex: "0x3842",
		},
		{
			description: "non-byte array value returns original value unchanged",
			variable:    trapVariable{OID: ".1.4.3.6.7.3.4.1.4.7", VarType: "string", Value: 42},
			varMetadata: oidresolver.VariableMetadata{
				Name: "myDummyVariable",
				Bits: map[int]string{
					2:  "test2",
					4:  "test4",
					6:  "test6",
					14: "test14",
				},
			},
			expectedMapping: 42,
			expectedHex:     "",
		},
		{
			description: "completely zeroed out bits returns zeroed out bits",
			variable:    trapVariable{OID: ".1.4.3.6.7.3.4.1.4.7", VarType: "string", Value: []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
			varMetadata: oidresolver.VariableMetadata{
				Name: "myDummyVariable",
				Bits: map[int]string{
					2:  "test2",
					4:  "test4",
					6:  "test6",
					14: "test14",
				},
			},
			expectedMapping: []interface{}{},
			expectedHex:     "0x000000000000",
		},
	}

	for _, d := range data {
		t.Run(d.description, func(t *testing.T) {
			actualMapping, actualHex := enrichBits(d.variable, d.varMetadata, logger)
			if diff := cmp.Diff(d.expectedMapping, actualMapping); diff != "" {
				t.Error(diff)
			}
			require.Equal(t, d.expectedHex, actualHex)
		})
	}
}

func TestVariableTypeFormat(t *testing.T) {
	data := []struct {
		description string
		variable    gosnmp.SnmpPDU
		expected    interface{}
	}{

		{
			description: "type gosnmp.Integer is correctly formatted",
			variable:    gosnmp.SnmpPDU{Name: "1.3.6.1.2.1.200.1.3.1.7", Type: gosnmp.Integer, Value: 10},
			expected:    "integer",
		},
		{
			description: "type gosnmp.Boolean is correctly formatted",
			variable:    gosnmp.SnmpPDU{Name: "1.3.6.1.2.1.200.1.3.1.7", Type: gosnmp.Boolean, Value: 1},
			expected:    "boolean",
		},
		{
			description: "type gosnmp.Uinteger32 is correctly formatted",
			variable:    gosnmp.SnmpPDU{Name: "1.3.6.1.2.1.200.1.3.1.7", Type: gosnmp.Uinteger32, Value: 15},
			expected:    "integer",
		},
		{
			description: "type gosnmp.Opaque is correctly formatted",
			variable:    gosnmp.SnmpPDU{Name: "1.3.6.1.4.1.6574.4.2.12.1.0", Type: gosnmp.Opaque, Value: 0.65},
			expected:    "opaque",
		},
		{
			description: "type gosnmp.OpaqueFloat is correctly formatted",
			variable:    gosnmp.SnmpPDU{Name: "1.3.6.1.4.1.6574.4.2.12.1.0", Type: gosnmp.OpaqueFloat, Value: 0.657685},
			expected:    "opaque",
		},
		{
			description: "type gosnmp.OpaqueDouble is correctly formatted",
			variable:    gosnmp.SnmpPDU{Name: "1.3.6.1.4.1.6574.4.2.12.1.0", Type: gosnmp.OpaqueDouble, Value: 0.685},
			expected:    "opaque",
		},
		{
			description: "type gosnmp.ObjectIdentifier is correctly formatted",
			variable:    gosnmp.SnmpPDU{Name: "1.3.6.1.2.1.200.1.3.1.7", Type: gosnmp.ObjectIdentifier, Value: "1.3.6.7.8"},
			expected:    "oid",
		},
		{
			description: "type gosnmp.OctetString is correctly formatted",
			variable:    gosnmp.SnmpPDU{Name: "1.3.6.1.4.1.193.183.4.1.4.5.1.8", Type: gosnmp.OctetString, Value: "teststring"},
			expected:    "string",
		},
		{
			description: "type gosnmp.BitString is correctly formatted",
			variable:    gosnmp.SnmpPDU{Name: "1.3.6.1.4.1.193.183.4.1.4.5.1.8", Type: gosnmp.BitString, Value: []byte{0x74, 0x65, 0x73, 0x74}},
			expected:    "string",
		},
		{
			description: "type gosnmp.IPAddress is correctly formatted",
			variable:    gosnmp.SnmpPDU{Name: "1.3.6.1.2.1.4.20.1.1", Type: gosnmp.IPAddress, Value: "127.0.0.1"},
			expected:    "ip-address",
		},
		{
			description: "type gosnmp.TimeTicks is correctly formatted",
			variable:    gosnmp.SnmpPDU{Name: "1.3.6.1.4.1.232.18.2.2.1.1.17", Type: gosnmp.TimeTicks, Value: 156},
			expected:    "time-ticks",
		},
		{
			description: "type gosnmp.Gauge32 is correctly formatted",
			variable:    gosnmp.SnmpPDU{Name: "1.3.6.1.4.1.232.18.2.2.1.1.17", Type: gosnmp.Gauge32, Value: 6},
			expected:    "gauge32",
		},
		{
			description: "type gosnmp.Counter32 is correctly formatted",
			variable:    gosnmp.SnmpPDU{Name: "1.3.6.1.2.1.2.2.1.15", Type: gosnmp.Counter32, Value: 34},
			expected:    "counter32",
		},
		{
			description: "type gosnmp.Counter64 is correctly formatted",
			variable:    gosnmp.SnmpPDU{Name: "1.3.6.1.2.1.2.2.1.15", Type: gosnmp.Counter64, Value: 34},
			expected:    "counter64",
		},
		{
			description: "type gosnmp.Null is correctly formatted",
			variable:    gosnmp.SnmpPDU{Name: "1.3.6.1.2.1.2.2.1.15", Type: gosnmp.Null, Value: "whatever"},
			expected:    "null",
		},
		{
			description: "type gosnmp.UnknownType is correctly formatted",
			variable:    gosnmp.SnmpPDU{Name: "1.3.6.1.2.1.2.2.1.19", Type: gosnmp.UnknownType, Value: "whatever"},
			expected:    "unknown-type",
		},
		{
			description: "type gosnmp.ObjectDescription is correctly formatted",
			variable:    gosnmp.SnmpPDU{Name: "1.3.6.1.2.1.2.2.1.8", Type: gosnmp.ObjectDescription, Value: "whatever"},
			expected:    "object-description",
		},
		{
			description: "type gosnmp.NsapAddress is correctly formatted",
			variable:    gosnmp.SnmpPDU{Name: "1.3.6.1.2.1.2.2.1.15", Type: gosnmp.NsapAddress, Value: []byte{0x74, 0x65, 0x73, 0x74}},
			expected:    "nsap-address",
		},
		{
			description: "type gosnmp.NoSuchObject is correctly formatted",
			variable:    gosnmp.SnmpPDU{Name: "1.3.6.1.2.1.2.2.1.15", Type: gosnmp.NoSuchObject, Value: "whatever"},
			expected:    "no-such-object",
		},
		{
			description: "type gosnmp.NoSuchInstance is correctly formatted",
			variable:    gosnmp.SnmpPDU{Name: "1.3.6.1.2.1.2.2.1.15", Type: gosnmp.NoSuchInstance, Value: "whatever"},
			expected:    "no-such-instance",
		},
		{
			description: "type gosnmp.EndOfMibView is correctly formatted",
			variable:    gosnmp.SnmpPDU{Name: "1.3.6.1.2.1.2.2.1.15", Type: gosnmp.EndOfMibView, Value: "whatever"},
			expected:    "end-of-mib-view",
		},
	}

	for _, d := range data {
		require.Equal(t, d.expected, formatType(d.variable), d.description)
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

func TestFormatterTelemetry(t *testing.T) {
	data := []struct {
		description    string
		packet         *packet.SnmpPacket
		expectedMetric string
		expectedValue  float64
		expectedTags   []string
	}{
		{
			description:    "Fail to enrich V1 trap",
			packet:         packet.CreateTestV1Packet(packet.Unknownv1Trap),
			expectedMetric: "datadog.snmp_traps.traps_not_enriched",
			expectedValue:  1,
			expectedTags: []string{
				"snmp_version:1",
				"device_namespace:jiji",
				"snmp_device:127.0.0.1",
			},
		},
		{
			description:    "Fail to enrich V1 trap variables",
			packet:         packet.CreateTestV1Packet(packet.Unknownv1Trap),
			expectedMetric: "datadog.snmp_traps.vars_not_enriched",
			expectedValue:  3,
			expectedTags: []string{
				"snmp_version:1",
				"device_namespace:jiji",
				"snmp_device:127.0.0.1",
			},
		},
		{
			description:    "Not enough variables",
			packet:         packet.CreateTestPacket(NotEnoughVarsExampleV2Trap),
			expectedMetric: "datadog.snmp_traps.incorrect_format",
			expectedValue:  1,
			expectedTags: []string{
				"snmp_version:2",
				"device_namespace:totoro",
				"snmp_device:127.0.0.1",
				"error:invalid_variables",
			},
		},
		{
			description:    "Missing SysUpTime",
			packet:         packet.CreateTestPacket(MissingSysUpTimeInstanceExampleV2Trap),
			expectedMetric: "datadog.snmp_traps.incorrect_format",
			expectedValue:  1,
			expectedTags: []string{
				"snmp_version:2",
				"device_namespace:totoro",
				"snmp_device:127.0.0.1",
				"error:invalid_sys_uptime",
			},
		},
		{
			description:    "Missing Trap OID",
			packet:         packet.CreateTestPacket(MissingTrapOIDExampleV2Trap),
			expectedMetric: "datadog.snmp_traps.incorrect_format",
			expectedValue:  1,
			expectedTags: []string{
				"snmp_version:2",
				"device_namespace:totoro",
				"snmp_device:127.0.0.1",
				"error:invalid_trap_oid",
			},
		},
		{
			description:    "Fail to enrich V2 trap",
			packet:         packet.CreateTestPacket(UnknownExampleV2Trap),
			expectedMetric: "datadog.snmp_traps.traps_not_enriched",
			expectedValue:  1,
			expectedTags: []string{
				"snmp_version:2",
				"device_namespace:totoro",
				"snmp_device:127.0.0.1",
			},
		},
		{
			description:    "Fail to enrich V2 trap variables",
			packet:         packet.CreateTestPacket(UnknownExampleV2Trap),
			expectedMetric: "datadog.snmp_traps.vars_not_enriched",
			expectedValue:  2,
			expectedTags: []string{
				"snmp_version:2",
				"device_namespace:totoro",
				"snmp_device:127.0.0.1",
			},
		},
	}

	var mockSender sender.MockComponent
	formatter := fxutil.Test[Component](t,
		testOptions,
		fx.Populate(&mockSender),
	)

	for _, d := range data {
		t.Run(d.description, func(t *testing.T) {
			_, _ = formatter.FormatPacket(d.packet)

			mockSender.AssertMetric(t, "Count", d.expectedMetric, d.expectedValue, "", d.expectedTags)
		})
	}
}
