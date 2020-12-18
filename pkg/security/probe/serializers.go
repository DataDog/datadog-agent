//go:generate go run github.com/mailru/easyjson/easyjson -build_tags linux $GOFILE

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

import (
	"syscall"
	"time"
)

// Event categories for JSON serialization
const (
	FIMCategory     = "File Activity"
	ProcessActivity = "Process Activity"
)

// FileSerializer serializes a file to JSON
// easyjson:json
type FileSerializer struct {
	Path            string     `json:"path,omitempty"`
	Name            string     `json:"name,omitempty"`
	ContainerPath   string     `json:"container_path,omitempty"`
	Inode           *uint64    `json:"inode,omitempty"`
	Mode            *uint32    `json:"mode,omitempty"`
	OverlayNumLower *int32     `json:"overlay_numlower,omitempty"`
	MountID         *uint32    `json:"mount_id,omitempty"`
	UID             *int32     `json:"uid,omitempty"`
	GID             *int32     `json:"gid,omitempty"`
	XAttrName       string     `json:"attribute_name,omitempty"`
	XAttrNamespace  string     `json:"attribute_namespace,omitempty"`
	Flags           []string   `json:"flags,omitempty"`
	Atime           *time.Time `json:"access_time,omitempty"`
	Mtime           *time.Time `json:"modification_time,omitempty"`
}

// UserContextSerializer serializes a user context to JSON
// easyjson:json
type UserContextSerializer struct {
	User  string `json:"user,omitempty"`
	Group string `json:"group,omitempty"`
}

// ProcessCacheEntrySerializer serializes a process cache entry to JSON
// easyjson:json
type ProcessCacheEntrySerializer struct {
	UserContextSerializer
	Pid           uint32     `json:"pid"`
	PPid          uint32     `json:"ppid"`
	Tid           uint32     `json:"tid"`
	UID           uint32     `json:"uid"`
	GID           uint32     `json:"gid"`
	Name          string     `json:"name"`
	ContainerPath string     `json:"executable_container_path,omitempty"`
	Path          string     `json:"executable_path"`
	Inode         uint64     `json:"executable_inode"`
	MountID       uint32     `json:"executable_mount_id"`
	TTY           string     `json:"tty,omitempty"`
	ForkTime      *time.Time `json:"fork_time,omitempty"`
	ExecTime      *time.Time `json:"exec_time,omitempty"`
	ExitTime      *time.Time `json:"exit_time,omitempty"`
}

// ContainerContextSerializer serializes a container context to JSON
// easyjson:json
type ContainerContextSerializer struct {
	ID string `json:"id,omitempty"`
}

// FileEventSerializer serializes a file event to JSON
// easyjson:json
type FileEventSerializer struct {
	FileSerializer `json:",omitempty"`
	Destination    *FileSerializer `json:"destination,omitempty"`

	// Specific to mount events
	NewMountID uint32 `json:"new_mount_id,omitempty"`
	GroupID    uint32 `json:"group_id,omitempty"`
	Device     uint32 `json:"device,omitempty"`
	FSType     string `json:"fstype,omitempty"`
}

// EventContextSerializer serializes an event context to JSON
// easyjson:json
type EventContextSerializer struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Outcome  string `json:"outcome"`
}

// ProcessContextSerializer serializes a process context to JSON
// easyjson:json
type ProcessContextSerializer struct {
	*ProcessCacheEntrySerializer
	Parent    *ProcessCacheEntrySerializer   `json:"parent"`
	Ancestors []*ProcessCacheEntrySerializer `json:"ancestors"`
}

// EventSerializer serializes an event to JSON
// easyjson:json
type EventSerializer struct {
	*EventContextSerializer    `json:"evt"`
	*FileEventSerializer       `json:"file,omitempty"`
	UserContextSerializer      UserContextSerializer       `json:"usr"`
	ProcessContextSerializer   *ProcessContextSerializer   `json:"process"`
	ContainerContextSerializer *ContainerContextSerializer `json:"container,omitempty"`
}

func newFileSerializer(fe *FileEvent, e *Event) *FileSerializer {
	return &FileSerializer{
		Path:            fe.ResolveInode(e),
		ContainerPath:   fe.ResolveContainerPath(e),
		Inode:           getUint64Pointer(&fe.Inode),
		MountID:         getUint32Pointer(&fe.MountID),
		OverlayNumLower: getInt32Pointer(&fe.OverlayNumLower),
	}
}

func getUint64Pointer(i *uint64) *uint64 {
	if *i == 0 {
		return nil
	}
	return i
}

func getUint32Pointer(i *uint32) *uint32 {
	if *i == 0 {
		return nil
	}
	return i
}

func getInt32Pointer(i *int32) *int32 {
	if *i == 0 {
		return nil
	}
	return i
}

