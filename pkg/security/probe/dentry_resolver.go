// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probe

import (
	"C"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/DataDog/datadog-go/statsd"
	lib "github.com/DataDog/ebpf"
	lru "github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

var (
	errDentryPathKeyNotFound = errors.New("error: dentry path key not found")
	fakeInodeMSW             = uint64(0xdeadc001)
)

// DentryResolver resolves inode/mountID to full paths
type DentryResolver struct {
	client                *statsd.Client
	pathnames             *lib.Map
	erpcStats             [2]*lib.Map
	bufferSelector        *lib.Map
	activeERPCStatsBuffer uint32
	dentryCacheSize       int
	cache                 map[uint32]*lru.Cache
	cacheGenerations      map[uint32]int
	cacheGenerationsLock  sync.Mutex
	erpc                  *ERPC
	erpcSegment           []byte
	erpcSegmentSize       int
	erpcRequest           ERPCRequest
	erpcEnabled           bool
	erpcStatsZero         []eRPCStats
	numCPU                int
	mapEnabled            bool

	hitsCounters map[string]map[string]*int64
	missCounters map[string]map[string]*int64
}

// ErrInvalidKeyPath is returned when inode or mountid are not valid
type ErrInvalidKeyPath struct {
	Inode   uint64
	MountID uint32
}

func (e *ErrInvalidKeyPath) Error() string {
	return fmt.Sprintf("invalid inode/mountID couple: %d/%d", e.Inode, e.MountID)
}

// ErrEntryNotFound is thrown when a path key was not found in the cache
var ErrEntryNotFound = errors.New("entry not found")

// PathKey identifies an entry in the dentry cache
type PathKey struct {
	Inode   uint64
	MountID uint32
	PathID  uint32
}

func (p *PathKey) Write(buffer []byte) {
	model.ByteOrder.PutUint64(buffer[0:8], p.Inode)
	model.ByteOrder.PutUint32(buffer[8:12], p.MountID)
	model.ByteOrder.PutUint32(buffer[12:16], p.PathID)
}

// IsNull returns true if a key is invalid
func (p *PathKey) IsNull() bool {
	return p.Inode == 0 && p.MountID == 0
}

func (p *PathKey) String() string {
	return fmt.Sprintf("%x/%x", p.MountID, p.Inode)
}

// MarshalBinary returns the binary representation of a path key
func (p *PathKey) MarshalBinary() ([]byte, error) {
	if p.IsNull() {
		return nil, &ErrInvalidKeyPath{Inode: p.Inode, MountID: p.MountID}
	}

	buff := make([]byte, 16)
	p.Write(buff)

	return buff, nil
}

// PathLeaf is the go representation of the eBPF path_leaf_t structure
type PathLeaf struct {
	Parent PathKey
	Name   [model.MaxSegmentLength + 1]byte
	Len    uint16
}

// PathEntry is the path structure saved in cache
type PathEntry struct {
	Parent     PathKey
	Name       string
	Generation int
}

// GetName returns the path value as a string
func (pv *PathLeaf) GetName() string {
	return C.GoString((*C.char)(unsafe.Pointer(&pv.Name)))
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
	case eRPCMajorPageFault:
		return "major_page_fault"
	case eRPCTailCallError:
		return "tail_call_error"
	default:
		return "unknown"
	}
}

const (
	eRPCok eRPCRet = iota
	eRPCCacheMiss
	eRPCBufferSize
	eRPCMajorPageFault
	eRPCTailCallError
	eRPCUnknownError
)

func allERPCRet() []eRPCRet {
	return []eRPCRet{eRPCok, eRPCCacheMiss, eRPCBufferSize, eRPCMajorPageFault, eRPCTailCallError, eRPCUnknownError}
}

// SendStats sends the dentry resolver metrics
func (dr *DentryResolver) SendStats() error {
	for resolution, hitsCounters := range dr.hitsCounters {
		for resolutionType, value := range hitsCounters {
			val := atomic.SwapInt64(value, 0)
			if val > 0 {
				_ = dr.client.Count(metrics.MetricDentryResolverHits, val, []string{resolutionType, resolution}, 1.0)
			}
		}
	}

	for resolution, hitsCounters := range dr.missCounters {
		for resolutionType, value := range hitsCounters {
			val := atomic.SwapInt64(value, 0)
			if val > 0 {
				_ = dr.client.Count(metrics.MetricDentryResolverMiss, val, []string{resolutionType, resolution}, 1.0)
			}
		}
	}

	return dr.sendERPCStats()
}

