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
	"encoding/json"
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

var MockTime = func() time.Time {
	layout := "2006-01-02 15:04:05"
	str := "2000-01-01 00:00:00"
	t, _ := time.Parse(layout, str)
	return t
}

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
	packetsIn := make(PacketsChannel)
	mockSender := mocksender.NewMockSender("snmp-traps-listener")
	mockSender.SetupAcceptAll()

	config := Config{Port: serverPort, CommunityStrings: []string{"public"}, Namespace: "default"}
	Configure(t, config)

	oidResolver, err := NewMultiFilesOIDResolver()
	if err != nil {
		return nil, err
	}

	formatter, err := NewJSONFormatter(oidResolver, mockSender)
	if err != nil {
		return nil, err
	}
	forwarder, err = NewTrapForwarder(formatter, mockSender, packetsIn)
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
	return &SnmpPacket{gosnmpPacket, simpleUDPAddr, "totoro", MockTime().UnixMilli()}
}

func getMockEvent() ([]byte, error) {
	expectedEvent := `
{
    "trap": {
        "ddsource": "snmp-traps",
        "ddtags": "snmp_version:2,device_namespace:totoro,snmp_device:1.1.1.1",
        "netSnmpExampleHeartbeatRate": 1024,
        "snmpTrapMIB": "NET-SNMP-EXAMPLES-MIB",
        "snmpTrapName": "netSnmpExampleHeartbeatNotification",
        "snmpTrapOID": "1.3.6.1.4.1.8072.2.3.0.1",
        "timestamp": 946684800000,
        "uptime": 1000,
        "variables": [
            {
                "oid": "1.3.6.1.4.1.8072.2.3.2.1",
                "type": "integer",
                "value": 1024
            },
            {
                "oid": "1.3.6.1.4.1.8072.2.3.2.2",
                "type": "string",
                "value": "test"
            }
        ]
    }
}`
	expectedEventCompact := new(bytes.Buffer)
	err := json.Compact(expectedEventCompact, []byte(expectedEvent))
	if err != nil {
		return nil, err
	}
	return expectedEventCompact.Bytes(), nil
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
	expectedEvent, err := getMockEvent()
	require.NoError(t, err)
	sender.AssertEventPlatformEvent(t, expectedEvent, epforwarder.EventTypeSnmpTraps)
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
