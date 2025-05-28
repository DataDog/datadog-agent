// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package icmp

import (
	"context"
	"fmt"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv6"
	"net"
	"net/netip"
	"os"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
)

const (
	ProtocolICMP = 1
)

type icmpResult struct {
	Hops      []*common.ProbeResponse
	LocalAddr net.IP
}

func RunICMPTraceroute(ctx context.Context, p Params) (*common.Results, error) {
	result, err := runICMPTraceroute(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("icmp traceroute failed: %w", err)
	}
	hops, err := common.ToHops(p.ParallelParams, result.Hops)
	if err != nil {
		return nil, fmt.Errorf("icmp traceroute ToHops failed: %w", err)
	}
	return &common.Results{
		Source: result.LocalAddr,
		Target: p.Target.AsSlice(),
		Hops:   hops,
	}, nil
}

func runICMPTraceroute(ctx context.Context, p Params) (*icmpResult, error) {
	if err := p.validate(); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	local, udpConn, err := common.LocalAddrForHost(p.Target.AsSlice(), 80)
	if err != nil {
		return nil, fmt.Errorf("failed to get local addr: %w", err)
	}
	udpConn.Close()

	deadline := time.Now().Add(p.Timeout())
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	driver, err := newICMPDriver(ctx, p, local.IP)
	if err != nil {
		return nil, fmt.Errorf("failed to init icmp driver: %w", err)
	}
	defer driver.Close()

	resp, err := common.TracerouteParallel(ctx, driver, p.ParallelParams)
	if err != nil {
		return nil, fmt.Errorf("icmp traceroute failed: %w", err)
	}

	return &icmpResult{Hops: resp, LocalAddr: local.IP}, nil
}

func newICMPDriver(ctx context.Context, p Params, laddr net.IP) (*ICMPTracerouteDriver, error) {
	var protocol string
	isIPV6 := false
	if laddr.To4() != nil {
		protocol = "ip4:icmp"
	} else {
		isIPV6 = true
		protocol = "ip6:ipv6-icmp"
	}
	conn, err := icmp.ListenPacket(protocol, laddr.String())
	if err != nil {
		return nil, err
	}
	return &ICMPTracerouteDriver{
		c:         conn,
		sendTimes: make([]time.Time, p.ParallelParams.MaxTTL+1),
		params:    p,
		echoID:    os.Getpid() & 0xffff,
		mu:        &sync.Mutex{},
		isIPV6:    isIPV6,
	}, nil
}

type ICMPTracerouteDriver struct {
	c         *icmp.PacketConn
	sendTimes []time.Time
	params    Params
	echoID    int
	mu        *sync.Mutex
	isIPV6    bool
}

func (d *ICMPTracerouteDriver) Close() error {
	return d.c.Close()
}

func (*ICMPTracerouteDriver) GetDriverInfo() common.TracerouteDriverInfo {
	return common.TracerouteDriverInfo{UsesReceiveICMPProbe: true}
}

func (d *ICMPTracerouteDriver) SendProbe(ttl uint8) error {
	if !d.sendTimes[ttl].IsZero() {
		return fmt.Errorf("probe already sent for TTL %d", ttl)
	}
	d.sendTimes[ttl] = time.Now()
	data, err := buildICMPEchoRequestPacket(d.echoID, ttl, d.isIPV6)
	if err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.isIPV6 {
		if err := d.c.IPv6PacketConn().SetHopLimit(int(ttl)); err != nil {
			return err
		}
	} else {
		if err := d.c.IPv4PacketConn().SetTTL(int(ttl)); err != nil {
			return err
		}
	}
	_, err = d.c.WriteTo(data, &net.IPAddr{IP: d.params.Target.AsSlice()})
	return err
}

func (d *ICMPTracerouteDriver) ReceiveProbe(timeout time.Duration) (*common.ProbeResponse, error) {
	return d.receiveProbe(timeout)
}

func (d *ICMPTracerouteDriver) ReceiveICMPProbe(timeout time.Duration) (*common.ProbeResponse, error) {
	return d.receiveProbe(timeout)
}

func (d *ICMPTracerouteDriver) receiveProbe(timeout time.Duration) (*common.ProbeResponse, error) {
	if err := d.c.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return nil, fmt.Errorf("set read deadline failed: %w", err)
	}
	buf := make([]byte, 1500)
	n, addr, err := d.c.ReadFrom(buf)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return nil, common.ErrReceiveProbeNoPkt
		}
		return nil, err
	}
	if n == 0 {
		return nil, common.ErrReceiveProbeNoPkt
	}
	return d.readAndParse(buf[:n], addr)
}

func (d *ICMPTracerouteDriver) readAndParse(buf []byte, addr net.Addr) (*common.ProbeResponse, error) {
	parser := common.NewICMPFrameParser()
	err := parser.ParseICMP(buf, d.isIPV6)
	if err != nil {
		return nil, err
	}
	var t uint8
	if d.isIPV6 {
		t = parser.ICMP6.TypeCode.Type()
	} else {
		t = parser.ICMP4.TypeCode.Type()
	}
	switch t {
	case layers.ICMPv4TypeTimeExceeded:
		return d.parseTimeExceeded(parser.Payload, addr)
	case layers.ICMPv6TypeTimeExceeded:
		return d.parseTimeExceeded(parser.ICMP6.Payload, addr)
	case layers.ICMPv4TypeEchoReply:
		if int(parser.ICMP4.Id) == d.echoID {
			return d.createResponse(addr, int(parser.ICMP4.Seq), true)
		}
		return nil, fmt.Errorf("invalid ICMP echo reply%s", t)
	case layers.ICMPv6TypeEchoReply:
		if int(parser.Echo.Identifier) == d.echoID {
			return d.createResponse(addr, int(parser.Echo.SeqNumber), true)
		}
		return nil, fmt.Errorf("invalid ICMPv6 echo reply%s", t)
	default:
		return nil, fmt.Errorf("unexpected ICMP layer %s", t)
	}
}

