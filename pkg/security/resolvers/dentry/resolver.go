// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package dentry holds dentry related files
package dentry

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"syscall"
	"unsafe"

	"github.com/DataDog/datadog-go/v5/statsd"
	lib "github.com/cilium/ebpf"
	lru "github.com/hashicorp/golang-lru/v2"
	"go.uber.org/atomic"
	"golang.org/x/sys/unix"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe/erpc"
	"github.com/DataDog/datadog-agent/pkg/security/probe/managerhelper"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

var (
	fakeInodeMSW = uint64(0xdeadc001)
)

type counterEntry struct {
	resolutionType string
	resolution     string
}

func (ce *counterEntry) Tags() []string {
	return []string{ce.resolutionType, ce.resolution}
}

// Resolver resolves inode/mountID to full paths
type Resolver struct {
	config                *config.Config
	statsdClient          statsd.ClientInterface
	dentries              *lib.Map
	erpcStats             [2]*lib.Map
	bufferSelector        *lib.Map
	activeERPCStatsBuffer uint32
	cache                 map[uint32]*lru.Cache[model.DentryKey, *cacheEntry]
	erpc                  *erpc.ERPC
	erpcSegment           []byte
	erpcSegmentSize       int
	useBPFProgWriteUser   bool
	erpcRequest           erpc.Request
	erpcStatsZero         []eRPCStats
	numCPU                int
	challenge             uint32

	hitsCounters map[counterEntry]*atomic.Int64
	missCounters map[counterEntry]*atomic.Int64
}

// ErrEntryNotFound is thrown when a path key was not found in the cache
var ErrEntryNotFound = errors.New("entry not found")

// cacheEntry holds data stored in the dentry cache
type cacheEntry struct {
	Parent model.DentryKey
}

// eRPCStats is used to collect kernel space metrics about the eRPC resolution
type eRPCStats struct {
	Count uint64
}

// eRPCRet is the type used to parse the eRPC return value
type eRPCRet uint32

func (ret eRPCRet) String() string {
	switch ret {
	case eRPCok:
		return "ok"
	case eRPCCacheMiss:
		return "cache_miss"
	case eRPCBufferSize:
		return "buffer_size"
	case eRPCWritePageFault:
		return "write_page_fault"
	case eRPCTailCallError:
		return "tail_call_error"
	case eRPCReadPageFault:
		return "read_page_fault"
	default:
		return "unknown"
	}
}

const (
	eRPCok eRPCRet = iota
	eRPCCacheMiss
	eRPCBufferSize
	eRPCWritePageFault
	eRPCTailCallError
	eRPCReadPageFault
	eRPCUnknownError
)

func allERPCRet() []eRPCRet {
	return []eRPCRet{eRPCok, eRPCCacheMiss, eRPCBufferSize, eRPCWritePageFault, eRPCTailCallError, eRPCReadPageFault, eRPCUnknownError}
}

// IsFakeInode returns whether the given inode is a fake inode
func IsFakeInode(inode uint64) bool {
	return inode>>32 == fakeInodeMSW
}

// SendStats sends the dentry resolver metrics
func (dr *Resolver) SendStats() error {
	for counterEntry, counter := range dr.hitsCounters {
		val := counter.Swap(0)
		if val > 0 {
			_ = dr.statsdClient.Count(metrics.MetricDentryResolverHits, val, counterEntry.Tags(), 1.0)
		}
	}

	for counterEntry, counter := range dr.missCounters {
		val := counter.Swap(0)
		if val > 0 {
			_ = dr.statsdClient.Count(metrics.MetricDentryResolverMiss, val, counterEntry.Tags(), 1.0)
		}
	}

	return dr.sendERPCStats()
}

