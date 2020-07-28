// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020 Datadog, Inc.

package traps

import (
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/soniah/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func parsePort(t *testing.T, addr string) uint16 {
	_, portString, err := net.SplitHostPort(addr)
	require.NoError(t, err)

	port, err := strconv.Atoi(portString)
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

func configure(t *testing.T, c Config) {
	config.Datadog.SetConfigType("yaml")
	out, err := yaml.Marshal(map[string]interface{}{"snmp_traps_enabled": true, "snmp_traps_config": c})
	require.NoError(t, err)
	err = config.Datadog.ReadConfig(strings.NewReader(string(out)))
	require.NoError(t, err)
}

// List of variables for a NetSNMP::ExampleHeartBeatNotification trap message.
// See: http://www.circitor.fr/Mibs/Html/N/NET-SNMP-EXAMPLES-MIB.php#netSnmpExampleHeartbeatNotification
var netSnmpExampleHeartbeatNotificationVariables = []gosnmp.SnmpPDU{
	// snmpTrapOID
	{Name: "1.3.6.1.6.3.1.1.4.1", Type: gosnmp.OctetString, Value: "1.3.6.1.4.1.8072.2.3.0.1"},
	// heartBeatRate
	{Name: "1.3.6.1.4.1.8072.2.3.2.1", Type: gosnmp.Integer, Value: 1024},
	// heartBeatName
	{Name: "1.3.6.1.4.1.8072.2.3.2.2", Type: gosnmp.OctetString, Value: "test"},
}

func sendTestV2Trap(t *testing.T, c Config, community string) *gosnmp.GoSNMP {
	params := c.BuildV2Params()
	params.Community = community
	params.Timeout = 1 * time.Second // Must be non-zero when sending traps.
	params.Retries = 1               // Must be non-zero when sending traps.

	err := params.Connect()
	require.NoError(t, err)
	defer params.Conn.Close()

	trap := gosnmp.SnmpTrap{Variables: netSnmpExampleHeartbeatNotificationVariables}
	_, err = params.SendTrap(trap)
	require.NoError(t, err)

	return params
}

// receivePacket waits for a received trap packet and returns it.
func receivePacket(t *testing.T) *SnmpPacket {
	select {
	case p := <-GetPacketsChannel():
		return p
	case <-time.After(3 * time.Second):
		t.Errorf("Trap not received")
		return nil
	}
}

func assertIsValidV2Packet(t *testing.T, p *SnmpPacket, c Config) {
	require.Equal(t, gosnmp.Version2c, p.Content.Version)
	communityValid := false
	for _, community := range c.CommunityStrings {
		if p.Content.Community == community {
			communityValid = true
		}
	}
	require.True(t, communityValid)
}

func assertV2Variables(t *testing.T, p *SnmpPacket) {
	vars := p.Content.Variables
	assert.Equal(t, 4, len(vars))

	uptime := vars[0]
	assert.Equal(t, ".1.3.6.1.2.1.1.3.0", uptime.Name)
	assert.Equal(t, gosnmp.TimeTicks, uptime.Type)

	snmptrapOID := vars[1]
	assert.Equal(t, ".1.3.6.1.6.3.1.1.4.1", snmptrapOID.Name)
	assert.Equal(t, gosnmp.OctetString, snmptrapOID.Type)
	assert.Equal(t, "1.3.6.1.4.1.8072.2.3.0.1", string(snmptrapOID.Value.([]byte)))

	heartBeatRate := vars[2]
	assert.Equal(t, ".1.3.6.1.4.1.8072.2.3.2.1", heartBeatRate.Name)
	assert.Equal(t, gosnmp.Integer, heartBeatRate.Type)
	assert.Equal(t, 1024, heartBeatRate.Value.(int))

	heartBeatName := vars[3]
	assert.Equal(t, ".1.3.6.1.4.1.8072.2.3.2.2", heartBeatName.Name)
	assert.Equal(t, gosnmp.OctetString, heartBeatName.Type)
	assert.Equal(t, "test", string(heartBeatName.Value.([]byte)))
}

func assertNoPacketReceived(t *testing.T) {
	select {
	case <-GetPacketsChannel():
		t.Errorf("Unexpectedly received an unauthorized packet")
	case <-time.After(100 * time.Millisecond):
		break
	}
}
