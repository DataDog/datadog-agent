// +build linux_bpf

package tracer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/netlink"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/ebpf"
	"github.com/DataDog/ebpf/manager"
	ct "github.com/florianl/go-conntrack"
	"golang.org/x/sys/unix"
)

var tuplePool = sync.Pool{
	New: func() interface{} {
		return new(netebpf.ConntrackTuple)
	},
}

type ebpfConntracker struct {
	m            *manager.Manager
	ctMap        *ebpf.Map
	telemetryMap *ebpf.Map
	// only kept around for stats purposes from initial dump
	consumer *netlink.Consumer
	decoder  *netlink.Decoder

	stats struct {
		gets                 int64
		getTotalTime         int64
		unregisters          int64
		unregistersTotalTime int64
	}
}

// NewEBPFConntracker creates a netlink.Conntracker that monitor conntrack NAT entries via eBPF
func NewEBPFConntracker(cfg *config.Config) (netlink.Conntracker, error) {
	buf, err := getRuntimeCompiledConntracker(cfg)
	if err != nil {
		return nil, fmt.Errorf("unable to compile ebpf conntracker: %w", err)
	}

	m, err := getManager(buf, cfg.ConntrackMaxStateSize)
	if err != nil {
		return nil, err
	}

	err = m.Start()
	if err != nil {
		_ = m.Stop(manager.CleanAll)
		return nil, fmt.Errorf("failed to start ebpf conntracker: %w", err)
	}

	ctMap, _, err := m.GetMap(string(probes.ConntrackMap))
	if err != nil {
		_ = m.Stop(manager.CleanAll)
		return nil, fmt.Errorf("unable to get conntrack map: %w", err)
	}

	telemetryMap, _, err := m.GetMap(string(probes.ConntrackTelemetryMap))
	if err != nil {
		_ = m.Stop(manager.CleanAll)
		return nil, fmt.Errorf("unable to get telemetry map: %w", err)
	}

	e := &ebpfConntracker{
		m:            m,
		ctMap:        ctMap,
		telemetryMap: telemetryMap,
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ConntrackInitTimeout)
	defer cancel()

	err = e.dumpInitialTables(ctx, cfg)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("could not initialize conntrack after %s", cfg.ConntrackInitTimeout)
		}
		return nil, err
	}
	log.Infof("initialized ebpf conntrack")
	return e, nil
}

func (e *ebpfConntracker) dumpInitialTables(ctx context.Context, cfg *config.Config) error {
	e.consumer = netlink.NewConsumer(cfg.ProcRoot, cfg.ConntrackRateLimit, true)
	e.decoder = netlink.NewDecoder()
	defer e.consumer.Stop()

	for _, family := range []uint8{unix.AF_INET, unix.AF_INET6} {
		events, err := e.consumer.DumpTable(family)
		if err != nil {
			return err
		}
		if err := e.loadInitialState(ctx, events); err != nil {
			return err
		}
	}
	return nil
}

func (e *ebpfConntracker) loadInitialState(ctx context.Context, events <-chan netlink.Event) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-events:
			if !ok {
				return nil
			}
			e.processEvent(ev)
		}
	}
}

func (e *ebpfConntracker) processEvent(ev netlink.Event) {
	conns := e.decoder.DecodeAndReleaseEvent(ev)
	for _, c := range conns {
		if netlink.IsNAT(c) {
			log.Tracef("initial conntrack %s", c)
			src := formatKey(uint32(c.NetNS), c.Origin)
			dst := formatKey(uint32(c.NetNS), c.Reply)
			if src != nil && dst != nil {
				if err := e.addTranslation(src, dst); err != nil {
					log.Warnf("error adding initial conntrack entry to ebpf map: %s", err)
				}
				if err := e.addTranslation(dst, src); err != nil {
					log.Warnf("error adding initial conntrack entry to ebpf map: %s", err)
				}
			}
		}
	}
}

