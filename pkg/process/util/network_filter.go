package util

import (
	"net"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/stretchr/stew/slice"
)

type Connection struct {
	IP    string
	CIDR  *net.IPNet
	Ports []string
}

// takes the user defined blacklist and returns a slice of Connections
func parseBlacklist(filters map[string][]string) (blacklist []*Connection) {
	for ip, ports := range filters {
		conn := &Connection{
			IP:    ip,
			Ports: ports,
		}

		if strings.Contains(ip, "/") {
			ipv4, subnet, err := net.ParseCIDR(ip)
			if err != nil {
				log.Debugf("Could not parse %s", err)
			}
			conn.IP, conn.CIDR = ipv4.String(), subnet
		}
		blacklist = append(blacklist, conn)
	}
	return blacklist
}

// determine whether we should be excluding a source or destination connection
func newNetworkFilter(direction string) (networkFilter []*Connection) {
	if direction == "source" {
		networkFilter = parseBlacklist(config.Datadog.GetStringMapStringSlice("system_probe_config.excluded_source_connections"))
	} else if direction == "destination" {
		networkFilter = parseBlacklist(config.Datadog.GetStringMapStringSlice("system_probe_config.excluded_destination_connections"))
	}
	return networkFilter
}

// IsBlacklistedConnection returns true if a given connection should be excluded
// by the tracer based on user defined filters
func IsBlacklistedConnection(dir string, addrIP Address, addrPort uint16) bool {
	if nf := newNetworkFilter(dir); len(nf) > 0 {
		for _, conn := range nf {
			// see if we should exclude this IP
			if conn.CIDR != nil && conn.CIDR.Contains(net.ParseIP(addrIP.String())) || conn.IP == addrIP.String() {
				switch {
				case slice.ContainsString(conn.Ports, "*") || len(conn.Ports) == 0:
					return true
				case slice.ContainsString(conn.Ports, strconv.Itoa(int(addrPort))):
					return true
				}
			}
			// see if we should exclude this port across all connections
			if (strings.Contains(conn.IP, "*") || len(conn.IP) == 0) && slice.ContainsString(conn.Ports, strconv.Itoa(int(addrPort))) {
				return true
			}
		}
	}
	return false
}
