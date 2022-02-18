//go:generate go run github.com/mailru/easyjson/easyjson -gen_build_flags=-mod=mod -no_std_marshalers -build_tags linux $GOFILE
//go:generate go run github.com/DataDog/datadog-agent/pkg/security/probe/doc_generator -output ../../../docs/cloud-workload-security/backend.schema.json

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"fmt"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// FileSerializer serializes a file to JSON
// easyjson:json
type FileSerializer struct {
	Path                string     `json:"path,omitempty" jsonschema_description:"File path"`
	Name                string     `json:"name,omitempty" jsonschema_description:"File basename"`
	PathResolutionError string     `json:"path_resolution_error,omitempty" jsonschema_description:"Error message from path resolution"`
	Inode               *uint64    `json:"inode,omitempty" jsonschema_description:"File inode number"`
	Mode                *uint32    `json:"mode,omitempty" jsonschema_description:"File mode"`
	InUpperLayer        *bool      `json:"in_upper_layer,omitempty" jsonschema_description:"Indicator of file OverlayFS layer"`
	MountID             *uint32    `json:"mount_id,omitempty" jsonschema_description:"File mount ID"`
	Filesystem          string     `json:"filesystem,omitempty" jsonschema_description:"File filesystem name"`
	UID                 int64      `json:"uid" jsonschema_description:"File User ID"`
	GID                 int64      `json:"gid" jsonschema_description:"File Group ID"`
	User                string     `json:"user,omitempty" jsonschema_description:"File user"`
	Group               string     `json:"group,omitempty" jsonschema_description:"File group"`
	XAttrName           string     `json:"attribute_name,omitempty" jsonschema_description:"File extended attribute name"`
	XAttrNamespace      string     `json:"attribute_namespace,omitempty" jsonschema_description:"File extended attribute namespace"`
	Flags               []string   `json:"flags,omitempty" jsonschema_description:"File flags"`
	Atime               *time.Time `json:"access_time,omitempty" jsonschema_descrition:"File access time"`
	Mtime               *time.Time `json:"modification_time,omitempty" jsonschema_description:"File modified time"`
	Ctime               *time.Time `json:"change_time,omitempty" jsonschema_description:"File change time"`
}

// UserContextSerializer serializes a user context to JSON
// easyjson:json
type UserContextSerializer struct {
	User  string `json:"id,omitempty" jsonschema_description:"User name"`
	Group string `json:"group,omitempty" jsonschema_description:"Group name"`
}

// CredentialsSerializer serializes a set credentials to JSON
// easyjson:json
type CredentialsSerializer struct {
	UID          int      `json:"uid" jsonschema_description:"User ID"`
	User         string   `json:"user,omitempty" jsonschema_description:"User name"`
	GID          int      `json:"gid" jsonschema_description:"Group ID"`
	Group        string   `json:"group,omitempty" jsonschema_description:"Group name"`
	EUID         int      `json:"euid" jsonschema_description:"Effective User ID"`
	EUser        string   `json:"euser,omitempty" jsonschema_description:"Effective User name"`
	EGID         int      `json:"egid" jsonschema_description:"Effective Group ID"`
	EGroup       string   `json:"egroup,omitempty" jsonschema_description:"Effective Group name"`
	FSUID        int      `json:"fsuid" jsonschema_description:"Filesystem User ID"`
	FSUser       string   `json:"fsuser,omitempty" jsonschema_description:"Filesystem User name"`
	FSGID        int      `json:"fsgid" jsonschema_description:"Filesystem Group ID"`
	FSGroup      string   `json:"fsgroup,omitempty" jsonschema_description:"Filesystem Group name"`
	CapEffective []string `json:"cap_effective" jsonschema_description:"Effective Capacity set"`
	CapPermitted []string `json:"cap_permitted" jsonschema_description:"Permitted Capacity set"`
}

// SetuidSerializer serializes a setuid event
// easyjson:json
type SetuidSerializer struct {
	UID    int    `json:"uid" jsonschema_description:"User ID"`
	User   string `json:"user,omitempty" jsonschema_description:"User name"`
	EUID   int    `json:"euid" jsonschema_description:"Effective User ID"`
	EUser  string `json:"euser,omitempty" jsonschema_description:"Effective User name"`
	FSUID  int    `json:"fsuid" jsonschema_description:"Filesystem User ID"`
	FSUser string `json:"fsuser,omitempty" jsonschema_description:"Filesystem User name"`
}

