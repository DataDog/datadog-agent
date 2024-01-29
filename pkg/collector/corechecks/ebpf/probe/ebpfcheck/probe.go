// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package ebpfcheck is the system-probe side of the eBPF check
package ebpfcheck

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"syscall"
	"time"
	"unsafe"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
	"golang.org/x/exp/maps"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/ebpfcheck/model"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// 5.16 for verified instruction count (reported if available)
// 5.12 for recursion misses (reported if available)
// 5.8 required for kernel stats (optional)
// 5.5 for security_perf_event_open
// 5.0 required for /proc/kallsyms BTF program full names
// 4.15 required for map names from kernel
var minimumKernelVersion = kernel.VersionCode(5, 5, 0)

const maxMapsTracked = 20

// Probe is the eBPF side of the eBPF check
type Probe struct {
	statsFD               io.Closer
	coll                  *ebpf.Collection
	perfBufferMap         *ebpf.Map
	ringBufferMap         *ebpf.Map
	pidMap                *ebpf.Map
	links                 []link.Link
	mapBuffers            entryCountBuffers
	entryCountMaxRestarts int

	nrcpus uint32
}

// NewProbe creates a [Probe]
func NewProbe(cfg *ddebpf.Config) (*Probe, error) {
	kv, err := kernel.HostVersion()
	if err != nil {
		return nil, fmt.Errorf("kernel version: %s", err)
	}
	if kv < minimumKernelVersion {
		return nil, fmt.Errorf("minimum kernel version %s not met, read %s", minimumKernelVersion, kv)
	}

	var probe *Probe
	filename := "ebpf.o"
	if cfg.BPFDebug {
		filename = "ebpf-debug.o"
	}
	err = ddebpf.LoadCOREAsset(filename, func(buf bytecode.AssetReader, opts manager.Options) error {
		var err error
		probe, err = startEBPFCheck(buf, opts)
		return err
	})
	if err != nil {
		return nil, err
	}

	if ddconfig.SystemProbe.GetBool("ebpf_check.kernel_bpf_stats") {
		probe.statsFD, err = ebpf.EnableStats(unix.BPF_STATS_RUN_TIME)
		if err != nil {
			log.Warnf("kernel ebpf stats failed to enable, program runtime and run count will be unavailable: %s", err)
		}
	}

	probe.mapBuffers.keysBufferSizeLimit = uint32(ddconfig.SystemProbe.GetInt("ebpf_check.entry_count.max_keys_buffer_size_bytes"))
	probe.mapBuffers.valuesBufferSizeLimit = uint32(ddconfig.SystemProbe.GetInt("ebpf_check.entry_count.max_values_buffer_size_bytes"))
	probe.entryCountMaxRestarts = ddconfig.SystemProbe.GetInt("ebpf_check.entry_count.max_restarts")

	log.Debugf("successfully loaded ebpf check probe")
	return probe, nil
}

func startEBPFCheck(buf bytecode.AssetReader, opts manager.Options) (*Probe, error) {
	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, err
	}

	cpus, err := kernel.PossibleCPUs()
	if err != nil {
		return nil, fmt.Errorf("error getting possible cpus: %s", err)
	}
	nrcpus := uint32(cpus)

	collSpec, err := ebpf.LoadCollectionSpecFromReader(buf)
	if err != nil {
		return nil, fmt.Errorf("load collection spec: %s", err)
	}
	for _, ms := range collSpec.Maps {
		switch ms.Name {
		case "perf_buffer_fds", "ring_buffer_fds", "map_pids", "ring_buffers":
			ms.MaxEntries = maxMapsTracked
		case "perf_buffers", "perf_event_mmap":
			ms.MaxEntries = nrcpus * maxMapsTracked
		}
	}

	p := Probe{nrcpus: nrcpus}
	p.coll, err = ebpf.NewCollectionWithOptions(collSpec, opts.VerifierOptions)
	if err != nil {
		var ve *ebpf.VerifierError
		if errors.As(err, &ve) {
			return nil, fmt.Errorf("verifier error loading ebpf collection: %s\n%+v", err, ve)
		}
		return nil, fmt.Errorf("new ebpf collection: %s", err)
	}
	p.perfBufferMap = p.coll.Maps["perf_buffers"]
	p.ringBufferMap = p.coll.Maps["ring_buffers"]
	p.pidMap = p.coll.Maps["map_pids"]
	AddNameMappingsCollection(p.coll, "ebpf_check")

	if err := p.attach(collSpec); err != nil {
		return nil, err
	}
	return &p, nil
}

