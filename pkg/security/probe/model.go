// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"fmt"
	"path"
	"strings"
	"syscall"
	"time"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/mailru/easyjson/jwriter"
	"golang.org/x/sys/unix"

	pconfig "github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe/constantfetch"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

const (
	// ServiceEnvVar environment variable used to report service
	ServiceEnvVar = "DD_SERVICE"
)

var eventZero Event

// Model describes the data model for the runtime security agent probe events
type Model struct {
	model.Model
	probe *Probe
}

// ValidateField validates the value of a field
func (m *Model) ValidateField(field eval.Field, fieldValue eval.FieldValue) error {
	if err := m.Model.ValidateField(field, fieldValue); err != nil {
		return err
	}

	switch field {
	case "bpf.map.name":
		if offset, found := m.probe.constantOffsets["bpf_map_name_offset"]; !found || offset == constantfetch.ErrorSentinel {
			return fmt.Errorf("%s is not available on this kernel version", field)
		}

	case "bpf.prog.name":
		if offset, found := m.probe.constantOffsets["bpf_prog_aux_name_offset"]; !found || offset == constantfetch.ErrorSentinel {
			return fmt.Errorf("%s is not available on this kernel version", field)
		}
	}

	return nil
}

// NewEvent returns a new Event
func (m *Model) NewEvent() eval.Event {
	return &Event{}
}

// NetDeviceKey is used to uniquely identify a network device
type NetDeviceKey struct {
	IfIndex          uint32
	NetNS            uint32
	NetworkDirection manager.TrafficType
}

// Event describes a probe event
type Event struct {
	model.Event

	resolvers           *Resolvers
	processCacheEntry   *model.ProcessCacheEntry
	pathResolutionError error
	scrubber            *pconfig.DataScrubber
	probe               *Probe
}

// Retain the event
func (ev *Event) Retain() Event {
	if ev.processCacheEntry != nil {
		ev.processCacheEntry.Retain()
	}
	return *ev
}

// Release the event
func (ev *Event) Release() {
	if ev.processCacheEntry != nil {
		ev.processCacheEntry.Release()
	}
}

// GetPathResolutionError returns the path resolution error as a string if there is one
func (ev *Event) GetPathResolutionError() error {
	return ev.pathResolutionError
}

// ResolveFilePath resolves the inode to a full path
func (ev *Event) ResolveFilePath(f *model.FileEvent) string {
	// do not try to resolve mmap events when they aren't backed by any file
	switch ev.GetEventType() {
	case model.MMapEventType:
		if ev.MMap.Flags&unix.MAP_ANONYMOUS != 0 {
			return ""
		}
	case model.LoadModuleEventType:
		if ev.LoadModule.LoadedFromMemory {
			return ""
		}
	}

	if len(f.PathnameStr) == 0 {
		path, err := ev.resolvers.resolveFileFieldsPath(&f.FileFields)
		if err != nil {
			switch err.(type) {
			case ErrDentryPathKeyNotFound:
				// this error is the only one we don't care about
			default:
				f.PathResolutionError = err
				ev.SetPathResolutionError(err)
			}
		}
		f.PathnameStr = path
	}
	return f.PathnameStr
}

// ResolveFileBasename resolves the inode to a full path
func (ev *Event) ResolveFileBasename(f *model.FileEvent) string {
	if len(f.BasenameStr) == 0 {
		if f.PathnameStr != "" {
			f.BasenameStr = path.Base(f.PathnameStr)
		} else {
			f.BasenameStr = ev.resolvers.resolveBasename(&f.FileFields)
		}
	}
	return f.BasenameStr
}

// ResolveFileFilesystem resolves the filesystem a file resides in
func (ev *Event) ResolveFileFilesystem(f *model.FileEvent) string {
	return ev.resolvers.MountResolver.GetFilesystem(f.FileFields.MountID)
}

// ResolveFileFieldsInUpperLayer resolves whether the file is in an upper layer
func (ev *Event) ResolveFileFieldsInUpperLayer(f *model.FileFields) bool {
	return f.GetInUpperLayer()
}

// ResolveXAttrName returns the string representation of the extended attribute name
func (ev *Event) ResolveXAttrName(e *model.SetXAttrEvent) string {
	if len(e.Name) == 0 {
		e.Name, _ = model.UnmarshalString(e.NameRaw[:], 200)
	}
	return e.Name
}

// ResolveHelpers returns the list of eBPF helpers used by the current program
func (ev *Event) ResolveHelpers(e *model.BPFProgram) []uint32 {
	return e.Helpers
}

// ResolveXAttrNamespace returns the string representation of the extended attribute namespace
func (ev *Event) ResolveXAttrNamespace(e *model.SetXAttrEvent) string {
	if len(e.Namespace) == 0 {
		fragments := strings.Split(ev.ResolveXAttrName(e), ".")
		if len(fragments) > 0 {
			e.Namespace = fragments[0]
		}
	}
	return e.Namespace
}