func (dr *DentryResolver) sendERPCStats() error {
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
			_ = dr.client.Count(metrics.MetricDentryERPC, count, []string{fmt.Sprintf("ret:%s", r)}, 1.0)
		}
	}
	for _, r := range allERPCRet() {
		_ = buffer.Put(r, dr.erpcStatsZero)
	}

	dr.activeERPCStatsBuffer = 1 - dr.activeERPCStatsBuffer
	return dr.bufferSelector.Put(ebpf.BufferSelectorERPCMonitorKey, dr.activeERPCStatsBuffer)
}

// DelCacheEntry removes an entry from the cache
func (dr *DentryResolver) DelCacheEntry(mountID uint32, inode uint64) {
	if entries, exists := dr.cache[mountID]; exists {
		key := PathKey{Inode: inode}

		// Delete path recursively
		for {
			path, exists := entries.Get(key.Inode)
			if !exists {
				break
			}
			entries.Remove(key.Inode)

			parent := path.(PathEntry).Parent
			if parent.Inode == 0 {
				break
			}

			// Prepare next key
			key = parent
		}
	}
}

// DelCacheEntries removes all the entries belonging to a mountID
func (dr *DentryResolver) DelCacheEntries(mountID uint32) {
	delete(dr.cache, mountID)
}

func (dr *DentryResolver) lookupInodeFromCache(mountID uint32, inode uint64) (*PathEntry, error) {
	entries, exists := dr.cache[mountID]
	if !exists {
		return nil, ErrEntryNotFound
	}

	entry, exists := entries.Get(inode)
	if !exists {
		return nil, ErrEntryNotFound
	}

	cacheEntry := entry.(PathEntry)
	dr.cacheGenerationsLock.Lock()
	defer dr.cacheGenerationsLock.Unlock()
	if cacheEntry.Generation < dr.cacheGenerations[mountID] {
		return nil, ErrEntryNotFound
	}

	return &cacheEntry, nil
}

func (dr *DentryResolver) cacheInode(key PathKey, path PathEntry) error {
	dr.cacheGenerationsLock.Lock()
	defer dr.cacheGenerationsLock.Unlock()

	entries, exists := dr.cache[key.MountID]
	if !exists {
		var err error

		entries, err = lru.New(dr.dentryCacheSize)
		if err != nil {
			return err
		}
		dr.cache[key.MountID] = entries
		dr.cacheGenerations[key.MountID] = 0
		path.Generation = 0
	} else {
		// lookup mount_id generation
		path.Generation = dr.cacheGenerations[key.MountID]
	}

	entries.Add(key.Inode, path)

	return nil
}

func (dr *DentryResolver) getNameFromCache(mountID uint32, inode uint64) (string, error) {
	path, err := dr.lookupInodeFromCache(mountID, inode)
	if err != nil {
		atomic.AddInt64(dr.missCounters[metrics.SegmentResolutionTag][metrics.CacheTag], 1)
		return "", err
	}

	atomic.AddInt64(dr.hitsCounters[metrics.SegmentResolutionTag][metrics.CacheTag], 1)
	return path.Name, nil
}

func (dr *DentryResolver) lookupInodeFromMap(mountID uint32, inode uint64, pathID uint32) (PathLeaf, error) {
	key := PathKey{MountID: mountID, Inode: inode, PathID: pathID}
	var pathLeaf PathLeaf
	if err := dr.pathnames.Lookup(key, &pathLeaf); err != nil {
		return pathLeaf, errors.Wrapf(err, "unable to get filename for mountID `%d` and inode `%d`", mountID, inode)
	}
	return pathLeaf, nil
}

