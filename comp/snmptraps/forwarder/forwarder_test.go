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
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/netflow/sender"
	"github.com/DataDog/datadog-agent/comp/snmptraps/formatter"
	"github.com/DataDog/datadog-agent/comp/snmptraps/packet"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

var simpleUDPAddr = &net.UDPAddr{IP: net.IPv4(1, 1, 1, 1), Port: 161}

func createForwarder(t *testing.T) *TrapForwarder {
	forwarder := fxutil.Test[Component](t,
		log.MockModule,
		sender.MockModule,
		formatter.MockModule,
		fx.Provide(newTrapForwarder),
		fx.Supply(make(packet.PacketsChannel)),
	).(*TrapForwarder)
	forwarder.Start()
	return forwarder
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
	forwarder := createForwarder(t)
	sender, ok := forwarder.sender.(*mocksender.MockSender)
	require.True(t, ok)
	packet := makeSnmpPacket(packet.LinkDownv1GenericTrap)
	rawEvent, err := forwarder.formatter.FormatPacket(packet)
	require.NoError(t, err)
	forwarder.trapsIn <- packet
	forwarder.Stop()
	sender.AssertEventPlatformEvent(t, rawEvent, epforwarder.EventTypeSnmpTraps)
}

func TestV1SpecificTrapAreForwarder(t *testing.T) {
	forwarder := createForwarder(t)
	sender, ok := forwarder.sender.(*mocksender.MockSender)
	require.True(t, ok)
	packet := makeSnmpPacket(packet.AlarmActiveStatev1SpecificTrap)
	rawEvent, err := forwarder.formatter.FormatPacket(packet)
	require.NoError(t, err)
	forwarder.trapsIn <- packet
	forwarder.Stop()
	sender.AssertEventPlatformEvent(t, rawEvent, epforwarder.EventTypeSnmpTraps)
}
func TestV2TrapAreForwarder(t *testing.T) {
	forwarder := createForwarder(t)
	sender, ok := forwarder.sender.(*mocksender.MockSender)
	require.True(t, ok)
	packet := makeSnmpPacket(packet.NetSNMPExampleHeartbeatNotification)
	rawEvent, err := forwarder.formatter.FormatPacket(packet)
	require.NoError(t, err)
	forwarder.trapsIn <- packet
	forwarder.Stop()
	sender.AssertEventPlatformEvent(t, rawEvent, epforwarder.EventTypeSnmpTraps)
}

func TestForwarderTelemetry(t *testing.T) {
	forwarder := createForwarder(t)
	sender, ok := forwarder.sender.(*mocksender.MockSender)
	require.True(t, ok)
	forwarder.trapsIn <- makeSnmpPacket(packet.NetSNMPExampleHeartbeatNotification)
	forwarder.Stop()
	sender.AssertMetric(t, "Count", "datadog.snmp_traps.forwarded", 1, "", []string{"snmp_device:1.1.1.1", "device_namespace:totoro", "snmp_version:2"})
}
