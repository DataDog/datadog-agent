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
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/mailru/easyjson/jwriter"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
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

	probe     *Probe
	resolvers *Resolvers
}

// ValidateField validates the value of a field
func (m *Model) ValidateField(field eval.Field, fieldValue eval.FieldValue) error {
	if err := m.Model.ValidateField(field, fieldValue); err != nil {
		return err
	}

	switch field {
	case "bpf.map.name":
		if offset, found := m.probe.constantOffsets[constantfetch.OffsetNameBPFMapStructName]; !found || offset == constantfetch.ErrorSentinel {
			return fmt.Errorf("%s is not available on this kernel version", field)
		}

	case "bpf.prog.name":
		if offset, found := m.probe.constantOffsets[constantfetch.OffsetNameBPFProgAuxStructName]; !found || offset == constantfetch.ErrorSentinel {
			return fmt.Errorf("%s is not available on this kernel version", field)
		}
	}

	return nil
}

// NewEvent returns a new Event
func (m *Model) NewEvent() eval.Event {
	return &Event{}
}

// Event describes a probe event
type Event struct {
	model.Event
}

// ResolveFilePath resolves the inode to a full path
func (m *Model) ResolveFilePath(ev *model.Event, f *model.FileEvent) string {
	if !f.IsPathnameStrResolved && len(f.PathnameStr) == 0 {
		path, err := m.resolvers.resolveFileFieldsPath(&f.FileFields, &ev.PIDContext, &ev.ContainerContext)
		if err != nil {
			ev.SetPathResolutionError(f, err)
		}
		f.SetPathnameStr(path)
	}

	return f.PathnameStr
}

// ResolveFileBasename resolves the inode to a full path
func (m *Model) ResolveFileBasename(ev *model.Event, f *model.FileEvent) string {
	if !f.IsBasenameStrResolved && len(f.BasenameStr) == 0 {
		if f.PathnameStr != "" {
			f.SetBasenameStr(path.Base(f.PathnameStr))
		} else {
			f.SetBasenameStr(m.resolvers.resolveBasename(&f.FileFields))
		}
	}
	return f.BasenameStr
}

// ResolveFileFilesystem resolves the filesystem a file resides in
func (m *Model) ResolveFileFilesystem(ev *model.Event, f *model.FileEvent) string {
	if f.Filesystem == "" && !f.IsFileless() {
		fs, err := m.resolvers.MountResolver.ResolveFilesystem(f.FileFields.MountID, ev.PIDContext.Pid, ev.ContainerContext.ID)
		if err != nil {
			ev.SetPathResolutionError(f, err)
		}
		f.Filesystem = fs
	}
	return f.Filesystem
}

// ResolveFileFieldsInUpperLayer resolves whether the file is in an upper layer
func (m *Model) ResolveFileFieldsInUpperLayer(ev *model.Event, f *model.FileFields) bool {
	return f.GetInUpperLayer()
}

// ResolveXAttrName returns the string representation of the extended attribute name
func (m *Model) ResolveXAttrName(ev *model.Event, e *model.SetXAttrEvent) string {
	if len(e.Name) == 0 {
		e.Name, _ = model.UnmarshalString(e.NameRaw[:], 200)
	}
	return e.Name
}

// ResolveHelpers returns the list of eBPF helpers used by the current program
func (m *Model) ResolveHelpers(ev *model.Event, e *model.BPFProgram) []uint32 {
	return e.Helpers
}

// ResolveXAttrNamespace returns the string representation of the extended attribute namespace
func (m *Model) ResolveXAttrNamespace(ev *model.Event, e *model.SetXAttrEvent) string {
	if len(e.Namespace) == 0 {
		ns, _, found := strings.Cut(m.ResolveXAttrName(ev, e), ".")
		if found {
			e.Namespace = ns
		}
	}
	return e.Namespace
}

// SetMountPoint set the mount point information
func (m *Model) SetMountPoint(ev *model.Event, e *model.Mount) error {
	var err error
	e.MountPointStr, err = m.resolvers.DentryResolver.Resolve(e.ParentMountID, e.ParentInode, 0, true)
	return err
}

// ResolveMountPoint resolves the mountpoint to a full path
func (m *Model) ResolveMountPoint(ev *model.Event, e *model.Mount) (string, error) {
	if len(e.MountPointStr) == 0 {
		if err := m.SetMountPoint(ev, e); err != nil {
			return "", err
		}
	}
	return e.MountPointStr, nil
}

