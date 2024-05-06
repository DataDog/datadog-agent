// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"errors"
	"fmt"
	"math"
	"os"
	"slices"
	"sync"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	manager "github.com/DataDog/ebpf-manager"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	TrackAllEBPFResources = true

	bpfObjectFile = "bytecode/build/co-re/lock_contention.o"

	// bpf map names
	mapAddrFdBpfMap = "map_addr_fd"
	lockStatBpfMap  = "lock_stat"
	rangesBpfMap    = "ranges"
	timeStampBpfMap = "tstamp"

	// bpf probe name
	fnDoVfsIoctl = "do_vfs_ioctl"

	// ioctl trigget code
	ioctlCollectLocksCmd = 0x70c13

	// bpf global constants
	numCpus           = "num_cpus"
	numRanges         = "num_of_ranges"
	logTwoNumOfRanges = "log2_num_of_ranges"

	disableKptrRestrict = "1"
	enableKptrRestrict  = "2"
)

type bpfPrograms struct {
	KprobeVfsIoctl    *ebpf.Program `ebpf:"kprobe__do_vfs_ioctl"`
	TpContentionBegin *ebpf.Program `ebpf:"tracepoint__contention_begin"`
	TpContentionEnd   *ebpf.Program `ebpf:"tracepoint__contention_end"`
}

type bpfMaps struct {
	MapAddrFd *ebpf.Map `ebpf:"map_addr_fd"`
	Ranges    *ebpf.Map `ebpf:"ranges"`
	LockStats *ebpf.Map `ebpf:"lock_stat"`
}

type bpfObjects struct {
	bpfPrograms
	bpfMaps
}

type targetMap struct {
	fd     int
	id     uint32
	name   string
	mp     *ebpf.Map
	mpInfo *ebpf.MapInfo
}

var kernelAddresses = []string{
	"bpf_map_fops",
	"__per_cpu_offset",
}

type LockContentionCollector struct {
	mtx             sync.Mutex
	maxContention   *prometheus.GaugeVec
	avgContention   *prometheus.GaugeVec
	totalContention *prometheus.CounterVec

	trackedLockMemRanges map[LockRange]*targetMap
	links                []link.Link
	objects              *bpfObjects
	cpus                 uint32
	ranges               uint32
}

var (
	ContentionCollector *LockContentionCollector
)

// NewLockContentionCollector creates a prometheus.Collector for eBPF lock contention metrics
func NewLockContentionCollector() *LockContentionCollector {
	kversion, err := kernel.HostVersion()
	if err != nil {
		return nil
	}

	// the tracepoints for collecting lock contention information
	// are only available after v5.19.0
	// https://github.com/torvalds/linux/commit/16edd9b511a13e7760ed4b92ba4e39bacda5c86f
	if kversion < kernel.VersionCode(5, 19, 0) {
		return nil
	}

	ContentionCollector = &LockContentionCollector{
		maxContention: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Subsystem: "ebpf_locks",
				Name:      "_max",
				Help:      "gauge tracking maximum time a tracked lock was contended for",
			},
			[]string{"name"},
		),
		avgContention: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Subsystem: "ebpf_locks",
				Name:      "_avg",
				Help:      "gauge tracking average time a tracked lock was contended for",
			},
			[]string{"name"},
		),
		totalContention: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: "ebpf_locks",
				Name:      "_total",
				Help:      "gauge tracking total time a tracked lock was contended for",
			},
			[]string{"name"},
		),
	}

	return ContentionCollector
}

// Describe implements prometheus.Collector.Describe
func (l *LockContentionCollector) Describe(descs chan<- *prometheus.Desc) {
	l.maxContention.Describe(descs)
	l.avgContention.Describe(descs)
	l.totalContention.Describe(descs)
}

// Collect implements prometheus.Collector.Collect
func (l *LockContentionCollector) Collect(metrics chan<- prometheus.Metric) {
	l.mtx.Lock()
	defer l.mtx.Unlock()

	var cursor ebpf.MapBatchCursor
	lockRanges := make([]LockRange, l.ranges)
	contention := make([]ContentionData, l.ranges)
	if _, err := l.objects.LockStats.BatchLookup(&cursor, lockRanges, contention, nil); !errors.Is(err, ebpf.ErrKeyNotExist) {
		log.Errorf("failed to perform batch lookup for lock stats: %v", err)
		return
	}

	for i, data := range contention {
		lr := lockRanges[i]
		if lr.Start == 0 {
			continue
		}
		mp, ok := l.trackedLockMemRanges[lr]
		if !ok {
			log.Errorf("found untracked lock range [0x%d, 0x%d+0x%d]", lr.Start, lr.Start, lr.Range)
			continue
		}

		if data.Total_time > 0 {
			avgTime := data.Total_time / uint64(data.Count)
			l.maxContention.WithLabelValues(mp.name).Set(float64(data.Max_time))
			l.avgContention.WithLabelValues(mp.name).Set(float64(avgTime))
			//l.totalContention.WithLabelValues(mp.Name).Sub(data.Total_time)
		}
	}

	l.maxContention.Collect(metrics)
	l.avgContention.Collect(metrics)
}

