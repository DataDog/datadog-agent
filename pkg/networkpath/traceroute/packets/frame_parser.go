// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package packets

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"slices"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

// FrameParser parses traceroute responses using gopacket.
type FrameParser struct {
	IP4      layers.IPv4
	IP6      layers.IPv6
	TCP      layers.TCP
	ICMP4    layers.ICMPv4
	ICMP6    layers.ICMPv6
	Payload  gopacket.Payload
	Layers   []gopacket.LayerType
	parserv4 *gopacket.DecodingLayerParser
	parserv6 *gopacket.DecodingLayerParser
}

var ignoredLayerErr = &common.ReceiveProbeNoPktError{
	Err: fmt.Errorf("FrameParser saw an a layer type not used by traceroute (e.g. SCTP) and decided to ignore it"),
}

const expectedLayerCount = 2

// NewFrameParser constructs a new FrameParser
func NewFrameParser() *FrameParser {
	p := &FrameParser{}
	p.parserv4 = gopacket.NewDecodingLayerParser(layers.LayerTypeIPv4, &p.IP4, &p.TCP, &p.ICMP4, &p.Payload)
	// TODO: IPv6 is not actually implemented yet
	p.parserv6 = gopacket.NewDecodingLayerParser(layers.LayerTypeIPv6, &p.IP6, &p.TCP, &p.ICMP6, &p.Payload)

	return p
}

// Parse parses an ethernet packet
func (p *FrameParser) Parse(buffer []byte) error {
	parser, err := p.getParser(buffer)
	if err != nil {
		return err
	}
	// TODO: currently we don't support ipv6
	if parser == p.parserv6 {
		return ignoredLayerErr
	}
	err = parser.DecodeLayers(buffer, &p.Layers)
	var unsupportedErr gopacket.UnsupportedLayerType
	if errors.As(err, &unsupportedErr) {
		if len(p.Layers) < expectedLayerCount {
			// we saw some other protocol we don't care about, skip
			return ignoredLayerErr
		}
		// there are extra layers beyond TLS, ignore those too
		err = nil
	}
	if err != nil {
		return fmt.Errorf("Parse: %w", err)
	}
	if err := p.checkLayers(); err != nil {
		return err
	}
	return nil
}

// GetIPLayer gets the layer type of the IP layer (right now, only IPv4)
func (p *FrameParser) GetIPLayer() gopacket.LayerType {
	if len(p.Layers) < expectedLayerCount {
		return gopacket.LayerTypeZero
	}
	return p.Layers[0]
}

// GetTransportLayer gets the layer type of the transport layer (e.g. TCP, ICMP)
func (p *FrameParser) GetTransportLayer() gopacket.LayerType {
	if len(p.Layers) < expectedLayerCount {
		return gopacket.LayerTypeZero
	}
	return p.Layers[1]
}

// TODO IPv6
var ipLayers = []gopacket.LayerType{layers.LayerTypeIPv4}
var transportLayers = []gopacket.LayerType{layers.LayerTypeTCP, layers.LayerTypeUDP, layers.LayerTypeICMPv4}

// checkLayers sanity checks the layers of the parse.
func (p *FrameParser) checkLayers() error {
	if !slices.Contains(ipLayers, p.GetIPLayer()) {
		return fmt.Errorf("CheckLayers: first layer %s is not IP", p.GetIPLayer())
	}
	if !slices.Contains(transportLayers, p.GetTransportLayer()) {
		return fmt.Errorf("CheckLayers: second layer %s is not transport", p.GetTransportLayer())
	}
	return nil
}

// IPPair combines a source/dest IP into a struct
type IPPair struct {
	SrcAddr netip.Addr
	DstAddr netip.Addr
}

// Flipped returns an IPPair with the source/dest swapped
func (p IPPair) Flipped() IPPair {
	return IPPair{
		SrcAddr: p.DstAddr,
		DstAddr: p.SrcAddr,
	}
}

