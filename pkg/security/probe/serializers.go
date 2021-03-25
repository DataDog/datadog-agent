//go:generate go run github.com/mailru/easyjson/easyjson -build_tags linux $GOFILE

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probe

import (
	"strings"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

// Event categories for JSON serialization
const (
	FIMCategory     = "File Activity"
	ProcessActivity = "Process Activity"
)

// FileSerializer serializes a file to JSON
// easyjson:json
type FileSerializer struct {
	Path                string     `json:"path,omitempty"`
	Name                string     `json:"name,omitempty"`
	ContainerPath       string     `json:"container_path,omitempty"`
	PathResolutionError string     `json:"path_resolution_error,omitempty"`
	Inode               *uint64    `json:"inode,omitempty"`
	Mode                *uint32    `json:"mode,omitempty"`
	InUpperLayer        *bool      `json:"in_upper_layer,omitempty"`
	MountID             *uint32    `json:"mount_id,omitempty"`
	Filesystem          string     `json:"filesystem,omitempty"`
	UID                 uint32     `json:"uid,omitempty"`
	GID                 uint32     `json:"gid,omitempty"`
	User                string     `json:"user,omitempty"`
	Group               string     `json:"group,omitempty"`
	XAttrName           string     `json:"attribute_name,omitempty"`
	XAttrNamespace      string     `json:"attribute_namespace,omitempty"`
	Flags               []string   `json:"flags,omitempty"`
	Atime               *time.Time `json:"access_time,omitempty"`
	Mtime               *time.Time `json:"modification_time,omitempty"`
	Ctime               *time.Time `json:"change_time,omitempty"`
}

// UserContextSerializer serializes a user context to JSON
// easyjson:json
type UserContextSerializer struct {
	User  string `json:"id,omitempty"`
	Group string `json:"group,omitempty"`
}

// CredentialsSerializer serializes a set credentials to JSON
// easyjson:json
type CredentialsSerializer struct {
	UID          int      `json:"uid"`
	User         string   `json:"user,omitempty"`
	GID          int      `json:"gid"`
	Group        string   `json:"group,omitempty"`
	EUID         int      `json:"euid"`
	EUser        string   `json:"euser,omitempty"`
	EGID         int      `json:"egid"`
	EGroup       string   `json:"egroup,omitempty"`
	FSUID        int      `json:"fsuid"`
	FSUser       string   `json:"fsuser,omitempty"`
	FSGID        int      `json:"fsgid"`
	FSGroup      string   `json:"fsgroup,omitempty"`
	CapEffective []string `json:"cap_effective,omitempty"`
	CapPermitted []string `json:"cap_permitted,omitempty"`
}

// SetuidSerializer serializes a setuid event
// easyjson:json
type SetuidSerializer struct {
	UID    int    `json:"uid"`
	User   string `json:"user,omitempty"`
	EUID   int    `json:"euid"`
	EUser  string `json:"euser,omitempty"`
	FSUID  int    `json:"fsuid"`
	FSUser string `json:"fsuser,omitempty"`
}

// SetgidSerializer serializes a setgid event
// easyjson:json
type SetgidSerializer struct {
	GID     int    `json:"gid"`
	Group   string `json:"group,omitempty"`
	EGID    int    `json:"egid"`
	EGroup  string `json:"egroup,omitempty"`
	FSGID   int    `json:"fsgid"`
	FSGroup string `json:"fsgroup,omitempty"`
}

// CapsetSerializer serializes a capset event
// easyjson:json
type CapsetSerializer struct {
	CapEffective []string `json:"cap_effective,omitempty"`
	CapPermitted []string `json:"cap_permitted,omitempty"`
}

// ProcessCredentialsSerializer serializes the process credentials to JSON
// easyjson:json
type ProcessCredentialsSerializer struct {
	*CredentialsSerializer `json:",omitempty"`
	Destination            interface{} `json:"destination,omitempty"`
}

// ProcessCacheEntrySerializer serializes a process cache entry to JSON
// easyjson:json
type ProcessCacheEntrySerializer struct {
	Pid                 uint32                        `json:"pid,omitempty"`
	PPid                uint32                        `json:"ppid,omitempty"`
	Tid                 uint32                        `json:"tid,omitempty"`
	UID                 int                           `json:"uid"`
	GID                 int                           `json:"gid"`
	User                string                        `json:"user,omitempty"`
	Group               string                        `json:"group,omitempty"`
	ContainerPath       string                        `json:"executable_container_path,omitempty"`
	Path                string                        `json:"executable_path,omitempty"`
	PathResolutionError string                        `json:"path_resolution_error,omitempty"`
	Comm                string                        `json:"comm,omitempty"`
	Inode               uint64                        `json:"executable_inode,omitempty"`
	MountID             uint32                        `json:"executable_mount_id,omitempty"`
	Filesystem          string                        `json:"executable_filesystem,omitempty"`
	TTY                 string                        `json:"tty,omitempty"`
	ForkTime            *time.Time                    `json:"fork_time,omitempty"`
	ExecTime            *time.Time                    `json:"exec_time,omitempty"`
	ExitTime            *time.Time                    `json:"exit_time,omitempty"`
	Credentials         *ProcessCredentialsSerializer `json:"credentials,omitempty"`
	Executable          *FileSerializer               `json:"executable,omitempty"`
	Container           *ContainerContextSerializer   `json:"container,omitempty"`
	Args                []string                      `json:"args,omitempty"`
	ArgsTruncated       bool                          `json:"args_truncated,omitempty"`
	Envs                []string                      `json:"envs,omitempty"`
	EnvsTruncated       bool                          `json:"envs_truncated,omitempty"`
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
	Name     string `json:"name,omitempty"`
	Category string `json:"category,omitempty"`
	Outcome  string `json:"outcome,omitempty"`
}

// ProcessContextSerializer serializes a process context to JSON
// easyjson:json
type ProcessContextSerializer struct {
	*ProcessCacheEntrySerializer
	Parent    *ProcessCacheEntrySerializer   `json:"parent,omitempty"`
	Ancestors []*ProcessCacheEntrySerializer `json:"ancestors,omitempty"`
}

// EventSerializer serializes an event to JSON
// easyjson:json
type EventSerializer struct {
	*EventContextSerializer    `json:"evt,omitempty"`
	*FileEventSerializer       `json:"file,omitempty"`
	UserContextSerializer      UserContextSerializer       `json:"usr,omitempty"`
	ProcessContextSerializer   *ProcessContextSerializer   `json:"process,omitempty"`
	ContainerContextSerializer *ContainerContextSerializer `json:"container,omitempty"`
	Date                       time.Time                   `json:"date,omitempty"`
}

func getInUpperLayer(r *Resolvers, f *model.FileFields) *bool {
	lowerLayer := f.GetInLowerLayer()
	upperLayer := f.GetInUpperLayer()
	if !lowerLayer && !upperLayer {
		return nil
	}
	return &upperLayer
}

func newFileSerializer(fe *model.FileEvent, e *Event) *FileSerializer {
	mode := uint32(fe.FileFields.Mode)
	return &FileSerializer{
		Path:                e.ResolveFileInode(fe),
		PathResolutionError: fe.GetPathResolutionError(),
		Name:                e.ResolveFileBasename(fe),
		ContainerPath:       e.ResolveFileContainerPath(fe),
		Inode:               getUint64Pointer(&fe.Inode),
		MountID:             getUint32Pointer(&fe.MountID),
		Filesystem:          e.ResolveFileFilesystem(fe),
		Mode:                getUint32Pointer(&mode),
		UID:                 fe.UID,
		GID:                 fe.GID,
		User:                e.ResolveUser(&fe.FileFields),
		Group:               e.ResolveGroup(&fe.FileFields),
		Mtime:               &fe.MTime,
		Ctime:               &fe.CTime,
		InUpperLayer:        getInUpperLayer(e.resolvers, &fe.FileFields),
	}
}

func newProcessFileSerializer(process *model.Process, e *Event) *FileSerializer {
	mode := uint32(process.FileFields.Mode)
	return &FileSerializer{
		Path:                e.ResolveProcessInode(process),
		PathResolutionError: process.GetPathResolutionError(),
		Name:                e.ResolveProcessBasename(process),
		ContainerPath:       e.ResolveProcessContainerPath(process),
		Inode:               getUint64Pointer(&process.FileFields.Inode),
		MountID:             getUint32Pointer(&process.FileFields.MountID),
		Filesystem:          e.ResolveProcessFilesystem(process),
		InUpperLayer:        getInUpperLayer(e.resolvers, &process.FileFields),
		Mode:                getUint32Pointer(&mode),
		UID:                 process.FileFields.UID,
		GID:                 process.FileFields.GID,
		User:                e.ResolveUser(&process.FileFields),
		Group:               e.ResolveGroup(&process.FileFields),
		Mtime:               &process.FileFields.MTime,
		Ctime:               &process.FileFields.CTime,
	}
}

func newProcessFileSerializerWithResolvers(process *model.Process, r *Resolvers) *FileSerializer {
	mode := uint32(process.FileFields.Mode)
	return &FileSerializer{
		Path:                process.PathnameStr,
		PathResolutionError: process.GetPathResolutionError(),
		Name:                process.BasenameStr,
		ContainerPath:       process.ContainerPath,
		Inode:               getUint64Pointer(&process.FileFields.Inode),
		MountID:             getUint32Pointer(&process.FileFields.MountID),
		Filesystem:          r.MountResolver.GetFilesystem(process.FileFields.MountID),
		InUpperLayer:        getInUpperLayer(r, &process.FileFields),
		Mode:                getUint32Pointer(&mode),
		UID:                 process.FileFields.UID,
		GID:                 process.FileFields.GID,
		User:                r.ResolveUser(&process.FileFields),
		Group:               r.ResolveGroup(&process.FileFields),
		Mtime:               &process.FileFields.MTime,
		Ctime:               &process.FileFields.CTime,
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

func getTimeIfNotZero(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func newCredentialsSerializer(ce *model.Credentials, e *Event) *CredentialsSerializer {
	return &CredentialsSerializer{
		UID:          e.ResolveCredentialsUID(ce),
		User:         e.ResolveCredentialsUser(ce),
		EUID:         e.ResolveCredentialsEUID(ce),
		EUser:        e.ResolveCredentialsEUser(ce),
		FSUID:        e.ResolveCredentialsFSUID(ce),
		FSUser:       e.ResolveCredentialsFSUser(ce),
		GID:          e.ResolveCredentialsGID(ce),
		Group:        e.ResolveCredentialsGroup(ce),
		EGID:         e.ResolveCredentialsEGID(ce),
		EGroup:       e.ResolveCredentialsEGroup(ce),
		FSGID:        e.ResolveCredentialsFSGID(ce),
		FSGroup:      e.ResolveCredentialsFSGroup(ce),
		CapEffective: model.KernelCapability(e.ResolveCredentialsCapEffective(ce)).StringArray(),
		CapPermitted: model.KernelCapability(e.ResolveCredentialsCapPermitted(ce)).StringArray(),
	}
}

func newCredentialsSerializerWithResolvers(ce *model.Credentials, r *Resolvers) *CredentialsSerializer {
	return &CredentialsSerializer{
		UID:          int(ce.UID),
		User:         r.ResolveCredentialsUser(ce),
		EUID:         int(ce.EUID),
		EUser:        r.ResolveCredentialsEUser(ce),
		FSUID:        int(ce.FSUID),
		FSUser:       r.ResolveCredentialsFSUser(ce),
		GID:          int(ce.GID),
		Group:        r.ResolveCredentialsGroup(ce),
		EGID:         int(ce.EGID),
		EGroup:       r.ResolveCredentialsEGroup(ce),
		FSGID:        int(ce.FSGID),
		FSGroup:      r.ResolveCredentialsFSGroup(ce),
		CapEffective: model.KernelCapability(ce.CapEffective).StringArray(),
		CapPermitted: model.KernelCapability(ce.CapPermitted).StringArray(),
	}
}

func scrubArgsEnvs(process *model.Process, e *Event) ([]string, []string) {
	args := process.ArgsArray
	envs := process.EnvsArray

	// scrub args, do not send args if no scrubber instance is passed
	// can be the case for some custom event
	if e.scrubber == nil {
		args = []string{}
		envs = []string{}
	} else {
		if newArgs, changed := e.scrubber.ScrubCommand(args); changed {
			args = newArgs
		}

		// for envs, we just keep the keys
		var newEnvs []string
		for _, env := range envs {
			if els := strings.SplitN(env, "=", 2); len(els) > 0 {
				newEnvs = append(newEnvs, els[0])
			}
		}
		envs = newEnvs
	}

	return args, envs
}

func newProcessCacheEntrySerializer(pce *model.ProcessCacheEntry, e *Event, topLevel bool) *ProcessCacheEntrySerializer {
	args, envs := scrubArgsEnvs(&pce.Process, e)

	pceSerializer := &ProcessCacheEntrySerializer{
		Inode:               pce.FileFields.Inode,
		MountID:             pce.FileFields.MountID,
		PathResolutionError: pce.GetPathResolutionError(),
		ForkTime:            getTimeIfNotZero(pce.ForkTime),
		ExecTime:            getTimeIfNotZero(pce.ExecTime),
		ExitTime:            getTimeIfNotZero(pce.ExitTime),

		Pid:           e.ProcessContext.Pid,
		PPid:          e.ProcessContext.PPid,
		Tid:           e.ProcessContext.Tid,
		Path:          e.ResolveProcessInode(&pce.Process),
		ContainerPath: e.ResolveProcessContainerPath(&pce.Process),
		Comm:          e.ResolveProcessComm(&pce.Process),
		TTY:           e.ResolveProcessTTY(&pce.Process),
		Executable:    newProcessFileSerializer(&pce.Process, e),
		Args:          args,
		ArgsTruncated: pce.Process.ArgsTruncated,
		Envs:          envs,
		EnvsTruncated: pce.Process.EnvsTruncated,
	}

	credsSerializer := newCredentialsSerializer(&pce.Credentials, e)
	// Populate legacy user / group fields
	pceSerializer.UID = credsSerializer.UID
	pceSerializer.User = credsSerializer.User
	pceSerializer.GID = credsSerializer.GID
	pceSerializer.Group = credsSerializer.Group
	pceSerializer.Credentials = &ProcessCredentialsSerializer{
		CredentialsSerializer: credsSerializer,
	}

	if !topLevel && len(e.ResolveContainerID(&e.ContainerContext)) > 0 {
		pceSerializer.Container = &ContainerContextSerializer{
			ID: e.ResolveContainerID(&e.ContainerContext),
		}
	}
	return pceSerializer
}

func newProcessCacheEntrySerializerWithResolvers(pce *model.ProcessCacheEntry, r *Resolvers, topLevel bool) *ProcessCacheEntrySerializer {
	pceSerializer := &ProcessCacheEntrySerializer{
		Inode:               pce.FileFields.Inode,
		MountID:             pce.FileFields.MountID,
		PathResolutionError: pce.GetPathResolutionError(),
		ForkTime:            getTimeIfNotZero(pce.ForkTime),
		ExecTime:            getTimeIfNotZero(pce.ExecTime),
		ExitTime:            getTimeIfNotZero(pce.ExitTime),

		Pid:           pce.Pid,
		PPid:          pce.PPid,
		Tid:           pce.Tid,
		Path:          pce.Process.PathnameStr,
		ContainerPath: pce.Process.ContainerPath,
		Comm:          pce.Comm,
		TTY:           pce.TTYName,
		Executable:    newProcessFileSerializerWithResolvers(&pce.Process, r),
	}

	credsSerializer := newCredentialsSerializerWithResolvers(&pce.Credentials, r)
	// Populate legacy user / group fields
	pceSerializer.UID = credsSerializer.UID
	pceSerializer.User = credsSerializer.User
	pceSerializer.GID = credsSerializer.GID
	pceSerializer.Group = credsSerializer.Group
	pceSerializer.Credentials = &ProcessCredentialsSerializer{
		CredentialsSerializer: credsSerializer,
	}

	if !topLevel && len(pce.ContainerContext.ID) != 0 {
		pceSerializer.Container = &ContainerContextSerializer{
			ID: pce.ContainerContext.ID,
		}
	}
	return pceSerializer
}

func newContainerContextSerializer(cc *model.ContainerContext, e *Event) *ContainerContextSerializer {
	return &ContainerContextSerializer{
		ID: e.ResolveContainerID(cc),
	}
}

func newProcessContextSerializer(entry *model.ProcessCacheEntry, e *Event, r *Resolvers) *ProcessContextSerializer {
	var ps *ProcessContextSerializer

	if e != nil {
		ps = &ProcessContextSerializer{
			ProcessCacheEntrySerializer: newProcessCacheEntrySerializer(entry, e, true),
		}
	} else {
		ps = &ProcessContextSerializer{
			ProcessCacheEntrySerializer: newProcessCacheEntrySerializerWithResolvers(entry, r, true),
		}
	}

	if e == nil {
		// custom events call newProcessContextSerializer with an empty Event
		e = NewEvent(r, nil)
		e.ProcessContext = model.ProcessContext{
			Ancestor: entry,
		}
	}

	ctx := eval.NewContext(e.GetPointer())

	it := &model.ProcessAncestorsIterator{}
	ptr := it.Front(ctx)

	first := true
	for ptr != nil {
		ancestor := (*model.ProcessCacheEntry)(ptr)
		// pass nil instead of e to prevent mixing values with the ancestors
		s := newProcessCacheEntrySerializerWithResolvers(ancestor, r, false)
		ps.Ancestors = append(ps.Ancestors, s)

		if first {
			ps.Parent = s
		}
		first = false

		ptr = it.Next()
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

func newEventSerializer(event *Event) *EventSerializer {
	s := &EventSerializer{
		EventContextSerializer: &EventContextSerializer{
			Name:     model.EventType(event.Type).String(),
			Category: FIMCategory,
		},
		ProcessContextSerializer: newProcessContextSerializer(event.ResolveProcessCacheEntry(), event, event.resolvers),
		Date:                     event.ResolveEventTimestamp(),
	}

	if event.ResolveContainerID(&event.ContainerContext) != "" {
		s.ContainerContextSerializer = newContainerContextSerializer(&event.ContainerContext, event)
	}

	s.UserContextSerializer.User = s.ProcessContextSerializer.User
	s.UserContextSerializer.Group = s.ProcessContextSerializer.Group

	switch model.EventType(event.Type) {
	case model.FileChmodEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Chmod.File, event),
			Destination: &FileSerializer{
				Mode: &event.Chmod.Mode,
			},
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Chmod.Retval)
	case model.FileChownEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Chown.File, event),
			Destination: &FileSerializer{
				UID: event.Chown.UID,
				GID: event.Chown.GID,
			},
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Chown.Retval)
	case model.FileLinkEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Link.Source, event),
			Destination:    newFileSerializer(&event.Link.Target, event),
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Link.Retval)
	case model.FileOpenEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Open.File, event),
		}

		if event.Open.Flags&syscall.O_CREAT > 0 {
			s.FileEventSerializer.Destination = &FileSerializer{
				Mode: &event.Open.Mode,
			}
		}

		s.FileSerializer.Flags = model.OpenFlags(event.Open.Flags).StringArray()
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Open.Retval)
	case model.FileMkdirEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Mkdir.File, event),
			Destination: &FileSerializer{
				Mode: &event.Mkdir.Mode,
			},
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Mkdir.Retval)
	case model.FileRmdirEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Rmdir.File, event),
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Rmdir.Retval)
	case model.FileUnlinkEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Unlink.File, event),
		}
		s.FileSerializer.Flags = model.UnlinkFlags(event.Unlink.Flags).StringArray()
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Unlink.Retval)
	case model.FileRenameEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Rename.Old, event),
			Destination:    newFileSerializer(&event.Rename.New, event),
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Rename.Retval)
	case model.FileRemoveXAttrEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.RemoveXAttr.File, event),
			Destination: &FileSerializer{
				XAttrName:      event.GetXAttrName(&event.RemoveXAttr),
				XAttrNamespace: event.GetXAttrNamespace(&event.RemoveXAttr),
			},
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.RemoveXAttr.Retval)
	case model.FileSetXAttrEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.SetXAttr.File, event),
			Destination: &FileSerializer{
				XAttrName:      event.GetXAttrName(&event.SetXAttr),
				XAttrNamespace: event.GetXAttrNamespace(&event.SetXAttr),
			},
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.SetXAttr.Retval)
	case model.FileUtimeEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Utimes.File, event),
			Destination: &FileSerializer{
				Atime: getTimeIfNotZero(event.Utimes.Atime),
				Mtime: getTimeIfNotZero(event.Utimes.Mtime),
			},
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Utimes.Retval)
	case model.FileMountEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: FileSerializer{
				Path:                event.ResolveMountRoot(&event.Mount),
				PathResolutionError: event.Mount.GetRootPathResolutionError(),
				MountID:             &event.Mount.RootMountID,
				Inode:               &event.Mount.RootInode,
			},
			Destination: &FileSerializer{
				Path:                event.ResolveMountPoint(&event.Mount),
				PathResolutionError: event.Mount.GetMountPointPathResolutionError(),
				MountID:             &event.Mount.ParentMountID,
				Inode:               &event.Mount.ParentInode,
			},
			NewMountID: event.Mount.MountID,
			GroupID:    event.Mount.GroupID,
			Device:     event.Mount.Device,
			FSType:     event.Mount.GetFSType(),
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Mount.Retval)
	case model.FileUmountEventType:
		s.FileEventSerializer = &FileEventSerializer{
			NewMountID: event.Umount.MountID,
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Umount.Retval)
	case model.SetuidEventType:
		s.ProcessContextSerializer.Credentials.Destination = &SetuidSerializer{
			UID:    int(event.SetUID.UID),
			User:   event.ResolveSetuidUser(&event.SetUID),
			EUID:   int(event.SetUID.EUID),
			EUser:  event.ResolveSetuidEUser(&event.SetUID),
			FSUID:  int(event.SetUID.FSUID),
			FSUser: event.ResolveSetuidFSUser(&event.SetUID),
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(0)
		s.Category = ProcessActivity
	case model.SetgidEventType:
		s.ProcessContextSerializer.Credentials.Destination = &SetgidSerializer{
			GID:     int(event.SetGID.GID),
			Group:   event.ResolveSetgidGroup(&event.SetGID),
			EGID:    int(event.SetGID.EGID),
			EGroup:  event.ResolveSetgidEGroup(&event.SetGID),
			FSGID:   int(event.SetGID.FSGID),
			FSGroup: event.ResolveSetgidFSGroup(&event.SetGID),
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(0)
		s.Category = ProcessActivity
	case model.CapsetEventType:
		s.ProcessContextSerializer.Credentials.Destination = &CapsetSerializer{
			CapEffective: model.KernelCapability(event.Capset.CapEffective).StringArray(),
			CapPermitted: model.KernelCapability(event.Capset.CapPermitted).StringArray(),
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(0)
		s.Category = ProcessActivity
	case model.ForkEventType:
		s.EventContextSerializer.Outcome = serializeSyscallRetval(0)
		s.Category = ProcessActivity
	case model.ExitEventType:
		s.EventContextSerializer.Outcome = serializeSyscallRetval(0)
		s.Category = ProcessActivity
	case model.ExecEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newProcessFileSerializer(&event.processCacheEntry.Process, event),
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(0)
		s.Category = ProcessActivity
	}

	return s
}