func (e *ebpfConntracker) addTranslation(src *netebpf.ConntrackTuple, dst *netebpf.ConntrackTuple) error {
	if err := e.ctMap.Update(unsafe.Pointer(src), unsafe.Pointer(dst), ebpf.UpdateNoExist); err != nil && !errors.Is(err, ebpf.ErrKeyExist) {
		return err
	}
	return nil
}

func formatKey(netns uint32, tuple *ct.IPTuple) *netebpf.ConntrackTuple {
	var proto network.ConnectionType
	switch *tuple.Proto.Number {
	case unix.IPPROTO_TCP:
		proto = network.TCP
	case unix.IPPROTO_UDP:
		proto = network.UDP
	default:
		return nil
	}

	nct := &netebpf.ConntrackTuple{
		Netns: netns,
		Sport: *tuple.Proto.SrcPort,
		Dport: *tuple.Proto.DstPort,
	}
	src := util.AddressFromNetIP(*tuple.Src)
	nct.Saddr_l, nct.Saddr_h = util.ToLowHigh(src)
	nct.Daddr_l, nct.Daddr_h = util.ToLowHigh(util.AddressFromNetIP(*tuple.Dst))

	switch len(src.Bytes()) {
	case net.IPv4len:
		nct.Metadata |= uint32(netebpf.IPv4)
	case net.IPv6len:
		nct.Metadata |= uint32(netebpf.IPv6)
	default:
		return nil
	}
	switch proto {
	case network.TCP:
		nct.Metadata |= uint32(netebpf.TCP)
	case network.UDP:
		nct.Metadata |= uint32(netebpf.UDP)
	}

	return nct
}

func toConntrackTupleFromStats(src *netebpf.ConntrackTuple, stats *network.ConnectionStats) {
	src.Netns = stats.NetNS
	src.Sport = stats.SPort
	src.Dport = stats.DPort
	src.Saddr_l, src.Saddr_h = util.ToLowHigh(stats.Source)
	src.Daddr_l, src.Daddr_h = util.ToLowHigh(stats.Dest)
	src.Metadata = 0
	switch stats.Type {
	case network.TCP:
		src.Metadata |= uint32(netebpf.TCP)
	case network.UDP:
		src.Metadata |= uint32(netebpf.UDP)
	}
	switch stats.Family {
	case network.AFINET:
		src.Metadata |= uint32(netebpf.IPv4)
	case network.AFINET6:
		src.Metadata |= uint32(netebpf.IPv6)
	}
}

func (e *ebpfConntracker) GetTranslationForConn(stats network.ConnectionStats) *network.IPTranslation {
	start := time.Now()
	src := tuplePool.Get().(*netebpf.ConntrackTuple)
	defer tuplePool.Put(src)

	toConntrackTupleFromStats(src, &stats)
	log.Tracef("looking up in conntrack (stats): %s", stats)
	log.Tracef("looking up in conntrack (tuple): %s", src)

	dst := e.get(src)
	if dst == nil {
		return nil
	}
	defer tuplePool.Put(dst)

	atomic.AddInt64(&e.stats.gets, 1)
	atomic.AddInt64(&e.stats.getTotalTime, time.Now().Sub(start).Nanoseconds())
	return &network.IPTranslation{
		ReplSrcIP:   dst.SourceAddress(),
		ReplDstIP:   dst.DestAddress(),
		ReplSrcPort: dst.Sport,
		ReplDstPort: dst.Dport,
	}
}

func (e *ebpfConntracker) get(src *netebpf.ConntrackTuple) *netebpf.ConntrackTuple {
	dst := tuplePool.Get().(*netebpf.ConntrackTuple)
	if err := e.ctMap.Lookup(unsafe.Pointer(src), unsafe.Pointer(dst)); err != nil {
		if !errors.Is(err, ebpf.ErrKeyNotExist) {
			log.Warnf("error looking up connection in ebpf conntrack map: %s", err)
		}
		tuplePool.Put(dst)
		return nil
	}
	return dst
}

