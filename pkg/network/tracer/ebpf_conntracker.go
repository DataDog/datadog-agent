// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

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

	nettelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"

	"github.com/cihub/seelog"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/features"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sys/unix"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/ebpfcheck"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/netlink"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/offsetguess"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var zero uint64

var tuplePool = sync.Pool{
	New: func() interface{} {
		return new(netebpf.ConntrackTuple)
	},
}

const ebpfConntrackerModuleName = "network_tracer__ebpf_conntracker"

var defaultBuckets = []float64{10, 25, 50, 75, 100, 250, 500, 1000, 10000}

var conntrackerTelemetry = struct {
	getsDuration        telemetry.Histogram
	unregistersDuration telemetry.Histogram
	getsTotal           telemetry.Counter
	unregistersTotal    telemetry.Counter
	registersTotal      *prometheus.Desc
	lastRegisters       uint64
}{
	telemetry.NewHistogram(ebpfConntrackerModuleName, "gets_duration_nanoseconds", []string{}, "Histogram measuring the time spent retrieving connection tuples from the EBPF map", defaultBuckets),
	telemetry.NewHistogram(ebpfConntrackerModuleName, "unregisters_duration_nanoseconds", []string{}, "Histogram measuring the time spent deleting connection tuples from the EBPF map", defaultBuckets),
	telemetry.NewCounter(ebpfConntrackerModuleName, "gets_total", []string{}, "Counter measuring the total number of attempts to get connection tuples from the EBPF map"),
	telemetry.NewCounter(ebpfConntrackerModuleName, "unregisters_total", []string{}, "Counter measuring the total number of attempts to delete connection tuples from the EBPF map"),
	prometheus.NewDesc(ebpfConntrackerModuleName+"__registers_total", "Counter measuring the total number of attempts to update/create connection tuples in the EBPF map", nil, nil),
	0,
}

type ebpfConntracker struct {
	m            *manager.Manager
	ctMap        *ebpf.Map
	telemetryMap *ebpf.Map
	rootNS       uint32
	// only kept around for stats purposes from initial dump
	consumer *netlink.Consumer

	stop chan struct{}

	isPrebuilt bool
}

var ebpfConntrackerRCCreator func(cfg *config.Config) (runtime.CompiledOutput, error) = getRuntimeCompiledConntracker
var ebpfConntrackerPrebuiltCreator func(*config.Config) (bytecode.AssetReader, []manager.ConstantEditor, error) = getPrebuiltConntracker

// NewEBPFConntracker creates a netlink.Conntracker that monitor conntrack NAT entries via eBPF
func NewEBPFConntracker(cfg *config.Config, bpfTelemetry *nettelemetry.EBPFTelemetry) (netlink.Conntracker, error) {
	if !cfg.EnableEbpfConntracker {
		return nil, fmt.Errorf("ebpf conntracker is disabled")
	}

	var err error
	var buf bytecode.AssetReader
	if cfg.EnableRuntimeCompiler {
		buf, err = ebpfConntrackerRCCreator(cfg)
		if err != nil {
			if !cfg.AllowPrecompiledFallback {
				return nil, fmt.Errorf("unable to compile ebpf conntracker: %w", err)
			}

			log.Warnf("unable to compile ebpf conntracker, falling back to prebuilt ebpf conntracker: %s", err)
		}
	}

	var isPrebuilt bool
	var constants []manager.ConstantEditor
	if buf == nil {
		buf, constants, err = ebpfConntrackerPrebuiltCreator(cfg)
		if err != nil {
			return nil, fmt.Errorf("could not load prebuilt ebpf conntracker: %w", err)
		}

		isPrebuilt = true
	}

	defer buf.Close()

	m, err := getManager(cfg, buf, bpfTelemetry, constants)
	if err != nil {
		return nil, err
	}

	err = m.Start()
	if err != nil {
		_ = m.Stop(manager.CleanAll)
		return nil, fmt.Errorf("failed to start ebpf conntracker: %w", err)
	}

	ctMap, _, err := m.GetMap(probes.ConntrackMap)
	if err != nil {
		_ = m.Stop(manager.CleanAll)
		return nil, fmt.Errorf("unable to get conntrack map: %w", err)
	}

	telemetryMap, _, err := m.GetMap(probes.ConntrackTelemetryMap)
	if err != nil {
		_ = m.Stop(manager.CleanAll)
		return nil, fmt.Errorf("unable to get telemetry map: %w", err)
	}

	rootNS, err := kernel.GetNetNsInoFromPid(cfg.ProcRoot, 1)
	if err != nil {
		return nil, fmt.Errorf("could not find network root namespace: %w", err)
	}

	e := &ebpfConntracker{
		m:            m,
		ctMap:        ctMap,
		telemetryMap: telemetryMap,
		rootNS:       rootNS,
		stop:         make(chan struct{}),
		isPrebuilt:   isPrebuilt,
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
	if err := e.m.DetachHook(manager.ProbeIdentificationPair{EBPFFuncName: probes.ConntrackFillInfo}); err != nil {
		log.Debugf("detachHook %s : %s", probes.ConntrackFillInfo, err)
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
			log.Tracef("looking up in conntrack (tuple,netns): %s", src)
		}
		dst = e.get(src)
	}

	if dst == nil {
		return nil
	}
	defer tuplePool.Put(dst)

	conntrackerTelemetry.getsTotal.Inc()
	conntrackerTelemetry.getsDuration.Observe(float64(time.Since(start).Nanoseconds()))
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
	start := time.Now()
	if err := e.ctMap.Delete(unsafe.Pointer(key)); err != nil {
		if errors.Is(err, ebpf.ErrKeyNotExist) {
			if log.ShouldLog(seelog.TraceLvl) {
				log.Tracef("connection does not exist in ebpf conntrack map: %s", key)
			}
			return
		}
		log.Warnf("unable to delete conntrack entry from eBPF map: %s", err)
	} else {
		conntrackerTelemetry.unregistersTotal.Inc()
		conntrackerTelemetry.unregistersDuration.Observe(float64(time.Since(start).Nanoseconds()))
	}
}