func (dr *Resolver) sendERPCStats() error {
	buffer := dr.erpcStats[1-dr.activeERPCStatsBuffer]
	iterator := buffer.Iterate()
	stats := make([]eRPCStats, dr.numCPU)
	counters := map[eRPCRet]int64{}
	var ret eRPCRet

	for iterator.Next(&ret, &stats) {
		if ret == eRPCok {
			continue
		}
		for _, count := range stats {
			if _, ok := counters[ret]; !ok {
				counters[ret] = 0
			}
			counters[ret] += int64(count.Count)
		}
	}
	for r, count := range counters {
		if count > 0 {
			_ = dr.statsdClient.Count(metrics.MetricDentryERPC, count, []string{fmt.Sprintf("ret:%s", r)}, 1.0)
		}
	}
	for _, r := range allERPCRet() {
		_ = buffer.Put(r, dr.erpcStatsZero)
	}

	dr.activeERPCStatsBuffer = 1 - dr.activeERPCStatsBuffer
	return dr.bufferSelector.Put(ebpf.BufferSelectorERPCMonitorKey, dr.activeERPCStatsBuffer)
}

// DelCacheEntries removes all the entries belonging to a mountID
func (dr *Resolver) DelCacheEntries(mountID uint32) {
	delete(dr.cache, mountID)
}

func (dr *Resolver) lookupInodeFromCache(key model.DentryKey) (*cacheEntry, error) {
	entries, exists := dr.cache[key.MountID]
	if !exists {
		return &cacheEntry{}, ErrEntryNotFound
	}

	entry, exists := entries.Get(key)
	if !exists {
		return &cacheEntry{}, ErrEntryNotFound
	}

	return entry, nil
}

// We need to cache inode by inode instead of caching the whole path in order to be
// able to invalidate the whole path if one of its element got rename or removed.
func (dr *Resolver) cacheInode(key model.DentryKey, entry *cacheEntry) error {
	entries, exists := dr.cache[key.MountID]
	if !exists {
		var err error

		entries, err = lru.New[model.DentryKey, *cacheEntry](dr.config.DentryCacheSize)
		if err != nil {
			return err
		}
		dr.cache[key.MountID] = entries
	}

	entries.Add(key, entry)

	return nil
}

func (dr *Resolver) lookupInodeFromMap(pathKey model.DentryKey) (model.DentryLeaf, error) {
	var pathLeaf model.DentryLeaf
	if err := dr.dentries.Lookup(pathKey, &pathLeaf); err != nil {
		return pathLeaf, fmt.Errorf("unable to get filename for mountID `%d` and inode `%d`: %w", pathKey.MountID, pathKey.Inode, err)
	}
	return pathLeaf, nil
}

func (dr *Resolver) newcacheEntry(parent model.DentryKey) *cacheEntry {
	return &cacheEntry{
		Parent: parent,
	}
}

// preventSegmentMajorPageFault prepares the userspace memory area where the dentry resolver response is written. Used in kernel versions where BPF_F_MMAPABLE array maps are not yet available.
func (dr *Resolver) preventSegmentMajorPageFault() {
	// if we don't access the segment, the eBPF program can't write to it ... (major page fault)
	dr.erpcSegment[0] = 0
	dr.erpcSegment[os.Getpagesize()] = 0
	dr.erpcSegment[2*os.Getpagesize()] = 0
	dr.erpcSegment[3*os.Getpagesize()] = 0
	dr.erpcSegment[4*os.Getpagesize()] = 0
	dr.erpcSegment[5*os.Getpagesize()] = 0
	dr.erpcSegment[6*os.Getpagesize()] = 0
}

func (dr *Resolver) requestResolve(op uint8, pathKey model.DentryKey) (uint32, error) {
	challenge := dr.challenge
	dr.challenge++

	// create eRPC request
	dr.erpcRequest.OP = op
	model.ByteOrder.PutUint64(dr.erpcRequest.Data[0:8], pathKey.Inode)
	model.ByteOrder.PutUint32(dr.erpcRequest.Data[8:12], pathKey.MountID)
	model.ByteOrder.PutUint32(dr.erpcRequest.Data[12:16], pathKey.PathID)
	// 16-28 populated at start
	model.ByteOrder.PutUint32(dr.erpcRequest.Data[28:32], challenge)

	// if we don't try to access the segment, the eBPF program can't write to it ... (major page fault)
	if dr.useBPFProgWriteUser {
		dr.preventSegmentMajorPageFault()
	}

	return challenge, dr.erpc.Request(&dr.erpcRequest)
}

