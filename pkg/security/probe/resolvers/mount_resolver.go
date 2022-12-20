// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package resolvers

import (
	"context"
	"errors"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/golang-lru/v2/simplelru"
	"github.com/moby/sys/mountinfo"
	"go.uber.org/atomic"
	"golang.org/x/sys/unix"

	skernel "github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-go/v5/statsd"
)

var (
	// ErrMountNotFound is used when an unknown mount identifier is found
	ErrMountNotFound = errors.New("unknown mount ID")
	// ErrMountUndefined is used when a mount identifier is undefined
	ErrMountUndefined = errors.New("undefined mountID")
	// ErrMountLoop is returned when there is a resolution loop
	ErrMountLoop = errors.New("mount resolution loop")
	// ErrMountPathEmpty is returned when the resolved mount path is empty
	ErrMountPathEmpty = errors.New("mount resolution return empty path")
	// ErrMountKernelID
	ErrMountKernelID = errors.New("not a critical error")
)

const (
	deleteDelayTime = 5 * time.Second
)

func parseGroupID(mnt *mountinfo.Info) (uint32, error) {
	// Has optional fields, which is a space separated list of values.
	// Example: shared:2 master:7
	if len(mnt.Optional) > 0 {
		for _, field := range strings.Split(mnt.Optional, " ") {
			target, value, found := strings.Cut(field, ":")
			if found {
				if target == "shared" || target == "master" {
					groupID, err := strconv.ParseUint(value, 10, 32)
					return uint32(groupID), err
				}
			}
		}
	}
	return 0, nil
}

// newMountFromMountInfo - Creates a new Mount from parsed MountInfo data
func newMountFromMountInfo(mnt *mountinfo.Info) *model.Mount {
	// groupID is not use for the path resolution, don't make it critical
	groupID, _ := parseGroupID(mnt)

	// create a Mount out of the parsed MountInfo
	return &model.Mount{
		MountID:       uint32(mnt.ID),
		GroupID:       groupID,
		Device:        uint32(unix.Mkdev(uint32(mnt.Major), uint32(mnt.Minor))),
		ParentMountID: uint32(mnt.Parent),
		FSType:        mnt.FSType,
		MountPointStr: mnt.Mountpoint,
		Path:          mnt.Mountpoint,
		RootStr:       mnt.Root,
	}
}

type deleteRequest struct {
	mount     *model.Mount
	timeoutAt time.Time
}

// MountResolverOpts defines mount resolver options
type MountResolverOpts struct {
	UseProcFS bool
}

// MountResolver represents a cache for mountpoints and the corresponding file systems
type MountResolver struct {
	opts            MountResolverOpts
	cgroupsResolver *CgroupsResolver
	statsdClient    statsd.ClientInterface
	lock            sync.RWMutex
	mounts          map[uint32]*model.Mount
	devices         map[uint32]map[uint32]*model.Mount
	deleteQueue     []deleteRequest
	minMountID      uint32
	redemption      *simplelru.LRU[uint32, *model.Mount]

	// stats
	cacheHitsStats *atomic.Int64
	cacheMissStats *atomic.Int64
	procHitsStats  *atomic.Int64
	procMissStats  *atomic.Int64
}

// IsMountIDValid returns whether the mountID is valid
func (mr *MountResolver) IsMountIDValid(mountID uint32) (bool, error) {
	if mountID == 0 {
		return false, ErrMountUndefined
	}

	if mountID < mr.minMountID {
		return false, ErrMountKernelID
	}

	return true, nil
}

// SyncCache - Snapshots the current mount points of the system by reading through /proc/[pid]/mountinfo.
func (mr *MountResolver) SyncCache(pid uint32) error {
	mr.lock.Lock()
	defer mr.lock.Unlock()

	if err := mr.syncCache(pid); err != nil {
		return err
	}

	// store the minimal mount ID found to use it a reference
	if pid == 1 {
		for mountID := range mr.mounts {
			if mr.minMountID == 0 || mr.minMountID > mountID {
				mr.minMountID = mountID
			}
		}
	}

	return nil
}

