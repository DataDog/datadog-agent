// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package tracer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"sync"
	"time"
	"unsafe"

	"github.com/cihub/seelog"
	"github.com/cilium/ebpf"
	libnetlink "github.com/mdlayher/netlink"
	"go.uber.org/atomic"
	"golang.org/x/sys/unix"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/netlink"
	errtelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var tuplePool = sync.Pool{
	New: func() interface{} {
		return new(netebpf.ConntrackTuple)
	},
}

type ebpfConntrackerStats struct {
	gets                 *atomic.Int64
	getTotalTime         *atomic.Int64
	unregisters          *atomic.Int64
	unregistersTotalTime *atomic.Int64
}

func newEbpfConntrackerStats() ebpfConntrackerStats {
	return ebpfConntrackerStats{
		gets:                 atomic.NewInt64(0),
		getTotalTime:         atomic.NewInt64(0),
		unregisters:          atomic.NewInt64(0),
		unregistersTotalTime: atomic.NewInt64(0),
	}
}

type ebpfConntracker struct {
	cfg          *config.Config
	m            *manager.Manager
	ctMap        *ebpf.Map
	telemetryMap *ebpf.Map
	rootNS       uint32
	// only kept around for stats purposes from initial dump
	consumer *netlink.Consumer

	stop chan struct{}

	stats ebpfConntrackerStats
}

// NewEBPFConntracker creates a netlink.Conntracker that monitors conntrack NAT entries via eBPF
func NewEBPFConntracker(cfg *config.Config, bpfTelemetry *errtelemetry.EBPFTelemetry) (netlink.Conntracker, error) {
	// dial the netlink layer aim to load nf_conntrack_netlink and nf_conntrack kernel modules
	// eBPF conntrack require nf_conntrack symbols
	conn, err := libnetlink.Dial(unix.NETLINK_NETFILTER, nil)
	if err == nil {
		conn.Close()
	}

	buf, err := getRuntimeCompiledConntracker(cfg)
	if err != nil {
		return nil, fmt.Errorf("unable to compile ebpf conntracker: %w", err)
	}

	var mapErr *ebpf.Map
	var helperErr *ebpf.Map
	if bpfTelemetry != nil {
		mapErr = bpfTelemetry.MapErrMap
		helperErr = bpfTelemetry.HelperErrMap
	}

	m, err := getManager(cfg, buf, cfg.ConntrackMaxStateSize, mapErr, helperErr)
	if err != nil {
		return nil, err
	}
	if bpfTelemetry != nil {
		if err := bpfTelemetry.RegisterEBPFTelemetry(m); err != nil {
			return nil, fmt.Errorf("could not register ebpf telemetry: %v", err)
		}
	}

	rootNS, err := util.GetNetNsInoFromPid(cfg.ProcRoot, 1)
	if err != nil {
		return nil, fmt.Errorf("could not find network root namespace: %w", err)
	}

	e := &ebpfConntracker{
		cfg:    cfg,
		m:      m,
		rootNS: rootNS,
		stats:  newEbpfConntrackerStats(),
		stop:   make(chan struct{}),
	}

	return e, nil
}

func (e *ebpfConntracker) Start() error {
	err := e.m.Start()
	if err != nil {
		_ = e.m.Stop(manager.CleanAll)
		return fmt.Errorf("failed to start ebpf conntracker: %w", err)
	}

	e.ctMap, _, err = e.m.GetMap(string(probes.ConntrackMap))
	if err != nil {
		_ = e.m.Stop(manager.CleanAll)
		return fmt.Errorf("unable to get conntrack map: %w", err)
	}

	e.telemetryMap, _, err = e.m.GetMap(string(probes.ConntrackTelemetryMap))
	if err != nil {
		_ = e.m.Stop(manager.CleanAll)
		return fmt.Errorf("unable to get telemetry map: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), e.cfg.ConntrackInitTimeout)
	defer cancel()

	err = e.dumpInitialTables(ctx, e.cfg)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("could not initialize conntrack after %s", e.cfg.ConntrackInitTimeout)
		}
		_ = e.m.Stop(manager.CleanAll)
		return err
	}
	log.Infof("initialized ebpf conntrack")
	return nil
}

func (e *ebpfConntracker) dumpInitialTables(ctx context.Context, cfg *config.Config) error {
	var err error
	e.consumer, err = netlink.NewConsumer(cfg)
	if err != nil {
		return err
	}
	defer e.consumer.Stop()

	for _, family := range []uint8{unix.AF_INET, unix.AF_INET6} {
		done, err := e.consumer.DumpAndDiscardTable(family)
		if err != nil {
			return err
		}

		if err := e.processEvents(ctx, done); err != nil {
			return err
		}
	}
	if err := e.m.DetachHook(manager.ProbeIdentificationPair{EBPFSection: string(probes.ConntrackFillInfo), EBPFFuncName: "kprobe_ctnetlink_fill_info"}); err != nil {
		log.Debugf("detachHook %s/kprobe_ctnetlink_fill_info : %s", string(probes.ConntrackFillInfo), err)
	}
	return nil
}

func (e *ebpfConntracker) processEvents(ctx context.Context, done <-chan bool) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-done:
			return nil
		}
	}
}