// SetMountPoint set the mount point information
func (ev *Event) SetMountPoint(e *model.MountEvent) {
	e.MountPointStr, e.MountPointPathResolutionError = ev.resolvers.DentryResolver.Resolve(e.ParentMountID, e.ParentInode, 0, true)
}

// ResolveMountPoint resolves the mountpoint to a full path
func (ev *Event) ResolveMountPoint(e *model.MountEvent) string {
	if len(e.MountPointStr) == 0 {
		ev.SetMountPoint(e)
	}
	return e.MountPointStr
}

// SetMountRoot set the mount point information
func (ev *Event) SetMountRoot(e *model.MountEvent) {
	e.RootStr, e.RootPathResolutionError = ev.resolvers.DentryResolver.Resolve(e.RootMountID, e.RootInode, 0, true)
}

// ResolveMountRoot resolves the mountpoint to a full path
func (ev *Event) ResolveMountRoot(e *model.MountEvent) string {
	if len(e.RootStr) == 0 {
		ev.SetMountRoot(e)
	}
	return e.RootStr
}

// ResolveContainerID resolves the container ID of the event
func (ev *Event) ResolveContainerID(e *model.ContainerContext) string {
	if len(e.ID) == 0 {
		if entry := ev.ResolveProcessCacheEntry(); entry != nil {
			e.ID = entry.ContainerID
		}
	}
	return e.ID
}

// ResolveContainerTags resolves the container tags of the event
func (ev *Event) ResolveContainerTags(e *model.ContainerContext) []string {
	if len(e.Tags) == 0 && e.ID != "" {
		e.Tags = ev.resolvers.TagsResolver.Resolve(e.ID)
	}
	return e.Tags
}

// UnmarshalProcess unmarshal a Process
func (ev *Event) UnmarshalProcess(data []byte) (int, error) {
	// reset the process cache entry of the current event
	entry := ev.resolvers.ProcessResolver.NewProcessCacheEntry()
	entry.Pid = ev.ProcessContext.Pid
	entry.Tid = ev.ProcessContext.Tid

	n, err := entry.Process.UnmarshalBinary(data)
	if err != nil {
		return n, err
	}
	entry.Process.ContainerID = ev.ContainerContext.ID

	ev.processCacheEntry = entry

	return n, nil
}

// ResolveFileFieldsUser resolves the user id of the file to a username
func (ev *Event) ResolveFileFieldsUser(e *model.FileFields) string {
	if len(e.User) == 0 {
		e.User, _ = ev.resolvers.UserGroupResolver.ResolveUser(int(e.UID))
	}
	return e.User
}

// ResolveFileFieldsGroup resolves the group id of the file to a group name
func (ev *Event) ResolveFileFieldsGroup(e *model.FileFields) string {
	if len(e.Group) == 0 {
		e.Group, _ = ev.resolvers.UserGroupResolver.ResolveGroup(int(e.GID))
	}
	return e.Group
}

// ResolveRights resolves the rights of a file
func (ev *Event) ResolveRights(e *model.FileFields) int {
	return int(e.Mode) & (syscall.S_ISUID | syscall.S_ISGID | syscall.S_ISVTX | syscall.S_IRWXU | syscall.S_IRWXG | syscall.S_IRWXO)
}

// ResolveChownUID resolves the user id of a chown event to a username
func (ev *Event) ResolveChownUID(e *model.ChownEvent) string {
	if len(e.User) == 0 {
		e.User, _ = ev.resolvers.UserGroupResolver.ResolveUser(int(e.UID))
	}
	return e.User
}

// ResolveChownGID resolves the group id of a chown event to a group name
func (ev *Event) ResolveChownGID(e *model.ChownEvent) string {
	if len(e.Group) == 0 {
		e.Group, _ = ev.resolvers.UserGroupResolver.ResolveGroup(int(e.GID))
	}
	return e.Group
}

// ResolveProcessCreatedAt resolves process creation time
func (ev *Event) ResolveProcessCreatedAt(e *model.Process) uint64 {
	return uint64(e.ExecTime.UnixNano())
}

// ResolveProcessArgv0 resolves the first arg of the event
func (ev *Event) ResolveProcessArgv0(process *model.Process) string {
	arg0, _ := ev.resolvers.ProcessResolver.GetProcessArgv0(process)
	return arg0
}

// ResolveProcessArgs resolves the args of the event
func (ev *Event) ResolveProcessArgs(process *model.Process) string {
	return strings.Join(ev.ResolveProcessArgv(process), " ")
}

