// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package forwarderimpl

import (
	"net"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/comp/snmptraps/config/configimpl"
	"github.com/DataDog/datadog-agent/comp/snmptraps/formatter"
	"github.com/DataDog/datadog-agent/comp/snmptraps/formatter/formatterimpl"
	"github.com/DataDog/datadog-agent/comp/snmptraps/forwarder"
	"github.com/DataDog/datadog-agent/comp/snmptraps/listener"
	"github.com/DataDog/datadog-agent/comp/snmptraps/listener/listenerimpl"
	"github.com/DataDog/datadog-agent/comp/snmptraps/packet"
	"github.com/DataDog/datadog-agent/comp/snmptraps/senderhelper"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

var simpleUDPAddr = &net.UDPAddr{IP: net.IPv4(1, 1, 1, 1), Port: 161}

type services struct {
	fx.In
	Sender    *mocksender.MockSender
	Formatter formatter.Component
	Listener  listener.MockComponent
	Forwarder forwarder.Component
}

func setUp(t *testing.T) *services {
	t.Helper()
	s := fxutil.Test[services](t,
		hostnameimpl.MockModule(),
		configimpl.MockModule,
		logimpl.MockModule(),
		senderhelper.Opts,
		formatterimpl.MockModule,
		listenerimpl.MockModule,
		Module,
	)
	return &s
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
	s := setUp(t)
	packet := makeSnmpPacket(packet.LinkDownv1GenericTrap)
	rawEvent, err := s.Formatter.FormatPacket(packet)
	require.NoError(t, err)
	s.Listener.Send(packet)
	time.Sleep(100 * time.Millisecond)
	s.Sender.AssertEventPlatformEvent(t, rawEvent, eventplatform.EventTypeSnmpTraps)
}

func TestV1SpecificTrapAreForwarder(t *testing.T) {
	s := setUp(t)
	packet := makeSnmpPacket(packet.AlarmActiveStatev1SpecificTrap)
	rawEvent, err := s.Formatter.FormatPacket(packet)
	require.NoError(t, err)
	s.Listener.Send(packet)
	time.Sleep(100 * time.Millisecond)
	s.Sender.AssertEventPlatformEvent(t, rawEvent, eventplatform.EventTypeSnmpTraps)
}
func TestV2TrapAreForwarder(t *testing.T) {
	s := setUp(t)
	packet := makeSnmpPacket(packet.NetSNMPExampleHeartbeatNotification)
	rawEvent, err := s.Formatter.FormatPacket(packet)
	require.NoError(t, err)
	s.Listener.Send(packet)
	time.Sleep(100 * time.Millisecond)
	s.Sender.AssertEventPlatformEvent(t, rawEvent, eventplatform.EventTypeSnmpTraps)
}

func TestForwarderTelemetry(t *testing.T) {
	s := setUp(t)
	s.Listener.Send(makeSnmpPacket(packet.NetSNMPExampleHeartbeatNotification))
	time.Sleep(100 * time.Millisecond)
	s.Sender.AssertMetric(t, "Count", "datadog.snmp_traps.forwarded", 1, "", []string{"snmp_device:1.1.1.1", "device_namespace:totoro", "snmp_version:2"})
}
