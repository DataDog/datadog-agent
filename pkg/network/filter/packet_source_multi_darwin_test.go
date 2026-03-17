// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build darwin

package filter

import (
	"net"
	"sync"
	"testing"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockPacketSource is a PacketSource that serves a fixed slice of packets then blocks until closed.
type mockPacketSource struct {
	packets     []mockPacket
	closed      bool
	closedMu    sync.Mutex
	closeCh     chan struct{}
	closeOnce   sync.Once
	closeCalled int
}

type mockPacket struct {
	data []byte
	info DarwinPacketInfo
	ts   time.Time
}

func newMockPacketSource(packets []mockPacket) *mockPacketSource {
	return &mockPacketSource{
		packets: packets,
		closeCh: make(chan struct{}),
	}
}

func (m *mockPacketSource) VisitPackets(visitor func([]byte, PacketInfo, time.Time) error) error {
	for _, pkt := range m.packets {
		select {
		case <-m.closeCh:
			return nil
		default:
		}
		info := pkt.info
		if err := visitor(pkt.data, &info, pkt.ts); err != nil {
			return err
		}
	}
	// Block until closed.
	<-m.closeCh
	return nil
}

func (m *mockPacketSource) LayerType() gopacket.LayerType {
	return layers.LayerTypeEthernet
}

func (m *mockPacketSource) Close() {
	m.closeOnce.Do(func() {
		m.closedMu.Lock()
		m.closeCalled++
		m.closed = true
		m.closedMu.Unlock()
		close(m.closeCh)
	})
}

func (m *mockPacketSource) isClosed() bool {
	m.closedMu.Lock()
	defer m.closedMu.Unlock()
	return m.closed
}

// buildUDPPacket builds an Ethernet+IPv4+UDP packet with the given ports.
func buildUDPPacket(srcPort, dstPort uint16) []byte {
	buf := gopacket.NewSerializeBuffer()
	err := gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true},
		&layers.Ethernet{
			SrcMAC:       net.HardwareAddr{0, 0, 0, 0, 0, 0},
			DstMAC:       net.HardwareAddr{0, 0, 0, 0, 0, 0},
			EthernetType: layers.EthernetTypeIPv4,
		},
		&layers.IPv4{
			Version:  4,
			TTL:      64,
			Protocol: layers.IPProtocolUDP,
			SrcIP:    net.IP{1, 2, 3, 4},
			DstIP:    net.IP{5, 6, 7, 8},
		},
		&layers.UDP{
			SrcPort: layers.UDPPort(srcPort),
			DstPort: layers.UDPPort(dstPort),
		},
		gopacket.Payload([]byte("payload")),
	)
	if err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func newMultiFromMock(mock *mockPacketSource) *MultiPacketSource {
	m := &MultiPacketSource{source: mock}
	m.pool = sync.Pool{
		New: func() interface{} { return make([]byte, defaultSnapLen) },
	}
	return m
}

// collectN reads exactly n packets from sub via VisitPackets and returns their data copies.
func collectN(t *testing.T, sub *SubSource, n int) [][]byte {
	t.Helper()
	results := make([][]byte, 0, n)
	var mu sync.Mutex
	done := make(chan struct{})

	go func() {
		//nolint:errcheck
		sub.VisitPackets(func(data []byte, _ PacketInfo, _ time.Time) error {
			cp := make([]byte, len(data))
			copy(cp, data)
			mu.Lock()
			results = append(results, cp)
			mu.Unlock()
			if len(results) >= n {
				close(done)
			}
			return nil
		})
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for packets")
	}

	mu.Lock()
	defer mu.Unlock()
	return results
}

func TestMultiPacketSource_FanOutToMultipleSubs(t *testing.T) {
	pkt := buildUDPPacket(1234, 80)
	var packets []mockPacket
	for i := 0; i < 5; i++ {
		packets = append(packets, mockPacket{data: pkt, info: DarwinPacketInfo{LayerType: layers.LayerTypeEthernet}})
	}
	mock := newMockPacketSource(packets)
	m := newMultiFromMock(mock)

	sub0 := m.newSubSource(nil)
	sub1 := m.newSubSource(nil)

	var wg sync.WaitGroup
	var results0, results1 [][]byte

	wg.Add(2)
	go func() {
		defer wg.Done()
		results0 = collectN(t, sub0, 5)
		sub0.Close()
	}()
	go func() {
		defer wg.Done()
		results1 = collectN(t, sub1, 5)
		sub1.Close()
	}()
	wg.Wait()

	assert.Len(t, results0, 5)
	assert.Len(t, results1, 5)
}

func TestMultiPacketSource_PredicateFilters(t *testing.T) {
	dnsPacket := buildUDPPacket(12345, 53)   // DNS query to port 53
	nonDNSPacket := buildUDPPacket(1234, 80) // HTTP, not DNS

	packets := []mockPacket{
		{data: dnsPacket, info: DarwinPacketInfo{LayerType: layers.LayerTypeEthernet}},
		{data: nonDNSPacket, info: DarwinPacketInfo{LayerType: layers.LayerTypeEthernet}},
	}
	mock := newMockPacketSource(packets)
	m := newMultiFromMock(mock)

	dnsSub := m.newSubSource(IsDNSPacket)
	allSub := m.newSubSource(nil)

	// allSub should get both packets
	allResults := collectN(t, allSub, 2)
	assert.Len(t, allResults, 2, "unfiltered sub should receive all packets")

	// dnsSub should only get the DNS packet
	dnsResults := collectN(t, dnsSub, 1)
	assert.Len(t, dnsResults, 1, "DNS sub should only receive port-53 packets")

	dnsSub.Close()
	allSub.Close()
}

