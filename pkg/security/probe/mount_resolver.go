// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/gopsutil/process"
	"github.com/hashicorp/golang-lru/simplelru"
	"github.com/moby/sys/mountinfo"
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"

	skernel "github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

var (
	// ErrMountNotFound is used when an unknown mount identifier is found
	ErrMountNotFound = errors.New("unknown mount ID")
)

const (
	deleteDelayTime = 5 * time.Second
)

//MountEventListener events progated to mount resolver listeners
type MountEventListener interface {
	OnMountEventInserted(e *model.MountEvent)
}

func parseGroupID(mnt *mountinfo.Info) (uint32, error) {
	// Has optional fields, which is a space separated list of values.
	// Example: shared:2 master:7
	if len(mnt.Optional) > 0 {
		for _, field := range strings.Split(mnt.Optional, " ") {
			optionSplit := strings.SplitN(field, ":", 2)
			if len(optionSplit) == 2 {
				target, value := optionSplit[0], optionSplit[1]
				if target == "shared" || target == "master" {
					groupID, err := strconv.ParseUint(value, 10, 32)
					return uint32(groupID), err
				}
			}
		}
	}
	return 0, nil
}

// newMountEventFromMountInfo - Creates a new MountEvent from parsed MountInfo data
func newMountEventFromMountInfo(mnt *mountinfo.Info) (*model.MountEvent, error) {
	groupID, err := parseGroupID(mnt)
	if err != nil {
		return nil, err
	}

	// create a MountEvent out of the parsed MountInfo
	return &model.MountEvent{
		ParentMountID: uint32(mnt.Parent),
		MountPointStr: mnt.Mountpoint,
		RootStr:       mnt.Root,
		MountID:       uint32(mnt.ID),
		GroupID:       groupID,
		Device:        uint32(unix.Mkdev(uint32(mnt.Major), uint32(mnt.Minor))),
		FSType:        mnt.FSType,
	}, nil
}

type deleteRequest struct {
	mount     *model.MountEvent
	timeoutAt time.Time
}

// MountResolver represents a cache for mountpoints and the corresponding file systems
type MountResolver struct {
	probe            *Probe
	lock             sync.RWMutex
	mounts           map[uint32]*model.MountEvent
	devices          map[uint32]map[uint32]*model.MountEvent
	deleteQueue      []deleteRequest
	overlayPathCache *simplelru.LRU
	parentPathCache  *simplelru.LRU
	listerners       []MountEventListener
}

// SyncCache - Snapshots the current mount points of the system by reading through /proc/[pid]/mountinfo.
func (mr *MountResolver) SyncCache(proc *process.Process) error {
	mr.lock.Lock()
	defer mr.lock.Unlock()

	mnts, err := kernel.ParseMountInfoFile(proc.Pid)
	if err != nil {
		pErr, ok := err.(*os.PathError)
		if !ok {
			return err
		}
		return pErr
	}

	for _, mnt := range mnts {
		e, err := newMountEventFromMountInfo(mnt)
		if err != nil {
			return err
		}

		if _, exists := mr.mounts[e.MountID]; exists {
			continue
		}
		mr.insert(e)
	}

	return nil
}

func (mr *MountResolver) deleteChildren(parent *model.MountEvent) {
	for _, mount := range mr.mounts {
		if mount.ParentMountID == parent.MountID {
			if _, exists := mr.mounts[mount.MountID]; exists {
				mr.delete(mount)
			}
		}
	}
}

// deleteDevice deletes MountEvent sharing the same device id for overlay fs mount
func (mr *MountResolver) deleteDevice(mount *model.MountEvent) {
	if !mount.IsOverlayFS() {
		return
	}

	for _, deviceMount := range mr.devices[mount.Device] {
		if mount.Device == deviceMount.Device && mount.MountID != deviceMount.MountID {
			mr.delete(deviceMount)
		}
	}
}

