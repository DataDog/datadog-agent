// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package nameresolver implements hostname, port, protocol, and address
// resolution for BPF filter compilation. It is a pure Go replacement for
// libpcap's nametoaddr.c.
package nameresolver

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

// Resolver implements name resolution using Go's net package and static tables.
type Resolver struct{}

// New creates a new Resolver.
func New() *Resolver {
	return &Resolver{}
}

// LookupHost resolves a hostname to a list of IPv4 addresses (as uint32 in network byte order).
func (r *Resolver) LookupHost(name string) ([]uint32, error) {
	// Try parsing as numeric IP first
	if ip := net.ParseIP(name); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			return []uint32{binary.BigEndian.Uint32(v4)}, nil
		}
		return nil, fmt.Errorf("not an IPv4 address: %s", name)
	}

	ips, err := net.LookupIP(name)
	if err != nil {
		return nil, fmt.Errorf("unknown host: %s", name)
	}

	var addrs []uint32
	for _, ip := range ips {
		if v4 := ip.To4(); v4 != nil {
			addrs = append(addrs, binary.BigEndian.Uint32(v4))
		}
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("no IPv4 addresses for host: %s", name)
	}
	return addrs, nil
}

// LookupHost6 resolves a hostname to a list of IPv6 addresses.
func (r *Resolver) LookupHost6(name string) ([][16]byte, error) {
	if ip := net.ParseIP(name); ip != nil {
		if v6 := ip.To16(); v6 != nil && ip.To4() == nil {
			var addr [16]byte
			copy(addr[:], v6)
			return [][16]byte{addr}, nil
		}
		if ip.To4() != nil {
			return nil, fmt.Errorf("not an IPv6 address: %s", name)
		}
	}

	ips, err := net.LookupIP(name)
	if err != nil {
		return nil, fmt.Errorf("unknown host: %s", name)
	}

	var addrs [][16]byte
	for _, ip := range ips {
		if ip.To4() == nil {
			var addr [16]byte
			copy(addr[:], ip.To16())
			addrs = append(addrs, addr)
		}
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("no IPv6 addresses for host: %s", name)
	}
	return addrs, nil
}

// LookupPort resolves a service name to a port number.
// proto is the IP protocol number (6=TCP, 17=UDP, 132=SCTP, -1=any).
func (r *Resolver) LookupPort(name string, proto int) (int, error) {
	// Try numeric port first
	if p, err := strconv.Atoi(name); err == nil {
		if p < 0 || p > 65535 {
			return 0, fmt.Errorf("port %d out of range", p)
		}
		return p, nil
	}

	network := protoToNetwork(proto)
	port, err := net.LookupPort(network, name)
	if err != nil {
		return 0, fmt.Errorf("unknown port: %s", name)
	}
	return port, nil
}

// LookupPortRange resolves a port range "PORT1-PORT2".
func (r *Resolver) LookupPortRange(name string, proto int) (int, int, error) {
	parts := strings.SplitN(name, "-", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid port range: %s", name)
	}
	p1, err := r.LookupPort(parts[0], proto)
	if err != nil {
		return 0, 0, err
	}
	p2, err := r.LookupPort(parts[1], proto)
	if err != nil {
		return 0, 0, err
	}
	return p1, p2, nil
}

// protoTable maps protocol names to numbers.
var protoTable = map[string]int{
	"ip":      0,
	"hopopt":  0,
	"icmp":    1,
	"igmp":    2,
	"ggp":     3,
	"ipencap": 4,
	"tcp":     6,
	"egp":     8,
	"igp":     9,
	"pup":     12,
	"udp":     17,
	"hmp":     20,
	"xns-idp": 22,
	"rdp":     27,
	"ipv6":    41,
	"ipv6-route": 43,
	"ipv6-frag":  44,
	"idrp":    45,
	"rsvp":    46,
	"gre":     47,
	"esp":     50,
	"ah":      51,
	"ipv6-icmp": 58,
	"icmpv6":  58,
	"ipv6-nonxt": 59,
	"ipv6-opts":  60,
	"ospf":    89,
	"pim":     103,
	"vrrp":    112,
	"sctp":    132,
}

// LookupProto resolves a protocol name to its number (e.g., "tcp" → 6).
func (r *Resolver) LookupProto(name string) (int, error) {
	if n, ok := protoTable[strings.ToLower(name)]; ok {
		return n, nil
	}
	return 0, fmt.Errorf("unknown protocol: %s", name)
}

// eprotoTable maps Ethernet protocol names to ethertypes.
var eprotoTable = map[string]int{
	"pup":    0x0200,
	"xns":    0x0600,
	"ip":     0x0800,
	"arp":    0x0806,
	"rarp":   0x8035,
	"sprite": 0x0500,
	"mopdl":  0x6001,
	"moprc":  0x6002,
	"decnet": 0x6003,
	"lat":    0x6004,
	"sca":    0x6007,
	"atalk":  0x809b,
	"aarp":   0x80f3,
	"ipx":    0x8137,
	"ipv6":   0x86dd,
	"loopback": 0x9000,
}

// LookupEProto resolves an Ethernet protocol name to its ethertype.
func (r *Resolver) LookupEProto(name string) (int, error) {
	if n, ok := eprotoTable[strings.ToLower(name)]; ok {
		return n, nil
	}
	return 0, fmt.Errorf("unknown ether proto: %s", name)
}

// llcTable maps LLC names to SAP values.
var llcTable = map[string]int{
	"iso":  0xfe,
	"stp":  0x42,
	"ipx":  0xe0,
	"netbeui": 0xf0,
}

// LookupLLC resolves an LLC name to its SAP value.
func (r *Resolver) LookupLLC(name string) (int, error) {
	if n, ok := llcTable[strings.ToLower(name)]; ok {
		return n, nil
	}
	return 0, fmt.Errorf("unknown LLC: %s", name)
}

// LookupNet resolves a network name to an address and mask.
// Falls back to parsing CIDR notation (e.g., "192.168.0.0/24").
func (r *Resolver) LookupNet(name string) (uint32, uint32, error) {
	if strings.Contains(name, "/") {
		_, ipnet, err := net.ParseCIDR(name)
		if err != nil {
			return 0, 0, err
		}
		v4 := ipnet.IP.To4()
		if v4 == nil {
			return 0, 0, fmt.Errorf("not an IPv4 CIDR: %s", name)
		}
		addr := binary.BigEndian.Uint32(v4)
		mask := binary.BigEndian.Uint32(ipnet.Mask)
		return addr, mask, nil
	}
	return 0, 0, fmt.Errorf("unknown network: %s", name)
}

// LookupEther resolves a hostname to a MAC address by reading /etc/ethers.
func (r *Resolver) LookupEther(name string) ([]byte, error) {
	// Try parsing as MAC address first
	hw, err := net.ParseMAC(name)
	if err == nil {
		return hw, nil
	}

	// Search /etc/ethers
	f, err := os.Open("/etc/ethers")
	if err != nil {
		return nil, fmt.Errorf("cannot resolve ethernet address for %s: %v", name, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.EqualFold(fields[1], name) {
			mac, err := net.ParseMAC(fields[0])
			if err != nil {
				continue
			}
			return mac, nil
		}
	}
	return nil, fmt.Errorf("unknown ether host: %s", name)
}

func protoToNetwork(proto int) string {
	switch proto {
	case 6:
		return "tcp"
	case 17:
		return "udp"
	default:
		return "ip" // net.LookupPort accepts "" or common names
	}
}
