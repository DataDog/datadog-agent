// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020 Datadog, Inc.

package traps

import (
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/soniah/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getAvailableUDPPort requests a random port number and makes sure it is available
func getAvailableUDPPort() (uint16, error) {
	conn, err := net.ListenPacket("udp", ":0")
	if err != nil {
		return 0, fmt.Errorf("can't find an available udp port: %s", err)
	}
	defer conn.Close()

	_, portString, err := net.SplitHostPort(conn.LocalAddr().String())
	if err != nil {
		return 0, fmt.Errorf("can't find an available udp port: %s", err)
	}
	port, err := strconv.Atoi(portString)
	if err != nil {
		return 0, fmt.Errorf("can't convert udp port: %s", err)
	}

	return uint16(port), nil
}

func configure(t *testing.T, yaml string) {
	config.Datadog.SetConfigType("yaml")
	err := config.Datadog.ReadConfig(strings.NewReader(yaml))
	require.NoError(t, err)
}

// http://www.circitor.fr/Mibs/Html/N/NET-SNMP-EXAMPLES-MIB.php#netSnmpExampleHeartbeatNotification
var netSnmpExampleHeartbeatNotification = []gosnmp.SnmpPDU{
	// snmpTrapOID
	{Name: "1.3.6.1.6.3.1.1.4.1", Type: gosnmp.OctetString, Value: "1.3.6.1.4.1.8072.2.3.0.1"},
	// heartBeatRate
	{Name: "1.3.6.1.4.1.8072.2.3.2.1", Type: gosnmp.Integer, Value: 1024},
	// heartBeatName
	{Name: "1.3.6.1.4.1.8072.2.3.2.2", Type: gosnmp.OctetString, Value: "test"},
}

func sendTestTrap(t *testing.T, c TrapListenerConfig) {
	params, err := c.BuildParams()
	require.NoError(t, err)
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

	_, err = params.SendTrap(gosnmp.SnmpTrap{Variables: netSnmpExampleHeartbeatNotification})
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

func assertV2c(t *testing.T, p *SnmpPacket, config TrapListenerConfig) {
	require.Equal(t, gosnmp.Version2c, p.Content.Version)
	require.Equal(t, config.Community, p.Content.Community)
}

func assertV3(t *testing.T, p *SnmpPacket, config TrapListenerConfig) {
	require.Equal(t, gosnmp.Version3, p.Content.Version)

	require.NotNil(t, p.Content.SecurityParameters)
	sp := p.Content.SecurityParameters.(*gosnmp.UsmSecurityParameters)

	if config.AuthProtocol != "" {
		authProtocol, err := BuildAuthProtocol(config.AuthProtocol)
		require.NoError(t, err)
		require.Equal(t, authProtocol, sp.AuthenticationProtocol)
	}

	if config.PrivProtocol != "" {
		privProtocol, err := BuildPrivProtocol(config.PrivProtocol)
		require.NoError(t, err)
		require.Equal(t, privProtocol, sp.PrivacyProtocol)
	}
}

func assertVariables(t *testing.T, p *SnmpPacket) {
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

func TestServerEmpty(t *testing.T) {
	s, err := NewTrapServer()
	require.NoError(t, err)
	assert.NotNil(t, s)
	defer s.Stop()
	assert.True(t, s.Started)
}

func TestServerV2(t *testing.T) {
	port, err := getAvailableUDPPort()
	require.NoError(t, err)

	config := TrapListenerConfig{
		Port:      port,
		Community: "public",
	}

	configure(t, fmt.Sprintf(`
snmp_traps_listeners:
  - port: %d
    community: %s
`, config.Port, config.Community))

	s, err := NewTrapServer()
	require.NoError(t, err)
	require.NotNil(t, s)
	defer s.Stop()
	require.True(t, s.Started)
	require.Equal(t, s.NumListeners(), 1)

	sendTestTrap(t, config)
	p := receivePacket(t, s)
	assertV2c(t, p, config)
	assertVariables(t, p)
}

func TestServerV3(t *testing.T) {
	t.Run("no-auth-no-priv", func(t *testing.T) {
		port, err := getAvailableUDPPort()
		require.NoError(t, err)

		config := TrapListenerConfig{
			Port: port,
			User: "doggo",
		}

		configure(t, fmt.Sprintf(`
snmp_traps_listeners:
  - port: %d
    user: %s
`, config.Port, config.User))

		s, err := NewTrapServer()
		require.NoError(t, err)
		require.NotNil(t, s)
		defer s.Stop()
		require.True(t, s.Started)
		require.Equal(t, s.NumListeners(), 1)

		sendTestTrap(t, config)
		p := receivePacket(t, s)
		assertV3(t, p, config)
		assertVariables(t, p)
	})

	t.Run("auth-no-priv", func(t *testing.T) {
		port, err := getAvailableUDPPort()
		require.NoError(t, err)

		config := TrapListenerConfig{
			Port:         port,
			User:         "doggo",
			AuthProtocol: "MD5",
			AuthKey:      "doggopass",
		}

		configure(t, fmt.Sprintf(`
snmp_traps_listeners:
  - port: %d
    user: %s
    auth_protocol: %s
    auth_key: %s
`, config.Port, config.User, config.AuthProtocol, config.AuthKey))

		s, err := NewTrapServer()
		require.NoError(t, err)
		require.NotNil(t, s)
		defer s.Stop()
		require.True(t, s.Started)
		require.Equal(t, s.NumListeners(), 1)

		sendTestTrap(t, config)
		p := receivePacket(t, s)
		assertV3(t, p, config)
		assertVariables(t, p)
	})

	t.Run("auth-priv", func(t *testing.T) {
		port, err := getAvailableUDPPort()
		require.NoError(t, err)

		config := TrapListenerConfig{
			Port:         port,
			User:         "doggo",
			AuthProtocol: "MD5",
			AuthKey:      "doggopass",
			PrivProtocol: "DES",
			PrivKey:      "doggokey",
		}

		configure(t, fmt.Sprintf(`
snmp_traps_listeners:
  - port: %d
    user: %s
    auth_protocol: %s
    auth_key: %s
    priv_protocol: %s
    priv_key: %s
`, config.Port, config.User, config.AuthProtocol, config.AuthKey, config.PrivProtocol, config.PrivKey))

		s, err := NewTrapServer()
		require.NoError(t, err)
		require.NotNil(t, s)
		defer s.Stop()
		require.True(t, s.Started)
		require.Equal(t, s.NumListeners(), 1)

		sendTestTrap(t, config)
		p := receivePacket(t, s)
		assertV3(t, p, config)
		assertVariables(t, p)
	})
}

func TestConcurrency(t *testing.T) {
	port0, err := getAvailableUDPPort()
	require.NoError(t, err)
	port1, err := getAvailableUDPPort()
	require.NoError(t, err)
	port2, err := getAvailableUDPPort()
	require.NoError(t, err)
	port3, err := getAvailableUDPPort()
	require.NoError(t, err)
	port4, err := getAvailableUDPPort()
	require.NoError(t, err)

	configs := []TrapListenerConfig{
		{Port: port0, Community: "public0"},
		{Port: port1, Community: "public1"},
		{Port: port2, User: "doggo"},
		{Port: port3, User: "bits", AuthProtocol: "MD5", AuthKey: "bitspass"},
		{Port: port4, User: "buddy", AuthProtocol: "SHA", AuthKey: "buddypass", PrivProtocol: "AES", PrivKey: "buddykey"},
	}

	configure(t, fmt.Sprintf(`
snmp_traps_listeners:
  - port: %d
    community: %s
  - port: %d
    community: %s
  - port: %d
    user: %s
  - port: %d
    user: %s
    auth_protocol: %s
    auth_key: %s
  - port: %d
    user: %s
    auth_protocol: %s
    auth_key: %s
    priv_protocol: %s
    priv_key: %s
`, configs[0].Port, configs[0].Community, configs[1].Port, configs[1].Community, configs[2].Port, configs[2].User, configs[3].Port, configs[3].User, configs[3].AuthProtocol, configs[3].AuthKey, configs[4].Port, configs[4].User, configs[4].AuthProtocol, configs[4].AuthKey, configs[4].PrivProtocol, configs[4].PrivKey))

	s, err := NewTrapServer()
	require.NoError(t, err)
	require.NotNil(t, s)
	defer s.Stop()
	require.True(t, s.Started)
	require.Equal(t, s.NumListeners(), 5)

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
				sendTestTrap(t, c)
			}
		}()
	}

	go func() {
		defer wg.Done()
		for i := 0; i < totalMessages; i++ {
			p := receivePacket(t, s)
			assertVariables(t, p)
		}
	}()

	wg.Wait()
}

func TestServerErrors(t *testing.T) {
	t.Run("listener-error", func(t *testing.T) {
		port, err := getAvailableUDPPort()
		require.NoError(t, err)

		// Use the same port to trigger an "address already in use" error for one of the listeners.
		configure(t, fmt.Sprintf(`
snmp_traps_listeners:
  - port: %d
    community: public0
  - port: %d
    community: public1
`, port, port))

		s, err := NewTrapServer()
		require.Error(t, err)
		assert.Nil(t, s)
	})

	// TODO test v2 with wrong community string
	// TODO test v3 with wrong user
	// TODO test v3 with wrong auth
	// TODO test v3 with wrong priv
}
