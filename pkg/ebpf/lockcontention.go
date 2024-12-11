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
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/DataDog/ebpf-manager/tracefs"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	// TrackAllEBPFResources decides if all system ebpf resources should be collected
	// or just system-probe resources
	TrackAllEBPFResources = true

	// ioctl trigger code
	ioctlCollectLocksCmd = 0x70c13

	// maximum lock ranges to track
	maxTrackedRanges = 16384

	// batch size when updating per cpu map storing lock ranges
	// this value is the chunks in which we add the ranges to the per-cpu map
	// the size of each entry is equal to `struct lock_range`, which is 20 bytes.
	// The expected upper bound for each batch is then
	// sizeof(struct lock_range) * updateBatchSize * ncpus
	// this does not strictly upper bound the memory since ncpus is uncontrolled
	// but in practise this should be a reasonable value
	updateBatchSize = 100
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

	// buffers used in Collect operation
	lockRanges []LockRange
	contention []ContentionData

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
	traceFSRoot, err := tracefs.Root()
	if err != nil {
		return false
	}

	if _, err := os.Stat(filepath.Join(traceFSRoot, "events/lock/contention_begin/id")); errors.Is(err, os.ErrNotExist) {
		return false
	}

	if _, err := os.Stat(filepath.Join(traceFSRoot, "events/lock/contention_end/id")); errors.Is(err, os.ErrNotExist) {
		return false
	}

	var platform, version string
	platform, err = kernel.Platform()
	if err != nil {
		return false
	}

	version, err = kernel.PlatformVersion()
	if err != nil {
		return false
	}

	// lock contention collector not supported on debian 12 arm64 because there is no easy way to get per cpu variable region
	if platform == "debian" && strings.HasPrefix(version, "12") && kernel.Arch() == "arm64" {
		return false
	}

	return true
}