// GetNameFromMap resolves the name of the provided inode
func (dr *DentryResolver) GetNameFromMap(mountID uint32, inode uint64, pathID uint32) (string, error) {
	pathLeaf, err := dr.lookupInodeFromMap(mountID, inode, pathID)
	if err != nil {
		atomic.AddInt64(dr.missCounters[metrics.SegmentResolutionTag][metrics.KernelMapsTag], 1)
		return "", errors.Wrapf(err, "unable to get filename for mountID `%d` and inode `%d`", mountID, inode)
	}

	atomic.AddInt64(dr.hitsCounters[metrics.SegmentResolutionTag][metrics.KernelMapsTag], 1)

	cacheKey := PathKey{MountID: mountID, Inode: inode}
	cacheEntry := PathEntry{Parent: pathLeaf.Parent, Name: pathLeaf.GetName()}
	_ = dr.cacheInode(cacheKey, cacheEntry)
	return cacheEntry.Name, nil
}

// GetName resolves a couple of mountID/inode to a path
func (dr *DentryResolver) GetName(mountID uint32, inode uint64, pathID uint32) string {
	name, err := dr.getNameFromCache(mountID, inode)
	if err != nil && dr.erpcEnabled {
		name, err = dr.GetNameFromERPC(mountID, inode, pathID)
	}
	if err != nil && dr.mapEnabled {
		name, err = dr.GetNameFromMap(mountID, inode, pathID)
	}

	if err != nil {
		name = ""
	}
	return name
}

// ResolveFromCache resolves path from the cache
func (dr *DentryResolver) ResolveFromCache(mountID uint32, inode uint64) (string, error) {
	var path *PathEntry
	var filename string
	var err error
	depth := int64(0)
	key := PathKey{MountID: mountID, Inode: inode}

	// Fetch path recursively
	for i := 0; i <= model.MaxPathDepth; i++ {
		path, err = dr.lookupInodeFromCache(key.MountID, key.Inode)
		if err != nil {
			atomic.AddInt64(dr.missCounters[metrics.PathResolutionTag][metrics.CacheTag], 1)
			break
		}
		depth++

		// Don't append dentry name if this is the root dentry (i.d. name == '/')
		if path.Name[0] != '\x00' && path.Name[0] != '/' {
			filename = "/" + path.Name + filename
		}

		if path.Parent.Inode == 0 {
			if len(filename) == 0 {
				filename = "/"
			}
			break
		}

		// Prepare next key
		key = path.Parent
	}

	if depth > 0 {
		atomic.AddInt64(dr.hitsCounters[metrics.PathResolutionTag][metrics.CacheTag], depth)
	}

	return filename, err
}

// ResolveFromMap resolves the path of the provided inode / mount id / path id
func (dr *DentryResolver) ResolveFromMap(mountID uint32, inode uint64, pathID uint32) (string, error) {
	var cacheKey PathKey
	var cacheEntry PathEntry
	var err, resolutionErr error
	var filename string
	var path PathLeaf
	key := PathKey{MountID: mountID, Inode: inode, PathID: pathID}

	keyBuffer, err := key.MarshalBinary()
	if err != nil {
		return "", err
	}

	depth := int64(0)
	toAdd := make(map[PathKey]PathEntry)

	// Fetch path recursively
	for i := 0; i <= model.MaxPathDepth; i++ {
		key.Write(keyBuffer)
		if err = dr.pathnames.Lookup(keyBuffer, &path); err != nil {
			filename = ""
			err = errDentryPathKeyNotFound
			atomic.AddInt64(dr.missCounters[metrics.PathResolutionTag][metrics.KernelMapsTag], 1)
			break
		}
		depth++

		cacheKey = PathKey{MountID: key.MountID, Inode: key.Inode}
		cacheEntry = PathEntry{Parent: path.Parent, Name: ""}

		if path.Name[0] == '\x00' {
			resolutionErr = errTruncatedParents
			break
		}

		// Don't append dentry name if this is the root dentry (i.d. name == '/')
		if path.Name[0] == '/' {
			cacheEntry.Name = "/"
		} else {
			cacheEntry.Name = C.GoString((*C.char)(unsafe.Pointer(&path.Name)))
			filename = "/" + cacheEntry.Name + filename
		}

		toAdd[cacheKey] = cacheEntry

		if path.Parent.Inode == 0 {
			break
		}

		// Prepare next key
		key = path.Parent
	}

	if depth > 0 {
		atomic.AddInt64(dr.hitsCounters[metrics.PathResolutionTag][metrics.KernelMapsTag], depth)
	}

	// resolution errors are more important than regular map lookup errors
	if resolutionErr != nil {
		err = resolutionErr
	}

	if len(filename) == 0 {
		filename = "/"
	}

	if err == nil {
		for k, v := range toAdd {
			// do not cache fake path keys in the case of rename events
			if k.Inode>>32 != fakeInodeMSW {
				_ = dr.cacheInode(k, v)
			}
		}
	}

	return filename, err
}

