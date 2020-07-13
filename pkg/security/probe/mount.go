package probe

import (
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/DataDog/gopsutil/process"
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"

	eprobe "github.com/DataDog/datadog-agent/pkg/ebpf/probe"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

var (
	ErrMountNotFound = errors.New("unknown mount ID")
)

func IsErrMountNotFound(err error) bool {
	return ErrMountNotFound.Error() == err.Error()
}

// MountHookPoints - Mount tracking probes
var MountHookPoints = []*HookPoint{
	{
		KProbes: []*eprobe.KProbe{{
			Name:      "attach_recursive_mnt",
			EntryFunc: "kprobe/attach_recursive_mnt",
		}},
		EventTypes: map[string]Capabilities{
			"*": Capabilities{},
		},
	},
	{
		KProbes: []*eprobe.KProbe{{
			Name:      "propagate_mnt",
			EntryFunc: "kprobe/propagate_mnt",
		}},
		EventTypes: map[string]Capabilities{
			"*": Capabilities{},
		},
	},
	{
		KProbes: syscallKprobe("mount"),
		EventTypes: map[string]Capabilities{
			"*": Capabilities{},
		},
	},
	{
		KProbes: []*eprobe.KProbe{{
			Name:      "security_sb_umount",
			EntryFunc: "kprobe/security_sb_umount",
		}},
		EventTypes: map[string]Capabilities{
			"*": Capabilities{},
		},
	},
	{
		KProbes: []*eprobe.KProbe{{
			Name:     "sys_umount",
			ExitFunc: "kretprobe/" + getSyscallFnName("umount"),
		}},
		EventTypes: map[string]Capabilities{
			"*": Capabilities{},
		},
	},
}

// newMountEventFromMountInfo - Creates a new MountEvent from parsed MountInfo data
func newMountEventFromMountInfo(mnt *utils.MountInfo) (*MountEvent, error) {
	// extract dev couple from "major:minor"
	devCouple := strings.Split(mnt.MajorMinorVer, ":")
	if len(devCouple) < 2 {
		// unknown device number ignore
		return nil, errors.New("invalid device number")
	}
	major, err := strconv.ParseUint(devCouple[0], 10, 32)
	if err != nil {
		// unknown device major number, ignore
		return nil, err
	}
	minor, err := strconv.ParseUint(devCouple[1], 10, 32)
	if err != nil {
		// unknown device minor number, ignore
		return nil, err
	}

	// extract group id from the parsed optional fields
	var groupID uint64
	if mnt.OptionalFields["shared"] != "" {
		groupID, err = strconv.ParseUint(mnt.OptionalFields["shared"], 10, 64)
		if err != nil {
			// unknown group ID, ignore
			return nil, err
		}
	}
	if mnt.OptionalFields["master"] != "" {
		groupID, err = strconv.ParseUint(mnt.OptionalFields["master"], 10, 64)
		if err != nil {
			// unknown group ID, ignore
			return nil, err
		}
	}

	// prepare path
	var path string
	if strings.HasSuffix(mnt.MountPoint, mnt.Root) {
		path = strings.TrimSuffix(mnt.MountPoint, mnt.Root)
	} else {
		path = mnt.MountPoint
	}

	// create a MountEvent out of the parsed MountInfo
	return &MountEvent{
		ParentMountID: uint32(mnt.ParentID),
		ParentPathStr: path,
		NewMountID:    uint32(mnt.MountID),
		NewGroupID:    uint32(groupID),
		NewDevice:     uint32(unix.Mkdev(uint32(major), uint32(minor))),
		FSType:        mnt.FSType,
	}, nil
}

// Mount - Mount represents a mount point on the system.
type Mount struct {
	*MountEvent

	containerMountPath string
	mountPath          string
	parent             *Mount
	children           []*Mount
	peerGroup          *OverlayGroup
}

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

func sanitizeContainerPath(eventPath string) string {
	// Look for the first container ID and remove everything that comes after
	r, _ := regexp.Compile("[0-9a-f]{64}")
	loc := r.FindStringIndex(eventPath)
	if len(loc) == 2 {
		return eventPath[:loc[1]]
	}
	return ""
}

// newMount - Creates a new Mount from a mount event and sets / updates its parent
func newMount(e *MountEvent, parent *Mount, group *OverlayGroup) *Mount {
	m := Mount{
		MountEvent: e,
		parent:     parent,
		peerGroup:  group,
	}
	eventPath := e.ParentPathStr
	if e.GetFSType() == "overlay" {
		m.containerMountPath = sanitizeContainerPath(eventPath)
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
		if group != nil && group.parent != nil && group.parent.NewMountID != e.NewMountID && group.parent.GetFSType() == "overlay" {
			m.containerMountPath = sanitizeContainerPath(group.parent.mountPath)
		}
	}
	return &m
}

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

// Delete - Deletes a mount point in the peer group. Returns true if the PeerGroup is empty after the deletion (a peer
// group is empty if its master is nil and its list of slaves is empty).
func (g *OverlayGroup) Delete(m *Mount) bool {
	if g.parent != nil && g.parent.NewMountID == m.NewMountID {
		g.parent = nil
	}
	delete(g.children, m.NewMountID)
	return g.IsEmpty()
}

// IsEmpty - Returns true if the overlay group is empty and should therefore be deleted
func (g *OverlayGroup) IsEmpty() bool {
	return g.parent == nil && len(g.children) == 0
}

// Insert - Inserts a new mount in the peer group form the provided parameters
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

// Delete - Deletes a mount from the device
func (d *FSDevice) Delete(m *Mount) bool {
	g, ok := d.peerGroups[m.NewGroupID]
	if ok {
		if g.Delete(m) {
			// delete the group as well
			delete(d.peerGroups, m.NewGroupID)
		}
	}
	return d.IsEmty()
}

// IsEmpty - Returns true if the device is empty and should therefore be deleted
func (d *FSDevice) IsEmty() bool {
	return len(d.peerGroups) == 0
}

// Insert - Inserts a new mount in the list of mount groups of the device
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

// MountResolver - Mount point cache
type MountResolver struct {
	lock    sync.RWMutex
	devices map[uint32]*FSDevice
	mounts  map[uint32]*Mount
}

// SyncCache - Snapshots the current mount points of the system by reading through /proc/[pid]/mountinfo. If pid is null,
// the function will parse the mountinfo entry of all the processes currently running.
func (mr *MountResolver) SyncCache(pid uint32) error {
	mr.lock.Lock()
	defer mr.lock.Unlock()
	if pid > 0 {
		return mr.syncCache(pid)
	}

	// List all processes and parse mountinfo
	processes, err := process.AllProcesses()
	if err != nil {
		return err
	}
	for _, process := range processes {
		if err := mr.syncCache(uint32(process.Pid)); err != nil {
			if !os.IsNotExist(err) {
				return err
			}
		}
	}
	return nil
}

func (mr *MountResolver) syncCache(pid uint32) error {
	// Parse /proc/[pid]/moutinfo
	mnts, err := utils.GetProcMounts(pid)
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
		mr.insert(e, false)
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

// Delete - Deletes a mount from the cache
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

// Insert - Inserts a new mount point in the cache
func (mr *MountResolver) Insert(e *MountEvent) {
	mr.lock.Lock()
	defer mr.lock.Unlock()
	mr.insert(e, true)
}

func (mr *MountResolver) insert(e *MountEvent, allowResync bool) {
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

// GetMountPath - Returns the path of a mount identified by its mount ID. The first path is the container mount path if
// it exists
func (mr *MountResolver) GetMountPath(mountID uint32, numlower int32) (string, string, error) {
	mr.lock.RLock()
	defer mr.lock.RUnlock()
	m, ok := mr.mounts[mountID]
	if !ok {
		if mountID == 0 {
			return "", "", nil
		}
		if !ok {
			return "", "", ErrMountNotFound
		}
	}
	if m.containerMountPath != "" {
		if numlower == 0 {
			return path.Join(m.containerMountPath, "diff"), m.mountPath, nil
		}
		return path.Join(m.containerMountPath, "merged"), m.mountPath, nil
	}
	return "", m.mountPath, nil
}

// NewMountResolver - Instantiates a new mount resolver
func NewMountResolver() *MountResolver {
	return &MountResolver{
		lock:    sync.RWMutex{},
		devices: make(map[uint32]*FSDevice),
		mounts:  make(map[uint32]*Mount),
	}

}
