// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package connection

import (
	"net"
	"testing"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/filter"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/nstat"
)

func TestDarwinPacketSidecarEnrichesWithoutOwningCountersOrLifecycle(t *testing.T) {
	primary := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	primary.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 1,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      testNStatTCPFlow(4242, tcpStateEstablished),
		Counts: &nstat.Counts{
			RXBytes:          1000,
			TXBytes:          2000,
			ConnectAttempts:  1,
			ConnectSuccesses: 1,
		},
	})

	source := &fakeDarwinPacketSource{
		packets: []fakeDarwinPacket{
			{data: serializeDarwinTCPPacket(t, 100, true, false, false, true, nil), outgoing: true},
			{data: serializeDarwinTCPPacket(t, 101, false, true, false, true, []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")), outgoing: true},
			{data: serializeDarwinTCPPacket(t, 101, false, true, false, true, []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")), outgoing: true},
			{data: serializeDarwinTCPPacket(t, 200, false, true, true, false, nil)},
		},
	}
	sidecar := newDarwinPacketSidecar(source, primary, 10)
	require.NoError(t, validateDarwinPacketSidecar(sidecar))
	require.NoError(t, sidecar.visitPackets())

	buffer := network.NewConnectionBuffer(1, 1)
	require.NoError(t, primary.GetConnections(buffer, nil))
	require.Len(t, buffer.Connections(), 1)
	conn := buffer.Connections()[0]
	require.Equal(t, uint64(1000), conn.Monotonic.RecvBytes)
	require.Equal(t, uint64(2000), conn.Monotonic.SentBytes)
	require.Equal(t, uint32(1), conn.Monotonic.Retransmits)
	require.Equal(t, network.OUTGOING, conn.Direction)
	require.Equal(t, directionEvidencePacket, primary.sources[1].directionEvidence)
	require.Equal(t, protocols.HTTP, conn.ProtocolStack.Application)
	require.Equal(t, uint32(1), conn.TCPFailures[network.TCPFailureErrnoConnReset])
	require.False(t, conn.IsClosed)
	require.Zero(t, conn.Monotonic.TCPClosed)

	conn.TCPFailures[network.TCPFailureErrnoConnReset] = 99
	buffer.Reset()
	require.NoError(t, primary.GetConnections(buffer, nil))
	require.Equal(t, uint32(1), buffer.Connections()[0].TCPFailures[network.TCPFailureErrnoConnReset])
}

type fakeDarwinPacket struct {
	data     []byte
	outgoing bool
}

type fakeDarwinPacketSource struct {
	packets []fakeDarwinPacket
}

func (f *fakeDarwinPacketSource) VisitPackets(visitor func([]byte, filter.PacketInfo, time.Time) error) error {
	for _, packet := range f.packets {
		info := &fakeDarwinPacketInfo{
			packetType: filter.PacketHost,
			layerType:  layers.LayerTypeEthernet,
			original:   len(packet.data),
			captured:   len(packet.data),
		}
		if packet.outgoing {
			info.packetType = filter.PacketOutgoing
		}
		if err := visitor(packet.data, info, time.Now()); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeDarwinPacketSource) LayerType() gopacket.LayerType {
	return layers.LayerTypeEthernet
}

func (f *fakeDarwinPacketSource) Close() {}

type fakeDarwinPacketInfo struct {
	packetType uint8
	layerType  gopacket.LayerType
	original   int
	captured   int
}

func (i *fakeDarwinPacketInfo) PacketType() uint8                 { return i.packetType }
func (i *fakeDarwinPacketInfo) LinkLayerType() gopacket.LayerType { return i.layerType }
func (i *fakeDarwinPacketInfo) OriginalLength() int               { return i.original }
func (i *fakeDarwinPacketInfo) CapturedLength() int               { return i.captured }

func serializeDarwinTCPPacket(t *testing.T, seq uint32, syn, ack, rst, outgoing bool, payload []byte) []byte {
	t.Helper()
	ethernet := &layers.Ethernet{
		SrcMAC:       net.HardwareAddr{0, 1, 2, 3, 4, 5},
		DstMAC:       net.HardwareAddr{6, 7, 8, 9, 10, 11},
		EthernetType: layers.EthernetTypeIPv4,
	}
	sourceIP := net.ParseIP("192.0.2.10")
	destIP := net.ParseIP("198.51.100.20")
	sourcePort := layers.TCPPort(50000)
	destPort := layers.TCPPort(443)
	if !outgoing {
		sourceIP, destIP = destIP, sourceIP
		sourcePort, destPort = destPort, sourcePort
	}
	ip := &layers.IPv4{
		Version:  4,
		TTL:      64,
		Protocol: layers.IPProtocolTCP,
		SrcIP:    sourceIP,
		DstIP:    destIP,
	}
	tcp := &layers.TCP{
		SrcPort: sourcePort,
		DstPort: destPort,
		Seq:     seq,
		SYN:     syn,
		ACK:     ack,
		RST:     rst,
	}
	require.NoError(t, tcp.SetNetworkLayerForChecksum(ip))
	buffer := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(
		buffer,
		gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true},
		ethernet,
		ip,
		tcp,
		gopacket.Payload(payload),
	))
	return buffer.Bytes()
}
