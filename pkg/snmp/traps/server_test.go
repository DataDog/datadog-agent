// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package traps

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestServerV2(t *testing.T) {
	config := Config{Port: GetPort(t), CommunityStrings: []string{"public"}}
	Configure(t, config)

	err := StartServer()
	require.NoError(t, err)
	defer StopServer()

	sendTestV2Trap(t, config, "public")
	packet := receivePacket(t)
	require.NotNil(t, packet)
	assertIsValidV2Packet(t, packet, config)
	assertV2Variables(t, packet)
}

func TestServerV2BadCredentials(t *testing.T) {
	config := Config{Port: GetPort(t), CommunityStrings: []string{"public"}}
	Configure(t, config)

	err := StartServer()
	require.NoError(t, err)
	defer StopServer()

	sendTestV2Trap(t, config, "wrong-community")
	assertNoPacketReceived(t)
}

func TestStartFailure(t *testing.T) {
	/*
		Start two servers with the same config to trigger an "address already in use" error.
	*/
	port := GetPort(t)

	config := Config{Port: port, CommunityStrings: []string{"public"}}
	Configure(t, config)

	sucessServer, err := NewTrapServer()
	require.NoError(t, err)
	require.NotNil(t, sucessServer)
	defer sucessServer.Stop()

	failedServer, err := NewTrapServer()
	require.Nil(t, failedServer)
	require.Error(t, err)
}
