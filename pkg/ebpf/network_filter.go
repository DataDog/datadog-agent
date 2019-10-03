package ebpf

import (
	"github.com/DataDog/datadog-agent/pkg/process/util"
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

// ConnTypeFilter holds user-defined protocols
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
		var validFilter bool
		for _, v := range ports {
			lowerPort, upperPort, connTypeFilter, valid := parsePortAndProtocolFilter(v)
			validFilter = valid

			if !validFilter {
				break
			}

			if lowerPort == 0 && upperPort == 0 { // port is wildcard
				if subnet == nil && connTypeFilter.TCP && connTypeFilter.UDP {
					log.Errorf("Given rule will not be respected. Invalid filter with IP/CIDR as * and port as *: %s", err)
					validFilter = false
					break
				}
				filter.AllPorts.TCP = connTypeFilter.TCP || filter.AllPorts.TCP
				filter.AllPorts.UDP = connTypeFilter.UDP || filter.AllPorts.UDP
			} else { // port is integer/range
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

// parsePortAndProtocolFilter checks for valid port(s) and protocol filters
// and returns a port/port range, protocol, and the validity of those values
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

	// The defined port is a wildcard
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
		if err != nil || lowerPort == 0 {
			log.Debugf("Parsed port %d was invalid: %s", lowerPort, err)
			validFilter = false
			return lowerPort, upperPort, connTypeFilter, validFilter
		}
		upperPort, err := strconv.ParseUint(strings.TrimSpace(portRange[1]), 10, 16)
		if err != nil || upperPort == 0 {
			log.Debugf("Parsed port %d was invalid: %s", upperPort, err)
			validFilter = false
			return lowerPort, upperPort, connTypeFilter, validFilter
		}

		// invalid configuration
		if lowerPort > upperPort {
			log.Debugf("Invalid port range %d-%d", lowerPort, upperPort)
			validFilter = false
			return lowerPort, upperPort, connTypeFilter, validFilter
		}

		return lowerPort, upperPort, connTypeFilter, validFilter
	}

	// The defined port is an integer
	lowerPort, err := strconv.ParseUint(v, 10, 16)
	upperPort = lowerPort
	if err != nil || lowerPort == 0 {
		log.Debugf("Parsed port %d was invalid: %s", lowerPort, err)
		validFilter = false
		return lowerPort, upperPort, connTypeFilter, validFilter
	}
	return lowerPort, upperPort, connTypeFilter, validFilter
}

// IsBlacklistedConnection returns true if a given connection should be excluded
// by the tracer based on user defined filters
func IsBlacklistedConnection(scf []*ConnectionFilter, dcf []*ConnectionFilter, conn *ConnectionStats) bool {
	// No filters so short-circuit
	if len(scf) == 0 && len(dcf) == 0 {
		return false
	}

	if len(scf) > 0 && conn.Source != nil {
		if findMatchingFilter(scf, util.NetIPFromAddress(conn.Source), conn.SPort, conn.Type) {
			return true
		}
	} else if len(dcf) > 0 && conn.Dest != nil {
		if findMatchingFilter(dcf, util.NetIPFromAddress(conn.Dest), conn.DPort, conn.Type) {
			return true
		}
	}
	return false
}

// findMatchingFilter iterates through filters to see if this connection matches any defined filter
func findMatchingFilter(cf []*ConnectionFilter, ip net.IP, addrPort uint16, addrType ConnectionType) bool {
	for _, filter := range cf {
		if filter.IP == nil || filter.IP.Contains(ip) {
			if filter.AllPorts.TCP && filter.AllPorts.UDP {
				return true
			} else if filter.AllPorts.TCP && addrType == TCP {
				return true
			} else if filter.AllPorts.UDP && addrType == UDP {
				return true
			} else if _, ok := filter.Ports[addrPort]; ok {
				if filter.Ports[addrPort].TCP && filter.Ports[addrPort].UDP {
					return true
				} else if filter.Ports[addrPort].TCP && addrType == TCP {
					return true
				} else if filter.Ports[addrPort].UDP && addrType == UDP {
					return true
				}
			}
		}
	}
	return false
}
