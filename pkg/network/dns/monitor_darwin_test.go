// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build darwin

package dns

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/filter"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// ============================================================================
// Mock PacketSource
// ============================================================================

// mockDNSPacket holds a pre-built packet and its metadata.
type mockDNSPacket struct {
	data      []byte
	layerType gopacket.LayerType
	ts        time.Time
}

// mockSubSource implements filter.PacketSource and serves a fixed set of
// packets then blocks until Close() is called.
type mockSubSource struct {
	packets   []mockDNSPacket
	exit      chan struct{}
	drained   chan struct{} // closed once all packets have been visited
	closeOnce sync.Once
	drainOnce sync.Once
}

func newMockSubSource(packets []mockDNSPacket) *mockSubSource {
	return &mockSubSource{
		packets: packets,
		exit:    make(chan struct{}),
		drained: make(chan struct{}),
	}
}

func (m *mockSubSource) VisitPackets(visitor func(data []byte, info filter.PacketInfo, timestamp time.Time) error) error {
	for _, pkt := range m.packets {
		select {
		case <-m.exit:
			return nil
		default:
		}
		info := &filter.DarwinPacketInfo{PktType: 0, LayerType: pkt.layerType}
		if err := visitor(pkt.data, info, pkt.ts); err != nil {
			return err
		}
	}
	// Signal that all packets have been visited.
	m.drainOnce.Do(func() { close(m.drained) })
	// Block until Close is called.
	<-m.exit
	return nil
}

func (m *mockSubSource) LayerType() gopacket.LayerType {
	return layers.LayerTypeEthernet
}

func (m *mockSubSource) Close() {
	m.closeOnce.Do(func() {
		close(m.exit)
	})
}

// ============================================================================
// Packet builder helpers
// ============================================================================

