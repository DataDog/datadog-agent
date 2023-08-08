// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package mount

import (
	"path"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/golang-lru/v2/simplelru"
	"github.com/moby/sys/mountinfo"
	"go.uber.org/atomic"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-go/v5/statsd"

	skernel "github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

const (
	deleteDelayTime                      = 5 * time.Second
	numAllowedMountIDsToResolvePerPeriod = 75
	fallbackLimiterPeriod                = 5 * time.Second
	redemptionTime                       = 5 * time.Second
)

type redemptionEntry struct {
	mount      *model.Mount
	insertedAt time.Time
}

// newMountFromMountInfo - Creates a new Mount from parsed MountInfo data
func newMountFromMountInfo(mnt *mountinfo.Info) *model.Mount {
	// create a Mount out of the parsed MountInfo
	return &model.Mount{
		MountID: uint32(mnt.ID),
		Device:  uint32(unix.Mkdev(uint32(mnt.Major), uint32(mnt.Minor))),
		ParentPathKey: model.PathKey{
			MountID: uint32(mnt.Parent),
		},
		FSType:        mnt.FSType,
		MountPointStr: mnt.Mountpoint,
		Path:          mnt.Mountpoint,
		RootStr:       mnt.Root,
	}
}

// ResolverOpts defines mount resolver options
type ResolverOpts struct {
	UseProcFS     bool
	UseRedemption bool
}

// Resolver represents a cache for mountpoints and the corresponding file systems
type Resolver struct {
	opts            ResolverOpts
	cgroupsResolver *cgroup.Resolver
	statsdClient    statsd.ClientInterface
	lock            sync.RWMutex
	mounts          map[uint32]*model.Mount
	devices         map[uint32]map[uint32]*model.Mount
	minMountID      uint32
	redemption      *simplelru.LRU[uint32, *redemptionEntry]
	fallbackLimiter *utils.Limiter[uint32]

	// stats
	cacheHitsStats *atomic.Int64
	cacheMissStats *atomic.Int64
	procHitsStats  *atomic.Int64
	procMissStats  *atomic.Int64
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

// SyncCache - Snapshots the current mount points of the system by reading through /proc/[pid]/mountinfo.
func (mr *Resolver) SyncCache(pid uint32) error {
	mr.lock.Lock()
	defer mr.lock.Unlock()

	err := mr.syncCache(pid)

	// store the minimal mount ID found to use it as a reference
	if pid == 1 {
		for mountID := range mr.mounts {
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
		if _, exists := mr.mounts[uint32(mnt.ID)]; exists {
			continue
		}

		m := newMountFromMountInfo(mnt)
		mr.insert(m)
	}

	return nil
}

// syncCache update cache with the first working pid
func (mr *Resolver) syncCache(pids ...uint32) error {
	var err error

	for _, pid := range pids {
		if err = mr.syncPid(pid); err == nil {
			return nil
		}
	}

	return err
}

func (mr *Resolver) finalize(first *model.Mount) {
	openQueue := make([]*model.Mount, 0, len(mr.mounts))
	openQueue = append(openQueue, first)

	for len(openQueue) != 0 {
		curr, rest := openQueue[len(openQueue)-1], openQueue[:len(openQueue)-1]
		openQueue = rest

		// pre-work
		delete(mr.mounts, curr.MountID)
		mounts, exists := mr.devices[curr.Device]
		if exists {
			delete(mounts, curr.MountID)
		}

		// finalize children
		for _, child := range mr.mounts {
			if child.ParentPathKey.MountID == curr.MountID {
				openQueue = append(openQueue, child)
			}
		}

		// finalize device
		if !curr.IsOverlayFS() {
			continue
		}

		for _, deviceMount := range mr.devices[curr.Device] {
			if curr.Device == deviceMount.Device && curr.MountID != deviceMount.MountID {
				openQueue = append(openQueue, deviceMount)
			}
		}
	}
}

// Delete a mount from the cache
func (mr *Resolver) Delete(mountID uint32) error {
	mr.lock.Lock()
	defer mr.lock.Unlock()

	if m, exists := mr.mounts[mountID]; exists {
		delete(mr.mounts, mountID)

		// keep the entry in the redemption LRU so that in case of event ordering issue(perf buffer)
		if mr.opts.UseRedemption {
			entry := redemptionEntry{
				mount:      m,
				insertedAt: time.Now(),
			}
			mr.redemption.Add(mountID, &entry)
		} else {
			mr.finalize(m)
		}

	} else {
		return &ErrMountNotFound{MountID: mountID}
	}

	return nil
}

// ResolveFilesystem returns the name of the filesystem
func (mr *Resolver) ResolveFilesystem(mountID, pid uint32, containerID string) (string, error) {
	mr.lock.Lock()
	defer mr.lock.Unlock()

	mount, err := mr.resolveMount(mountID, containerID, pid)
	if err != nil {
		return model.UnknownFS, err
	}

	return mount.GetFSType(), nil
}

// Insert a new mount point in the cache
func (mr *Resolver) Insert(e model.Mount) error {
	if e.MountID == 0 {
		return ErrMountUndefined
	}

	mr.lock.Lock()
	defer mr.lock.Unlock()

	mr.insert(&e)

	return nil
}

func (mr *Resolver) insert(m *model.Mount) {
	// umount the previous one if exists
	if prev, ok := mr.mounts[m.MountID]; ok {
		mr.finalize(prev)
	} else if mr.opts.UseRedemption {
		if prev, ok := mr.redemption.Get(m.MountID); ok {
			mr.redemption.Remove(m.MountID)
			mr.finalize(prev.mount)
		}
	}

	// if we're inserting a mountpoint from a kernel event (!= procfs) that isn't the root fs
	// then remove the leading slash from the mountpoint
	if len(m.Path) == 0 && m.MountPointStr != "/" {
		m.MountPointStr = strings.TrimPrefix(m.MountPointStr, "/")
	}

	deviceMounts := mr.devices[m.Device]
	if deviceMounts == nil {
		deviceMounts = make(map[uint32]*model.Mount)
		mr.devices[m.Device] = deviceMounts
	}
	deviceMounts[m.MountID] = m
	mr.mounts[m.MountID] = m

	if mr.minMountID > m.MountID {
		mr.minMountID = m.MountID
	}
}

func (mr *Resolver) getFromRedemption(mountID uint32) *model.Mount {
	entry, exists := mr.redemption.Get(mountID)
	if !exists || time.Now().Sub(entry.insertedAt) > redemptionTime {
		return nil
	}
	return entry.mount
}

func (mr *Resolver) _getMountPath(mountID uint32, cache map[uint32]bool) (string, error) {
	if _, err := mr.IsMountIDValid(mountID); err != nil {
		return "", err
	}

	mount := mr.mounts[mountID]
	if mount == nil {
		if mr.opts.UseRedemption {
			if mount = mr.getFromRedemption(mountID); mount == nil {
				return "", &ErrMountNotFound{MountID: mountID}
			}
		} else {
			return "", &ErrMountNotFound{MountID: mountID}
		}
	}

	if len(mount.Path) > 0 {
		return mount.Path, nil
	}

	mountPointStr := mount.MountPointStr
	if mountPointStr == "/" {
		return mountPointStr, nil
	}

	// avoid infinite loop
	if _, exists := cache[mountID]; exists {
		return "", ErrMountLoop
	}
	cache[mountID] = true

	if mount.ParentPathKey.MountID == 0 {
		return "", ErrMountUndefined
	}

	parentMountPath, err := mr._getMountPath(mount.ParentPathKey.MountID, cache)
	if err != nil {
		return "", err
	}
	mountPointStr = path.Join(parentMountPath, mountPointStr)

	if len(mountPointStr) == 0 {
		return "", ErrMountPathEmpty
	}

	mount.Path = mountPointStr

	return mountPointStr, nil
}

func (mr *Resolver) getMountPath(mountID uint32) (string, error) {
	return mr._getMountPath(mountID, map[uint32]bool{})
}

// ResolveMountPath returns the root of a mount identified by its mount ID.
func (mr *Resolver) ResolveMountRoot(mountID, pid uint32, containerID string) (string, error) {
	mr.lock.Lock()
	defer mr.lock.Unlock()

	return mr.resolveMountRoot(mountID, pid, containerID)
}

func (mr *Resolver) resolveMountRoot(mountID, pid uint32, containerID string) (string, error) {
	mount, err := mr.resolveMount(mountID, containerID, pid)
	if err != nil {
		return "", err
	}
	return mount.RootStr, nil
}

// ResolveMountRoot returns the root of a mount identified by its mount ID.
func (mr *Resolver) ResolveMountPath(mountID, pid uint32, containerID string) (string, error) {
	mr.lock.Lock()
	defer mr.lock.Unlock()

	return mr.resolveMountPath(mountID, containerID, pid)
}

func (mr *Resolver) syncCacheMiss(mountID uint32) {
	mr.procMissStats.Inc()
}

func (mr *Resolver) resolveMountPath(mountID uint32, containerID string, pid uint32) (string, error) {
	if _, err := mr.IsMountIDValid(mountID); err != nil {
		return "", err
	}

	pids := []uint32{pid}

	// force a resolution here to make sure the LRU keeps doing its job and doesn't evict important entries
	workload, workloadExists := mr.cgroupsResolver.GetWorkload(containerID)

	path, err := mr.getMountPath(mountID)
	if err == nil {
		mr.cacheHitsStats.Inc()
		return path, nil
	}
	mr.cacheMissStats.Inc()

	if !mr.opts.UseProcFS {
		return "", &ErrMountNotFound{MountID: mountID}
	}

	if !mr.fallbackLimiter.Allow(mountID) {
		return "", &ErrMountNotFound{MountID: mountID}
	}

	if workloadExists {
		pids = append(pids, workload.GetPIDs()...)
	} else if len(containerID) == 0 && pid != 1 {
		pids = append(pids, 1)
	}

	if err := mr.syncCache(pids...); err != nil {
		mr.syncCacheMiss(mountID)
		return "", err
	}

	path, err = mr.getMountPath(mountID)
	if err == nil {
		mr.procHitsStats.Inc()
		return path, nil
	}
	mr.procMissStats.Inc()

	return "", err
}

// ResolveMount returns the mount
func (mr *Resolver) ResolveMount(mountID, pid uint32, containerID string) (*model.Mount, error) {
	mr.lock.Lock()
	defer mr.lock.Unlock()

	return mr.resolveMount(mountID, containerID, pid)
}

func (mr *Resolver) resolveMount(mountID uint32, containerID string, pids ...uint32) (*model.Mount, error) {
	if _, err := mr.IsMountIDValid(mountID); err != nil {
		return nil, err
	}

	// force a resolution here to make sure the LRU keeps doing its job and doesn't evict important entries
	workload, workloadExists := mr.cgroupsResolver.GetWorkload(containerID)

	mount := mr.mounts[mountID]
	if mount == nil && mr.opts.UseRedemption {
		mount = mr.getFromRedemption(mountID)
	}
	if mount != nil {
		mr.cacheHitsStats.Inc()
		return mount, nil
	}
	mr.cacheMissStats.Inc()

	if !mr.opts.UseProcFS {
		return nil, &ErrMountNotFound{MountID: mountID}
	}

	if !mr.fallbackLimiter.Allow(mountID) {
		return nil, &ErrMountNotFound{MountID: mountID}
	}

	if workloadExists {
		pids = append(pids, workload.GetPIDs()...)
	} else if len(containerID) == 0 {
		pids = append(pids, 1)
	}

	if err := mr.syncCache(pids...); err != nil {
		mr.syncCacheMiss(mountID)
		return nil, err
	}

	mount = mr.mounts[mountID]
	if mount != nil {
		mr.procHitsStats.Inc()
		return mount, nil
	}
	mr.procMissStats.Inc()

	return nil, &ErrMountNotFound{MountID: mountID}
}

// GetMountIDOffset returns the mount id offset
func GetMountIDOffset(kernelVersion *skernel.Version) uint64 {
	offset := uint64(284)

	switch {
	case kernelVersion.IsSuseKernel() || kernelVersion.Code >= skernel.Kernel5_12:
		offset = 292
	case kernelVersion.Code != 0 && kernelVersion.Code < skernel.Kernel4_13:
		offset = 268
	}

	return offset
}

func GetVFSLinkDentryPosition(kernelVersion *skernel.Version) uint64 {
	position := uint64(2)

	if kernelVersion.Code != 0 && kernelVersion.Code >= skernel.Kernel5_12 {
		position = 3
	}

	return position
}

func GetVFSMKDirDentryPosition(kernelVersion *skernel.Version) uint64 {
	position := uint64(2)

	if kernelVersion.Code != 0 && kernelVersion.Code >= skernel.Kernel5_12 {
		position = 3
	}

	return position
}

func GetVFSLinkTargetDentryPosition(kernelVersion *skernel.Version) uint64 {
	position := uint64(3)

	if kernelVersion.Code != 0 && kernelVersion.Code >= skernel.Kernel5_12 {
		position = 4
	}

	return position
}

func GetVFSSetxattrDentryPosition(kernelVersion *skernel.Version) uint64 {
	position := uint64(1)

	if kernelVersion.Code != 0 && kernelVersion.Code >= skernel.Kernel5_12 {
		position = 2
	}

	return position
}

func GetVFSRemovexattrDentryPosition(kernelVersion *skernel.Version) uint64 {
	position := uint64(1)

	if kernelVersion.Code != 0 && kernelVersion.Code >= skernel.Kernel5_12 {
		position = 2
	}

	return position
}

func GetVFSRenameInputType(kernelVersion *skernel.Version) uint64 {
	inputType := uint64(1)

	if kernelVersion.Code != 0 && kernelVersion.Code >= skernel.Kernel5_12 {
		inputType = 2
	}

	return inputType
}

// SendStats sends metrics about the current state of the namespace resolver
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

	return mr.statsdClient.Gauge(metrics.MetricMountResolverCacheSize, float64(len(mr.mounts)), []string{}, 1.0)
}

// NewResolver instantiates a new mount resolver
func NewResolver(statsdClient statsd.ClientInterface, cgroupsResolver *cgroup.Resolver, opts ResolverOpts) (*Resolver, error) {
	mr := &Resolver{
		opts:            opts,
		statsdClient:    statsdClient,
		cgroupsResolver: cgroupsResolver,
		lock:            sync.RWMutex{},
		devices:         make(map[uint32]map[uint32]*model.Mount),
		mounts:          make(map[uint32]*model.Mount),
		cacheHitsStats:  atomic.NewInt64(0),
		procHitsStats:   atomic.NewInt64(0),
		cacheMissStats:  atomic.NewInt64(0),
		procMissStats:   atomic.NewInt64(0),
	}

	if opts.UseRedemption {
		redemption, err := simplelru.NewLRU(1024, func(mountID uint32, entry *redemptionEntry) {
			mr.finalize(entry.mount)
		})
		if err != nil {
			return nil, err
		}
		mr.redemption = redemption
	}

	// create a rate limiter that allows for 64 mount IDs
	limiter, err := utils.NewLimiter[uint32](64, numAllowedMountIDsToResolvePerPeriod, fallbackLimiterPeriod)
	if err != nil {
		return nil, err
	}
	mr.fallbackLimiter = limiter

	return mr, nil
}
