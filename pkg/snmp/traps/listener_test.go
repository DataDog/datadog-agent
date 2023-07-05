// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package traps

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var serverPort = getFreePort()

const defaultTimeout = 1 * time.Second

func TestListenV1GenericTrap(t *testing.T) {
	mockSender := mocksender.NewMockSender("snmp-traps-telemetry")
	mockSender.SetupAcceptAll()

	config := Config{Port: serverPort, CommunityStrings: []string{"public"}, Namespace: "totoro"}
	Configure(t, config)

	packetOutChan := make(PacketsChannel)
	trapListener, err := startSNMPTrapListener(config, mockSender, packetOutChan)
	require.NoError(t, err)
	defer trapListener.Stop()

	sendTestV1GenericTrap(t, config, "public")
	packet := receivePacket(t, trapListener, defaultTimeout)
	require.NotNil(t, packet)
	packet.Content.SnmpTrap.Variables = packet.Content.Variables
	assert.Equal(t, LinkDownv1GenericTrap, packet.Content.SnmpTrap)
}

func TestServerV1SpecificTrap(t *testing.T) {
	mockSender := mocksender.NewMockSender("snmp-traps-telemetry")
	mockSender.SetupAcceptAll()

	config := Config{Port: serverPort, CommunityStrings: []string{"public"}}
	Configure(t, config)

	packetOutChan := make(PacketsChannel)
	trapListener, err := startSNMPTrapListener(config, mockSender, packetOutChan)
	require.NoError(t, err)
	defer trapListener.Stop()

	sendTestV1SpecificTrap(t, config, "public")
	packet := receivePacket(t, trapListener, defaultTimeout)
	require.NotNil(t, packet)
	packet.Content.SnmpTrap.Variables = packet.Content.Variables
	assert.Equal(t, AlarmActiveStatev1SpecificTrap, packet.Content.SnmpTrap)
}

func TestServerV2(t *testing.T) {
	mockSender := mocksender.NewMockSender("snmp-traps-telemetry")
	mockSender.SetupAcceptAll()

	config := Config{Port: serverPort, CommunityStrings: []string{"public"}}
	Configure(t, config)

	packetOutChan := make(PacketsChannel)
	trapListener, err := startSNMPTrapListener(config, mockSender, packetOutChan)
	require.NoError(t, err)
	defer trapListener.Stop()

	sendTestV2Trap(t, config, "public")
	packet := receivePacket(t, trapListener, defaultTimeout)
	require.NotNil(t, packet)
	assertIsValidV2Packet(t, packet, config)
	assertVariables(t, packet)
}

func TestServerV2BadCredentials(t *testing.T) {
	mockSender := mocksender.NewMockSender("snmp-traps-telemetry")
	mockSender.SetupAcceptAll()

	config := Config{Port: serverPort, CommunityStrings: []string{"public"}, Namespace: "totoro"}
	Configure(t, config)

	packetOutChan := make(PacketsChannel)
	trapListener, err := startSNMPTrapListener(config, mockSender, packetOutChan)
	require.NoError(t, err)
	defer trapListener.Stop()

	sendTestV2Trap(t, config, "wrong-community")
	packet := receivePacket(t, trapListener, defaultTimeout)
	require.Nil(t, packet)

	mockSender.AssertMetric(t, "Count", "datadog.snmp_traps.received", 1, "", []string{"snmp_device:127.0.0.1", "device_namespace:totoro", "snmp_version:2"})
	mockSender.AssertMetric(t, "Count", "datadog.snmp_traps.invalid_packet", 1, "", []string{"snmp_device:127.0.0.1", "device_namespace:totoro", "snmp_version:2", "reason:unknown_community_string"})
}

func TestServerV3(t *testing.T) {
	mockSender := mocksender.NewMockSender("snmp-traps-telemetry")
	mockSender.SetupAcceptAll()

	userV3 := UserV3{Username: "user", AuthKey: "password", AuthProtocol: "sha", PrivKey: "password", PrivProtocol: "aes"}
	config := Config{Port: serverPort, Users: []UserV3{userV3}}
	Configure(t, config)

	packetOutChan := make(PacketsChannel)
	trapListener, err := startSNMPTrapListener(config, mockSender, packetOutChan)
	require.NoError(t, err)
	defer trapListener.Stop()

	sendTestV3Trap(t, config, &gosnmp.UsmSecurityParameters{
		UserName:                 "user",
		AuthoritativeEngineID:    "foobarbaz",
		AuthenticationPassphrase: "password",
		AuthenticationProtocol:   gosnmp.SHA,
		PrivacyPassphrase:        "password",
		PrivacyProtocol:          gosnmp.AES,
	})
	packet := receivePacket(t, trapListener, defaultTimeout)
	require.NotNil(t, packet)
	assertVariables(t, packet)
}

func TestServerV3BadCredentials(t *testing.T) {
	mockSender := mocksender.NewMockSender("snmp-traps-telemetry")
	mockSender.SetupAcceptAll()

	userV3 := UserV3{Username: "user", AuthKey: "password", AuthProtocol: "sha", PrivKey: "password", PrivProtocol: "aes"}
	config := Config{Port: serverPort, Users: []UserV3{userV3}}
	Configure(t, config)

	packetOutChan := make(PacketsChannel)
	trapListener, err := startSNMPTrapListener(config, mockSender, packetOutChan)
	require.NoError(t, err)
	defer trapListener.Stop()

	sendTestV3Trap(t, config, &gosnmp.UsmSecurityParameters{
		UserName:                 "user",
		AuthoritativeEngineID:    "foobarbaz",
		AuthenticationPassphrase: "password",
		AuthenticationProtocol:   gosnmp.SHA,
		PrivacyPassphrase:        "wrong_password",
		PrivacyProtocol:          gosnmp.AES,
	})
	assertNoPacketReceived(t, trapListener)
}

func TestListenerTrapsReceivedTelemetry(t *testing.T) {
	mockSender := mocksender.NewMockSender("snmp-traps-telemetry")
	mockSender.SetupAcceptAll()

	config := Config{Port: serverPort, CommunityStrings: []string{"public"}, Namespace: "totoro"}
	Configure(t, config)

	packetOutChan := make(PacketsChannel)
	trapListener, err := startSNMPTrapListener(config, mockSender, packetOutChan)
	require.NoError(t, err)
	defer trapListener.Stop()

	sendTestV1GenericTrap(t, config, "public")
	packet := receivePacket(t, trapListener, defaultTimeout) // Wait for packet
	require.NotNil(t, packet)
	mockSender.AssertMetric(t, "Count", "datadog.snmp_traps.received", 1, "", []string{"snmp_device:127.0.0.1", "device_namespace:totoro", "snmp_version:1"})
}

func receivePacket(t *testing.T, listener *TrapListener, timeoutDuration time.Duration) *SnmpPacket {
	timeout := time.After(timeoutDuration)
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	// Wait for a packet to be received, if packet is invalid wait until receivedTrapsCount is incremented
	for {
		select {
		// Got a timeout! fail with a timeout error
		case <-timeout:
			t.Error("timeout error waiting for trap")
			return nil
		case packet := <-listener.packets:
			return packet
		case <-ticker.C:
			if listener.receivedTrapsCount.Load() > 0 {
				return nil // We received an invalid packet
			}
		}
	}
}

func assertNoPacketReceived(t *testing.T, listener *TrapListener) {
	select {
	case <-listener.packets:
		t.Error("Unexpectedly received an unauthorized packet")
	case <-time.After(100 * time.Millisecond):
		break
	}
}
