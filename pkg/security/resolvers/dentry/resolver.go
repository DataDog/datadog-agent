// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package dentry

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"syscall"
	"unsafe"

	"github.com/DataDog/datadog-go/v5/statsd"
	lib "github.com/cilium/ebpf"
	lru "github.com/hashicorp/golang-lru/v2"
	"go.uber.org/atomic"
	"golang.org/x/sys/unix"

	"strings"

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
	pathnames             *lib.Map
	erpcStats             [2]*lib.Map
	bufferSelector        *lib.Map
	activeERPCStatsBuffer uint32
	cache                 map[uint32]*lru.Cache[uint64, *PathEntry]
	cacheGeneration       *atomic.Uint64
	erpc                  *erpc.ERPC
	erpcSegment           []byte
	erpcSegmentSize       int
	useBPFProgWriteUser   bool
	erpcRequest           erpc.ERPCRequest
	erpcStatsZero         []eRPCStats
	numCPU                int

	hitsCounters map[counterEntry]*atomic.Int64
	missCounters map[counterEntry]*atomic.Int64

	pathEntryPool *sync.Pool
}

// ErrEntryNotFound is thrown when a path key was not found in the cache
var ErrEntryNotFound = errors.New("entry not found")

// PathEntry is the path structure saved in cache
type PathEntry struct {
	Parent     model.PathKey
	Name       string
	Generation uint64
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

// DelCacheEntry removes an entry from the cache
func (dr *Resolver) DelCacheEntry(mountID uint32, inode uint64) {
	if entries, exists := dr.cache[mountID]; exists {
		key := model.PathKey{Inode: inode}

		// Delete path recursively
		for {
			path, exists := entries.Get(key.Inode)
			if !exists {
				break
			}
			// this is also call the onEvict function of LRU thus releasing the entry from the pool
			entries.Remove(key.Inode)

			parent := path.Parent
			if parent.Inode == 0 {
				break
			}

			// Prepare next key
			key = parent
		}
	}
}

// DelCacheEntries removes all the entries belonging to a mountID
func (dr *Resolver) DelCacheEntries(mountID uint32) {
	delete(dr.cache, mountID)
}

func (dr *Resolver) lookupInodeFromCache(mountID uint32, inode uint64) (*PathEntry, error) {
	entries, exists := dr.cache[mountID]
	if !exists {
		return nil, ErrEntryNotFound
	}

	entry, exists := entries.Get(inode)
	if !exists {
		return nil, ErrEntryNotFound
	}

	if entry.Generation < dr.cacheGeneration.Load() {
		return nil, ErrEntryNotFound
	}

	return entry, nil
}

// We need to cache inode by inode instead of caching the whole path in order to be
// able to invalidate the whole path if one of its element got rename or removed.
func (dr *Resolver) cacheInode(key model.PathKey, path *PathEntry) error {
	entries, exists := dr.cache[key.MountID]
	if !exists {
		var err error

		entries, err = lru.NewWithEvict(dr.config.DentryCacheSize, func(_ uint64, value *PathEntry) {
			dr.pathEntryPool.Put(value)
		})
		if err != nil {
			return err
		}
		dr.cache[key.MountID] = entries
	}
	path.Generation = dr.cacheGeneration.Load()

	// release before in case of override
	if prev, exists := entries.Get(key.Inode); exists {
		dr.pathEntryPool.Put(prev)
	}

	entries.Add(key.Inode, path)

	return nil
}

// ResolveNameFromCache returns the name
func (dr *Resolver) ResolveNameFromCache(mountID uint32, inode uint64) (string, error) {
	entry := counterEntry{
		resolutionType: metrics.CacheTag,
		resolution:     metrics.SegmentResolutionTag,
	}

	path, err := dr.lookupInodeFromCache(mountID, inode)
	if err != nil {
		dr.missCounters[entry].Inc()
		return "", err
	}

	dr.hitsCounters[entry].Inc()
	return path.Name, nil
}

func (dr *Resolver) lookupInodeFromMap(mountID uint32, inode uint64, pathID uint32) (model.PathLeaf, error) {
	key := model.PathKey{MountID: mountID, Inode: inode, PathID: pathID}
	var pathLeaf model.PathLeaf
	if err := dr.pathnames.Lookup(key, &pathLeaf); err != nil {
		return pathLeaf, fmt.Errorf("unable to get filename for mountID `%d` and inode `%d`: %w", mountID, inode, err)
	}
	return pathLeaf, nil
}

func (dr *Resolver) getPathEntryFromPool(parent model.PathKey, name string) *PathEntry {
	entry := dr.pathEntryPool.Get().(*PathEntry)
	entry.Parent = parent
	entry.Name = name
	entry.Generation = dr.cacheGeneration.Load()

	return entry
}

// ResolveNameFromMap resolves the name of the provided inode
func (dr *Resolver) ResolveNameFromMap(mountID uint32, inode uint64, pathID uint32) (string, error) {
	entry := counterEntry{
		resolutionType: metrics.KernelMapsTag,
		resolution:     metrics.SegmentResolutionTag,
	}

	pathLeaf, err := dr.lookupInodeFromMap(mountID, inode, pathID)
	if err != nil {
		dr.missCounters[entry].Inc()
		return "", fmt.Errorf("unable to get filename for mountID `%d` and inode `%d`: %w", mountID, inode, err)
	}

	dr.hitsCounters[entry].Inc()

	name := pathLeaf.GetName()

	if !IsFakeInode(inode) {
		cacheKey := model.PathKey{MountID: mountID, Inode: inode}
		cacheEntry := dr.getPathEntryFromPool(pathLeaf.Parent, name)
		if err := dr.cacheInode(cacheKey, cacheEntry); err != nil {
			dr.pathEntryPool.Put(cacheEntry)
		}
	}

	return name, nil
}

// ResolveName resolves an inode/mount ID pair to a file basename
func (dr *Resolver) ResolveName(mountID uint32, inode uint64, pathID uint32) string {
	name, err := dr.ResolveNameFromCache(mountID, inode)
	if err != nil && dr.config.ERPCDentryResolutionEnabled {
		name, err = dr.ResolveNameFromERPC(mountID, inode, pathID)
	}
	if err != nil && dr.config.MapDentryResolutionEnabled {
		name, err = dr.ResolveNameFromMap(mountID, inode, pathID)
	}

	if err != nil {
		name = ""
	}
	return name
}

// ResolveFromCache resolves path from the cache
func (dr *Resolver) ResolveFromCache(mountID uint32, inode uint64) (string, error) {
	var path *PathEntry
	var err error
	depth := int64(0)
	key := model.PathKey{MountID: mountID, Inode: inode}
	filenameParts := make([]string, 0, 128)

	entry := counterEntry{
		resolutionType: metrics.CacheTag,
		resolution:     metrics.PathResolutionTag,
	}

	// Fetch path recursively
	for i := 0; i <= model.MaxPathDepth; i++ {
		path, err = dr.lookupInodeFromCache(key.MountID, key.Inode)
		if err != nil {
			dr.missCounters[entry].Inc()
			break
		}
		depth++

		// Don't append dentry name if this is the root dentry (i.d. name == '/')
		if len(path.Name) != 0 && path.Name[0] != '\x00' && path.Name[0] != '/' {
			filenameParts = append(filenameParts, path.Name)
		}

		if path.Parent.Inode == 0 {
			break
		}

		// Prepare next key
		key = path.Parent
	}

	if depth > 0 {
		dr.hitsCounters[entry].Add(depth)
	}

	return computeFilenameFromParts(filenameParts), err
}

func computeFilenameFromParts(parts []string) string {
	if len(parts) == 0 {
		return "/"
	}

	var builder strings.Builder

	// pre-allocation
	for _, part := range parts {
		builder.Grow(len(part) + 1)
	}

	// reverse iteration
	for i := 0; i < len(parts); i++ {
		j := len(parts) - 1 - i

		builder.WriteRune('/')
		builder.WriteString(parts[j])
	}
	return builder.String()
}

// ResolveFromMap resolves the path of the provided inode / mount id / path id
func (dr *Resolver) ResolveFromMap(mountID uint32, inode uint64, pathID uint32, cache bool) (string, error) {
	var cacheKey model.PathKey
	var cacheEntry *PathEntry
	var resolutionErr error
	var name string
	var pathLeaf model.PathLeaf
	key := model.PathKey{MountID: mountID, Inode: inode, PathID: pathID}

	keyBuffer, err := key.MarshalBinary()
	if err != nil {
		return "", err
	}

	depth := int64(0)

	var keys []model.PathKey
	var entries []*PathEntry

	filenameParts := make([]string, 0, 128)

	// Fetch path recursively
	for i := 0; i <= model.MaxPathDepth; i++ {
		key.Write(keyBuffer)
		if err := dr.pathnames.Lookup(keyBuffer, &pathLeaf); err != nil {
			filenameParts = nil
			resolutionErr = errDentryPathKeyNotFound
			break
		}
		depth++

		cacheKey = model.PathKey{MountID: key.MountID, Inode: key.Inode}

		if pathLeaf.Name[0] == '\x00' {
			if depth >= model.MaxPathDepth {
				resolutionErr = errTruncatedParents
			} else {
				resolutionErr = errKernelMapResolution
			}
			break
		}

		// Don't append dentry name if this is the root dentry (i.d. name == '/')
		if pathLeaf.Name[0] == '/' {
			name = "/"
		} else {
			name = model.NullTerminatedString(pathLeaf.Name[:])
			filenameParts = append(filenameParts, name)
		}

		// do not cache fake path keys in the case of rename events
		if !IsFakeInode(key.Inode) && cache {
			cacheEntry = dr.getPathEntryFromPool(pathLeaf.Parent, name)

			keys = append(keys, cacheKey)
			entries = append(entries, cacheEntry)
		}

		if pathLeaf.Parent.Inode == 0 {
			break
		}

		// Prepare next key
		key = pathLeaf.Parent
	}

	filename := computeFilenameFromParts(filenameParts)

	entry := counterEntry{
		resolutionType: metrics.KernelMapsTag,
		resolution:     metrics.PathResolutionTag,
	}

	if resolutionErr == nil {
		dr.cacheEntries(keys, entries)

		if depth > 0 {
			dr.hitsCounters[entry].Add(depth)
		}
	} else {
		// nothing inserted in cache, release everything
		for _, entry := range entries {
			dr.pathEntryPool.Put(entry)
		}

		dr.missCounters[entry].Inc()
	}

	return filename, resolutionErr
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

func (dr *Resolver) requestResolve(op uint8, mountID uint32, inode uint64, pathID uint32) (uint32, error) {
	// create eRPC request
	challenge := rand.Uint32()
	dr.erpcRequest.OP = op
	model.ByteOrder.PutUint64(dr.erpcRequest.Data[0:8], inode)
	model.ByteOrder.PutUint32(dr.erpcRequest.Data[8:12], mountID)
	model.ByteOrder.PutUint32(dr.erpcRequest.Data[12:16], pathID)
	model.ByteOrder.PutUint32(dr.erpcRequest.Data[28:32], challenge)

	// if we don't try to access the segment, the eBPF program can't write to it ... (major page fault)
	if dr.useBPFProgWriteUser {
		dr.preventSegmentMajorPageFault()
	}

	return challenge, dr.erpc.Request(&dr.erpcRequest)
}

// ResolveNameFromERPC resolves the name of the provided inode / mount id / path id
func (dr *Resolver) ResolveNameFromERPC(mountID uint32, inode uint64, pathID uint32) (string, error) {
	entry := counterEntry{
		resolutionType: metrics.ERPCTag,
		resolution:     metrics.SegmentResolutionTag,
	}

	challenge, err := dr.requestResolve(erpc.ResolveSegmentOp, mountID, inode, pathID)
	if err != nil {
		dr.missCounters[entry].Inc()
		return "", fmt.Errorf("unable to get the name of mountID `%d` and inode `%d` with eRPC: %w", mountID, inode, err)
	}

	if challenge != model.ByteOrder.Uint32(dr.erpcSegment[12:16]) {
		dr.missCounters[entry].Inc()
		return "", errERPCRequestNotProcessed
	}

	seg := model.NullTerminatedString(dr.erpcSegment[16:])
	if len(seg) == 0 || len(seg) > 0 && seg[0] == 0 {
		dr.missCounters[entry].Inc()
		return "", fmt.Errorf("couldn't resolve segment (len: %d)", len(seg))
	}

	dr.hitsCounters[entry].Inc()
	return seg, nil
}

func (dr *Resolver) cacheEntries(keys []model.PathKey, entries []*PathEntry) {
	var cacheEntry *PathEntry

	for i, k := range keys {
		if i >= len(entries) {
			break
		}

		cacheEntry = entries[i]
		if len(keys) > i+1 {
			cacheEntry.Parent = keys[i+1]
		}

		if err := dr.cacheInode(k, cacheEntry); err != nil {
			dr.pathEntryPool.Put(cacheEntry)
		}
	}
}

// ResolveFromERPC resolves the path of the provided inode / mount id / path id
func (dr *Resolver) ResolveFromERPC(mountID uint32, inode uint64, pathID uint32, cache bool) (string, error) {
	var segment string
	var resolutionErr error
	depth := int64(0)

	entry := counterEntry{
		resolutionType: metrics.ERPCTag,
		resolution:     metrics.PathResolutionTag,
	}

	// create eRPC request and send using the ioctl syscall
	challenge, err := dr.requestResolve(erpc.ResolvePathOp, mountID, inode, pathID)
	if err != nil {
		dr.missCounters[entry].Inc()
		return "", fmt.Errorf("unable to resolve the path of mountID `%d` and inode `%d` with eRPC: %w", mountID, inode, err)
	}

	var keys []model.PathKey
	var entries []*PathEntry

	filenameParts := make([]string, 0, 128)

	i := 0
	// make sure that we keep room for at least one pathID + character + \0 => (sizeof(pathID) + 1 = 17)
	for i < dr.erpcSegmentSize-17 {
		depth++

		// parse the path_key_t structure
		cacheKey := model.PathKey{
			Inode:   model.ByteOrder.Uint64(dr.erpcSegment[i : i+8]),
			MountID: model.ByteOrder.Uint32(dr.erpcSegment[i+8 : i+12]),
		}

		// check challenge
		if challenge != model.ByteOrder.Uint32(dr.erpcSegment[i+12:i+16]) {
			if depth >= model.MaxPathDepth {
				resolutionErr = errTruncatedParentsERPC
				break
			}
			dr.missCounters[entry].Inc()
			return "", errERPCRequestNotProcessed
		}

		// skip PathID
		i += 16

		if dr.erpcSegment[i] == 0 {
			if depth >= model.MaxPathDepth {
				resolutionErr = errTruncatedParentsERPC
			} else {
				resolutionErr = errERPCResolution
			}
			break
		}

		if dr.erpcSegment[i] != '/' {
			segment = model.NullTerminatedString(dr.erpcSegment[i:])
			filenameParts = append(filenameParts, segment)
			i += len(segment) + 1
		} else {
			break
		}

		if !IsFakeInode(cacheKey.Inode) && cache {
			keys = append(keys, cacheKey)

			entry := dr.getPathEntryFromPool(model.PathKey{}, segment)
			entries = append(entries, entry)
		}
	}

	if resolutionErr == nil {
		dr.cacheEntries(keys, entries)

		if depth > 0 {
			dr.hitsCounters[entry].Add(depth)
		}
	} else {
		dr.missCounters[entry].Inc()
	}

	return computeFilenameFromParts(filenameParts), resolutionErr
}

// Resolve the pathname of a dentry, starting at the pathnameKey in the pathnames table
func (dr *Resolver) Resolve(mountID uint32, inode uint64, pathID uint32, cache bool) (string, error) {
	var path string
	var err = ErrEntryNotFound

	if cache {
		path, err = dr.ResolveFromCache(mountID, inode)
	}
	if err != nil && dr.config.ERPCDentryResolutionEnabled {
		path, err = dr.ResolveFromERPC(mountID, inode, pathID, cache)
	}
	if err != nil && err != errTruncatedParentsERPC && dr.config.MapDentryResolutionEnabled {
		path, err = dr.ResolveFromMap(mountID, inode, pathID, cache)
	}
	return path, err
}

// ResolveParentFromCache resolves the parent
func (dr *Resolver) ResolveParentFromCache(mountID uint32, inode uint64) (uint32, uint64, error) {
	entry := counterEntry{
		resolutionType: metrics.CacheTag,
		resolution:     metrics.ParentResolutionTag,
	}

	path, err := dr.lookupInodeFromCache(mountID, inode)
	if err != nil {
		dr.missCounters[entry].Inc()
		return 0, 0, ErrEntryNotFound
	}

	dr.hitsCounters[entry].Inc()
	return path.Parent.MountID, path.Parent.Inode, nil
}

// ResolveParentFromERPC resolves the parent
func (dr *Resolver) ResolveParentFromERPC(mountID uint32, inode uint64, pathID uint32) (uint32, uint64, error) {
	entry := counterEntry{
		resolutionType: metrics.ERPCTag,
		resolution:     metrics.ParentResolutionTag,
	}

	// create eRPC request
	challenge, err := dr.requestResolve(erpc.ResolveParentOp, mountID, inode, pathID)
	if err != nil {
		dr.missCounters[entry].Inc()
		return 0, 0, fmt.Errorf("unable to resolve the parent of mountID `%d` and inode `%d` with eRPC: %w", mountID, inode, err)
	}

	if challenge != model.ByteOrder.Uint32(dr.erpcSegment[12:16]) {
		dr.missCounters[entry].Inc()
		return 0, 0, errERPCRequestNotProcessed
	}

	parentInode := model.ByteOrder.Uint64(dr.erpcSegment[0:8])
	parentMountID := model.ByteOrder.Uint32(dr.erpcSegment[8:12])

	dr.hitsCounters[entry].Inc()
	return parentMountID, parentInode, nil
}

// ResolveParentFromMap resolves the parent
func (dr *Resolver) ResolveParentFromMap(mountID uint32, inode uint64, pathID uint32) (uint32, uint64, error) {
	entry := counterEntry{
		resolutionType: metrics.KernelMapsTag,
		resolution:     metrics.ParentResolutionTag,
	}

	path, err := dr.lookupInodeFromMap(mountID, inode, pathID)
	if err != nil {
		dr.missCounters[entry].Inc()
		return 0, 0, err
	}

	dr.hitsCounters[entry].Inc()
	return path.Parent.MountID, path.Parent.Inode, nil
}

// GetParent returns the parent mount_id/inode
func (dr *Resolver) GetParent(mountID uint32, inode uint64, pathID uint32) (uint32, uint64, error) {
	parentMountID, parentInode, err := dr.ResolveParentFromCache(mountID, inode)
	if err != nil && dr.config.ERPCDentryResolutionEnabled {
		parentMountID, parentInode, err = dr.ResolveParentFromERPC(mountID, inode, pathID)
	}
	if err != nil && err != errTruncatedParentsERPC && dr.config.MapDentryResolutionEnabled {
		parentMountID, parentInode, err = dr.ResolveParentFromMap(mountID, inode, pathID)
	}

	if parentInode == 0 {
		return 0, 0, ErrEntryNotFound
	}

	return parentMountID, parentInode, err
}

// BumpCacheGenerations bumps the generations of all the mount points
func (dr *Resolver) BumpCacheGenerations() {
	dr.cacheGeneration.Inc()
}

// Start the dentry resolver
func (dr *Resolver) Start(manager *manager.Manager) error {
	pathnames, err := managerhelper.Map(manager, "pathnames")
	if err != nil {
		return err
	}
	dr.pathnames = pathnames

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

// ErrERPCRequestNotProcessed is used to notify that the eRPC request was not processed
type ErrERPCRequestNotProcessed struct{}

func (err ErrERPCRequestNotProcessed) Error() string {
	return "erpc_not_processed"
}

var errERPCRequestNotProcessed ErrERPCRequestNotProcessed

// ErrTruncatedParentsERPC is used to notify that some parents of the path are missing
type ErrTruncatedParentsERPC struct{}

func (err ErrTruncatedParentsERPC) Error() string {
	return "truncated_parents_erpc"
}

var errTruncatedParentsERPC ErrTruncatedParentsERPC

// ErrTruncatedParents is used to notify that some parents of the path are missing
type ErrTruncatedParents struct{}

func (err ErrTruncatedParents) Error() string {
	return "truncated_parents"
}

var errTruncatedParents ErrTruncatedParents

// ErrERPCResolution is used to notify that the eRPC resolution failed
type ErrERPCResolution struct{}

func (err ErrERPCResolution) Error() string {
	return "erpc_resolution"
}

var errERPCResolution ErrERPCResolution

// ErrKernelMapResolution is used to notify that the Kernel maps resolution failed
type ErrKernelMapResolution struct{}

func (err ErrKernelMapResolution) Error() string {
	return "map_resolution"
}

var errKernelMapResolution ErrKernelMapResolution

// ErrDentryPathKeyNotFound is used to notify that the request key is missing from the kernel maps
type ErrDentryPathKeyNotFound struct{}

func (err ErrDentryPathKeyNotFound) Error() string {
	return "dentry_path_key_not_found"
}

var errDentryPathKeyNotFound ErrDentryPathKeyNotFound

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

	pathEntryPool := &sync.Pool{}
	pathEntryPool.New = func() interface{} {
		return &PathEntry{}
	}

	return &Resolver{
		config:          config,
		statsdClient:    statsdClient,
		cache:           make(map[uint32]*lru.Cache[uint64, *PathEntry]),
		erpc:            e,
		erpcRequest:     erpc.ERPCRequest{},
		erpcStatsZero:   make([]eRPCStats, numCPU),
		hitsCounters:    hitsCounters,
		missCounters:    missCounters,
		cacheGeneration: atomic.NewUint64(0),
		numCPU:          numCPU,
		pathEntryPool:   pathEntryPool,
	}, nil
}
