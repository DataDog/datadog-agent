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

// http://www.circitor.fr/Mibs/Html/N/NET-SNMP-EXAMPLES-MIB.php#netSnmpExampleHeartbeatNotification
var netSnmpExampleHeartbeatNotification = []gosnmp.SnmpPDU{
	// snmpTrapOID
	{Name: "1.3.6.1.6.3.1.1.4.1", Type: gosnmp.OctetString, Value: "1.3.6.1.4.1.8072.2.3.0.1"},
	// heartBeatRate
	{Name: "1.3.6.1.4.1.8072.2.3.2.1", Type: gosnmp.Integer, Value: 1024},
	// heartBeatName
	{Name: "1.3.6.1.4.1.8072.2.3.2.2", Type: gosnmp.OctetString, Value: "test"},
}

func sendTestV2Trap(t *testing.T, c TrapListenerConfig, community string) *gosnmp.GoSNMP {
	params, err := c.BuildParams()
	require.NoError(t, err)
	params.Community = community
	params.Timeout = 1 * time.Second // Must be non-zero
	params.Retries = 1               // Must be non-zero

	if sp, ok := params.SecurityParameters.(*gosnmp.UsmSecurityParameters); ok {
		// The GoSNMP trap listener does not support responding to security parameters discovery requests,
		// so we need to set these options explicitly (otherwise the discovery request is sent and it times out).
		sp.AuthoritativeEngineID = "test"
		sp.AuthoritativeEngineBoots = 1
		sp.AuthoritativeEngineTime = 0
	}

	err = params.Connect()
	require.NoError(t, err)
	defer params.Conn.Close()

	trap := gosnmp.SnmpTrap{Variables: netSnmpExampleHeartbeatNotification}
	_, err = params.SendTrap(trap)
	require.NoError(t, err)

	return params
}

// receivePacket waits for a received trap packet and returns it. May not be the same than one that has just been sent.
func receivePacket(t *testing.T, s *TrapServer) *SnmpPacket {
	select {
	case p := <-s.Output():
		return p
	case <-time.After(3 * time.Second):
		t.Errorf("Trap not received")
		return nil
	}
}

/* Assertion helpers */

func assertV2(t *testing.T, p *SnmpPacket, config TrapListenerConfig) {
	require.Equal(t, gosnmp.Version2c, p.Content.Version)
	communityValid := false
	for _, community := range config.CommunityStrings {
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

func assertNoPacketReceived(t *testing.T, s *TrapServer) {
	select {
	case <-s.Output():
		t.Errorf("Unexpectedly received an unauthorized packet")
	case <-time.After(100 * time.Millisecond):
		break
	}
}

func parsePort(t *testing.T, addr string) uint16 {
	_, portString, err := net.SplitHostPort(addr)
	require.NoError(t, err)

	port, err := strconv.Atoi(portString)
	require.NoError(t, err)

	return uint16(port)
}

/* Test helpers */

// Builder is a testing utility for managing listener integration test setups.
type builder struct {
	t       *testing.T
	configs []TrapListenerConfig
}

// NewBuilder return a new builder instance.
func newBuilder(t *testing.T) *builder {
	return &builder{t: t}
}

// GetPort requests a random UDP port number and makes sure it is available
func (b *builder) GetPort() uint16 {
	conn, err := net.ListenPacket("udp", ":0")
	require.NoError(b.t, err)
	defer conn.Close()
	return parsePort(b.t, conn.LocalAddr().String())
}

func (b *builder) Add(config TrapListenerConfig) TrapListenerConfig {
	if config.Port == 0 {
		config.Port = b.GetPort()
	}
	b.configs = append(b.configs, config)
	return config
}

func (b *builder) Configure() {
	out, err := yaml.Marshal(map[string]interface{}{"snmp_traps_listeners": b.configs})
	require.NoError(b.t, err)
	config.Datadog.SetConfigType("yaml")
	err = config.Datadog.ReadConfig(strings.NewReader(string(out)))
	require.NoError(b.t, err)
}

// StartServer starts a trap server and makes sure it is running and has the expected number of running listeners.
func (b *builder) StartServer() *TrapServer {
	s, err := NewTrapServer()
	require.NoError(b.t, err)
	require.NotNil(b.t, s)
	require.True(b.t, s.Started)
	require.Equal(b.t, len(b.configs), s.NumListeners())
	require.Equal(b.t, 0, s.NumFailedListeners())
	return s
}
