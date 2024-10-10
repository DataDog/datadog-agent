// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package tracer

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/ebpf/maps"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/netlink"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// from github.com/cilium/cilium/pkg/maps/ctmap/lookup.go
	TUPLE_F_OUT     = 0
	TUPLE_F_IN      = 1
	TUPLE_F_RELATED = 2
	TUPLE_F_SERVICE = 4
)

type TupleKey4 struct {
	DestAddr   [4]byte
	SourceAddr [4]byte
	SourcePort uint16
	DestPort   uint16
	NextHeader uint8
	Flags      uint8
}

type CtEntry struct {
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

type Backend4KeyV3 struct {
	ID uint32
}

type Backend4ValueV3 struct {
	Address   [4]byte
	Port      uint16
	Proto     uint8
	Flags     uint8
	ClusterID uint16
	Zone      uint8
	Pad       uint8
}

type backend struct {
	addr util.Address
	port uint16
}

type ciliumLoadBalancerConntracker struct {
	m                  sync.Mutex
	backends           *maps.GenericMap[Backend4KeyV3, Backend4ValueV3]
	ctTcp, ctUdp       *maps.GenericMap[TupleKey4, CtEntry]
	backendIDToBackend map[uint32]backend
	stop               chan struct{}
}

func newCiliumLoadBalancerConntracker(cfg *config.Config) (netlink.Conntracker, error) {
	if !cfg.EnableCiliumLBConntracker {
		return netlink.NewNoOpConntracker(), nil
	}

	ctTcp, err := ebpf.LoadPinnedMap("/sys/fs/bpf/tc/globals/cilium_ct4_global", &ebpf.LoadPinOptions{
		ReadOnly: true,
	})
	if err != nil {
		return nil, fmt.Errorf("error loading pinned ct TCP map: %w", err)
	}

	ctUdp, err := ebpf.LoadPinnedMap("/sys/fs/bpf/tc/globals/cilium_ct_any4_global", &ebpf.LoadPinOptions{
		ReadOnly: true,
	})
	if err != nil {
		return nil, fmt.Errorf("error loading pinned ct UDP map: %w", err)
	}

	backends, err := ebpf.LoadPinnedMap("/sys/fs/bpf/tc/globals/cilium_lb4_backends_v3", &ebpf.LoadPinOptions{
		ReadOnly: true,
	})
	if err != nil {
		return nil, fmt.Errorf("error loading pinned backends map: %w", err)
	}

	clb := &ciliumLoadBalancerConntracker{
		backendIDToBackend: make(map[uint32]backend),
	}
	if clb.ctTcp, err = maps.Map[TupleKey4, CtEntry](ctTcp); err != nil {
		return nil, fmt.Errorf("could not make generic map for ct TCP map: %w", err)
	}
	if clb.ctUdp, err = maps.Map[TupleKey4, CtEntry](ctUdp); err != nil {
		return nil, fmt.Errorf("could not make generic map for ct UDP map: %w", err)
	}
	if clb.backends, err = maps.Map[Backend4KeyV3, Backend4ValueV3](backends); err != nil {
		return nil, fmt.Errorf("could not make generic map for backends map: %w", err)
	}

	clb.stop = make(chan struct{})
	go func() {
		tick := time.NewTicker(10 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-tick.C:
				clb.updateBackends()
			case <-clb.stop:
				close(clb.stop)
				return
			}
		}
	}()

	return clb, nil
}

func ntohs(n uint16) uint16 {
	return binary.BigEndian.Uint16([]byte{byte(n), byte(n >> 8)})
}

func htons(n uint16) uint16 {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, n)
	return *(*uint16)(unsafe.Pointer(&b[0]))
}

func (clb *ciliumLoadBalancerConntracker) updateBackends() {
	clb.m.Lock()
	defer clb.m.Unlock()

	it := clb.backends.Iterate()
	var k Backend4KeyV3
	var v Backend4ValueV3
	for it.Next(&k, &v) {
		clb.backendIDToBackend[k.ID] = backend{
			addr: util.AddressFromNetIP(net.IPv4(v.Address[0], v.Address[1], v.Address[2], v.Address[3])),
			port: ntohs(v.Port),
		}
	}

	if it.Err() != nil {
		log.Warnf("error iterating lb backends map: %s", it.Err())
	}
}

func (clb *ciliumLoadBalancerConntracker) Describe(descs chan<- *prometheus.Desc) {
}

// Collect returns the current state of all metrics of the collector
func (clb *ciliumLoadBalancerConntracker) Collect(metrics chan<- prometheus.Metric) {
}

func (clb *ciliumLoadBalancerConntracker) GetTranslationForConn(c *network.ConnectionStats) *network.IPTranslation {
	// TODO: add ipv6 support
	if c.Family != network.AFINET {
		return nil
	}

	if c.Direction != network.OUTGOING {
		return nil
	}

	if c.Dest.IsLoopback() {
		return nil
	}

	queryMap := clb.ctTcp
	t := TupleKey4{
		Flags:      TUPLE_F_OUT | TUPLE_F_SERVICE,
		NextHeader: uint8(unix.IPPROTO_TCP),
		SourcePort: htons(c.SPort),
		DestPort:   htons(c.DPort),
		SourceAddr: c.Source.As4(),
		DestAddr:   c.Dest.As4(),
	}
	if c.Type == network.UDP {
		t.NextHeader = unix.IPPROTO_UDP
		queryMap = clb.ctUdp
	}

	log.TraceFunc(func() string {
		return fmt.Sprintf("looking up tuple %+v in ct map", t)
	})

	var ctEntry CtEntry
	var err error
	if err = queryMap.Lookup(&t, &ctEntry); err != nil {
		if !errors.Is(err, ebpf.ErrKeyNotExist) {
			log.Warnf("error looking up %+v in ct map: %s", t, err)
		}

		log.TraceFunc(func() string {
			return fmt.Sprintf("lookup failed for %+v in ct map", t)
		})

		return nil
	}

	log.TraceFunc(func() string {
		return fmt.Sprintf("found ct entry for %+v: %+v", t, ctEntry)
	})

	clb.m.Lock()
	defer clb.m.Unlock()

	if b, ok := clb.backendIDToBackend[uint32(ctEntry.BackendID)]; ok && (b.addr != c.Dest || b.port != c.DPort) {
		return &network.IPTranslation{
			ReplDstIP:   c.Source,
			ReplDstPort: c.SPort,
			ReplSrcIP:   b.addr,
			ReplSrcPort: b.port,
		}
	}

	return nil
}

// GetType returns a string describing whether the conntracker is "ebpf" or "netlink"
func (clb *ciliumLoadBalancerConntracker) GetType() string {
	return "cilium_lb"
}

func (clb *ciliumLoadBalancerConntracker) DeleteTranslation(*network.ConnectionStats) {
}

func (clb *ciliumLoadBalancerConntracker) DumpCachedTable(context.Context) (map[uint32][]netlink.DebugConntrackEntry, error) {
	return nil, nil
}

func (clb *ciliumLoadBalancerConntracker) Close() {
	clb.stop <- struct{}{}
	<-clb.stop
	clb.ctTcp.Map().Close()
	clb.backends.Map().Close()
}
