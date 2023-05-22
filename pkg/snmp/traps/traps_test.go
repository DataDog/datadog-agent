// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build !serverless

package traps

import (
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// List of variables for a NetSNMP::ExampleHeartBeatNotification trap message.
// See: http://www.circitor.fr/Mibs/Html/N/NET-SNMP-EXAMPLES-MIB.php#netSnmpExampleHeartbeatNotification
var (
	NetSNMPExampleHeartbeatNotification = gosnmp.SnmpTrap{
		Variables: []gosnmp.SnmpPDU{
			// sysUpTimeInstance
			{Name: "1.3.6.1.2.1.1.3.0", Type: gosnmp.TimeTicks, Value: uint32(1000)},
			// snmpTrapOID
			{Name: "1.3.6.1.6.3.1.1.4.1.0", Type: gosnmp.OctetString, Value: "1.3.6.1.4.1.8072.2.3.0.1"},
			// heartBeatRate
			{Name: "1.3.6.1.4.1.8072.2.3.2.1", Type: gosnmp.Integer, Value: 1024},
			// heartBeatName
			{Name: "1.3.6.1.4.1.8072.2.3.2.2", Type: gosnmp.OctetString, Value: "test"},
		},
	}
	LinkDownv1GenericTrap = gosnmp.SnmpTrap{
		AgentAddress: "127.0.0.1",
		Enterprise:   ".1.3.6.1.6.3.1.1.5",
		GenericTrap:  2,
		SpecificTrap: 0,
		Timestamp:    1000,
		Variables: []gosnmp.SnmpPDU{
			// ifIndex
			{Name: ".1.3.6.1.2.1.2.2.1.1", Type: gosnmp.Integer, Value: 2},
			// ifAdminStatus
			{Name: ".1.3.6.1.2.1.2.2.1.7", Type: gosnmp.Integer, Value: 1},
			// ifOperStatus
			{Name: ".1.3.6.1.2.1.2.2.1.8", Type: gosnmp.Integer, Value: 2},
			// myFakeVarType 0, 1, 2, 3, 12, 13, 14, 15, 95, and 130 are set
			{Name: ".1.3.6.1.2.1.200.1.3.1.5", Type: gosnmp.OctetString, Value: []uint8{0xf0, 0x0f, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x01, 0, 0, 0, 0, 0x20}},
		},
	}
	AlarmActiveStatev1SpecificTrap = gosnmp.SnmpTrap{
		AgentAddress: "127.0.0.1",
		Enterprise:   ".1.3.6.1.2.1.118",
		GenericTrap:  6,
		SpecificTrap: 2,
		Timestamp:    1000,
		Variables: []gosnmp.SnmpPDU{
			// alarmActiveModelPointer
			{Name: ".1.3.6.1.2.1.118.1.2.2.1.13", Type: gosnmp.OctetString, Value: []uint8{0x66, 0x6f, 0x6f}},
			// alarmActiveResourceId
			{Name: ".1.3.6.1.2.1.118.1.2.2.1.10", Type: gosnmp.OctetString, Value: []uint8{0x62, 0x61, 0x72}},
			// myFakeVarType 0, 1, 2, 3, 12, 13, 14, 15, 95, and 130 are set
			{Name: ".1.3.6.1.2.1.200.1.3.1.5", Type: gosnmp.OctetString, Value: []uint8{0xf0, 0x0f, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x01, 0, 0, 0, 0, 0x20}},
		},
	}
)

func getFreePort() uint16 {
	var port uint16
	for i := 0; i < 5; i++ {
		conn, err := net.ListenPacket("udp", ":0")
		if err != nil {
			continue
		}
		conn.Close()
		port, err = parsePort(conn.LocalAddr().String())
		if err != nil {
			continue
		}
		listener, err := startSNMPTrapListener(Config{Port: port}, nil)
		if err != nil {
			continue
		}
		listener.Stop()
		return port
	}
	panic("unable to find free port for starting the trap listener")
}

func parsePort(addr string) (uint16, error) {
	_, portString, err := net.SplitHostPort(addr)
	if err != nil {
		return 0, err
	}

	port, err := strconv.ParseUint(portString, 10, 16)
	if err != nil {
		return 0, err
	}
	return uint16(port), nil
}

// Configure sets Datadog Agent configuration from a config object.
func Configure(t *testing.T, trapConfig Config) {
	ConfigureWithGlobalNamespace(t, trapConfig, "")
}

// ConfigureWithGlobalNamespace sets Datadog Agent configuration from a config object and a namespace
func ConfigureWithGlobalNamespace(t *testing.T, trapConfig Config, globalNamespace string) {
	trapConfig.Enabled = true
	datadogYaml := map[string]map[string]interface{}{
		"network_devices": {
			"snmp_traps": trapConfig,
		},
	}
	if globalNamespace != "" {
		datadogYaml["network_devices"]["namespace"] = globalNamespace
	}

	config.Datadog.SetConfigType("yaml")
	out, err := yaml.Marshal(datadogYaml)
	require.NoError(t, err)

	err = config.Datadog.ReadConfig(strings.NewReader(string(out)))
	require.NoError(t, err)
}

func sendTestV1GenericTrap(t *testing.T, trapConfig Config, community string) *gosnmp.GoSNMP {
	params, err := trapConfig.BuildSNMPParams()
	require.NoError(t, err)
	params.Community = community
	params.Timeout = 1 * time.Second // Must be non-zero when sending traps.
	params.Retries = 1               // Must be non-zero when sending traps.
	params.Version = gosnmp.Version1

	err = params.Connect()
	require.NoError(t, err)
	defer params.Conn.Close()

	_, err = params.SendTrap(LinkDownv1GenericTrap)
	require.NoError(t, err)

	return params
}

func sendTestV1SpecificTrap(t *testing.T, trapConfig Config, community string) *gosnmp.GoSNMP {
	params, err := trapConfig.BuildSNMPParams()
	require.NoError(t, err)
	params.Community = community
	params.Timeout = 1 * time.Second // Must be non-zero when sending traps.
	params.Retries = 1               // Must be non-zero when sending traps.
	params.Version = gosnmp.Version1

	err = params.Connect()
	require.NoError(t, err)
	defer params.Conn.Close()

	_, err = params.SendTrap(AlarmActiveStatev1SpecificTrap)
	require.NoError(t, err)

	return params
}

func sendTestV2Trap(t *testing.T, trapConfig Config, community string) *gosnmp.GoSNMP {
	params, err := trapConfig.BuildSNMPParams()
	require.NoError(t, err)
	params.Community = community
	params.Timeout = 1 * time.Second // Must be non-zero when sending traps.
	params.Retries = 1               // Must be non-zero when sending traps.

	err = params.Connect()
	require.NoError(t, err)
	defer params.Conn.Close()

	trap := NetSNMPExampleHeartbeatNotification
	_, err = params.SendTrap(trap)
	require.NoError(t, err)

	return params
}

func sendTestV3Trap(t *testing.T, trapConfig Config, securityParams *gosnmp.UsmSecurityParameters) *gosnmp.GoSNMP {
	params, err := trapConfig.BuildSNMPParams()
	require.NoError(t, err)
	params.MsgFlags = gosnmp.AuthPriv
	params.SecurityParameters = securityParams
	params.Timeout = 1 * time.Second // Must be non-zero when sending traps.
	params.Retries = 1               // Must be non-zero when sending traps.

	err = params.Connect()
	require.NoError(t, err)
	defer params.Conn.Close()

	trap := NetSNMPExampleHeartbeatNotification
	_, err = params.SendTrap(trap)
	require.NoError(t, err)

	return params
}

func assertIsValidV2Packet(t *testing.T, packet *SnmpPacket, trapConfig Config) {
	require.Equal(t, gosnmp.Version2c, packet.Content.Version)
	communityValid := false
	for _, community := range trapConfig.CommunityStrings {
		if packet.Content.Community == community {
			communityValid = true
		}
	}
	require.True(t, communityValid)
}

func assertVariables(t *testing.T, packet *SnmpPacket) {
	variables := packet.Content.Variables
	assert.Equal(t, 4, len(variables))

	sysUptimeInstance := variables[0]
	assert.Equal(t, ".1.3.6.1.2.1.1.3.0", sysUptimeInstance.Name)
	assert.Equal(t, gosnmp.TimeTicks, sysUptimeInstance.Type)

	snmptrapOID := variables[1]
	assert.Equal(t, ".1.3.6.1.6.3.1.1.4.1.0", snmptrapOID.Name)
	assert.Equal(t, gosnmp.OctetString, snmptrapOID.Type)
	assert.Equal(t, "1.3.6.1.4.1.8072.2.3.0.1", string(snmptrapOID.Value.([]byte)))

	heartBeatRate := variables[2]
	assert.Equal(t, ".1.3.6.1.4.1.8072.2.3.2.1", heartBeatRate.Name)
	assert.Equal(t, gosnmp.Integer, heartBeatRate.Type)
	assert.Equal(t, 1024, heartBeatRate.Value.(int))

	heartBeatName := variables[3]
	assert.Equal(t, ".1.3.6.1.4.1.8072.2.3.2.2", heartBeatName.Name)
	assert.Equal(t, gosnmp.OctetString, heartBeatName.Type)
	assert.Equal(t, "test", string(heartBeatName.Value.([]byte)))
}
