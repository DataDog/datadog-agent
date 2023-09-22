// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package ebpfcheck is the system-probe side of the eBPF check
package ebpfcheck

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
	"unsafe"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/ebpfcheck/model"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TODO estimated minimum kernel version: 5.8/5.5
// 5.8 required for kernel stats (optional)
// 5.5 for security_perf_event_open
// 5.0 required for /proc/kallsyms BTF program full names
// 4.15 required for map names from kernel
// some detailed program statistics may require even higher kernel versions
var minimumKernelVersion = kernel.VersionCode(5, 5, 0)

const maxMapsTracked = 20

// Probe is the eBPF side of the eBPF check
type Probe struct {
	statsFD       io.Closer
	coll          *ebpf.Collection
	perfBufferMap *ebpf.Map
	ringBufferMap *ebpf.Map
	pidMap        *ebpf.Map
	links         []link.Link

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
	log.Debugf("loading %s", filename)
	err = ddebpf.LoadCOREAsset(cfg, filename, func(buf bytecode.AssetReader, opts manager.Options) error {
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
			const kProbePrefix, kretprobePrefix = "kprobe/", "kretprobe/"
			if strings.HasPrefix(spec.SectionName, kProbePrefix) {
				attachPoint := spec.SectionName[len(kProbePrefix):]
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
		pr, err := ebpf.NewProgramFromID(progid)
		if err != nil {
			log.Debugf("unable to get program %d: %s", progid, err)
			continue
		}
		var info ProgInfo
		if err := ProgObjInfo(uint32(pr.FD()), &info); err != nil {
			log.Debugf("error getting program info %d: %s", progid, err)
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

	log.Debugf("found %d programs", len(stats.Programs))
	deduplicateProgramNames(stats)
	for _, ps := range stats.Programs {
		log.Debugf("%s(%d) => %s", ps.Name, ps.ID, ps.Type.String())
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
			log.Debugf("unable to get map %d: %s", mapid, err)
			continue
		}
		mapCount++

		// TODO this call was already done by cilium/ebpf internally
		// we could maybe avoid the duplicate call by doing the id->fd->info chain ourselves
		info, err := mp.Info()
		if err != nil {
			log.Debugf("error getting map info %d: %s", mapid, err)
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
		}
		ebpfmaps[baseMapStats.Name] = &baseMapStats

		switch info.Type {
		case ebpf.PerfEventArray:
			mapStats := model.EBPFPerfBufferStats{EBPFMapStats: baseMapStats}
			err := perfBufferMemoryUsage(&mapStats, info, k)
			if err != nil {
				log.Debug(err.Error())
				continue
			}
			stats.PerfBuffers = append(stats.PerfBuffers, mapStats)
			continue
		case ebpf.RingBuf:
			err := ringBufferMemoryUsage(&baseMapStats, info, k)
			if err != nil {
				log.Debug(err.Error())
				continue
			}
		case ebpf.Hash, ebpf.LRUHash, ebpf.PerCPUHash, ebpf.LRUCPUHash, ebpf.HashOfMaps:
			baseMapStats.MaxSize, baseMapStats.RSS = hashMapMemoryUsage(info, uint64(k.nrcpus))
		case ebpf.Array, ebpf.PerCPUArray, ebpf.ProgramArray, ebpf.CGroupArray, ebpf.ArrayOfMaps:
			baseMapStats.MaxSize, baseMapStats.RSS = arrayMemoryUsage(info, uint64(k.nrcpus))
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

	log.Debugf("found %d maps", mapCount)
	deduplicateMapNames(stats)
	for _, mp := range stats.Maps {
		log.Debugf("%s(%d) => max=%d rss=%d", mp.Name, mp.ID, mp.MaxSize, mp.RSS)
	}
	for _, mp := range stats.PerfBuffers {
		log.Debugf("%s(%d) => max=%d rss=%d", mp.Name, mp.ID, mp.MaxSize, mp.RSS)
	}

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

func perfBufferMemoryUsage(mapStats *model.EBPFPerfBufferStats, info *ebpf.MapInfo, k *Probe) error {
	mapStats.MaxSize, mapStats.RSS = arrayMemoryUsage(info, uint64(k.nrcpus))

	mapid, _ := info.ID()
	key := perfBufferKey{Id: uint32(mapid)}
	var region mmapRegion
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
		log.Debugf("map_id=%d cpu=%d len=%d addr=%x", mapid, i, region.Len, region.Addr)
		mapStats.MaxSize += region.Len
		mapStats.CPUBuffers = append(mapStats.CPUBuffers, model.EBPFCPUPerfBufferStats{
			CPU: i,
			EBPFMmapStats: model.EBPFMmapStats{
				Size: region.Len,
				Addr: uintptr(region.Addr),
			},
		})
	}

	addrs := make([]uintptr, 0, len(mapStats.CPUBuffers))
	for _, b := range mapStats.CPUBuffers {
		addrs = append(addrs, b.Addr)
	}
	rssMap, err := k.getMmapRSS(uint32(mapid), addrs)
	if err != nil {
		log.Debugf("error getting mmap data id=%d: %s", mapid, err)
		// default RSS to MaxSize in case of error
		mapStats.RSS = mapStats.MaxSize
		for i := range mapStats.CPUBuffers {
			cpub := &mapStats.CPUBuffers[i]
			cpub.RSS = cpub.Size
		}
	} else {
		for i := range mapStats.CPUBuffers {
			cpub := &mapStats.CPUBuffers[i]
			if rss, ok := rssMap[cpub.Addr]; ok {
				cpub.RSS = rss
				mapStats.RSS += rss
				log.Debugf("perf buffer id=%d cpu=%d rss=%d", mapid, cpub.CPU, cpub.RSS)
			} else {
				log.Debugf("unable to find RSS data id=%d cpu=%d addr=%x", mapid, cpub.CPU, cpub.Addr)
			}
		}
	}
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
	log.Debugf("map_id=%d data_len=%d data_addr=%x cons_len=%d cons_addr=%x", mapid, ringInfo.Data.Len, ringInfo.Data.Addr, ringInfo.Consumer.Len, ringInfo.Consumer.Addr)
	mapStats.MaxSize += ringInfo.Consumer.Len + ringInfo.Data.Len

	addrs := []uintptr{uintptr(ringInfo.Consumer.Addr), uintptr(ringInfo.Data.Addr)}
	rss, err := k.getMmapRSS(uint32(mapid), addrs)
	if err != nil {
		log.Debugf("error getting mmap data id=%d: %s", mapid, err)
		// default RSS to MaxSize in case of error
		mapStats.RSS = mapStats.MaxSize
	} else {
		for _, size := range rss {
			mapStats.RSS += size
		}
	}
	return nil
}

func (k *Probe) getMmapRSS(mapid uint32, addrs []uintptr) (map[uintptr]uint64, error) {
	var pid uint32
	if err := k.pidMap.Lookup(unsafe.Pointer(&mapid), unsafe.Pointer(&pid)); err != nil {
		return nil, fmt.Errorf("pid map lookup: %s", err)
	}

	log.Debugf("map pid=%d id=%d", pid, mapid)
	return matchProcessRSS(int(pid), addrs)
}

func matchProcessRSS(pid int, addrs []uintptr) (map[uintptr]uint64, error) {
	smaps, err := os.Open(kernel.HostProc(strconv.Itoa(pid), "smaps"))
	if err != nil {
		return nil, fmt.Errorf("smaps open: %s", err)
	}
	defer smaps.Close()

	return matchRSS(smaps, addrs)
}

func matchRSS(smaps io.Reader, addrs []uintptr) (map[uintptr]uint64, error) {
	matchaddrs := slices.Clone(addrs)
	rss := make(map[uintptr]uint64, len(addrs))
	var matchAddr uintptr
	scanner := bufio.NewScanner(bufio.NewReader(smaps))
	for scanner.Scan() {
		key, val, found := bytes.Cut(bytes.TrimSpace(scanner.Bytes()), []byte{' '})
		if !found {
			continue
		}
		if !bytes.HasSuffix(key, []byte(":")) {
			if matchAddr != uintptr(0) {
				return nil, fmt.Errorf("no RSS for matching address %x", matchAddr)
			}
			s, _, ok := bytes.Cut(key, []byte("-"))
			if !ok {
				return nil, fmt.Errorf("invalid smaps address format: %s", string(key))
			}
			start, err := strconv.ParseUint(string(s), 16, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid smaps address format: %s", string(key))
			}
			for i, addr := range matchaddrs {
				if uintptr(start) == addr {
					matchAddr = addr
					deleteAtNoOrder(matchaddrs, i)
					break
				}
			}
		} else {
			// if we have already parsed Rss, just keep skipping
			if matchAddr == uintptr(0) {
				continue
			}
			if string(key) == "Rss:" {
				v := string(bytes.TrimSpace(bytes.TrimSuffix(val, []byte(" kB"))))
				t, err := strconv.ParseUint(v, 10, 64)
				if err != nil {
					return nil, fmt.Errorf("parse rss: %s", err)
				}
				rss[matchAddr] = t * 1024
				matchAddr = uintptr(0)
			}
		}
	}
	return rss, nil
}