func (dr *DentryResolver) preventSegmentMajorPageFault() {
	// if we don't access the segment, the eBPF program can't write to it ... (major page fault)
	dr.erpcSegment[0] = 0
	dr.erpcSegment[os.Getpagesize()] = 0
	dr.erpcSegment[2*os.Getpagesize()] = 0
	dr.erpcSegment[3*os.Getpagesize()] = 0
	dr.erpcSegment[4*os.Getpagesize()] = 0
	dr.erpcSegment[5*os.Getpagesize()] = 0
	dr.erpcSegment[6*os.Getpagesize()] = 0
}

func (dr *DentryResolver) markSegmentAsZero() {
	model.ByteOrder.PutUint64(dr.erpcSegment[0:8], 0)
}

// GetNameFromERPC resolves the name of the provided inode / mount id / path id
func (dr *DentryResolver) GetNameFromERPC(mountID uint32, inode uint64, pathID uint32) (string, error) {
	// create eRPC request
	dr.erpcRequest.OP = ResolveSegmentOp
	model.ByteOrder.PutUint64(dr.erpcRequest.Data[0:8], inode)
	model.ByteOrder.PutUint32(dr.erpcRequest.Data[8:12], mountID)
	model.ByteOrder.PutUint32(dr.erpcRequest.Data[12:16], pathID)
	model.ByteOrder.PutUint64(dr.erpcRequest.Data[16:24], uint64(uintptr(unsafe.Pointer(&dr.erpcSegment[0]))))
	model.ByteOrder.PutUint32(dr.erpcRequest.Data[24:28], uint32(dr.erpcSegmentSize))

	// if we don't try to access the segment, the eBPF program can't write to it ... (major page fault)
	dr.preventSegmentMajorPageFault()

	// ensure to zero the segments in order check the in-kernel execution
	dr.markSegmentAsZero()

	if err := dr.erpc.Request(&dr.erpcRequest); err != nil {
		atomic.AddInt64(dr.missCounters[metrics.SegmentResolutionTag][metrics.ERPCTag], 1)
		return "", errors.Wrapf(err, "unable to get filename for mountID `%d` and inode `%d` with eRPC", mountID, inode)
	}

	if model.ByteOrder.Uint64(dr.erpcSegment[0:8]) == 0 {
		atomic.AddInt64(dr.missCounters[metrics.SegmentResolutionTag][metrics.ERPCTag], 1)
		return "", errors.Errorf("eRPC request wasn't processed")
	}

	seg := C.GoString((*C.char)(unsafe.Pointer(&dr.erpcSegment[16])))
	if len(seg) == 0 || len(seg) > 0 && seg[0] == 0 {
		atomic.AddInt64(dr.missCounters[metrics.SegmentResolutionTag][metrics.ERPCTag], 1)
		return "", errors.Errorf("couldn't resolve segment (len: %d)", len(seg))
	}

	atomic.AddInt64(dr.hitsCounters[metrics.SegmentResolutionTag][metrics.ERPCTag], 1)
	return seg, nil
}

