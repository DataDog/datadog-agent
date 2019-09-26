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
	Ports    map[uint16]ConnType
	AllPorts ConnType
}

type ConnType struct {
	TCP bool
	UDP bool
}

// ParseConnectionFilters takes the user defined blacklist and returns a slice of ConnectionFilters
func ParseConnectionFilters(filters map[string][]string) (blacklist []*ConnectionFilter) {
	for ip, ports := range filters {
		filter := &ConnectionFilter{Ports: map[uint16]ConnType{}}
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
			// reset validFilter for each port
			validFilter = true
			v = strings.ToUpper(v)
			// set tcp and udp to true to prevent exclusion by default
			tcp, udp := true, true
			switch {
			case strings.HasPrefix(v, "TCP"):
				udp = false
				v = strings.TrimPrefix(v, "TCP")
			case strings.HasPrefix(v, "UDP"):
				tcp = false
				v = strings.TrimPrefix(v, "UDP")
			}

			if strings.ContainsRune(v, '*') {
				filter.AllPorts.TCP, filter.AllPorts.UDP = tcp || filter.AllPorts.TCP, udp || filter.AllPorts.UDP
				// This means that IP + port are both *, which effectively blacklists all conns, which is invalid.
				if subnet == nil && filter.AllPorts.TCP && filter.AllPorts.UDP {
					log.Errorf("Given rule will not be respected. Invalid filter with IP/CIDR as * and port as *: %s", err)
					validFilter = false
					break
				}
				continue
			}

			// The defined port is a range
			if strings.ContainsRune(v, '-') {
				portRange := strings.Split(v, "-")
				startPort, err := strconv.ParseUint(strings.TrimSpace(portRange[0]), 10, 16)
				if err != nil {
					log.Debugf("Could not parse list of ports: %s", err)
					validFilter = false
					break
				}
				endPort, err := strconv.ParseUint(strings.TrimSpace(portRange[1]), 10, 16)
				if err != nil {
					log.Debugf("Could not parse list of ports: %s", err)
					validFilter = false
					break
				}

				// invalid configuration
				if startPort > endPort {
					log.Debugf("Invalid port range %d-%d", startPort, endPort)
					validFilter = false
					break
				}
				for startPort <= endPort {
					filter.Ports[uint16(startPort)] = ConnType{TCP: tcp || filter.Ports[uint16(startPort)].TCP, UDP: udp || filter.Ports[uint16(startPort)].UDP}
					startPort++
				}
				continue
			}

			// The defined port is an integer, lets handle that
			k, err := strconv.ParseUint(strings.TrimSpace(v), 10, 16)
			if err != nil {
				log.Debugf("Could not parse list of ports: %s", err)
				validFilter = false
			} else {
				filter.Ports[uint16(k)] = ConnType{TCP: tcp, UDP: udp}
			}
		}

		if validFilter {
			blacklist = append(blacklist, filter)
		}
	}
	return blacklist
}

// IsBlacklistedConnection returns true if a given connection should be excluded
// by the tracer based on user defined filters
func IsBlacklistedConnection(cf []*ConnectionFilter, addrIP Address, addrPort uint16, addrType string) bool {
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
			} else if filter.AllPorts.TCP && addrType == "TCP" {
				return true
			} else if filter.AllPorts.UDP && addrType == "UDP" {
				return true
			} else if _, ok := filter.Ports[addrPort]; ok {
				if filter.Ports[addrPort].TCP && filter.Ports[addrPort].UDP {
					return true
				} else if filter.Ports[addrPort].TCP && addrType == "TCP" {
					return true
				} else if filter.Ports[addrPort].UDP && addrType == "UDP" {
					return true
				}
			}
		}
	}

	return false
}
