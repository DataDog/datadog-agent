// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build darwin

package dns

import (
	"fmt"
	"maps"
	"net/netip"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/filter"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// dnsBPFBufferSize is the per-interface BPF ring buffer used for DNS-only capture.
	// DNS traffic is low-volume so 1 MB is sufficient.
	dnsBPFBufferSize = 1 * 1024 * 1024
	// dnsSnapLen is the maximum bytes captured per packet for DNS.
	// Matches the standard EDNS0 buffer size (RFC 6891) and the Linux default,
	// ensuring full DNS responses including frame headers are captured.
	dnsSnapLen = 4096
	// connectionDomainTTL keeps a domain reported by the connection tracer
	// cached until clients collect its connection. The tracer reports the
	// domain of each live connection again every minute, so an idle
	// connection's name stays cached until it sends or closes.
	connectionDomainTTL = 10 * time.Minute
	// dnsServersMax bounds the system's DNS servers in the capture filter. Each
	// adds about 11 ns per packet that reaches its address check, which on a
	// busy port such as 443 can be most packets.
	dnsServersMax = 16
	// dnsCaptureFilterMaxInstructions bounds the compiled capture filter. The
	// kernel accepts at most 512 instructions, and when it refuses a filter,
	// libpcap runs it in user space without an error. The margin covers link
	// types whose programs run a few instructions longer than Ethernet's.
	dnsCaptureFilterMaxInstructions = 500
)

// darwinDNSMonitor captures DNS traffic on the configured DNS monitoring
// ports, and to and from the servers the system sends plain DNS to on other
// ports.
type darwinDNSMonitor struct {
	*socketFilterSnooper

	// monitoredPorts holds the configured DNS monitoring ports.
	monitoredPorts []int
	// servers holds the system's DNS servers on other ports, sorted.
	servers atomic.Pointer[[]netip.AddrPort]
}

// NewReverseDNS starts DNS traffic monitoring on macOS and returns a ReverseDNS
// implementation backed by libpcap packet capture.
func NewReverseDNS(cfg *config.Config, _ telemetry.Component) (ReverseDNS, error) {
	expr, _ := dnsCaptureFilter(cfg.DNSMonitoringPortList, nil)
	src, err := filter.NewLibpcapSource(
		filter.OptBPFFilter(expr),
		filter.OptBPFBufferSize(dnsBPFBufferSize),
		filter.OptSnapLen(dnsSnapLen),
	)
	if err != nil {
		return nil, err
	}
	m, err := newDarwinDNSMonitorWithSource(cfg, src)
	if err != nil {
		return nil, err
	}
	if err := m.refreshDNSServers(listDNSServers, src.SetBPFFilter); err != nil {
		log.Warnf("unable to list the system's DNS servers, DNS is captured on the configured ports only: %v", err)
	}
	m.watchDNSServers(listDNSServers, src.SetBPFFilter)
	return m, nil
}

// newDarwinDNSMonitorWithSource constructs a DNS monitor using the provided
// PacketSource. Used by NewReverseDNS and tests that inject a mock source.
func newDarwinDNSMonitorWithSource(cfg *config.Config, src filter.PacketSource) (*darwinDNSMonitor, error) {
	ethernetParser := newDNSParser(layers.LayerTypeEthernet, cfg)
	loopbackParser := newDNSParser(layers.LayerTypeLoopback, cfg)

	parserSelector := func(info filter.PacketInfo) *dnsParser {
		if info.LinkLayerType() == layers.LayerTypeLoopback {
			return loopbackParser
		}
		return ethernetParser
	}

	snoop, err := newSocketFilterSnooper(cfg, src, parserSelector)
	if err != nil {
		return nil, err
	}
	m := &darwinDNSMonitor{
		socketFilterSnooper: snoop,
		monitoredPorts:      cfg.DNSMonitoringPortList,
	}
	m.servers.Store(&[]netip.AddrPort{})
	snoop.startPolling()
	return m, nil
}

// AddDomain caches domain as a name of addr. The macOS connection tracer
// reports the domain each connection was opened for, which also covers
// connections whose DNS traffic is encrypted or proxied.
func (m *darwinDNSMonitor) AddDomain(addr util.Address, domain string) {
	m.cache.Add(&translation{
		dns: ToHostname(domain),
		ips: map[util.Address]time.Time{addr: time.Now().Add(connectionDomainTTL)},
	})
}