func (k *Probe) attach(collSpec *ebpf.CollectionSpec) (err error) {
	defer func() {
		// if anything fails, we need to close/detach everything
		if err != nil {
			k.Close()
		}
	}()

	for name, prog := range k.coll.Programs {
		spec := collSpec.Programs[name]
		switch prog.Type() {
		case ebpf.Kprobe:
			const kprobePrefix, kretprobePrefix = "kprobe/", "kretprobe/"
			if strings.HasPrefix(spec.SectionName, kprobePrefix) {
				attachPoint := spec.SectionName[len(kprobePrefix):]
				l, err := link.Kprobe(attachPoint, prog, &link.KprobeOptions{
					TraceFSPrefix: "ddebpfc",
				})
				if err != nil {
					return fmt.Errorf("link kprobe %s to %s: %s", spec.Name, attachPoint, err)
				}
				k.links = append(k.links, l)
			} else if strings.HasPrefix(spec.SectionName, kretprobePrefix) {
				attachPoint := spec.SectionName[len(kretprobePrefix):]
				l, err := link.Kretprobe(attachPoint, prog, &link.KprobeOptions{
					TraceFSPrefix: "ddebpfc",
				})
				if err != nil {
					return fmt.Errorf("link kretprobe %s to %s: %s", spec.Name, attachPoint, err)
				}
				k.links = append(k.links, l)
			} else {
				return fmt.Errorf("unknown section prefix: %s", spec.SectionName)
			}
		case ebpf.TracePoint:
			const tracepointPrefix = "tracepoint/"
			attachPoint := spec.SectionName[len(tracepointPrefix):]
			parts := strings.Split(attachPoint, "/")
			l, err := link.Tracepoint(parts[0], parts[1], prog, nil)
			if err != nil {
				return fmt.Errorf("link tracepoint %s to %s: %s", spec.Name, attachPoint, err)
			}
			k.links = append(k.links, l)
		default:
			return fmt.Errorf("unknown program %s type: %T", spec.Name, prog.Type())
		}
	}
	return nil
}

// Close releases all associated resources
func (k *Probe) Close() {
	RemoveNameMappingsCollection(k.coll)
	for _, l := range k.links {
		if err := l.Close(); err != nil {
			log.Warnf("error unlinking program: %s", err)
		}
	}
	k.coll.Close()
	if k.statsFD != nil {
		_ = k.statsFD.Close()
	}
}

// GetAndFlush gets the stats
func (k *Probe) GetAndFlush() (results model.EBPFStats) {
	if err := k.getMapStats(&results); err != nil {
		log.Debugf("error getting map stats: %s", err)
		return
	}
	if err := k.getProgramStats(&results); err != nil {
		log.Debugf("error getting program stats: %s", err)
		return
	}
	return
}

func (k *Probe) getProgramStats(stats *model.EBPFStats) error {
	var err error
	progid := ebpf.ProgramID(0)
	for progid, err = ebpf.ProgramGetNextID(progid); err == nil; progid, err = ebpf.ProgramGetNextID(progid) {
		fd, err := ProgGetFdByID(&ProgGetFdByIDAttr{ID: uint32(progid)})
		if err != nil {
			log.Debugf("error getting program fd prog_id=%d: %s", progid, err)
			continue
		}
		defer func() {
			err := syscall.Close(int(fd))
			if err != nil {
				log.Debugf("error closing fd %d: %s", fd, err)
			}
		}()

		var info ProgInfo
		if err := ProgObjInfo(fd, &info); err != nil {
			log.Debugf("error getting program info prog_id=%d: %s", progid, err)
			continue
		}

		mappingLock.RLock()
		name := unix.ByteSliceToString(info.Name[:])
		if pn, ok := progNameMapping[uint32(progid)]; ok {
			name = pn
		}
		// we require a name, so use program type for unnamed programs
		if name == "" {
			name = strings.ToLower(ebpf.ProgramType(info.Type).String())
		}
		module := "unknown"
		if mod, ok := progModuleMapping[uint32(progid)]; ok {
			module = mod
		}
		mappingLock.RUnlock()

		tag := hex.EncodeToString(info.Tag[:])
		ps := model.EBPFProgramStats{
			ID:              uint32(progid),
			Name:            name,
			Module:          module,
			Tag:             tag,
			Type:            ebpf.ProgramType(info.Type),
			XlatedProgLen:   info.XlatedProgLen,
			RSS:             uint64(roundUp(info.XlatedProgLen, uint32(pageSize))),
			VerifiedInsns:   info.VerifiedInsns,
			Runtime:         time.Duration(info.RunTimeNs),
			RunCount:        info.RunCnt,
			RecursionMisses: info.RecursionMisses,
		}
		stats.Programs = append(stats.Programs, ps)
	}

	log.Tracef("found %d programs", len(stats.Programs))
	deduplicateProgramNames(stats)
	for _, ps := range stats.Programs {
		log.Tracef("name=%s prog_id=%d type=%s", ps.Name, ps.ID, ps.Type.String())
	}

	return nil
}

