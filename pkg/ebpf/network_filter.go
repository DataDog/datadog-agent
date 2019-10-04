package ebpf

import (
	"fmt"
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
			log.Errorf("Invalid IP/CIDR/* defined for connection filter: %s", err)
			continue
		}

		if err != nil {
			log.Errorf("Given filter will not be respected. Could not parse given IPs: %s", err)
			continue
		}

		filter.IP = subnet
		for _, v := range ports {
			var lowerPort, upperPort uint64
			var connTypeFilter ConnTypeFilter

			lowerPort, upperPort, connTypeFilter, err = parsePortAndProtocolFilter(v)

			if err != nil {
				log.Error(err)
				break
			}

			// port is wildcard
			if lowerPort == 0 && upperPort == 0 {
				filter.AllPorts.TCP = connTypeFilter.TCP || filter.AllPorts.TCP
				filter.AllPorts.UDP = connTypeFilter.UDP || filter.AllPorts.UDP
				if subnet == nil && filter.AllPorts.TCP && filter.AllPorts.UDP {
					err = log.Errorf("Given rule will not be respected. Invalid filter with IP/CIDR as * and port as *")
					break
				}
			} else {
				// port is integer/range
				for port := lowerPort; port <= upperPort; port++ {
					filter.Ports[uint16(port)] = ConnTypeFilter{
						TCP: connTypeFilter.TCP || filter.Ports[uint16(port)].TCP,
						UDP: connTypeFilter.UDP || filter.Ports[uint16(port)].UDP,
					}
				}
			}
		}

		if err == nil {
			blacklist = append(blacklist, filter)
		}
	}
	return blacklist
}

// parsePortAndProtocolFilter checks for valid port(s) and protocol filters
// and returns a port/port range, protocol, and the validity of those values
func parsePortAndProtocolFilter(v string) (uint64, uint64, ConnTypeFilter, error) {
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
		return lowerPort, upperPort, connTypeFilter, nil
	}

	// The defined port is a range
	if strings.ContainsRune(v, '-') {
		portRange := strings.Split(v, "-")

		// invalid configuration
		if len(portRange) != 2 {
			return lowerPort, upperPort, connTypeFilter, fmt.Errorf("invalid port range %v", portRange)
		}
		lowerPort, err := strconv.ParseUint(strings.TrimSpace(portRange[0]), 10, 16)
		if err != nil {
			return lowerPort, upperPort, connTypeFilter, fmt.Errorf("error parsing port: %s", err)
		} else if lowerPort == 0 {
			return lowerPort, upperPort, connTypeFilter, fmt.Errorf("invalid port %d", lowerPort)
		}
		upperPort, err := strconv.ParseUint(strings.TrimSpace(portRange[1]), 10, 16)
		if err != nil {
			return lowerPort, upperPort, connTypeFilter, fmt.Errorf("error parsing port: %s", err)
		} else if upperPort == 0 {
			return lowerPort, upperPort, connTypeFilter, fmt.Errorf("invalid port %d", upperPort)
		}

		// invalid configuration
		if lowerPort > upperPort {
			return lowerPort, upperPort, connTypeFilter, fmt.Errorf("invalid port range %d-%d", lowerPort, upperPort)
		}

		return lowerPort, upperPort, connTypeFilter, nil
	}

	// The defined port is an integer
	lowerPort, err := strconv.ParseUint(v, 10, 16)
	upperPort = lowerPort
	if err != nil {
		return lowerPort, upperPort, connTypeFilter, fmt.Errorf("error parsing port: %s", err)
	} else if lowerPort == 0 {
		return lowerPort, upperPort, connTypeFilter, fmt.Errorf("invalid port %d", lowerPort)
	}
	return lowerPort, upperPort, connTypeFilter, nil
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
	}
	if len(dcf) > 0 && conn.Dest != nil {
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
