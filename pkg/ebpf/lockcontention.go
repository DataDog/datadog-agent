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
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	manager "github.com/DataDog/ebpf-manager"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	// TrackAllEBPFResources decides if all system ebpf resources should be collected
	// or just system-probe resources
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

	// maximum lock ranges to track
	maxTrackedRanges = 16384
)

// always use maxTrackedRanges
var staticRanges = false

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

type mapStats struct {
	targetMap
	totalTime uint64
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
	"bpf_dummy_read",
}

// LockContentionCollector implements the prometheus Collector interface
// for exposing metrics
type LockContentionCollector struct {
	mtx             sync.Mutex
	maxContention   *prometheus.GaugeVec
	avgContention   *prometheus.GaugeVec
	totalContention *prometheus.CounterVec

	trackedLockMemRanges map[LockRange]*mapStats
	links                []link.Link
	objects              *bpfObjects
	cpus                 uint32
	ranges               uint32

	initialized bool
}

// ContentionCollector is the global stats collector
var ContentionCollector *LockContentionCollector

var lockTypes = map[uint32]string{
	1: "hash-bucket-locks",
	2: "hash-pcpu-freelist-locks",
	3: "hash-global-freelist-locks",
	4: "percpu-lru-freelist-locks",
	5: "lru-global-freelist-locks",
	6: "lru-pcpu-freelist-locks",
	7: "ringbuf-spinlock",
	8: "ringbuf-waitq-spinlock",
}

func lockContentionCollectorSupported() bool {
	if _, err := os.Stat("/sys/kernel/tracing/events/lock/contention_begin/id"); errors.Is(err, os.ErrNotExist) {
		return false
	}

	if _, err := os.Stat("/sys/kernel/tracing/events/lock/contention_end/id"); errors.Is(err, os.ErrNotExist) {
		return false
	}

	return true
}

// NewLockContentionCollector creates a prometheus.Collector for eBPF lock contention metrics
func NewLockContentionCollector() *LockContentionCollector {
	if !lockContentionCollectorSupported() {
		return nil
	}

	ContentionCollector = &LockContentionCollector{
		maxContention: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Subsystem: "ebpf__locks",
				Name:      "_max",
				Help:      "gauge tracking maximum time a tracked lock was contended for",
			},
			[]string{"name", "lock_type"},
		),
		avgContention: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Subsystem: "ebpf__locks",
				Name:      "_avg",
				Help:      "gauge tracking average time a tracked lock was contended for",
			},
			[]string{"name", "lock_type"},
		),
		totalContention: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: "ebpf__locks",
				Name:      "_total",
				Help:      "counter tracking total time a tracked lock was contended for",
			},
			[]string{"name", "lock_type"},
		),
	}

	return ContentionCollector
}

// Describe implements prometheus.Collector.Describe
func (l *LockContentionCollector) Describe(descs chan<- *prometheus.Desc) {
	// ContentionCollector not initialized without kernel version support
	if l == nil {
		return
	}

	l.maxContention.Describe(descs)
	l.avgContention.Describe(descs)
	l.totalContention.Describe(descs)
}

// Collect implements prometheus.Collector.Collect
func (l *LockContentionCollector) Collect(metrics chan<- prometheus.Metric) {
	// ContentionCollector not initialized without kernel version support
	if l == nil {
		return
	}

	l.mtx.Lock()
	defer l.mtx.Unlock()

	if !l.initialized {
		return
	}

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

		if (data.Total_time > 0) && (mp.totalTime != data.Total_time) {
			avgTime := data.Total_time / uint64(data.Count)

			lockType := lockTypes[lr.Type]
			l.maxContention.WithLabelValues(mp.name, lockType).Set(float64(data.Max_time))
			l.avgContention.WithLabelValues(mp.name, lockType).Set(float64(avgTime))

			// TODO: should we consider overflows. u64 overflow seems very unlikely?
			l.totalContention.WithLabelValues(mp.name, lockType).Add(float64(data.Total_time - mp.totalTime))
			mp.totalTime = data.Total_time
		}
	}

	l.maxContention.Collect(metrics)
	l.avgContention.Collect(metrics)
}