func (k *Probe) getMapStats(stats *model.EBPFStats) error {
	var err error
	mapCount := 0
	ebpfmaps := make(map[string]*model.EBPFMapStats)
	defer maps.Clear(ebpfmaps)

	mapid := ebpf.MapID(0)
	for mapid, err = ebpf.MapGetNextID(mapid); err == nil; mapid, err = ebpf.MapGetNextID(mapid) {
		mp, err := ebpf.NewMapFromID(mapid)
		if err != nil {
			log.Debugf("unable to get map map_id=%d: %s", mapid, err)
			continue
		}
		mapCount++

		// TODO this call was already done by cilium/ebpf internally
		// we could maybe avoid the duplicate call by doing the id->fd->info chain ourselves
		info, err := mp.Info()
		if err != nil {
			log.Debugf("error getting map info map_id=%d: %s", mapid, err)
			continue
		}
		name := info.Name
		mappingLock.RLock()
		if mn, ok := mapNameMapping[uint32(mapid)]; ok {
			name = mn
		}
		if name == "" {
			name = info.Type.String()
		}
		module := "unknown"
		if mod, ok := mapModuleMapping[uint32(mapid)]; ok {
			module = mod
		}
		mappingLock.RUnlock()

		baseMapStats := model.EBPFMapStats{
			ID:         uint32(mapid),
			Name:       name,
			Module:     module,
			Type:       info.Type,
			MaxEntries: info.MaxEntries,
			Entries:    -1, // Indicates no entries were calculated
		}
		ebpfmaps[baseMapStats.Name] = &baseMapStats

		switch info.Type {
		case ebpf.PerfEventArray:
			err := perfBufferMemoryUsage(&baseMapStats, info, k)
			if err != nil {
				log.Debug(err.Error())
				continue
			}
		case ebpf.RingBuf:
			err := ringBufferMemoryUsage(&baseMapStats, info, k)
			if err != nil {
				log.Debug(err.Error())
				continue
			}
		case ebpf.Hash, ebpf.LRUHash, ebpf.PerCPUHash, ebpf.LRUCPUHash, ebpf.HashOfMaps:
			baseMapStats.MaxSize, baseMapStats.RSS = hashMapMemoryUsage(info, uint64(k.nrcpus))
			if module != "unknown" {
				// hashMapNumberOfEntries might allocate memory, so we only do it if we have a module name, as
				// unknown modules get discarded anyway (only RSS is used for total counts)
				baseMapStats.Entries = hashMapNumberOfEntries(mp, &k.mapBuffers, k.entryCountMaxRestarts)
			}
		case ebpf.Array, ebpf.PerCPUArray, ebpf.ProgramArray, ebpf.CGroupArray, ebpf.ArrayOfMaps:
			baseMapStats.MaxSize, baseMapStats.RSS = arrayMemoryUsage(info, uint64(k.nrcpus))
		case ebpf.LPMTrie:
			baseMapStats.MaxSize, baseMapStats.RSS = trieMemoryUsage(info, uint64(k.nrcpus))
		// TODO other map types
		//case ebpf.Stack:
		//case ebpf.ReusePortSockArray:
		//case ebpf.CPUMap:
		//case ebpf.DevMap, ebpf.DevMapHash:
		//case ebpf.Queue:
		//case ebpf.StructOpsMap:
		//case ebpf.CGroupStorage:
		//case ebpf.TaskStorage, ebpf.SkStorage, ebpf.InodeStorage:
		default:
			log.Debugf("unsupported map %s(%d) type %s", name, mapid, info.Type.String())
			continue
		}
		stats.Maps = append(stats.Maps, baseMapStats)
	}

	log.Tracef("found %d maps", mapCount)
	deduplicateMapNames(stats)
	for _, mp := range stats.Maps {
		log.Tracef("name=%s map_id=%d max=%d rss=%d type=%s", mp.Name, mp.ID, mp.MaxSize, mp.RSS, mp.Type)
	}
	// Allow the maps to be garbage collected
	k.mapBuffers.resetBuffers()

	return nil
}