// Initialize will collect all the memory ranges we wish to monitor in ours lock stats eBPF programs
// These memory ranges correspond to locks taken by eBPF programs and are collected by walking
// fds representing the resource of interest, for example an eBPF map.
func (l *LockContentionCollector) Initialize(trackAllResources bool) error {
	var name string
	var err error

	l.mtx.Lock()
	defer l.mtx.Unlock()

	if l.trackedLockMemRanges != nil {
		return nil
	}

	l.trackedLockMemRanges = make(map[LockRange]*targetMap)
	maps := make(map[int]*targetMap)

	mapid := ebpf.MapID(0)
	for mapid, err = ebpf.MapGetNextID(mapid); err == nil; mapid, err = ebpf.MapGetNextID(mapid) {
		mp, err := ebpf.NewMapFromID(mapid)
		if err != nil {
			continue
		}

		info, err := mp.Info()
		if err != nil {
			return err
		}

		if name, err = GetMapNameFromMapID(uint32(mapid)); err != nil {
			if !trackAllResources {
				if err := mp.Close(); err != nil {
					return fmt.Errorf("failed to close map: %w", err)
				}
				continue
			}

			// this map is not tracked as part of system-probe
			name = info.Name
		}

		maps[mp.FD()] = &targetMap{mp.FD(), uint32(mapid), name, mp, info}
	}

	constants := make(map[string]interface{})
	l.objects = new(bpfObjects)

	var ranges uint32
	var cpus uint32
	if err := LoadCOREAsset(bpfObjectFile, func(bc bytecode.AssetReader, managerOptions manager.Options) error {
		collectionSpec, err := ebpf.LoadCollectionSpecFromReader(bc)
		if err != nil {
			return fmt.Errorf("failed to load collection spec: %w", err)
		}

		c, err := kernel.PossibleCPUs()
		if err != nil {
			return fmt.Errorf("unable to get possible cpus: %w", err)
		}
		cpus = uint32(c)
		l.cpus = cpus

		ranges = estimateNumOfLockRanges(maps, cpus)
		l.ranges = ranges
		collectionSpec.Maps[mapAddrFdBpfMap].MaxEntries = ranges
		collectionSpec.Maps[lockStatBpfMap].MaxEntries = ranges
		collectionSpec.Maps[rangesBpfMap].MaxEntries = ranges
		collectionSpec.Maps[timeStampBpfMap].MaxEntries = 16384

		addrs, err := getKernelSymbolsAddressesWithKptrRestrict(kernelAddresses...)
		if err != nil {
			return fmt.Errorf("unable to fetch kernel symbol addresses: %w", err)
		}

		constants[numCpus] = uint64(cpus)
		for ksym, addr := range addrs {
			constants[ksym] = addr
		}
		constants[numRanges] = uint64(ranges)
		constants[logTwoNumOfRanges] = uint64(math.Log2(float64(ranges)))

		if err := collectionSpec.RewriteConstants(constants); err != nil {
			return fmt.Errorf("failed to write constant: %w", err)
		}

		opts := ebpf.CollectionOptions{
			Programs: ebpf.ProgramOptions{
				LogLevel:    ebpf.LogLevelBranch,
				LogSize:     10 * 1024 * 1024,
				KernelTypes: managerOptions.VerifierOptions.Programs.KernelTypes,
			},
		}

		if err := collectionSpec.LoadAndAssign(l.objects, &opts); err != nil {
			return fmt.Errorf("failed to load objects: %w", err)
		}

		return nil
	}); err != nil {
		return err
	}

	kp, err := link.Kprobe(fnDoVfsIoctl, l.objects.KprobeVfsIoctl, nil)
	if err != nil {
		return fmt.Errorf("failed to attack kprobe: %w", err)
	}
	defer kp.Close()

	tpContentionBegin, err := link.AttachTracing(link.TracingOptions{
		Program: l.objects.TpContentionBegin,
	})
	if err != nil {
		return fmt.Errorf("failed to attach tracepoint: %w", err)
	}
	l.links = append(l.links, tpContentionBegin)

	tpContentionEnd, err := link.AttachTracing(link.TracingOptions{
		Program: l.objects.TpContentionEnd,
	})
	if err != nil {
		return fmt.Errorf("failed to attach tracepoint: %w", err)
	}
	l.links = append(l.links, tpContentionEnd)

	for _, tm := range maps {
		syscall.Syscall(syscall.SYS_IOCTL, uintptr(tm.fd), ioctlCollectLocksCmd, uintptr(0))

		// close all dupped maps so we do not waste fds
		tm.mp.Close()
		tm.mp = nil
		tm.mpInfo = nil
	}

	var cursor ebpf.MapBatchCursor
	lockRanges := make([]LockRange, ranges)
	fds := make([]uint32, ranges)
	count, err := l.objects.MapAddrFd.BatchLookup(&cursor, lockRanges, fds, nil)
	if !errors.Is(err, ebpf.ErrKeyNotExist) {
		return fmt.Errorf("unable to lookup up lock ranges: %w", err)
	}

	if uint32(count) < ranges {
		return fmt.Errorf("discovered fewer ranges than expected: %d < %d\n", count, ranges)
	}

	for i, fd := range fds {
		tm, ok := maps[int(fd)]
		if !ok {
			return fmt.Errorf("map with fd %d not tracked", fd)
		}
		l.trackedLockMemRanges[lockRanges[i]] = tm
	}

	// sort lock ranges and write to per cpu array map
	slices.SortFunc(lockRanges, func(a, b LockRange) int {
		return int(int64(a.Start) - int64(b.Start))
	})

	keys := make([]uint32, ranges)
	values := make([]LockRange, cpus*ranges)
	var i, j uint32
	for i = 0; i < ranges; i++ {
		keys[i] = i
		for j = 0; j < cpus; j++ {
			values[(j*ranges)+i] = lockRanges[i]
		}
	}

	if _, err := l.objects.Ranges.BatchUpdate(keys, values, nil); err != nil {
		return fmt.Errorf("unable to perform batch update on per cpu array map: %w", err)
	}

	return nil
}

