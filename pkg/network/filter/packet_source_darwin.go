// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build darwin

// Package filter exposes interfaces and implementations for packet capture
package filter

import (
	"errors"
	"fmt"
	"net"
	"strings"
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

	// localAddrRefreshInterval controls how often we discover new interfaces
	// and refresh local address caches. After a BPF error (e.g. interface
	// removal), capture on that interface stops immediately; if the interface
	// reappears it will be detected on the next tick (up to this interval).
	localAddrRefreshInterval = 30 * time.Second

	// statsInterval controls how often per-interface pcap stats are polled
	statsInterval = 5 * time.Second

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

// bufferPool is used to reuse packet buffers and reduce GC pressure.
// All buffers have the same capacity (defaultSnapLen) since libpcap truncates
// packets at snapLen anyway, guaranteeing all packets fit.
var bufferPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, defaultSnapLen)
	},
}

// getBuffer retrieves a defaultSnapLen-capacity buffer from the pool.
// The returned slice always has len == cap == defaultSnapLen.
func getBuffer() []byte {
	return bufferPool.Get().([]byte)
}

// putBuffer returns a buffer to the pool. The slice is always reset to its
// full capacity before pooling so that the next caller always receives a
// slice with len == cap == defaultSnapLen, preventing a panic when a future
// caller tries to reslice to a length larger than the stored len.
func putBuffer(buf []byte) {
	bufferPool.Put(buf[:cap(buf)])
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
	interfacesMu sync.RWMutex
	interfaces   map[string]*interfaceHandle // keyed by interface name
	snapLen      int

	exit      chan struct{}
	closeOnce sync.Once // ensures Close() is safe to call multiple times

	// packetChan is a persistent channel fed by reader goroutines.
	// VisitPackets drains this channel.
	packetChan chan packetWithInfo
	// readerWg tracks reader goroutines so Close() can wait for them to finish
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

// isEligibleInterface reports whether an interface should be captured.
// Skips loopback and bridge/vlan virtual interfaces.
func isEligibleInterface(iface net.Interface) bool {
	if iface.Flags&net.FlagLoopback != 0 {
		return false
	}
	name := iface.Name
	if strings.HasPrefix(name, "bridge") || strings.HasPrefix(name, "vlan") {
		return false
	}
	return true
}

// NewLibpcapSource creates a LibpcapSource using libpcap
func NewLibpcapSource(_ int, opts ...interface{}) (*LibpcapSource, error) {
	snapLen := defaultSnapLen
	for _, opt := range opts {
		switch o := opt.(type) {
		case OptSnapLen:
			snapLen = int(o)
			if snapLen <= 0 || snapLen > 65536 {
				return nil, errors.New("snap len should be between 0 and 65536")
			}
		default:
			return nil, fmt.Errorf("unknown option %+v", opt)
		}
	}

	ps := &LibpcapSource{
		interfaces: make(map[string]*interfaceHandle),
		snapLen:    snapLen,
		exit:       make(chan struct{}),
		packetChan: make(chan packetWithInfo, packetChannelSize),
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to list network interfaces: %w", err)
	}

	for _, iface := range ifaces {
		if !isEligibleInterface(iface) {
			continue
		}
		if err := ps.addInterface(iface.Name); err != nil {
			log.Warnf("skipping interface %s: %v", iface.Name, err)
		}
	}

	if len(ps.interfaces) == 0 {
		return nil, errors.New("no eligible network interfaces found for packet capture")
	}

	go ps.refreshLocalAddrs()

	log.Infof("created libpcap source on %d interfaces, snaplen=%d", len(ps.interfaces), snapLen)
	return ps, nil
}

// addInterface opens a pcap handle on ifaceName, registers it in p.interfaces,
// and starts a reader goroutine that owns the handle for its lifetime.
// When the reader exits for any reason it removes itself from the map and
// closes the handle, so the caller never needs to do that explicitly.
func (p *LibpcapSource) addInterface(ifaceName string) error {
	handle, err := pcap.OpenLive(ifaceName, int32(p.snapLen), true, pcapTimeout)
	if err != nil {
		return fmt.Errorf("error opening pcap handle on %s: %w", ifaceName, err)
	}

	if err := handle.SetBPFFilter("tcp or udp"); err != nil {
		handle.Close()
		return fmt.Errorf("error setting BPF filter on %s: %w", ifaceName, err)
	}

	ih := &interfaceHandle{
		handle:     handle,
		ifaceName:  ifaceName,
		localAddrs: make(map[string]struct{}),
	}

	if err := ih.refreshLocalAddrs(); err != nil {
		log.Warnf("failed to get local addresses for %s: %v", ifaceName, err)
	}

	p.interfacesMu.Lock()
	p.interfaces[ifaceName] = ih
	p.interfacesMu.Unlock()

	p.readerWg.Add(1)
	go func() {
		defer p.readerWg.Done()
		// Self-cleanup: remove from the map and close the handle regardless of
		// why this goroutine exits (global shutdown, interface removal, or error).
		defer func() {
			p.interfacesMu.Lock()

			delete(p.interfaces, ih.ifaceName)
			p.interfacesMu.Unlock()
			ih.handle.Close()
			log.Infof("closed pcap handle on interface %s", ih.ifaceName)
		}()

		p.readPacketsFromInterface(ih)
	}()

	log.Infof("opened pcap handle on interface %s", ifaceName)
	return nil
}

// VisitPackets reads packets from the persistent channel and invokes the visitor
// callback for each. The data slice and PacketInfo pointer passed to the visitor
// are only valid for the duration of the call and must not be retained.
func (p *LibpcapSource) VisitPackets(visitor func(data []byte, info PacketInfo, timestamp time.Time) error) error {
	packetInfo := &DarwinPacketInfo{}

	for {
		select {
		case pkt := <-p.packetChan:
			defer putBuffer(pkt.data)
			packetInfo.PktType = pkt.direction

			err := visitor(pkt.data, packetInfo, pkt.timestamp)
			if err != nil {
				return err
			}

			packetSourceTelemetry.processed.Add(1)

		case <-p.exit:
			return nil
		}
	}
}

// readPacketsFromInterface reads packets from a single interface and sends them
// to the shared packet channel. It also polls pcap stats on a periodic ticker,
// which keeps stats collection inside the goroutine that owns the handle,
// eliminating any use-after-close risk from an external stats goroutine.
//
// The function returns when p.exit is closed (clean shutdown) or when the
// underlying handle returns a non-timeout error (e.g. the interface was removed).
func (p *LibpcapSource) readPacketsFromInterface(ih *interfaceHandle) {
	statsTicker := time.NewTicker(statsInterval)
	defer statsTicker.Stop()

	prevStats := struct{ captured, dropped uint64 }{}

	for {
		// Check for shutdown or a stats tick before blocking on the next read.
		// ZeroCopyReadPacketData returns at least every pcapTimeout (1s) so
		// the ticker fires with at most 1s extra latency.
		select {
		case <-p.exit:
			return
		case <-statsTicker.C:
			p.collectStats(ih, &prevStats)
		default:
		}

		// Zero-copy read — buffer is only valid until next read
		data, ci, err := ih.handle.ZeroCopyReadPacketData()

		if err != nil {
			if err == pcap.NextErrorTimeoutExpired {
				continue
			}
			// Any other error (e.g. EIO when the interface is removed) means
			// this reader can no longer function. Log and exit so the deferred
			// cleanup runs; syncInterfaces will re-add the interface if it
			// reappears later.
			select {
			case <-p.exit:
				// Clean shutdown coincided with a read error — don't alarm.
			default:
				log.Warnf("pcap read error on interface %s, stopping capture: %v", ih.ifaceName, err)
			}
			return
		}

		direction := ih.determinePacketDirection(data)

		// Copy data to a pooled buffer immediately (before the zero-copy
		// buffer is reused by the next ZeroCopyReadPacketData call).
		buf := getBuffer()
		buf = buf[:len(data)]
		copy(buf, data)

		select {
		case p.packetChan <- packetWithInfo{
			data:      buf,
			timestamp: ci.Timestamp,
			direction: direction,
		}:
		case <-p.exit:
			putBuffer(buf)
			return
		}
	}
}

// collectStats polls pcap stats for ih and updates the telemetry counters.
// It is called from within the reader goroutine that owns ih.handle, so there
// is no risk of calling Stats() on a closed handle.
func (p *LibpcapSource) collectStats(ih *interfaceHandle, prev *struct{ captured, dropped uint64 }) {
	stats, err := ih.handle.Stats()
	if err != nil {
		log.Debugf("error polling pcap stats for %s: %s", ih.ifaceName, err)
		return
	}

	captured := uint64(stats.PacketsReceived) - prev.captured
	dropped := uint64(stats.PacketsDropped) - prev.dropped

	if captured > 0 || dropped > 0 {
		log.Debugf("pcap stats (%s): captured=%d dropped=%d", ih.ifaceName, captured, dropped)
	}

	packetSourceTelemetry.captured.Add(int64(captured))
	packetSourceTelemetry.dropped.Add(int64(dropped))

	prev.captured = uint64(stats.PacketsReceived)
	prev.dropped = uint64(stats.PacketsDropped)
}

// LayerType returns the layer type for packets from this source.
// Returns LayerTypeEthernet since libpcap on macOS returns full Ethernet frames.
func (p *LibpcapSource) LayerType() gopacket.LayerType {
	return layers.LayerTypeEthernet
}

// Close stops packet capture and cleans up resources. Safe to call multiple times.
// It signals all goroutines to stop and waits for reader goroutines to finish;
// each reader goroutine closes its own pcap handle on exit.
func (p *LibpcapSource) Close() {
	p.closeOnce.Do(func() {
		close(p.exit)
	})
	// Readers check p.exit and return after at most one pcap timeout (1s),
	// at which point they each close their own handle via deferred cleanup.
	p.readerWg.Wait()
}

// refreshLocalAddrs is the periodic loop that discovers new interfaces and
// refreshes local address caches. Interface removal is handled automatically
// by each reader goroutine when its underlying handle errors out, so this
// loop only needs to look for additions.
func (p *LibpcapSource) refreshLocalAddrs() {
	ticker := time.NewTicker(localAddrRefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.syncInterfaces()
		case <-p.exit:
			return
		}
	}
}