// ResolveFromERPC resolves the path of the provided inode / mount id / path id
func (dr *DentryResolver) ResolveFromERPC(mountID uint32, inode uint64, pathID uint32) (string, error) {
	var filename, segment string
	var err, resolutionErr error
	var cacheKey PathKey
	var cacheEntry PathEntry
	depth := int64(0)

	// create eRPC request
	dr.erpcRequest.OP = ResolvePathOp
	model.ByteOrder.PutUint64(dr.erpcRequest.Data[0:8], inode)
	model.ByteOrder.PutUint32(dr.erpcRequest.Data[8:12], mountID)
	model.ByteOrder.PutUint32(dr.erpcRequest.Data[12:16], pathID)
	model.ByteOrder.PutUint64(dr.erpcRequest.Data[16:24], uint64(uintptr(unsafe.Pointer(&dr.erpcSegment[0]))))
	model.ByteOrder.PutUint32(dr.erpcRequest.Data[24:28], uint32(dr.erpcSegmentSize))

	// if we don't try to access the segment, the eBPF program can't write to it ... (major page fault)
	dr.preventSegmentMajorPageFault()

	// ensure to zero the segments in order check the in-kernel execution
	dr.markSegmentAsZero()

	if err = dr.erpc.Request(&dr.erpcRequest); err != nil {
		atomic.AddInt64(dr.missCounters[metrics.PathResolutionTag][metrics.ERPCTag], 1)
		return "", errors.Wrapf(err, "unable to get filename for mountID `%d` and inode `%d` with eRPC", mountID, inode)
	}

	var keys []PathKey
	var entries []PathEntry

	if model.ByteOrder.Uint64(dr.erpcSegment[0:8]) == 0 {
		atomic.AddInt64(dr.missCounters[metrics.PathResolutionTag][metrics.ERPCTag], 1)
		return "", errors.Errorf("eRPC request wasn't processed")
	}

	i := 0
	// make sure that we keep room for at least one pathID + character + \0 => (sizeof(pathID) + 1 = 17)
	for i < dr.erpcSegmentSize-17 {
		depth++

		// parse the path_key_t structure
		cacheKey.Inode = model.ByteOrder.Uint64(dr.erpcSegment[i : i+8])
		cacheKey.MountID = model.ByteOrder.Uint32(dr.erpcSegment[i+8 : i+12])
		// skip PathID
		i += 16

		if dr.erpcSegment[i] == 0 {
			if depth >= model.MaxPathDepth {
				resolutionErr = errTruncatedParents
			} else {
				resolutionErr = errERPCResolution
			}
			break
		}

		if dr.erpcSegment[i] != '/' {
			segment = C.GoString((*C.char)(unsafe.Pointer(&dr.erpcSegment[i])))
			filename = "/" + segment + filename
			i += len(segment) + 1
		} else {
			break
		}

		keys = append(keys, cacheKey)
		entries = append(entries, PathEntry{Name: segment})
	}

	if len(filename) == 0 {
		filename = "/"
	}

	if resolutionErr == nil {
		for i, k := range keys {
			if k.Inode>>32 == fakeInodeMSW {
				continue
			}

			if len(entries) > i {
				cacheEntry = entries[i]
			} else {
				continue
			}

			if len(keys) > i+1 {
				cacheEntry.Parent = keys[i+1]
			}

			_ = dr.cacheInode(k, cacheEntry)
		}

		if depth > 0 {
			atomic.AddInt64(dr.hitsCounters[metrics.PathResolutionTag][metrics.ERPCTag], depth)
		}
	} else {
		atomic.AddInt64(dr.missCounters[metrics.PathResolutionTag][metrics.ERPCTag], 1)
	}

	return filename, resolutionErr
}

// Resolve the pathname of a dentry, starting at the pathnameKey in the pathnames table
func (dr *DentryResolver) Resolve(mountID uint32, inode uint64, pathID uint32) (string, error) {
	path, err := dr.ResolveFromCache(mountID, inode)
	if err != nil && dr.erpcEnabled {
		path, err = dr.ResolveFromERPC(mountID, inode, pathID)
	}
	if err != nil && err != errTruncatedParents && dr.mapEnabled {
		path, err = dr.ResolveFromMap(mountID, inode, pathID)
	}
	return path, err
}

func (dr *DentryResolver) resolveParentFromCache(mountID uint32, inode uint64) (uint32, uint64, error) {
	path, err := dr.lookupInodeFromCache(mountID, inode)
	if err != nil {
		atomic.AddInt64(dr.missCounters[metrics.ParentResolutionTag][metrics.CacheTag], 1)
		return 0, 0, ErrEntryNotFound
	}

	atomic.AddInt64(dr.hitsCounters[metrics.ParentResolutionTag][metrics.CacheTag], 1)
	return path.Parent.MountID, path.Parent.Inode, nil
}

