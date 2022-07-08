package traps

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"net"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/require"
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
	packetsIn := make(PacketsChannel)
	mockSender := mocksender.NewMockSender("snmp-traps-listener")
	mockSender.SetupAcceptAll()
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
	return &SnmpPacket{gosnmpPacket, simpleUDPAddr, time.Now().UnixMilli()}
}

func TestV1GenericTrapAreForwarder(t *testing.T) {
	forwarder, err := createForwarder(t)
	require.NoError(t, err)
	sender, ok := forwarder.sender.(*mocksender.MockSender)
	require.True(t, ok)
	forwarder.trapsIn <- makeSnmpPacket(LinkDownv1GenericTrap)
	forwarder.Stop()
	sender.AssertEventPlatformEvent(t, "0db8c621a456b368b4af7570211b94769376541120b4110ef08c50226fcc63b4", epforwarder.EventTypeSnmpTraps)
}

func TestV1SpecificTrapAreForwarder(t *testing.T) {
	forwarder, err := createForwarder(t)
	require.NoError(t, err)
	sender, ok := forwarder.sender.(*mocksender.MockSender)
	require.True(t, ok)
	forwarder.trapsIn <- makeSnmpPacket(AlarmActiveStatev1SpecificTrap)
	forwarder.Stop()
	sender.AssertEventPlatformEvent(t, "5605a8dd09e575df722273edc9fae2c47e4a17c9d6844ff856ad0cd5913da04d", epforwarder.EventTypeSnmpTraps)
}
func TestV2TrapAreForwarder(t *testing.T) {
	forwarder, err := createForwarder(t)
	require.NoError(t, err)
	sender, ok := forwarder.sender.(*mocksender.MockSender)
	require.True(t, ok)
	forwarder.trapsIn <- makeSnmpPacket(NetSNMPExampleHeartbeatNotification)
	forwarder.Stop()
	sender.AssertEventPlatformEvent(t, "0dee7422f503d972db97b711e39a5003d1995c0d2f718542813acc4c46053ef0", epforwarder.EventTypeSnmpTraps)
}