// IsDNSServer reports whether addr serves plain DNS on port: on dnsPort or a
// configured DNS monitoring port, or as one of the system's DNS servers.
// dnsPort counts even when the configured ports leave it out, since the system
// resolver sends queries to it whatever is monitored.
func (m *darwinDNSMonitor) IsDNSServer(addr util.Address, port uint16) bool {
	return port == dnsPort || slices.Contains(m.monitoredPorts, int(port)) ||
		slices.Contains(*m.servers.Load(), netip.AddrPortFrom(addr.Addr.Unmap(), port))
}

// watchDNSServers lists the system's DNS servers every
// dnsServersRefreshInterval until the monitor closes.
func (m *darwinDNSMonitor) watchDNSServers(list func() ([]netip.AddrPort, error), setFilter func(string)) {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ticker := time.NewTicker(dnsServersRefreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-m.exit:
				return
			case <-ticker.C:
				if err := m.refreshDNSServers(list, setFilter); err != nil {
					log.Debugf("unable to list the system's DNS servers: %v", err)
				}
			}
		}
	}()
}

// refreshDNSServers lists the system's DNS servers and, when they changed,
// passes setFilter a capture filter that covers them. The servers last listed
// stay when listing fails.
func (m *darwinDNSMonitor) refreshDNSServers(list func() ([]netip.AddrPort, error), setFilter func(string)) error {
	servers, err := list()
	if err != nil {
		return err
	}
	if slices.Equal(servers, *m.servers.Load()) {
		return nil
	}
	m.servers.Store(&servers)
	log.Infof("DNS servers on ports other than %d: %v", dnsPort, servers)
	expr, left := dnsCaptureFilter(m.monitoredPorts, servers)
	if len(left) > 0 {
		log.Warnf("not capturing DNS traffic of %v, which the capture filter has no room for", left)
	}
	setFilter(expr)
	return nil
}

// dnsCaptureFilter returns the capture filter for DNS traffic on ports, and to
// and from the servers that need their own term, along with the servers left
// out. It takes servers in order, up to dnsServersMax and as many as fit in
// the kernel. An empty filter would capture every packet, so no ports means
// dnsPort.
func dnsCaptureFilter(ports []int, servers []netip.AddrPort) (expr string, left []netip.AddrPort) {
	if len(ports) == 0 {
		ports = []int{dnsPort}
	}
	// A server on one of ports is captured by its port term, and a server on
	// loopback can't be captured, since capture leaves out loopback interfaces.
	servers = slices.DeleteFunc(slices.Clone(servers), func(server netip.AddrPort) bool {
		return slices.Contains(ports, int(server.Port())) || server.Addr().IsLoopback()
	})
	n := min(len(servers), dnsServersMax)
	for n > 0 && !dnsCaptureFilterFits(dnsCaptureFilterExpr(ports, servers[:n])) {
		n--
	}
	return dnsCaptureFilterExpr(ports, servers[:n]), servers[n:]
}

// dnsCaptureFilterExpr returns a filter for traffic on ports, and to and from
// servers on their ports. Servers on the same port share its check, which
// keeps the filter short and spares packets on other ports their address
// checks.
func dnsCaptureFilterExpr(ports []int, servers []netip.AddrPort) string {
	var terms []string
	for _, port := range ports {
		terms = append(terms, fmt.Sprintf("port %d", port))
	}
	hosts := make(map[uint16][]string)
	for _, server := range servers {
		hosts[server.Port()] = append(hosts[server.Port()], "host "+server.Addr().String())
	}
	for _, port := range slices.Sorted(maps.Keys(hosts)) {
		terms = append(terms, fmt.Sprintf("(port %d and (%s))", port, strings.Join(hosts[port], " or ")))
	}
	return strings.Join(terms, " or ")
}

// dnsCaptureFilterFits reports whether expr compiles to a program within
// dnsCaptureFilterMaxInstructions. Programs differ by a few instructions at
// most across link types, so Ethernet stands in for all.
func dnsCaptureFilterFits(expr string) bool {
	instructions, err := pcap.CompileBPFFilter(layers.LinkTypeEthernet, dnsSnapLen, expr)
	return err == nil && len(instructions) <= dnsCaptureFilterMaxInstructions
}
