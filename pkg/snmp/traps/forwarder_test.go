// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package traps

import (
	"fmt"
	"github.com/mitchellh/hashstructure/v2"
	"net"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
)

type DummyFormatter struct{}

var simpleUDPAddr = &net.UDPAddr{IP: net.IPv4(1, 1, 1, 1), Port: 161}

type dummyFormatterHashStruct struct {
	addr      net.UDPAddr
	community string
	snmpTraps gosnmp.SnmpTrap
	variables []gosnmp.SnmpPDU
	version   gosnmp.SnmpVersion
}

// FormatPacket is a dummy formatter method that hashes an SnmpPacket object
func (f DummyFormatter) FormatPacket(packet *SnmpPacket) ([]byte, error) {
	v := dummyFormatterHashStruct{
		addr:      *packet.Addr,
		community: packet.Content.Community,
		snmpTraps: packet.Content.SnmpTrap,
		variables: packet.Content.Variables,
		version:   packet.Content.Version,
	}
	hash, err := hashstructure.Hash(v, hashstructure.FormatV2, nil)
	if err != nil {
		return nil, err
	}
	hex := fmt.Sprintf("%x", hash)
	return []byte(hex), nil
}

func createForwarder(t *testing.T) (forwarder *TrapForwarder, err error) {
	packetsIn := make(PacketsChannel)
	mockSender := mocksender.NewMockSender("snmp-traps-listener")
	mockSender.SetupAcceptAll()

	config := Config{Port: serverPort, CommunityStrings: []string{"public"}, Namespace: "default"}
	Configure(t, config)

	forwarder, err = NewTrapForwarder(&DummyFormatter{}, mockSender, packetsIn)
	if err != nil {
		return nil, err
	}
	forwarder.Start()
	return forwarder, err
}

func makeSnmpPacket(trap gosnmp.SnmpTrap) *SnmpPacket {
	gosnmpPacket := &gosnmp.SnmpPacket{
		Version:   gosnmp.Version2c,
		Community: "public",
		Variables: trap.Variables,
		SnmpTrap:  trap,
	}
	return &SnmpPacket{gosnmpPacket, simpleUDPAddr, "totoro", time.Now().UnixMilli()}
}

func TestV1GenericTrapAreForwarder(t *testing.T) {
	forwarder, err := createForwarder(t)
	require.NoError(t, err)
	sender, ok := forwarder.sender.(*mocksender.MockSender)
	require.True(t, ok)
	packet := makeSnmpPacket(LinkDownv1GenericTrap)
	rawEvent, err := forwarder.formatter.FormatPacket(packet)
	require.NoError(t, err)
	forwarder.trapsIn <- packet
	forwarder.Stop()
	sender.AssertEventPlatformEvent(t, rawEvent, epforwarder.EventTypeSnmpTraps)
}

func TestV1SpecificTrapAreForwarder(t *testing.T) {
	forwarder, err := createForwarder(t)
	require.NoError(t, err)
	sender, ok := forwarder.sender.(*mocksender.MockSender)
	require.True(t, ok)
	packet := makeSnmpPacket(AlarmActiveStatev1SpecificTrap)
	rawEvent, err := forwarder.formatter.FormatPacket(packet)
	require.NoError(t, err)
	forwarder.trapsIn <- packet
	forwarder.Stop()
	sender.AssertEventPlatformEvent(t, rawEvent, epforwarder.EventTypeSnmpTraps)
}
func TestV2TrapAreForwarder(t *testing.T) {
	forwarder, err := createForwarder(t)
	require.NoError(t, err)
	sender, ok := forwarder.sender.(*mocksender.MockSender)
	require.True(t, ok)
	forwarder.trapsIn <- makeSnmpPacket(NetSNMPExampleHeartbeatNotification)
	forwarder.Stop()
	sender.AssertEventPlatformEvent(t, []byte("a0c2196b2643152f"), epforwarder.EventTypeSnmpTraps)
}

func TestForwarderTelemetry(t *testing.T) {
	forwarder, err := createForwarder(t)
	require.NoError(t, err)
	sender, ok := forwarder.sender.(*mocksender.MockSender)
	require.True(t, ok)
	forwarder.trapsIn <- makeSnmpPacket(NetSNMPExampleHeartbeatNotification)
	forwarder.Stop()
	sender.AssertMetric(t, "Count", "datadog.snmp_traps.forwarded", 1, "", []string{"snmp_device:1.1.1.1", "device_namespace:totoro", "snmp_version:2"})
}
