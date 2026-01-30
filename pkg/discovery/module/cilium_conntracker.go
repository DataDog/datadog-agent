// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"encoding/binary"
	"fmt"
	"os"
	"unsafe"

	"github.com/cilium/ebpf"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/discovery/model"
	"github.com/DataDog/datadog-agent/pkg/ebpf/maps"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	//revive:disable
	// from github.com/cilium/cilium/pkg/maps/ctmap/lookup.go
	TUPLE_F_OUT     = 0
	TUPLE_F_IN      = 1
	TUPLE_F_RELATED = 2
	TUPLE_F_SERVICE = 4
	//revive:enable
)

// tupleKey4 represents a 4-tuple key for IPv4 connections in Cilium's connection tracking map.
// This matches the structure from pkg/network/tracer/cilium_lb.go.
type tupleKey4 struct {
	DestAddr   [4]byte
	SourceAddr [4]byte
	SourcePort uint16
	DestPort   uint16
	NextHeader uint8
	Flags      uint8
}

// ctEntry represents a connection tracking entry in Cilium's ct map.
// This matches the structure from pkg/network/tracer/cilium_lb.go.
type ctEntry struct {
	Reserved0 uint64
	BackendID uint64
	Packets   uint64
	Bytes     uint64
	Lifetime  uint32
	Flags     uint16
	// RevNAT is in network byte order
	RevNAT           uint16
	IfIndex          uint16
	TxFlagsSeen      uint8
	RxFlagsSeen      uint8
	SourceSecurityID uint32
	LastTxReport     uint32
	LastRxReport     uint32
}

// backend4KeyV3 represents a key for looking up backend information.
type backend4KeyV3 struct {
	ID uint32
}

// backend4ValueV3 represents backend address and port information.
type backend4ValueV3 struct {
	Address   [4]byte
	Port      uint16
	Proto     uint8
	Flags     uint8
	ClusterID uint16
	Zone      uint8
	Pad       uint8
}

// backend stores parsed backend address and port.
type backend struct {
	ip   string
	port uint16
}

// ciliumConntracker reads Cilium eBPF maps to discover NAT-translated connections.
type ciliumConntracker struct {
	ctTCP    *maps.GenericMap[tupleKey4, ctEntry]
	ctUDP    *maps.GenericMap[tupleKey4, ctEntry]
	backends *maps.GenericMap[backend4KeyV3, backend4ValueV3]
}

// newCiliumConntracker creates a new Cilium conntracker.
// Returns nil if Cilium maps are not available (graceful degradation).
func newCiliumConntracker() (*ciliumConntracker, error) {
	ctTCP, ctUDP, backends, err := loadCiliumMaps()
	if err != nil {
		// If maps don't exist, log info and return nil (not an error)
		if os.IsNotExist(err) {
			log.Info("Cilium maps not found, using /proc only for connection discovery")
			return nil, nil
		}
		// Permission denied is also common in restricted environments
		if os.IsPermission(err) {
			log.Info("Cannot access Cilium maps (permission denied), using /proc only for connection discovery")
			return nil, nil
		}
		// Other errors - log debug and degrade gracefully
		log.Debugf("Failed to load Cilium maps: %v, using /proc only for connection discovery", err)
		return nil, nil
	}

	cc := &ciliumConntracker{}

	// Wrap the raw eBPF maps with GenericMap for type-safe access
	if cc.ctTCP, err = maps.Map[tupleKey4, ctEntry](ctTCP); err != nil {
		log.Debugf("Could not make generic map for Cilium TCP ct map: %v", err)
		return nil, nil
	}
	if cc.ctUDP, err = maps.Map[tupleKey4, ctEntry](ctUDP); err != nil {
		log.Debugf("Could not make generic map for Cilium UDP ct map: %v", err)
		return nil, nil
	}
	if cc.backends, err = maps.Map[backend4KeyV3, backend4ValueV3](backends); err != nil {
		log.Debugf("Could not make generic map for Cilium backends map: %v", err)
		return nil, nil
	}

	log.Info("Cilium conntracker initialized successfully")
	return cc, nil
}

// loadCiliumMaps loads the Cilium eBPF maps from /sys/fs/bpf.
func loadCiliumMaps() (ctTCP, ctUDP, backends *ebpf.Map, err error) {
	defer func() {
		if err != nil {
			if ctTCP != nil {
				ctTCP.Close()
			}
			if ctUDP != nil {
				ctUDP.Close()
			}
			if backends != nil {
				backends.Close()
			}
		}
	}()

	ctTCP, err = loadCiliumMap("/sys/fs/bpf/tc/globals/cilium_ct4_global")
	if err != nil {
		return nil, nil, nil, err
	}

	ctUDP, err = loadCiliumMap("/sys/fs/bpf/tc/globals/cilium_ct_any4_global")
	if err != nil {
		return nil, nil, nil, err
	}

	backends, err = loadCiliumMap("/sys/fs/bpf/tc/globals/cilium_lb4_backends_v3")
	if err != nil {
		return nil, nil, nil, err
	}

	return ctTCP, ctUDP, backends, nil
}

