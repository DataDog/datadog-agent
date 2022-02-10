// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build !serverless
// +build !serverless

package traps

import (
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
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
			// ifOperStatusjq
			{Name: ".1.3.6.1.2.1.2.2.1.8", Type: gosnmp.Integer, Value: 2},
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
		},
	}
)

func parsePort(t *testing.T, addr string) uint16 {
	_, portString, err := net.SplitHostPort(addr)
	require.NoError(t, err)

	port, err := strconv.ParseUint(portString, 10, 16)
	require.NoError(t, err)

	return uint16(port)
}

// GetPort requests a random UDP port number and makes sure it is available
func GetPort(t *testing.T) uint16 {
	conn, err := net.ListenPacket("udp", ":0")
	require.NoError(t, err)
	defer conn.Close()
	return parsePort(t, conn.LocalAddr().String())
}

// Configure sets Datadog Agent configuration from a config object.
func Configure(t *testing.T, trapConfig Config) {
	datadogYaml := map[string]interface{}{
		"snmp_traps_enabled": true,
		"snmp_traps_config":  trapConfig,
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

// receivePacket waits for a received trap packet and returns it.
func receivePacket(t *testing.T) *SnmpPacket {
	select {
	case packet := <-GetPacketsChannel():
		return packet
	case <-time.After(3 * time.Second):
		t.Error("Trap not received")
		return nil
	}
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

func assertNoPacketReceived(t *testing.T) {
	select {
	case <-GetPacketsChannel():
		t.Error("Unexpectedly received an unauthorized packet")
	case <-time.After(100 * time.Millisecond):
		break
	}
}
