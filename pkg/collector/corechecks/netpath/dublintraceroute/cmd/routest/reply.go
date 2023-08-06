/* SPDX-License-Identifier: BSD-2-Clause */

package main

import (
	"errors"
	"fmt"

	inet "github.com/DataDog/datadog-agent/pkg/collector/corechecks/netpath/dublintraceroute/net"
	"golang.org/x/net/ipv4"
)

// ErrNoMatch signals a packet not matching the desired criteria.
var ErrNoMatch = errors.New("packet not matching")

// forgeReplyv4 forges a reply for the provided input, assuming that this
// is UDP over IPv4.
// If the packet doesn't match in the configuration, an ErrNoMatch is
// returned.
// The function returns the IPv4 header and its serialized payload.
func forgeReplyv4(cfg *Config, payload []byte) (*ipv4.Header, []byte, error) {
	p, err := ipv4.ParseHeader(payload)
	if err != nil {
		return nil, nil, err
	}
	udp, err := inet.NewUDP(payload[p.Len:])
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse UDP header: %w", err)
	}
	log.Debugf("Matching packet: %+v >> %+v", p, udp)
	var match *Probe
	for _, c := range *cfg {
		if p.Dst.Equal(c.Dst) &&
			(c.Src == nil || p.Src.Equal(*c.Src)) &&
			int(c.TTL) == p.TTL &&
			c.DstPort == udp.Dst &&
			(c.SrcPort == nil || *c.SrcPort == udp.Src) {
			match = &c
			break
		}
	}
	if match == nil {
		return nil, nil, ErrNoMatch
	}
	log.Debugf("Found match %+v", *match)
	dst := p.Src
	if match.Reply.Dst != nil {
		dst = *match.Reply.Dst
	}
	ip := ipv4.Header{
		Version:  4,
		Len:      ipv4.HeaderLen,
		TotalLen: ipv4.HeaderLen + inet.UDPHeaderLen + len(payload),
		TTL:      64, // dummy value, good enough for a reply
		Protocol: int(inet.ProtoICMP),
		Src:      match.Reply.Src,
		Dst:      dst,
	}
	if match.Reply.Payload != nil {
		payload = match.Reply.Payload
	}
	icmp := inet.ICMP{
		Type:    inet.ICMPType(match.Reply.IcmpType),
		Code:    inet.ICMPCode(match.Reply.IcmpCode),
		Payload: payload,
	}
	icmpBytes, err := icmp.MarshalBinary()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to serialize ICMPv4: %w", err)
	}
	return &ip, icmpBytes, nil
}