// SetgidSerializer serializes a setgid event
// easyjson:json
type SetgidSerializer struct {
	GID     int    `json:"gid" jsonschema_description:"Group ID"`
	Group   string `json:"group,omitempty" jsonschema_description:"Group name"`
	EGID    int    `json:"egid" jsonschema_description:"Effective Group ID"`
	EGroup  string `json:"egroup,omitempty" jsonschema_description:"Effective Group name"`
	FSGID   int    `json:"fsgid" jsonschema_description:"Filesystem Group ID"`
	FSGroup string `json:"fsgroup,omitempty" jsonschema_description:"Filesystem Group name"`
}

// CapsetSerializer serializes a capset event
// easyjson:json
type CapsetSerializer struct {
	CapEffective []string `json:"cap_effective" jsonschema_description:"Effective Capacity set"`
	CapPermitted []string `json:"cap_permitted" jsonschema_description:"Permitted Capacity set"`
}

// ProcessCredentialsSerializer serializes the process credentials to JSON
// easyjson:json
type ProcessCredentialsSerializer struct {
	*CredentialsSerializer
	Destination interface{} `json:"destination,omitempty" jsonschema_description:"Credentials after the operation"`
}

// ProcessCacheEntrySerializer serializes a process cache entry to JSON
// easyjson:json
type ProcessCacheEntrySerializer struct {
	Pid                 uint32                        `json:"pid,omitempty" jsonschema_description:"Process ID"`
	PPid                uint32                        `json:"ppid,omitempty" jsonschema_description:"Parent Process ID"`
	Tid                 uint32                        `json:"tid,omitempty" jsonschema_description:"Thread ID"`
	UID                 int                           `json:"uid" jsonschema_description:"User ID"`
	GID                 int                           `json:"gid" jsonschema_description:"Group ID"`
	User                string                        `json:"user,omitempty" jsonschema_description:"User name"`
	Group               string                        `json:"group,omitempty" jsonschema_description:"Group name"`
	PathResolutionError string                        `json:"path_resolution_error,omitempty" jsonschema_description:"Description of an error in the path resolution"`
	Comm                string                        `json:"comm,omitempty" jsonschema_description:"Command name"`
	TTY                 string                        `json:"tty,omitempty" jsonschema_description:"TTY associated with the process"`
	ForkTime            *time.Time                    `json:"fork_time,omitempty" jsonschema_description:"Fork time of the process"`
	ExecTime            *time.Time                    `json:"exec_time,omitempty" jsonschema_description:"Exec time of the process"`
	ExitTime            *time.Time                    `json:"exit_time,omitempty" jsonschema_description:"Exit time of the process"`
	Credentials         *ProcessCredentialsSerializer `json:"credentials,omitempty" jsonschema_description:"Credentials associated with the process"`
	Executable          *FileSerializer               `json:"executable,omitempty" jsonschema_description:"File information of the executable"`
	Container           *ContainerContextSerializer   `json:"container,omitempty" jsonschema_description:"Container context"`
	Argv0               string                        `json:"argv0,omitempty" jsonschema_description:"First command line argument"`
	Args                []string                      `json:"args,omitempty" jsonschema_description:"Command line arguments"`
	ArgsTruncated       bool                          `json:"args_truncated,omitempty" jsonschema_description:"Indicator of arguments truncation"`
	Envs                []string                      `json:"envs,omitempty" jsonschema_description:"Environment variables of the process"`
	EnvsTruncated       bool                          `json:"envs_truncated,omitempty" jsonschema_description:"Indicator of environments variable truncation"`
}

// ContainerContextSerializer serializes a container context to JSON
// easyjson:json
type ContainerContextSerializer struct {
	ID string `json:"id,omitempty" jsonschema_description:"Container ID"`
}

