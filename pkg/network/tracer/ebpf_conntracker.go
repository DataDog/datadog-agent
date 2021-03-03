// +build linux_bpf

package tracer

import "C"
import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/netlink"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/ebpf"
	"github.com/DataDog/ebpf/manager"
	ct "github.com/florianl/go-conntrack"
	"golang.org/x/sys/unix"
)

/*
#include "../ebpf/c/runtime/conntrack-types.h"
*/
import "C"

const (
	initializationTimeout = time.Second * 10
)

type conntrackTelemetry C.conntrack_telemetry_t

type ebpfConntracker struct {
	m            *manager.Manager
	ctMap        *ebpf.Map
	telemetryMap *ebpf.Map
	// only kept around for stats purposes from initial dump
	consumer *netlink.Consumer

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
		return nil, fmt.Errorf("unable to compile ebpf conntracker: %s", err)
	}

	m, err := getManager(buf, cfg.ConntrackMaxStateSize)
	if err != nil {
		return nil, err
	}

	err = m.Start()
	if err != nil {
		_ = m.Stop(manager.CleanAll)
		return nil, fmt.Errorf("failed to start ebpf conntracker: %s", err)
	}

	ctMap, _, err := m.GetMap(string(probes.ConntrackMap))
	if err != nil {
		_ = m.Stop(manager.CleanAll)
		return nil, fmt.Errorf("unable to get conntrack map: %s", err)
	}

	telemetryMap, _, err := m.GetMap(string(probes.ConntrackTelemetryMap))
	if err != nil {
		_ = m.Stop(manager.CleanAll)
		return nil, fmt.Errorf("unable to get telemetry map: %s", err)
	}

	e := &ebpfConntracker{
		m:            m,
		ctMap:        ctMap,
		telemetryMap: telemetryMap,
	}

	ctx, cancel := context.WithTimeout(context.Background(), initializationTimeout)
	defer cancel()

	err = e.dumpInitialTables(ctx, cfg)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("could not initialize conntrack after %s", initializationTimeout)
		}
		return nil, err
	}
	log.Infof("initialized ebpf conntrack")
	return e, nil
}

func (e *ebpfConntracker) dumpInitialTables(ctx context.Context, cfg *config.Config) error {
	var err error
	e.consumer, err = netlink.NewConsumer(cfg.ProcRoot, cfg.ConntrackRateLimit, true)
	if err != nil {
		return err
	}
	defer e.consumer.Stop()

	if err := e.loadInitialState(ctx, e.consumer.DumpTable(unix.AF_INET)); err != nil {
		return err
	}
	if err := e.loadInitialState(ctx, e.consumer.DumpTable(unix.AF_INET6)); err != nil {
		return err
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
	conns := netlink.DecodeAndReleaseEvent(ev)
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

func (e *ebpfConntracker) addTranslation(src *ConnTuple, dst *ConnTuple) error {
	if err := e.ctMap.Update(unsafe.Pointer(src), unsafe.Pointer(dst), ebpf.UpdateNoExist); err != nil && !errors.Is(err, ebpf.ErrKeyExist) {
		return err
	}
	return nil
}

func formatKey(netns uint32, tuple *ct.IPTuple) *ConnTuple {
	var proto network.ConnectionType
	switch *tuple.Proto.Number {
	case unix.IPPROTO_TCP:
		proto = network.TCP
	case unix.IPPROTO_UDP:
		proto = network.UDP
	default:
		return nil
	}

	return newConnTuple(0,
		netns,
		util.AddressFromNetIP(*tuple.Src),
		util.AddressFromNetIP(*tuple.Dst),
		*tuple.Proto.SrcPort,
		*tuple.Proto.DstPort,
		proto)
}

func (e *ebpfConntracker) GetTranslationForConn(stats network.ConnectionStats) *network.IPTranslation {
	start := time.Now()
	src := connTupleFromConnectionStats(&stats)
	src.pid = 0
	log.Tracef("looking up in conntrack: %s", src)

	var dst ConnTuple
	if err := e.ctMap.Lookup(unsafe.Pointer(src), unsafe.Pointer(&dst)); err != nil {
		if !errors.Is(err, ebpf.ErrKeyNotExist) {
			log.Warnf("error looking up connection in ebpf conntrack map: %s", err)
		}
		return nil
	}

	atomic.AddInt64(&e.stats.gets, 1)
	atomic.AddInt64(&e.stats.getTotalTime, time.Now().Sub(start).Nanoseconds())
	return &network.IPTranslation{
		ReplSrcIP:   dst.SourceAddress(),
		ReplDstIP:   dst.DestAddress(),
		ReplSrcPort: uint16(dst.sport),
		ReplDstPort: uint16(dst.dport),
	}
}

func (e *ebpfConntracker) DeleteTranslation(stats network.ConnectionStats) {
	start := time.Now()
	key := connTupleFromConnectionStats(&stats)
	key.pid = 0
	if err := e.ctMap.Delete(unsafe.Pointer(key)); err != nil {
		if errors.Is(err, ebpf.ErrKeyNotExist) {
			log.Tracef("connection does not exist in ebpf conntrack map: %s", stats)
			return
		}
		log.Warnf("unable to delete conntrack entry from eBPF map: %s", err)
	}
	atomic.AddInt64(&e.stats.unregisters, 1)
	atomic.AddInt64(&e.stats.unregistersTotalTime, time.Now().Sub(start).Nanoseconds())
}

func (e *ebpfConntracker) GetStats() map[string]int64 {
	m := map[string]int64{
		"state_size": 0,
	}
	telemetry := &conntrackTelemetry{}
	if err := e.telemetryMap.Lookup(unsafe.Pointer(&zero), unsafe.Pointer(telemetry)); err != nil {
		log.Tracef("error retrieving the telemetry struct: %s", err)
	} else {
		m["registers_total"] = int64(telemetry.registers)
		m["registers_dropped"] = int64(telemetry.registers_dropped)
	}

	gets := atomic.LoadInt64(&e.stats.gets)
	getTimeTotal := atomic.LoadInt64(&e.stats.getTotalTime)
	m["gets_total"] = gets
	if gets > 0 {
		m["nanoseconds_per_get"] = gets / getTimeTotal
	}

	unregisters := atomic.LoadInt64(&e.stats.unregisters)
	unregistersTimeTotal := atomic.LoadInt64(&e.stats.unregistersTotalTime)
	m["unregisters_total"] = unregisters
	if unregisters > 0 {
		m["nanoseconds_per_unregister"] = unregisters / unregistersTimeTotal
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

func getManager(buf io.ReaderAt, maxTrackedConnections int) (*manager.Manager, error) {
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
			string(probes.ConntrackMap): {Type: ebpf.Hash, MaxEntries: uint32(maxTrackedConnections), EditorFlag: manager.EditMaxEntries},
		},
	}

	err := mgr.InitWithOptions(buf, opts)
	if err != nil {
		return nil, err
	}
	return mgr, nil
}