// NewLockContentionCollector creates a prometheus.Collector for eBPF lock contention metrics
func NewLockContentionCollector() *LockContentionCollector {
	if !lockContentionCollectorSupported() {
		log.Infof("lock contention collector not supported")
		return nil
	}

	ContentionCollector = &LockContentionCollector{
		maxContention: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Subsystem: "ebpf__locks",
				Name:      "_max",
				Help:      "gauge tracking maximum time a tracked lock was contended for",
			},
			[]string{"resource_name", "lock_type", "module"},
		),
		avgContention: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Subsystem: "ebpf__locks",
				Name:      "_avg",
				Help:      "gauge tracking average time a tracked lock was contended for",
			},
			[]string{"resource_name", "lock_type", "module"},
		),
		totalContention: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: "ebpf__locks",
				Name:      "_total",
				Help:      "counter tracking total time a tracked lock was contended for",
			},
			[]string{"resource_name", "lock_type", "module"},
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

	// reset buffers
	l.lockRanges[0] = LockRange{}
	for i := 1; i < len(l.lockRanges); i *= 2 {
		copy(l.lockRanges[i:], l.lockRanges[:i])
	}

	l.contention[0] = ContentionData{}
	for i := 1; i < len(l.contention); i *= 2 {
		copy(l.contention[i:], l.contention[:i])
	}

	if _, err := l.objects.LockStats.BatchLookup(&cursor, l.lockRanges, l.contention, nil); !errors.Is(err, ebpf.ErrKeyNotExist) {
		log.Errorf("failed to perform batch lookup for lock stats: %v", err)
		return
	}

	for i, data := range l.contention {
		lr := l.lockRanges[i]
		if lr.Start == 0 {
			continue
		}
		mp, ok := l.trackedLockMemRanges[lr]
		if !ok {
			log.Errorf("found untracked lock range [0x%d, 0x%d+0x%d]", lr.Start, lr.Start, lr.Range)
			continue
		}

		module, err := GetModuleFromMapID(mp.id)
		if err != nil {
			module = "n/a"
		}

		if (data.Total_time > 0) && (mp.totalTime != data.Total_time) {
			avgTime := data.Total_time / uint64(data.Count)

			lockType := lockTypes[lr.Type]
			l.maxContention.WithLabelValues(mp.name, lockType, module).Set(float64(data.Max_time))
			l.avgContention.WithLabelValues(mp.name, lockType, module).Set(float64(avgTime))

			// TODO: should we consider overflows. u64 overflow seems very unlikely?
			l.totalContention.WithLabelValues(mp.name, lockType, module).Add(float64(data.Total_time - mp.totalTime))
			mp.totalTime = data.Total_time
		}
	}

	l.maxContention.Collect(metrics)
	l.avgContention.Collect(metrics)
	l.totalContention.Collect(metrics)
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

	kaddrs, err := getKernelSymbolsAddressesWithKallsymsIterator(kernelAddresses...)
	if err != nil {
		return fmt.Errorf("unable to fetch kernel symbol addresses: %w", err)
	}

	var ranges uint32
	var cpus uint32
	if err := LoadCOREAsset("lock_contention.o", func(bc bytecode.AssetReader, managerOptions manager.Options) error {
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
		collectionSpec.Maps["map_addr_fd"].MaxEntries = ranges
		collectionSpec.Maps["lock_stat"].MaxEntries = ranges
		collectionSpec.Maps["ranges"].MaxEntries = ranges

		// Ideally we would want this to be the max number of processes allowed
		// by the kernel, however verifier constraints force us to choose a smaller
		// value. This value has been experimentally determined to pass the verifier.
		collectionSpec.Maps["tstamp"].MaxEntries = 16384

		constants["num_cpus"] = uint64(cpus)
		for ksym, addr := range kaddrs {
			constants[ksym] = addr
		}
		constants["num_of_ranges"] = uint64(ranges)
		constants["log2_num_of_ranges"] = uint64(math.Log2(float64(ranges)))

		if err := collectionSpec.RewriteConstants(constants); err != nil {
			return fmt.Errorf("failed to write constant: %w", err)
		}

		opts := ebpf.CollectionOptions{
			Programs: ebpf.ProgramOptions{
				LogLevel:    ebpf.LogLevelBranch,
				KernelTypes: managerOptions.VerifierOptions.Programs.KernelTypes,
			},
		}

		if err := collectionSpec.LoadAndAssign(l.objects, &opts); err != nil {
			var ve *ebpf.VerifierError
			if errors.As(err, &ve) {
				return fmt.Errorf("verfier error loading collection: %s\n%+v", err, ve)
			}
			return fmt.Errorf("failed to load objects (%d ranges): %w", l.ranges, err)
		}

		return nil
	}); err != nil {
		return err
	}

	kp, err := link.Kprobe("do_vfs_ioctl", l.objects.KprobeVfsIoctl, nil)
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
		log.Warnf("discovered fewer ranges than expected: %d < %d", count, ranges)
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

	batchSize := uint32(updateBatchSize)
	if batchSize > ranges {
		batchSize = ranges
	}

	var iter uint32

	// this loop inserts the lock_ranges we have previously collected
	// into the per-cpu map `ranges`. We perform the update in batches
	// of size `batchSize`.
	for iter = 0; iter < (ranges/batchSize)+1; iter++ {
		keys := make([]uint32, batchSize)
		values := make([]LockRange, cpus*batchSize)

		var i, j uint32

		// this loop builds the `values` and `keys` slices for this batch
		for i = 0; i < batchSize; i++ {
			key := (iter * batchSize) + i
			if key >= ranges {
				break
			}

			keys[i] = key

			// Since `ranges` is a per-cpu map we need to duplicate each entry
			// for the number of CPUs on this system
			for j = 0; j < cpus; j++ {
				values[(j*batchSize)+i] = lockRanges[key]
			}
		}

		if _, err := l.objects.Ranges.BatchUpdate(keys, values, nil); err != nil {
			return fmt.Errorf("unable to perform batch update on per cpu array map: %w", err)
		}
	}

	// initialize buffers used in Collect
	l.lockRanges = make([]LockRange, l.ranges)
	l.contention = make([]ContentionData, l.ranges)

	log.Infof("lock contention collector initialized")
	l.initialized = true

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

type ksymIterProgram struct {
	BpfIteratorDumpKsyms *ebpf.Program `ebpf:"bpf_iter__dump_ksyms"`
}

func getKernelSymbolsAddressesWithKallsymsIterator(kernelAddresses ...string) (map[string]uint64, error) {
	var prog ksymIterProgram

	if err := LoadCOREAsset("ksyms_iter.o", func(bc bytecode.AssetReader, managerOptions manager.Options) error {
		collectionSpec, err := ebpf.LoadCollectionSpecFromReader(bc)
		if err != nil {
			return fmt.Errorf("failed to load collection spec: %w", err)
		}

		opts := ebpf.CollectionOptions{
			Programs: ebpf.ProgramOptions{
				LogLevel:    ebpf.LogLevelBranch,
				KernelTypes: managerOptions.VerifierOptions.Programs.KernelTypes,
			},
		}

		if err := collectionSpec.LoadAndAssign(&prog, &opts); err != nil {
			var ve *ebpf.VerifierError
			if errors.As(err, &ve) {
				return fmt.Errorf("verfier error loading collection: %s\n%+v", err, ve)
			}
			return fmt.Errorf("failed to load objects: %w", err)
		}

		return nil
	}); err != nil {
		return nil, err
	}

	iter, err := link.AttachIter(link.IterOptions{
		Program: prog.BpfIteratorDumpKsyms,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to attach bpf iterator: %w", err)
	}
	defer iter.Close()

	ksymsReader, err := iter.Open()
	if err != nil {
		return nil, err
	}
	defer ksymsReader.Close()

	addrs, err := GetKernelSymbolsAddressesNoCache(ksymsReader, kernelAddresses...)
	if err != nil {
		return nil, err
	}

	return addrs, nil
}