// Close all eBPF resources setup up the LockContentionCollector
func (l *LockContentionCollector) Close() {
	for _, ebpfLink := range l.links {
		ebpfLink.Close()
	}

	l.objects.KprobeVfsIoctl.Close()
	l.objects.TpContentionBegin.Close()
	l.objects.TpContentionEnd.Close()
	l.objects.MapAddrFd.Close()
}

func hashMapLockRanges(cpu uint32) uint32 {
	// buckets locks + (cpu * htab->freelist.freelist) + htab->freelist.extralist
	return uint32(cpu + 2)
}

func estimateNumOfLockRanges(tm map[int]*targetMap, cpu uint32) uint32 {
	var num uint32

	for _, m := range tm {
		t := m.mpInfo.Type

		if t == ebpf.Hash || t == ebpf.PerCPUHash || t == ebpf.LRUHash || t == ebpf.LRUCPUHash || t == ebpf.HashOfMaps {
			num += hashMapLockRanges(cpu)
		}
	}

	return num
}

func setKptrRestrict(val string) error {
	kptrRestrict := "/proc/sys/kernel/kptr_restrict"

	if !(val == enableKptrRestrict || val == disableKptrRestrict) {
		return fmt.Errorf("invalid value %q to write to %q", val, kptrRestrict)
	}

	f, err := os.OpenFile(kptrRestrict, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("error opening file %q: %w", kptrRestrict, err)
	}
	defer f.Close()

	_, err = f.WriteString(val)
	if err != nil {
		return fmt.Errorf("error writing to file %q: %w", kptrRestrict, err)
	}

	return nil
}

func getKernelSymbolsAddressesWithKptrRestrict(kernelAddresses ...string) (map[string]uint64, error) {
	if err := setKptrRestrict(disableKptrRestrict); err != nil {
		return nil, fmt.Errorf("unable to disable kptr_restrict: %w", err)
	}

	addrs, err := GetKernelSymbolsAddresses(kernelAddresses...)
	if err != nil {
		return nil, err
	}

	if err := setKptrRestrict(enableKptrRestrict); err != nil {
		return nil, fmt.Errorf("unable to enable kptr_restrict: %w", err)
	}

	return addrs, nil
}