const sizeofBpfArray = 320 // struct bpf_array

func arrayMemoryUsage(info *ebpf.MapInfo, nrCPUS uint64) (max uint64, rss uint64) {
	perCPU := isPerCPU(info.Type)
	numEntries := uint64(info.MaxEntries)
	elemSize := uint64(roundUpPow2(info.ValueSize, 8))

	usage := uint64(sizeofBpfArray)

	if perCPU {
		usage += numEntries * sizeOfPointer
		usage += numEntries * elemSize * nrCPUS
	} else {
		if info.Flags&unix.BPF_F_MMAPABLE > 0 {
			usage = pageAlign(usage)
			usage += pageAlign(numEntries * elemSize)
		} else {
			usage += numEntries * elemSize
		}
	}

	return usage, usage
}

const sizeofHtab = uint64(704)    // struct bpf_htab
const sizeofHtabElem = uint64(48) // struct htab_elem
const sizeOfBucket = uint64(16)   // struct bucket
const hashtabMapLockCount = 8
const sizeOfPointer = uint64(unsafe.Sizeof(uintptr(0)))
const sizeOfInt = uint64(unsafe.Sizeof(1))

func isPerCPU(typ ebpf.MapType) bool {
	switch typ {
	case ebpf.PerCPUHash, ebpf.PerCPUArray, ebpf.LRUCPUHash:
		return true
	}
	return false
}

func isLRU(typ ebpf.MapType) bool {
	switch typ {
	case ebpf.LRUHash, ebpf.LRUCPUHash:
		return true
	}
	return false
}

func hashMapMemoryUsage(info *ebpf.MapInfo, nrCPUS uint64) (max uint64, rss uint64) {
	valueSize := uint64(roundUpPow2(info.ValueSize, 8))
	keySize := uint64(roundUpPow2(info.KeySize, 8))
	perCPU := isPerCPU(info.Type)
	lru := isLRU(info.Type)
	//prealloc := (info.Flags & unix.BPF_F_NO_PREALLOC) == 0
	hasExtraElems := !perCPU && !lru

	nBuckets := uint64(roundUpNearestPow2(info.MaxEntries))
	usage := sizeofHtab
	usage += sizeOfBucket * nBuckets
	// could we get the size of the locks more directly with BTF?
	usage += sizeOfInt * nrCPUS * hashtabMapLockCount

	// TODO proper support of non-preallocated maps, will require coordination with eBPF to read size (if possible)
	//if prealloc {

	numEntries := uint64(info.MaxEntries)
	if hasExtraElems {
		numEntries += nrCPUS
	}

	elemSize := sizeofHtabElem + keySize
	if perCPU {
		elemSize += sizeOfPointer
	} else {
		elemSize += valueSize
	}
	usage += elemSize * numEntries

	if perCPU {
		usage += valueSize * nrCPUS * numEntries
	} else if !lru {
		usage += sizeOfPointer * nrCPUS
	}

	//
	//} else { // !prealloc
	//
	//}

	return usage, usage
}

const sizeofLPMTrieNode = 40          // struct lpm_trie_node
const offsetOfDataInBPFLPMTrieKey = 4 // offsetof(struct bpf_lpm_trie_key, data)

func trieMemoryUsage(info *ebpf.MapInfo, _ uint64) (max uint64, rss uint64) {
	dataSize := uint64(info.KeySize) - offsetOfDataInBPFLPMTrieKey
	elemSize := sizeofLPMTrieNode + dataSize + uint64(info.ValueSize)
	size := elemSize * uint64(info.MaxEntries)
	// accurate RSS would require knowing the number of entries in the trie
	return size, size
}