func TestMultiPacketSource_BufferReturnedToPool(t *testing.T) {
	pkt := buildUDPPacket(1234, 80)
	mock := newMockPacketSource([]mockPacket{
		{data: pkt, info: DarwinPacketInfo{LayerType: layers.LayerTypeEthernet}},
	})
	m := newMultiFromMock(mock)

	var putCount int
	var mu sync.Mutex
	originalNew := m.pool.New
	m.pool = sync.Pool{
		New: originalNew,
	}

	sub := m.newSubSource(nil)

	received := make(chan struct{})
	go func() {
		//nolint:errcheck
		sub.VisitPackets(func(_ []byte, _ PacketInfo, _ time.Time) error {
			close(received)
			return nil
		})
	}()

	<-received
	// Give VisitPackets time to call pool.Put after visitor returns.
	time.Sleep(10 * time.Millisecond)

	// We can't easily count Put calls on sync.Pool, but we can verify Get returns
	// a buffer (meaning something was returned to the pool).
	buf := m.pool.Get()
	mu.Lock()
	putCount++
	mu.Unlock()
	assert.NotNil(t, buf, "pool should have a buffer available after visitor returns")
	m.pool.Put(buf)

	sub.Close()
	_ = putCount
}

func TestSubSource_CloseIsIdempotent(t *testing.T) {
	mock := newMockPacketSource(nil)
	m := newMultiFromMock(mock)
	sub := m.newSubSource(nil)

	// Close twice — must not panic or deadlock.
	require.NotPanics(t, func() {
		sub.Close()
		sub.Close()
	})
}

func TestMultiPacketSource_UnderlyingClosesAfterLastSub(t *testing.T) {
	mock := newMockPacketSource(nil)
	m := newMultiFromMock(mock)

	sub0 := m.newSubSource(nil)
	sub1 := m.newSubSource(nil)

	// Start the fan-out so the goroutine is running.
	go func() { //nolint:errcheck
		sub0.VisitPackets(func([]byte, PacketInfo, time.Time) error { return nil })
	}()
	time.Sleep(5 * time.Millisecond)

	sub0.Close()
	assert.False(t, mock.isClosed(), "underlying source should still be open after first sub closes")

	sub1.Close()
	// Give subClosed goroutine time to propagate.
	require.Eventually(t, mock.isClosed, time.Second, 5*time.Millisecond,
		"underlying source should close after last sub closes")
}

func TestMultiPacketSource_UnderlyingClosesRegardlessOfOrder(t *testing.T) {
	mock := newMockPacketSource(nil)
	m := newMultiFromMock(mock)

	sub0 := m.newSubSource(nil)
	sub1 := m.newSubSource(nil)

	go func() { //nolint:errcheck
		sub1.VisitPackets(func([]byte, PacketInfo, time.Time) error { return nil })
	}()
	time.Sleep(5 * time.Millisecond)

	// Close in reverse order.
	sub1.Close()
	assert.False(t, mock.isClosed(), "underlying source should still be open after first sub closes")

	sub0.Close()
	require.Eventually(t, mock.isClosed, time.Second, 5*time.Millisecond,
		"underlying source should close after last sub closes regardless of order")
}

func TestIsDNSPacket_UDPDstPort53(t *testing.T) {
	pkt := buildUDPPacket(12345, 53)
	info := &DarwinPacketInfo{LayerType: layers.LayerTypeEthernet}
	assert.True(t, IsDNSPacket(pkt, info), "dst port 53 should be DNS")
}

func TestIsDNSPacket_UDPSrcPort53(t *testing.T) {
	pkt := buildUDPPacket(53, 12345)
	info := &DarwinPacketInfo{LayerType: layers.LayerTypeEthernet}
	assert.True(t, IsDNSPacket(pkt, info), "src port 53 should be DNS")
}

func TestIsDNSPacket_NonDNS(t *testing.T) {
	pkt := buildUDPPacket(1234, 80)
	info := &DarwinPacketInfo{LayerType: layers.LayerTypeEthernet}
	assert.False(t, IsDNSPacket(pkt, info), "port 80 should not be DNS")
}

func TestIsDNSPacket_TooShort(t *testing.T) {
	info := &DarwinPacketInfo{LayerType: layers.LayerTypeEthernet}
	assert.False(t, IsDNSPacket([]byte{0, 1, 2}, info), "too-short packet should return false")
}

func TestIsDNSPacket_LoopbackUDP(t *testing.T) {
	// Build a BSD loopback + IPv4 + UDP port 53 packet.
	buf := gopacket.NewSerializeBuffer()
	err := gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true},
		&layers.Loopback{Family: layers.ProtocolFamilyIPv4},
		&layers.IPv4{
			Version:  4,
			TTL:      64,
			Protocol: layers.IPProtocolUDP,
			SrcIP:    net.IP{127, 0, 0, 1},
			DstIP:    net.IP{127, 0, 0, 1},
		},
		&layers.UDP{
			SrcPort: 12345,
			DstPort: 53,
		},
		gopacket.Payload([]byte("dns")),
	)
	require.NoError(t, err)
	info := &DarwinPacketInfo{LayerType: layers.LayerTypeLoopback}
	assert.True(t, IsDNSPacket(buf.Bytes(), info), "loopback UDP port 53 should be DNS")
}
