// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build darwin

// Package filter exposes interfaces and implementations for packet capture
package filter

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	telemetryModuleName = "network_tracer__filter"
	defaultSnapLen      = 4096
	pcapTimeout         = time.Second

	// localAddrRefreshInterval is how often we refresh local addresses
	localAddrRefreshInterval = 30 * time.Second

	// packetChannelSize is the buffer size for the merged packet channel
	packetChannelSize = 1000
)

// Telemetry
var packetSourceTelemetry = struct {
	processed *telemetry.StatCounterWrapper
	captured  *telemetry.StatCounterWrapper
	dropped   *telemetry.StatCounterWrapper
}{
	telemetry.NewStatCounterWrapper(telemetryModuleName, "processed_packets", []string{}, "Counter measuring the number of processed packets"),
	telemetry.NewStatCounterWrapper(telemetryModuleName, "captured_packets", []string{}, "Counter measuring the number of captured packets"),
	telemetry.NewStatCounterWrapper(telemetryModuleName, "dropped_packets", []string{}, "Counter measuring the number of dropped packets"),
}

// bufferPool is used to reuse packet buffers and reduce GC pressure
// All buffers have the same capacity (defaultSnapLen) since libpcap truncates
// packets at snapLen anyway, guaranteeing all packets fit
var bufferPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, defaultSnapLen)
	},
}

// getBuffer retrieves a snapLen-sized buffer from the pool
func getBuffer() []byte {
	return bufferPool.Get().([]byte)
}

// putBuffer returns a buffer to the pool
func putBuffer(buf []byte) {
	bufferPool.Put(buf)
}

// packetWithInfo wraps copied packet data with metadata
type packetWithInfo struct {
	data      []byte // Copied data from pool, caller must return via putBuffer
	timestamp time.Time
	direction uint8 // PACKET_HOST or PACKET_OUTGOING
}

// interfaceHandle holds a pcap handle and its associated local addresses
type interfaceHandle struct {
	handle    *pcap.Handle
	ifaceName string

	// localAddrs contains the local IP addresses for this interface
	localAddrs   map[string]struct{}
	localAddrsMu sync.RWMutex
}

// LibpcapSource provides packet capture using libpcap/BPF on macOS
type LibpcapSource struct {
	interfaces []*interfaceHandle
	snapLen    int

	exit chan struct{}

	// packetChan is a persistent channel fed by reader goroutines started once
	// in the constructor. VisitPackets drains this channel.
	packetChan chan packetWithInfo
	// errChan receives errors from reader goroutines
	errChan chan error
	// readerWg tracks reader goroutines so Close() can wait for them to finish
	// before closing pcap handles
	readerWg sync.WaitGroup
}

// DarwinPacketInfo holds information about a packet on Darwin
type DarwinPacketInfo struct {
	// PktType indicates packet direction
	// PACKET_HOST (0) for incoming, PACKET_OUTGOING (4) for outgoing
	PktType uint8
}

// OptSnapLen specifies the maximum length of the packet to read
//
// Defaults to 4096 bytes
type OptSnapLen int

// NewLibpcapSource creates a LibpcapSource using libpcap
func NewLibpcapSource(size int, opts ...interface{}) (*LibpcapSource, error) {
	snapLen := defaultSnapLen
	for _, opt := range opts {
		switch o := opt.(type) {
		case OptSnapLen:
			snapLen = int(o)
			if snapLen <= 0 || snapLen > 65536 {
				return nil, fmt.Errorf("snap len should be between 0 and 65536")
			}
		default:
			return nil, fmt.Errorf("unknown option %+v", opt)
		}
	}

	// TODO: Make this configurable - for now just use en0
	ifaceNames := []string{"en0"}

	ps := &LibpcapSource{
		interfaces: make([]*interfaceHandle, 0, len(ifaceNames)),
		snapLen:    snapLen,
		exit:       make(chan struct{}),
		packetChan: make(chan packetWithInfo, packetChannelSize),
		errChan:    make(chan error, len(ifaceNames)),
	}

	// Open a handle for each interface
	for _, ifaceName := range ifaceNames {
		ih, err := ps.openInterface(ifaceName, snapLen)
		if err != nil {
			// Clean up any already-opened handles
			for _, existing := range ps.interfaces {
				existing.handle.Close()
			}
			return nil, err
		}
		ps.interfaces = append(ps.interfaces, ih)
	}

	// Start persistent reader goroutines (one per interface)
	for _, ih := range ps.interfaces {
		ps.readerWg.Add(1)
		go func(ih *interfaceHandle) {
			defer ps.readerWg.Done()
			if err := ps.readPacketsFromInterface(ih); err != nil {
				select {
				case ps.errChan <- fmt.Errorf("interface %s error: %w", ih.ifaceName, err):
				case <-ps.exit:
				}
			}
		}(ih)
	}

	// Start background goroutines
	go ps.pollStats()
	go ps.refreshLocalAddrsLoop()

	log.Infof("created libpcap source on %d interfaces, snaplen=%d", len(ps.interfaces), snapLen)
	return ps, nil
}