// buildEthernetDNSResponse builds an Ethernet+IPv4+UDP packet containing a
// DNS A-record response mapping domain → ip.
func buildEthernetDNSResponse(domain string, ip net.IP) []byte {
	dnsLayer := &layers.DNS{
		ID:           1,
		QR:           true, // response
		OpCode:       layers.DNSOpCodeQuery,
		AA:           false,
		TC:           false,
		RD:           true,
		RA:           true,
		ResponseCode: layers.DNSResponseCodeNoErr,
		Questions: []layers.DNSQuestion{
			{
				Name:  []byte(domain),
				Type:  layers.DNSTypeA,
				Class: layers.DNSClassIN,
			},
		},
		Answers: []layers.DNSResourceRecord{
			{
				Name:  []byte(domain),
				Type:  layers.DNSTypeA,
				Class: layers.DNSClassIN,
				TTL:   300,
				IP:    ip,
			},
		},
	}

	ipLayer := &layers.IPv4{
		Version:  4,
		TTL:      64,
		Protocol: layers.IPProtocolUDP,
		SrcIP:    net.IP{8, 8, 8, 8},  // DNS server
		DstIP:    net.IP{10, 0, 0, 1}, // client
	}
	udpLayer := &layers.UDP{
		SrcPort: 53,
		DstPort: 12345,
	}
	// SetNetworkLayerForChecksum is required when ComputeChecksums is true.
	if err := udpLayer.SetNetworkLayerForChecksum(ipLayer); err != nil {
		panic(err)
	}

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	err := gopacket.SerializeLayers(buf, opts,
		&layers.Ethernet{
			SrcMAC:       net.HardwareAddr{0xde, 0xad, 0xbe, 0xef, 0x00, 0x01},
			DstMAC:       net.HardwareAddr{0xde, 0xad, 0xbe, 0xef, 0x00, 0x02},
			EthernetType: layers.EthernetTypeIPv4,
		},
		ipLayer,
		udpLayer,
		dnsLayer,
	)
	if err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// buildNonDNSPacket builds an Ethernet+IPv4+UDP packet on port 80 (not DNS).
func buildNonDNSPacket() []byte {
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true}
	err := gopacket.SerializeLayers(buf, opts,
		&layers.Ethernet{
			SrcMAC:       net.HardwareAddr{0, 0, 0, 0, 0, 0},
			DstMAC:       net.HardwareAddr{0, 0, 0, 0, 0, 0},
			EthernetType: layers.EthernetTypeIPv4,
		},
		&layers.IPv4{
			Version:  4,
			TTL:      64,
			Protocol: layers.IPProtocolUDP,
			SrcIP:    net.IP{10, 0, 0, 1},
			DstIP:    net.IP{10, 0, 0, 2},
		},
		&layers.UDP{
			SrcPort: 1234,
			DstPort: 80,
		},
		gopacket.Payload([]byte("http data")),
	)
	if err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// ============================================================================
// Test helpers
// ============================================================================

// newTestMonitor constructs a darwinDNSMonitor with default config and the
// given mock source, suitable for unit tests.
func newTestMonitor(t *testing.T, src filter.PacketSource) *darwinDNSMonitor {
	t.Helper()
	cfg := config.New()
	m, err := newDarwinDNSMonitorWithSource(cfg, src)
	require.NoError(t, err)
	return m
}

// ============================================================================
// Tests
// ============================================================================

func TestDarwinDNSMonitor_EthernetDNSResponse(t *testing.T) {
	answerIP := net.IP{1, 2, 3, 4}
	pkt := buildEthernetDNSResponse("example.com", answerIP)

	src := newMockSubSource([]mockDNSPacket{
		{data: pkt, layerType: layers.LayerTypeEthernet, ts: time.Now()},
	})
	m := newTestMonitor(t, src)
	defer m.Close()

	// Wait until the cache has at least one entry.
	require.Eventually(t, func() bool {
		return m.cache.Len() >= 1
	}, 5*time.Second, 10*time.Millisecond, "cache should contain the resolved DNS entry")

	addr := util.AddressFromNetIP(answerIP)
	names := m.Resolve(map[util.Address]struct{}{addr: {}})
	require.Contains(t, names, addr, "resolved map should contain the answer IP")
	assert.Contains(t, names[addr], ToHostname("example.com"), "example.com should be in the resolved names")
}

func TestDarwinDNSMonitor_AddDomain(t *testing.T) {
	answerIP := net.IP{1, 2, 3, 4}
	src := newMockSubSource([]mockDNSPacket{
		{data: buildEthernetDNSResponse("example.com", answerIP), layerType: layers.LayerTypeEthernet, ts: time.Now()},
	})
	m := newTestMonitor(t, src)
	defer m.Close()
	require.Eventually(t, func() bool {
		return m.cache.Len() >= 1
	}, 5*time.Second, 10*time.Millisecond, "cache should contain the resolved DNS entry")

	addr := util.AddressFromNetIP(answerIP)
	m.AddDomain(addr, "api.example.com")
	m.AddDomain(addr, "example.com")

	names := m.Resolve(map[util.Address]struct{}{addr: {}})
	assert.ElementsMatch(t, []Hostname{ToHostname("example.com"), ToHostname("api.example.com")}, names[addr])
}

func TestDarwinDNSMonitor_IsDNSServer(t *testing.T) {
	cfg := config.New()
	cfg.DNSMonitoringPortList = []int{5300}
	m, err := newDarwinDNSMonitorWithSource(cfg, newMockSubSource(nil))
	require.NoError(t, err)
	defer m.Close()
	server := util.AddressFromString("208.67.220.220")
	other := util.AddressFromString("192.0.2.1")
	m.servers.Store(&[]netip.AddrPort{netip.AddrPortFrom(server.Addr, 443)})

	assert.True(t, m.IsDNSServer(other, 53), "port 53 serves DNS even when the configured ports leave it out")
	assert.True(t, m.IsDNSServer(other, 5300))
	assert.True(t, m.IsDNSServer(server, 443))
	assert.True(t, m.IsDNSServer(util.Address{Addr: netip.AddrFrom16(server.Addr.As16())}, 443), "an IPv4-mapped address matches its IPv4 server")
	assert.False(t, m.IsDNSServer(other, 443), "the port of a system DNS server serves DNS only at its address")
	assert.False(t, m.IsDNSServer(server, 8443))
}

func TestDarwinDNSMonitor_RefreshDNSServers(t *testing.T) {
	m := newTestMonitor(t, newMockSubSource(nil))
	defer m.Close()
	addr := util.AddressFromString("208.67.220.220")
	var listed []netip.AddrPort
	var listErr error
	list := func() ([]netip.AddrPort, error) { return listed, listErr }
	var filters []string
	setFilter := func(expr string) { filters = append(filters, expr) }

	listed = []netip.AddrPort{netip.AddrPortFrom(addr.Addr, 443)}
	require.NoError(t, m.refreshDNSServers(list, setFilter))
	require.NoError(t, m.refreshDNSServers(list, setFilter))
	assert.Equal(t, []string{"port 53 or (port 443 and (host 208.67.220.220))"}, filters, "unchanged servers set no filter")
	assert.True(t, m.IsDNSServer(addr, 443))

	listErr = errors.New("scutil failed")
	require.ErrorIs(t, m.refreshDNSServers(list, setFilter), listErr)
	assert.Len(t, filters, 1)
	assert.True(t, m.IsDNSServer(addr, 443), "a failed listing keeps the servers")

	listed, listErr = nil, nil
	require.NoError(t, m.refreshDNSServers(list, setFilter))
	assert.Equal(t, []string{"port 53 or (port 443 and (host 208.67.220.220))", "port 53"}, filters)
	assert.False(t, m.IsDNSServer(addr, 443))
}

func TestDarwinDNSMonitor_CloseStopsWatchingDNSServers(t *testing.T) {
	m := newTestMonitor(t, newMockSubSource(nil))
	m.watchDNSServers(func() ([]netip.AddrPort, error) { return nil, nil }, func(string) {})

	done := make(chan struct{})
	go func() {
		m.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Close() did not return within timeout")
	}
}

func TestDNSCaptureFilter(t *testing.T) {
	servers := func(addrPorts ...string) []netip.AddrPort {
		var parsed []netip.AddrPort
		for _, addrPort := range addrPorts {
			parsed = append(parsed, netip.MustParseAddrPort(addrPort))
		}
		return parsed
	}
	for _, tc := range []struct {
		name    string
		ports   []int
		servers []netip.AddrPort
		want    string
	}{
		{"no ports", nil, nil, "port 53"},
		{"default ports", []int{53}, nil, "port 53"},
		{"configured ports", []int{53, 5300}, nil, "port 53 or port 5300"},
		{"server", []int{53}, servers("208.67.220.220:443"), "port 53 or (port 443 and (host 208.67.220.220))"},
		{"IPv6 server", []int{53}, servers("[2620:119:35::35]:443"), "port 53 or (port 443 and (host 2620:119:35::35))"},
		{"servers on one port", []int{53}, servers("192.0.2.1:5300", "[2001:db8::53]:5300"), "port 53 or (port 5300 and (host 192.0.2.1 or host 2001:db8::53))"},
		{"servers on two ports", []int{53}, servers("192.0.2.1:5300", "198.51.100.1:443"), "port 53 or (port 443 and (host 198.51.100.1)) or (port 5300 and (host 192.0.2.1))"},
		{"server on a configured port", []int{53, 5300}, servers("192.0.2.1:5300"), "port 53 or port 5300"},
		{"server without ports", nil, servers("192.0.2.1:5300"), "port 53 or (port 5300 and (host 192.0.2.1))"},
		{"loopback servers", []int{53}, servers("127.0.0.1:5300", "[::1]:5300"), "port 53"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expr, left := dnsCaptureFilter(tc.ports, tc.servers)
			assert.Equal(t, tc.want, expr)
			assert.Empty(t, left)
		})
	}
}

