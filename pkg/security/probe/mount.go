// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probe

import (
	"os"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/DataDog/gopsutil/process"

	"github.com/cobaugh/osrelease"
	"github.com/moby/sys/mountinfo"
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

var (
	// KERNEL_VERSION(a,b,c) = (a << 16) + (b << 8) + (c)
	kernel4_13 = kernel.VersionCode(4, 13, 0) //nolint:deadcode,unused
	kernel4_16 = kernel.VersionCode(4, 16, 0) //nolint:deadcode,unused
	kernel5_3  = kernel.VersionCode(5, 3, 0)  //nolint:deadcode,unused
)

var (
	// ErrMountNotFound is used when an unknown mount identifier is found
	ErrMountNotFound = errors.New("unknown mount ID")
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

// MountResolver represents a cache for mountpoints and the corresponding file systems
type MountResolver struct {
	probe   *Probe
	lock    sync.RWMutex
	mounts  map[uint32]*model.MountEvent
	devices map[uint32]map[uint32]*model.MountEvent
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
	mr.delete(mount)

	return nil
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

	mr.insert(e)

	// init discarder revisions
	mr.probe.inodeDiscarders.initRevision(&e)
}

func (mr *MountResolver) insert(e model.MountEvent) {
	// Retrieve the parent paths and strip it from the event
	p, ok := mr.mounts[e.ParentMountID]
	if ok {
		prefix := mr.getParentPath(p.MountID)
		if len(prefix) > 0 && prefix != "/" {
			e.MountPointStr = strings.TrimPrefix(e.MountPointStr, prefix)
		}
	}

	mounts := mr.devices[e.Device]
	if mounts == nil {
		mounts = make(map[uint32]*model.MountEvent)
		mr.devices[e.Device] = mounts
	}
	mounts[e.MountID] = &e

	mr.mounts[e.MountID] = &e
}

func (mr *MountResolver) getParentPath(mountID uint32) string {
	mount, exists := mr.mounts[mountID]
	if !exists {
		return ""
	}

	mountPointStr := mount.MountPointStr

	if mount.ParentMountID != 0 {
		p := mr.getParentPath(mount.ParentMountID)
		if p == "" {
			return mountPointStr
		}

		if p != "/" && !strings.HasPrefix(mount.MountPointStr, p) {
			mountPointStr = path.Join(p, mount.MountPointStr)
		}
	}

	return mountPointStr
}

func (mr *MountResolver) getAncestor(mount *model.MountEvent, maxDepth int) *model.MountEvent {
	if maxDepth <= 0 {
		return nil
	}

	parent, ok := mr.mounts[mount.ParentMountID]
	if !ok {
		return nil
	}

	if grandParent := mr.getAncestor(parent, maxDepth-1); grandParent != nil {
		return grandParent
	}

	return parent
}

// getOverlayPath uses deviceID to find overlay path
func (mr *MountResolver) getOverlayPath(mount *model.MountEvent) string {
	for _, deviceMount := range mr.devices[mount.Device] {
		if mount.Device == deviceMount.Device && mount.MountID != deviceMount.MountID && deviceMount.IsOverlayFS() {
			if p := mr.getParentPath(deviceMount.MountID); p != "" {
				return p
			}
		}
	}

	return ""
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
	if ancestor := mr.getAncestor(mount, 5); ancestor != nil {
		ref = ancestor
	}

	return mr.getOverlayPath(ref), mr.getParentPath(mountID), mount.RootStr, nil
}

func getMountIDOffset(probe *Probe) uint64 {
	var suseKernel bool

	osrelease, err := osrelease.Read()
	if err == nil {
		suseKernel = (osrelease["ID"] == "sles") || (osrelease["ID"] == "opensuse-leap")
	}

	var offset uint64
	if suseKernel {
		offset = 292
	} else if probe.kernelVersion != 0 && probe.kernelVersion < kernel4_13 {
		offset = 268
	} else {
		offset = 284
	}

	return offset
}

func getSizeOfStructInode(probe *Probe) uint64 {
	var rh7Kernel bool
	var rh8Kernel bool
	var suse12Kernel bool

	osrelease, err := osrelease.Read()
	if err == nil {
		rh7Kernel = (osrelease["ID"] == "centos" || osrelease["ID"] == "rhel") && osrelease["VERSION_ID"] == "7"
		rh8Kernel = osrelease["PLATFORM_ID"] == "platform:el8"
		suse12Kernel = ((osrelease["ID"] == "sles") || (osrelease["ID"] == "opensuse-leap")) && strings.HasPrefix(osrelease["VERSION_ID"], "12")
	}

	var sizeOf uint64
	if rh7Kernel {
		sizeOf = 584
	} else if rh8Kernel {
		sizeOf = 648
	} else if suse12Kernel {
		sizeOf = 560
	} else if probe.kernelVersion != 0 && probe.kernelVersion < kernel4_16 {
		sizeOf = 608
	} else {
		sizeOf = 600
	}

	return sizeOf
}

func getSuperBlockMagicOffset(probe *Probe) uint64 {
	var rh7Kernel bool

	osrelease, err := osrelease.Read()
	if err == nil {
		rh7Kernel = (osrelease["ID"] == "centos" || osrelease["ID"] == "rhel") && (osrelease["VERSION_ID"] == "7")
	}

	var sizeOf uint64
	if rh7Kernel {
		sizeOf = 88
	} else {
		sizeOf = 96
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