// FileEventSerializer serializes a file event to JSON
// easyjson:json
type FileEventSerializer struct {
	FileSerializer
	Destination *FileSerializer `json:"destination,omitempty" jsonschema_description:"Target file information"`

	// Specific to mount events
	NewMountID uint32 `json:"new_mount_id,omitempty" jsonschema_description:"New Mount ID"`
	GroupID    uint32 `json:"group_id,omitempty" jsonschema_description:"Group ID"`
	Device     uint32 `json:"device,omitempty" jsonschema_description:"Device associated with the file"`
	FSType     string `json:"fstype,omitempty" jsonschema_description:"Filesystem type"`
}

// EventContextSerializer serializes an event context to JSON
// easyjson:json
type EventContextSerializer struct {
	Name     string `json:"name,omitempty" jsonschema_description:"Event name"`
	Category string `json:"category,omitempty" jsonschema_description:"Event category"`
	Outcome  string `json:"outcome,omitempty" jsonschema_description:"Event outcome"`
}

// ProcessContextSerializer serializes a process context to JSON
// easyjson:json
type ProcessContextSerializer struct {
	*ProcessCacheEntrySerializer
	Parent    *ProcessCacheEntrySerializer   `json:"parent,omitempty" jsonschema_description:"Parent process"`
	Ancestors []*ProcessCacheEntrySerializer `json:"ancestors,omitempty" jsonschema_description:"Ancestor processes"`
}

// easyjson:json
type selinuxBoolChangeSerializer struct {
	Name  string `json:"name,omitempty" jsonschema_description:"SELinux boolean name"`
	State string `json:"state,omitempty" jsonschema_description:"SELinux boolean state ('on' or 'off')"`
}

// easyjson:json
type selinuxEnforceStatusSerializer struct {
	Status string `json:"status,omitempty" jsonschema_description:"SELinux enforcement status (one of 'enforcing', 'permissive' or 'disabled')"`
}

// easyjson:json
type selinuxBoolCommitSerializer struct {
	State bool `json:"state,omitempty" jsonschema_description:"SELinux boolean commit operation"`
}

// SELinuxEventSerializer serializes a SELinux context to JSON
// easyjson:json
type SELinuxEventSerializer struct {
	BoolChange    *selinuxBoolChangeSerializer    `json:"bool,omitempty" jsonschema_description:"SELinux boolean operation"`
	EnforceStatus *selinuxEnforceStatusSerializer `json:"enforce,omitempty" jsonschema_description:"SELinux enforcement change"`
	BoolCommit    *selinuxBoolCommitSerializer    `json:"bool_commit,omitempty" jsonschema_description:"SELinux boolean commit"`
}

// BPFMapSerializer serializes a BPF map to JSON
// easyjson:json
type BPFMapSerializer struct {
	Name    string `json:"name,omitempty" jsonschema_description:"Name of the BPF map"`
	MapType string `json:"map_type,omitempty" jsonschema_description:"Type of the BPF map"`
}

// BPFProgramSerializer serializes a BPF map to JSON
// easyjson:json
type BPFProgramSerializer struct {
	Name        string   `json:"name,omitempty" jsonschema_description:"Name of the BPF program"`
	Tag         string   `json:"tag,omitempty" jsonschema_description:"Hash (sha1) of the BPF program"`
	ProgramType string   `json:"program_type,omitempty" jsonschema_description:"Type of the BPF program"`
	AttachType  string   `json:"attach_type,omitempty" jsonschema_description:"Attach type of the BPF program"`
	Helpers     []string `json:"helpers,omitempty" jsonschema_description:"List of helpers used by the BPF program"`
}

// BPFEventSerializer serializes a BPF event to JSON
// easyjson:json
type BPFEventSerializer struct {
	Cmd     string                `json:"cmd" jsonschema_description:"BPF command"`
	Map     *BPFMapSerializer     `json:"map,omitempty" jsonschema_description:"BPF map"`
	Program *BPFProgramSerializer `json:"program,omitempty" jsonschema_description:"BPF program"`
}

// MMapEventSerializer serializes a mmap event to JSON
type MMapEventSerializer struct {
	Address    string `json:"address" jsonschema_description:"memory segment address"`
	Offset     uint64 `json:"offset" jsonschema_description:"file offset"`
	Len        uint32 `json:"length" jsonschema_description:"memory segment length"`
	Protection string `json:"protection" jsonschema_description:"memory segment protection"`
	Flags      string `json:"flags" jsonschema_description:"memory segment flags"`
}

