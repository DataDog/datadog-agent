// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build test

package listener

import (
	"errors"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/netflow/hostname"
	"github.com/DataDog/datadog-agent/comp/netflow/sender"
	"github.com/DataDog/datadog-agent/comp/netflow/testutil"
	"github.com/DataDog/datadog-agent/comp/snmptraps/config"
	packetModule "github.com/DataDog/datadog-agent/comp/snmptraps/packet"
	"github.com/DataDog/datadog-agent/comp/snmptraps/status"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var serverPort = testutil.GetFreePort()
var initialTrapsPacketsAuthErrors int64

const defaultTimeout = 1 * time.Second

func listenerTestSetup(t *testing.T, conf *config.TrapsConfig) (sender.MockComponent, *TrapListener, status.Component) {
	var trapSender sender.MockComponent
	var stats status.Component
	listener := fxutil.Test[Component](t,
		log.MockModule,
		sender.MockModule,
		config.MockModule,
		hostname.MockModule,
		status.MockModule,
		Module,
		fx.Replace(conf),
		fx.Populate(&trapSender),
		fx.Populate(&stats),
	).(*TrapListener)
	require.NoError(t, listener.Start())
	return trapSender, listener, stats
}

func TestListenV1GenericTrap(t *testing.T) {
	config := &config.TrapsConfig{Port: serverPort, CommunityStrings: []string{"public"}, Namespace: "totoro"}
	_, trapListener, stats := listenerTestSetup(t, config)
	defer trapListener.Stop()

	SendTestV1GenericTrap(t, trapListener.config, "public")
	packet, err := receivePacket(t, trapListener, defaultTimeout, stats)
	require.NoError(t, err)
	packet.Content.SnmpTrap.Variables = packet.Content.Variables
	assert.Equal(t, packetModule.LinkDownv1GenericTrap, packet.Content.SnmpTrap)
}

func TestServerV1SpecificTrap(t *testing.T) {
	config := &config.TrapsConfig{Port: serverPort, CommunityStrings: []string{"public"}}
	_, trapListener, stats := listenerTestSetup(t, config)
	defer trapListener.Stop()

	SendTestV1SpecificTrap(t, config, "public")
	packet, err := receivePacket(t, trapListener, defaultTimeout, stats)
	require.NoError(t, err)
	packet.Content.SnmpTrap.Variables = packet.Content.Variables
	assert.Equal(t, packetModule.AlarmActiveStatev1SpecificTrap, packet.Content.SnmpTrap)
}

func TestServerV2(t *testing.T) {
	config := &config.TrapsConfig{Port: serverPort, CommunityStrings: []string{"public"}}
	_, trapListener, stats := listenerTestSetup(t, config)
	defer trapListener.Stop()

	SendTestV2Trap(t, config, "public")
	packet, err := receivePacket(t, trapListener, defaultTimeout, stats)
	require.NoError(t, err)
	AssertIsValidV2Packet(t, packet, config)
	AssertVariables(t, packet)
}

func TestServerV2BadCredentials(t *testing.T) {
	config := &config.TrapsConfig{Port: serverPort, CommunityStrings: []string{"public"}, Namespace: "totoro"}
	mockSender, trapListener, stats := listenerTestSetup(t, config)
	defer trapListener.Stop()

	SendTestV2Trap(t, config, "wrong-community")
	_, err2 := receivePacket(t, trapListener, defaultTimeout, stats)
	require.EqualError(t, err2, "invalid packet")

	mockSender.AssertMetric(t, "Count", "datadog.snmp_traps.received", 1, "", []string{"snmp_device:127.0.0.1", "device_namespace:totoro", "snmp_version:2"})
	mockSender.AssertMetric(t, "Count", "datadog.snmp_traps.invalid_packet", 1, "", []string{"snmp_device:127.0.0.1", "device_namespace:totoro", "snmp_version:2", "reason:unknown_community_string"})
}

func TestServerV3(t *testing.T) {
	userV3 := config.UserV3{Username: "user", AuthKey: "password", AuthProtocol: "sha", PrivKey: "password", PrivProtocol: "aes"}
	config := &config.TrapsConfig{Port: serverPort, Users: []config.UserV3{userV3}}
	_, trapListener, stats := listenerTestSetup(t, config)
	defer trapListener.Stop()

	SendTestV3Trap(t, config, &gosnmp.UsmSecurityParameters{
		UserName:                 "user",
		AuthoritativeEngineID:    "foobarbaz",
		AuthenticationPassphrase: "password",
		AuthenticationProtocol:   gosnmp.SHA,
		PrivacyPassphrase:        "password",
		PrivacyProtocol:          gosnmp.AES,
	})
	packet, err := receivePacket(t, trapListener, defaultTimeout, stats)
	require.NoError(t, err)
	AssertVariables(t, packet)
}

func TestServerV3BadCredentials(t *testing.T) {
	userV3 := config.UserV3{Username: "user", AuthKey: "password", AuthProtocol: "sha", PrivKey: "password", PrivProtocol: "aes"}
	config := &config.TrapsConfig{Port: serverPort, Users: []config.UserV3{userV3}}
	_, trapListener, _ := listenerTestSetup(t, config)
	defer trapListener.Stop()

	SendTestV3Trap(t, config, &gosnmp.UsmSecurityParameters{
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
	config := &config.TrapsConfig{Port: serverPort, CommunityStrings: []string{"public"}, Namespace: "totoro"}
	mockSender, trapListener, stats := listenerTestSetup(t, config)
	defer trapListener.Stop()

	SendTestV1GenericTrap(t, config, "public")
	_, err2 := receivePacket(t, trapListener, defaultTimeout, stats) // Wait for packet
	require.NoError(t, err2)
	mockSender.AssertMetric(t, "Count", "datadog.snmp_traps.received", 1, "", []string{"snmp_device:127.0.0.1", "device_namespace:totoro", "snmp_version:1"})
}

func receivePacket(t *testing.T, listener *TrapListener, timeoutDuration time.Duration, stats status.Component) (*packetModule.SnmpPacket, error) {
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
			if stats.GetTrapsPacketsAuthErrors() > 0 {
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