// ResolveProcessArgv resolves the args of the event as an array
func (ev *Event) ResolveProcessArgv(process *model.Process) []string {
	argv, _ := ev.resolvers.ProcessResolver.GetProcessArgv(process)
	return argv
}

// ResolveProcessEnvp resolves the envp of the event as an array
func (ev *Event) ResolveProcessEnvp(process *model.Process) []string {
	envp, _ := ev.resolvers.ProcessResolver.GetProcessEnvp(process)
	return envp
}

// ResolveProcessArgsTruncated returns whether the args are truncated
func (ev *Event) ResolveProcessArgsTruncated(process *model.Process) bool {
	_, truncated := ev.resolvers.ProcessResolver.GetProcessArgv(process)
	return truncated
}

// ResolveProcessArgsFlags resolves the arguments flags of the event
func (ev *Event) ResolveProcessArgsFlags(process *model.Process) (flags []string) {
	for _, arg := range ev.ResolveProcessArgv(process) {
		if len(arg) > 1 && arg[0] == '-' {
			isFlag := true
			name := arg[1:]
			if len(name) >= 1 && name[0] == '-' {
				name = name[1:]
				isFlag = false
			}

			isOption := false
			for _, r := range name {
				isFlag = isFlag && model.IsAlphaNumeric(r)
				isOption = isOption || r == '='
			}

			if len(name) > 0 {
				if isFlag {
					for _, r := range name {
						flags = append(flags, string(r))
					}
				}
				if !isOption && len(name) > 1 {
					flags = append(flags, name)
				}
			}
		}
	}
	return
}

// ResolveProcessArgsOptions resolves the arguments options of the event
func (ev *Event) ResolveProcessArgsOptions(process *model.Process) (options []string) {
	args := ev.ResolveProcessArgv(process)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if len(arg) > 1 && arg[0] == '-' {
			name := arg[1:]
			if len(name) >= 1 && name[0] == '-' {
				name = name[1:]
			}
			if len(name) > 0 && model.IsAlphaNumeric(rune(name[0])) {
				if index := strings.IndexRune(name, '='); index == -1 {
					if i < len(args)-1 && (len(args[i+1]) == 0 || args[i+1][0] != '-') {
						options = append(options, name+"="+args[i+1])
						i++
					}
				} else {
					options = append(options, name)
				}
			}
		}
	}
	return
}

// ResolveProcessEnvsTruncated returns whether the envs are truncated
func (ev *Event) ResolveProcessEnvsTruncated(process *model.Process) bool {
	_, truncated := ev.resolvers.ProcessResolver.GetProcessEnvs(process)
	return truncated
}

// ResolveProcessEnvs resolves the envs of the event
func (ev *Event) ResolveProcessEnvs(process *model.Process) []string {
	envs, _ := ev.resolvers.ProcessResolver.GetProcessEnvs(process)
	return envs
}

// ResolveSetuidUser resolves the user of the Setuid event
func (ev *Event) ResolveSetuidUser(e *model.SetuidEvent) string {
	if len(e.User) == 0 && ev != nil {
		e.User, _ = ev.resolvers.UserGroupResolver.ResolveUser(int(e.UID))
	}
	return e.User
}

// ResolveSetuidEUser resolves the effective user of the Setuid event
func (ev *Event) ResolveSetuidEUser(e *model.SetuidEvent) string {
	if len(e.EUser) == 0 && ev != nil {
		e.EUser, _ = ev.resolvers.UserGroupResolver.ResolveUser(int(e.EUID))
	}
	return e.EUser
}

// ResolveSetuidFSUser resolves the file-system user of the Setuid event
func (ev *Event) ResolveSetuidFSUser(e *model.SetuidEvent) string {
	if len(e.FSUser) == 0 && ev != nil {
		e.FSUser, _ = ev.resolvers.UserGroupResolver.ResolveUser(int(e.FSUID))
	}
	return e.FSUser
}

// ResolveSetgidGroup resolves the group of the Setgid event
func (ev *Event) ResolveSetgidGroup(e *model.SetgidEvent) string {
	if len(e.Group) == 0 && ev != nil {
		e.Group, _ = ev.resolvers.UserGroupResolver.ResolveUser(int(e.GID))
	}
	return e.Group
}

// ResolveSetgidEGroup resolves the effective group of the Setgid event
func (ev *Event) ResolveSetgidEGroup(e *model.SetgidEvent) string {
	if len(e.EGroup) == 0 && ev != nil {
		e.EGroup, _ = ev.resolvers.UserGroupResolver.ResolveUser(int(e.EGID))
	}
	return e.EGroup
}

// ResolveSetgidFSGroup resolves the file-system group of the Setgid event
func (ev *Event) ResolveSetgidFSGroup(e *model.SetgidEvent) string {
	if len(e.FSGroup) == 0 && ev != nil {
		e.FSGroup, _ = ev.resolvers.UserGroupResolver.ResolveUser(int(e.FSGID))
	}
	return e.FSGroup
}