func (dr *DentryResolver) resolveParentFromMap(mountID uint32, inode uint64, pathID uint32) (uint32, uint64, error) {
	path, err := dr.lookupInodeFromMap(mountID, inode, pathID)
	if err != nil {
		atomic.AddInt64(dr.missCounters[metrics.ParentResolutionTag][metrics.KernelMapsTag], 1)
		return 0, 0, err
	}

	atomic.AddInt64(dr.hitsCounters[metrics.ParentResolutionTag][metrics.KernelMapsTag], 1)
	return path.Parent.MountID, path.Parent.Inode, nil
}

// GetParent returns the parent mount_id/inode
func (dr *DentryResolver) GetParent(mountID uint32, inode uint64, pathID uint32) (uint32, uint64, error) {
	parentMountID, parentInode, err := dr.resolveParentFromCache(mountID, inode)
	if err != nil {
		parentMountID, parentInode, err = dr.resolveParentFromMap(mountID, inode, pathID)
	}
	return parentMountID, parentInode, err
}

func (dr *DentryResolver) BumpCacheGenerations() {
	dr.cacheGenerationsLock.Lock()
	defer dr.cacheGenerationsLock.Unlock()

	for genMountID := range dr.cacheGenerations {
		dr.cacheGenerations[genMountID] += 1
	}
}

// Start the dentry resolver
func (dr *DentryResolver) Start(probe *Probe) error {
	pathnames, err := probe.Map("pathnames")
	if err != nil {
		return err
	}
	dr.pathnames = pathnames

	erpcStatsFB, err := probe.Map("dr_erpc_stats_fb")
	if err != nil {
		return err
	}
	dr.erpcStats[0] = erpcStatsFB

	erpcStatsBB, err := probe.Map("dr_erpc_stats_bb")
	if err != nil {
		return err
	}
	dr.erpcStats[1] = erpcStatsBB

	bufferSelector, err := probe.Map("buffer_selector")
	if err != nil {
		return err
	}
	dr.bufferSelector = bufferSelector

	return nil
}

// Close cleans up the eRPC segment
func (dr *DentryResolver) Close() error {
	return errors.Wrap(unix.Munmap(dr.erpcSegment), "couldn't cleanup eRPC memory segment")
}

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

// NewDentryResolver returns a new dentry resolver
func NewDentryResolver(probe *Probe) (*DentryResolver, error) {
	// We need at least 7 memory pages for the eRPC segment method to work.
	// For each segment of a path, we write 16 bytes to store (inode, mount_id, path_id), and then at least 2 bytes to
	// store the smallest possible path (segment of size 1 + trailing 0). 18 * 1500 = 27 000.
	// Then, 27k + 256 / page_size < 7.
	segment := make([]byte, 7*os.Getpagesize())

	hitsCounters := make(map[string]map[string]*int64)
	missCounters := make(map[string]map[string]*int64)
	for _, resolution := range metrics.AllResolutionsTags {
		hitsCounters[resolution] = make(map[string]*int64)
		missCounters[resolution] = make(map[string]*int64)
		for _, resolutionType := range metrics.AllTypesTags {
			// procfs resolution doesn't exist in the dentry resolver
			if resolutionType == metrics.ProcFSTag {
				continue
			}
			hits := int64(0)
			miss := int64(0)
			hitsCounters[resolution][resolutionType] = &hits
			missCounters[resolution][resolutionType] = &miss
		}
	}

	numCPU, err := utils.NumCPU()
	if err != nil {
		return nil, errors.Wrapf(err, "couldn't fetch the host CPU count")
	}

	return &DentryResolver{
		client:           probe.statsdClient,
		cache:            make(map[uint32]*lru.Cache),
		cacheGenerations: make(map[uint32]int),
		dentryCacheSize:  probe.config.DentryCacheSize,
		erpc:             probe.erpc,
		erpcSegment:      segment,
		erpcSegmentSize:  len(segment),
		erpcRequest:      ERPCRequest{},
		erpcEnabled:      probe.config.ERPCDentryResolutionEnabled,
		erpcStatsZero:    make([]eRPCStats, numCPU),
		mapEnabled:       probe.config.MapDentryResolutionEnabled,
		hitsCounters:     hitsCounters,
		missCounters:     missCounters,
		numCPU:           numCPU,
	}, nil
}