func (e *ebpfConntracker) DeleteTranslation(stats network.ConnectionStats) {
	key := tuplePool.Get().(*netebpf.ConntrackTuple)
	defer tuplePool.Put(key)

	toConntrackTupleFromStats(key, &stats)

	dst := e.get(key)
	e.delete(key)
	if dst != nil {
		e.delete(dst)
		tuplePool.Put(dst)
	}
}

func (e *ebpfConntracker) GetTelemetryMap() *ebpf.Map {
	return e.telemetryMap
}

func (e *ebpfConntracker) Close() {
	ebpfcheck.RemoveNameMappings(e.m)
	err := e.m.Stop(manager.CleanAll)
	if err != nil {
		log.Warnf("error cleaning up ebpf conntrack: %s", err)
	}
	close(e.stop)
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
			Proto:  src.Type().String(),
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

// Describe returns all descriptions of the collector
func (e *ebpfConntracker) Describe(ch chan<- *prometheus.Desc) {
	ch <- conntrackerTelemetry.registersTotal
}

// Collect returns the current state of all metrics of the collector
func (e *ebpfConntracker) Collect(ch chan<- prometheus.Metric) {
	ebpfTelemetry := &netebpf.ConntrackTelemetry{}
	if err := e.telemetryMap.Lookup(unsafe.Pointer(&zero), unsafe.Pointer(ebpfTelemetry)); err != nil {
		log.Tracef("error retrieving the telemetry struct: %s", err)
	} else {
		delta := ebpfTelemetry.Registers - conntrackerTelemetry.lastRegisters
		conntrackerTelemetry.lastRegisters = ebpfTelemetry.Registers
		ch <- prometheus.MustNewConstMetric(conntrackerTelemetry.registersTotal, prometheus.CounterValue, float64(delta))
	}
}

func getManager(cfg *config.Config, buf io.ReaderAt, bpfTelemetry *nettelemetry.EBPFTelemetry, constants []manager.ConstantEditor) (*manager.Manager, error) {
	mgr := nettelemetry.NewManager(&manager.Manager{
		Maps: []*manager.Map{
			{Name: probes.ConntrackMap},
			{Name: probes.ConntrackTelemetryMap},
		},
		PerfMaps: []*manager.PerfMap{},
		Probes: []*manager.Probe{
			{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: probes.ConntrackHashInsert,
					UID:          "conntracker",
				},
			},
			{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: probes.ConntrackFillInfo,
					UID:          "conntracker",
				},
				MatchFuncName: "^ctnetlink_fill_info(\\.constprop\\.0)?$",
			},
		},
	}, bpfTelemetry)

	kprobeAttachMethod := manager.AttachKprobeWithPerfEventOpen
	if cfg.AttachKprobesWithKprobeEventsABI {
		kprobeAttachMethod = manager.AttachKprobeWithKprobeEvents
	}

	pid, err := kernel.RootNSPID()
	if err != nil {
		return nil, fmt.Errorf("failed to get system-probe pid in root pid namespace")
	}

	constants = append(constants, manager.ConstantEditor{
		Name:  "systemprobe_pid",
		Value: uint64(pid),
	})

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
			probes.ConntrackMap: {MaxEntries: uint32(cfg.ConntrackMaxStateSize), EditorFlag: manager.EditMaxEntries},
		},
		ConstantEditors:           constants,
		DefaultKprobeAttachMethod: kprobeAttachMethod,
		MapEditors:                make(map[string]*ebpf.Map),
		VerifierOptions: ebpf.CollectionOptions{
			Programs: ebpf.ProgramOptions{
				LogSize: 10 * 1024 * 1024,
			},
		},
	}

	if err := features.HaveMapType(ebpf.LRUHash); err == nil {
		me := opts.MapSpecEditors[probes.ConntrackMap]
		me.Type = ebpf.LRUHash
		me.EditorFlag |= manager.EditType
	}

	err = mgr.InitWithOptions(buf, opts)
	if err != nil {
		return nil, err
	}
	ebpfcheck.AddNameMappings(mgr.Manager, "npm_conntracker")
	return mgr.Manager, nil
}

func getPrebuiltConntracker(cfg *config.Config) (bytecode.AssetReader, []manager.ConstantEditor, error) {
	buf, err := netebpf.ReadConntrackBPFModule(cfg.BPFDir, cfg.BPFDebug)
	if err != nil {
		return nil, nil, fmt.Errorf("could not read bpf module: %s", err)
	}

	offsetBuf, err := netebpf.ReadOffsetBPFModule(cfg.BPFDir, cfg.BPFDebug)
	if err != nil {
		return nil, nil, fmt.Errorf("could not load offset guessing module: %w", err)
	}
	defer offsetBuf.Close()

	constants, err := offsetguess.RunOffsetGuessing(cfg, offsetBuf, func() (offsetguess.OffsetGuesser, error) {
		return offsetguess.NewConntrackOffsetGuesser(cfg)
	})
	if err != nil {
		return nil, nil, fmt.Errorf("could not guess offsets for ebpf conntracker: %w", err)
	}

	return buf, constants, nil
}