// ResolveParentFromCache resolves the parent
func (dr *Resolver) ResolveParentFromCache(pathKey model.DentryKey) (model.DentryKey, error) {
	entry := counterEntry{
		resolutionType: metrics.CacheTag,
		resolution:     metrics.ParentResolutionTag,
	}

	path, err := dr.lookupInodeFromCache(pathKey)
	if err != nil {
		dr.missCounters[entry].Inc()
		return model.DentryKey{}, ErrEntryNotFound
	}

	dr.hitsCounters[entry].Inc()
	return path.Parent, nil
}

// ResolveParentFromERPC resolves the parent
func (dr *Resolver) ResolveParentFromERPC(pathKey model.DentryKey) (model.DentryKey, error) {
	entry := counterEntry{
		resolutionType: metrics.ERPCTag,
		resolution:     metrics.ParentResolutionTag,
	}

	// create eRPC request
	challenge, err := dr.requestResolve(erpc.ResolveParentDentryOp, pathKey)
	if err != nil {
		dr.missCounters[entry].Inc()
		return model.DentryKey{}, fmt.Errorf("unable to resolve the parent of mountID `%d` and inode `%d` with eRPC: %w", pathKey.MountID, pathKey.Inode, err)
	}

	if challenge != model.ByteOrder.Uint32(dr.erpcSegment[12:16]) {
		dr.missCounters[entry].Inc()
		return model.DentryKey{}, errERPCRequestNotProcessed
	}

	pathKey.Inode = model.ByteOrder.Uint64(dr.erpcSegment[0:8])
	pathKey.MountID = model.ByteOrder.Uint32(dr.erpcSegment[8:12])

	dr.hitsCounters[entry].Inc()
	return pathKey, nil
}

// ResolveParentFromMap resolves the parent
func (dr *Resolver) ResolveParentFromMap(pathKey model.DentryKey) (model.DentryKey, error) {
	entry := counterEntry{
		resolutionType: metrics.KernelMapsTag,
		resolution:     metrics.ParentResolutionTag,
	}

	path, err := dr.lookupInodeFromMap(pathKey)
	if err != nil {
		dr.missCounters[entry].Inc()
		return model.DentryKey{}, err
	}

	dr.hitsCounters[entry].Inc()
	return path.Parent, nil
}

// GetParent returns the parent mount_id/inode
func (dr *Resolver) GetParent(pathKey model.DentryKey) (model.DentryKey, error) {
	parentKey, err := dr.ResolveParentFromCache(pathKey)
	if err != nil && dr.config.ERPCDentryResolutionEnabled {
		parentKey, err = dr.ResolveParentFromERPC(pathKey)
		if err == nil && parentKey.MountID != 0 && parentKey.Inode != 0 && !IsFakeInode(parentKey.Inode) {
			entry := dr.newcacheEntry(parentKey)
			_ = dr.cacheInode(pathKey, entry)
		}
	}

	if err != nil && dr.config.MapDentryResolutionEnabled {
		parentKey, err = dr.ResolveParentFromMap(pathKey)
		if err == nil && parentKey.MountID != 0 && parentKey.Inode != 0 && !IsFakeInode(parentKey.Inode) {
			entry := dr.newcacheEntry(parentKey)
			_ = dr.cacheInode(pathKey, entry)
		}
	}

	if parentKey.Inode == 0 {
		return model.DentryKey{}, ErrEntryNotFound
	}

	return parentKey, nil
}

