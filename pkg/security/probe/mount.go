// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

import (
	"os"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/moby/sys/mountinfo"
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

var (
	// ErrMountNotFound is used when an unknown mount identifier is found
	ErrMountNotFound = errors.New("unknown mount ID")
)

// mountHookPoints holds the list of probes required to track mounts
var mountHookPoints = []*HookPoint{
	{
		Name: "attach_recursive_mnt",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/attach_recursive_mnt",
		}},
		EventTypes: []eval.EventType{"*"},
	},
	{
		Name: "propagate_mnt",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/propagate_mnt",
		}},
		EventTypes: []eval.EventType{"*"},
	},
	{
		Name:       "sys_mount",
		KProbes:    syscallKprobe("mount"),
		EventTypes: []eval.EventType{"*"},
	},
	{
		Name: "security_sb_umount",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/security_sb_umount",
		}},
		EventTypes: []eval.EventType{"*"},
	},
	{
		Name: "sys_umount",
		KProbes: []*ebpf.KProbe{{
			ExitFunc: "kretprobe/" + getSyscallFnName("umount"),
		}},
		EventTypes: []eval.EventType{"*"},
	},
}

// newMountEventFromMountInfo - Creates a new MountEvent from parsed MountInfo data
func newMountEventFromMountInfo(mnt *mountinfo.Info) (*MountEvent, error) {
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
	return &MountEvent{
		ParentMountID: uint32(mnt.Parent),
		MountPointStr: mnt.Mountpoint,
		RootStr:       mnt.Root,
		NewMountID:    uint32(mnt.ID),
		NewGroupID:    uint32(groupID),
		NewDevice:     uint32(unix.Mkdev(uint32(mnt.Major), uint32(mnt.Minor))),
		FSType:        mnt.Fstype,
	}, nil
}

// Mount represents a mount point on the system.
type Mount struct {
	*MountEvent

	containerMountPath string
	mountPath          string
	parent             *Mount
	children           []*Mount
	peerGroup          *OverlayGroup
}

// DFS performs a Depth-First Search of the mount point tree used to compute
// the list of inter dependent mount points
func (m *Mount) DFS(mask map[uint32]bool) []*Mount {
	var mounts []*Mount
	if mask == nil {
		mask = map[uint32]bool{}
	}
	if !mask[m.NewMountID] {
		mounts = append(mounts, m)
		mask[m.NewMountID] = true
	}
	for _, child := range m.children {
		if !mask[child.NewMountID] {
			mask[child.NewMountID] = true
			mounts = append(mounts, child)
			mounts = append(mounts, child.DFS(mask)...)
		}
	}
	return mounts
}

// newMount creates a new Mount from a mount event and sets / updates its parent
func newMount(e *MountEvent, parent *Mount, group *OverlayGroup) *Mount {
	m := Mount{
		MountEvent: e,
		parent:     parent,
		peerGroup:  group,
	}
	eventPath := e.MountPointStr
	if e.GetFSType() == "overlay" && eventPath != "/" {
		m.containerMountPath = eventPath
	}
	if parent != nil {
		if strings.HasPrefix(eventPath, parent.mountPath) {
			m.mountPath = eventPath
		} else {
			m.mountPath = path.Join(parent.mountPath, eventPath)
		}
		if m.containerMountPath == "" {
			m.containerMountPath = parent.containerMountPath
		}
		parent.children = append(parent.children, &m)
	}
	if m.containerMountPath == "" {
		if group != nil && group.parent != nil && group.parent.NewMountID != e.NewMountID && group.parent.GetFSType() == "overlay" && group.parent.mountPath != "/" {
			m.containerMountPath = group.parent.mountPath
		}
	}
	return &m
}

// OverlayGroup groups the mount points of an overlay filesystem
type OverlayGroup struct {
	parent   *Mount
	children map[uint32]*Mount
}

// dryDelete - If the provided mount was deleted, dryDeletes returns the list of mounts that should be deleted as well
func (g *OverlayGroup) dryDelete(m *Mount) []*Mount {
	var mounts []*Mount
	mask := map[uint32]bool{}

	// Mark the immediate children of the mount for deletion
	mounts = append(mounts, m.DFS(mask)...)

	// Mark the children of the overlay group for deletion
	if g.parent != nil && m.NewMountID == g.parent.NewMountID {
		for _, v := range g.children {
			mounts = append(mounts, v.DFS(mask)...)
		}
	}
	return mounts
}

// Delete a mount point in the peer group. Returns true if the PeerGroup is empty after the deletion (a peer
// group is empty if its master is nil and its list of slaves is empty).
func (g *OverlayGroup) Delete(m *Mount) bool {
	if g.parent != nil && g.parent.NewMountID == m.NewMountID {
		g.parent = nil
	}
	delete(g.children, m.NewMountID)
	return g.IsEmpty()
}

// IsEmpty returns true if the overlay group is empty and should therefore be deleted
func (g *OverlayGroup) IsEmpty() bool {
	return g.parent == nil && len(g.children) == 0
}

// Insert a new mount in the peer group form the provided parameters
func (g *OverlayGroup) Insert(e *MountEvent, parent *Mount) *Mount {
	// create new mount
	m := newMount(e, parent, g)

	// Check if this is a slave mount
	if m.GetFSType() == "overlay" {
		g.parent = m
	} else {
		g.children[m.NewMountID] = m
	}
	return m
}

func newPeerGroup() *OverlayGroup {
	return &OverlayGroup{
		children: make(map[uint32]*Mount),
	}
}

