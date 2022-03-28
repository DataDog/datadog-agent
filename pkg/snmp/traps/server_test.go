// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package traps

import (
	"net"
	"strconv"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var serverPort = getFreePort()

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
		listener, err := startSNMPTrapListener(&Config{Port: port}, nil)
		if err != nil {
			continue
		}
		listener.Close()
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

func TestServerV1GenericTrap(t *testing.T) {
	config := Config{Port: serverPort, CommunityStrings: []string{"public"}}
	Configure(t, config)

	err := StartServer("dummy_hostname")
	require.NoError(t, err)
	defer StopServer()

	sendTestV1GenericTrap(t, config, "public")
	packet := receivePacket(t)
	require.NotNil(t, packet)
	packet.Content.SnmpTrap.Variables = packet.Content.Variables
	assert.Equal(t, LinkDownv1GenericTrap, packet.Content.SnmpTrap)

}

func TestServerV1SpecificTrap(t *testing.T) {
	config := Config{Port: serverPort, CommunityStrings: []string{"public"}}
	Configure(t, config)

	err := StartServer("dummy_hostname")
	require.NoError(t, err)
	defer StopServer()

	sendTestV1SpecificTrap(t, config, "public")
	packet := receivePacket(t)
	require.NotNil(t, packet)
	packet.Content.SnmpTrap.Variables = packet.Content.Variables
	assert.Equal(t, AlarmActiveStatev1SpecificTrap, packet.Content.SnmpTrap)
}

func TestServerV2(t *testing.T) {
	config := Config{Port: serverPort, CommunityStrings: []string{"public"}}
	Configure(t, config)

	err := StartServer("dummy_hostname")
	require.NoError(t, err)
	defer StopServer()

	sendTestV2Trap(t, config, "public")
	packet := receivePacket(t)
	require.NotNil(t, packet)
	assertIsValidV2Packet(t, packet, config)
	assertVariables(t, packet)
}

func TestServerV2BadCredentials(t *testing.T) {
	config := Config{Port: serverPort, CommunityStrings: []string{"public"}}
	Configure(t, config)

	err := StartServer("dummy_hostname")
	require.NoError(t, err)
	defer StopServer()

	sendTestV2Trap(t, config, "wrong-community")
	assertNoPacketReceived(t)
}

func TestServerV3(t *testing.T) {
	userV3 := UserV3{Username: "user", AuthKey: "password", AuthProtocol: "sha", PrivKey: "password", PrivProtocol: "aes"}
	config := Config{Port: serverPort, Users: []UserV3{userV3}}
	Configure(t, config)

	err := StartServer("dummy_hostname")
	require.NoError(t, err)
	defer StopServer()

	sendTestV3Trap(t, config, &gosnmp.UsmSecurityParameters{
		UserName:                 "user",
		AuthoritativeEngineID:    "foo",
		AuthenticationPassphrase: "password",
		AuthenticationProtocol:   gosnmp.SHA,
		PrivacyPassphrase:        "password",
		PrivacyProtocol:          gosnmp.AES,
	})
	packet := receivePacket(t)
	require.NotNil(t, packet)
	assertVariables(t, packet)
}

func TestServerV3BadCredentials(t *testing.T) {
	userV3 := UserV3{Username: "user", AuthKey: "password", AuthProtocol: "sha", PrivKey: "password", PrivProtocol: "aes"}
	config := Config{Port: serverPort, Users: []UserV3{userV3}}
	Configure(t, config)

	err := StartServer("dummy_hostname")
	require.NoError(t, err)
	defer StopServer()

	sendTestV3Trap(t, config, &gosnmp.UsmSecurityParameters{
		UserName:                 "user",
		AuthoritativeEngineID:    "foo",
		AuthenticationPassphrase: "password",
		AuthenticationProtocol:   gosnmp.SHA,
		PrivacyPassphrase:        "wrong_password",
		PrivacyProtocol:          gosnmp.AES,
	})
	assertNoPacketReceived(t)
}

func TestStartFailure(t *testing.T) {
	/*
		Start two servers with the same config to trigger an "address already in use" error.
	*/
	port := serverPort

	config := Config{Port: port, CommunityStrings: []string{"public"}}
	Configure(t, config)

	sucessServer, err := NewTrapServer("dummy_hostname")
	require.NoError(t, err)
	require.NotNil(t, sucessServer)
	defer sucessServer.Stop()

	failedServer, err := NewTrapServer("dummy_hostname")
	require.Nil(t, failedServer)
	require.Error(t, err)
}