// openInterface opens a pcap handle on the specified interface
func (p *LibpcapSource) openInterface(ifaceName string, snapLen int) (*interfaceHandle, error) {
	handle, err := pcap.OpenLive(ifaceName, int32(snapLen), true, pcapTimeout)
	if err != nil {
		return nil, fmt.Errorf("error opening pcap handle on %s: %w", ifaceName, err)
	}

	// Set BPF filter to only capture TCP and UDP packets
	if err := handle.SetBPFFilter("tcp or udp"); err != nil {
		handle.Close()
		return nil, fmt.Errorf("error setting BPF filter on %s: %w", ifaceName, err)
	}

	ih := &interfaceHandle{
		handle:     handle,
		ifaceName:  ifaceName,
		localAddrs: make(map[string]struct{}),
	}

	// Initialize local addresses for direction detection
	if err := ih.refreshLocalAddrs(); err != nil {
		log.Warnf("failed to get local addresses for %s: %v", ifaceName, err)
	}

	log.Infof("opened pcap handle on interface %s", ifaceName)
	return ih, nil
}

// VisitPackets reads packets from the persistent channel and invokes the visitor callback for each.
// Reader goroutines are started once in the constructor, so calling VisitPackets multiple times
// (e.g. on retry) does not leak goroutines.
func (p *LibpcapSource) VisitPackets(visitor func(data []byte, info PacketInfo, timestamp time.Time) error) error {
	packetInfo := &DarwinPacketInfo{}

	// Process packets from the persistent channel
	for {
		select {
		case pkt := <-p.packetChan:
			packetInfo.PktType = pkt.direction

			// Call visitor with packet data
			err := visitor(pkt.data, packetInfo, pkt.timestamp)

			// Return buffer to pool after visitor completes
			putBuffer(pkt.data)

			if err != nil {
				return err
			}

			packetSourceTelemetry.processed.Add(1)

		case err := <-p.errChan:
			return err

		case <-p.exit:
			return nil
		}
	}
}

// readPacketsFromInterface reads packets from a single interface and sends them to the persistent channel.
// This runs for the lifetime of the LibpcapSource.
func (p *LibpcapSource) readPacketsFromInterface(ih *interfaceHandle) error {
	for {
		select {
		case <-p.exit:
			return nil
		default:
		}

		// Zero-copy read - buffer is only valid until next read
		data, ci, err := ih.handle.ZeroCopyReadPacketData()

		if err != nil {
			if err == pcap.NextErrorTimeoutExpired {
				continue
			}
			return fmt.Errorf("error reading packet: %w", err)
		}

		// Determine direction before copying
		direction := ih.determinePacketDirection(data)

		// Copy data to pooled buffer immediately (before buffer is reused)
		buf := getBuffer()
		buf = buf[:len(data)] // Resize to actual packet size
		copy(buf, data)

		// Send to channel
		select {
		case p.packetChan <- packetWithInfo{
			data:      buf,
			timestamp: ci.Timestamp,
			direction: direction,
		}:
		case <-p.exit:
			putBuffer(buf) // Don't leak buffer
			return nil
		}
	}
}

// LayerType returns the layer type for packets from this source
// Returns LayerTypeEthernet since libpcap on macOS returns full Ethernet frames
func (p *LibpcapSource) LayerType() gopacket.LayerType {
	return layers.LayerTypeEthernet
}

// Close stops packet capture and cleans up resources.
// It signals all goroutines to stop, waits for reader goroutines to finish,
// then closes pcap handles.
func (p *LibpcapSource) Close() {
	// Signal all goroutines to exit
	close(p.exit)

	// Wait for reader goroutines to finish before closing handles.
	// Readers check p.exit and will return after at most one pcap timeout (1s).
	p.readerWg.Wait()

	// Now safe to close handles - no goroutines are reading from them
	for _, ih := range p.interfaces {
		if ih.handle != nil {
			ih.handle.Close()
		}
	}
}

// refreshLocalAddrs updates the local addresses for a single interface
func (ih *interfaceHandle) refreshLocalAddrs() error {
	iface, err := net.InterfaceByName(ih.ifaceName)
	if err != nil {
		return fmt.Errorf("failed to get interface %s: %w", ih.ifaceName, err)
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return fmt.Errorf("failed to get addresses for %s: %w", ih.ifaceName, err)
	}

	ih.localAddrsMu.Lock()
	defer ih.localAddrsMu.Unlock()

	// Clear existing addresses
	ih.localAddrs = make(map[string]struct{})

	// Add each address to the set
	for _, addr := range addrs {
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if ip != nil {
			// Store as raw bytes for fast lookup
			if ip4 := ip.To4(); ip4 != nil {
				ih.localAddrs[string(ip4)] = struct{}{}
			} else {
				ih.localAddrs[string(ip.To16())] = struct{}{}
			}
		}
	}

	log.Debugf("refreshed local addresses for %s: %d addresses found", ih.ifaceName, len(ih.localAddrs))
	return nil
}

