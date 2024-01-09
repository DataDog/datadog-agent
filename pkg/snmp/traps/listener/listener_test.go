// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package listener

import (
	"errors"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/snmp/traps/config"
	packetModule "github.com/DataDog/datadog-agent/pkg/snmp/traps/packet"
	"github.com/DataDog/datadog-agent/pkg/snmp/traps/status"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ndmtestutils "github.com/DataDog/datadog-agent/pkg/networkdevice/testutils"
)

const defaultTimeout = 1 * time.Second

func listenerTestSetup(t *testing.T, config *config.TrapsConfig) (*mocksender.MockSender, *TrapListener, status.Manager) {
	logger := fxutil.Test[log.Component](t, logimpl.MockModule())
	mockSender := mocksender.NewMockSender("snmp-traps-telemetry")
	mockSender.SetupAcceptAll()
	packetOutChan := make(packetModule.PacketsChannel, config.GetPacketChannelSize())
	status := status.NewMock()
	trapListener, err := NewTrapListener(config, mockSender, packetOutChan, logger, status)
	require.NoError(t, err)
	err = trapListener.Start()
	require.NoError(t, err)
	return mockSender, trapListener, status
}

func TestListenV1GenericTrap(t *testing.T) {
	serverPort, err := ndmtestutils.GetFreePort()
	require.NoError(t, err)
	config := &config.TrapsConfig{Port: serverPort, CommunityStrings: []string{"public"}, Namespace: "totoro"}
	_, trapListener, status := listenerTestSetup(t, config)
	defer trapListener.Stop()

	sendTestV1GenericTrap(t, config, "public")
	packet, err := receivePacket(t, trapListener, defaultTimeout, status)
	require.NoError(t, err)
	packet.Content.SnmpTrap.Variables = packet.Content.Variables
	assert.Equal(t, packetModule.LinkDownv1GenericTrap, packet.Content.SnmpTrap)
}

func TestServerV1SpecificTrap(t *testing.T) {
	serverPort, err := ndmtestutils.GetFreePort()
	require.NoError(t, err)
	config := &config.TrapsConfig{Port: serverPort, CommunityStrings: []string{"public"}}
	_, trapListener, status := listenerTestSetup(t, config)
	defer trapListener.Stop()

	sendTestV1SpecificTrap(t, config, "public")
	packet, err := receivePacket(t, trapListener, defaultTimeout, status)
	require.NoError(t, err)
	packet.Content.SnmpTrap.Variables = packet.Content.Variables
	assert.Equal(t, packetModule.AlarmActiveStatev1SpecificTrap, packet.Content.SnmpTrap)
}

func TestServerV2(t *testing.T) {
	serverPort, err := ndmtestutils.GetFreePort()
	require.NoError(t, err)
	config := &config.TrapsConfig{Port: serverPort, CommunityStrings: []string{"public"}}
	_, trapListener, status := listenerTestSetup(t, config)
	defer trapListener.Stop()

	sendTestV2Trap(t, config, "public")
	packet, err := receivePacket(t, trapListener, defaultTimeout, status)
	require.NoError(t, err)
	assertIsValidV2Packet(t, packet, config)
	assertVariables(t, packet)
}

func TestServerV2BadCredentials(t *testing.T) {
	serverPort, err := ndmtestutils.GetFreePort()
	require.NoError(t, err)
	config := &config.TrapsConfig{Port: serverPort, CommunityStrings: []string{"public"}, Namespace: "totoro"}
	mockSender, trapListener, status := listenerTestSetup(t, config)
	defer trapListener.Stop()

	sendTestV2Trap(t, config, "wrong-community")
	_, err2 := receivePacket(t, trapListener, defaultTimeout, status)
	require.EqualError(t, err2, "invalid packet")

	mockSender.AssertMetric(t, "Count", "datadog.snmp_traps.received", 1, "", []string{"snmp_device:127.0.0.1", "device_namespace:totoro", "snmp_version:2"})
	mockSender.AssertMetric(t, "Count", "datadog.snmp_traps.invalid_packet", 1, "", []string{"snmp_device:127.0.0.1", "device_namespace:totoro", "snmp_version:2", "reason:unknown_community_string"})
}

func TestServerV3(t *testing.T) {
	serverPort, err := ndmtestutils.GetFreePort()
	require.NoError(t, err)
	userV3 := config.UserV3{Username: "user", AuthKey: "password", AuthProtocol: "sha", PrivKey: "password", PrivProtocol: "aes"}
	config := &config.TrapsConfig{Port: serverPort, Users: []config.UserV3{userV3}}
	_, trapListener, status := listenerTestSetup(t, config)
	defer trapListener.Stop()

	sendTestV3Trap(t, config, gosnmp.AuthPriv, &gosnmp.UsmSecurityParameters{
		UserName:                 "user",
		AuthoritativeEngineID:    "foobarbaz",
		AuthenticationPassphrase: "password",
		AuthenticationProtocol:   gosnmp.SHA,
		PrivacyPassphrase:        "password",
		PrivacyProtocol:          gosnmp.AES,
	})
	packet, err := receivePacket(t, trapListener, defaultTimeout, status)
	require.NoError(t, err)
	assertVariables(t, packet)
}

