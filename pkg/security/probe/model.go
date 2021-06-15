// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probe

import (
	"bytes"
	"encoding/json"
	"path"
	"strings"
	"syscall"
	"time"

	pconfig "github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/security/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

const (
	// ServiceEnvVar environment variable used to report service
	ServiceEnvVar = "DD_SERVICE"
)

var eventZero Event

// Model describes the data model for the runtime security agent probe events
type Model struct {
	model.Model
}

// NewEvent returns a new Event
func (m *Model) NewEvent() eval.Event {
	return &Event{Event: model.Event{}}
}

// Event describes a probe event
type Event struct {
	model.Event

	resolvers           *Resolvers
	processCacheEntry   *model.ProcessCacheEntry
	pathResolutionError error
	scrubber            *pconfig.DataScrubber
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
	if len(f.PathnameStr) == 0 {
		path, err := ev.resolvers.resolveFileFieldsPath(&f.FileFields)
		if err != nil {
			if _, ok := err.(ErrTruncatedParents); ok {
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

// ResolveFileContainerPath resolves the inode to a full path
func (ev *Event) ResolveFileContainerPath(f *model.FileEvent) string {
	if len(f.ContainerPath) == 0 {
		f.ContainerPath = ev.resolvers.resolveContainerPath(&f.FileFields)
	}
	return f.ContainerPath
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
		e.Name = string(bytes.Trim(e.NameRaw[:], "\x00"))
	}
	return e.Name
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
	e.MountPointStr, e.MountPointPathResolutionError = ev.resolvers.DentryResolver.Resolve(e.ParentMountID, e.ParentInode, 0)
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
	e.RootStr, e.RootPathResolutionError = ev.resolvers.DentryResolver.Resolve(e.RootMountID, e.RootInode, 0)
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

// ResolveExecArgs resolves the args of the event
func (ev *Event) ResolveExecArgs(e *model.ExecEvent) string {
	if ev.Exec.Args == "" {
		ev.Exec.Args = strings.Join(ev.ResolveExecArgv(e), " ")
	}
	return ev.Exec.Args
}

// ResolveExecArgv resolves the args of the event as an array
func (ev *Event) ResolveExecArgv(e *model.ExecEvent) []string {
	if len(ev.Exec.Argv) == 0 {
		ev.Exec.Argv, ev.Exec.ArgsTruncated = ev.resolvers.ProcessResolver.GetProcessArgv(&e.Process)
	}
	return ev.Exec.Argv
}

// ResolveExecArgsTruncated returns whether the args are truncated
func (ev *Event) ResolveExecArgsTruncated(e *model.ExecEvent) bool {
	_ = ev.ResolveExecArgs(e)
	return ev.Exec.ArgsTruncated
}

// ResolveExecArgsFlags resolves the arguments flags of the event
func (ev *Event) ResolveExecArgsFlags(e *model.ExecEvent) (flags []string) {
	for _, arg := range ev.ResolveExecArgv(e) {
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

// ResolveExecArgsOptions resolves the arguments options of the event
func (ev *Event) ResolveExecArgsOptions(e *model.ExecEvent) (options []string) {
	args := ev.ResolveExecArgv(e)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if len(arg) > 1 && arg[0] == '-' {
			name := arg[1:]
			if len(name) >= 1 && name[0] == '-' {
				name = name[1:]
			}
			if len(name) > 0 && model.IsAlphaNumeric(rune(name[0])) {
				if index := strings.IndexRune(name, '='); index == -1 {
					if i < len(args)-1 && args[i+1][0] != '-' {
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

// ResolveExecEnvsTruncated returns whether the envs are truncated
func (ev *Event) ResolveExecEnvsTruncated(e *model.ExecEvent) bool {
	_ = ev.ResolveExecEnvs(e)
	return ev.Exec.EnvsTruncated
}

// ResolveExecEnvs resolves the envs of the event
func (ev *Event) ResolveExecEnvs(e *model.ExecEvent) []string {
	if len(e.Envs) == 0 {
		envs, truncated := ev.resolvers.ProcessResolver.GetProcessEnvs(&e.Process)
		if envs != nil {
			ev.Exec.Envs = make([]string, 0, len(envs))
			for key := range envs {
				ev.Exec.Envs = append(ev.Exec.Envs, key)
			}
			ev.Exec.EnvsTruncated = truncated
		}
	}
	return ev.Exec.Envs
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

func (ev *Event) String() string {
	d, err := json.Marshal(ev)
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
	s := newEventSerializer(ev)
	return json.Marshal(s)
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

// NewEvent returns a new event
func NewEvent(resolvers *Resolvers, scrubber *pconfig.DataScrubber) *Event {
	return &Event{
		Event:     model.Event{},
		resolvers: resolvers,
		scrubber:  scrubber,
	}
}
