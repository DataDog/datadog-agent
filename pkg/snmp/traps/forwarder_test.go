// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package traps

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"net"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type DummyFormatter struct{}

var simpleUDPAddr = &net.UDPAddr{IP: net.IPv4(1, 1, 1, 1), Port: 161}

// FormatPacket is a dummy formatter method that hashes an SnmpPacket object
func (f DummyFormatter) FormatPacket(packet *SnmpPacket) ([]byte, error) {
	var b bytes.Buffer
	gob.NewEncoder(&b).Encode(packet.Addr)
	gob.NewEncoder(&b).Encode(packet.Content.Community)
	gob.NewEncoder(&b).Encode(packet.Content.SnmpTrap)
	gob.NewEncoder(&b).Encode(packet.Content.Variables)
	gob.NewEncoder(&b).Encode(packet.Content.Version)

	h := sha256.New()
	h.Write(b.Bytes())
	hexHash := hex.EncodeToString(h.Sum(nil))
	return []byte(hexHash), nil
}

func createForwarder(t *testing.T) (forwarder *TrapForwarder, err error) {
	logger := fxutil.Test[log.Component](t, log.MockModule)
	packetsIn := make(PacketsChannel)
	mockSender := mocksender.NewMockSender("snmp-traps-listener")
	mockSender.SetupAcceptAll()

	forwarder, err = NewTrapForwarder(&DummyFormatter{}, mockSender, packetsIn, logger)
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
	sender.AssertEventPlatformEvent(t, []byte("0dee7422f503d972db97b711e39a5003d1995c0d2f718542813acc4c46053ef0"), epforwarder.EventTypeSnmpTraps)
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