func TestDNSCaptureFilterCountsOnlyServersWithTerms(t *testing.T) {
	ports := []int{53, 5300}
	// Sorted servers put these two first, and neither needs a term of its own.
	servers := []netip.AddrPort{
		netip.MustParseAddrPort("127.0.0.1:5353"),
		netip.MustParseAddrPort("192.0.2.1:5300"),
	}
	for i := range dnsServersMax {
		servers = append(servers, netip.MustParseAddrPort(fmt.Sprintf("198.51.100.%d:443", i+1)))
	}
	listed := slices.Clone(servers)

	expr, left := dnsCaptureFilter(ports, servers)
	assert.Equal(t, listed, servers, "the monitor's list of servers is left as it was")
	assert.Empty(t, left)
	assert.Equal(t, dnsServersMax, strings.Count(expr, "host "))
	assert.NotContains(t, expr, "127.0.0.1")
	assert.NotContains(t, expr, "192.0.2.1")
}

func TestDNSCaptureFilterLeavesOutServersPastTheMax(t *testing.T) {
	var servers []netip.AddrPort
	for i := range dnsServersMax + 2 {
		servers = append(servers, netip.MustParseAddrPort(fmt.Sprintf("198.51.100.%d:443", i+1)))
	}

	expr, left := dnsCaptureFilter([]int{53}, servers)
	assert.Equal(t, dnsServersMax, strings.Count(expr, "host "))
	assert.Equal(t, servers[dnsServersMax:], left)
}