// ResolveSELinuxBoolName resolves the boolean name of the SELinux event
func (ev *Event) ResolveSELinuxBoolName(e *model.SELinuxEvent) string {
	if e.EventKind != model.SELinuxBoolChangeEventKind {
		return ""
	}

	if len(ev.SELinux.BoolName) == 0 {
		ev.SELinux.BoolName = ev.resolvers.resolveBasename(&e.File.FileFields)
	}
	return ev.SELinux.BoolName
}

func (ev *Event) String() string {
	d, err := ev.MarshalJSON()
	if err != nil {
		return err.Error()
	}
	return string(d)
}

// SetPathResolutionError sets the Event.pathResolutionError
func (ev *Event) SetPathResolutionError(err error) {
	ev.pathResolutionError = err
}

// MarshalJSON returns the JSON encoding of the event
func (ev *Event) MarshalJSON() ([]byte, error) {
	s := NewEventSerializer(ev)
	w := &jwriter.Writer{
		Flags: jwriter.NilSliceAsEmpty | jwriter.NilMapAsEmpty,
	}
	s.MarshalEasyJSON(w)
	return w.BuildBytes()
}

// ExtractEventInfo extracts cpu and timestamp from the raw data event
func ExtractEventInfo(data []byte) (uint64, uint64, error) {
	if len(data) < 16 {
		return 0, 0, model.ErrNotEnoughData
	}

	return model.ByteOrder.Uint64(data[0:8]), model.ByteOrder.Uint64(data[8:16]), nil
}

// ResolveEventTimestamp resolves the monolitic kernel event timestamp to an absolute time
func (ev *Event) ResolveEventTimestamp() time.Time {
	if ev.Timestamp.IsZero() {
		ev.Timestamp = ev.resolvers.TimeResolver.ResolveMonotonicTimestamp(ev.TimestampRaw)
		if ev.Timestamp.IsZero() {
			ev.Timestamp = time.Now()
		}
	}
	return ev.Timestamp
}

// ResolveProcessCacheEntry queries the ProcessResolver to retrieve the ProcessCacheEntry of the event
func (ev *Event) ResolveProcessCacheEntry() *model.ProcessCacheEntry {
	if ev.processCacheEntry == nil {
		ev.processCacheEntry = ev.resolvers.ProcessResolver.Resolve(ev.ProcessContext.Pid, ev.ProcessContext.Tid)
		if ev.processCacheEntry == nil {
			ev.processCacheEntry = &model.ProcessCacheEntry{}
		}
	}

	return ev.processCacheEntry
}

// GetProcessServiceTag returns the service tag based on the process context
func (ev *Event) GetProcessServiceTag() string {
	entry := ev.ResolveProcessCacheEntry()
	if entry == nil {
		return ""
	}

	// first search in the process context itself
	if entry.EnvsEntry != nil {
		if service := entry.EnvsEntry.Get(ServiceEnvVar); service != "" {
			return service
		}
	}

	inContainer := entry.ContainerID != ""

	// while in container check for each ancestor
	for ancestor := entry.Ancestor; ancestor != nil; ancestor = ancestor.Ancestor {
		if inContainer && ancestor.ContainerID == "" {
			break
		}

		if ancestor.EnvsEntry != nil {
			if service := ancestor.EnvsEntry.Get(ServiceEnvVar); service != "" {
				return service
			}
		}
	}

	return ""
}

// ResolveNetworkDeviceIfName returns the network iterface name from the network context
func (ev *Event) ResolveNetworkDeviceIfName(device *model.NetworkDeviceContext) string {
	if len(device.IfName) == 0 && ev.probe != nil {
		key := NetDeviceKey{
			NetNS:            device.NetNS,
			IfIndex:          device.IfIndex,
			NetworkDirection: manager.Egress,
		}

		ev.probe.tcProgramsLock.RLock()
		defer ev.probe.tcProgramsLock.RUnlock()

		tcProbe, ok := ev.probe.tcPrograms[key]
		if !ok {
			key.NetworkDirection = manager.Ingress
			tcProbe = ev.probe.tcPrograms[key]
		}

		if tcProbe != nil {
			device.IfName = tcProbe.IfName
		}
	}
	if ev.probe == nil {
		fmt.Println("NILLY")
	}
	return device.IfName
}

// NewEvent returns a new event
func NewEvent(resolvers *Resolvers, scrubber *pconfig.DataScrubber, probe *Probe) *Event {
	return &Event{
		Event:     model.Event{},
		resolvers: resolvers,
		scrubber:  scrubber,
		probe:     probe,
	}
}
