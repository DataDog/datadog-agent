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
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/moby/sys/mountinfo"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-go/v5/statsd"

	skernel "github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup"
	cmodel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/debugging"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/security/utils/cache"
	"github.com/DataDog/datadog-agent/pkg/security/utils/lru/simplelru"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

const (
	numAllowedMountIDsToResolvePerPeriod = 5
	fallbackLimiterPeriod                = time.Second
	redemptionTime                       = 2 * time.Second
)

type redemptionEntry struct {
	mount      *model.Mount
	insertedAt time.Time
}

// newMountFromMountInfo - Creates a new Mount from parsed MountInfo data
func newMountFromMountInfo(mnt *mountinfo.Info) *model.Mount {
	root := mnt.Root

	if mnt.FSType == "btrfs" {
		var subvol string
		for _, opt := range strings.Split(mnt.VFSOptions, ",") {
			name, val, ok := strings.Cut(opt, "=")
			if ok && name == "subvol" {
				subvol = val
			}
		}

		if subvol != "" {
			root = strings.TrimPrefix(root, subvol)
		}

		if root == "" {
			root = "/"
		}
	}

	// create a Mount out of the parsed MountInfo
	return &model.Mount{
		MountID: uint32(mnt.ID),
		Device:  utils.Mkdev(uint32(mnt.Major), uint32(mnt.Minor)),
		ParentPathKey: model.PathKey{
			MountID: uint32(mnt.Parent),
		},
		FSType:        mnt.FSType,
		MountPointStr: mnt.Mountpoint,
		Path:          mnt.Mountpoint,
		RootStr:       root,
		Origin:        model.MountOriginProcfs,
	}
}

// ResolverOpts defines mount resolver options
type ResolverOpts struct {
	UseProcFS bool
}

// Resolver represents a cache for mountpoints and the corresponding file systems
type Resolver struct {
	opts            ResolverOpts
	cgroupsResolver *cgroup.Resolver
	statsdClient    statsd.ClientInterface
	lock            sync.RWMutex
	mounts          *simplelru.LRU[uint32, *model.Mount]
	pidToMounts     *cache.TwoLayersLRU[uint32, uint32, *model.Mount]
	minMountID      uint32 // used to find the first userspace visible mount ID
	redemption      *simplelru.LRU[uint32, *redemptionEntry]
	fallbackLimiter *utils.Limiter[uint64]
	debugLog        *debugging.AtomicString
	mountLog        *debugging.RollingLog

	// stats
	cacheHitsStats *atomic.Int64
	cacheMissStats *atomic.Int64
	procHitsStats  *atomic.Int64
	procMissStats  *atomic.Int64
}

// ResetDebugLog clears the debug log
func (mr *Resolver) ResetDebugLog() {
	mr.debugLog.Clear()
}

// IsMountIDValid returns whether the mountID is valid
func (mr *Resolver) IsMountIDValid(mountID uint32) (bool, error) {
	mr.debugLog.Add(fmt.Sprintf("IsMountIDValid() [In] mountID = %d\n", mountID))

	if mountID == 0 {
		mr.debugLog.Add(fmt.Sprintf("IsMountIDValid() [Out] mountID = %d is undefined\n", mountID))
		return false, ErrMountUndefined
	}

	if mountID < mr.minMountID {
		mr.debugLog.Add(fmt.Sprintf("IsMountIDValid() [Out] mountID = %d is a kernel ID (< %d)\n", mountID, mr.minMountID))
		return false, ErrMountKernelID
	}

	mr.debugLog.Add(fmt.Sprintf("IsMountIDValid() [Out] mountID = %d is valid\n", mountID))
	return true, nil
}

