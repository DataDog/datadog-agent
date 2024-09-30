/* SPDX-License-Identifier: BSD-2-Clause */

package probev6

import (
	"fmt"
	"net"
	"time"

	inet "github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/udp/net"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/udp/probes"
	"golang.org/x/net/ipv6"
)

// ProbeUDPv6 represents a sent probe packet with its metadata
type ProbeUDPv6 struct {
	// Payload of the sent IPv6 packet
	Payload []byte
	// HopLimit value when the packet was sent
	HopLimit int
	// time the packet is set at
	Timestamp time.Time
	// local address of the packet sender
	LocalAddr, RemoteAddr net.IP
	// internal fields
	udp *inet.UDP
}

// Validate verifies that the probe has the expected structure, and returns an error if not
func (p *ProbeUDPv6) Validate() error {
	if p.udp == nil {
		// decode packet
		udp, err := inet.NewUDP(p.Payload)
		if err != nil {
			return err
		}
		p.udp = udp
	}
	return nil
}

// UDP returns the UDP layer of the probe. If not decoded yet, will return nil.
func (p ProbeUDPv6) UDP() *inet.UDP {
	return p.udp
}

// ProbeResponseUDPv6 represents a received probe response with its metadata
type ProbeResponseUDPv6 struct {
	// payload of the received IPv6 packet (expected ICMPv6 -> IPv6 -> UDP)
	Data []byte
	// time the packet is received at
	Timestamp time.Time
	// sender IP address
	Addr net.IP
	// internal fields
	icmp      *inet.ICMPv6
	innerIPv6 *ipv6.Header
	innerUDP  *inet.UDP
	payload   []byte
}

// Validate verifies that the probe response has the expected structure, and
// returns an error if not
func (pr *ProbeResponseUDPv6) Validate() error {
	if pr.icmp != nil && pr.innerIPv6 != nil && pr.innerUDP != nil {
		return nil
	}
	// decode ICMPv6 layer
	icmp, err := inet.NewICMPv6(pr.Data)
	if err != nil {
		return fmt.Errorf("failed to decode ICMPv6: %w", err)
	}
	pr.icmp = icmp
	// decode inner IPv6 layer
	ip, err := ipv6.ParseHeader(pr.Data[inet.ICMPv6HeaderLen:])
	if err != nil {
		return fmt.Errorf("failed to decode inner IPv6: %w", err)
	}
	pr.innerIPv6 = ip
	// decode inner UDP layer
	udp, err := inet.NewUDP(pr.Data[inet.ICMPv6HeaderLen+inet.IPv6HeaderLen:])
	if err != nil {
		return fmt.Errorf("failed to decode inner UDP: %w", err)
	}
	pr.innerUDP = udp
	pr.payload = pr.Data[inet.ICMPv6HeaderLen+inet.IPv6HeaderLen+inet.UDPHeaderLen:]
	return nil
}

// Matches returns true if this probe response matches the given probe. Both
// probes must have been already validated with Validate, this function may
// panic otherwise.
func (pr ProbeResponseUDPv6) Matches(pi probes.Probe) bool {
	p, ok := pi.(*ProbeUDPv6)
	if !ok || p == nil {
		return false
	}
	icmp := pr.ICMPv6()
	if icmp.Type != inet.ICMPv6TypeTimeExceeded &&
		!(icmp.Type == inet.ICMPv6TypeDestUnreachable && icmp.Code == inet.ICMPv6CodePortUnreachable) {
		// we want time-exceeded or port-unreachable
		return false
	}
	// TODO check that To16() is the right thing to call here
	if !pr.InnerIPv6().Dst.To16().Equal(p.RemoteAddr.To16()) {
		// this is not a response to any of our probes, discard it
		return false
	}
	innerUDP := pr.InnerUDP()
	if p.UDP().Dst != innerUDP.Dst {
		// this is not our packet
		return false
	}
	if pr.InnerIPv6().PayloadLen != len(p.Payload)+inet.UDPHeaderLen {
		// different payload length, not our packet
		// NOTE: here I am using pr.InnerIPv6().PayloadLen instead of len(pr.payload)
		// because the responding hop might use an RFC4884 multi-part ICMPv6 message,
		// which has extra data at the end of time-exceeded and destination-unreachable
		// messages
		return false
	}
	return true
}

// ICMPv6 returns the ICMPv6 layer of the probe.
func (pr *ProbeResponseUDPv6) ICMPv6() *inet.ICMPv6 {
	return pr.icmp
}

// InnerIPv6 returns the IP layer of the inner packet of the probe.
func (pr *ProbeResponseUDPv6) InnerIPv6() *ipv6.Header {
	return pr.innerIPv6
}

// InnerUDP returns the UDP layer of the inner packet of the probe.
func (pr *ProbeResponseUDPv6) InnerUDP() *inet.UDP {
	return pr.innerUDP
}

// InnerPayload returns the payload of the inner UDP packet
func (pr *ProbeResponseUDPv6) InnerPayload() []byte {
	return pr.payload
}
