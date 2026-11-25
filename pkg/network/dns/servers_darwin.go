// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package dns

import (
	"context"
	"net/netip"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	dnsPort = 53

	scutilPath = "/usr/sbin/scutil"
	// dnsServersTimeout bounds each listing of the system's DNS servers.
	dnsServersTimeout = 5 * time.Second
	// dnsServersRefreshInterval is how often the system's DNS servers are
	// listed again, since they change as networks and VPNs come and go.
	dnsServersRefreshInterval = 30 * time.Second
)

// listDNSServers returns the DNS servers that `scutil --dns` lists on ports
// other than dnsPort, as configured by a file in /etc/resolver or by a VPN.
func listDNSServers() ([]netip.AddrPort, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dnsServersTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, scutilPath, "--dns").Output()
	if err != nil {
		return nil, err
	}
	return parseDNSServers(string(out)), nil
}

// parseDNSServers returns the DNS servers on ports other than dnsPort that
// the output of `scutil --dns` lists, sorted and without duplicates. Each
// resolver lists its servers as "nameserver[N] : <address>", and a
// "port : <port>" line only when it sets a port. Its servers use dnsPort
// otherwise.
func parseDNSServers(out string) []netip.AddrPort {
	var servers []netip.AddrPort
	var addrs []netip.Addr
	var port uint16
	endResolver := func() {
		if port != 0 && port != dnsPort {
			for _, addr := range addrs {
				servers = append(servers, netip.AddrPortFrom(addr, port))
			}
		}
		addrs, port = addrs[:0], 0
	}
	for line := range strings.Lines(out) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "resolver #") || strings.HasPrefix(line, "DNS configuration") {
			endResolver()
			continue
		}
		key, value, _ := strings.Cut(line, ":")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		switch {
		case strings.HasPrefix(key, "nameserver["):
			if addr, err := netip.ParseAddr(value); err == nil {
				addrs = append(addrs, addr.Unmap().WithZone(""))
			}
		case key == "port":
			if p, err := strconv.ParseUint(value, 10, 16); err == nil {
				port = uint16(p)
			}
		}
	}
	endResolver()
	slices.SortFunc(servers, netip.AddrPort.Compare)
	return slices.Compact(servers)
}
