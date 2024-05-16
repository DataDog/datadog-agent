// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package ebpfless

import (
	"fmt"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"

	"github.com/DataDog/datadog-agent/pkg/network"
)

type UDPConn = conn[*layers.UDP, *udpLayer]
type TCPConn = conn[*layers.TCP, *tcpLayer]

func NewUDPConn(c *network.ConnectionStats, ip4 *layers.IPv4, ip6 *layers.IPv6, udp *layers.UDP) *UDPConn {
	layer := &udpLayer{UDP: udp}
	switch c.Family {
	case network.AFINET:
		layer.ipl = &ip4Layer{IPv4: ip4}
	case network.AFINET6:
		layer.ipl = &ip6Layer{IPv6: ip6}
	}

	return &UDPConn{
		ConnectionStats: c,
		layer:           layer,
	}
}

func NewTCPConn(c *network.ConnectionStats, ip4 *layers.IPv4, ip6 *layers.IPv6, tcp *layers.TCP) *TCPConn {
	layer := &tcpLayer{TCP: tcp}
	switch c.Family {
	case network.AFINET:
		layer.ipl = &ip4Layer{IPv4: ip4}
	case network.AFINET6:
		layer.ipl = &ip6Layer{IPv6: ip6}
	}
	return &TCPConn{
		ConnectionStats: c,
		layer:           &tcpLayer{TCP: tcp},
	}
}

type layer[G gopacket.Layer] interface {
	PacketLayer() G
	PayloadLen() (uint16, error)
}

type conn[G gopacket.Layer, L layer[G]] struct {
	*network.ConnectionStats
	layer layer[G]
}

type ipLayer interface {
	PayloadLen() (uint16, error)
}

type ip4Layer struct {
	*layers.IPv4
}

func (ip4 *ip4Layer) PayloadLen() (uint16, error) {
	return ip4.Length - uint16(ip4.IHL)*4, nil
}

type ip6Layer struct {
	*layers.IPv6
	ipl ipLayer
}

func (ip6 *ip6Layer) PayloadLen() (uint16, error) {
	if ip6.NextHeader == layers.IPProtocolUDP || ip6.NextHeader == layers.IPProtocolTCP {
		return ip6.Length, nil
	}

	var ipExt layers.IPv6ExtensionSkipper
	parser := gopacket.NewDecodingLayerParser(gopacket.LayerTypePayload, &ipExt)
	decoded := make([]gopacket.LayerType, 0, 1)
	l := ip6.Length
	payload := ip6.Payload
	for len(payload) > 0 {
		err := parser.DecodeLayers(payload, &decoded)
		if err != nil {
			return 0, fmt.Errorf("error decoding with ipv6 extension skipper: %w", err)
		}

		if len(decoded) == 0 {
			return l, nil
		}

		l -= uint16(len(ipExt.Contents))
		if ipExt.NextHeader == layers.IPProtocolTCP || ipExt.NextHeader == layers.IPProtocolUDP {
			break
		}

		payload = ipExt.Payload
	}

	return l, nil
}

type udpLayer struct {
	*layers.UDP
	ipl ipLayer
}

func (u *udpLayer) PacketLayer() *layers.UDP {
	return u.UDP
}

func (u *udpLayer) PayloadLen() (uint16, error) {
	if u.Length == 0 {
		return 0, fmt.Errorf("UDP length no specified")
	}

	return u.Length - 8, nil
}

type tcpLayer struct {
	*layers.TCP
	ipl ipLayer
}

func (t *tcpLayer) PacketLayer() *layers.TCP {
	return t.TCP
}

func (t *tcpLayer) PayloadLen() (uint16, error) {
	l, err := t.ipl.PayloadLen()
	if err != nil {
		return 0, err
	}

	return l - uint16(t.DataOffset)*4, nil
}

func (c *conn[G, L]) Layer() G {
	return c.layer.PacketLayer()
}

func (c *conn[G, L]) Stats() *network.ConnectionStats {
	return c.ConnectionStats
}

func (c *conn[G, L]) PayloadLen() (uint16, error) {
	return c.layer.PayloadLen()
}