func perfBufferMemoryUsage(mapStats *model.EBPFMapStats, info *ebpf.MapInfo, k *Probe) error {
	mapStats.MaxSize, mapStats.RSS = arrayMemoryUsage(info, uint64(k.nrcpus))

	mapid, _ := info.ID()
	key := perfBufferKey{Id: uint32(mapid)}
	var region mmapRegion
	numCPUs := uint32(0)
	for i := uint32(0); i < k.nrcpus; i++ {
		key.Cpu = i
		if err := k.perfBufferMap.Lookup(unsafe.Pointer(&key), unsafe.Pointer(&region)); err != nil {
			if errors.Is(err, ebpf.ErrKeyNotExist) {
				// /sys/devices/system/cpu/possible can report way more CPUs than actually present
				// cilium/ebpf handles this when creating perf buffer by ignoring ENODEV
				// assume errors here are offline CPUs and keep trucking
				continue
			}
			return fmt.Errorf("error reading perf buffer fd map %s, mapid=%d cpu=%d: %s", info.Name, mapid, i, err)
		}
		log.Tracef("map_id=%d cpu=%d len=%d addr=%x", mapid, i, region.Len, region.Addr)
		mapStats.MaxSize += region.Len
		numCPUs++
	}

	log.Tracef("map_id=%d num_cpus=%d", mapid, numCPUs)
	mapStats.RSS = mapStats.MaxSize
	mapStats.NumCPUs = numCPUs
	return nil
}

const sizeofBpfRingBuf = 320
const sizeofPageStruct = 64
const ringbufPosPages = 2

var (
	// use BTF to get this offset?
	offsetConsumerInRingbuf = pageSize
	// (offsetof(struct bpf_ringbuf, consumer_pos) >> PAGE_SHIFT)
	ringbufPgOff = offsetConsumerInRingbuf >> pageShift
)

func ringBufferMemoryUsage(mapStats *model.EBPFMapStats, info *ebpf.MapInfo, k *Probe) error {
	mapStats.MaxSize = uint64(sizeofBpfRingBuf)
	numEntries := uint64(info.MaxEntries)
	numMetaPages := ringbufPgOff + ringbufPosPages
	numDataPages := numEntries >> pageShift
	mapStats.MaxSize += (uint64(numMetaPages) + 2*numDataPages) * sizeofPageStruct
	mapStats.RSS = mapStats.MaxSize

	mapid, _ := info.ID()
	var ringInfo ringMmap
	if err := k.ringBufferMap.Lookup(unsafe.Pointer(&mapid), unsafe.Pointer(&ringInfo)); err != nil {
		return fmt.Errorf("error reading ring buffer map %s, mapid=%d: %s", info.Name, mapid, err)
	}
	log.Tracef("map_id=%d data_len=%d data_addr=%x cons_len=%d cons_addr=%x", mapid, ringInfo.Data.Len, ringInfo.Data.Addr, ringInfo.Consumer.Len, ringInfo.Consumer.Addr)
	mapStats.MaxSize += ringInfo.Consumer.Len + ringInfo.Data.Len
	mapStats.RSS = mapStats.MaxSize
	return nil
}

// entryCountBuffers is a struct that contains buffers used to get the number of entries
// with the batch API. It is used to avoid allocating buffers for every map. This structure
// also keeps track of the biggest allocation performed, so that on repeated calls we always
// allocate the biggest map we have seen so far, reducing the number of allocations.
type entryCountBuffers struct {
	// Buffer to store the keys returned from the batch
	keys []byte

	// Buffer to store the values from the batch
	values []byte

	// A map that stores the hashes of the keys seen in the first batch of a batch lookup
	// This is only used when the buffer limits do not allow us to get all the entries in a single batch
	// and we need to iterate. In that case, we need to check against the keys we got in the first batch
	// to see if we got restarted.
	firstBatchKeys inplaceSet

	// Buffer for the cursor indicating the next key to get
	cursor []byte

	// Track the maximum size of each buffer type, to avoid multiple reallocations
	// each time we call the function. This way, after buffers are reset, we allocate
	// directly the maximum size we will need and save on allocations
	maxKeysSize   uint32
	maxValuesSize uint32
	maxCursorSize uint32

	// size limits, originating from configuration
	keysBufferSizeLimit   uint32
	valuesBufferSizeLimit uint32
}

