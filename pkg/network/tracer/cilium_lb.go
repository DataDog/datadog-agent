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
	"os"
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
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
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

const ciliumConntrackerModuleName = "network_tracer__cilium_conntracker"

var ciliumConntrackerTelemetry = struct {
	getsDuration telemetry.Histogram
	getsTotal    telemetry.Counter
}{
	telemetry.NewHistogram(ciliumConntrackerModuleName, "gets_duration_nanoseconds", []string{}, "Histogram measuring the time spent retrieving connection tuples from the EBPF map", defaultBuckets),
	telemetry.NewCounter(ciliumConntrackerModuleName, "gets_total", []string{}, "Counter measuring the total number of attempts to get connection tuples from the EBPF map"),
}

type tupleKey4 struct {
	DestAddr   [4]byte
	SourceAddr [4]byte
	SourcePort uint16
	DestPort   uint16
	NextHeader uint8
	Flags      uint8
}

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

type backend4KeyV3 struct {
	ID uint32
}

type backend4ValueV3 struct {
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
	backends           *maps.GenericMap[backend4KeyV3, backend4ValueV3]
	ctTCP, ctUDP       *maps.GenericMap[tupleKey4, ctEntry]
	backendIDToBackend map[uint32]backend
	stop               chan struct{}
	closeOnce          sync.Once
}

func newCiliumLoadBalancerConntracker(cfg *config.Config) (netlink.Conntracker, error) {
	if !cfg.EnableCiliumLBConntracker {
		log.Info("cilium conntracker disabled")
		return nil, nil
	}

	ctTCP, ctUDP, backends, err := loadMaps()
	if err != nil {
		// special case where we couldn't find at least one map
		if os.IsNotExist(err) {
			log.Info("not loading cilium conntracker since cilium maps are not present")
			return nil, nil
		}

		return nil, err
	}

	clb := &ciliumLoadBalancerConntracker{
		backendIDToBackend: make(map[uint32]backend),
	}
	if clb.ctTCP, err = maps.Map[tupleKey4, ctEntry](ctTCP); err != nil {
		return nil, fmt.Errorf("could not make generic map for ct TCP map: %w", err)
	}
	if clb.ctUDP, err = maps.Map[tupleKey4, ctEntry](ctUDP); err != nil {
		return nil, fmt.Errorf("could not make generic map for ct UDP map: %w", err)
	}
	if clb.backends, err = maps.Map[backend4KeyV3, backend4ValueV3](backends); err != nil {
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

	log.Info("cilium conntracker initialized")
	return clb, nil
}

func loadMaps() (ctTCP, ctUDP, backends *ebpf.Map, err error) {
	defer func() {
		if err != nil {
			ctTCP.Close()
			ctUDP.Close()
			backends.Close()
		}
	}()

	ctTCP, err = loadMap(kernel.HostSys("/fs/bpf/tc/globals/cilium_ct4_global"))
	if ctTCP == nil {
		return nil, nil, nil, err
	}

	ctUDP, err = loadMap(kernel.HostSys("/fs/bpf/tc/globals/cilium_ct_any4_global"))
	if ctUDP == nil {
		return nil, nil, nil, err
	}

	backends, err = loadMap(kernel.HostSys("/fs/bpf/tc/globals/cilium_lb4_backends_v3"))
	if backends == nil {
		return nil, nil, nil, err
	}

	return ctTCP, ctUDP, backends, nil
}

func loadMap(path string) (m *ebpf.Map, err error) {
	// check if the path exists first, since the errors returned
	// from LoadPinnedMap are not consistent if it doesn't
	if _, err = os.Stat(path); err != nil {
		return nil, err
	}

	m, err = ebpf.LoadPinnedMap(path, &ebpf.LoadPinOptions{
		ReadOnly: true,
	})
	if err != nil {
		return nil, fmt.Errorf("error loading pinned cilium map at %s: %w", path, err)
	}

	return m, nil
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
	var k backend4KeyV3
	var v backend4ValueV3
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

// Describe returns all descriptions of the collector
func (clb *ciliumLoadBalancerConntracker) Describe(chan<- *prometheus.Desc) {
}

// Collect returns the current state of all metrics of the collector
func (clb *ciliumLoadBalancerConntracker) Collect(chan<- prometheus.Metric) {
}

// GetTranslationForConn returns the network address translation for a given connection tuple
func (clb *ciliumLoadBalancerConntracker) GetTranslationForConn(c *network.ConnectionTuple) *network.IPTranslation {
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

	startTime := time.Now()
	defer func() {
		ciliumConntrackerTelemetry.getsTotal.Inc()
		ciliumConntrackerTelemetry.getsDuration.Observe(float64(time.Since(startTime).Nanoseconds()))
	}()

	queryMap := clb.ctTCP
	t := tupleKey4{
		Flags:      TUPLE_F_OUT | TUPLE_F_SERVICE,
		NextHeader: uint8(unix.IPPROTO_TCP),
		SourcePort: htons(c.SPort),
		DestPort:   htons(c.DPort),
		SourceAddr: c.Source.As4(),
		DestAddr:   c.Dest.As4(),
	}
	if c.Type == network.UDP {
		t.NextHeader = unix.IPPROTO_UDP
		queryMap = clb.ctUDP
	}

	log.TraceFunc(func() string {
		return fmt.Sprintf("looking up tuple %+v in ct map", t)
	})

	var ctEntry ctEntry
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

// GetType returns a string describing the conntracker type
func (clb *ciliumLoadBalancerConntracker) GetType() string {
	return "cilium_lb"
}

// DeleteTranslation delete the network address translation for a tuple
func (clb *ciliumLoadBalancerConntracker) DeleteTranslation(*network.ConnectionTuple) {
}

// DumpCachedTable dumps the in-memory address translation table
func (clb *ciliumLoadBalancerConntracker) DumpCachedTable(context.Context) (map[uint32][]netlink.DebugConntrackEntry, error) {
	return nil, nil
}

// Close closes the conntracker
func (clb *ciliumLoadBalancerConntracker) Close() {
	clb.closeOnce.Do(func() {
		clb.stop <- struct{}{}
		<-clb.stop
		clb.ctTCP.Map().Close()
		clb.ctUDP.Map().Close()
		clb.backends.Map().Close()
	})
}