func getTimeIfNotZero(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func newProcessCacheEntrySerializer(pce *ProcessCacheEntry, e *Event, useEvent bool) *ProcessCacheEntrySerializer {
	var pid, ppid, tid, uid, gid uint32
	var user, group string

	if useEvent {
		pid = e.Process.Pid
		ppid = e.Process.PPid
		tid = e.Process.Tid
		uid = e.Process.UID
		gid = e.Process.GID
		user = e.Process.ResolveUser(e)
		group = e.Process.ResolveGroup(e)
	} else {
		pid = pce.Pid
		ppid = pce.PPid
		tid = pce.Tid
		uid = pce.UID
		gid = pce.GID
		user = pce.ResolveUser(e)
		group = pce.ResolveGroup(e)
	}

	return &ProcessCacheEntrySerializer{
		UserContextSerializer: UserContextSerializer{
			User:  user,
			Group: group,
		},
		Pid:      pid,
		PPid:     ppid,
		Tid:      tid,
		UID:      uid,
		GID:      gid,
		Name:     pce.Comm,
		Path:     pce.ResolveInode(e),
		Inode:    pce.Inode,
		MountID:  pce.MountID,
		TTY:      pce.ResolveTTY(e),
		ForkTime: getTimeIfNotZero(pce.ForkTimestamp),
		ExecTime: getTimeIfNotZero(pce.ExecTimestamp),
		ExitTime: getTimeIfNotZero(pce.ExitTimestamp),
	}
}

func newContainerContextSerializer(cc *ContainerContext, e *Event) *ContainerContextSerializer {
	return &ContainerContextSerializer{
		ID: cc.ResolveContainerID(e),
	}
}

func newProcessContextSerializer(pc *ProcessContext, e *Event) *ProcessContextSerializer {
	entry := e.ResolveProcessCacheEntry()

	ps := &ProcessContextSerializer{
		ProcessCacheEntrySerializer: newProcessCacheEntrySerializer(entry, e, true),
	}

	ancestor := entry.Parent
	for i := 0; ancestor != nil && len(ancestor.PathnameStr) > 0; i++ {
		s := newProcessCacheEntrySerializer(ancestor, e, false)
		ps.Ancestors = append(ps.Ancestors, s)
		if i == 0 {
			ps.Parent = s
		}
		ancestor = ancestor.Parent
	}

	return ps
}

func serializeSyscallRetval(retval int64) string {
	switch {
	case syscall.Errno(retval) == syscall.EACCES || syscall.Errno(retval) == syscall.EPERM:
		return "Refused"
	case retval < 0:
		return "Error"
	default:
		return "Success"
	}
}

func newEventSerializer(event *Event) (*EventSerializer, error) {
	s := &EventSerializer{
		EventContextSerializer: &EventContextSerializer{
			Name:     EventType(event.Type).String(),
			Category: FIMCategory,
		},
		ProcessContextSerializer: newProcessContextSerializer(&event.Process, event),
	}

	if event.Container.ID != "" {
		s.ContainerContextSerializer = newContainerContextSerializer(&event.Container, event)
	}

	s.UserContextSerializer = s.ProcessContextSerializer.UserContextSerializer

	switch EventType(event.Type) {
	case FileChmodEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Chmod.FileEvent, event),
		}
		s.FileSerializer.Mode = &event.Chmod.Mode
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Chmod.Retval)
	case FileChownEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Chown.FileEvent, event),
		}
		s.FileSerializer.UID = &event.Chown.UID
		s.FileSerializer.GID = &event.Chown.GID
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Chown.Retval)
	case FileLinkEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Link.Source, event),
			Destination:    newFileSerializer(&event.Link.Target, event),
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Link.Retval)
	case FileOpenEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Open.FileEvent, event),
		}
		s.FileSerializer.Mode = &event.Open.Mode
		s.FileSerializer.Flags = OpenFlags(event.Open.Flags).StringArray()
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Open.Retval)
	case FileMkdirEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Mkdir.FileEvent, event),
		}
		s.FileSerializer.Mode = &event.Mkdir.Mode
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Mkdir.Retval)
	case FileRmdirEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Rmdir.FileEvent, event),
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Rmdir.Retval)
	case FileUnlinkEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Unlink.FileEvent, event),
		}
		s.FileSerializer.Flags = UnlinkFlags(event.Unlink.Flags).StringArray()
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Unlink.Retval)
	case FileRenameEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Rename.Old, event),
			Destination:    newFileSerializer(&event.Rename.New, event),
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Rename.Retval)
	case FileRemoveXAttrEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.RemoveXAttr.FileEvent, event),
		}
		s.FileSerializer.XAttrName = event.RemoveXAttr.GetName(event)
		s.FileSerializer.XAttrNamespace = event.RemoveXAttr.GetNamespace(event)
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.RemoveXAttr.Retval)
	case FileSetXAttrEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.SetXAttr.FileEvent, event),
		}
		s.FileSerializer.XAttrName = event.SetXAttr.GetName(event)
		s.FileSerializer.XAttrNamespace = event.SetXAttr.GetNamespace(event)
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.SetXAttr.Retval)
	case FileUtimeEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Utimes.FileEvent, event),
		}
		s.FileSerializer.Atime = getTimeIfNotZero(event.Utimes.Atime)
		s.FileSerializer.Mtime = getTimeIfNotZero(event.Utimes.Mtime)
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Utimes.Retval)
	case FileMountEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: FileSerializer{
				Path:    event.Mount.ResolveRoot(event),
				MountID: &event.Mount.RootMountID,
				Inode:   &event.Mount.RootInode,
			},
			Destination: &FileSerializer{
				Path:    event.Mount.ResolveMountPoint(event),
				MountID: &event.Mount.ParentMountID,
				Inode:   &event.Mount.ParentInode,
			},
			NewMountID: event.Mount.MountID,
			GroupID:    event.Mount.GroupID,
			Device:     event.Mount.Device,
			FSType:     event.Mount.GetFSType(),
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Mount.Retval)
	case FileUmountEventType:
		s.FileEventSerializer = &FileEventSerializer{
			NewMountID: event.Umount.MountID,
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Umount.Retval)
	case ForkEventType:
		s.EventContextSerializer.Outcome = serializeSyscallRetval(0)
	case ExitEventType:
		s.EventContextSerializer.Outcome = serializeSyscallRetval(0)
	case ExecEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.processCacheEntry.FileEvent, event),
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(0)
		s.Category = ProcessActivity
	}

	return s, nil
}
