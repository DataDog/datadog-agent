//go:generate go run github.com/mailru/easyjson/easyjson -build_tags linux $GOFILE

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probe

import (
	"encoding/json"
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
	UID                 uint32     `json:"uid"`
	GID                 uint32     `json:"gid"`
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
	UID          int          `json:"uid"`
	User         string       `json:"user,omitempty"`
	GID          int          `json:"gid"`
	Group        string       `json:"group,omitempty"`
	EUID         int          `json:"euid"`
	EUser        string       `json:"euser,omitempty"`
	EGID         int          `json:"egid"`
	EGroup       string       `json:"egroup,omitempty"`
	FSUID        int          `json:"fsuid"`
	FSUser       string       `json:"fsuser,omitempty"`
	FSGID        int          `json:"fsgid"`
	FSGroup      string       `json:"fsgroup,omitempty"`
	CapEffective JStringArray `json:"cap_effective"`
	CapPermitted JStringArray `json:"cap_permitted"`
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

// JStringArray handles empty array properly not generating null output but []
type JStringArray []string

// MarshalJSON custom marshaller to handle empty array
func (j *JStringArray) MarshalJSON() ([]byte, error) {
	if len(*j) == 0 {
		return []byte("[]"), nil
	}
	return json.Marshal([]string(*j))
}

// CapsetSerializer serializes a capset event
// easyjson:json
type CapsetSerializer struct {
	CapEffective JStringArray `json:"cap_effective"`
	CapPermitted JStringArray `json:"cap_permitted"`
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
		Path:                e.ResolveFilePath(fe),
		PathResolutionError: fe.GetPathResolutionError(),
		Name:                e.ResolveFileBasename(fe),
		ContainerPath:       e.ResolveFileContainerPath(fe),
		Inode:               getUint64Pointer(&fe.Inode),
		MountID:             getUint32Pointer(&fe.MountID),
		Filesystem:          e.ResolveFileFilesystem(fe),
		Mode:                getUint32Pointer(&mode),
		UID:                 fe.UID,
		GID:                 fe.GID,
		User:                e.ResolveFileFieldsUser(&fe.FileFields),
		Group:               e.ResolveFileFieldsGroup(&fe.FileFields),
		Mtime:               &fe.MTime,
		Ctime:               &fe.CTime,
		InUpperLayer:        getInUpperLayer(e.resolvers, &fe.FileFields),
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
		Filesystem:          process.Filesystem,
		InUpperLayer:        getInUpperLayer(r, &process.FileFields),
		Mode:                getUint32Pointer(&mode),
		UID:                 process.FileFields.UID,
		GID:                 process.FileFields.GID,
		User:                r.ResolveFileFieldsUser(&process.FileFields),
		Group:               r.ResolveFileFieldsGroup(&process.FileFields),
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

func newCredentialsSerializer(ce *model.Credentials) *CredentialsSerializer {
	return &CredentialsSerializer{
		UID:          int(ce.UID),
		User:         ce.User,
		EUID:         int(ce.EUID),
		EUser:        ce.EUser,
		FSUID:        int(ce.FSUID),
		FSUser:       ce.FSUser,
		GID:          int(ce.GID),
		Group:        ce.Group,
		EGID:         int(ce.EGID),
		EGroup:       ce.EGroup,
		FSGID:        int(ce.FSGID),
		FSGroup:      ce.FSGroup,
		CapEffective: JStringArray(model.KernelCapability(ce.CapEffective).StringArray()),
		CapPermitted: JStringArray(model.KernelCapability(ce.CapPermitted).StringArray()),
	}
}

func scrubArgs(pr *model.Process, e *Event) ([]string, bool) {
	argv, truncated := e.resolvers.ProcessResolver.GetProcessArgv(pr)

	// scrub args, do not send args if no scrubber instance is passed
	// can be the case for some custom event
	if e.scrubber == nil {
		argv = []string{}
	} else {
		if newArgv, changed := e.scrubber.ScrubCommand(argv); changed {
			argv = newArgv
		}
	}

	return argv, truncated
}

func scrubEnvs(pr *model.Process, e *Event) ([]string, bool) {
	envs, truncated := e.resolvers.ProcessResolver.GetProcessEnvs(pr)
	if envs == nil {
		return nil, false
	}

	result := make([]string, 0, len(envs))
	for key := range envs {
		result = append(result, key)
	}

	return result, truncated
}

func newProcessCacheEntrySerializer(pce *model.ProcessCacheEntry, e *Event) *ProcessCacheEntrySerializer {
	argv, argvTruncated := scrubArgs(&pce.Process, e)
	envs, EnvsTruncated := scrubEnvs(&pce.Process, e)

	pceSerializer := &ProcessCacheEntrySerializer{
		Inode:               pce.FileFields.Inode,
		MountID:             pce.FileFields.MountID,
		PathResolutionError: pce.GetPathResolutionError(),
		ForkTime:            getTimeIfNotZero(pce.ForkTime),
		ExecTime:            getTimeIfNotZero(pce.ExecTime),
		ExitTime:            getTimeIfNotZero(pce.ExitTime),

		Pid:           pce.Process.Pid,
		Tid:           pce.Process.Tid,
		PPid:          pce.Process.PPid,
		Path:          pce.Process.PathnameStr,
		ContainerPath: pce.Process.ContainerPath,
		Comm:          pce.Process.Comm,
		TTY:           pce.Process.TTYName,
		Executable:    newProcessFileSerializerWithResolvers(&pce.Process, e.resolvers),
		Args:          argv,
		ArgsTruncated: argvTruncated,
		Envs:          envs,
		EnvsTruncated: EnvsTruncated,
	}

	credsSerializer := newCredentialsSerializer(&pce.Credentials)
	// Populate legacy user / group fields
	pceSerializer.UID = credsSerializer.UID
	pceSerializer.User = credsSerializer.User
	pceSerializer.GID = credsSerializer.GID
	pceSerializer.Group = credsSerializer.Group
	pceSerializer.Credentials = &ProcessCredentialsSerializer{
		CredentialsSerializer: credsSerializer,
	}

	if len(pce.ContainerID) != 0 {
		pceSerializer.Container = &ContainerContextSerializer{
			ID: pce.ContainerID,
		}
	}
	return pceSerializer
}

func newProcessContextSerializer(entry *model.ProcessCacheEntry, e *Event, r *Resolvers) *ProcessContextSerializer {
	var ps *ProcessContextSerializer

	if e == nil {
		// custom events call newProcessContextSerializer with an empty Event
		e = NewEvent(r, nil)
		e.ProcessContext = model.ProcessContext{
			Ancestor: entry,
		}
	}

	ps = &ProcessContextSerializer{
		ProcessCacheEntrySerializer: newProcessCacheEntrySerializer(entry, e),
	}

	ctx := eval.NewContext(e.GetPointer())

	it := &model.ProcessAncestorsIterator{}
	ptr := it.Front(ctx)

	var prev *ProcessCacheEntrySerializer
	first := true

	for ptr != nil {
		ancestor := (*model.ProcessCacheEntry)(ptr)

		s := newProcessCacheEntrySerializer(ancestor, e)
		ps.Ancestors = append(ps.Ancestors, s)

		if first {
			ps.Parent = s
		}
		first = false

		// dedup args/envs
		if prev != nil {
			// parent/child with the same comm then a fork thus we
			// can remove the child args/envs
			if prev.PPid == s.Pid && prev.Comm == s.Comm {
				prev.Args, prev.ArgsTruncated = prev.Args[0:0], false
				prev.Envs, prev.EnvsTruncated = prev.Envs[0:0], false
			}
		}
		prev = s

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

	if id := event.ResolveContainerID(&event.ContainerContext); id != "" {
		s.ContainerContextSerializer = &ContainerContextSerializer{
			ID: id,
		}
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
				XAttrName:      event.ResolveXAttrName(&event.RemoveXAttr),
				XAttrNamespace: event.ResolveXAttrNamespace(&event.RemoveXAttr),
			},
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.RemoveXAttr.Retval)
	case model.FileSetXAttrEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.SetXAttr.File, event),
			Destination: &FileSerializer{
				XAttrName:      event.ResolveXAttrName(&event.SetXAttr),
				XAttrNamespace: event.ResolveXAttrNamespace(&event.SetXAttr),
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
			CapEffective: JStringArray(model.KernelCapability(event.Capset.CapEffective).StringArray()),
			CapPermitted: JStringArray(model.KernelCapability(event.Capset.CapPermitted).StringArray()),
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
			FileSerializer: *newProcessFileSerializerWithResolvers(&event.processCacheEntry.Process, event.resolvers),
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(0)
		s.Category = ProcessActivity
	}

	return s
}