func (mr *MountResolver) delete(mount *model.MountEvent) {
	delete(mr.mounts, mount.MountID)

	mounts, exists := mr.devices[mount.Device]
	if exists {
		delete(mounts, mount.MountID)
	}

	mr.deleteChildren(mount)
	mr.deleteDevice(mount)
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

// GetFilesystem returns the name of the filesystem
func (mr *MountResolver) GetFilesystem(mountID uint32) string {
	mr.lock.RLock()
	defer mr.lock.RUnlock()

	mount, exists := mr.mounts[mountID]
	if !exists {
		return ""
	}

	return mount.GetFSType()
}

// IsOverlayFS returns the type of a mountID
func (mr *MountResolver) IsOverlayFS(mountID uint32) bool {
	mr.lock.RLock()
	defer mr.lock.RUnlock()

	mount, exists := mr.mounts[mountID]
	if !exists {
		return false
	}

	return mount.IsOverlayFS()
}

// Insert a new mount point in the cache
func (mr *MountResolver) Insert(e model.MountEvent) error {
	mr.lock.Lock()
	defer mr.lock.Unlock()

	if e.MountPointPathResolutionError != nil || e.RootPathResolutionError != nil {
		// do not insert an invalid value
		return errors.Errorf("couldn't insert mount_id %d: mount_point_error:%v root_error:%v", e.MountID, e.MountPointPathResolutionError, e.RootPathResolutionError)
	}

	mr.insert(&e)

	return nil
}

func (mr *MountResolver) insert(e *model.MountEvent) {
	// umount the previous one if exists
	if prev, ok := mr.mounts[e.MountID]; ok {
		mr.delete(prev)
	}

	// Retrieve the parent paths and strip it from the event
	p, ok := mr.mounts[e.ParentMountID]
	if ok {
		prefix := mr.getParentPath(p.MountID)
		if len(prefix) > 0 && prefix != "/" {
			e.MountPointStr = strings.TrimPrefix(e.MountPointStr, prefix)
		}
	}

	deviceMounts := mr.devices[e.Device]
	if deviceMounts == nil {
		deviceMounts = make(map[uint32]*model.MountEvent)
		mr.devices[e.Device] = deviceMounts
	}
	deviceMounts[e.MountID] = e

	mr.mounts[e.MountID] = e

	mr.NotifyNewInsert(e)
}

// NotifyNewInsert notifies all the listeners
func (mr *MountResolver) NotifyNewInsert(e *model.MountEvent) {
	for _, listener := range mr.listerners {
		listener.OnMountEventInserted(e)
	}
}

func (mr *MountResolver) _getParentPath(mountID uint32, cache map[uint32]bool) string {
	mount, exists := mr.mounts[mountID]
	if !exists {
		return ""
	}

	mountPointStr := mount.MountPointStr

	if _, exists := cache[mountID]; exists {
		return ""
	}
	cache[mountID] = true

	if mount.ParentMountID != 0 {
		p := mr._getParentPath(mount.ParentMountID, cache)
		if p == "" {
			return mountPointStr
		}

		if p != "/" && !strings.HasPrefix(mount.MountPointStr, p) {
			mountPointStr = p + mount.MountPointStr
		}
	}

	return mountPointStr
}

func (mr *MountResolver) getParentPath(mountID uint32) string {
	if entry, found := mr.parentPathCache.Get(mountID); found {
		return entry.(string)
	}

	path := mr._getParentPath(mountID, map[uint32]bool{})
	mr.parentPathCache.Add(mountID, path)
	return path
}

func (mr *MountResolver) _getAncestor(mount *model.MountEvent, cache map[uint32]bool) *model.MountEvent {
	if _, exists := cache[mount.MountID]; exists {
		return nil
	}
	cache[mount.MountID] = true

	parent, ok := mr.mounts[mount.ParentMountID]
	if !ok {
		return nil
	}

	if grandParent := mr._getAncestor(parent, cache); grandParent != nil {
		return grandParent
	}

	return parent
}

func (mr *MountResolver) getAncestor(mount *model.MountEvent) *model.MountEvent {
	return mr._getAncestor(mount, map[uint32]bool{})
}

// getOverlayPath uses deviceID to find overlay path
func (mr *MountResolver) getOverlayPath(mount *model.MountEvent) string {
	if entry, found := mr.overlayPathCache.Get(mount.MountID); found {
		return entry.(string)
	}

	if ancestor := mr.getAncestor(mount); ancestor != nil {
		mount = ancestor
	}

	for _, deviceMount := range mr.devices[mount.Device] {
		if mount.MountID != deviceMount.MountID && deviceMount.IsOverlayFS() {
			if p := mr.getParentPath(deviceMount.MountID); p != "" {
				mr.overlayPathCache.Add(mount.MountID, p)
				return p
			}
		}
	}

	return ""
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

		// clear cache anyway
		mr.parentPathCache.Remove(req.mount.MountID)
		mr.overlayPathCache.Remove(req.mount.MountID)

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

// GetMountPath returns the path of a mount identified by its mount ID. The first path is the container mount path if
// it exists, the second parameter is the mount point path, and the third parameter is the root path.
func (mr *MountResolver) GetMountPath(mountID uint32) (string, string, string, error) {
	if mountID == 0 {
		return "", "", "", nil
	}
	mr.lock.RLock()
	defer mr.lock.RUnlock()

	mount, ok := mr.mounts[mountID]
	if !ok {
		return "", "", "", nil
	}

	return mr.getOverlayPath(mount), mr.getParentPath(mountID), mount.RootStr, nil
}

func getMountIDOffset(probe *Probe) uint64 {
	offset := uint64(284)

	switch {
	case probe.kernelVersion.IsSuseKernel() || probe.kernelVersion.Code >= skernel.Kernel5_12:
		offset = 292
	case probe.kernelVersion.Code != 0 && probe.kernelVersion.Code < skernel.Kernel4_13:
		offset = 268
	}

	return offset
}

func getVFSLinkDentryPosition(probe *Probe) uint64 {
	position := uint64(2)

	if probe.kernelVersion.Code != 0 && probe.kernelVersion.Code >= skernel.Kernel5_12 {
		position = 3
	}

	return position
}

func getVFSMKDirDentryPosition(probe *Probe) uint64 {
	position := uint64(2)

	if probe.kernelVersion.Code != 0 && probe.kernelVersion.Code >= skernel.Kernel5_12 {
		position = 3
	}

	return position
}

func getVFSLinkTargetDentryPosition(probe *Probe) uint64 {
	position := uint64(3)

	if probe.kernelVersion.Code != 0 && probe.kernelVersion.Code >= skernel.Kernel5_12 {
		position = 4
	}

	return position
}

func getVFSSetxattrDentryPosition(probe *Probe) uint64 {
	position := uint64(1)

	if probe.kernelVersion.Code != 0 && probe.kernelVersion.Code >= skernel.Kernel5_12 {
		position = 2
	}

	return position
}

func getVFSRemovexattrDentryPosition(probe *Probe) uint64 {
	position := uint64(1)

	if probe.kernelVersion.Code != 0 && probe.kernelVersion.Code >= skernel.Kernel5_12 {
		position = 2
	}

	return position
}

func getVFSRenameInputType(probe *Probe) uint64 {
	inputType := uint64(1)

	if probe.kernelVersion.Code != 0 && probe.kernelVersion.Code >= skernel.Kernel5_12 {
		inputType = 2
	}

	return inputType
}

// NewMountResolver instantiates a new mount resolver
func NewMountResolver(probe *Probe) (*MountResolver, error) {
	overlayPathCache, err := simplelru.NewLRU(256, nil)
	if err != nil {
		return nil, err
	}

	parentPathCache, err := simplelru.NewLRU(256, nil)
	if err != nil {
		return nil, err
	}

	return &MountResolver{
		probe:            probe,
		lock:             sync.RWMutex{},
		devices:          make(map[uint32]map[uint32]*model.MountEvent),
		mounts:           make(map[uint32]*model.MountEvent),
		overlayPathCache: overlayPathCache,
		parentPathCache:  parentPathCache,
	}, nil
}