// SetMountRoot set the mount point information
func (m *Model) SetMountRoot(ev *model.Event, e *model.Mount) error {
	var err error
	e.RootStr, err = m.resolvers.DentryResolver.Resolve(e.RootMountID, e.RootInode, 0, true)
	return err
}

// ResolveMountRoot resolves the mountpoint to a full path
func (m *Model) ResolveMountRoot(ev *model.Event, e *model.Mount) (string, error) {
	if len(e.RootStr) == 0 {
		if err := m.SetMountRoot(ev, e); err != nil {
			return "", err
		}
	}
	return e.RootStr, nil
}

func (m *Model) ResolveMountPointPath(ev *model.Event, e *model.MountEvent) string {
	if len(e.MountPointPath) == 0 {
		mountPointPath, err := m.resolvers.MountResolver.ResolveMountPath(e.MountID, ev.PIDContext.Pid, ev.ContainerContext.ID)
		if err != nil {
			e.MountPointPathResolutionError = err
			return ""
		}
		e.MountPointPath = mountPointPath
	}
	return e.MountPointPath
}

func (m *Model) ResolveMountSourcePath(ev *model.Event, e *model.MountEvent) string {
	if e.BindSrcMountID != 0 && len(e.MountSourcePath) == 0 {
		bindSourceMountPath, err := m.resolvers.MountResolver.ResolveMountPath(e.BindSrcMountID, ev.PIDContext.Pid, ev.ContainerContext.ID)
		if err != nil {
			e.MountSourcePathResolutionError = err
			return ""
		}
		rootStr, err := m.ResolveMountRoot(ev, &e.Mount)
		if err != nil {
			e.MountSourcePathResolutionError = err
			return ""
		}
		e.MountSourcePath = path.Join(bindSourceMountPath, rootStr)
	}
	return e.MountSourcePath
}

// ResolveContainerID resolves the container ID of the event
func (m *Model) ResolveContainerID(ev *model.Event, e *model.ContainerContext) string {
	if len(e.ID) == 0 {
		if entry, _ := m.ResolveProcessCacheEntry(ev); entry != nil {
			e.ID = entry.ContainerID
		}
	}
	return e.ID
}

// ResolveContainerTags resolves the container tags of the event
func (m *Model) ResolveContainerTags(ev *model.Event, e *model.ContainerContext) []string {
	if len(e.Tags) == 0 && e.ID != "" {
		e.Tags = m.resolvers.TagsResolver.Resolve(e.ID)
	}
	return e.Tags
}

// UnmarshalProcessCacheEntry unmarshal a Process
func (m *Model) UnmarshalProcessCacheEntry(ev *model.Event, data []byte) (int, error) {
	entry := m.resolvers.ProcessResolver.NewProcessCacheEntry(ev.PIDContext)
	ev.ProcessCacheEntry = entry

	n, err := entry.Process.UnmarshalBinary(data)
	if err != nil {
		return n, err
	}
	entry.Process.ContainerID = ev.ContainerContext.ID

	return n, nil
}

// ResolveRights resolves the rights of a file
func (m *Model) ResolveRights(ev *model.Event, e *model.FileFields) int {
	return int(e.Mode) & (syscall.S_ISUID | syscall.S_ISGID | syscall.S_ISVTX | syscall.S_IRWXU | syscall.S_IRWXG | syscall.S_IRWXO)
}

// ResolveChownUID resolves the user id of a chown event to a username
func (m *Model) ResolveChownUID(ev *model.Event, e *model.ChownEvent) string {
	if len(e.User) == 0 {
		e.User, _ = m.resolvers.UserGroupResolver.ResolveUser(int(e.UID))
	}
	return e.User
}

// ResolveChownGID resolves the group id of a chown event to a group name
func (m *Model) ResolveChownGID(ev *model.Event, e *model.ChownEvent) string {
	if len(e.Group) == 0 {
		e.Group, _ = m.resolvers.UserGroupResolver.ResolveGroup(int(e.GID))
	}
	return e.Group
}

// ResolveProcessCreatedAt resolves process creation time
func (m *Model) ResolveProcessCreatedAt(ev *model.Event, e *model.Process) uint64 {
	return uint64(e.ExecTime.UnixNano())
}

// ResolveProcessArgv0 resolves the first arg of the event
func (m *Model) ResolveProcessArgv0(ev *model.Event, process *model.Process) string {
	arg0, _ := m.resolvers.ProcessResolver.GetProcessArgv0(process)
	return arg0
}