// growBufferWithLimit creates or grows the given buffer with a configured limit.
// Returns the new buffer, the length allocated and the boolean indicating if the buffer
// was allocated the desired size
func growBufferWithLimit(buffer []byte, newSize uint32, limit uint32) ([]byte, uint32, bool) {
	if newSize <= uint32(len(buffer)) {
		return buffer, uint32(len(buffer)), true
	}
	if limit > 0 && newSize > limit && len(buffer) < int(limit) {
		return make([]byte, limit), limit, false
	}
	return make([]byte, newSize), newSize, true
}

func (e *entryCountBuffers) tryEnsureSizeForFullBatch(referenceMap *ebpf.Map) bool {
	maxSize := referenceMap.MaxEntries()
	var keysAllocatedAsNeeded, valuesAllocatedAsNeeded bool
	e.keys, e.maxKeysSize, keysAllocatedAsNeeded = growBufferWithLimit(e.keys, max(e.maxKeysSize, referenceMap.KeySize()*maxSize), e.keysBufferSizeLimit)
	e.values, e.maxValuesSize, valuesAllocatedAsNeeded = growBufferWithLimit(e.values, max(e.maxKeysSize, referenceMap.ValueSize()*maxSize), e.valuesBufferSizeLimit)

	e.ensureSizeCursor(referenceMap)

	return keysAllocatedAsNeeded || valuesAllocatedAsNeeded
}

func (e *entryCountBuffers) prepareFirstBatchKeys(referenceMap *ebpf.Map) {
	e.firstBatchKeys.keySize = int(referenceMap.KeySize())
	e.firstBatchKeys.prepare(len(e.keys) / int(referenceMap.KeySize()))
	// Maps grow automatically and do not shrink, so it does not make sense
	// to reallocate them if we already have a map. However, we do want to clear it
	// so that we don't keep old keys from previous iterations
	e.firstBatchKeys.clear()
}

func (e *entryCountBuffers) ensureSizeCursor(referenceMap *ebpf.Map) {
	e.cursor, e.maxCursorSize, _ = growBufferWithLimit(e.cursor, max(e.maxCursorSize, referenceMap.KeySize()), 0) // No limit with cursors, they are always small
}

// resetBuffers resets the buffers to nil, so that they can be garbage collected
func (e *entryCountBuffers) resetBuffers() {
	e.keys = nil
	e.firstBatchKeys.reset()
	e.values = nil
	e.cursor = nil
}

// inplaceSet is a set that stores the hashes of keys. It's only used for the specific case of this entry count, where
// we want to check if we got a restarted iteration.
//
// Context: when iterating eBPF maps, the kernel returns a cursor that indicates the next key to get. If the map is changed
// in between calls and that "next key" disappears, the kernel just starts the iteration from the beginning. This means that
// we could be infinitely restarting the iteration if the map is constantly changing. To avoid this, we keep track of the
// entries we have seen in the first batch lookup call. If for any subsequent batch we get a repeated key, then we know the
// kernel restarted the iteration.
//
// Now, considering that we want to reduce as much as possible the memory usage, and that we are dealing with keys of arbitrary
// sizes (only known at runtime), we cannot just use a map[[]byte]struct{} to store the keys. Solutions like using a string for the keys
// would work, but they require extra copies and allocations. So, instead, we store the hashes of the keys in a map and check against
// that later. This is not perfect as there is a chance of collision between hashes (0.003% for 131072 keys). However, saving memory
// is more important and this approach lets us have a set without any allocations or copies, everything is done in-place as much as possible.
//
// One thing to note is that, in the case of a hash collision, we would detect a restart (false positive). The problem is that the collision
// would always happen no matter how many restarts we have, which means that if we have two colliding keys in a map we would never
// be able to get the number of entries. To mitigate that, we add the number of restarts we have to the hash as a "seed", so that if
// we get restarted we will get a different hash and we should be able to get the number of entries.
type inplaceSet struct {
	set     map[uint32]struct{}
	keySize int
	seed    uint32
}