func TestDNSCaptureFilterFitsTheKernel(t *testing.T) {
	// BPF_MAXINSNS of the macOS kernel, which rejects longer filters.
	const bpfMaxInstructions = 512
	// With many configured ports, IPv6 servers on ports of their own need more
	// instructions than the filter has room for.
	var ports []int
	for i := range 24 {
		ports = append(ports, 1053+i)
	}
	var servers []netip.AddrPort
	for i := range dnsServersMax {
		servers = append(servers, netip.MustParseAddrPort(fmt.Sprintf("[2001:db8::%x]:%d", i+1, 6000+i)))
	}

	expr, left := dnsCaptureFilter(ports, servers)
	require.NotEmpty(t, left)
	captured := len(servers) - len(left)
	assert.Equal(t, servers[captured:], left)
	assert.Equal(t, captured, strings.Count(expr, "host "))
	assert.False(t, dnsCaptureFilterFits(dnsCaptureFilterExpr(ports, servers[:captured+1])), "one more server would not fit")
	for _, linkType := range []layers.LinkType{layers.LinkTypeEthernet, layers.LinkTypeNull} {
		instructions, err := pcap.CompileBPFFilter(linkType, dnsSnapLen, expr)
		require.NoError(t, err, linkType)
		assert.LessOrEqual(t, len(instructions), bpfMaxInstructions, linkType)
	}
}

func TestDarwinDNSMonitor_LoopbackDNSResponse(t *testing.T) {
	// Build an Ethernet-framed DNS response but tag it as LayerTypeLoopback.
	// The loopbackParser will be selected but the packet format doesn't match,
	// so ParseInto will return a decode error — the monitor should handle
	// this gracefully (no panic, no crash) and simply skip the packet.
	answerIP := net.IP{127, 0, 0, 1}
	pkt := buildEthernetDNSResponse("local.test", answerIP)

	src := newMockSubSource([]mockDNSPacket{
		{data: pkt, layerType: layers.LayerTypeLoopback, ts: time.Now()},
	})
	m := newTestMonitor(t, src)
	defer m.Close()

	// Wait until all packets have been visited before asserting.
	select {
	case <-src.drained:
	case <-time.After(5 * time.Second):
		t.Fatal("mock source did not drain within timeout")
	}

	// The cache should remain empty because the loopback parser cannot parse
	// an Ethernet-framed packet.
	assert.Equal(t, 0, m.cache.Len(), "cache should be empty after a malformed loopback packet")
}