func (d *ICMPTracerouteDriver) parseTimeExceeded(payload []byte, addr net.Addr) (*common.ProbeResponse, error) {
	if d.isIPV6 {
		return d.parseTimeExceededICMPv6(payload, addr)
	}

	parser := common.NewFrameParser()
	if err := parser.ParseIPv4(payload); err != nil {
		return nil, fmt.Errorf("failed to decode layers: %v", err)
	}
	if int(parser.ICMP4.Id) != d.echoID {
		return nil, fmt.Errorf("mismatched echo ID")
	}
	seq := int(parser.ICMP4.Seq)
	return d.createResponse(addr, seq, false)
}

func (d *ICMPTracerouteDriver) parseTimeExceededICMPv6(payload []byte, addr net.Addr) (*common.ProbeResponse, error) {
	var embedded []byte
	switch {
	case len(payload) > 4 && payload[4]>>4 == 6:
		// Strip 4-byte prefix and parse IPv6 from offset 4
		embedded = payload[4:]

	case len(payload) > 8 && payload[0] == 0x03 && payload[1] == 0x00 && payload[8]>>4 == 6:
		// ICMPv6 Time Exceeded + embedded IPv6
		embedded = payload[8:]

	case len(payload) > 0 && payload[0]>>4 == 6:
		// Raw IPv6 packet (no prefix, no ICMPv6 wrapper)
		embedded = payload

	default:
		return nil, fmt.Errorf("Embedded packet is not a valid IPv6 header")
	}
	var (
		ip6  layers.IPv6
		icmp layers.ICMPv6
		echo layers.ICMPv6Echo
	)

	parser := gopacket.NewDecodingLayerParser(layers.LayerTypeIPv6, &ip6, &icmp, &echo)
	decoded := []gopacket.LayerType{}

	err := parser.DecodeLayers(embedded, &decoded)
	if err != nil {
		return nil, fmt.Errorf("Embedded decode error:", err)
	}
	return d.createResponse(addr, int(echo.SeqNumber), false)
}

func (d *ICMPTracerouteDriver) createResponse(addr net.Addr, seq int, isDest bool) (*common.ProbeResponse, error) {
	rtt, err := d.getRTTFromRelSeq(seq)
	if err != nil {
		return nil, err
	}
	ip, err := netipAddrFromNetAddr(addr)
	if err != nil {
		return nil, err
	}
	return &common.ProbeResponse{
		TTL:    uint8(seq),
		IP:     ip,
		RTT:    rtt,
		IsDest: isDest,
	}, nil
}

func (d *ICMPTracerouteDriver) getRTTFromRelSeq(seq int) (time.Duration, error) {
	if seq < int(d.params.ParallelParams.MinTTL) || seq > int(d.params.ParallelParams.MaxTTL) {
		return 0, fmt.Errorf("invalid sequence number %d", seq)
	}
	if d.sendTimes[seq].IsZero() {
		return 0, fmt.Errorf("no probe sent for sequence %d", seq)
	}
	return time.Since(d.sendTimes[seq]), nil
}

func netipAddrFromNetAddr(addr net.Addr) (netip.Addr, error) {
	var ip net.IP
	switch a := addr.(type) {
	case *net.IPAddr:
		ip = a.IP
	case *net.IPNet:
		ip = a.IP
	case *net.TCPAddr:
		ip = a.IP
	case *net.UDPAddr:
		ip = a.IP
	default:
		return netip.Addr{}, fmt.Errorf("unsupported address type %T", addr)
	}
	netipAddr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return netip.Addr{}, fmt.Errorf("invalid IP from %q", ip)
	}
	return netipAddr, nil
}

func buildICMPEchoRequestPacket(id int, ttl uint8, ipv6 bool) ([]byte, error) {
	if ipv6 {
		return buildICMPEchoRequestPacketv6(id, ttl)
	}
	return buildICMPEchoRequestPacketV4(id, ttl)
}

func buildICMPEchoRequestPacketV4(id int, ttl uint8) ([]byte, error) {
	buffer := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		ComputeChecksums: true,
		FixLengths:       true,
	}
	icmpEchoRequestLayer := layers.ICMPv4{
		TypeCode: layers.CreateICMPv4TypeCode(layers.ICMPv4TypeEchoRequest, 0),
		Id:       uint16(id),
		Seq:      uint16(ttl),
	}
	if err := gopacket.SerializeLayers(buffer, opts, &icmpEchoRequestLayer, gopacket.Payload("datadog-traceroute ping")); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
func buildICMPEchoRequestPacketv6(id int, ttl uint8) ([]byte, error) {
	msg := icmp.Message{
		Type: ipv6.ICMPTypeEchoRequest,
		Code: 0,
		Body: &icmp.Echo{
			ID:   id,
			Seq:  int(ttl),
			Data: []byte("datadog-traceroute ping"),
		},
	}
	b, err := msg.Marshal(nil)
	if err != nil {
		return nil, err
	}
	return b, nil
}
