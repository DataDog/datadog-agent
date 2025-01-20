// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package udp

import (
	"fmt"
	"net"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"golang.org/x/net/ipv4"
)

func createRawUDPBuffer(sourceIP net.IP, sourcePort uint16, destIP net.IP, destPort uint16, ttl int) (*ipv4.Header, []byte, uint16, int, error) {
	ipLayer := &layers.IPv4{
		Version:  4,
		Length:   20,
		TTL:      uint8(ttl),
		Id:       uint16(41821),
		Protocol: 17, // hard code UDP so other OSs can use it
		DstIP:    destIP,
		SrcIP:    sourceIP,
		Flags:    layers.IPv4DontFragment, // needed for dublin-traceroute-like NAT detection
	}
	udpLayer := &layers.UDP{
		SrcPort: layers.UDPPort(sourcePort),
		DstPort: layers.UDPPort(destPort),
	}
	udpPaylod := []byte("NSMNC\x00\x00\x00")

	// TODO: compute checksum before serialization so we
	// can set ID field of the IP header to detect NATs just
	// as is done in dublin-traceroute. Gopacket doesn't expose
	// the checksum computations and modifying the IP header after
	// serialization would change its checksum
	err := udpLayer.SetNetworkLayerForChecksum(ipLayer)
	if err != nil {
		return nil, nil, 0, 0, fmt.Errorf("failed to create packet checksum: %w", err)
	}
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	err = gopacket.SerializeLayers(buf, opts,
		ipLayer,
		udpLayer,
		gopacket.Payload(udpPaylod),
	)
	if err != nil {
		return nil, nil, 0, 0, fmt.Errorf("failed to serialize packet: %w", err)
	}
	packet := buf.Bytes()

	var ipHdr ipv4.Header
	if err := ipHdr.Parse(packet[:20]); err != nil {
		return nil, nil, 0, 0, fmt.Errorf("failed to parse IP header: %w", err)
	}

	return &ipHdr, packet, udpLayer.Checksum, 20, nil
}