// Start the dentry resolver
func (dr *Resolver) Start(manager *manager.Manager) error {
	dentries, err := managerhelper.Map(manager, "dentries")
	if err != nil {
		return err
	}
	dr.dentries = dentries

	erpcStatsFB, err := managerhelper.Map(manager, "dr_erpc_stats_fb")
	if err != nil {
		return err
	}
	dr.erpcStats[0] = erpcStatsFB

	erpcStatsBB, err := managerhelper.Map(manager, "dr_erpc_stats_bb")
	if err != nil {
		return err
	}
	dr.erpcStats[1] = erpcStatsBB

	bufferSelector, err := managerhelper.Map(manager, "buffer_selector")
	if err != nil {
		return err
	}
	dr.bufferSelector = bufferSelector

	erpcBuffer, err := managerhelper.Map(manager, "dr_erpc_buffer")
	if err != nil {
		return err
	}

	// Memory map a BPF_F_MMAPABLE array map that ebpf writes to so that userspace can read it
	if erpcBuffer.Flags()&unix.BPF_F_MMAPABLE != 0 {
		dr.erpcSegment, err = syscall.Mmap(erpcBuffer.FD(), 0, 8*4096, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
		if err != nil {
			return fmt.Errorf("failed to mmap dr_erpc_buffer map: %w", err)
		}
	}

	// BPF_F_MMAPABLE array maps were introduced in kernel version 5.5, so we need a fallback for older versions.
	// Allocate memory area in userspace that ebpf programs will write to. Will receive warning because the kernel writing to userspace memory can cause instability.
	if dr.erpcSegment == nil {
		// We need at least 7 memory pages for the eRPC segment method to work.
		// For each segment of a path, we write 16 bytes to store (inode, mount_id, path_id), and then at least 2 bytes to
		// store the smallest possible path (segment of size 1 + trailing 0). 18 * 1500 = 27 000.
		// Then, 27k + 256 / page_size < 7.
		dr.erpcSegment = make([]byte, 7*4096)
		dr.useBPFProgWriteUser = true

		model.ByteOrder.PutUint64(dr.erpcRequest.Data[16:24], uint64(uintptr(unsafe.Pointer(&dr.erpcSegment[0]))))
	}

	dr.erpcSegmentSize = len(dr.erpcSegment)
	model.ByteOrder.PutUint32(dr.erpcRequest.Data[24:28], uint32(dr.erpcSegmentSize))

	return nil
}

// Close cleans up the eRPC segment
func (dr *Resolver) Close() error {
	return fmt.Errorf("couldn't cleanup eRPC memory segment: %w", unix.Munmap(dr.erpcSegment))
}

// NewResolver returns a new dentry resolver
func NewResolver(config *config.Config, statsdClient statsd.ClientInterface, e *erpc.ERPC) (*Resolver, error) {
	hitsCounters := make(map[counterEntry]*atomic.Int64)
	missCounters := make(map[counterEntry]*atomic.Int64)
	for _, resolution := range metrics.AllResolutionsTags {
		for _, resolutionType := range metrics.AllTypesTags {
			// procfs resolution doesn't exist in the dentry resolver
			if resolutionType == metrics.ProcFSTag {
				continue
			}
			entry := counterEntry{
				resolutionType: resolutionType,
				resolution:     resolution,
			}
			hitsCounters[entry] = atomic.NewInt64(0)
			missCounters[entry] = atomic.NewInt64(0)
		}
	}

	numCPU, err := utils.NumCPU()
	if err != nil {
		return nil, fmt.Errorf("couldn't fetch the host CPU count: %w", err)
	}

	return &Resolver{
		config:        config,
		statsdClient:  statsdClient,
		cache:         make(map[uint32]*lru.Cache[model.DentryKey, *cacheEntry]),
		erpc:          e,
		erpcRequest:   erpc.Request{},
		erpcStatsZero: make([]eRPCStats, numCPU),
		hitsCounters:  hitsCounters,
		missCounters:  missCounters,
		numCPU:        numCPU,
		challenge:     rand.Uint32(),
	}, nil
}