func (mr *MountResolver) syncCache(pid uint32) error {
	mnts, err := kernel.ParseMountInfoFile(int32(pid))
	if err != nil {
		mr.cgroupsResolver.DelByPID1(pid)
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

func (mr *MountResolver) finalizeChildren(parent *model.Mount) {
	for _, mount := range mr.mounts {
		if mount.ParentMountID == parent.MountID {
			if _, exists := mr.mounts[mount.MountID]; exists {
				mr.finalize(mount)
			}
		}
	}
}

// finalizeDevice deletes Mount sharing the same device id for overlay fs mount
func (mr *MountResolver) finalizeDevice(mount *model.Mount) {
	if !mount.IsOverlayFS() {
		return
	}

	for _, deviceMount := range mr.devices[mount.Device] {
		if mount.Device == deviceMount.Device && mount.MountID != deviceMount.MountID {
			mr.finalize(deviceMount)
		}
	}
}

func (mr *MountResolver) finalize(mount *model.Mount) {
	delete(mr.mounts, mount.MountID)

	mounts, exists := mr.devices[mount.Device]
	if exists {
		delete(mounts, mount.MountID)
	}

	mr.finalizeChildren(mount)
	mr.finalizeDevice(mount)
}

func (mr *MountResolver) delete(mount *model.Mount) {
	if m, exists := mr.mounts[mount.MountID]; exists {
		mr.redemption.Add(mount.MountID, m)
	}
}

// Delete a mount from the cache
func (mr *MountResolver) Delete(mountID uint32) error {
	mr.lock.Lock()
	defer mr.lock.Unlock()

	mount, exists := mr.mounts[mountID]
	if !exists {
		return ErrMountNotFound
	}

	mr.deleteQueue = append(mr.deleteQueue, deleteRequest{mount: mount, timeoutAt: time.Now().Add(deleteDelayTime)})

	return nil
}

// ResolveFilesystem returns the name of the filesystem
func (mr *MountResolver) ResolveFilesystem(mountID, pid uint32, containerID string) (string, error) {
	mr.lock.Lock()
	defer mr.lock.Unlock()

	mount, err := mr.resolveMount(mountID, pid, containerID)
	if err != nil {
		return "", err
	}

	return mount.GetFSType(), nil
}

// Insert a new mount point in the cache
func (mr *MountResolver) Insert(e model.Mount, pid uint32, containerID string) error {
	if e.MountID == 0 {
		return ErrMountUndefined
	}

	mr.lock.Lock()
	defer mr.lock.Unlock()

	mr.insert(&e)

	return nil
}

func (mr *MountResolver) insert(m *model.Mount) {
	// umount the previous one if exists
	if prev, ok := mr.mounts[m.MountID]; ok {
		// if present in the redemption that the evict function that will remove the entry
		if present := mr.redemption.Remove(prev.MountID); !present {
			mr.finalize(prev)
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

func (mr *MountResolver) _getMountPath(mountID uint32, cache map[uint32]bool) (string, error) {
	if _, err := mr.IsMountIDValid(mountID); err != nil {
		return "", err
	}

	mount, exists := mr.mounts[mountID]
	if !exists {
		return "", ErrMountNotFound
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

	if mount.ParentMountID == 0 {
		return "", ErrMountUndefined
	}

	parentMountPath, err := mr._getMountPath(mount.ParentMountID, cache)
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

func (mr *MountResolver) getMountPath(mountID uint32) (string, error) {
	return mr._getMountPath(mountID, map[uint32]bool{})
}

func (mr *MountResolver) dequeue(now time.Time) {
	mr.lock.Lock()

	var i int
	var req deleteRequest

	for i != len(mr.deleteQueue) {
		req = mr.deleteQueue[i]
		if req.timeoutAt.After(now) {
			break
		}

		// check if not already replaced
		if prev := mr.mounts[req.mount.MountID]; prev == req.mount {
			mr.delete(req.mount)
		}

		i++
	}

	if i >= len(mr.deleteQueue) {
		mr.deleteQueue = mr.deleteQueue[0:0]
	} else if i > 0 {
		mr.deleteQueue = mr.deleteQueue[i:]
	}

	mr.lock.Unlock()
}

// Start starts the resolver
func (mr *MountResolver) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case now := <-ticker.C:
				mr.dequeue(now)
			case <-ctx.Done():
				return
			}
		}
	}()
}

// ResolveMountPath returns the root of a mount identified by its mount ID.
func (mr *MountResolver) ResolveMountRoot(mountID, pid uint32, containerID string) (string, error) {
	mr.lock.Lock()
	defer mr.lock.Unlock()

	return mr.resolveMountRoot(mountID, pid, containerID)
}

func (mr *MountResolver) resolveMountRoot(mountID, pid uint32, containerID string) (string, error) {
	mount, err := mr.resolveMount(mountID, pid, containerID)
	if err != nil {
		return "", err
	}
	return mount.RootStr, nil
}

// ResolveMountRoot returns the root of a mount identified by its mount ID.
func (mr *MountResolver) ResolveMountPath(mountID, pid uint32, containerID string) (string, error) {
	mr.lock.Lock()
	defer mr.lock.Unlock()

	return mr.resolveMountPath(mountID, pid, containerID)
}

func (mr *MountResolver) resolveMountPath(mountID, pid uint32, containerID string) (string, error) {
	if _, err := mr.IsMountIDValid(mountID); err != nil {
		return "", err
	}
	// force pid1 resolution here to keep the LRU doing his job and not evicting important entries
	if pid1, exists := mr.cgroupsResolver.GetPID1(containerID); exists {
		pid = pid1
	}

	path, err := mr.getMountPath(mountID)
	if err == nil {
		mr.cacheHitsStats.Inc()

		// touch the redemption entry to maintain the entry
		_, _ = mr.redemption.Get(mountID)

		return path, nil
	}
	mr.cacheMissStats.Inc()

	if !mr.opts.UseProcFS {
		return "", ErrMountNotFound
	}

	if err := mr.syncCache(pid); err != nil {
		mr.procMissStats.Inc()
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
func (mr *MountResolver) ResolveMount(mountID, pid uint32, containerID string) (*model.Mount, error) {
	mr.lock.Lock()
	defer mr.lock.Unlock()

	return mr.resolveMount(mountID, pid, containerID)
}

func (mr *MountResolver) resolveMount(mountID, pid uint32, containerID string) (*model.Mount, error) {
	if _, err := mr.IsMountIDValid(mountID); err != nil {
		return nil, err
	}

	// force pid1 resolution here to keep the LRU doing his job and not evicting important entries
	if pid1, exists := mr.cgroupsResolver.GetPID1(containerID); exists {
		pid = pid1
	}

	mount, exists := mr.mounts[mountID]
	if exists {
		mr.cacheHitsStats.Inc()

		// touch the redemption entry to maintain the entry
		_, _ = mr.redemption.Get(mountID)

		return mount, nil
	}
	mr.cacheMissStats.Inc()

	if !mr.opts.UseProcFS {
		return nil, ErrMountNotFound
	}

	if err := mr.syncCache(pid); err != nil {
		mr.procMissStats.Inc()
		return nil, err
	}
	mount, exists = mr.mounts[mountID]
	if exists {
		mr.procMissStats.Inc()
		return mount, nil
	}
	mr.procMissStats.Inc()

	return nil, ErrMountNotFound
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
func (mr *MountResolver) SendStats() error {
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

// NewMountResolver instantiates a new mount resolver
func NewMountResolver(statsdClient statsd.ClientInterface, cgroupsResolver *CgroupsResolver, opts MountResolverOpts) (*MountResolver, error) {
	mr := &MountResolver{
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

	redemption, err := simplelru.NewLRU(1024, func(mountID uint32, mount *model.Mount) {
		mr.finalize(mount)
	})
	if err != nil {
		return nil, err
	}
	mr.redemption = redemption

	return mr, nil
}
