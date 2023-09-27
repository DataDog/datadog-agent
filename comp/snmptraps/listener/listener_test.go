// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package listener

import (
	"errors"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/hostname"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/ndmtmp/sender"
	"github.com/DataDog/datadog-agent/comp/snmptraps/config"
	packetModule "github.com/DataDog/datadog-agent/comp/snmptraps/packet"
	"github.com/DataDog/datadog-agent/comp/snmptraps/status"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ndmtestutils "github.com/DataDog/datadog-agent/pkg/networkdevice/testutils"
)

const defaultTimeout = 1 * time.Second

type services struct {
	fx.In
	Config   config.Component
	Sender   sender.MockComponent
	Status   status.Component
	Listener Component
	Logger   log.Component
}

func listenerTestSetup(t *testing.T, conf *config.TrapsConfig) *services {
	s := fxutil.Test[services](t,
		log.MockModule,
		sender.MockModule,
		config.MockModule,
		hostname.MockModule,
		status.MockModule,
		Module,
		fx.Replace(conf),
	)
	require.NoError(t, s.Listener.Start())
	t.Cleanup(func() { s.Listener.Stop() })
	return &s
}

func TestListenV1GenericTrap(t *testing.T) {
	serverPort, err := ndmtestutils.GetFreePort()
	require.NoError(t, err)
	config := &config.TrapsConfig{Port: serverPort, CommunityStrings: []string{"public"}, Namespace: "totoro"}
	s := listenerTestSetup(t, config)

	sendTestV1GenericTrap(t, s.Config.Get(), "public")
	packet, err := receivePacket(t, s, defaultTimeout)
	require.NoError(t, err)
	packet.Content.SnmpTrap.Variables = packet.Content.Variables
	assert.Equal(t, packetModule.LinkDownv1GenericTrap, packet.Content.SnmpTrap)
}

func TestServerV1SpecificTrap(t *testing.T) {
	serverPort, err := ndmtestutils.GetFreePort()
	require.NoError(t, err)
	config := &config.TrapsConfig{Port: serverPort, CommunityStrings: []string{"public"}}
	s := listenerTestSetup(t, config)

	sendTestV1SpecificTrap(t, config, "public")
	packet, err := receivePacket(t, s, defaultTimeout)
	require.NoError(t, err)
	packet.Content.SnmpTrap.Variables = packet.Content.Variables
	assert.Equal(t, packetModule.AlarmActiveStatev1SpecificTrap, packet.Content.SnmpTrap)
}

func TestServerV2(t *testing.T) {
	serverPort, err := ndmtestutils.GetFreePort()
	require.NoError(t, err)
	config := &config.TrapsConfig{Port: serverPort, CommunityStrings: []string{"public"}}
	s := listenerTestSetup(t, config)

	sendTestV2Trap(t, config, "public")
	packet, err := receivePacket(t, s, defaultTimeout)
	require.NoError(t, err)
	assertIsValidV2Packet(t, packet, config)
	assertVariables(t, packet)
}

func TestServerV2BadCredentials(t *testing.T) {
	serverPort, err := ndmtestutils.GetFreePort()
	require.NoError(t, err)
	config := &config.TrapsConfig{Port: serverPort, CommunityStrings: []string{"public"}, Namespace: "totoro"}
	s := listenerTestSetup(t, config)

	sendTestV2Trap(t, config, "wrong-community")
	_, err2 := receivePacket(t, s, defaultTimeout)
	require.EqualError(t, err2, "invalid packet")

	s.Sender.AssertMetric(t, "Count", "datadog.snmp_traps.received", 1, "", []string{"snmp_device:127.0.0.1", "device_namespace:totoro", "snmp_version:2"})
	s.Sender.AssertMetric(t, "Count", "datadog.snmp_traps.invalid_packet", 1, "", []string{"snmp_device:127.0.0.1", "device_namespace:totoro", "snmp_version:2", "reason:unknown_community_string"})
}

func TestServerV3(t *testing.T) {
	serverPort, err := ndmtestutils.GetFreePort()
	require.NoError(t, err)
	userV3 := config.UserV3{Username: "user", AuthKey: "password", AuthProtocol: "sha", PrivKey: "password", PrivProtocol: "aes"}
	config := &config.TrapsConfig{Port: serverPort, Users: []config.UserV3{userV3}}
	s := listenerTestSetup(t, config)

	sendTestV3Trap(t, config, &gosnmp.UsmSecurityParameters{
		UserName:                 "user",
		AuthoritativeEngineID:    "foobarbaz",
		AuthenticationPassphrase: "password",
		AuthenticationProtocol:   gosnmp.SHA,
		PrivacyPassphrase:        "password",
		PrivacyProtocol:          gosnmp.AES,
	})
	packet, err := receivePacket(t, s, defaultTimeout)
	require.NoError(t, err)
	assertVariables(t, packet)
}

func TestServerV3BadCredentials(t *testing.T) {
	serverPort, err := ndmtestutils.GetFreePort()
	require.NoError(t, err)
	userV3 := config.UserV3{Username: "user", AuthKey: "password", AuthProtocol: "sha", PrivKey: "password", PrivProtocol: "aes"}
	config := &config.TrapsConfig{Port: serverPort, Users: []config.UserV3{userV3}}
	s := listenerTestSetup(t, config)

	sendTestV3Trap(t, config, &gosnmp.UsmSecurityParameters{
		UserName:                 "user",
		AuthoritativeEngineID:    "foobarbaz",
		AuthenticationPassphrase: "password",
		AuthenticationProtocol:   gosnmp.SHA,
		PrivacyPassphrase:        "wrong_password",
		PrivacyProtocol:          gosnmp.AES,
	})
	assertNoPacketReceived(t, s.Listener)
}

func TestListenerTrapsReceivedTelemetry(t *testing.T) {
	serverPort, err := ndmtestutils.GetFreePort()
	require.NoError(t, err)
	config := &config.TrapsConfig{Port: serverPort, CommunityStrings: []string{"public"}, Namespace: "totoro"}
	s := listenerTestSetup(t, config)

	sendTestV1GenericTrap(t, config, "public")
	_, err2 := receivePacket(t, s, defaultTimeout) // Wait fot
	require.NoError(t, err2)
	s.Sender.AssertMetric(t, "Count", "datadog.snmp_traps.received", 1, "", []string{"snmp_device:127.0.0.1", "device_namespace:totoro", "snmp_version:1"})
}

func receivePacket(t *testing.T, s *services, timeoutDuration time.Duration) (*packetModule.SnmpPacket, error) {
	timeout := time.After(timeoutDuration)
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return nil, errors.New("timeout waiting for trap")
		case packet := <-s.Listener.Packets():
			return packet, nil
		case <-ticker.C:
			if s.Status.GetTrapsPacketsAuthErrors() > 0 {
				// invalid packet/bad credentials
				return nil, errors.New("invalid packet")
			}
		}
	}
}

func assertNoPacketReceived(t *testing.T, listener Component) {
	select {
	case <-listener.Packets():
		t.Error("Unexpectedly received an unauthorized packet")
	case <-time.After(100 * time.Millisecond):
		break
	}
}