// MProtectEventSerializer serializes a mmap event to JSON
type MProtectEventSerializer struct {
	VMStart       string `json:"vm_start" jsonschema_description:"memory segment start address"`
	VMEnd         string `json:"vm_end" jsonschema_description:"memory segment end address"`
	VMProtection  string `json:"vm_protection" jsonschema_description:"initial memory segment protection"`
	ReqProtection string `json:"req_protection" jsonschema_description:"new memory segment protection"`
}

// PTraceEventSerializer serializes a mmap event to JSON
type PTraceEventSerializer struct {
	Request string                    `json:"request" jsonschema_description:"ptrace request"`
	Address string                    `json:"address" jsonschema_description:"address at which the ptrace request was executed"`
	Tracee  *ProcessContextSerializer `json:"tracee,omitempty" jsonschema_description:"process context of the tracee"`
}

// DDContextSerializer serializes a span context to JSON
// easyjson:json
type DDContextSerializer struct {
	SpanID  uint64 `json:"span_id,omitempty" jsonschema_description:"Span ID used for APM correlation"`
	TraceID uint64 `json:"trace_id,omitempty" jsonschema_description:"Trace ID used for APM correlation"`
}

// EventSerializer serializes an event to JSON
// easyjson:json
type EventSerializer struct {
	EventContextSerializer     `json:"evt,omitempty"`
	*FileEventSerializer       `json:"file,omitempty"`
	*SELinuxEventSerializer    `json:"selinux,omitempty"`
	*BPFEventSerializer        `json:"bpf,omitempty"`
	*MMapEventSerializer       `json:"mmap,omitempty"`
	*MProtectEventSerializer   `json:"mprotect,omitempty"`
	*PTraceEventSerializer     `json:"ptrace,omitempty"`
	UserContextSerializer      UserContextSerializer       `json:"usr,omitempty"`
	ProcessContextSerializer   ProcessContextSerializer    `json:"process,omitempty"`
	DDContextSerializer        DDContextSerializer         `json:"dd,omitempty"`
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

func newFileSerializer(fe *model.FileEvent, e *Event, forceInode ...uint64) *FileSerializer {
	inode := fe.Inode
	if len(forceInode) > 0 {
		inode = forceInode[0]
	}

	mode := uint32(fe.FileFields.Mode)
	return &FileSerializer{
		Path:                e.ResolveFilePath(fe),
		PathResolutionError: fe.GetPathResolutionError(),
		Name:                e.ResolveFileBasename(fe),
		Inode:               getUint64Pointer(&inode),
		MountID:             getUint32Pointer(&fe.MountID),
		Filesystem:          e.ResolveFileFilesystem(fe),
		Mode:                getUint32Pointer(&mode), // only used by open events
		UID:                 int64(fe.UID),
		GID:                 int64(fe.GID),
		User:                e.ResolveFileFieldsUser(&fe.FileFields),
		Group:               e.ResolveFileFieldsGroup(&fe.FileFields),
		Mtime:               getTimeIfNotZero(time.Unix(0, int64(fe.MTime))),
		Ctime:               getTimeIfNotZero(time.Unix(0, int64(fe.CTime))),
		InUpperLayer:        getInUpperLayer(e.resolvers, &fe.FileFields),
	}
}

func newProcessFileSerializerWithResolvers(process *model.Process, r *Resolvers) *FileSerializer {
	mode := uint32(process.FileFields.Mode)
	return &FileSerializer{
		Path:                process.PathnameStr,
		PathResolutionError: process.GetPathResolutionError(),
		Name:                process.BasenameStr,
		Inode:               getUint64Pointer(&process.FileFields.Inode),
		MountID:             getUint32Pointer(&process.FileFields.MountID),
		Filesystem:          process.Filesystem,
		InUpperLayer:        getInUpperLayer(r, &process.FileFields),
		Mode:                getUint32Pointer(&mode),
		UID:                 int64(process.FileFields.UID),
		GID:                 int64(process.FileFields.GID),
		User:                r.ResolveFileFieldsUser(&process.FileFields),
		Group:               r.ResolveFileFieldsGroup(&process.FileFields),
		Mtime:               getTimeIfNotZero(time.Unix(0, int64(process.FileFields.MTime))),
		Ctime:               getTimeIfNotZero(time.Unix(0, int64(process.FileFields.CTime))),
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
		CapEffective: model.KernelCapability(ce.CapEffective).StringArray(),
		CapPermitted: model.KernelCapability(ce.CapPermitted).StringArray(),
	}
}

func newProcessCacheEntrySerializer(pce *model.ProcessCacheEntry, e *Event) *ProcessCacheEntrySerializer {
	argv, argvTruncated := e.resolvers.ProcessResolver.GetProcessScrubbedArgv(&pce.Process)
	envs, EnvsTruncated := e.resolvers.ProcessResolver.GetProcessEnvs(&pce.Process)
	argv0, _ := e.resolvers.ProcessResolver.GetProcessArgv0(&pce.Process)

	pceSerializer := &ProcessCacheEntrySerializer{
		ForkTime: getTimeIfNotZero(pce.ForkTime),
		ExecTime: getTimeIfNotZero(pce.ExecTime),
		ExitTime: getTimeIfNotZero(pce.ExitTime),

		Pid:           pce.Process.Pid,
		Tid:           pce.Process.Tid,
		PPid:          pce.Process.PPid,
		Comm:          pce.Process.Comm,
		TTY:           pce.Process.TTYName,
		Executable:    newProcessFileSerializerWithResolvers(&pce.Process, e.resolvers),
		Argv0:         argv0,
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

func newDDContextSerializer(e *Event) DDContextSerializer {
	return DDContextSerializer{
		SpanID:  e.SpanContext.SpanID,
		TraceID: e.SpanContext.TraceID,
	}
}

func newProcessContextSerializer(entry *model.ProcessCacheEntry, e *Event, r *Resolvers) ProcessContextSerializer {
	var ps ProcessContextSerializer

	if e == nil {
		// custom events create an empty event
		e = NewEvent(r, nil)
		e.ProcessContext = model.ProcessContext{
			Ancestor: entry,
		}
	}

	ps = ProcessContextSerializer{
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

func newSELinuxSerializer(e *Event) *SELinuxEventSerializer {
	switch e.SELinux.EventKind {
	case model.SELinuxBoolChangeEventKind:
		return &SELinuxEventSerializer{
			BoolChange: &selinuxBoolChangeSerializer{
				Name:  e.ResolveSELinuxBoolName(&e.SELinux),
				State: e.SELinux.BoolChangeValue,
			},
		}
	case model.SELinuxStatusChangeEventKind:
		return &SELinuxEventSerializer{
			EnforceStatus: &selinuxEnforceStatusSerializer{
				Status: e.SELinux.EnforceStatus,
			},
		}
	case model.SELinuxBoolCommitEventKind:
		return &SELinuxEventSerializer{
			BoolCommit: &selinuxBoolCommitSerializer{
				State: e.SELinux.BoolCommitValue,
			},
		}
	default:
		return nil
	}
}

func newBPFMapSerializer(e *Event) *BPFMapSerializer {
	if e.BPF.Map.ID == 0 {
		return nil
	}
	return &BPFMapSerializer{
		Name:    e.BPF.Map.Name,
		MapType: model.BPFMapType(e.BPF.Map.Type).String(),
	}
}

func newBPFProgramSerializer(e *Event) *BPFProgramSerializer {
	if e.BPF.Program.ID == 0 {
		return nil
	}

	return &BPFProgramSerializer{
		Name:        e.BPF.Program.Name,
		Tag:         e.BPF.Program.Tag,
		ProgramType: model.BPFProgramType(e.BPF.Program.Type).String(),
		AttachType:  model.BPFAttachType(e.BPF.Program.AttachType).String(),
		Helpers:     model.StringifyHelpersList(e.BPF.Program.Helpers),
	}
}

func newBPFEventSerializer(e *Event) *BPFEventSerializer {
	return &BPFEventSerializer{
		Cmd:     model.BPFCmd(e.BPF.Cmd).String(),
		Map:     newBPFMapSerializer(e),
		Program: newBPFProgramSerializer(e),
	}
}

func newMMapEventSerializer(e *Event) *MMapEventSerializer {
	return &MMapEventSerializer{
		Address:    fmt.Sprintf("0x%x", e.MMap.Addr),
		Offset:     e.MMap.Offset,
		Len:        e.MMap.Len,
		Protection: model.Protection(e.MMap.Protection).String(),
		Flags:      model.MMapFlag(e.MMap.Flags).String(),
	}
}

func newMProtectEventSerializer(e *Event) *MProtectEventSerializer {
	return &MProtectEventSerializer{
		VMStart:       fmt.Sprintf("0x%x", e.MProtect.VMStart),
		VMEnd:         fmt.Sprintf("0x%x", e.MProtect.VMEnd),
		VMProtection:  model.VMFlag(e.MProtect.VMProtection).String(),
		ReqProtection: model.VMFlag(e.MProtect.ReqProtection).String(),
	}
}

func newPTraceEventSerializer(e *Event) *PTraceEventSerializer {
	ptes := &PTraceEventSerializer{
		Request: model.PTraceRequest(e.PTrace.Request).String(),
		Address: fmt.Sprintf("0x%x", e.PTrace.Address),
	}

	if e.PTrace.TraceeProcessCacheEntry != nil {
		pcs := newProcessContextSerializer(e.PTrace.TraceeProcessCacheEntry, e, e.resolvers)
		ptes.Tracee = &pcs
	}
	return ptes
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

// NewEventSerializer creates a new event serializer based on the event type
func NewEventSerializer(event *Event) *EventSerializer {
	s := &EventSerializer{
		EventContextSerializer: EventContextSerializer{
			Name: model.EventType(event.Type).String(),
		},
		ProcessContextSerializer: newProcessContextSerializer(event.ResolveProcessCacheEntry(), event, event.resolvers),
		DDContextSerializer:      newDDContextSerializer(event),
		Date:                     event.ResolveEventTimestamp(),
	}

	if id := event.ResolveContainerID(&event.ContainerContext); id != "" {
		s.ContainerContextSerializer = &ContainerContextSerializer{
			ID: id,
		}
	}

	s.UserContextSerializer.User = s.ProcessContextSerializer.User
	s.UserContextSerializer.Group = s.ProcessContextSerializer.Group

	eventType := model.EventType(event.Type)

	s.Category = model.GetEventTypeCategory(eventType.String())

	switch eventType {
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
		// use the source inode as the target one is a fake inode
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Link.Source, event),
			Destination:    newFileSerializer(&event.Link.Target, event, event.Link.Source.Inode),
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
		// use the new inode as the old one is a fake inode
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Rename.Old, event, event.Rename.New.Inode),
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
	case model.FileUtimesEventType:
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
	case model.CapsetEventType:
		s.ProcessContextSerializer.Credentials.Destination = &CapsetSerializer{
			CapEffective: model.KernelCapability(event.Capset.CapEffective).StringArray(),
			CapPermitted: model.KernelCapability(event.Capset.CapPermitted).StringArray(),
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(0)
	case model.ForkEventType:
		s.EventContextSerializer.Outcome = serializeSyscallRetval(0)
	case model.ExitEventType:
		s.EventContextSerializer.Outcome = serializeSyscallRetval(0)
	case model.ExecEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newProcessFileSerializerWithResolvers(&event.processCacheEntry.Process, event.resolvers),
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(0)
	case model.SELinuxEventType:
		s.EventContextSerializer.Outcome = serializeSyscallRetval(0)
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.SELinux.File, event),
		}
		s.SELinuxEventSerializer = newSELinuxSerializer(event)
	case model.BPFEventType:
		s.EventContextSerializer.Outcome = serializeSyscallRetval(0)
		s.BPFEventSerializer = newBPFEventSerializer(event)
	case model.MMapEventType:
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.MMap.Retval)
		if event.MMap.Flags&unix.MAP_ANONYMOUS == 0 {
			s.FileEventSerializer = &FileEventSerializer{
				FileSerializer: *newFileSerializer(&event.MMap.File, event),
			}
		}
		s.MMapEventSerializer = newMMapEventSerializer(event)
	case model.MProtectEventType:
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.MProtect.Retval)
		s.MProtectEventSerializer = newMProtectEventSerializer(event)
	case model.PTraceEventType:
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.PTrace.Retval)
		s.PTraceEventSerializer = newPTraceEventSerializer(event)
	}

	return s
}
