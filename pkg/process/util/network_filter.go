package util

import (
	"net"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ConnectionFilter holds a user-defined blacklisted IP/CIDR, and ports
type ConnectionFilter struct {
	IP    *net.IPNet
	Ports map[uint16]struct{}
}

// ParseBlacklist takes the user defined blacklist and returns a slice of ConnectionFilters
func ParseBlacklist(filters map[string][]string) (blacklist []*ConnectionFilter) {
	for ip, ports := range filters {
		conn := &ConnectionFilter{Ports: map[uint16]struct{}{}}

		// retrieve valid IPs
		if strings.Contains(ip, "/") {
			_, subnet, err := net.ParseCIDR(ip)
			if err != nil {
				log.Debugf("Could not parse %s", err)
			}
			conn.IP = subnet
		} else if strings.Contains(ip, ":") {
			_, subnet, err := net.ParseCIDR(ip + "/64") // if given ipv6, prefix length of 64
			if err != nil {
				log.Debugf("Could not parse %s", err)
			}
			conn.IP = subnet

		} else {
			_, subnet, err := net.ParseCIDR(ip + "/32") // if given ipv4, prefix length of 32
			if err != nil {
				log.Debugf("Could not parse given IPs: %s", err)
			}
			conn.IP = subnet
		}

		// convert and store given ports
		for _, v := range ports {
			k, err := strconv.ParseUint(v, 10, 16)
			if err != nil {
				log.Debugf("Could not parse given ports: %s", err)
				continue
			}
			conn.Ports[uint16(k)] = struct{}{}
		}
		blacklist = append(blacklist, conn)
	}
	return blacklist
}

// IsBlacklistedConnection returns true if a given connection should be excluded
// by the tracer based on user defined filters
func IsBlacklistedConnection(c []*ConnectionFilter, addrIP Address, addrPort uint16) bool {
	ip := NetIPFromAddress(addrIP)

	if len(c) > 0 {
		for _, conn := range c {
			if conn.IP != nil && conn.IP.Contains(ip) {
				if _, ok := conn.Ports[addrPort]; ok {
					return true
				}
			}
		}
	}
	return false
}
