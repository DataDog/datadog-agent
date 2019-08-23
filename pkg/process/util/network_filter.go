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

// ParseConnectionFilters takes the user defined blacklist and returns a slice of ConnectionFilters
func ParseConnectionFilters(filters map[string][]string) (blacklist []*ConnectionFilter) {
out:
	for ip, ports := range filters {
		conn := &ConnectionFilter{Ports: map[uint16]struct{}{}}
		var subnet *net.IPNet
		var err error
		var ignorePorts bool

		// retrieve valid IPs
		if strings.ContainsRune(ip, 42) {
			subnet = nil // use for wildcard
		} else if strings.ContainsRune(ip, 47) {
			_, subnet, err = net.ParseCIDR(ip)
		} else if strings.ContainsRune(ip, 58) {
			_, subnet, err = net.ParseCIDR(ip + "/64") // if given ipv6, prefix length of 64
		} else {
			_, subnet, err = net.ParseCIDR(ip + "/32") // if given ipv4, prefix length of 32
		}

		if err != nil {
			log.Debugf("Could not parse given IPs: %s", err)
			continue
		}
		conn.IP = subnet

		// convert and store given ports
		for _, v := range ports {
			// look for wildcard
			if strings.ContainsRune(v, 42) {
				ignorePorts = true
				continue
			}

			k, err := strconv.ParseUint(v, 10, 16)
			if err != nil {
				log.Debugf("Could not parse list of ports: %s", err)
				continue out
			}
			conn.Ports[uint16(k)] = struct{}{}
		}

		// declare empty ports if wildcard is found
		if ignorePorts {
			conn.Ports = map[uint16]struct{}{}
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
			switch {
			case conn.IP == nil:
				if _, ok := conn.Ports[addrPort]; ok {
					return true
				}
			case conn.IP.Contains(ip):
				if _, ok := conn.Ports[addrPort]; ok {
					return true
				} else if len(conn.Ports) == 0 {
					return true
				}
			}
		}
	}
	return false
}