func (s *inplaceSet) reset() {
	s.set = nil
}

// prepare prepares the set to store the given number of entries. Maps in Go grow automatically and never shrink, so we don't need to reallocate
// anything. This just ensures that the map is initialized and ready to be used and with a size hint to reduce reallocations.
func (s *inplaceSet) prepare(hintNumEntries int) {
	if s.set == nil {
		s.set = make(map[uint32]struct{}, hintNumEntries)
	}
}

// clear deletes all entries in the set
func (s *inplaceSet) clear() {
	for k := range s.set {
		delete(s.set, k)
	}
}

// hash calculates the hash of the key at the given offset in a certain buffer
// This is a FNV hash function with 32 bits
func (s *inplaceSet) hash(data []byte, offset int) uint32 {
	hash := uint32(2166136261)
	prime32 := uint32(16777619)
	hash *= prime32
	hash ^= s.seed
	for i := 0; i < int(s.keySize); i++ {
		hash *= prime32
		hash ^= uint32(data[offset+i])
	}
	return hash
}

// load loads the keys from the given buffer into the set, clearing any old entry
func (s *inplaceSet) load(buffer []byte, entries int) {
	s.clear()
	for keyOffset := 0; keyOffset < entries*s.keySize; keyOffset += s.keySize {
		// To avoid allocations, we calculate hash in-place, without copying the slice
		s.set[s.hash(buffer, keyOffset)] = struct{}{}
	}
}

// containsAny checks if any of the keys in the given buffer is present in the set
func (s *inplaceSet) containsAny(buffer []byte, entries int) bool {
	for keyOffset := 0; keyOffset < entries*s.keySize; keyOffset += s.keySize {
		// To avoid allocations, we calculate hash in-place, without copying the slice
		if _, present := s.set[s.hash(buffer, keyOffset)]; present {
			return true
		}
	}
	return false
}