// FSDevice represents a peer group
type FSDevice struct {
	OverlayGroupID uint32
	peerGroups     map[uint32]*OverlayGroup
}

// dryDelete - If the provided mount was deleted, dryDeletes returns the list of mounts that should be deleted as well
func (d *FSDevice) dryDelete(m *Mount) []*Mount {
	g, ok := d.peerGroups[m.NewGroupID]
	if !ok {
		return []*Mount{m}
	}
	return g.dryDelete(m)
}

// Delete a mount from the device
func (d *FSDevice) Delete(m *Mount) bool {
	g, ok := d.peerGroups[m.NewGroupID]
	if ok {
		if g.Delete(m) {
			// delete the group as well
			delete(d.peerGroups, m.NewGroupID)
		}
	}
	return d.IsEmpty()
}

// IsEmpty returns true if the device is empty and should therefore be deleted
func (d *FSDevice) IsEmpty() bool {
	return len(d.peerGroups) == 0
}

// Insert a new mount in the list of mount groups of the device
func (d *FSDevice) Insert(e *MountEvent, parent *Mount) *Mount {
	// The first mount of the overlay inside the container is technically a bind. Map it to its rightful overlay
	// group ID if there is one.
	if e.GetFSType() == "bind" && d.OverlayGroupID != 0 && e.NewGroupID == 0 {
		e.NewGroupID = d.OverlayGroupID
	}
	// Select overlay group
	pg, ok := d.peerGroups[e.NewGroupID]
	if !ok {
		pg = newPeerGroup()
		d.peerGroups[e.NewGroupID] = pg
	}

	// Insert mount in peer group
	return pg.Insert(e, parent)
}

func newFSDevice() *FSDevice {
	return &FSDevice{
		peerGroups: make(map[uint32]*OverlayGroup),
	}
}

// MountResolver represents a cache for mountpoints and the corresponding file systems
type MountResolver struct {
	lock    sync.RWMutex
	devices map[uint32]*FSDevice
	mounts  map[uint32]*Mount
}

// SyncCache - Snapshots the current mount points of the system by reading through /proc/[pid]/mountinfo.
func (mr *MountResolver) SyncCache(pid uint32) error {
	mr.lock.Lock()
	defer mr.lock.Unlock()
	// Parse /proc/[pid]/moutinfo
	mnts, err := mountinfo.PidMountInfo(int(pid))
	if err != nil {
		pErr, ok := err.(*os.PathError)
		if !ok {
			return err
		}
		return pErr
	}

	// Insert each mount in cache
	for _, mnt := range mnts {
		e, err := newMountEventFromMountInfo(mnt)
		if err != nil {
			return err
		}

		// Insert mount point
		mr.insert(e)
	}
	return nil
}

// dryDelete - If the provided mount was deleted, dryDeletes returns the list of mounts that should be deleted as well
func (mr *MountResolver) dryDelete(m *Mount) []*Mount {
	d, ok := mr.devices[m.NewDevice]
	if !ok {
		return []*Mount{m}
	}
	return d.dryDelete(m)
}

// Delete a mount from the cache
func (mr *MountResolver) Delete(mountID uint32) error {
	mr.lock.Lock()
	defer mr.lock.Unlock()
	m, ok := mr.mounts[mountID]
	if !ok {
		return ErrMountNotFound
	}

	// computes the list of mounts that should be deleted if m is deleted
	mnts := mr.dryDelete(m)

	// delete m and all the mounts that depend on it
	for _, mnt := range mnts {
		d, ok := mr.devices[mnt.NewDevice]
		if ok {
			if d.Delete(mnt) {
				delete(mr.devices, mnt.NewDevice)
			}
		}
		delete(mr.mounts, mnt.NewMountID)
	}
	return nil
}

// Insert a new mount point in the cache
func (mr *MountResolver) Insert(e *MountEvent) {
	mr.lock.Lock()
	defer mr.lock.Unlock()
	mr.insert(e)
}

func (mr *MountResolver) insert(e *MountEvent) {
	// Fetch the device of the new mount point
	d, ok := mr.devices[e.NewDevice]
	if !ok {
		d = newFSDevice()
		// Set the overlay group ID if necessary
		if e.GetFSType() == "overlay" {
			d.OverlayGroupID = e.NewGroupID
		}
		mr.devices[e.NewDevice] = d
	}

	// Fetch the new mount point parent
	parent, _ := mr.mounts[e.ParentMountID]

	// Insert the new mount point in the device cache
	m := d.Insert(e, parent)

	// Insert the mount point in the top level list of mounts
	mr.mounts[e.NewMountID] = m
}

// GetMountPath returns the path of a mount identified by its mount ID. The first path is the container mount path if
// it exists
func (mr *MountResolver) GetMountPath(mountID uint32, numlower int32) (string, string, string, error) {
	mr.lock.RLock()
	defer mr.lock.RUnlock()
	m, ok := mr.mounts[mountID]
	if !ok {
		if mountID == 0 {
			return "", "", "", nil
		}
		if !ok {
			return "", "", "", ErrMountNotFound
		}
	}
	// The containerMountPath will always refer to the merged layer of the overlay filesystem (when there is one)
	// Look at the numlower field of the event to differentiate merged / diff layers
	// (numlower == 0 => diff | numlower > 0 => merged, therefore the file is from the original container)
	return m.containerMountPath, m.mountPath, m.RootStr, nil
}

// NewMountResolver instantiates a new mount resolver
func NewMountResolver() *MountResolver {
	return &MountResolver{
		lock:    sync.RWMutex{},
		devices: make(map[uint32]*FSDevice),
		mounts:  make(map[uint32]*Mount),
	}
}