// ResolveProcessArgs resolves the args of the event
func (m *Model) ResolveProcessArgs(ev *model.Event, process *model.Process) string {
	return strings.Join(m.ResolveProcessArgv(ev, process), " ")
}

// ResolveProcessArgv resolves the args of the event as an array
func (m *Model) ResolveProcessArgv(ev *model.Event, process *model.Process) []string {
	argv, _ := m.resolvers.ProcessResolver.GetProcessArgv(process)
	return argv
}

// ResolveProcessEnvp resolves the envp of the event as an array
func (m *Model) ResolveProcessEnvp(process *model.Process) []string {
	envp, _ := m.resolvers.ProcessResolver.GetProcessEnvp(process)
	return envp
}

// ResolveProcessArgsTruncated returns whether the args are truncated
func (m *Model) ResolveProcessArgsTruncated(process *model.Process) bool {
	_, truncated := m.resolvers.ProcessResolver.GetProcessArgv(process)
	return truncated
}

// ResolveProcessArgsFlags resolves the arguments flags of the event
func (m *Model) ResolveProcessArgsFlags(ev *model.Event, process *model.Process) (flags []string) {
	for _, arg := range m.ResolveProcessArgv(ev, process) {
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
func (m *Model) ResolveProcessArgsOptions(ev *model.Event, process *model.Process) (options []string) {
	args := m.ResolveProcessArgv(ev, process)
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
func (m *Model) ResolveProcessEnvsTruncated(ev *model.Event, process *model.Process) bool {
	_, truncated := m.resolvers.ProcessResolver.GetProcessEnvs(process)
	return truncated
}

// ResolveProcessEnvs resolves the envs of the event
func (m *Model) ResolveProcessEnvs(ev *model.Event, process *model.Process) []string {
	envs, _ := m.resolvers.ProcessResolver.GetProcessEnvs(process)
	return envs
}

// ResolveSetuidUser resolves the user of the Setuid event
func (m *Model) ResolveSetuidUser(ev *model.Event, e *model.SetuidEvent) string {
	if len(e.User) == 0 {
		e.User, _ = m.resolvers.UserGroupResolver.ResolveUser(int(e.UID))
	}
	return e.User
}

// ResolveSetuidEUser resolves the effective user of the Setuid event
func (m *Model) ResolveSetuidEUser(ev *model.Event, e *model.SetuidEvent) string {
	if len(e.EUser) == 0 {
		e.EUser, _ = m.resolvers.UserGroupResolver.ResolveUser(int(e.EUID))
	}
	return e.EUser
}

// ResolveSetuidFSUser resolves the file-system user of the Setuid event
func (m *Model) ResolveSetuidFSUser(ev *model.Event, e *model.SetuidEvent) string {
	if len(e.FSUser) == 0 {
		e.FSUser, _ = m.resolvers.UserGroupResolver.ResolveUser(int(e.FSUID))
	}
	return e.FSUser
}

// ResolveSetgidGroup resolves the group of the Setgid event
func (m *Model) ResolveSetgidGroup(ev *model.Event, e *model.SetgidEvent) string {
	if len(e.Group) == 0 {
		e.Group, _ = m.resolvers.UserGroupResolver.ResolveUser(int(e.GID))
	}
	return e.Group
}

// ResolveSetgidEGroup resolves the effective group of the Setgid event
func (m *Model) ResolveSetgidEGroup(ev *model.Event, e *model.SetgidEvent) string {
	if len(e.EGroup) == 0 {
		e.EGroup, _ = m.resolvers.UserGroupResolver.ResolveUser(int(e.EGID))
	}
	return e.EGroup
}

// ResolveSetgidFSGroup resolves the file-system group of the Setgid event
func (m *Model) ResolveSetgidFSGroup(ev *model.Event, e *model.SetgidEvent) string {
	if len(e.FSGroup) == 0 {
		e.FSGroup, _ = m.resolvers.UserGroupResolver.ResolveUser(int(e.FSGID))
	}
	return e.FSGroup
}

// ResolveSELinuxBoolName resolves the boolean name of the SELinux event
func (m *Model) ResolveSELinuxBoolName(ev *model.Event, e *model.SELinuxEvent) string {
	if e.EventKind != model.SELinuxBoolChangeEventKind {
		return ""
	}

	if len(e.BoolName) == 0 {
		e.BoolName = m.resolvers.resolveBasename(&e.File.FileFields)
	}
	return e.BoolName
}

// MarshalJSONEvent returns the JSON encoding of the event
func MarshalJSONEvent(ev *model.Event) ([]byte, error) {
	s := NewEventSerializer(ev)
	w := &jwriter.Writer{
		Flags: jwriter.NilSliceAsEmpty | jwriter.NilMapAsEmpty,
	}
	s.MarshalEasyJSON(w)
	return w.BuildBytes()
}

// ResolveEventTimestamp resolves the monolitic kernel event timestamp to an absolute time
func (m *Model) ResolveEventTimestamp(ev *model.Event) time.Time {
	if ev.Timestamp.IsZero() {
		ev.Timestamp = m.resolvers.TimeResolver.ResolveMonotonicTimestamp(ev.TimestampRaw)
		if ev.Timestamp.IsZero() {
			ev.Timestamp = time.Now()
		}
	}
	return ev.Timestamp
}

// ResolveProcessCacheEntry queries the ProcessResolver to retrieve the ProcessContext of the event
func (m *Model) ResolveProcessCacheEntry(ev *model.Event) (*model.ProcessCacheEntry, bool) {
	if ev.PIDContext.IsKworker {
		return ev.NewEmptyProcessCacheEntry(), false
	}

	if ev.ProcessCacheEntry == nil {
		ev.ProcessCacheEntry = m.resolvers.ProcessResolver.Resolve(ev.PIDContext.Pid, ev.PIDContext.Tid)
	}

	if ev.ProcessCacheEntry == nil {
		// keep the original PIDContext
		ev.ProcessCacheEntry = model.NewProcessCacheEntry(nil)
		ev.ProcessCacheEntry.PIDContext = ev.PIDContext

		ev.ProcessCacheEntry.FileEvent.SetPathnameStr("")
		ev.ProcessCacheEntry.FileEvent.SetBasenameStr("")

		// mark interpreter as resolved too
		ev.ProcessCacheEntry.LinuxBinprm.FileEvent.SetPathnameStr("")
		ev.ProcessCacheEntry.LinuxBinprm.FileEvent.SetBasenameStr("")

		return ev.ProcessCacheEntry, false
	}

	return ev.ProcessCacheEntry, true
}

// GetProcessServiceTag returns the service tag based on the process context
func (m *Model) GetProcessServiceTag(ev *model.Event) string {
	entry, _ := m.ResolveProcessCacheEntry(ev)
	if entry == nil {
		return ""
	}

	var serviceValues []string

	// first search in the process context itself
	if entry.EnvsEntry != nil {
		if service := entry.EnvsEntry.Get(ServiceEnvVar); service != "" {
			serviceValues = append(serviceValues, service)
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
				serviceValues = append(serviceValues, service)
			}
		}
	}

	return bestGuessServiceTag(serviceValues)
}

func bestGuessServiceTag(serviceValues []string) string {
	if len(serviceValues) == 0 {
		return ""
	}

	firstGuess := serviceValues[0]

	// first we sort base on len, biggest len first
	sort.Slice(serviceValues, func(i, j int) bool {
		return len(serviceValues[j]) < len(serviceValues[i]) // reverse
	})

	// we then compare [i] and [i + 1] to check if [i + 1] is a prefix of [i]
	for i := 0; i < len(serviceValues)-1; i++ {
		if !strings.HasPrefix(serviceValues[i], serviceValues[i+1]) {
			// if it's not a prefix it means we have multiple disjoints services
			// we then return the first guess, closest in the process tree
			return firstGuess
		}
	}

	// we have a prefix chain, let's return the biggest one
	return serviceValues[0]
}

// ResolveNetworkDeviceIfName returns the network iterface name from the network context
func (m *Model) ResolveNetworkDeviceIfName(ev *model.Event, device *model.NetworkDeviceContext) string {
	if len(device.IfName) == 0 && m.resolvers.TCResolver != nil {
		ifName, ok := m.resolvers.TCResolver.ResolveNetworkDeviceIfName(device.IfIndex, device.NetNS)
		if ok {
			device.IfName = ifName
		}
	}

	return device.IfName
}

// NewEvent returns a new event
func NewEvent(resolvers *Resolvers, scrubber *procutil.DataScrubber) *Event {
	return &Event{
		Event: model.Event{},
	}
}
