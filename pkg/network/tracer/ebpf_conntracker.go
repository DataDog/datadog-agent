package tracer

import "C"
import (
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
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/ebpf"
	"github.com/DataDog/ebpf/manager"
	"golang.org/x/sys/unix"
)

/*
#include "../ebpf/c/runtime/conntrack-types.h"
*/
import "C"

type conntrackTelemetry C.conntrack_telemetry_t

type ebpfConntracker struct {
	m            *manager.Manager
	ctMap        *ebpf.Map
	telemetryMap *ebpf.Map

	stats struct {
		gets            int64
		getTimeTotal    int64
		deletes         int64
		deleteTimeTotal int64
	}
}

func NewEBPFConntracker(config *config.Config) (netlink.Conntracker, error) {
	buf, err := getRuntimeCompiledConntracker(config)
	if err != nil {
		return nil, fmt.Errorf("unable to compile ebpf conntracker: %s", err)
	}

	m, err := getManager(buf, config.ConntrackMaxStateSize)
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

	return &ebpfConntracker{
		m:            m,
		ctMap:        ctMap,
		telemetryMap: telemetryMap,
	}, nil
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
	atomic.AddInt64(&e.stats.getTimeTotal, time.Now().Sub(start).Nanoseconds())
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
	atomic.AddInt64(&e.stats.deletes, 1)
	atomic.AddInt64(&e.stats.deleteTimeTotal, time.Now().Sub(start).Nanoseconds())
}

func (e *ebpfConntracker) GetStats() map[string]int64 {
	m := map[string]int64{}
	telemetry := &conntrackTelemetry{}
	if err := e.telemetryMap.Lookup(unsafe.Pointer(&zero), unsafe.Pointer(telemetry)); err != nil {
		log.Tracef("error retrieving the telemetry struct: %s", err)
	} else {
		m["registers_total"] = int64(telemetry.registers)
		m["unregisters_total"] = int64(telemetry.unregisters)
	}

	gets := atomic.LoadInt64(&e.stats.gets)
	getTimeTotal := atomic.LoadInt64(&e.stats.getTimeTotal)
	m["gets_total"] = gets
	if gets > 0 {
		m["nanoseconds_per_get"] = gets / getTimeTotal
	}

	deletes := atomic.LoadInt64(&e.stats.deletes)
	deleteTimeTotal := atomic.LoadInt64(&e.stats.deleteTimeTotal)
	m["deletes_total"] = deletes
	if deletes > 0 {
		m["nanoseconds_per_delete"] = deletes / deleteTimeTotal
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
			{Section: string(probes.ConntrackDelete)},
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
