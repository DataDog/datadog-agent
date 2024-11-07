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
	"net/netip"
	"time"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cihub/seelog"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/features"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sys/unix"

	telemetryComp "github.com/DataDog/datadog-agent/comp/core/telemetry"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/maps"
	"github.com/DataDog/datadog-agent/pkg/ebpf/prebuilt"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/netlink"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/offsetguess"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	ebpfkernel "github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
)

var zero uint32

var tuplePool = ddsync.NewDefaultTypedPool[netebpf.ConntrackTuple]()

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
	ctMap        *maps.GenericMap[netebpf.ConntrackTuple, netebpf.ConntrackTuple]
	telemetryMap *maps.GenericMap[uint32, netebpf.ConntrackTelemetry]
	rootNS       uint32
	// only kept around for stats purposes from initial dump
	consumer *netlink.Consumer

	stop chan struct{}

	isPrebuilt bool
}

var ebpfConntrackerCORECreator func(cfg *config.Config) (*manager.Manager, error) = getCOREConntracker
var ebpfConntrackerRCCreator func(cfg *config.Config) (*manager.Manager, error) = getRCConntracker
var ebpfConntrackerPrebuiltCreator func(cfg *config.Config) (*manager.Manager, error) = getPrebuiltConntracker

// NewEBPFConntracker creates a netlink.Conntracker that monitor conntrack NAT entries via eBPF
func NewEBPFConntracker(cfg *config.Config, telemetrycomp telemetryComp.Component) (netlink.Conntracker, error) {
	allowRC := cfg.EnableRuntimeCompiler
	var m *manager.Manager
	var err error
	if cfg.EnableCORE {
		m, err = ebpfConntrackerCORECreator(cfg)
		if err != nil {
			if cfg.EnableRuntimeCompiler && cfg.AllowRuntimeCompiledFallback {
				log.Warnf("error loading CO-RE conntracker, falling back to runtime compiled: %s", err)
			} else if cfg.AllowPrebuiltFallback {
				allowRC = false
				log.Warnf("error loading CO-RE conntracker, falling back to prebuilt: %s", err)
			} else {
				return nil, fmt.Errorf("error loading CO-RE conntracker: %w", err)
			}
		}
	}

	if m == nil && allowRC {
		m, err = ebpfConntrackerRCCreator(cfg)
		if err != nil {
			if !cfg.AllowPrebuiltFallback {
				return nil, fmt.Errorf("unable to compile ebpf conntracker: %w", err)
			}

			log.Warnf("unable to compile ebpf conntracker, falling back to prebuilt ebpf conntracker: %s", err)
		}
	}

	var isPrebuilt bool
	if m == nil {
		m, err = ebpfConntrackerPrebuiltCreator(cfg)
		if err != nil {
			return nil, fmt.Errorf("could not load prebuilt ebpf conntracker: %w", err)
		}

		isPrebuilt = true
	}

	if isPrebuilt && prebuilt.IsDeprecated() {
		log.Warn("using deprecated prebuilt conntracker")
	}

	err = m.Start()
	if err != nil {
		_ = m.Stop(manager.CleanAll)
		return nil, fmt.Errorf("failed to start ebpf conntracker: %w", err)
	}

	ctMap, err := maps.GetMap[netebpf.ConntrackTuple, netebpf.ConntrackTuple](m, probes.ConntrackMap)
	if err != nil {
		_ = m.Stop(manager.CleanAll)
		return nil, fmt.Errorf("unable to get conntrack map: %w", err)
	}

	telemetryMap, err := maps.GetMap[uint32, netebpf.ConntrackTelemetry](m, probes.ConntrackTelemetryMap)
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

	err = e.dumpInitialTables(ctx, cfg, telemetrycomp)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("could not initialize conntrack after %s", cfg.ConntrackInitTimeout)
		}
		return nil, err
	}
	log.Infof("initialized ebpf conntracker")
	return e, nil
}