// Initialize will collect all the memory ranges we wish to monitor in our lock stats eBPF programs
// These memory ranges correspond to locks taken by eBPF programs and are collected by walking the
// fds representing the resource of interest, for example an eBPF map.
func (l *LockContentionCollector) Initialize(trackAllResources bool) error {
	var name string
	var err error

	l.mtx.Lock()
	defer l.mtx.Unlock()

	if l.initialized {
		return nil
	}
	defer func() {
		l.initialized = true
	}()

	l.trackedLockMemRanges = make(map[LockRange]*mapStats)
	maps := make(map[uint32]*targetMap)

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

		maps[uint32(mapid)] = &targetMap{mp.FD(), uint32(mapid), name, mp, info}
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

		ranges = constrainMaxRanges(estimateNumOfLockRanges(maps, cpus))
		l.ranges = ranges
		collectionSpec.Maps[mapAddrFdBpfMap].MaxEntries = ranges
		collectionSpec.Maps[lockStatBpfMap].MaxEntries = ranges
		collectionSpec.Maps[rangesBpfMap].MaxEntries = ranges

		// Ideally we would want this to be the max number of proccesses allowed
		// by the kernel, however verifier constraints force us to choose a smaller
		// value. This value has been experimentally verifier to pass the verifier.
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
			var ve *ebpf.VerifierError
			if errors.As(err, &ve) {
				return fmt.Errorf("verfier error loading collection: %s\n%+v", err, ve)
			}
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
		mapidPtr := unsafe.Pointer(&tm.id)
		_, _, _ = syscall.Syscall(syscall.SYS_IOCTL, uintptr(tm.fd), ioctlCollectLocksCmd, uintptr(mapidPtr))

		// close all dupped maps so we do not waste fds
		tm.mp.Close()
		tm.mp = nil
		tm.mpInfo = nil
	}

	var cursor ebpf.MapBatchCursor
	lockRanges := make([]LockRange, ranges)
	mapids := make([]uint32, ranges)
	count, err := l.objects.MapAddrFd.BatchLookup(&cursor, lockRanges, mapids, nil)
	if !errors.Is(err, ebpf.ErrKeyNotExist) {
		return fmt.Errorf("unable to lookup up lock ranges: %w", err)
	}

	if uint32(count) < ranges && !staticRanges {
		return fmt.Errorf("discovered fewer ranges than expected: %d < %d", count, ranges)
	}

	for i, id := range mapids {
		// id can be zero when staticRanges is set and tracked lock ranges are
		// less than maxTrackedRanges
		if id == 0 {
			continue
		}
		tm, ok := maps[id]
		if !ok {
			return fmt.Errorf("map with id %d not tracked", id)
		}
		l.trackedLockMemRanges[lockRanges[i]] = &mapStats{*tm, 0}
	}

	// sort lock ranges and write to per cpu array map
	// we sort so the bpf code can perform a quick binary search
	// over all the ranges to find if a lock address is tracked.
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

func lruMapLockRanges(cpu uint32) uint32 {
	// global freelist lock + (cpu * pcpu_free_lock)
	return uint32(cpu + 1)
}

func pcpuLruMapLockRanges(cpu uint32) uint32 {
	// cpu * freelist_lock
	return cpu
}

func ringbufMapLockRanges(_ uint32) uint32 {
	// waitq lock + rb lock
	return 2
}

func constrainMaxRanges(ranges uint32) uint32 {
	if ranges > maxTrackedRanges || staticRanges {
		return maxTrackedRanges
	}

	return ranges
}

func estimateNumOfLockRanges(tm map[uint32]*targetMap, cpu uint32) uint32 {
	var num uint32

	for _, m := range tm {
		t := m.mpInfo.Type

		if t == ebpf.Hash || t == ebpf.PerCPUHash || t == ebpf.LRUHash || t == ebpf.LRUCPUHash || t == ebpf.HashOfMaps {
			num += hashMapLockRanges(cpu)
		}
		if t == ebpf.LRUHash {
			num += lruMapLockRanges(cpu)
		}
		if t == ebpf.LRUCPUHash {
			num += pcpuLruMapLockRanges(cpu)
		}
		if t == ebpf.RingBuf {
			num += ringbufMapLockRanges(cpu)
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

func use_dummy_read(addrs map[string]uint64) bool {
	_, ok_fops := addrs["bpf_map_fops"]
	_, ok_read := addrs["bpf_dummy_read"]

	return !ok_fops && ok_read
}

func getKernelSymbolsAddressesWithKptrRestrict(kernelAddresses ...string) (map[string]uint64, error) {
	if err := setKptrRestrict(disableKptrRestrict); err != nil {
		return nil, fmt.Errorf("unable to disable kptr_restrict: %w", err)
	}

	addrs, err := GetKernelSymbolsAddresses(kernelAddresses...)
	if err != nil {
		// on debian 12 bpf_map_fops is not exported, so we use
		// bpf_dummy_read instead
		if dummy_read := use_dummy_read(addrs); !dummy_read {
			return nil, err
		}
	}

	if err := setKptrRestrict(enableKptrRestrict); err != nil {
		return nil, fmt.Errorf("unable to enable kptr_restrict: %w", err)
	}

	return addrs, nil
}
