// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package sack

import (
	"encoding/binary"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/packets"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

type sackTCPState struct {
	localInitSeq uint32
	localInitAck uint32

	hasTS   bool
	tsValue uint32
	tsEcr   uint32
}

type sackPacketGen struct {
	ipPair packets.IPPair
	sPort  uint16
	dPort  uint16

	state sackTCPState
}

func (s *sackPacketGen) generateTSOption(ttl uint8) []layers.TCPOption {
	if !s.state.hasTS {
		return nil
	}

	timestamps := make([]byte, 8)
	binary.BigEndian.PutUint32(timestamps, s.state.tsValue+uint32(ttl))
	binary.BigEndian.PutUint32(timestamps[4:], s.state.tsEcr)
	return []layers.TCPOption{
		{
			// timestamps: 8+2 bytes
			OptionType: layers.TCPOptionKindTimestamps,
			OptionData: timestamps,
		}, {
			// now we have 10 bytes, need two NOPs to align to 32 bits
			OptionType: layers.TCPOptionKindNop,
		}, {
			OptionType: layers.TCPOptionKindNop,
		},
	}
}

func (s *sackPacketGen) generatePacketV4(ttl uint8) (*layers.IPv4, *layers.TCP, error) {
	ipLayer := &layers.IPv4{
		Version:  4,
		TTL:      ttl,
		SrcIP:    s.ipPair.SrcAddr.AsSlice(),
		DstIP:    s.ipPair.DstAddr.AsSlice(),
		Id:       41821,
		Protocol: layers.IPProtocolTCP,
	}
	tcpLayer := &layers.TCP{
		SrcPort: layers.TCPPort(s.sPort),
		DstPort: layers.TCPPort(s.dPort),
		ACK:     true,
		PSH:     true,
		Seq:     s.state.localInitSeq + uint32(ttl),
		Ack:     s.state.localInitAck,
		Window:  1024,
		Options: s.generateTSOption(ttl),
	}
	err := tcpLayer.SetNetworkLayerForChecksum(ipLayer)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to set network layer for checksum: %w", err)
	}
	return ipLayer, tcpLayer, nil
}

func (s *sackPacketGen) generateV4(ttl uint8) ([]byte, error) {
	ip4, tcp, err := s.generatePacketV4(ttl)
	if err != nil {
		return nil, fmt.Errorf("failed to generate packet: %w", err)
	}

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}
	err = gopacket.SerializeLayers(buf, opts, ip4, tcp, gopacket.Payload([]byte{ttl}))
	if err != nil {
		return nil, fmt.Errorf("failed to serialize packet: %w", err)
	}
	return buf.Bytes(), nil
}