var users = []config.UserV3{
	{Username: "user", AuthKey: "password", AuthProtocol: "sha", PrivKey: "password", PrivProtocol: "aes"},
	{Username: "user2", AuthKey: "password2", AuthProtocol: "md5", PrivKey: "password", PrivProtocol: "des"},
	{Username: "user2", AuthKey: "password2", AuthProtocol: "sha", PrivKey: "password", PrivProtocol: "aes"},
	{Username: "user3", AuthKey: "password", AuthProtocol: "sha"},
}

func TestServerV3MultipleCredentials(t *testing.T) {
	tests := []struct {
		name      string
		msgFlags  gosnmp.SnmpV3MsgFlags
		secParams *gosnmp.UsmSecurityParameters
	}{
		{"user AuthPriv SHA/AES succeeds",
			gosnmp.AuthPriv,
			&gosnmp.UsmSecurityParameters{
				UserName:                 "user",
				AuthoritativeEngineID:    "foobarbaz",
				AuthenticationPassphrase: "password",
				AuthenticationProtocol:   gosnmp.SHA,
				PrivacyPassphrase:        "password",
				PrivacyProtocol:          gosnmp.AES,
			},
		},
		{"user2 (multiple entries) AuthPriv MD5/DES succeeds",
			gosnmp.AuthPriv,
			&gosnmp.UsmSecurityParameters{
				UserName:                 "user",
				AuthoritativeEngineID:    "foobarbaz",
				AuthenticationPassphrase: "password",
				AuthenticationProtocol:   gosnmp.SHA,
				PrivacyPassphrase:        "password",
				PrivacyProtocol:          gosnmp.AES,
			},
		},
		{"user2 (multiple entries) AuthPriv SHA/AES succeeds",
			gosnmp.AuthPriv,
			&gosnmp.UsmSecurityParameters{
				UserName:                 "user",
				AuthoritativeEngineID:    "foobarbaz",
				AuthenticationPassphrase: "password",
				AuthenticationProtocol:   gosnmp.SHA,
				PrivacyPassphrase:        "password",
				PrivacyProtocol:          gosnmp.AES,
			},
		},
		{"user3 AuthNoPriv SHA succeeds",
			gosnmp.AuthNoPriv,
			&gosnmp.UsmSecurityParameters{
				UserName:                 "user",
				AuthoritativeEngineID:    "foobarbaz",
				AuthenticationPassphrase: "password",
				AuthenticationProtocol:   gosnmp.SHA,
				PrivacyPassphrase:        "password",
				PrivacyProtocol:          gosnmp.AES,
			},
		},
	}
	serverPort, err := ndmtestutils.GetFreePort()
	require.NoError(t, err)

	config := &config.TrapsConfig{Port: serverPort, Users: users}
	_, trapListener, status := listenerTestSetup(t, config)
	defer trapListener.Stop()

	for _, test := range tests {
		sendTestV3Trap(t, config, test.msgFlags, test.secParams)
		packet, err := receivePacket(t, trapListener, defaultTimeout, status)
		require.NoError(t, err)
		assertVariables(t, packet)
	}
}

func TestServerV3BadCredentialsWithMultipleUsers(t *testing.T) {
	serverPort, err := ndmtestutils.GetFreePort()
	require.NoError(t, err)
	config := &config.TrapsConfig{Port: serverPort, Users: users}
	_, trapListener, _ := listenerTestSetup(t, config)
	defer trapListener.Stop()

	sendTestV3Trap(t, config, gosnmp.AuthPriv, &gosnmp.UsmSecurityParameters{
		UserName:                 "user2",
		AuthoritativeEngineID:    "foobarbaz",
		AuthenticationPassphrase: "password2",
		AuthenticationProtocol:   gosnmp.SHA,
		PrivacyPassphrase:        "wrong_password",
		PrivacyProtocol:          gosnmp.AES,
	})
	assertNoPacketReceived(t, trapListener)
}

func TestServerV3BadCredentials(t *testing.T) {
	serverPort, err := ndmtestutils.GetFreePort()
	require.NoError(t, err)
	userV3 := config.UserV3{Username: "user", AuthKey: "password", AuthProtocol: "sha", PrivKey: "password", PrivProtocol: "aes"}
	config := &config.TrapsConfig{Port: serverPort, Users: []config.UserV3{userV3}}
	_, trapListener, _ := listenerTestSetup(t, config)
	defer trapListener.Stop()

	sendTestV3Trap(t, config, gosnmp.AuthPriv, &gosnmp.UsmSecurityParameters{
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
	serverPort, err := ndmtestutils.GetFreePort()
	require.NoError(t, err)
	config := &config.TrapsConfig{Port: serverPort, CommunityStrings: []string{"public"}, Namespace: "totoro"}
	mockSender, trapListener, status := listenerTestSetup(t, config)
	defer trapListener.Stop()

	sendTestV1GenericTrap(t, config, "public")
	_, err2 := receivePacket(t, trapListener, defaultTimeout, status) // Wait for packet
	require.NoError(t, err2)
	mockSender.AssertMetric(t, "Count", "datadog.snmp_traps.received", 1, "", []string{"snmp_device:127.0.0.1", "device_namespace:totoro", "snmp_version:1"})
}

func receivePacket(t *testing.T, listener *TrapListener, timeoutDuration time.Duration, status status.Manager) (*packetModule.SnmpPacket, error) { //nolint:revive // TODO fix revive unused-parameter
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
			if status.GetTrapsPacketsAuthErrors() > 0 {
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
