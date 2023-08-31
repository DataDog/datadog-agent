// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package traps

import (
	"errors"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var serverPort = getFreePort()
var initialTrapsPacketsAuthErrors int64

const defaultTimeout = 1 * time.Second

func listenerTestSetup(t *testing.T, config Config) (*mocksender.MockSender, *TrapListener) {
	mockSender := mocksender.NewMockSender("snmp-traps-telemetry")
	mockSender.SetupAcceptAll()
	packetOutChan := make(PacketsChannel, packetsChanSize)

	trapListener, err := startSNMPTrapListener(config, mockSender, packetOutChan)
	require.NoError(t, err)

	// trapsPacketsAuthErrors is global so its value carries over from test to test.  Capture its initial value to determine if it changes during an individual test run.
	initialTrapsPacketsAuthErrors = trapsPacketsAuthErrors.Value()

	return mockSender, trapListener
}

func TestListenV1GenericTrap(t *testing.T) {
	config := Config{Port: serverPort, CommunityStrings: []string{"public"}, Namespace: "totoro"}
	_, trapListener := listenerTestSetup(t, config)
	defer trapListener.Stop()

	sendTestV1GenericTrap(t, config, "public")
	packet, err := receivePacket(t, trapListener, defaultTimeout)
	require.NoError(t, err)
	packet.Content.SnmpTrap.Variables = packet.Content.Variables
	assert.Equal(t, LinkDownv1GenericTrap, packet.Content.SnmpTrap)
}

func TestServerV1SpecificTrap(t *testing.T) {
	config := Config{Port: serverPort, CommunityStrings: []string{"public"}}
	_, trapListener := listenerTestSetup(t, config)
	defer trapListener.Stop()

	sendTestV1SpecificTrap(t, config, "public")
	packet, err := receivePacket(t, trapListener, defaultTimeout)
	require.NoError(t, err)
	packet.Content.SnmpTrap.Variables = packet.Content.Variables
	assert.Equal(t, AlarmActiveStatev1SpecificTrap, packet.Content.SnmpTrap)
}

func TestServerV2(t *testing.T) {
	config := Config{Port: serverPort, CommunityStrings: []string{"public"}}
	_, trapListener := listenerTestSetup(t, config)
	defer trapListener.Stop()

	sendTestV2Trap(t, config, "public")
	packet, err := receivePacket(t, trapListener, defaultTimeout)
	require.NoError(t, err)
	assertIsValidV2Packet(t, packet, config)
	assertVariables(t, packet)
}

func TestServerV2BadCredentials(t *testing.T) {
	config := Config{Port: serverPort, CommunityStrings: []string{"public"}, Namespace: "totoro"}
	mockSender, trapListener := listenerTestSetup(t, config)
	defer trapListener.Stop()

	sendTestV2Trap(t, config, "wrong-community")
	_, err2 := receivePacket(t, trapListener, defaultTimeout)
	require.EqualError(t, err2, "invalid packet")

	mockSender.AssertMetric(t, "Count", "datadog.snmp_traps.received", 1, "", []string{"snmp_device:127.0.0.1", "device_namespace:totoro", "snmp_version:2"})
	mockSender.AssertMetric(t, "Count", "datadog.snmp_traps.invalid_packet", 1, "", []string{"snmp_device:127.0.0.1", "device_namespace:totoro", "snmp_version:2", "reason:unknown_community_string"})
}

func TestServerV3(t *testing.T) {
	userV3 := UserV3{Username: "user", AuthKey: "password", AuthProtocol: "sha", PrivKey: "password", PrivProtocol: "aes"}
	config := Config{Port: serverPort, Users: []UserV3{userV3}}
	_, trapListener := listenerTestSetup(t, config)
	defer trapListener.Stop()

	sendTestV3Trap(t, config, &gosnmp.UsmSecurityParameters{
		UserName:                 "user",
		AuthoritativeEngineID:    "foobarbaz",
		AuthenticationPassphrase: "password",
		AuthenticationProtocol:   gosnmp.SHA,
		PrivacyPassphrase:        "password",
		PrivacyProtocol:          gosnmp.AES,
	})
	packet, err := receivePacket(t, trapListener, defaultTimeout)
	require.NoError(t, err)
	assertVariables(t, packet)
}

func TestServerV3BadCredentials(t *testing.T) {
	userV3 := UserV3{Username: "user", AuthKey: "password", AuthProtocol: "sha", PrivKey: "password", PrivProtocol: "aes"}
	config := Config{Port: serverPort, Users: []UserV3{userV3}}
	_, trapListener := listenerTestSetup(t, config)
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
	config := Config{Port: serverPort, CommunityStrings: []string{"public"}, Namespace: "totoro"}
	mockSender, trapListener := listenerTestSetup(t, config)
	defer trapListener.Stop()

	sendTestV1GenericTrap(t, config, "public")
	_, err2 := receivePacket(t, trapListener, defaultTimeout) // Wait for packet
	require.NoError(t, err2)
	mockSender.AssertMetric(t, "Count", "datadog.snmp_traps.received", 1, "", []string{"snmp_device:127.0.0.1", "device_namespace:totoro", "snmp_version:1"})
}

func receivePacket(t *testing.T, listener *TrapListener, timeoutDuration time.Duration) (*SnmpPacket, error) {
	timeout := time.After(timeoutDuration)
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return nil, errors.New("timeout waiting for trap")
		case packet := <-listener.packets:
			return packet, nil
		case <-ticker.C:
			if trapsPacketsAuthErrors.Value() > initialTrapsPacketsAuthErrors {
				// invalid packet/bad credentials
				return nil, errors.New("invalid packet")
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
