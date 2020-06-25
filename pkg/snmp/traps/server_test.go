// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020 Datadog, Inc.

package traps

import (
	"math/rand"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/soniah/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/* Test traps sending/reception helpers */

// http://www.circitor.fr/Mibs/Html/N/NET-SNMP-EXAMPLES-MIB.php#netSnmpExampleHeartbeatNotification
var netSnmpExampleHeartbeatNotification = []gosnmp.SnmpPDU{
	// snmpTrapOID
	{Name: "1.3.6.1.6.3.1.1.4.1", Type: gosnmp.OctetString, Value: "1.3.6.1.4.1.8072.2.3.0.1"},
	// heartBeatRate
	{Name: "1.3.6.1.4.1.8072.2.3.2.1", Type: gosnmp.Integer, Value: 1024},
	// heartBeatName
	{Name: "1.3.6.1.4.1.8072.2.3.2.2", Type: gosnmp.OctetString, Value: "test"},
}

func sendTestV2Trap(t *testing.T, c TrapListenerConfig, community string) {
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
	for _, community := range config.Community {
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

/* Test helpers */

// Builder is a testing utility for managing listener integration test setups.
type Builder struct {
	t       *testing.T
	configs []TrapListenerConfig
}

// NewBuilder return a new builder instance.
func NewBuilder(t *testing.T) *Builder {
	return &Builder{t: t}
}

// GetPort requests a random UDP port number and makes sure it is available
func (b *Builder) GetPort() uint16 {
	conn, err := net.ListenPacket("udp", ":0")
	require.NoError(b.t, err)
	defer conn.Close()

	_, portString, err := net.SplitHostPort(conn.LocalAddr().String())
	require.NoError(b.t, err)

	port, err := strconv.Atoi(portString)
	require.NoError(b.t, err)

	return uint16(port)
}

func (b *Builder) Add(config TrapListenerConfig) TrapListenerConfig {
	if config.Port == 0 {
		config.Port = b.GetPort()
	}
	b.configs = append(b.configs, config)
	return config
}

func (b *Builder) Configure() {
	out, err := yaml.Marshal(map[string]interface{}{"snmp_traps_listeners": b.configs})
	require.NoError(b.t, err)
	config.Datadog.SetConfigType("yaml")
	err = config.Datadog.ReadConfig(strings.NewReader(string(out)))
	require.NoError(b.t, err)
}

// StartServer starts a trap server and makes sure it is running and has the expected number of running listeners.
func (b *Builder) StartServer() *TrapServer {
	s, err := NewTrapServer()
	require.NoError(b.t, err)
	require.NotNil(b.t, s)
	require.True(b.t, s.Started)
	require.Equal(b.t, s.NumListeners(), len(b.configs))
	return s
}

/* Tests */

func TestServerEmpty(t *testing.T) {
	b := NewBuilder(t)
	s := b.StartServer()
	s.Stop()
}

func TestServerV2(t *testing.T) {
	b := NewBuilder(t)
	config := b.Add(TrapListenerConfig{Community: []string{"public"}})
	b.Configure()

	s := b.StartServer()
	defer s.Stop()

	sendTestV2Trap(t, config, "public")
	p := receivePacket(t, s)
	require.NotNil(t, p)
	assertV2(t, p, config)
	assertV2Variables(t, p)
}

func TestServerV2BadCredentials(t *testing.T) {
	b := NewBuilder(t)
	config := b.Add(TrapListenerConfig{Community: []string{"public"}})
	b.Configure()

	s := b.StartServer()
	defer s.Stop()

	sendTestV2Trap(t, config, "wrong")
	assertNoPacketReceived(t, s)
}

func TestConcurrency(t *testing.T) {
	b := NewBuilder(t)
	configs := []TrapListenerConfig{
		b.Add(TrapListenerConfig{Community: []string{"public0"}}),
		b.Add(TrapListenerConfig{Community: []string{"public1"}}),
		b.Add(TrapListenerConfig{Community: []string{"public2"}}),
	}
	b.Configure()

	s := b.StartServer()
	defer s.Stop()

	numMessagesPerListener := 100
	totalMessages := numMessagesPerListener * len(configs)

	wg := sync.WaitGroup{}
	wg.Add(len(configs) + 1)

	for _, config := range configs {
		c := config
		go func() {
			defer wg.Done()
			for i := 0; i < numMessagesPerListener; i++ {
				time.Sleep(time.Duration(rand.Float64()) * time.Microsecond) // Prevent serial execution.
				sendTestV2Trap(t, c, c.Community[0])
			}
		}()
	}

	go func() {
		defer wg.Done()
		for i := 0; i < totalMessages; i++ {
			p := receivePacket(t, s)
			require.NotNil(t, p)
			assertV2Variables(t, p)
		}
	}()

	wg.Wait()
}

func TestPortConflict(t *testing.T) {
	b := NewBuilder(t)
	port := b.GetPort()

	// Triggers an "address already in use" error for one of the listeners.
	b.Add(TrapListenerConfig{Port: port, Community: []string{"public0"}})
	b.Add(TrapListenerConfig{Port: port, Community: []string{"public1"}})
	b.Configure()

	s, err := NewTrapServer()
	require.Error(t, err)
	assert.Nil(t, s)
}
