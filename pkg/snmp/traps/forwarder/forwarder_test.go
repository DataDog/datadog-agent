// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package forwarder

import (
	"net"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"

	"github.com/DataDog/datadog-agent/pkg/snmp/traps/formatter"
	"github.com/DataDog/datadog-agent/pkg/snmp/traps/packet"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

var simpleUDPAddr = &net.UDPAddr{IP: net.IPv4(1, 1, 1, 1), Port: 161}

func createForwarder(t *testing.T) (forwarder *TrapForwarder, err error) {
	logger := fxutil.Test[log.Component](t, logimpl.MockModule())
	packetsIn := make(packet.PacketsChannel)
	mockSender := mocksender.NewMockSender("snmp-traps-listener")
	mockSender.SetupAcceptAll()

	forwarder, err = NewTrapForwarder(&formatter.DummyFormatter{}, mockSender, packetsIn, logger)
	if err != nil {
		return nil, err
	}
	forwarder.Start()
	t.Cleanup(func() { forwarder.Stop() })
	return forwarder, err
}

func makeSnmpPacket(trap gosnmp.SnmpTrap) *packet.SnmpPacket {
	gosnmpPacket := &gosnmp.SnmpPacket{
		Version:   gosnmp.Version2c,
		Community: "public",
		Variables: trap.Variables,
		SnmpTrap:  trap,
	}
	return &packet.SnmpPacket{
		Content:   gosnmpPacket,
		Addr:      simpleUDPAddr,
		Namespace: "totoro",
		Timestamp: time.Now().UnixMilli()}
}

func TestV1GenericTrapAreForwarder(t *testing.T) {
	forwarder, err := createForwarder(t)
	require.NoError(t, err)
	sender, ok := forwarder.sender.(*mocksender.MockSender)
	require.True(t, ok)
	packet := makeSnmpPacket(packet.LinkDownv1GenericTrap)
	rawEvent, err := forwarder.formatter.FormatPacket(packet)
	require.NoError(t, err)
	forwarder.trapsIn <- packet
	time.Sleep(100 * time.Millisecond)
	sender.AssertEventPlatformEvent(t, rawEvent, epforwarder.EventTypeSnmpTraps)
}

func TestV1SpecificTrapAreForwarder(t *testing.T) {
	forwarder, err := createForwarder(t)
	require.NoError(t, err)
	sender, ok := forwarder.sender.(*mocksender.MockSender)
	require.True(t, ok)
	packet := makeSnmpPacket(packet.AlarmActiveStatev1SpecificTrap)
	rawEvent, err := forwarder.formatter.FormatPacket(packet)
	require.NoError(t, err)
	forwarder.trapsIn <- packet
	time.Sleep(100 * time.Millisecond)
	sender.AssertEventPlatformEvent(t, rawEvent, epforwarder.EventTypeSnmpTraps)
}
func TestV2TrapAreForwarder(t *testing.T) {
	forwarder, err := createForwarder(t)
	require.NoError(t, err)
	sender, ok := forwarder.sender.(*mocksender.MockSender)
	require.True(t, ok)
	packet := makeSnmpPacket(packet.NetSNMPExampleHeartbeatNotification)
	rawEvent, err := forwarder.formatter.FormatPacket(packet)
	require.NoError(t, err)
	forwarder.trapsIn <- packet
	time.Sleep(100 * time.Millisecond)
	sender.AssertEventPlatformEvent(t, rawEvent, epforwarder.EventTypeSnmpTraps)
}

func TestForwarderTelemetry(t *testing.T) {
	forwarder, err := createForwarder(t)
	require.NoError(t, err)
	sender, ok := forwarder.sender.(*mocksender.MockSender)
	require.True(t, ok)
	forwarder.trapsIn <- makeSnmpPacket(packet.NetSNMPExampleHeartbeatNotification)
	time.Sleep(100 * time.Millisecond)
	sender.AssertMetric(t, "Count", "datadog.snmp_traps.forwarded", 1, "", []string{"snmp_device:1.1.1.1", "device_namespace:totoro", "snmp_version:2"})
}