func TestDarwinDNSMonitor_NonDNSIgnored(t *testing.T) {
	pkt := buildNonDNSPacket()

	src := newMockSubSource([]mockDNSPacket{
		{data: pkt, layerType: layers.LayerTypeEthernet, ts: time.Now()},
	})
	m := newTestMonitor(t, src)
	defer m.Close()

	// Wait until all packets have been visited before asserting.
	select {
	case <-src.drained:
	case <-time.After(5 * time.Second):
		t.Fatal("mock source did not drain within timeout")
	}

	assert.Equal(t, 0, m.cache.Len(), "non-DNS packet should not populate the cache")
}

func TestDarwinDNSMonitor_Close(t *testing.T) {
	src := newMockSubSource(nil) // no packets — blocks immediately
	m := newTestMonitor(t, src)

	done := make(chan struct{})
	go func() {
		m.Close()
		close(done)
	}()

	select {
	case <-done:
		// expected
	case <-time.After(5 * time.Second):
		t.Fatal("Close() did not return within timeout")
	}
}

func TestDarwinDNSMonitor_CloseIdempotent(t *testing.T) {
	src := newMockSubSource(nil)
	m := newTestMonitor(t, src)

	require.NotPanics(t, func() {
		m.Close()
		m.Close()
	})
}

func TestDarwinDNSMonitor_StartIsNoOp(t *testing.T) {
	src := newMockSubSource(nil)
	m := newTestMonitor(t, src)
	defer m.Close()

	err := m.Start()
	assert.NoError(t, err, "Start() should be a no-op and return nil")
}

func TestDarwinDNSMonitor_GetDNSStats_NilWhenDisabled(t *testing.T) {
	src := newMockSubSource(nil)
	cfg := config.New()
	cfg.CollectDNSStats = false
	m, err := newDarwinDNSMonitorWithSource(cfg, src)
	require.NoError(t, err)
	defer m.Close()

	stats := m.GetDNSStats()
	assert.Nil(t, stats, "GetDNSStats should return nil when stats collection is disabled")
}

func TestDarwinDNSMonitor_WaitForDomain_NilStatKeeper(t *testing.T) {
	src := newMockSubSource(nil)
	cfg := config.New()
	cfg.CollectDNSStats = false
	m, err := newDarwinDNSMonitorWithSource(cfg, src)
	require.NoError(t, err)
	defer m.Close()

	// Should return nil immediately when statKeeper is nil.
	err = m.WaitForDomain("anything.test")
	assert.NoError(t, err)
}

func TestDarwinDNSMonitor_MultipleDNSResponses(t *testing.T) {
	ip1 := net.IP{1, 2, 3, 4}
	ip2 := net.IP{5, 6, 7, 8}

	src := newMockSubSource([]mockDNSPacket{
		{data: buildEthernetDNSResponse("foo.example.com", ip1), layerType: layers.LayerTypeEthernet, ts: time.Now()},
		{data: buildEthernetDNSResponse("bar.example.com", ip2), layerType: layers.LayerTypeEthernet, ts: time.Now()},
	})
	m := newTestMonitor(t, src)
	defer m.Close()

	require.Eventually(t, func() bool {
		return m.cache.Len() >= 2
	}, 5*time.Second, 10*time.Millisecond, "both DNS entries should be cached")

	addr1 := util.AddressFromNetIP(ip1)
	addr2 := util.AddressFromNetIP(ip2)
	names := m.Resolve(map[util.Address]struct{}{addr1: {}, addr2: {}})

	require.Contains(t, names, addr1)
	assert.Contains(t, names[addr1], ToHostname("foo.example.com"))
	require.Contains(t, names, addr2)
	assert.Contains(t, names[addr2], ToHostname("bar.example.com"))
}