// syncInterfaces adds captures for any newly-appeared eligible interfaces and
// refreshes local addresses for existing ones. Removal of gone interfaces is
// handled by reader goroutines self-terminating on BPF errors.
func (p *LibpcapSource) syncInterfaces() {
	systemIfaces, err := net.Interfaces()
	if err != nil {
		log.Warnf("failed to list network interfaces during refresh: %v", err)
		return
	}

	// Add captures for eligible interfaces not yet in the map
	p.interfacesMu.RLock()
	var toAdd []string
	for _, iface := range systemIfaces {
		if !isEligibleInterface(iface) {
			continue
		}
		if _, ok := p.interfaces[iface.Name]; !ok {
			toAdd = append(toAdd, iface.Name)
		}
	}
	p.interfacesMu.RUnlock()

	for _, name := range toAdd {
		log.Infof("new interface %s detected, starting capture", name)
		if err := p.addInterface(name); err != nil {
			log.Warnf("failed to add capture on interface %s: %v", name, err)
		}
	}

	// Refresh local addresses for all current interfaces.
	// net.InterfaceByName does not touch the pcap handle, so this is safe
	// even if a reader goroutine is concurrently exiting.
	p.interfacesMu.RLock()
	current := make([]*interfaceHandle, 0, len(p.interfaces))
	for _, ih := range p.interfaces {
		current = append(current, ih)
	}
	p.interfacesMu.RUnlock()

	for _, ih := range current {
		if err := ih.refreshLocalAddrs(); err != nil {
			log.Debugf("failed to refresh local addresses for %s: %v", ih.ifaceName, err)
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

	ih.localAddrs = make(map[string]struct{})

	for _, addr := range addrs {
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if ip != nil {
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

// isLocalAddr checks if an IP (as raw bytes) is a local address for this interface
func (ih *interfaceHandle) isLocalAddr(ip []byte) bool {
	ih.localAddrsMu.RLock()
	defer ih.localAddrsMu.RUnlock()

	_, exists := ih.localAddrs[string(ip)]
	return exists
}

// determinePacketDirection examines the packet's IP addresses to determine direction.
// Returns PacketOutgoing if the source is a local address, PacketHost otherwise.
func (ih *interfaceHandle) determinePacketDirection(data []byte) uint8 {
	// Need at least Ethernet header (14 bytes)
	if len(data) < 14 {
		return PacketHost
	}

	// EtherType is at offset 12-13
	etherType := uint16(data[12])<<8 | uint16(data[13])

	switch etherType {
	case 0x0800: // IPv4
		if len(data) < 34 { // Ethernet (14) + IPv4 header minimum (20)
			return PacketHost
		}
		// Source IP at offset 26-29 (14 + 12)
		// Destination IP at offset 30-33 (14 + 16)
		if ih.isLocalAddr(data[26:30]) && !ih.isLocalAddr(data[30:34]) {
			return PacketOutgoing
		}
		return PacketHost

	case 0x86DD: // IPv6
		if len(data) < 54 { // Ethernet (14) + IPv6 header minimum (40)
			return PacketHost
		}
		// Source IP at offset 22-37 (14 + 8)
		// Destination IP at offset 38-53 (14 + 24)
		if ih.isLocalAddr(data[22:38]) && !ih.isLocalAddr(data[38:54]) {
			return PacketOutgoing
		}
		return PacketHost

	default:
		return PacketHost
	}
}

var _ PacketSource = &LibpcapSource{}
