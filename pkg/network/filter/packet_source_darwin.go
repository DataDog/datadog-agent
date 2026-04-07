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
	"maps"
	"net"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
)

const (
	telemetryModuleName = "network_tracer__filter"
	defaultSnapLen      = 4096
	pcapTimeout         = time.Second
	pcapBPFBufferSize   = 16 * 1024 * 1024 // 16 MB per-interface BPF ring buffer

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

// packetWithInfo wraps copied packet data with metadata
type packetWithInfo struct {
	data      []byte // Copied data from pool, caller must return via putBuffer
	timestamp time.Time
	direction uint8 // PACKET_HOST or PACKET_OUTGOING
	layerType gopacket.LayerType
}

// directionDecoder is a placeholder for gopacket-based direction decoding.
// decodeAndGetIPs uses gopacket.NewPacket to parse link and network layers.
type directionDecoder struct{}

// interfaceHandle holds a pcap handle and its associated local addresses
type interfaceHandle struct {
	handle    *pcap.Handle
	ifaceName string
	// linkType is the raw pcap DLT for this interface, used by
	// determinePacketDirection to select the correct header parser.
	linkType layers.LinkType
	// goPacketLayerType is the gopacket.LayerType derived from linkType,
	// computed once at open time and stamped on every outgoing packet so
	// the decoder can select the right parser without a per-packet conversion.
	goPacketLayerType gopacket.LayerType

	// dirDecoder decodes link-layer frames to extract IPs for direction classification.
	dirDecoder *directionDecoder

	// localAddrs contains the local IP addresses for this interface
	localAddrs   map[netip.Addr]struct{}
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
	// bgWg tracks background (non-reader) goroutines such as refreshLocalAddrs.
	// bgWg.Wait() must complete before readerWg.Wait() in Close() to guarantee
	// that syncInterfaces cannot call addInterface (which calls readerWg.Add)
	// after readerWg.Wait() has begun.
	bgWg sync.WaitGroup

	// bufPool reuses packet copy buffers of exactly snapLen bytes to reduce
	// GC pressure. The pool is initialized in NewLibpcapSource with the
	// configured snapLen so that buf[:len(data)] is always safe (libpcap
	// truncates captured data to snapLen).
	bufPool *ddsync.TypedPool[[]byte]
}

// DarwinPacketInfo holds information about a packet on Darwin
type DarwinPacketInfo struct {
	// PktType indicates packet direction:
	// PACKET_HOST (0) for incoming, PACKET_OUTGOING (4) for outgoing
	PktType uint8
	// layerType is the gopacket layer type for this packet's link-layer
	// encapsulation. Callers must use this to select the correct decoder —
	// different interfaces on macOS may use different encapsulations
	// (e.g. LayerTypeEthernet for en0, LayerTypeLoopback for utun0).
	layerType gopacket.LayerType
}

// PacketType returns the packet direction type
func (d *DarwinPacketInfo) PacketType() uint8 {
	return d.PktType
}

// LinkLayerType returns the gopacket layer type for this packet's
// link-layer encapsulation. Falls back to LayerTypeEthernet if unset.
func (d *DarwinPacketInfo) LinkLayerType() gopacket.LayerType {
	if d.layerType != 0 {
		return d.layerType
	}
	return layers.LayerTypeEthernet
}

// Option configures a LibpcapSource.
type Option func(*libpcapConfig)

type libpcapConfig struct {
	snapLen int
}

// OptSnapLen specifies the maximum length of the packet to read.
// Defaults to 4096 bytes.
func OptSnapLen(n int) Option {
	return func(c *libpcapConfig) {
		c.snapLen = n
	}
}

// isEligibleInterface reports whether an interface should be captured.
// Skips loopback, virtual/tunnel interfaces that never carry TCP/UDP connections,
// Apple-internal interfaces, and virtualization/hardware interconnect interfaces.
func isEligibleInterface(iface net.Interface) bool {
	if iface.Flags&net.FlagLoopback != 0 {
		return false
	}
	name := iface.Name
	for _, prefix := range []string{
		"bridge", // virtual bridge interfaces
		"vlan",   // virtual LAN interfaces
		"awdl",   // Apple Wireless Direct Link (AirDrop) — not TCP/UDP
		"ap",     // Apple private interfaces (AirDrop/AWDL related)
		"p2p",    // peer-to-peer WiFi — not TCP/UDP
		"llw",    // low-latency WLAN (Sidecar/Handoff) — not TCP/UDP
		"gif",    // IPv6-in-IPv4 generic tunnel — rarely used
		"stf",    // 6to4 IPv6 transition tunnel — rarely used
		"anpi",   // Apple Network Processing Interconnect (Thunderbolt internal)
		"vmenet", // virtualization network interfaces
	} {
		if strings.HasPrefix(name, prefix) {
			return false
		}
	}
	return true
}

// NewLibpcapSource creates a LibpcapSource using libpcap
func NewLibpcapSource(opts ...Option) (*LibpcapSource, error) {
	cfg := libpcapConfig{snapLen: defaultSnapLen}
	for _, opt := range opts {
		opt(&cfg)
	}
	snapLen := cfg.snapLen
	if snapLen <= 0 || snapLen > 65536 {
		return nil, errors.New("snap len should be between 0 and 65536")
	}

	ps := &LibpcapSource{
		interfaces: make(map[string]*interfaceHandle),
		snapLen:    snapLen,
		exit:       make(chan struct{}),
		packetChan: make(chan packetWithInfo, packetChannelSize),
	}
	ps.bufPool = ddsync.NewSlicePool[byte](snapLen, snapLen)

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

	ps.bgWg.Add(1)
	go func() {
		defer ps.bgWg.Done()
		ps.refreshLocalAddrs()
	}()

	log.Infof("created libpcap source on %d interfaces, snaplen=%d", len(ps.interfaces), snapLen)
	log.Debugf("capturing on interfaces: %v", slices.Collect(maps.Keys(ps.interfaces)))
	return ps, nil
}

// addInterface opens a pcap handle on ifaceName, registers it in p.interfaces,
// and starts a reader goroutine that owns the handle for its lifetime.
// When the reader exits for any reason it removes itself from the map and
// closes the handle, so the caller never needs to do that explicitly.
func (p *LibpcapSource) addInterface(ifaceName string) error {
	// Don't open new handles after shutdown has been signalled.
	select {
	case <-p.exit:
		return nil
	default:
	}

	// Add this to insure we wait for this initialization to complete before exiting
	p.readerWg.Add(1)
	inactive, err := pcap.NewInactiveHandle(ifaceName)
	if err != nil {
		p.readerWg.Done()
		return fmt.Errorf("error creating pcap handle on %s: %w", ifaceName, err)
	}
	if err := inactive.SetSnapLen(p.snapLen); err != nil {
		inactive.CleanUp()
		p.readerWg.Done()
		return fmt.Errorf("error setting snap len on %s: %w", ifaceName, err)
	}
	if err := inactive.SetPromisc(false); err != nil {
		inactive.CleanUp()
		p.readerWg.Done()
		return fmt.Errorf("error setting promisc on %s: %w", ifaceName, err)
	}
	if err := inactive.SetTimeout(pcapTimeout); err != nil {
		inactive.CleanUp()
		p.readerWg.Done()
		return fmt.Errorf("error setting timeout on %s: %w", ifaceName, err)
	}
	if err := inactive.SetBufferSize(pcapBPFBufferSize); err != nil {
		inactive.CleanUp()
		p.readerWg.Done()
		return fmt.Errorf("error setting buffer size on %s: %w", ifaceName, err)
	}
	handle, err := inactive.Activate()
	if err != nil {
		inactive.CleanUp()
		p.readerWg.Done()
		return fmt.Errorf("error activating pcap handle on %s: %w", ifaceName, err)
	}

	if err := handle.SetBPFFilter("tcp or udp"); err != nil {
		handle.Close()
		p.readerWg.Done()
		return fmt.Errorf("error setting BPF filter on %s: %w", ifaceName, err)
	}

	lt := handle.LinkType()
	ih := &interfaceHandle{
		handle:            handle,
		ifaceName:         ifaceName,
		linkType:          lt,
		goPacketLayerType: linkTypeToLayerType(lt),
		dirDecoder:        newDirectionDecoder(),
		localAddrs:        make(map[netip.Addr]struct{}),
	}

	if err := ih.refreshLocalAddrs(); err != nil {
		log.Warnf("failed to get local addresses for %s: %v", ifaceName, err)
	}

	p.interfacesMu.Lock()
	p.interfaces[ifaceName] = ih
	p.interfacesMu.Unlock()

	go func() {
		defer p.readerWg.Done()
		// Self-cleanup: remove from the map and close the handle regardless of
		// why this goroutine exits (global shutdown, interface removal, or error).
		defer func() {
			// close the handle before removing the interface from the map
			ih.handle.Close()

			p.interfacesMu.Lock()
			delete(p.interfaces, ih.ifaceName)
			p.interfacesMu.Unlock()

			// Distinguish a clean shutdown from an unexpected interface removal
			// so the logs are actionable.
			select {
			case <-p.exit:
				log.Debugf("stopped capture on interface %s (shutdown)", ih.ifaceName)
			default:
				log.Infof("interface %s removed or errored, stopping capture", ih.ifaceName)
			}
		}()

		p.readPacketsFromInterface(ih)
	}()

	log.Infof("opened pcap handle on interface %s", ifaceName)
	return nil
}

// getBuffer retrieves a snapLen-capacity buffer from the pool.
// The returned slice has len == cap == snapLen.
func (p *LibpcapSource) getBuffer() []byte {
	return *p.bufPool.Get()
}

// putBuffer returns a buffer to the pool. The slice is reset to its
// full capacity before pooling so that the next caller receives a
// slice with len == cap == snapLen.
func (p *LibpcapSource) putBuffer(buf []byte) {
	buf = buf[:cap(buf)]
	p.bufPool.Put(&buf)
}

// VisitPackets reads packets from the persistent channel and invokes the visitor
// callback for each. The data slice and PacketInfo pointer passed to the visitor
// are only valid for the duration of the call and must not be retained.
func (p *LibpcapSource) VisitPackets(visitor func(data []byte, info PacketInfo, timestamp time.Time) error) error {
	packetInfo := &DarwinPacketInfo{}

	for {
		select {
		case pkt := <-p.packetChan:
			packetInfo.PktType = pkt.direction
			packetInfo.layerType = pkt.layerType

			// Wrap in a closure so putBuffer runs via defer even if visitor
			// panics, preventing a permanent pool leak.
			var visitorErr error
			func() {
				defer p.putBuffer(pkt.data)
				visitorErr = visitor(pkt.data, packetInfo, pkt.timestamp)
			}()

			if visitorErr != nil {
				// tell all other readers to stop
				p.closeOnce.Do(func() {
					close(p.exit)
				})
				return visitorErr
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

		// Drop packets that belong to neither side of our connections.
		// In promiscuous mode we capture all frames on the wire, including
		// traffic between other hosts, broadcasts, and multicast — none of
		// which should influence our connection stats.
		if direction == PacketOtherHost {
			continue
		}

		// Copy data to a pooled buffer immediately (before the zero-copy
		// buffer is reused by the next ZeroCopyReadPacketData call).
		buf := p.getBuffer()
		buf = buf[:len(data)]
		copy(buf, data)

		select {
		case p.packetChan <- packetWithInfo{
			data:      buf,
			timestamp: ci.Timestamp,
			direction: direction,
			layerType: ih.goPacketLayerType,
		}:
		case <-p.exit:
			p.putBuffer(buf)
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

// LayerType returns a default layer type for this source. On Darwin, packets
// may come from both Ethernet interfaces (LayerTypeEthernet) and BSD-loopback
// interfaces such as utun* (LayerTypeLoopback). Callers that need the accurate
// per-packet type should use DarwinPacketInfo.LinkLayerType() instead of relying on
// this method.
func (p *LibpcapSource) LayerType() gopacket.LayerType {
	return layers.LayerTypeEthernet
}

// Close stops packet capture and cleans up resources. Safe to call multiple times.
// It signals all goroutines to stop, then waits in two stages:
//  1. bgWg: wait for refreshLocalAddrs to stop so that no further addInterface
//     calls (which call readerWg.Add) can race with the readerWg.Wait below.
//  2. readerWg: wait for all reader goroutines to finish and close their handles.
func (p *LibpcapSource) Close() {
	p.closeOnce.Do(func() {
		close(p.exit)
	})
	// Stage 1: stop background goroutines first to prevent new readerWg.Add calls.
	p.bgWg.Wait()
	// Stage 2: drain all reader goroutines (each closes its own pcap handle).
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

	ih.localAddrs = make(map[netip.Addr]struct{})

	for _, addr := range addrs {
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if ip != nil {
			if a, ok := netip.AddrFromSlice(ip); ok {
				ih.localAddrs[a.Unmap()] = struct{}{}
			}
		}
	}

	log.Debugf("refreshed local addresses for %s: %d addresses found", ih.ifaceName, len(ih.localAddrs))
	return nil
}

// isLocalAddr checks if an IP is a local address for this interface
func (ih *interfaceHandle) isLocalAddr(addr netip.Addr) bool {
	ih.localAddrsMu.RLock()
	defer ih.localAddrsMu.RUnlock()

	_, exists := ih.localAddrs[addr]
	return exists
}

// newDirectionDecoder creates a directionDecoder for gopacket-based parsing.
func newDirectionDecoder() *directionDecoder {
	return &directionDecoder{}
}

// decodeAndGetIPs decodes the packet with gopacket and returns the source and
// destination IP bytes from the network layer (IPv4 or IPv6). ok is false if
// the packet is too short, decoding fails, or no network layer is present.
// For Ethernet we require enough bytes for the link header plus a full IP
// header (IPv4 20 bytes or IPv6 40 bytes) based on EtherType; for loopback
// we require 4-byte header plus 20 bytes (IPv4 minimum).
func (d *directionDecoder) decodeAndGetIPs(data []byte, firstLayer gopacket.LayerType) (srcIP, dstIP netip.Addr, ok bool) {
	var minLen int
	if firstLayer == layers.LayerTypeEthernet {
		if len(data) < 14 {
			return netip.Addr{}, netip.Addr{}, false
		}
		etherType := uint16(data[12])<<8 | uint16(data[13])
		if etherType == 0x86DD {
			minLen = 14 + 40 // IPv6 header
		} else {
			minLen = 14 + 20 // IPv4 minimum
		}
	} else {
		minLen = 4 + 20 // loopback header + IPv4 minimum
	}
	if len(data) < minLen {
		return netip.Addr{}, netip.Addr{}, false
	}
	pkt := gopacket.NewPacket(data, firstLayer, gopacket.NoCopy)
	netLayer := pkt.NetworkLayer()
	if netLayer == nil {
		return netip.Addr{}, netip.Addr{}, false
	}
	flow := netLayer.NetworkFlow()
	src, _ := netip.AddrFromSlice(flow.Src().Raw())
	dst, _ := netip.AddrFromSlice(flow.Dst().Raw())
	return src.Unmap(), dst.Unmap(), true
}

// linkTypeToLayerType converts a pcap DLT link type to the gopacket.LayerType
// needed by the packet decoder. We map explicitly rather than calling
// LinkType.LayerType() because that method returns LayerTypeZero (0) for
// LinkTypeNull when the gopacket metadata table has no entry for DLT 0,
// which causes the downstream parser selector to fall back to Ethernet and
// misparse BSD-loopback packets from utun/VPN interfaces.
func linkTypeToLayerType(lt layers.LinkType) gopacket.LayerType {
	switch lt {
	case layers.LinkTypeNull, layers.LinkTypeLoop:
		return layers.LayerTypeLoopback
	default:
		return layers.LayerTypeEthernet
	}
}

// determinePacketDirection examines the packet's IP addresses to determine direction.
// Returns:
//   - PacketOutgoing  — source is local, destination is not local
//   - PacketHost      — destination is local (incoming)
//   - PacketOtherHost — neither source nor destination is a local address
//     (e.g. promiscuous-mode traffic between other hosts, broadcast, multicast)
func (ih *interfaceHandle) determinePacketDirection(data []byte) uint8 {
	if ih.dirDecoder == nil {
		return PacketHost
	}
	srcIP, dstIP, ok := ih.dirDecoder.decodeAndGetIPs(data, ih.goPacketLayerType)
	if !ok {
		return PacketHost
	}
	return classifyDirection(ih, srcIP, dstIP)
}

// classifyDirection returns the packet direction given source and
// destination addresses checked against the interface's local address set.
func classifyDirection(ih *interfaceHandle, srcIP, dstIP netip.Addr) uint8 {
	srcIsLocal := ih.isLocalAddr(srcIP)
	dstIsLocal := ih.isLocalAddr(dstIP)
	if srcIsLocal && !dstIsLocal {
		return PacketOutgoing
	}
	if dstIsLocal {
		return PacketHost
	}
	return PacketOtherHost
}

var _ PacketSource = &LibpcapSource{}
