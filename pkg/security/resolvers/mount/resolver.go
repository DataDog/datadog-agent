// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package mount holds mount related files
package mount

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers/dentry"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"

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
	// mounts LRU limit: 100000 mounts
	mountsLimit       = 100000
	danglingListLimit = 2000
)

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
	dangling        *simplelru.LRU[uint32, *model.Mount]
	fallbackLimiter *utils.Limiter[uint64]

	// stats
	cacheHitsStats atomic.Int64
	cacheMissStats atomic.Int64
	procHitsStats  atomic.Int64
	procMissStats  atomic.Int64

	// debug tracing (set by caller, safe because lock is held)
	activeTrace     *[]model.ProcessingCheckpoint
	activeStartTime time.Time
}

// traceCheckpoint appends a checkpoint to the active trace if set
func (mr *Resolver) traceCheckpoint(name string) {
	if mr.activeTrace != nil && !mr.activeStartTime.IsZero() {
		*mr.activeTrace = append(*mr.activeTrace, model.ProcessingCheckpoint{
			Name:      name,
			ElapsedUs: time.Since(mr.activeStartTime).Microseconds(),
		})
	}
}

// SetActiveTrace sets the trace target for subsequent calls
func (mr *Resolver) SetActiveTrace(trace *[]model.ProcessingCheckpoint, startTime time.Time) {
	mr.lock.Lock()
	mr.activeTrace = trace
	mr.activeStartTime = startTime
	mr.lock.Unlock()
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
		mr.insert(sm)
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
		mr.insert(sm)
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
	mounts := []*model.Mount{}

	err := GetPidProcfs(kernel.ProcFSRoot(), pid, func(sm *model.Mount) {
		mr.mounts.Remove(sm.MountID)
		mounts = append(mounts, sm)
		nrMounts++
	})

	for _, m := range mounts {
		mr.insert(m)
	}

	if err != nil {
		return fmt.Errorf("error synchronizing the pid procfs: %v", err)
	}
	seclog.Infof("procfs sync pid cache found %d entries", nrMounts)
	return nil
}

// syncCacheFromProcfs Snapshots the mounts of the pid namespace using the listmount api
func (mr *Resolver) syncPidListmount(pid uint32) error {
	nrMounts := 0
	mounts := []*model.Mount{}

	err := GetPidListmount(kernel.ProcFSRoot(), pid, func(sm *model.Mount) {
		mr.mounts.Remove(sm.MountID)
		mounts = append(mounts, sm)
		nrMounts++
	})

	for _, m := range mounts {
		mr.insert(m)
	}

	if err != nil {
		return fmt.Errorf("error synchronizing from procfs: %v", err)
	}
	seclog.Infof("listmount sync pid found %d entries", nrMounts)
	return nil
}