// SyncCache Snapshots the current mount points of the system by reading through /proc/[pid]/mountinfo.
func (mr *Resolver) SyncCache(pid uint32) error {
	mr.lock.Lock()
	defer mr.lock.Unlock()

	err := mr.syncPid(pid)

	// store the minimal mount ID found to use it as a reference
	if pid == 1 {
		for mountID := range mr.mounts.KeysIter() {
			if mr.minMountID == 0 || mr.minMountID > mountID {
				mr.minMountID = mountID
			}
		}
	}

	return err
}

func (mr *Resolver) syncPid(pid uint32) error {
	mnts, err := kernel.ParseMountInfoFile(int32(pid))
	if err != nil {
		return err
	}

	for _, mnt := range mnts {
		if m, exists := mr.mounts.Get(uint32(mnt.ID)); m != nil && exists {
			mr.updatePidMapping(m, pid)
			continue
		}

		m := newMountFromMountInfo(mnt)
		mr.insert(m, pid)
	}

	return nil
}

// syncCache update cache with the first working pid
func (mr *Resolver) syncCache(mountID uint32, pids []uint32) error {
	var err error

	for _, pid := range pids {
		key := uint64(mountID)<<32 | uint64(pid)
		if !mr.fallbackLimiter.Allow(key) {
			continue
		}

		if err = mr.syncPid(pid); err == nil {
			return nil
		}
	}

	return err
}

const openQueuePreAllocSize = 32 // should be enough to handle most of in queue mounts waiting to be deleted

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

func (mr *Resolver) AddToMountLog(s string) {
	mr.mountLog.Add(s)
}