// loadCiliumMap loads a single Cilium eBPF map from the given path.
func loadCiliumMap(path string) (*ebpf.Map, error) {
	// Check if the path exists first, since errors from LoadPinnedMap
	// are not consistent if it doesn't exist
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}

	m, err := ebpf.LoadPinnedMap(path, &ebpf.LoadPinOptions{
		ReadOnly: true,
	})
	if err != nil {
		return nil, fmt.Errorf("error loading pinned Cilium map at %s: %w", path, err)
	}

	return m, nil
}

// snapshotBackends reads all backends from the eBPF map into a snapshot map.
func (cc *ciliumConntracker) snapshotBackends() map[uint64]*backend {
	backendCache := make(map[uint64]*backend)

	it := cc.backends.Iterate()
	var k backend4KeyV3
	var v backend4ValueV3
	for it.Next(&k, &v) {
		backendCache[uint64(k.ID)] = &backend{
			ip:   fmt.Sprintf("%d.%d.%d.%d", v.Address[0], v.Address[1], v.Address[2], v.Address[3]),
			port: ntohs(v.Port),
		}
	}

	if err := it.Err(); err != nil {
		log.Warnf("Error iterating Cilium backends map: %v", err)
	}

	return backendCache
}

// getConnections retrieves all connections from Cilium's connection tracking maps.
func (cc *ciliumConntracker) getConnections() ([]model.Connection, error) {
	// Snapshot backends once at the start
	backendCache := cc.snapshotBackends()

	var connections []model.Connection

	// Get TCP connections
	tcpConns, err := cc.getConnectionsFromMap(cc.ctTCP, unix.IPPROTO_TCP, backendCache)
	if err != nil {
		log.Warnf("Failed to get TCP connections from Cilium: %v", err)
	} else {
		connections = append(connections, tcpConns...)
	}

	// Get UDP connections
	udpConns, err := cc.getConnectionsFromMap(cc.ctUDP, unix.IPPROTO_UDP, backendCache)
	if err != nil {
		log.Warnf("Failed to get UDP connections from Cilium: %v", err)
	} else {
		connections = append(connections, udpConns...)
	}

	return connections, nil
}

// getConnectionsFromMap extracts connections from a specific Cilium ct map.
func (cc *ciliumConntracker) getConnectionsFromMap(ctMap *maps.GenericMap[tupleKey4, ctEntry], proto uint8, backendCache map[uint64]*backend) ([]model.Connection, error) {
	var connections []model.Connection

	it := ctMap.Iterate()
	var key tupleKey4
	var value ctEntry

	for it.Next(&key, &value) {
		// Only process outgoing service connections
		if key.Flags != (TUPLE_F_OUT | TUPLE_F_SERVICE) {
			continue
		}

		// Only process matching protocol
		if key.NextHeader != proto {
			continue
		}

		// Build the connection model
		conn := model.Connection{
			Laddr: model.Address{
				IP:   fmt.Sprintf("%d.%d.%d.%d", key.SourceAddr[0], key.SourceAddr[1], key.SourceAddr[2], key.SourceAddr[3]),
				Port: ntohs(key.SourcePort),
			},
			Raddr: model.Address{
				IP:   fmt.Sprintf("%d.%d.%d.%d", key.DestAddr[0], key.DestAddr[1], key.DestAddr[2], key.DestAddr[3]),
				Port: ntohs(key.DestPort),
			},
		}

		// Look up backend if available
		backend, hasBackend := backendCache[value.BackendID]

		if hasBackend && backend != nil {
			// Check if backend is different from destination (indicates NAT)
			if backend.ip != conn.Raddr.IP || backend.port != conn.Raddr.Port {
				conn.TranslatedRaddr = &model.Address{
					IP:   backend.ip,
					Port: backend.port,
				}
				log.Debugf("Cilium NAT detected: %s:%d -> %s:%d (translated to %s:%d)",
					conn.Laddr.IP, conn.Laddr.Port,
					conn.Raddr.IP, conn.Raddr.Port,
					backend.ip, backend.port)
			}
		}

		connections = append(connections, conn)
	}

	if err := it.Err(); err != nil {
		return nil, fmt.Errorf("error iterating Cilium ct map: %w", err)
	}

	return connections, nil
}

// Close closes the Cilium conntracker and releases resources.
func (cc *ciliumConntracker) Close() error {
	if cc.ctTCP != nil {
		cc.ctTCP.Map().Close()
	}
	if cc.ctUDP != nil {
		cc.ctUDP.Map().Close()
	}
	if cc.backends != nil {
		cc.backends.Map().Close()
	}
	return nil
}

// ntohs converts a uint16 from network byte order to host byte order.
func ntohs(n uint16) uint16 {
	return binary.BigEndian.Uint16([]byte{byte(n), byte(n >> 8)})
}

// htons converts a uint16 from host byte order to network byte order.
func htons(n uint16) uint16 {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, n)
	return *(*uint16)(unsafe.Pointer(&b[0]))
}
