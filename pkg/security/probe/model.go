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
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

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
}

// GetPathResolutionError returns the path resolution error as a string if there is one
func (ev *Event) GetPathResolutionError() error {
	return ev.pathResolutionError
}

// ResolveFileInode resolves the inode to a full path
func (ev *Event) ResolveFileInode(f *model.FileEvent) string {
	if len(f.PathnameStr) == 0 {
		path, err := ev.resolvers.resolveInode(&f.FileFields)
		if err != nil {
			if _, ok := err.(ErrTruncatedSegment); ok {
				ev.SetPathResolutionError(err)
			} else if _, ok := err.(ErrTruncatedParents); ok {
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

// GetXAttrName returns the string representation of the extended attribute name
func (ev *Event) GetXAttrName(e *model.SetXAttrEvent) string {
	if len(e.Name) == 0 {
		e.Name = string(bytes.Trim(e.NameRaw[:], "\x00"))
	}
	return e.Name
}

// GetXAttrNamespace returns the string representation of the extended attribute namespace
func (ev *Event) GetXAttrNamespace(e *model.SetXAttrEvent) string {
	if len(e.Namespace) == 0 {
		fragments := strings.Split(ev.GetXAttrName(e), ".")
		if len(fragments) > 0 {
			e.Namespace = fragments[0]
		}
	}
	return e.Namespace
}

// ResolveMountPoint resolves the mountpoint to a full path
func (ev *Event) ResolveMountPoint(e *model.MountEvent) string {
	if len(e.MountPointStr) == 0 {
		e.MountPointStr, e.MountPointPathResolutionError = ev.resolvers.DentryResolver.Resolve(e.ParentMountID, e.ParentInode, 0)
	}
	return e.MountPointStr
}

// ResolveMountRoot resolves the mountpoint to a full path
func (ev *Event) ResolveMountRoot(e *model.MountEvent) string {
	if len(e.RootStr) == 0 {
		e.RootStr, e.RootPathResolutionError = ev.resolvers.DentryResolver.Resolve(e.RootMountID, e.RootInode, 0)
	}
	return e.RootStr
}

// ResolveContainerID resolves the container ID of the event
func (ev *Event) ResolveContainerID(e *model.ContainerContext) string {
	if len(e.ID) == 0 {
		if entry := ev.ResolveProcessCacheEntry(); entry != nil {
			e.ID = entry.ID
		}
	}
	return e.ID
}

// UnmarshalExecEvent unmarshal an ExecEvent
func (ev *Event) UnmarshalExecEvent(data []byte) (int, error) {
	if len(data) < 136 {
		return 0, model.ErrNotEnoughData
	}

	// reset the process cache entry of the current event
	entry := NewProcessCacheEntry()
	entry.ContainerContext = ev.Container
	entry.ProcessContext = model.ProcessContext{
		Pid: ev.Process.Pid,
		Tid: ev.Process.Tid,
		UID: ev.Process.UID,
		GID: ev.Process.GID,
	}
	ev.processCacheEntry = entry

	n, err := ev.resolvers.ProcessResolver.unmarshalProcessCacheEntry(ev.processCacheEntry, data, false)
	if err != nil {
		return n, err
	}

	// Some fields need to be copied manually in the ExecEvent structure because they do not have "Exec" specific
	// resolvers, and the data was parsed in the ProcessCacheEntry structure
	ev.Exec.FileFields = ev.processCacheEntry.ProcessContext.ExecEvent.FileFields
	return n, nil
}

// ResolveUID resolves the user id of the file to a username
func (ev *Event) ResolveUID(e *model.FileFields) string {
	if len(e.User) == 0 {
		e.User, _ = ev.resolvers.UserGroupResolver.ResolveUser(int(e.UID))
	}
	return e.User
}

// ResolveGID resolves the group id of the file to a group name
func (ev *Event) ResolveGID(e *model.FileFields) string {
	if len(e.Group) == 0 {
		e.Group, _ = ev.resolvers.UserGroupResolver.ResolveGroup(int(e.GID))
	}
	return e.Group
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

// ResolveExecPPID resolves the parent process ID
func (ev *Event) ResolveExecPPID(e *model.ExecEvent) int {
	if e.PPid == 0 && ev != nil {
		if entry := ev.ResolveProcessCacheEntry(); entry != nil {
			e.PPid = entry.PPid
		}
	}
	return int(e.PPid)
}

// ResolveExecInode resolves the executable inode to a full path
func (ev *Event) ResolveExecInode(e *model.ExecEvent) string {
	if len(e.PathnameStr) == 0 && ev != nil {
		if entry := ev.ResolveProcessCacheEntry(); entry != nil {
			e.PathnameStr = entry.PathnameStr
		}
	}
	return e.PathnameStr
}

// ResolveExecContainerPath resolves the inode to a path relative to the container
func (ev *Event) ResolveExecContainerPath(e *model.ExecEvent) string {
	if len(e.ContainerPath) == 0 && ev != nil {
		if entry := ev.ResolveProcessCacheEntry(); entry != nil {
			e.ContainerPath = entry.ContainerPath
		}
	}
	return e.ContainerPath
}

// ResolveExecBasename resolves the inode to a filename
func (ev *Event) ResolveExecBasename(e *model.ExecEvent) string {
	if len(e.BasenameStr) == 0 {
		if e.PathnameStr == "" {
			e.PathnameStr = ev.ResolveExecInode(e)
		}

		e.BasenameStr = path.Base(e.PathnameStr)
	}
	return e.BasenameStr
}

// ResolveExecCookie resolves the cookie of the process
func (ev *Event) ResolveExecCookie(e *model.ExecEvent) int {
	if e.Cookie == 0 && ev != nil {
		if entry := ev.ResolveProcessCacheEntry(); entry != nil {
			e.Cookie = entry.Cookie
		}
	}
	return int(e.Cookie)
}

// ResolveExecTTY resolves the name of the process tty
func (ev *Event) ResolveExecTTY(e *model.ExecEvent) string {
	if e.TTYName == "" && ev != nil {
		if entry := ev.ResolveProcessCacheEntry(); entry != nil {
			e.TTYName = entry.TTYName
		}
	}
	return e.TTYName
}

// ResolveExecComm resolves the comm of the process
func (ev *Event) ResolveExecComm(e *model.ExecEvent) string {
	if len(e.Comm) == 0 && ev != nil {
		if entry := ev.ResolveProcessCacheEntry(); entry != nil {
			e.Comm = entry.Comm
		}
	}
	return e.Comm
}

// ResolveExecUID resolves the user id of the process
func (ev *Event) ResolveExecUID(e *model.ExecEvent) int {
	if e.UID == 0 && ev != nil {
		if entry := ev.ResolveProcessCacheEntry(); entry != nil {
			e.UID = entry.UID
		}
	}
	return int(e.UID)
}

// ResolveExecGID resolves the group id of the process
func (ev *Event) ResolveExecGID(e *model.ExecEvent) int {
	if e.GID == 0 && ev != nil {
		if entry := ev.ResolveProcessCacheEntry(); entry != nil {
			e.GID = entry.GID
		}
	}
	return int(e.GID)
}

// ResolveExecUser resolves the user id of the process to a username
func (ev *Event) ResolveExecUser(e *model.ExecEvent) string {
	if len(e.User) == 0 && ev != nil {
		e.User, _ = ev.resolvers.UserGroupResolver.ResolveUser(int(ev.Process.UID))
	}
	return e.User
}

// ResolveExecGroup resolves the group id of the process to a group name
func (ev *Event) ResolveExecGroup(e *model.ExecEvent) string {
	if len(e.Group) == 0 && ev != nil {
		e.Group, _ = ev.resolvers.UserGroupResolver.ResolveGroup(int(ev.Process.GID))
	}
	return e.Group
}

// ResolveExecForkTimestamp returns the fork timestamp of the process
func (ev *Event) ResolveExecForkTimestamp(e *model.ExecEvent) time.Time {
	if e.ForkTime.IsZero() && ev != nil {
		if entry := ev.ResolveProcessCacheEntry(); entry != nil {
			e.ForkTime = entry.ForkTime
		}
	}
	return e.ForkTime
}

// ResolveExecExecTimestamp returns the execve timestamp of the process
func (ev *Event) ResolveExecExecTimestamp(e *model.ExecEvent) time.Time {
	if e.ExecTime.IsZero() && ev != nil {
		if entry := ev.ResolveProcessCacheEntry(); entry != nil {
			e.ExecTime = entry.ExecTime
		}
	}
	return e.ExecTime
}

// ResolveExecExitTimestamp returns the exit timestamp of the process
func (ev *Event) ResolveExecExitTimestamp(e *model.ExecEvent) time.Time {
	if e.ExitTime.IsZero() && ev != nil {
		if entry := ev.ResolveProcessCacheEntry(); entry != nil {
			e.ExitTime = entry.ExitTime
		}
	}
	return e.ExitTime
}

// NewProcessCacheEntry returns an empty instance of ProcessCacheEntry
func NewProcessCacheEntry() *model.ProcessCacheEntry {
	return &model.ProcessCacheEntry{}
}

// ResolveProcessUser resolves the user id of the process to a username
func (ev *Event) ResolveProcessUser(p *model.ProcessContext) string {
	return ev.resolvers.ResolveProcessUser(p)
}

// ResolveProcessGroup resolves the group id of the process to a group name
func (ev *Event) ResolveProcessGroup(p *model.ProcessContext) string {
	return ev.resolvers.ResolveProcessGroup(p)
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
		ev.processCacheEntry = ev.resolvers.ProcessResolver.Resolve(ev.Process.Pid)
		if ev.processCacheEntry == nil {
			ev.processCacheEntry = &model.ProcessCacheEntry{}
		}
	}
	ev.Process.Ancestor = ev.processCacheEntry.Ancestor
	return ev.processCacheEntry
}

// updateProcessCachePointer updates the internal pointers of the event structure to the ProcessCacheEntry of the event
func (ev *Event) updateProcessCachePointer(entry *model.ProcessCacheEntry) {
	ev.processCacheEntry = entry
	ev.Process.Ancestor = entry.Ancestor
}

// Clone returns a copy on the event
func (ev *Event) Clone() Event {
	return *ev
}

// NewEvent returns a new event
func NewEvent(resolvers *Resolvers) *Event {
	return &Event{
		Event:     model.Event{},
		resolvers: resolvers,
	}
}
