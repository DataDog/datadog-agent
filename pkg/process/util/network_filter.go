package util

import (
	"github.com/DataDog/agent-payload/process"
	"net"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ConnectionFilter holds a user-defined blacklisted IP/CIDR, and ports
type ConnectionFilter struct {
	IP       *net.IPNet
	Ports    map[uint16]ConnTypeFilter
	AllPorts ConnTypeFilter
}

type ConnTypeFilter struct {
	TCP bool
	UDP bool
}

// ParseConnectionFilters takes the user defined blacklist and returns a slice of ConnectionFilters
func ParseConnectionFilters(filters map[string][]string) (blacklist []*ConnectionFilter) {
	for ip, ports := range filters {
		filter := &ConnectionFilter{Ports: map[uint16]ConnTypeFilter{}}
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

			lowerPort, upperPort, connTypeFilter, valid := parsePortAndProtocolFilter(v)
			validFilter = valid

			if !validFilter {
				break
			}

			// port is wildcard
			if lowerPort == 0 && upperPort == 0 && validFilter {
				if subnet == nil && connTypeFilter.TCP && connTypeFilter.UDP {
					log.Errorf("Given rule will not be respected. Invalid filter with IP/CIDR as * and port as *: %s", err)
					validFilter = false
					break
				}
				filter.AllPorts.TCP = connTypeFilter.TCP || filter.AllPorts.TCP
				filter.AllPorts.UDP = connTypeFilter.UDP || filter.AllPorts.UDP

				continue
			}

			// port is integer
			if upperPort == 0 && validFilter {
				filter.Ports[uint16(lowerPort)] = ConnTypeFilter{
					TCP: connTypeFilter.TCP || filter.Ports[uint16(lowerPort)].TCP,
					UDP: connTypeFilter.UDP || filter.Ports[uint16(lowerPort)].UDP,
				}

				continue
			}

			// port range
			if validFilter {
				for port := lowerPort; port <= upperPort; port++ {
					filter.Ports[uint16(port)] = ConnTypeFilter{
						TCP: connTypeFilter.TCP || filter.Ports[uint16(port)].TCP,
						UDP: connTypeFilter.UDP || filter.Ports[uint16(port)].UDP,
					}
				}
			}
		}

		if validFilter {
			blacklist = append(blacklist, filter)
		}
	}

	return blacklist
}

func parsePortAndProtocolFilter(v string) (uint64, uint64, ConnTypeFilter, bool) {
	validFilter := true
	lowerPort, upperPort := uint64(0), uint64(0)
	v = strings.ToUpper(v)
	connTypeFilter := ConnTypeFilter{TCP: true, UDP: true}

	switch {
	case strings.HasPrefix(v, "TCP"):
		connTypeFilter.UDP = false
		v = strings.TrimPrefix(v, "TCP")
	case strings.HasPrefix(v, "UDP"):
		connTypeFilter.TCP = false
		v = strings.TrimPrefix(v, "UDP")
	}

	v = strings.TrimSpace(v)
	if v == "*" {
		return lowerPort, upperPort, connTypeFilter, validFilter
	}

	// The defined port is a range
	if strings.ContainsRune(v, '-') {
		portRange := strings.Split(v, "-")

		// invalid configuration
		if len(portRange) != 2 {
			validFilter = false
			return lowerPort, upperPort, connTypeFilter, validFilter
		}
		lowerPort, err := strconv.ParseUint(strings.TrimSpace(portRange[0]), 10, 16)
		if err != nil {
			log.Debugf("Parsed port was invalid: %s", err)
			validFilter = false
			return lowerPort, upperPort, connTypeFilter, validFilter
		}
		upperPort, err := strconv.ParseUint(strings.TrimSpace(portRange[1]), 10, 16)
		if err != nil {
			log.Debugf("Parsed port was invalid: %s", err)
			validFilter = false
			return lowerPort, upperPort, connTypeFilter, validFilter
		}

		// invalid configuration
		if lowerPort > upperPort {
			log.Debugf("Invalid port range %d-%d", lowerPort, upperPort)
			validFilter = false
			return lowerPort, upperPort, connTypeFilter, validFilter
		}

		// lp #, up #, ctf tcp udp, valid +
		return lowerPort, upperPort, connTypeFilter, validFilter
	}

	// The defined port is an integer
	lowerPort, err := strconv.ParseUint(v, 10, 16)
	if err != nil {
		log.Debugf("Parsed port was invalid: %s", err)
		validFilter = false
		return lowerPort, upperPort, connTypeFilter, validFilter
	}
	return lowerPort, upperPort, connTypeFilter, validFilter
}

// IsBlacklistedConnection returns true if a given connection should be excluded
// by the tracer based on user defined filters
func IsBlacklistedConnection(cf []*ConnectionFilter, addrIP Address, addrPort uint16, addrType process.ConnectionType) bool {
	ip := NetIPFromAddress(addrIP)
	// No filters so short-circuit
	if len(cf) == 0 {
		return false
	}

	// Iterate through filters to see if this connection matches any defined filter.
	for _, filter := range cf {
		if filter.IP == nil || filter.IP.Contains(ip) {
			if filter.AllPorts.TCP && filter.AllPorts.UDP {
				return true
			} else if filter.AllPorts.TCP && addrType == process.ConnectionType_tcp {
				return true
			} else if filter.AllPorts.UDP && addrType == process.ConnectionType_udp {
				return true
			} else if _, ok := filter.Ports[addrPort]; ok {
				if filter.Ports[addrPort].TCP && filter.Ports[addrPort].UDP {
					return true
				} else if filter.Ports[addrPort].TCP && addrType == process.ConnectionType_tcp {
					return true
				} else if filter.Ports[addrPort].UDP && addrType == process.ConnectionType_udp {
					return true
				}
			}
		}
	}

	return false
}
