// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probe

import (
	"context"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/gopsutil/process"
	"github.com/moby/sys/mountinfo"
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

var (
	// ErrMountNotFound is used when an unknown mount identifier is found
	ErrMountNotFound = errors.New("unknown mount ID")
)

const (
	deleteDelayTime = 5 * time.Second
)

// newMountEventFromMountInfo - Creates a new MountEvent from parsed MountInfo data
func newMountEventFromMountInfo(mnt *mountinfo.Info) (*model.MountEvent, error) {
	var err error
	var groupID uint64

	// Has optional fields, which is a space separated list of values.
	// Example: shared:2 master:7
	if len(mnt.Optional) > 0 {
		for _, field := range strings.Split(mnt.Optional, ",") {
			optionSplit := strings.SplitN(field, ":", 2)
			if len(optionSplit) == 2 {
				target, value := optionSplit[0], optionSplit[1]
				if target == "shared" || target == "master" {
					if groupID, err = strconv.ParseUint(value, 10, 64); err != nil {
						return nil, err
					}
				}
			}
		}
	}

	// create a MountEvent out of the parsed MountInfo
	return &model.MountEvent{
		ParentMountID: uint32(mnt.Parent),
		MountPointStr: mnt.Mountpoint,
		RootStr:       mnt.Root,
		MountID:       uint32(mnt.ID),
		GroupID:       uint32(groupID),
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
	probe       *Probe
	lock        sync.RWMutex
	mounts      map[uint32]*model.MountEvent
	devices     map[uint32]map[uint32]*model.MountEvent
	deleteQueue []deleteRequest
}

// SyncCache - Snapshots the current mount points of the system by reading through /proc/[pid]/mountinfo.
func (mr *MountResolver) SyncCache(proc *process.Process) error {
	mr.lock.Lock()
	defer mr.lock.Unlock()

	mnts, err := utils.ParseMountInfoFile(proc.Pid)
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
		mr.insert(*e)

		// init discarder revisions
		mr.probe.inodeDiscarders.initRevision(e)
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
func (mr *MountResolver) Insert(e model.MountEvent) {
	mr.lock.Lock()
	defer mr.lock.Unlock()

	if e.MountPointPathResolutionError != nil || e.RootPathResolutionError != nil {
		// do not insert an invalid value
		return
	}

	mr.insert(e)

	// init discarder revisions
	mr.probe.inodeDiscarders.initRevision(&e)
}

func (mr *MountResolver) insert(e model.MountEvent) {
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
	deviceMounts[e.MountID] = &e

	mr.mounts[e.MountID] = &e
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
			mountPointStr = path.Join(p, mount.MountPointStr)
		}
	}

	return mountPointStr
}

func (mr *MountResolver) getParentPath(mountID uint32) string {
	return mr._getParentPath(mountID, map[uint32]bool{})
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
	for _, deviceMount := range mr.devices[mount.Device] {
		if mount.MountID != deviceMount.MountID && deviceMount.IsOverlayFS() {
			if p := mr.getParentPath(deviceMount.MountID); p != "" {
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
// it exists
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

	ref := mount
	if ancestor := mr.getAncestor(mount); ancestor != nil {
		ref = ancestor
	}

	return mr.getOverlayPath(ref), mr.getParentPath(mountID), mount.RootStr, nil
}

func getMountIDOffset(probe *Probe) uint64 {
	offset := uint64(284)

	switch {
	case probe.kernelVersion.IsSuseKernel():
		offset = 292
	case probe.kernelVersion.Code != 0 && probe.kernelVersion.Code < kernel.Kernel4_13:
		offset = 268
	}

	return offset
}

func getSizeOfStructInode(probe *Probe) uint64 {
	sizeOf := uint64(600)

	switch {
	case probe.kernelVersion.IsRH7Kernel():
		sizeOf = 584
	case probe.kernelVersion.IsRH8Kernel():
		sizeOf = 648
	case probe.kernelVersion.IsSLES12Kernel():
		sizeOf = 560
	case probe.kernelVersion.IsSLES15Kernel():
		sizeOf = 592
	case probe.kernelVersion.IsOracleUEKKernel():
		sizeOf = 632
	case probe.kernelVersion.Code != 0 && probe.kernelVersion.Code < kernel.Kernel4_16:
		sizeOf = 608
	}

	return sizeOf
}

func getSuperBlockMagicOffset(probe *Probe) uint64 {
	sizeOf := uint64(96)

	if probe.kernelVersion.IsRH7Kernel() {
		sizeOf = 88
	}

	return sizeOf
}

// NewMountResolver instantiates a new mount resolver
func NewMountResolver(probe *Probe) *MountResolver {
	return &MountResolver{
		probe:   probe,
		lock:    sync.RWMutex{},
		devices: make(map[uint32]map[uint32]*model.MountEvent),
		mounts:  make(map[uint32]*model.MountEvent),
	}
}