// syncPidNamespace snapshots the namespace of the pid
func (mr *Resolver) syncPidNamespace(pid uint32) error {
	mr.traceCheckpoint(fmt.Sprintf("mount_sync_pid_ns_%d", pid))
	var err error
	if mr.opts.SnapshotUsingListMount {
		mr.traceCheckpoint("mount_sync_listmount")
		err = mr.syncPidListmount(pid)
		// TODO: Decide if it makes sense to fully regress to procfs when it fails only once
		if err != nil {
			mr.opts.SnapshotUsingListMount = false
		}
	}

	if !mr.opts.SnapshotUsingListMount {
		mr.traceCheckpoint("mount_sync_procfs")
		err = mr.syncPidProcfs(pid)
		mr.traceCheckpoint("mount_sync_procfs_done")
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

	mr.insert(mount)
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

	// Update the mount path for all the children
	mr.walkMountSubtree(mount, func(child *model.Mount) {
		child.Path = ""
		_, _, _, _ = mr.getMountPath(child.MountID, 0)
	})
}

func (mr *Resolver) walkMountSubtree(mount *model.Mount, cb func(*model.Mount)) {
	stack := []*model.Mount{mount}
	visited := make(map[uint32]struct{})

	for len(stack) > 0 {
		n := len(stack) - 1
		curr := stack[n]
		stack = stack[:n]

		if _, ok := visited[curr.MountID]; ok {
			continue
		}
		visited[curr.MountID] = struct{}{}

		for _, childID := range curr.Children {
			if child, _ := mr.mounts.Get(childID); child != nil {
				if _, seen := visited[childID]; !seen {
					stack = append(stack, child)
				}
			}
		}

		cb(curr)
	}
}

func (mr *Resolver) delete(mount *model.Mount) {
	// Remove it from the parents' list of children
	// Parent MountID == 0 means that it was a detached mount, no need to update its parent
	if mount.ParentPathKey.MountID != 0 {
		parent, exists := mr.mounts.Get(mount.ParentPathKey.MountID)
		if exists {
			for i := 0; i != len(parent.Children); i++ {
				if parent.Children[i] == mount.MountID {
					parent.Children = append(parent.Children[:i], parent.Children[i+1:]...)
					break
				}
			}
		}
	}

	// Add any children to the dangling list
	for _, childID := range mount.Children {
		child, ok := mr.mounts.Get(childID)
		if ok {
			mr.dangling.Add(childID, child)
		}
	}

	// Remove it from the dangling list too
	if _, exists := mr.dangling.Get(mount.MountID); exists {
		mr.dangling.Remove(mount.MountID)
	}

	mr.mounts.Remove(mount.MountID)
}

// Delete a mount from the cache. Set mountIDUnique to 0 if you don't have a unique mount id.
func (mr *Resolver) Delete(mountID uint32, mountIDUnique uint64) error {
	if mountID == 0 {
		return errors.New("tried to delete mountid=0")
	}
	mr.lock.Lock()
	defer mr.lock.Unlock()

	m, exists := mr.mounts.Get(mountID)
	if exists && (m.MountIDUnique == 0 || mountIDUnique == 0 || m.MountIDUnique == mountIDUnique) {
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

	mr.insert(&m)

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

func (mr *Resolver) insert(m *model.Mount) {
	if mr.minMountID > m.MountID {
		mr.minMountID = m.MountID
	}

	invalidateChildrenPath := false
	// Remove the previous one if exists
	if m.Origin != model.MountOriginProcfs {
		if prev, ok := mr.mounts.Get(m.MountID); prev != nil && ok {
			if prev.ParentPathKey != m.ParentPathKey {
				invalidateChildrenPath = true
			}
			m.Children = prev.Children
			prev.Children = []uint32{}
			mr.delete(prev)
		}

		// if we're inserting a mount from a kernel event (!= procfs) that isn't the root fs
		// then remove the leading slash from the mountpoint
		if len(m.Path) == 0 && m.MountPointStr != "/" {
			m.MountPointStr = strings.TrimPrefix(m.MountPointStr, "/")
		}
	}

	// Update the list of children of the parent
	parent, ok := mr.mounts.Get(m.ParentPathKey.MountID)
	if ok {
		if !slices.Contains(parent.Children, m.MountID) {
			parent.Children = append(parent.Children, m.MountID)
		}
	} else if m.ParentPathKey.MountID != 0 {
		// No parent found. Add to dangling list
		mr.dangling.Add(m.MountID, m)
	}

	if invalidateChildrenPath {
		mr.walkMountSubtree(m, func(child *model.Mount) {
			child.Path = ""
		})
	}

	// check if this mount has any dangling children
	if m.Origin != model.MountOriginProcfs {
		start := len(m.Children)
		for danglingElem := range mr.dangling.ValuesIter() {
			if danglingElem.ParentPathKey.MountID == m.MountID {
				m.Children = append(m.Children, danglingElem.MountID)
			}
		}

		// remove appended from dangling list
		for i := start; i < len(m.Children); i++ {
			mr.dangling.Remove(m.Children[i])
		}
	}

	mr.mounts.Add(m.MountID, m)
}

func (mr *Resolver) lookupByMountID(mountID uint32) *model.Mount {
	if mount, ok := mr.mounts.Get(mountID); mount != nil && ok {
		return mount
	}

	return nil
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
		return "", source, mount.Origin, ErrParentMountUndefined
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

	mr.traceCheckpoint("mount_lookup_cache")
	mount, source, origin := mr.lookupMount(mountID)
	if mount != nil {
		mr.cacheHitsStats.Inc()
		mr.traceCheckpoint("mount_cache_hit")
		return mount, source, origin, nil
	}
	mr.cacheMissStats.Inc()

	mr.traceCheckpoint("mount_cache_miss_sync")
	if err := mr.syncPidNamespace(pid); err != nil {
		return nil, model.MountSourceUnknown, model.MountOriginUnknown, err
	}
	mr.traceCheckpoint("mount_cache_miss_sync_done")

	if mount, ok := mr.mounts.Get(mountID); mount != nil && ok {
		mr.procHitsStats.Inc()
		mr.traceCheckpoint("mount_proc_hit")
		return mount, model.MountSourceMountID, mount.Origin, nil
	}
	mr.procMissStats.Inc()
	mr.traceCheckpoint("mount_proc_miss")

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

	if err := mr.statsdClient.Count(metrics.MetricMountResolverProcfsHits, mr.procHitsStats.Swap(0), []string{metrics.CacheTag}, 1.0); err != nil {
		return err
	}

	if err := mr.statsdClient.Count(metrics.MetricMountResolverProcfsMiss, mr.procMissStats.Swap(0), []string{metrics.CacheTag}, 1.0); err != nil {
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

// Iterate iterates over all the mounts in the cache and calls the callback function for each mount
func (mr *Resolver) Iterate(cb func(*model.Mount)) {
	mr.lock.RLock()
	defer mr.lock.RUnlock()

	for mount := range mr.mounts.ValuesIter() {
		cb(mount)
	}
}

// NewResolver instantiates a new mount resolver
func NewResolver(statsdClient statsd.ClientInterface, cgroupsResolver *cgroup.Resolver, dentryResolver *dentry.Resolver, opts ResolverOpts) (*Resolver, error) {
	mounts, err := simplelru.NewLRU[uint32, *model.Mount](mountsLimit, nil)
	if err != nil {
		return nil, err
	}

	dangling, err := simplelru.NewLRU[uint32, *model.Mount](danglingListLimit, nil)
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
		dangling:        dangling,
	}

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