func getIPv4Pair(ip4 *layers.IPv4) IPPair {
	srcAddr, ok := netip.AddrFromSlice(ip4.SrcIP)
	if !ok {
		return IPPair{}
	}
	dstAddr, ok := netip.AddrFromSlice(ip4.DstIP)
	if !ok {
		return IPPair{}
	}
	return IPPair{srcAddr, dstAddr}
}

// GetIPPair gets the IPPair of the IP layer
func (p *FrameParser) GetIPPair() (IPPair, error) {
	switch p.GetIPLayer() {
	case layers.LayerTypeIPv4:
		return getIPv4Pair(&p.IP4), nil
	default:
		// TODO IPv6
		return IPPair{}, fmt.Errorf("GetIPPair: unexpected IP layer type %s", p.Layers[0])
	}
}

// TCPInfo is the info we get back from ICMP exceeded payload in a TCP probe.
type TCPInfo struct {
	SrcPort uint16
	DstPort uint16
	Seq     uint32
}

// ParseTCPFirstBytes parses the first 8 bytes an ICMP response is expected to have, as TCP
func ParseTCPFirstBytes(buffer []byte) (TCPInfo, error) {
	if len(buffer) < 8 {
		return TCPInfo{}, fmt.Errorf("ParseTCPFirstBytes: buffer too short (%d bytes)", len(buffer))
	}
	tcp := TCPInfo{
		SrcPort: binary.BigEndian.Uint16(buffer[0:2]),
		DstPort: binary.BigEndian.Uint16(buffer[2:4]),
		Seq:     binary.BigEndian.Uint32(buffer[4:8]),
	}
	return tcp, nil
}

// ICMPInfo encodes the information relevant to traceroutes from an ICMP response
type ICMPInfo struct {
	// IPPair is the source/dest IPs from the IP layer
	IPPair IPPair
	// ICMPType is the kind of ICMP packet (e.g. TTL exceeded)
	ICMPType layers.ICMPv4TypeCode
	// ICMPPair is the source/dest IPs from the wrapped IP payload
	ICMPPair IPPair
	// Payload is the payload from within the wrapped IP packet, typically containing the first 8 bytes of TCP/UDP.
	Payload []byte
}

// GetICMPInfo gets the ICMP details relevant to traceroutes from an ICMP response
func (p *FrameParser) GetICMPInfo() (ICMPInfo, error) {
	ipPair, err := p.GetIPPair()
	if err != nil {
		return ICMPInfo{}, err
	}
	switch p.GetTransportLayer() {
	case layers.LayerTypeICMPv4:
		var innerPkt layers.IPv4
		err = (&innerPkt).DecodeFromBytes(p.ICMP4.Payload, gopacket.NilDecodeFeedback)
		if err != nil {
			return ICMPInfo{}, fmt.Errorf("GetICMPInfo failed to decode inner packet: %w", err)
		}

		icmpInfo := ICMPInfo{
			IPPair:   ipPair,
			ICMPType: p.ICMP4.TypeCode,
			ICMPPair: getIPv4Pair(&innerPkt),
			Payload:  slices.Clone(innerPkt.Payload),
		}
		return icmpInfo, nil
	default:
		// TODO IPv6
		return ICMPInfo{}, fmt.Errorf("GetICMPInfo: unexpected layer type %s", p.Layers[1])
	}
}

// TTLExceeded4 is the TTL Exceeded ICMP4 TypeCode
var TTLExceeded4 = layers.CreateICMPv4TypeCode(layers.ICMPv4TypeTimeExceeded, layers.ICMPv4CodeTTLExceeded)

func (p *FrameParser) getParser(buffer []byte) (*gopacket.DecodingLayerParser, error) {
	if len(buffer) < 1 {
		return nil, fmt.Errorf("getParser: buffer was empty")
	}
	version := buffer[0] >> 4
	switch version {
	case 4:
		return p.parserv4, nil
	case 6:
		return p.parserv6, nil
	default:
		return nil, fmt.Errorf("unexpected IP version %d", version)
	}
}
