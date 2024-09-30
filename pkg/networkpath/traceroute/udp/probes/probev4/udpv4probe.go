/* SPDX-License-Identifier: BSD-2-Clause */

package probev4

import (
	"errors"
	"fmt"
	"net"
	"time"

	inet "github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/udp/net"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/udp/probes"
	"golang.org/x/net/ipv4"
)

// ProbeUDPv4 represents a sent probe packet with its metadata
type ProbeUDPv4 struct {
	Data    []byte
	ip      *ipv4.Header
	udp     *inet.UDP
	payload []byte
	// time the packet is sent at
	Timestamp time.Time
	// local address of the packet sender
	LocalAddr net.IP
}

// Validate verifies that the probe has the expected structure, and returns an error if not
func (p *ProbeUDPv4) Validate() error {
	if p.ip == nil {
		// decode packet
		hdr, err := ipv4.ParseHeader(p.Data)
		if err != nil {
			return err
		}
		p.ip = hdr
	}
	if p.ip.Protocol != int(inet.ProtoUDP) {
		return fmt.Errorf("IP payload is not UDP, expected type %d, got %d", inet.ProtoUDP, p.ip.Protocol)
	}
	p.payload = p.Data[p.ip.Len:]
	if len(p.payload) == 0 {
		return errors.New("IP layer has no payload")
	}
	udp, err := inet.NewUDP(p.Data[p.ip.Len:])
	if err != nil {
		return fmt.Errorf("failed to parse UDP header: %w", err)
	}
	p.udp = udp
	p.payload = p.Data[p.ip.Len+inet.UDPHeaderLen:]
	return nil
}

// IP returns the IP header of the probe. If not decoded yet, will return nil.
func (p ProbeUDPv4) IP() *ipv4.Header {
	return p.ip
}

// UDP returns the payload of the IP header of the probe. If not decoded yet,
// will return nil.
func (p ProbeUDPv4) UDP() *inet.UDP {
	return p.udp
}

// ProbeResponseUDPv4 represents a received probe response with its metadata
type ProbeResponseUDPv4 struct {
	// header, payload and timestamp are expected to be passed at object creation
	Header *ipv4.Header
	// the IPv4 payload (expected ICMP -> IP -> UDP)
	Payload []byte
	// Addr is the IP address of the response sender
	Addr net.IP
	// time the packet is received at
	Timestamp time.Time

	// the following are computed, internal fields instead
	icmp         *inet.ICMP
	innerIP      *ipv4.Header
	innerUDP     *inet.UDP
	innerPayload []byte
}

// Validate verifies that the probe response has the expected structure, and returns an error if not
func (pr *ProbeResponseUDPv4) Validate() error {
	if pr.icmp == nil {
		// decode packet
		icmp, err := inet.NewICMP(pr.Payload)
		if err != nil {
			return err
		}
		pr.icmp = icmp
	}
	if len(pr.icmp.Payload) == 0 {
		return errors.New("ICMP layer has no payload")
	}
	ip, err := ipv4.ParseHeader(pr.icmp.Payload)
	if err != nil {
		return fmt.Errorf("failed to parse inner IPv4 header: %w", err)
	}
	pr.innerIP = ip
	payload := pr.icmp.Payload[ip.Len:]
	if len(payload) == 0 {
		return errors.New("inner IP layer has no payload")
	}
	if ip.Protocol != int(inet.ProtoUDP) {
		return fmt.Errorf("inner IP payload is not UDP, want protocol %d, got %d", inet.ProtoUDP, ip.Protocol)
	}
	udp, err := inet.NewUDP(payload)
	if err != nil {
		return fmt.Errorf("failed to decode inner UDP header: %w", err)
	}
	pr.innerUDP = udp
	pr.innerPayload = payload[inet.UDPHeaderLen:]
	return nil
}

// ICMP returns the ICMP layer of the probe response. If not decoded yet, will return nil.
func (pr *ProbeResponseUDPv4) ICMP() *inet.ICMP {
	return pr.icmp
}

// InnerIP returns the inner IP layer of the probe response. If not decoded yet, will return nil.
func (pr *ProbeResponseUDPv4) InnerIP() *ipv4.Header {
	return pr.innerIP
}

// InnerUDP returns the UDP layer of the probe. If not decoded yet, will return nil.
func (pr *ProbeResponseUDPv4) InnerUDP() *inet.UDP {
	return pr.innerUDP
}

// Matches returns true if this probe response matches the given probe. Both
// probes must have been already validated with Validate, this function may
// panic otherwise.
func (pr *ProbeResponseUDPv4) Matches(pi probes.Probe) bool {
	p := pi.(*ProbeUDPv4)
	if p == nil {
		return false
	}
	if pr.icmp.Type != inet.ICMPTimeExceeded && !(pr.icmp.Type == inet.ICMPDestUnreachable && pr.icmp.Code == 3) {
		// we want time-exceeded or port-unreachable
		return false
	}
	if !pr.InnerIP().Dst.To4().Equal(p.ip.Dst.To4()) {
		return false
	}
	if p.UDP().Src != pr.InnerUDP().Src || p.UDP().Dst != pr.InnerUDP().Dst {
		// source and destination ports do not match
		return false
	}
	if pr.InnerIP().ID != p.ip.ID {
		// the two packets do not belong to the same flow
		return false
	}
	return true
}
