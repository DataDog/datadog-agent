package util

import (
	"net"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ConnectionFilter holds a user-defined blacklisted IP/CIDR, and ports
type ConnectionFilter struct {
	IP       *net.IPNet
	Ports    map[uint16]struct{}
	AllPorts bool
}

// ParseConnectionFilters takes the user defined blacklist and returns a slice of ConnectionFilters
func ParseConnectionFilters(filters map[string][]string) (blacklist []*ConnectionFilter) {
	for ip, ports := range filters {
		filter := &ConnectionFilter{Ports: map[uint16]struct{}{}}
		var subnet *net.IPNet
		var err error

		// retrieve valid IPs
		if strings.ContainsRune(ip, '*') {
			subnet = nil // use for wildcard
		} else if strings.ContainsRune(ip, '/') {
			_, subnet, err = net.ParseCIDR(ip)
		} else if strings.ContainsRune(ip, '.') {
			_, subnet, err = net.ParseCIDR(ip + "/32") // if given ipv4, prefix length of 32
		} else if strings.Contains(ip, "::") {
			_, subnet, err = net.ParseCIDR(ip + "/64") // if given ipv6, prefix length of 64
		} else {
			log.Debugf("Invalid IP/CIDR/* defined for connection filter: %s", err)
			continue
		}

		if err != nil {
			log.Errorf("Given filter will not be respected. Could not parse given IPs: %s", err)
			continue
		}
		filter.IP = subnet

		validFilter := true
		for _, v := range ports {
			if v == "*" {
				// This means that IP + port are both *, which effectively blacklists all conns, which is invalid.
				if subnet == nil {
					log.Errorf("Given rule will not be respected. Invalid filter with IP/CIDR as * and port as *: %s", err)
					validFilter = false
					break
				}

				filter.AllPorts = true
				continue
			}

			// The defined port is an integer, lets handle that
			k, err := strconv.ParseUint(v, 10, 16)
			if err != nil {
				log.Debugf("Could not parse list of ports: %s", err)
				validFilter = false
			}
			filter.Ports[uint16(k)] = struct{}{}
		}

		if validFilter {
			blacklist = append(blacklist, filter)
		}
	}
	return blacklist
}

// IsBlacklistedConnection returns true if a given connection should be excluded
// by the tracer based on user defined filters
func IsBlacklistedConnection(cf []*ConnectionFilter, addrIP Address, addrPort uint16) bool {
	ip := NetIPFromAddress(addrIP)

	// No filters so short-circuit
	if len(cf) == 0 {
		return false
	}

	// Iterate through filters to see if this connection matches any defined filter.
	for _, filter := range cf {
		// IP is wildcard (*) so only ports are defined
		if filter.IP == nil {
			if _, ok := filter.Ports[addrPort]; ok {
				return true
			}
		} else if filter.IP.Contains(ip) {
			if filter.AllPorts {
				return true
			} else if _, ok := filter.Ports[addrPort]; ok {
				return true
			}
		}
	}

	return false
}