func (e *ebpfConntracker) dumpInitialTables(ctx context.Context, cfg *config.Config, telemetrycomp telemetryComp.Component) error {
	var err error
	e.consumer, err = netlink.NewConsumer(cfg, telemetrycomp)
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

func toConntrackTupleFromTuple(src *netebpf.ConntrackTuple, stats *network.ConnectionTuple) {
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

// GetType returns a string describing whether the conntracker is "ebpf" or "netlink"
func (e *ebpfConntracker) GetType() string {
	return "ebpf"
}

func (e *ebpfConntracker) GetTranslationForConn(stats *network.ConnectionTuple) *network.IPTranslation {
	start := time.Now()
	src := tuplePool.Get()
	defer tuplePool.Put(src)

	toConntrackTupleFromTuple(src, stats)
	if log.ShouldLog(seelog.TraceLvl) {
		log.Tracef("looking up in conntrack (stats): %s", stats)
	}

	// Try the lookup in the root namespace first, since usually
	// NAT rules referencing conntrack are installed there instead
	// of other network namespaces (for pods, for instance)
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
	dst := tuplePool.Get()
	if err := e.ctMap.Lookup(src, dst); err != nil {
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
	defer func() {
		conntrackerTelemetry.unregistersDuration.Observe(float64(time.Since(start).Nanoseconds()))
	}()

	if err := e.ctMap.Delete(key); err != nil {
		if errors.Is(err, ebpf.ErrKeyNotExist) {
			if log.ShouldLog(seelog.TraceLvl) {
				log.Tracef("connection does not exist in ebpf conntrack map: %s", key)
			}

			return
		}

		log.Warnf("unable to delete conntrack entry from eBPF map: %s", err)
		return
	}

	conntrackerTelemetry.unregistersTotal.Inc()
}

func (e *ebpfConntracker) deleteTranslationNs(key *netebpf.ConntrackTuple, ns uint32) *netebpf.ConntrackTuple {
	key.Netns = ns
	dst := e.get(key)
	e.delete(key)
	if dst != nil {
		e.delete(dst)
	}

	return dst
}

func (e *ebpfConntracker) DeleteTranslation(stats *network.ConnectionTuple) {
	key := tuplePool.Get()
	defer tuplePool.Put(key)

	toConntrackTupleFromTuple(key, stats)

	// attempt a delete from both root and connection's network namespace
	if dst := e.deleteTranslationNs(key, e.rootNS); dst != nil {
		tuplePool.Put(dst)
	}

	if dst := e.deleteTranslationNs(key, stats.NetNS); dst != nil {
		tuplePool.Put(dst)
	}
}

func (e *ebpfConntracker) GetTelemetryMap() *ebpf.Map {
	return e.telemetryMap.Map()
}

func (e *ebpfConntracker) Close() {
	ddebpf.RemoveNameMappings(e.m)
	err := e.m.Stop(manager.CleanAll)
	if err != nil {
		log.Warnf("error cleaning up ebpf conntrack: %s", err)
	}
	close(e.stop)
}

// DumpCachedTable dumps the cached conntrack NAT entries grouped by network namespace
func (e *ebpfConntracker) DumpCachedTable(ctx context.Context) (map[uint32][]netlink.DebugConntrackEntry, error) {
	src := tuplePool.Get()
	defer tuplePool.Put(src)
	dst := tuplePool.Get()
	defer tuplePool.Put(dst)

	entries := make(map[uint32][]netlink.DebugConntrackEntry)

	it := e.ctMap.Iterate()
	for it.Next(src, dst) {
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
				Src: netip.AddrPortFrom(src.SourceAddress().Addr, src.Sport),
				Dst: netip.AddrPortFrom(src.DestAddress().Addr, src.Dport),
			},
			Reply: netlink.DebugConntrackTuple{
				Src: netip.AddrPortFrom(dst.SourceAddress().Addr, dst.Sport),
				Dst: netip.AddrPortFrom(dst.DestAddress().Addr, dst.Dport),
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
	if err := e.telemetryMap.Lookup(&zero, ebpfTelemetry); err != nil {
		log.Tracef("error retrieving the telemetry struct: %s", err)
	} else {
		delta := ebpfTelemetry.Registers - conntrackerTelemetry.lastRegisters
		conntrackerTelemetry.lastRegisters = ebpfTelemetry.Registers
		ch <- prometheus.MustNewConstMetric(conntrackerTelemetry.registersTotal, prometheus.CounterValue, float64(delta))
	}
}

func getManager(cfg *config.Config, buf io.ReaderAt, opts manager.Options) (*manager.Manager, error) {
	mgr := ddebpf.NewManagerWithDefault(&manager.Manager{
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
	}, &ebpftelemetry.ErrorsTelemetryModifier{})

	opts.DefaultKprobeAttachMethod = manager.AttachKprobeWithPerfEventOpen
	if cfg.AttachKprobesWithKprobeEventsABI {
		opts.DefaultKprobeAttachMethod = manager.AttachKprobeWithKprobeEvents
	}

	pid, err := kernel.RootNSPID()
	if err != nil {
		return nil, fmt.Errorf("failed to get system-probe pid in root pid namespace")
	}

	opts.ConstantEditors = append(opts.ConstantEditors, manager.ConstantEditor{
		Name:  "systemprobe_pid",
		Value: uint64(pid),
	})

	if opts.MapSpecEditors == nil {
		opts.MapSpecEditors = make(map[string]manager.MapSpecEditor)
	}
	opts.MapSpecEditors[probes.ConntrackMap] = manager.MapSpecEditor{MaxEntries: uint32(cfg.ConntrackMaxStateSize), EditorFlag: manager.EditMaxEntries}
	if opts.MapEditors == nil {
		opts.MapEditors = make(map[string]*ebpf.Map)
	}
	opts.BypassEnabled = cfg.BypassEnabled

	if err := features.HaveMapType(ebpf.LRUHash); err == nil {
		me := opts.MapSpecEditors[probes.ConntrackMap]
		me.Type = ebpf.LRUHash
		me.EditorFlag |= manager.EditType
		opts.MapSpecEditors[probes.ConntrackMap] = me
	}

	err = mgr.InitWithOptions(buf, &opts)
	if err != nil {
		return nil, err
	}
	ddebpf.AddNameMappings(mgr.Manager, "npm_conntracker")
	return mgr.Manager, nil
}

var errPrebuiltConntrackerUnsupported = errors.New("prebuilt ebpf conntracker requires kernel version 4.14 or higher or a RHEL kernel with backported eBPF support")
var errCOREConntrackerUnsupported = errors.New("CO-RE ebpf conntracker requires kernel version 4.14 or higher or a RHEL kernel with backported eBPF support")

func getPrebuiltConntracker(cfg *config.Config) (*manager.Manager, error) {
	supportedOnKernel, err := ebpfPrebuiltConntrackerSupportedOnKernel()
	if err != nil {
		return nil, fmt.Errorf("could not check if prebuilt ebpf conntracker is supported on kernel: %w", err)
	}
	if !supportedOnKernel {
		return nil, errPrebuiltConntrackerUnsupported
	}

	buf, err := netebpf.ReadConntrackBPFModule(cfg.BPFDir, cfg.BPFDebug)
	if err != nil {
		return nil, fmt.Errorf("could not read bpf module: %s", err)
	}
	defer buf.Close()

	offsetBuf, err := netebpf.ReadOffsetBPFModule(cfg.BPFDir, cfg.BPFDebug)
	if err != nil {
		return nil, fmt.Errorf("could not load offset guessing module: %w", err)
	}
	defer offsetBuf.Close()

	constants, err := offsetguess.RunOffsetGuessing(cfg, offsetBuf, func() (offsetguess.OffsetGuesser, error) {
		return offsetguess.NewConntrackOffsetGuesser(cfg)
	})
	if err != nil {
		return nil, fmt.Errorf("could not guess offsets for ebpf conntracker: %w", err)
	}

	opts := manager.Options{ConstantEditors: constants}
	return getManager(cfg, buf, opts)
}

func ebpfPrebuiltConntrackerSupportedOnKernel() (bool, error) {
	kv, err := ebpfkernel.NewKernelVersion()
	if err != nil {
		return false, fmt.Errorf("could not get kernel version: %s", err)
	}

	if kv.Code >= ebpfkernel.Kernel4_14 || kv.IsRH7Kernel() {
		return true, nil
	}
	return false, nil
}

func ebpfCOREConntrackerSupportedOnKernel() (bool, error) {
	kv, err := ebpfkernel.NewKernelVersion()
	if err != nil {
		return false, fmt.Errorf("could not get kernel version: %s", err)
	}

	if kv.Code >= ebpfkernel.Kernel4_14 || kv.IsRH7Kernel() {
		return true, nil
	}
	return false, nil
}

func getRCConntracker(cfg *config.Config) (*manager.Manager, error) {
	buf, err := getRuntimeCompiledConntracker(cfg)
	if err != nil {
		return nil, err
	}
	defer buf.Close()

	return getManager(cfg, buf, manager.Options{})
}

func getCOREConntracker(cfg *config.Config) (*manager.Manager, error) {
	supportedOnKernel, err := ebpfCOREConntrackerSupportedOnKernel()
	if err != nil {
		return nil, fmt.Errorf("could not check if CO-RE ebpf conntracker is supported on kernel: %w", err)
	}
	if !supportedOnKernel {
		return nil, errCOREConntrackerUnsupported
	}

	var m *manager.Manager
	err = ddebpf.LoadCOREAsset(netebpf.ModuleFileName("conntrack", cfg.BPFDebug), func(ar bytecode.AssetReader, o manager.Options) error {
		o.ConstantEditors = append(o.ConstantEditors,
			boolConst("tcpv6_enabled", cfg.CollectTCPv6Conns),
			boolConst("udpv6_enabled", cfg.CollectUDPv6Conns),
		)
		m, err = getManager(cfg, ar, o)
		return err
	})
	return m, err
}

func boolConst(name string, value bool) manager.ConstantEditor {
	c := manager.ConstantEditor{
		Name:  name,
		Value: uint64(1),
	}
	if !value {
		c.Value = uint64(0)
	}

	return c
}
