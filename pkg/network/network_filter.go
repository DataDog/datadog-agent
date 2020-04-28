package network

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/process/util"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ConnectionFilter holds a user-defined blacklisted IP/CIDR, and ports
type ConnectionFilter struct {
	IP       *net.IPNet // If nil, then all IPs will be considered matching.
	AllPorts ConnTypeFilter

	Ports map[uint16]ConnTypeFilter
}

// ConnTypeFilter holds user-defined protocols
type ConnTypeFilter struct {
	TCP bool
	UDP bool
}

// ParseConnectionFilters takes the user defined blacklist and returns a slice of ConnectionFilters
func ParseConnectionFilters(filters map[string][]string) (blacklist []*ConnectionFilter) {
	for ip, portFilters := range filters {
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
			log.Errorf("Invalid IP/CIDR/* defined for connection filter")
			continue
		}

		if err != nil {
			log.Errorf("Given filter will not be respected. Could not parse address: %s", err)
			continue
		}

		filter.IP = subnet

		// Process port filters for the above parsed address range
		for _, rawPortMapping := range portFilters {
			lowerPort, upperPort, transportFilter, e := parsePortFilter(rawPortMapping)
			if e != nil {
				err = log.Error(e)
				break
			}

			// Port filter for is a wildcard
			if lowerPort == 0 && upperPort == 0 {
				if subnet == nil { // Check that theres no wildcard filter above, or we'd just skip everything which is invalid
					err = log.Errorf("Given rule will not be respected. Invalid filter with IP/CIDR as * and port as *")
					break
				}

				// There can be multiple wildcard port filters.
				// Since we can do something like "udp *", "*", we want to widen the scope as much as possible.
				filter.AllPorts.UDP = filter.AllPorts.UDP || transportFilter.UDP
				filter.AllPorts.TCP = filter.AllPorts.TCP || transportFilter.TCP
			} else { // Otherwise the port filter for this address range is an integer range.
				for port := lowerPort; port <= upperPort; port++ {
					filter.Ports[uint16(port)] = ConnTypeFilter{
						TCP: transportFilter.TCP || filter.Ports[uint16(port)].TCP,
						UDP: transportFilter.UDP || filter.Ports[uint16(port)].UDP,
					}
				}
			}
		}

		// If there were any errors in parsing the port filters above, we'll skip this entry.
		if err == nil {
			blacklist = append(blacklist, filter)
		}
	}
	return blacklist
}

// parsePortFilter checks for valid port(s) and protocol filters
// and returns a port/port range, protocol, and the validity of those values
func parsePortFilter(pf string) (uint64, uint64, ConnTypeFilter, error) {
	lowerPort, upperPort := uint64(0), uint64(0)
	connTypeFilter := ConnTypeFilter{TCP: true, UDP: true}
	var err error

	pf = strings.ToUpper(pf)

	// Check if this port range depends on a particular transport type
	switch {
	case strings.HasPrefix(pf, "TCP"):
		connTypeFilter.UDP = false
		pf = strings.TrimPrefix(pf, "TCP")
	case strings.HasPrefix(pf, "UDP"):
		connTypeFilter.TCP = false
		pf = strings.TrimPrefix(pf, "UDP")
	}

	pf = strings.TrimSpace(pf)
	if pf == "*" { // The defined port is a wildcard
		return 0, 0, connTypeFilter, nil // lowerPort = upperPort = 0 signals a wildcard port range.
	}

	// The defined port is a range
	if strings.ContainsRune(pf, '-') {
		if portRange := strings.Split(pf, "-"); len(portRange) == 2 {
			lowerPort, err = parsePortString(strings.TrimSpace(portRange[0])) // Parse lower port into lowerPort
			if err == nil {
				upperPort, err = parsePortString(strings.TrimSpace(portRange[1])) // Parse upper port into upperPort
			}
		} else {
			err = fmt.Errorf("invalid port range doesn't have enough components: %pf", portRange)
		}
	} else { // The defined port is an integer
		lowerPort, err = parsePortString(pf)
		upperPort = lowerPort
	}

	// More validation (ports can't be 0, or out of order: e.g. 321-100)
	if err != nil {
		return 0, 0, connTypeFilter, fmt.Errorf("failed to parse ports: %s", err)
	} else if lowerPort == 0 || upperPort == 0 {
		return 0, 0, connTypeFilter, fmt.Errorf("invalid port 0")
	} else if lowerPort > upperPort {
		return 0, 0, connTypeFilter, fmt.Errorf("invalid port range %d-%d", lowerPort, upperPort)
	}

	return lowerPort, upperPort, connTypeFilter, nil
}

func parsePortString(port string) (uint64, error) {
	p, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("error parsing port: %s", err)
	}
	return p, nil
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
			if filter.AllPorts.TCP && filter.AllPorts.UDP { // Wildcard port range case
				return true
			} else if filter.AllPorts.TCP && addrType == TCP { // Wildcard port range for only TCP
				return true
			} else if filter.AllPorts.UDP && addrType == UDP { // Wildcard port range for only UDP
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