// hashMapNumberOfEntries gets the number of entries in the given map using the batch API.
// Batch lookups are used to improve the behavior when maps are constantly changing, reducing the chance
// that we get a deleted key forcing us to restart the iteration, getting stuck in an infinite loop or
// returning completely wrong counts.
// The function is a little bit complex because it needs to deal with arbitrary key sizes and partial batches
// to reduce the number of allocations. See the comments below for more details.
func hashMapNumberOfEntriesWithBatch(mp *ebpf.Map, buffers *entryCountBuffers, maxRestarts int) (int64, error) {
	// Here we duplicate a bit the code from cilium/ebpf to use the batch API
	// in our own way, because the way it's coded there it cannot be used with
	// key sizes that are only known at runtime.
	//
	// The BatchLookup function from cilium wants to receive buffers for the keys and
	// values in the form of slices, and it expects that the elements of the slice
	// are of the same size as the key and value size of the map. I haven't found a
	// way to do this with arbitrary key sizes only known at runtime (i.e., I can't do
	// a type [][mp.KeySize()]byte), and instead of defining one type per possible key size,
	// I just replicated the system call here. It requires redinifing the struct used in cilium to
	// pass arguments, but it's not a big amount of code.
	const BpfMapLookupBatchCommandCode = uint32(24)
	type MapLookupBatchAttr struct {
		InBatch   Pointer
		OutBatch  Pointer
		Keys      Pointer
		Values    Pointer
		Count     uint32
		MapFd     uint32
		ElemFlags uint64
		Flags     uint64
	}

	// Allocate the buffers we need, and check if we got enough for getting
	// all the entries in a single batch or if we reached the limit and we need to
	// iterate
	allocatedEnoughSpace := buffers.tryEnsureSizeForFullBatch(mp)
	if !allocatedEnoughSpace {
		buffers.prepareFirstBatchKeys(mp)
	}

	batchSize := min(mp.MaxEntries(), uint32(len(buffers.keys)))
	totalCount := int64(0)

	// To avoid inifinte loops, we limit the number of restarts and we
	for restarts, batchIndex := 0, uint32(0); restarts < maxRestarts && batchIndex*batchSize < mp.MaxEntries(); batchIndex++ {
		// Ensure that we get a different hash if we get restarted, so false positives caused by hash collisions can be mitigated
		buffers.firstBatchKeys.seed = uint32(restarts)

		// Prepare the arguments to the lookup call
		attr := MapLookupBatchAttr{
			MapFd:    uint32(mp.FD()),
			Values:   NewPointer(unsafe.Pointer(&buffers.values[0])),
			Keys:     NewPointer(unsafe.Pointer(&buffers.keys[0])),
			Count:    batchSize,
			OutBatch: NewPointer(unsafe.Pointer(&buffers.cursor[0])),
			InBatch:  NewPointer(nil), // nil means start at the beginning
		}
		if batchIndex == 0 {
			// First batch, start at the beginning
			attr.InBatch = NewPointer(nil)
		} else {
			// continue from where we left off
			attr.InBatch = NewPointer(unsafe.Pointer(&buffers.cursor[0]))
		}

		_, _, errno := unix.Syscall(unix.SYS_BPF, uintptr(BpfMapLookupBatchCommandCode), uintptr(unsafe.Pointer(&attr)), unsafe.Sizeof(attr))
		totalCount += int64(attr.Count)

		if errno == 0 && batchIndex == 0 {
			// We got a batch and it's the first one, and we didn't reach the end of the map, so we need to store the keys we got here
			// so that later on we can check against them to see if we got an iteration restart
			if !allocatedEnoughSpace { // A sanity check
				return -1, fmt.Errorf("Unexpected batch lookup result: we should have enough space to get the full map in one batch, but BatchLookup returned a partial result")
			}

			// Keep track the keys of the first batch so we can look them up later to see if we got restarted
			buffers.firstBatchKeys.load(buffers.keys, int(attr.Count))
		} else if batchIndex > 0 {
			// We got a batch and it's not the first one. Check against the keys received in the first batch
			// to see if we got an iteration restart
			if buffers.firstBatchKeys.containsAny(buffers.keys, int(attr.Count)) {
				// We got a restart, reset the counters and start from this batch as if were the first
				buffers.firstBatchKeys.load(buffers.keys, int(attr.Count))
				restarts++
				batchIndex = 0
				totalCount = 0
				continue
			}
		}

		if errno == unix.ENOENT {
			// We looked up all elements, count is valid, return it
			return totalCount, nil
		} else if errno != 0 {
			// Something happened, abort everything
			return -1, fmt.Errorf("error iterating map %s: %s", mp.String(), errno)
		}

	}

	return -1, fmt.Errorf("the iteration got restarted too many times for map %s (%d entries)", mp.String(), mp.MaxEntries())
}

func hashMapNumberOfEntriesWithIteration(mp *ebpf.Map, buffers *entryCountBuffers) (int64, error) {
	numElements := int64(0)
	maxEntries := int64(mp.MaxEntries())
	firstIter := true
	buffers.ensureSizeCursor(mp)

	for numElements <= maxEntries {
		var err error
		if firstIter {
			// Pass nil as the current key to signal that we start at the beginning of the map
			err = mp.NextKey(nil, unsafe.Pointer(&buffers.cursor[0]))
			firstIter = false
		} else {
			// Normal operation, get the next key to the one we have
			err = mp.NextKey(unsafe.Pointer(&buffers.cursor[0]), unsafe.Pointer(&buffers.cursor[0]))
		}

		if err != nil {
			if errors.Is(err, ebpf.ErrKeyNotExist) {
				// we reached the end of the map
				break
			}
			return -1, err
		}

		numElements++
	}

	if numElements > maxEntries {
		return -1, fmt.Errorf("map %s has more elements than its max entries (%d), not returning a count", mp.String(), mp.MaxEntries())
	}
	return numElements, nil
}

func hashMapNumberOfEntries(mp *ebpf.Map, buffers *entryCountBuffers, maxRestarts int) int64 {
	if isPerCPU(mp.Type()) {
		return -1
	}

	var numElements int64
	var err error
	if ddebpf.BatchAPISupported() && mp.Type() != ebpf.HashOfMaps { // HashOfMaps doesn't work with batch API
		numElements, err = hashMapNumberOfEntriesWithBatch(mp, buffers, maxRestarts)
	} else {
		numElements, err = hashMapNumberOfEntriesWithIteration(mp, buffers)
	}
	if err != nil {
		log.Debugf("error getting number of elements for map %s: %s", mp.String(), err)
		return -1
	}

	return numElements
}
