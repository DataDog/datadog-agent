// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package mount holds mount related files
package mount

import (
	"encoding/json"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/dentry"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/security/utils/lru/simplelru"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

const (
	numAllowedMountIDsToResolvePerPeriod = 5
	fallbackLimiterPeriod                = time.Second
	redemptionTime                       = 2 * time.Second
	// should be enough to handle most of in-queue mounts waiting to be deleted
	openQueuePreAllocSize = 32
	// mounts LRU limit: 100000 mounts
	mountsLimit = 100000
)

type redemptionEntry struct {
	mount      *model.Mount
	insertedAt time.Time
}

// ResolverOpts defines mount resolver options
type ResolverOpts struct {
	UseProcFS              bool
	SnapshotUsingListMount bool
}

// Resolver represents a cache for mountpoints and the corresponding file systems
type Resolver struct {
	opts            ResolverOpts
	dentryResolver  *dentry.Resolver
	cgroupsResolver *cgroup.Resolver
	statsdClient    statsd.ClientInterface
	lock            sync.RWMutex
	mounts          *simplelru.LRU[uint32, *model.Mount]
	minMountID      uint32 // used to find the first userspace visible mount ID
	redemption      *simplelru.LRU[uint32, *redemptionEntry]
	fallbackLimiter *utils.Limiter[uint64]

	// stats
	cacheHitsStats atomic.Int64
	cacheMissStats atomic.Int64
	procHitsStats  atomic.Int64
	procMissStats  atomic.Int64
}

// IsMountIDValid returns whether the mountID is valid
func (mr *Resolver) IsMountIDValid(mountID uint32) (bool, error) {
	if mountID == 0 {
		return false, ErrMountUndefined
	}

	if mountID < mr.minMountID {
		return false, ErrMountKernelID
	}

	return true, nil
}

// syncCacheFromListMount Snapshots the current mountpoints using the listmount api
func (mr *Resolver) syncCacheFromListMount() error {
	nrMounts := 0
	err := GetAll(kernel.ProcFSRoot(), func(sm *model.Mount) {
		mr.insert(sm, false)
		nrMounts++
	})

	if err != nil {
		return fmt.Errorf("error synchronizing cache from listmount: %v", err)
	}
	seclog.Infof("listmount sync cache found %d entries", nrMounts)
	return nil
}

// syncCacheFromProcfs Snapshots the current mountpoints using the listmount api
func (mr *Resolver) syncCacheFromProcfs() error {
	nrMounts := 0
	err := GetAllProcfs(kernel.ProcFSRoot(), func(sm *model.Mount) {
		mr.insert(sm, false)
		nrMounts++
	})

	if err != nil {
		return fmt.Errorf("error synchronizing from procfs: %v", err)
	}
	seclog.Infof("procfs sync cache found %d entries", nrMounts)
	return nil
}

// SyncCache Snapshots the current mount points of the system by reading through /proc/[pid]/mountinfo.
func (mr *Resolver) syncCache() error {
	var err error
	if mr.opts.SnapshotUsingListMount {
		err = mr.syncCacheFromListMount()
		// TODO: Decide if it makes sense to fully regress to procfs when it fails only once
		if err != nil {
			mr.opts.SnapshotUsingListMount = false
		}
	}

	if !mr.opts.SnapshotUsingListMount {
		err = mr.syncCacheFromProcfs()
	}

	for mountID := range mr.mounts.KeysIter() {
		if mr.minMountID == 0 || mr.minMountID > mountID {
			mr.minMountID = mountID
		}
	}

	return err
}

// syncCacheFromListMount Snapshots the current mountpoints using procfs
func (mr *Resolver) syncPidProcfs(pid uint32) error {
	nrMounts := 0
	err := GetPidProcfs(kernel.ProcFSRoot(), pid, func(sm *model.Mount) {
		mr.insert(sm, false)
		nrMounts++
	})

	if err != nil {
		return fmt.Errorf("error synchronizing the pid procfs: %v", err)
	}
	seclog.Infof("procfs sync pid cache found %d entries", nrMounts)
	return nil
}

// syncCacheFromProcfs Snapshots the mounts of the pid namespace using the listmount api
func (mr *Resolver) syncPidListmount(pid uint32) error {
	nrMounts := 0
	err := GetPidListmount(kernel.ProcFSRoot(), pid, func(sm *model.Mount) {
		mr.insert(sm, false)
		nrMounts++
	})

	if err != nil {
		return fmt.Errorf("error synchronizing from procfs: %v", err)
	}
	seclog.Infof("listmount sync pid found %d entries", nrMounts)
	return nil
}

// syncPidNamespace snapshots the namespace of the pid
func (mr *Resolver) syncPidNamespace(pid uint32) error {
	var err error
	if mr.opts.SnapshotUsingListMount {
		err = mr.syncPidListmount(pid)
		// TODO: Decide if it makes sense to fully regress to procfs when it fails only once
		if err != nil {
			mr.opts.SnapshotUsingListMount = false
		}
	}

	if !mr.opts.SnapshotUsingListMount {
		err = mr.syncPidProcfs(pid)
	}

	return err
}

// SyncCache Snapshots the current mount points of the system by reading through /proc/[pid]/mountinfo.
func (mr *Resolver) SyncCache() error {
	mr.lock.Lock()
	defer mr.lock.Unlock()

	return mr.syncCache()
}

func (mr *Resolver) insertMoved(mount *model.Mount) {
	mount.MountPointStr, _ = mr.dentryResolver.Resolve(mount.ParentPathKey, false)

	mr.insert(mount, true)
	_, _, _, _ = mr.getMountPath(mount.MountID, 0)

	// Find all the mounts that I'm the parent of
	for mnt := range mr.mounts.ValuesIter() {
		if mnt.ParentPathKey.MountID == mount.MountID {
			if slices.Contains(mount.Children, mnt.MountID) {
				continue
			}

			mount.Children = append(mount.Children, mnt.MountID)
		}
	}

	allChildren, err := mr.getAllChildren(mount)
	if err != nil {
		seclog.Warnf("Error getting the list of children for mount id %d. err = %v", mount.MountID, err)
	}

	for _, child := range allChildren {
		child.Path = ""
		_, _, _, _ = mr.getMountPath(child.MountID, 0)
	}
}

func (mr *Resolver) getAllChildren(mount *model.Mount) (map[uint32]*model.Mount, error) {
	children := map[uint32]*model.Mount{}

	err := mr.getAllChildrenRecursive(mount, children)

	return children, err
}

func (mr *Resolver) getAllChildrenRecursive(mount *model.Mount, mountList map[uint32]*model.Mount) error {
	if _, existed := mountList[mount.MountID]; existed {
		return nil
	}
	mountList[mount.MountID] = mount

	for _, mountid := range mount.Children {
		mnt := mr.lookupByMountID(mountid)
		if mnt != nil {
			err := mr.getAllChildrenRecursive(mnt, mountList)
			if err != nil {
				return err
			}
		} else {
			return fmt.Errorf("could not find mount with the id %d", mountid)
		}
	}
	return nil
}

func (mr *Resolver) delete(mount *model.Mount) {
	now := time.Now()

	mr.deleteOne(mount, now)

	openQueue := make([]uint32, 0, openQueuePreAllocSize)
	openQueue = append(openQueue, mount.MountID)

	for len(openQueue) != 0 {
		curr, rest := openQueue[len(openQueue)-1], openQueue[:len(openQueue)-1]
		openQueue = rest

		for child := range mr.mounts.ValuesIter() {
			if child.ParentPathKey.MountID == curr {
				openQueue = append(openQueue, child.MountID)
				mr.deleteOne(child, now)
			}
		}
	}
}

func (mr *Resolver) deleteOne(curr *model.Mount, now time.Time) {
	parent, exists := mr.mounts.Get(curr.ParentPathKey.MountID)
	if exists {
		for i := 0; i != len(parent.Children); i++ {
			if parent.Children[i] == curr.MountID {
				parent.Children = append(parent.Children[:i], parent.Children[i+1:]...)
				break
			}
		}
	}

	mr.mounts.Remove(curr.MountID)

	entry := redemptionEntry{
		mount:      curr,
		insertedAt: now,
	}
	mr.redemption.Add(curr.MountID, &entry)
}

func (mr *Resolver) finalize(mount *model.Mount) {
	mr.mounts.Remove(mount.MountID)
}

// Delete a mount from the cache
func (mr *Resolver) Delete(mountID uint32) error {
	mr.lock.Lock()
	defer mr.lock.Unlock()

	if m, exists := mr.mounts.Get(mountID); exists {
		mr.delete(m)
	} else {
		return &ErrMountNotFound{MountID: mountID}
	}

	return nil
}

// ResolveFilesystem returns the name of the filesystem
func (mr *Resolver) ResolveFilesystem(mountID uint32, pid uint32) (string, error) {
	mr.lock.Lock()
	defer mr.lock.Unlock()

	mount, _, _, err := mr.resolveMount(mountID, pid)
	if err != nil {
		return model.UnknownFS, err
	}

	return mount.GetFSType(), nil
}

// Insert a new mount point in the cache
func (mr *Resolver) Insert(m model.Mount) error {
	if m.MountID == 0 {
		return ErrMountUndefined
	}

	mr.lock.Lock()
	defer mr.lock.Unlock()

	mr.insert(&m, false)

	return nil
}

// InsertMoved inserts a mount point from move_mount
func (mr *Resolver) InsertMoved(m model.Mount) error {
	if m.MountID == 0 {
		return ErrMountUndefined
	}

	mr.lock.Lock()
	defer mr.lock.Unlock()

	mr.insertMoved(&m)

	return nil
}

func (mr *Resolver) insert(m *model.Mount, moved bool) {
	// umount the previous one if exists
	if prev, ok := mr.mounts.Get(m.MountID); prev != nil && ok {
		m.Children = prev.Children

		if !moved {
			// put the prev entry and the all the children in the redemption list
			mr.delete(prev)
			// force a finalize on the entry itself as it will be overridden by the new one
			mr.finalize(prev)
		}
	} else if _, ok := mr.redemption.Get(m.MountID); ok {
		// this will call the eviction function that will call the finalize
		mr.redemption.Remove(m.MountID)
	}

	// if we're inserting a mountpoint from a kernel event (!= procfs) that isn't the root fs
	// then remove the leading slash from the mountpoint
	if len(m.Path) == 0 && m.MountPointStr != "/" {
		m.MountPointStr = strings.TrimPrefix(m.MountPointStr, "/")
	}

	if mr.minMountID > m.MountID {
		mr.minMountID = m.MountID
	}

	// Update the list of children of the parent
	parent, ok := mr.mounts.Get(m.ParentPathKey.MountID)
	if ok {
		if !slices.Contains(parent.Children, m.MountID) {
			parent.Children = append(parent.Children, m.MountID)
		}
	}

	mr.mounts.Add(m.MountID, m)
}

func (mr *Resolver) getFromRedemption(mountID uint32) *model.Mount {
	entry, exists := mr.redemption.Get(mountID)
	if !exists || time.Since(entry.insertedAt) > redemptionTime {
		return nil
	}
	return entry.mount
}

func (mr *Resolver) lookupByMountID(mountID uint32) *model.Mount {
	if mount, ok := mr.mounts.Get(mountID); mount != nil && ok {
		return mount
	}

	return mr.getFromRedemption(mountID)
}

func (mr *Resolver) lookupMount(mountID uint32) (*model.Mount, model.MountSource, model.MountOrigin) {
	mount := mr.lookupByMountID(mountID)

	if mount == nil {
		return nil, model.MountSourceUnknown, model.MountOriginUnknown
	}

	return mount, model.MountSourceMountID, mount.Origin
}

func (mr *Resolver) _getMountPath(mountID uint32, pid uint32, cache map[uint32]bool) (string, model.MountSource, model.MountOrigin, error) {
	if _, err := mr.IsMountIDValid(mountID); err != nil {
		return "", model.MountSourceUnknown, model.MountOriginUnknown, err
	}

	mount, source, origin := mr.lookupMount(mountID)
	if mount == nil {
		return "", source, origin, &ErrMountNotFound{MountID: mountID}
	}

	if len(mount.Path) > 0 {
		return mount.Path, source, origin, nil
	}

	mountPointStr := mount.MountPointStr
	if mountPointStr == "/" {
		return mountPointStr, source, mount.Origin, nil
	}

	// avoid infinite loop
	if _, exists := cache[mountID]; exists {
		return "", source, mount.Origin, ErrMountLoop
	}
	cache[mountID] = true

	if mount.Detached {
		return "/", source, mount.Origin, nil
	}

	if mount.ParentPathKey.MountID == 0 {
		return "", source, mount.Origin, ErrMountUndefined
	}

	parentMountPath, parentSource, parentOrigin, err := mr._getMountPath(mount.ParentPathKey.MountID, pid, cache)
	if err != nil {
		return "", parentSource, parentOrigin, err
	}
	mountPointStr = path.Join(parentMountPath, mountPointStr)

	if parentSource != model.MountSourceMountID {
		source = parentSource
	}

	if parentOrigin != model.MountOriginEvent {
		origin = parentOrigin
	}

	if len(mountPointStr) == 0 {
		return "", source, origin, ErrMountPathEmpty
	}

	mount.Path = mountPointStr

	return mountPointStr, source, origin, nil
}

func (mr *Resolver) getMountPath(mountID uint32, pid uint32) (string, model.MountSource, model.MountOrigin, error) {
	return mr._getMountPath(mountID, pid, map[uint32]bool{})
}

// ResolveMountRoot returns the root of a mount identified by its mount ID.
func (mr *Resolver) ResolveMountRoot(mountID uint32, pid uint32) (string, model.MountSource, model.MountOrigin, error) {
	mr.lock.Lock()
	defer mr.lock.Unlock()

	return mr.resolveMountRoot(mountID, pid)
}

func (mr *Resolver) resolveMountRoot(mountID uint32, pid uint32) (string, model.MountSource, model.MountOrigin, error) {
	mount, source, origin, err := mr.resolveMount(mountID, pid)
	if err != nil {
		return "", source, origin, err
	}
	return mount.RootStr, source, origin, err
}

// ResolveMountPath returns the path of a mount identified by its mount ID.
func (mr *Resolver) ResolveMountPath(mountID uint32, pid uint32) (string, model.MountSource, model.MountOrigin, error) {
	mr.lock.Lock()
	defer mr.lock.Unlock()

	return mr.resolveMountPath(mountID, pid)
}

func (mr *Resolver) resolveMountPath(mountID uint32, pid uint32) (string, model.MountSource, model.MountOrigin, error) {
	if _, err := mr.IsMountIDValid(mountID); err != nil {
		return "", model.MountSourceUnknown, model.MountOriginUnknown, err
	}

	path, source, origin, err := mr.getMountPath(mountID, pid)
	if err == nil {
		mr.cacheHitsStats.Inc()
		return path, source, origin, nil
	}
	mr.cacheMissStats.Inc()

	if !mr.opts.UseProcFS {
		return "", model.MountSourceUnknown, model.MountOriginUnknown, &ErrMountNotFound{MountID: mountID}
	}

	if err := mr.syncPidNamespace(pid); err != nil {
		return "", model.MountSourceUnknown, model.MountOriginUnknown, err
	}

	path, source, origin, err = mr.getMountPath(mountID, pid)
	if err == nil {
		mr.procHitsStats.Inc()
		return path, source, origin, nil
	}
	mr.procMissStats.Inc()

	return "", model.MountSourceUnknown, model.MountOriginUnknown, err
}

// ResolveMount returns the mount
func (mr *Resolver) ResolveMount(mountID uint32, pid uint32) (*model.Mount, model.MountSource, model.MountOrigin, error) {
	mr.lock.Lock()
	defer mr.lock.Unlock()

	return mr.resolveMount(mountID, pid)
}

func (mr *Resolver) resolveMount(mountID uint32, pid uint32) (*model.Mount, model.MountSource, model.MountOrigin, error) {
	if _, err := mr.IsMountIDValid(mountID); err != nil {
		return nil, model.MountSourceUnknown, model.MountOriginUnknown, err
	}

	mount, source, origin := mr.lookupMount(mountID)
	if mount != nil {
		mr.cacheHitsStats.Inc()
		return mount, source, origin, nil
	}
	mr.cacheMissStats.Inc()

	if err := mr.syncPidNamespace(pid); err != nil {
		return nil, model.MountSourceUnknown, model.MountOriginUnknown, err
	}

	if mount, ok := mr.mounts.Get(mountID); mount != nil && ok {
		mr.procHitsStats.Inc()
		return mount, model.MountSourceMountID, mount.Origin, nil
	}
	mr.procMissStats.Inc()

	return nil, model.MountSourceUnknown, model.MountOriginUnknown, &ErrMountNotFound{MountID: mountID}
}

// SendStats sends metrics about the current state of the mount resolver
func (mr *Resolver) SendStats() error {
	mr.lock.RLock()
	defer mr.lock.RUnlock()

	if err := mr.statsdClient.Count(metrics.MetricMountResolverHits, mr.cacheHitsStats.Swap(0), []string{metrics.CacheTag}, 1.0); err != nil {
		return err
	}

	if err := mr.statsdClient.Count(metrics.MetricMountResolverMiss, mr.cacheMissStats.Swap(0), []string{metrics.CacheTag}, 1.0); err != nil {
		return err
	}

	return mr.statsdClient.Gauge(metrics.MetricMountResolverCacheSize, float64(mr.mounts.Len()), []string{}, 1.0)
}

// ToJSON return a json version of the cache
func (mr *Resolver) ToJSON() ([]byte, error) {
	dump := struct {
		Entries []json.RawMessage
	}{}

	mr.lock.RLock()
	defer mr.lock.RUnlock()

	for mount := range mr.mounts.ValuesIter() {
		d, err := json.Marshal(mount)
		if err == nil {
			dump.Entries = append(dump.Entries, d)
		}
	}

	return json.Marshal(dump)
}

// NewResolver instantiates a new mount resolver
func NewResolver(statsdClient statsd.ClientInterface, cgroupsResolver *cgroup.Resolver, dentryResolver *dentry.Resolver, opts ResolverOpts) (*Resolver, error) {
	mounts, err := simplelru.NewLRU[uint32, *model.Mount](mountsLimit, nil)
	if err != nil {
		return nil, err
	}

	mr := &Resolver{
		opts:            opts,
		statsdClient:    statsdClient,
		cgroupsResolver: cgroupsResolver,
		lock:            sync.RWMutex{},
		mounts:          mounts,
		dentryResolver:  dentryResolver,
	}

	redemption, err := simplelru.NewLRU(1024, func(_ uint32, entry *redemptionEntry) {
		mr.finalize(entry.mount)
	})
	if err != nil {
		return nil, err
	}
	mr.redemption = redemption

	// create a rate limiter that allows for 64 mount IDs
	limiter, err := utils.NewLimiter[uint64](64, numAllowedMountIDsToResolvePerPeriod, fallbackLimiterPeriod)
	if err != nil {
		return nil, err
	}
	mr.fallbackLimiter = limiter

	if mr.opts.SnapshotUsingListMount && !HasListMount() {
		mr.opts.SnapshotUsingListMount = false
	}

	return mr, nil
}