func (e *ebpfConntracker) delete(key *netebpf.ConntrackTuple) {
	if err := e.ctMap.Delete(unsafe.Pointer(key)); err != nil {
		if errors.Is(err, ebpf.ErrKeyNotExist) {
			log.Tracef("connection does not exist in ebpf conntrack map: %s", key)
			return
		}
		log.Warnf("unable to delete conntrack entry from eBPF map: %s", err)
	}
}

func (e *ebpfConntracker) DeleteTranslation(stats network.ConnectionStats) {
	start := time.Now()
	key := tuplePool.Get().(*netebpf.ConntrackTuple)
	defer tuplePool.Put(key)

	toConntrackTupleFromStats(key, &stats)

	dst := e.get(key)
	e.delete(key)
	if dst != nil {
		e.delete(dst)
		tuplePool.Put(dst)
	}
	atomic.AddInt64(&e.stats.unregisters, 1)
	atomic.AddInt64(&e.stats.unregistersTotalTime, time.Now().Sub(start).Nanoseconds())
}

func (e *ebpfConntracker) GetStats() map[string]int64 {
	m := map[string]int64{
		"state_size": 0,
	}
	telemetry := &netebpf.ConntrackTelemetry{}
	if err := e.telemetryMap.Lookup(unsafe.Pointer(&zero), unsafe.Pointer(telemetry)); err != nil {
		log.Tracef("error retrieving the telemetry struct: %s", err)
	} else {
		m["registers_total"] = int64(telemetry.Registers)
		m["registers_dropped"] = int64(telemetry.Dropped)
	}

	gets := atomic.LoadInt64(&e.stats.gets)
	getTimeTotal := atomic.LoadInt64(&e.stats.getTotalTime)
	m["gets_total"] = gets
	if gets > 0 {
		m["nanoseconds_per_get"] = getTimeTotal / gets
	}

	unregisters := atomic.LoadInt64(&e.stats.unregisters)
	unregistersTimeTotal := atomic.LoadInt64(&e.stats.unregistersTotalTime)
	m["unregisters_total"] = unregisters
	if unregisters > 0 {
		m["nanoseconds_per_unregister"] = unregistersTimeTotal / unregisters
	}

	// Merge telemetry from the consumer
	for k, v := range e.consumer.GetStats() {
		m[k] = v
	}

	return m
}

func (e *ebpfConntracker) Close() {
	err := e.m.Stop(manager.CleanAll)
	if err != nil {
		log.Warnf("error cleaning up ebpf conntrack: %s", err)
	}
}

func getManager(buf io.ReaderAt, maxStateSize int) (*manager.Manager, error) {
	mgr := &manager.Manager{
		Maps: []*manager.Map{
			{Name: string(probes.ConntrackMap)},
			{Name: string(probes.ConntrackTelemetryMap)},
		},
		PerfMaps: []*manager.PerfMap{},
		Probes: []*manager.Probe{
			{Section: string(probes.ConntrackHashInsert)},
		},
	}

	opts := manager.Options{
		// Extend RLIMIT_MEMLOCK (8) size
		// On some systems, the default for RLIMIT_MEMLOCK may be as low as 64 bytes.
		// This will result in an EPERM (Operation not permitted) error, when trying to create an eBPF map
		// using bpf(2) with BPF_MAP_CREATE.
		//
		// We are setting the limit to infinity until we have a better handle on the true requirements.
		RLimit: &unix.Rlimit{
			Cur: math.MaxUint64,
			Max: math.MaxUint64,
		},
		MapSpecEditors: map[string]manager.MapSpecEditor{
			string(probes.ConntrackMap): {Type: ebpf.Hash, MaxEntries: uint32(maxStateSize), EditorFlag: manager.EditMaxEntries},
		},
	}

	err := mgr.InitWithOptions(buf, opts)
	if err != nil {
		return nil, err
	}
	return mgr, nil
}