func toConntrackTupleFromStats(src *netebpf.ConntrackTuple, stats *network.ConnectionStats) {
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
	if log.ShouldLog(seelog.TraceLvl) {
		log.Tracef("looking up in conntrack (stats): %s", stats)
	}

	// Try the lookup in the root namespace first
	src.Netns = e.rootNS
	if log.ShouldLog(seelog.TraceLvl) {
		log.Tracef("looking up in conntrack (tuple): %s", src)
	}
	dst := e.get(src)

	if dst == nil && stats.NetNS != e.rootNS {
		// Perform another lookup, this time using the connection namespace
		src.Netns = stats.NetNS
		if log.ShouldLog(seelog.TraceLvl) {
			log.Tracef("looking up in conntrack (tuple): %s", src)
		}
		dst = e.get(src)
	}

	if dst == nil {
		return nil
	}
	defer tuplePool.Put(dst)

	e.stats.gets.Inc()
	e.stats.getTotalTime.Add(time.Now().Sub(start).Nanoseconds())
	return &network.IPTranslation{
		ReplSrcIP:   dst.SourceAddress(),
		ReplDstIP:   dst.DestAddress(),
		ReplSrcPort: dst.Sport,
		ReplDstPort: dst.Dport,
	}
}

func (*ebpfConntracker) IsSampling() bool {
	return false
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
	e.stats.unregisters.Inc()
	e.stats.unregistersTotalTime.Add(time.Now().Sub(start).Nanoseconds())
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
	}

	gets := e.stats.gets.Load()
	getTimeTotal := e.stats.getTotalTime.Load()
	m["gets_total"] = gets
	if gets > 0 {
		m["nanoseconds_per_get"] = getTimeTotal / gets
	}

	unregisters := e.stats.unregisters.Load()
	unregistersTimeTotal := e.stats.unregistersTotalTime.Load()
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

// DumpCachedTable dumps the cached conntrack NAT entries grouped by network namespace
func (e *ebpfConntracker) DumpCachedTable(ctx context.Context) (map[uint32][]netlink.DebugConntrackEntry, error) {
	src := tuplePool.Get().(*netebpf.ConntrackTuple)
	defer tuplePool.Put(src)
	dst := tuplePool.Get().(*netebpf.ConntrackTuple)
	defer tuplePool.Put(dst)

	entries := make(map[uint32][]netlink.DebugConntrackEntry)

	it := e.ctMap.Iterate()
	for it.Next(unsafe.Pointer(src), unsafe.Pointer(dst)) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		_, ok := entries[src.Netns]
		if !ok {
			entries[src.Netns] = []netlink.DebugConntrackEntry{}
		}
		entries[src.Netns] = append(entries[src.Netns], netlink.DebugConntrackEntry{
			Family: src.Family().String(),
			Proto:  network.ConnectionType(src.Type()).String(),
			Origin: netlink.DebugConntrackTuple{
				Src: netlink.DebugConntrackAddress{
					IP:   src.SourceAddress().String(),
					Port: src.Sport,
				},
				Dst: netlink.DebugConntrackAddress{
					IP:   src.DestAddress().String(),
					Port: src.Dport,
				},
			},
			Reply: netlink.DebugConntrackTuple{
				Src: netlink.DebugConntrackAddress{
					IP:   dst.SourceAddress().String(),
					Port: dst.Sport,
				},
				Dst: netlink.DebugConntrackAddress{
					IP:   dst.DestAddress().String(),
					Port: dst.Dport,
				},
			},
		})
	}
	if it.Err() != nil {
		return nil, it.Err()
	}
	return entries, nil
}

func getManager(cfg *config.Config, buf io.ReaderAt, maxStateSize int, mapErrTelemetryMap, helperErrTelemetryMap *ebpf.Map) (*manager.Manager, error) {
	mgr := &manager.Manager{
		Maps: []*manager.Map{
			{Name: string(probes.ConntrackMap)},
			{Name: string(probes.ConntrackTelemetryMap)},
		},
		PerfMaps: []*manager.PerfMap{},
		Probes: []*manager.Probe{
			{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  string(probes.ConntrackHashInsert),
					EBPFFuncName: "kprobe___nf_conntrack_hash_insert",
					UID:          "conntracker",
				},
			},
			{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  string(probes.ConntrackFillInfo),
					EBPFFuncName: "kprobe_ctnetlink_fill_info",
					UID:          "conntracker",
				},
			},
		},
	}

	currKernelVersion, err := kernel.HostVersion()
	if err != nil {
		return nil, errors.New("failed to detect kernel version")
	}
	activateBPFTelemetry := currKernelVersion >= kernel.VersionCode(4, 14, 0)
	mgr.InstructionPatcher = func(m *manager.Manager) error {
		return errtelemetry.PatchEBPFTelemetry(m, activateBPFTelemetry, []manager.ProbeIdentificationPair{})
	}

	telemetryMapKeys := errtelemetry.BuildTelemetryKeys(mgr)

	kprobeAttachMethod := manager.AttachKprobeWithPerfEventOpen
	if cfg.AttachKprobesWithKprobeEventsABI {
		kprobeAttachMethod = manager.AttachKprobeWithKprobeEvents
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
			string(probes.ConntrackMap): {Type: ebpf.Hash, MaxEntries: uint32(cfg.ConntrackMaxStateSize), EditorFlag: manager.EditMaxEntries},
		},
		ConstantEditors:           telemetryMapKeys,
		DefaultKprobeAttachMethod: kprobeAttachMethod,
	}
	if (mapErrTelemetryMap != nil) || (helperErrTelemetryMap != nil) {
		opts.MapEditors = make(map[string]*ebpf.Map)
	}

	if mapErrTelemetryMap != nil {
		opts.MapEditors[string(probes.MapErrTelemetryMap)] = mapErrTelemetryMap
	}
	if helperErrTelemetryMap != nil {
		opts.MapEditors[string(probes.HelperErrTelemetryMap)] = helperErrTelemetryMap
	}

	err = mgr.InitWithOptions(buf, opts)
	if err != nil {
		return nil, err
	}
	return mgr, nil
}