func (mr *Resolver) deleteOne(curr *model.Mount, now time.Time) {
	mr.mounts.Remove(curr.MountID)
	mr.pidToMounts.RemoveKey2(curr.MountID)

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
func (mr *Resolver) ResolveFilesystem(mountID uint32, device uint32, pid uint32, containerID containerutils.ContainerID) (string, error) {
	mr.lock.Lock()
	defer mr.lock.Unlock()

	mount, _, _, err := mr.resolveMount(mountID, device, pid, containerID)
	if err != nil {
		return model.UnknownFS, err
	}

	return mount.GetFSType(), nil
}

// Insert a new mount point in the cache
func (mr *Resolver) Insert(m model.Mount, pid uint32) error {
	if m.MountID == 0 {
		return ErrMountUndefined
	}

	mr.lock.Lock()
	defer mr.lock.Unlock()

	mr.insert(&m, pid)

	return nil
}

func (mr *Resolver) updatePidMapping(m *model.Mount, pid uint32) {
	if pid == 0 {
		return
	}

	mr.pidToMounts.Add(pid, m.MountID, m)
}

// DelPid removes the pid form the pid mapping
func (mr *Resolver) DelPid(pid uint32) {
	if pid == 0 {
		return
	}

	mr.lock.Lock()
	defer mr.lock.Unlock()

	mr.pidToMounts.RemoveKey1(pid)
}

func (mr *Resolver) insert(m *model.Mount, pid uint32) {
	mr.mountLog.Add(fmt.Sprintf("insert pid=%d: %+v", pid, *m))

	// umount the previous one if exists
	if prev, ok := mr.mounts.Get(m.MountID); prev != nil && ok {
		// Log duplicate mountID detection
		prevPath := prev.Path
		if prevPath == "" {
			prevPath = prev.MountPointStr
		}
		newPath := m.Path
		if newPath == "" {
			newPath = m.MountPointStr
		}

		mr.mountLog.Add(fmt.Sprintf("dupe. prev:%+v", *prev))

		// put the prev entry and the all the children in the redemption list
		mr.delete(prev)
		// force a finalize on the entry itself as it will be overridden by the new one
		mr.finalize(prev)
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

	mr.mounts.Add(m.MountID, m)

	mr.updatePidMapping(m, pid)
}

func (mr *Resolver) getFromRedemption(mountID uint32) *model.Mount {
	mr.debugLog.Add(fmt.Sprintf("getFromRedemption() [In] mountID = %d\n", mountID))

	entry, exists := mr.redemption.Get(mountID)
	if !exists || time.Since(entry.insertedAt) > redemptionTime {
		mr.debugLog.Add(fmt.Sprintf("getFromRedemption() [Out] mountID = %d not found or expired\n", mountID))
		return nil
	}

	mr.debugLog.Add(fmt.Sprintf("getFromRedemption() [Out] mountID = %d found. %+v\n", mountID, *entry.mount))
	return entry.mount
}

// func (mr *Resolver) lookupByMountID(mountID uint32, pid uint32) *model.Mount {
func (mr *Resolver) lookupByMountID(mountID uint32) *model.Mount {
	mr.debugLog.Add(fmt.Sprintf("lookupByMountID() [In] mountID = %d\n", mountID))

	//if mount, ok := mr.pidToMounts.Get(mountID, pid); mount != nil && ok {
	if mount, ok := mr.mounts.Get(mountID); mount != nil && ok {
		mr.debugLog.Add(fmt.Sprintf("lookupByMountID() [Out] mountID = %d found in mounts cache: %+v\n", mountID, *mount))
		return mount
	}

	mr.debugLog.Add(fmt.Sprintf("lookupByMountID() [Out] mountID = %d not found in mounts cache, trying redemption\n", mountID))
	return mr.getFromRedemption(mountID)
}

func (mr *Resolver) lookupByDevice(device uint32, pid uint32) *model.Mount {
	mr.debugLog.Add(fmt.Sprintf("lookupByDevice() [In] device = %d, pid = %d\n", device, pid))

	var result *model.Mount

	mr.pidToMounts.WalkInner(pid, func(_ uint32, mount *model.Mount) bool {
		if mount.Device == device {
			// should be consistent across all the mounts
			if result != nil && result.MountPointStr != mount.MountPointStr {
				mr.debugLog.Add(fmt.Sprintf("lookupByDevice() inconsistent mountpoints for device = %d\n", device))
				result = nil
				return false
			}
			mr.debugLog.Add(fmt.Sprintf("lookupByDevice() found mount with device = %d, mountID = %d\n", device, mount.MountID))
			result = mount
		}
		return true
	})

	if result == nil {
		mr.debugLog.Add(fmt.Sprintf("lookupByDevice() [Out] no mount found for device = %d, pid = %d\n", device, pid))
	} else {
		mr.debugLog.Add(fmt.Sprintf("lookupByDevice() [Out] found mount with mountID = %d for device = %d, pid = %d\n", result.MountID, device, pid))
	}

	return result
}

func (mr *Resolver) lookupMount(mountID uint32, device uint32, pid uint32) (*model.Mount, model.MountSource, model.MountOrigin) {
	mr.debugLog.Add(fmt.Sprintf("lookupMount() [In] mountID = %d, device = %d, pid = %d\n", mountID, device, pid))

	mount := mr.lookupByMountID(mountID)
	if mount != nil {
		mr.debugLog.Add(fmt.Sprintf("lookupMount() [Out] found by mountID = %d\n", mountID))
		return mount, model.MountSourceMountID, mount.Origin
	}

	mr.debugLog.Add(fmt.Sprintf("lookupMount() not found by mountID, trying by device\n"))
	mount = mr.lookupByDevice(device, pid)
	if mount == nil {
		mr.debugLog.Add(fmt.Sprintf("lookupMount() [Out] mount not found\n"))
		return nil, model.MountSourceUnknown, model.MountOriginUnknown
	}

	mr.debugLog.Add(fmt.Sprintf("lookupMount() [Out] found by device = %d\n", device))
	return mount, model.MountSourceDevice, mount.Origin
}

func (mr *Resolver) _getMountPath(mountID uint32, device uint32, pid uint32, cache map[uint32]bool) (string, model.MountSource, model.MountOrigin, error) {
	mr.debugLog.Add(fmt.Sprintf("_getMountPath() [In] mountID = %d, device = %d, pid = %d\n", mountID, device, pid))

	if _, err := mr.IsMountIDValid(mountID); err != nil {
		mr.debugLog.Add(fmt.Sprintf("_getMountPath() [Out] invalid mountID: %v\n", err))
		return "", model.MountSourceUnknown, model.MountOriginUnknown, err
	}

	mount, source, origin := mr.lookupMount(mountID, device, pid)
	if mount == nil {
		mr.debugLog.Add(fmt.Sprintf("_getMountPath() [Out] mount not found\n"))
		return "", source, origin, &ErrMountNotFound{MountID: mountID}
	}

	if len(mount.Path) > 0 {
		// INVALID PATH BEING RETURNED HERE
		mr.debugLog.Add(fmt.Sprintf("_getMountPath() [Out] cached path: %+v\n", mount))
		return mount.Path, source, origin, nil
	}

	mountPointStr := mount.MountPointStr
	if mountPointStr == "/" {
		mr.debugLog.Add(fmt.Sprintf("_getMountPath() [Out] root path: %s\n", mountPointStr))
		return mountPointStr, source, mount.Origin, nil
	}

	// avoid infinite loop
	if _, exists := cache[mountID]; exists {
		mr.debugLog.Add(fmt.Sprintf("_getMountPath() [Out] loop detected for mountID = %d\n", mountID))
		return "", source, mount.Origin, ErrMountLoop
	}
	cache[mountID] = true

	if mount.ParentPathKey.MountID == 0 {
		mr.debugLog.Add(fmt.Sprintf("_getMountPath() [Out] undefined parent mountID\n"))
		return "", source, mount.Origin, ErrMountUndefined
	}

	mr.debugLog.Add(fmt.Sprintf("_getMountPath() recursing to parent mountID = %d\n", mount.ParentPathKey.MountID))
	parentMountPath, parentSource, parentOrigin, err := mr._getMountPath(mount.ParentPathKey.MountID, mount.Device, pid, cache)
	if err != nil {
		mr.debugLog.Add(fmt.Sprintf("_getMountPath() [Out] error resolving parent path: %v\n", err))
		return "", parentSource, parentOrigin, err
	}
	mountPointStr = path.Join(parentMountPath, mountPointStr)
	mr.debugLog.Add(fmt.Sprintf("_getMountPath() joined path: %s\n", mountPointStr))

	if parentSource != model.MountSourceMountID {
		source = parentSource
	}

	if parentOrigin != model.MountOriginEvent {
		origin = parentOrigin
	}

	if len(mountPointStr) == 0 {
		mr.debugLog.Add(fmt.Sprintf("_getMountPath() [Out] empty path\n"))
		return "", source, origin, ErrMountPathEmpty
	}

	mount.Path = mountPointStr
	mr.debugLog.Add(fmt.Sprintf("_getMountPath() [Out] resolved path: %s\n", mountPointStr))

	return mountPointStr, source, origin, nil
}

func (mr *Resolver) getMountPath(mountID uint32, device uint32, pid uint32) (string, model.MountSource, model.MountOrigin, error) {
	mr.debugLog.Add(fmt.Sprintf("getMountPath() [In] mountID = %d, device = %d, pid = %d\n", mountID, device, pid))
	path, source, origin, err := mr._getMountPath(mountID, device, pid, map[uint32]bool{})
	mr.debugLog.Add(fmt.Sprintf("getMountPath() [Out] path = %s, source = %d, origin = %d, err = %v\n", path, source, origin, err))
	return path, source, origin, err
}

// ResolveMountRoot returns the root of a mount identified by its mount ID.
func (mr *Resolver) ResolveMountRoot(mountID uint32, device uint32, pid uint32, containerID containerutils.ContainerID) (string, model.MountSource, model.MountOrigin, error) {
	mr.lock.Lock()
	defer mr.lock.Unlock()

	return mr.resolveMountRoot(mountID, device, pid, containerID)
}

func (mr *Resolver) resolveMountRoot(mountID uint32, device uint32, pid uint32, containerID containerutils.ContainerID) (string, model.MountSource, model.MountOrigin, error) {
	mr.debugLog.Add(fmt.Sprintf("resolveMountRoot() [In] mountID = %d, device = %d, pid = %d, containerID = %s\n", mountID, device, pid, containerID))
	mount, source, origin, err := mr.resolveMount(mountID, device, pid, containerID)
	if err != nil {
		mr.debugLog.Add(fmt.Sprintf("resolveMountRoot() [Out] error resolving mount: %v\n", err))
		return "", source, origin, err
	}
	mr.debugLog.Add(fmt.Sprintf("resolveMountRoot() [Out] root = %s\n", mount.RootStr))
	return mount.RootStr, source, origin, err
}

// ResolveMountPath returns the path of a mount identified by its mount ID.
func (mr *Resolver) ResolveMountPath(mountID uint32, device uint32, pid uint32, containerID containerutils.ContainerID) (string, model.MountSource, model.MountOrigin, error) {
	mr.lock.Lock()
	defer mr.lock.Unlock()

	return mr.resolveMountPath(mountID, device, pid, containerID)
}

func (mr *Resolver) syncCacheMiss() {
	mr.procMissStats.Inc()
}

func (mr *Resolver) reSyncCache(mountID uint32, pids []uint32, containerID containerutils.ContainerID, workload *cmodel.CacheEntry) error {
	mr.debugLog.Add(fmt.Sprintf("reSyncCache() [In] mountID = %d, containerID = %s\n", mountID, containerID))

	if workload != nil {
		pids = append(pids, workload.GetPIDs()...)
		mr.debugLog.Add(fmt.Sprintf("reSyncCache() added workload PIDs, total PIDs: %d\n", len(pids)))
	} else if len(containerID) == 0 && !slices.Contains(pids, 1) {
		pids = append(pids, 1)
		mr.debugLog.Add(fmt.Sprintf("reSyncCache() added PID 1\n"))
	}

	if err := mr.syncCache(mountID, pids); err != nil {
		mr.syncCacheMiss()
		mr.debugLog.Add(fmt.Sprintf("reSyncCache() [Out] sync cache failed: %v\n", err))
		return err
	}

	mr.debugLog.Add(fmt.Sprintf("reSyncCache() [Out] sync completed successfully\n"))
	return nil
}

func (mr *Resolver) resolveMountPath(mountID uint32, device uint32, pid uint32, containerID containerutils.ContainerID) (string, model.MountSource, model.MountOrigin, error) {
	mr.debugLog.Add(fmt.Sprintf("resolveMountPath() [In] mountID = %d, device = %d, pid = %d, containerID = %s\n", mountID, device, pid, containerID))

	if _, err := mr.IsMountIDValid(mountID); err != nil {
		mr.debugLog.Add(fmt.Sprintf("resolveMountPath() [Out] invalid mountID: %v\n", err))
		return "", model.MountSourceUnknown, model.MountOriginUnknown, err
	}

	// force a resolution here to make sure the LRU keeps doing its job and doesn't evict important entries
	workload, _ := mr.cgroupsResolver.GetWorkload(containerID)
	mr.debugLog.Add(fmt.Sprintf("resolveMountPath() retrieved workload for containerID = %s\n", containerID))

	path, source, origin, err := mr.getMountPath(mountID, device, pid)
	if err == nil {
		mr.cacheHitsStats.Inc()
		mr.debugLog.Add(fmt.Sprintf("resolveMountPath() [Out] cache hit, path = %s\n", path))
		return path, source, origin, nil
	}
	mr.cacheMissStats.Inc()
	mr.debugLog.Add(fmt.Sprintf("resolveMountPath() cache miss: %v\n", err))

	if !mr.opts.UseProcFS {
		mr.debugLog.Add(fmt.Sprintf("resolveMountPath() [Out] procfs disabled\n"))
		return "", model.MountSourceUnknown, model.MountOriginUnknown, &ErrMountNotFound{MountID: mountID}
	}

	if err := mr.reSyncCache(mountID, []uint32{pid}, containerID, workload); err != nil {
		mr.debugLog.Add(fmt.Sprintf("resolveMountPath() [Out] resync failed: %v\n", err))
		return "", model.MountSourceUnknown, model.MountOriginUnknown, err
	}

	path, source, origin, err = mr.getMountPath(mountID, device, pid)
	if err == nil {
		mr.procHitsStats.Inc()
		mr.debugLog.Add(fmt.Sprintf("resolveMountPath() [Out] procfs hit, path = %s\n", path))
		return path, source, origin, nil
	}
	mr.procMissStats.Inc()
	mr.debugLog.Add(fmt.Sprintf("resolveMountPath() [Out] procfs miss: %v\n", err))

	return "", model.MountSourceUnknown, model.MountOriginUnknown, err
}

// ResolveMount returns the mount
func (mr *Resolver) ResolveMount(mountID uint32, device uint32, pid uint32, containerID containerutils.ContainerID) (*model.Mount, model.MountSource, model.MountOrigin, error) {
	mr.lock.Lock()
	defer mr.lock.Unlock()

	return mr.resolveMount(mountID, device, pid, containerID)
}

func (mr *Resolver) resolveMount(mountID uint32, device uint32, pid uint32, containerID containerutils.ContainerID) (*model.Mount, model.MountSource, model.MountOrigin, error) {
	mr.debugLog.Add(fmt.Sprintf("resolveMount() [In] mountID = %d, device = %d, pid = %d, containerID = %s\n", mountID, device, pid, containerID))

	if _, err := mr.IsMountIDValid(mountID); err != nil {
		mr.debugLog.Add(fmt.Sprintf("resolveMount() [Out] invalid mountID: %v\n", err))
		return nil, model.MountSourceUnknown, model.MountOriginUnknown, err
	}

	// force a resolution here to make sure the LRU keeps doing its job and doesn't evict important entries
	workload, _ := mr.cgroupsResolver.GetWorkload(containerID)
	mr.debugLog.Add(fmt.Sprintf("resolveMount() retrieved workload for containerID = %s\n", containerID))

	mount, source, origin := mr.lookupMount(mountID, device, pid)
	if mount != nil {
		mr.cacheHitsStats.Inc()
		mr.debugLog.Add(fmt.Sprintf("resolveMount() [Out] cache hit, mountID = %d\n", mountID))
		return mount, source, origin, nil
	}
	mr.cacheMissStats.Inc()
	mr.debugLog.Add(fmt.Sprintf("resolveMount() cache miss\n"))

	if !mr.opts.UseProcFS {
		mr.debugLog.Add(fmt.Sprintf("resolveMount() [Out] procfs disabled\n"))
		return nil, model.MountSourceUnknown, model.MountOriginUnknown, &ErrMountNotFound{MountID: mountID}
	}

	if err := mr.reSyncCache(mountID, []uint32{pid}, containerID, workload); err != nil {
		mr.debugLog.Add(fmt.Sprintf("resolveMount() [Out] resync failed: %v\n", err))
		return nil, model.MountSourceUnknown, model.MountOriginUnknown, err
	}

	if mount, ok := mr.mounts.Get(mountID); mount != nil && ok {
		mr.procHitsStats.Inc()
		mr.debugLog.Add(fmt.Sprintf("resolveMount() Global [Out] procfs hit, mountID = %d. \n", mountID))
		return mount, model.MountSourceMountID, mount.Origin, nil
	}

	mr.procMissStats.Inc()
	mr.debugLog.Add(fmt.Sprintf("resolveMount() [Out] procfs miss\n"))

	return nil, model.MountSourceUnknown, model.MountOriginUnknown, &ErrMountNotFound{MountID: mountID}
}

// GetVFSLinkDentryPosition gets VFS link dentry position
func GetVFSLinkDentryPosition(kernelVersion *skernel.Version) uint64 {
	position := uint64(2)

	if kernelVersion.Code != 0 && kernelVersion.Code >= skernel.Kernel5_12 {
		position = 3
	}

	return position
}

// GetVFSMKDirDentryPosition gets VFS MKDir dentry position
func GetVFSMKDirDentryPosition(kernelVersion *skernel.Version) uint64 {
	position := uint64(2)

	if kernelVersion.Code != 0 && kernelVersion.Code >= skernel.Kernel5_12 {
		position = 3
	}

	return position
}

// GetVFSSetxattrDentryPosition gets VFS set xattr dentry position
func GetVFSSetxattrDentryPosition(kernelVersion *skernel.Version) uint64 {
	position := uint64(1)

	if kernelVersion.Code != 0 && kernelVersion.Code >= skernel.Kernel5_12 {
		position = 2
	}

	return position
}

// GetVFSRemovexattrDentryPosition gets VFS remove xattr dentry position
func GetVFSRemovexattrDentryPosition(kernelVersion *skernel.Version) uint64 {
	position := uint64(1)

	if kernelVersion.Code != 0 && kernelVersion.Code >= skernel.Kernel5_12 {
		position = 2
	}

	return position
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

	if err := mr.statsdClient.Count(metrics.MetricMountResolverHits, mr.procHitsStats.Swap(0), []string{metrics.ProcFSTag}, 1.0); err != nil {
		return err
	}

	if err := mr.statsdClient.Count(metrics.MetricMountResolverMiss, mr.procMissStats.Swap(0), []string{metrics.ProcFSTag}, 1.0); err != nil {
		return err
	}

	return mr.statsdClient.Gauge(metrics.MetricMountResolverCacheSize, float64(mr.mounts.Len()), []string{}, 1.0)
}

// ToJSON return a json version of the cache
func (mr *Resolver) ToJSON() ([]byte, error) {
	dump := struct {
		Entries   []json.RawMessage
		Trace     string
		Conflicts string
	}{}

	mr.lock.RLock()
	defer mr.lock.RUnlock()

	for mount := range mr.mounts.ValuesIter() {
		d, err := json.Marshal(mount)
		if err == nil {
			dump.Entries = append(dump.Entries, d)
		}
	}

	dump.Trace = mr.debugLog.Get()
	dump.Conflicts = mr.mountLog.Get()
	return json.Marshal(dump)
}

const (
	// mounts LRU limit: 100000 mounts
	mountsLimit = 100000
	// pidToMounts LRU limits: 1000 pids, and 1000 mounts per pid
	pidLimit          = 1000
	mountsPerPidLimit = 1000
)

// NewResolver instantiates a new mount resolver
func NewResolver(statsdClient statsd.ClientInterface, cgroupsResolver *cgroup.Resolver, opts ResolverOpts, debugLog *debugging.AtomicString) (*Resolver, error) {
	mounts, err := simplelru.NewLRU[uint32, *model.Mount](mountsLimit, nil)
	if err != nil {
		return nil, err
	}

	pidToMounts, err := cache.NewTwoLayersLRU[uint32, uint32, *model.Mount](pidLimit * mountsPerPidLimit)
	if err != nil {
		return nil, err
	}

	mr := &Resolver{
		opts:            opts,
		statsdClient:    statsdClient,
		cgroupsResolver: cgroupsResolver,
		lock:            sync.RWMutex{},
		mounts:          mounts,
		pidToMounts:     pidToMounts,
		cacheHitsStats:  atomic.NewInt64(0),
		procHitsStats:   atomic.NewInt64(0),
		cacheMissStats:  atomic.NewInt64(0),
		procMissStats:   atomic.NewInt64(0),
		debugLog:        debugLog,
		mountLog:        debugging.NewRollingLog(1024 * 1024 * 20),
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

	return mr, nil
}