// refreshLocalAddrsLoop periodically refreshes the local address cache for all interfaces
func (p *LibpcapSource) refreshLocalAddrsLoop() {
	ticker := time.NewTicker(localAddrRefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			for _, ih := range p.interfaces {
				if err := ih.refreshLocalAddrs(); err != nil {
					log.Debugf("failed to refresh local addresses for %s: %v", ih.ifaceName, err)
				}
			}
		case <-p.exit:
			return
		}
	}
}

// isLocalAddr checks if an IP (as raw bytes) is a local address for this interface
func (ih *interfaceHandle) isLocalAddr(ip []byte) bool {
	ih.localAddrsMu.RLock()
	defer ih.localAddrsMu.RUnlock()

	_, exists := ih.localAddrs[string(ip)]
	return exists
}

// determinePacketDirection examines the packet's IP addresses to determine direction
// Returns PACKET_OUTGOING if source is local, PACKET_HOST if destination is local
func (ih *interfaceHandle) determinePacketDirection(data []byte) uint8 {
	// Need at least Ethernet header (14 bytes)
	if len(data) < 14 {
		return PACKET_HOST
	}

	// EtherType is at offset 12-13
	etherType := uint16(data[12])<<8 | uint16(data[13])

	switch etherType {
	case 0x0800: // IPv4
		if len(data) < 34 { // Ethernet (14) + IPv4 header minimum (20)
			return PACKET_HOST
		}
		// Source IP at offset 26-29 (14 + 12)
		// Destination IP at offset 30-33 (14 + 16)
		srcIP := data[26:30]
		dstIP := data[30:34]

		srcIsLocal := ih.isLocalAddr(srcIP)
		dstIsLocal := ih.isLocalAddr(dstIP)

		log.Debugf("srcIP: %s, dstIP: %s", net.IP(srcIP).String(), net.IP(dstIP).String())
		log.Debugf("srcIsLocal: %v, dstIsLocal: %v", srcIsLocal, dstIsLocal)

		if srcIsLocal && !dstIsLocal {
			log.Debugf("returning PACKET_OUTGOING")
			return PACKET_OUTGOING
		}
		return PACKET_HOST

	case 0x86DD: // IPv6
		if len(data) < 54 { // Ethernet (14) + IPv6 header minimum (40)
			return PACKET_HOST
		}
		// Source IP at offset 22-37 (14 + 8)
		// Destination IP at offset 38-53 (14 + 24)
		srcIP := data[22:38]
		dstIP := data[38:54]

		srcIsLocal := ih.isLocalAddr(srcIP)
		dstIsLocal := ih.isLocalAddr(dstIP)

		log.Debugf("srcIP: %s, dstIP: %s", net.IP(srcIP).String(), net.IP(dstIP).String())
		log.Debugf("srcIsLocal: %v, dstIsLocal: %v", srcIsLocal, dstIsLocal)

		if srcIsLocal && !dstIsLocal {
			log.Debugf("returning PACKET_OUTGOING")
			return PACKET_OUTGOING
		}
		return PACKET_HOST

	default:
		return PACKET_HOST
	}
}

// pollStats periodically polls capture statistics from all handles
func (p *LibpcapSource) pollStats() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Track previous stats per interface
	prevStats := make(map[string]struct{ captured, dropped uint64 })

	for {
		select {
		case <-ticker.C:
			for _, ih := range p.interfaces {
				stats, err := ih.handle.Stats()
				if err != nil {
					log.Debugf("error polling pcap stats for %s: %s", ih.ifaceName, err)
					continue
				}

				prev := prevStats[ih.ifaceName]
				captured := uint64(stats.PacketsReceived) - prev.captured
				dropped := uint64(stats.PacketsDropped) - prev.dropped

				if captured > 0 || dropped > 0 {
					log.Debugf("pcap stats (%s): captured=%d dropped=%d", ih.ifaceName, captured, dropped)
				}

				packetSourceTelemetry.captured.Add(int64(captured))
				packetSourceTelemetry.dropped.Add(int64(dropped))

				prevStats[ih.ifaceName] = struct{ captured, dropped uint64 }{
					captured: uint64(stats.PacketsReceived),
					dropped:  uint64(stats.PacketsDropped),
				}
			}

		case <-p.exit:
			return
		}
	}
}

var _ PacketSource = &LibpcapSource{}
