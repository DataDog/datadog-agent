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
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// FileSerializer serializes a file to JSON
// easyjson:json
type FileSerializer struct {
	Path                string              `json:"path,omitempty" jsonschema_description:"File path"`
	Name                string              `json:"name,omitempty" jsonschema_description:"File basename"`
	PathResolutionError string              `json:"path_resolution_error,omitempty" jsonschema_description:"Error message from path resolution"`
	Inode               *uint64             `json:"inode,omitempty" jsonschema_description:"File inode number"`
	Mode                *uint32             `json:"mode,omitempty" jsonschema_description:"File mode"`
	InUpperLayer        *bool               `json:"in_upper_layer,omitempty" jsonschema_description:"Indicator of file OverlayFS layer"`
	MountID             *uint32             `json:"mount_id,omitempty" jsonschema_description:"File mount ID"`
	Filesystem          string              `json:"filesystem,omitempty" jsonschema_description:"File filesystem name"`
	UID                 int64               `json:"uid" jsonschema_description:"File User ID"`
	GID                 int64               `json:"gid" jsonschema_description:"File Group ID"`
	User                string              `json:"user,omitempty" jsonschema_description:"File user"`
	Group               string              `json:"group,omitempty" jsonschema_description:"File group"`
	XAttrName           string              `json:"attribute_name,omitempty" jsonschema_description:"File extended attribute name"`
	XAttrNamespace      string              `json:"attribute_namespace,omitempty" jsonschema_description:"File extended attribute namespace"`
	Flags               []string            `json:"flags,omitempty" jsonschema_description:"File flags"`
	Atime               *utils.EasyjsonTime `json:"access_time,omitempty" jsonschema_descrition:"File access time"`
	Mtime               *utils.EasyjsonTime `json:"modification_time,omitempty" jsonschema_description:"File modified time"`
	Ctime               *utils.EasyjsonTime `json:"change_time,omitempty" jsonschema_description:"File change time"`
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

// ProcessSerializer serializes a process to JSON
// easyjson:json
type ProcessSerializer struct {
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
	ForkTime            *utils.EasyjsonTime           `json:"fork_time,omitempty" jsonschema_description:"Fork time of the process"`
	ExecTime            *utils.EasyjsonTime           `json:"exec_time,omitempty" jsonschema_description:"Exec time of the process"`
	ExitTime            *utils.EasyjsonTime           `json:"exit_time,omitempty" jsonschema_description:"Exit time of the process"`
	Credentials         *ProcessCredentialsSerializer `json:"credentials,omitempty" jsonschema_description:"Credentials associated with the process"`
	Executable          *FileSerializer               `json:"executable,omitempty" jsonschema_description:"File information of the executable"`
	Container           *ContainerContextSerializer   `json:"container,omitempty" jsonschema_description:"Container context"`
	Argv0               string                        `json:"argv0,omitempty" jsonschema_description:"First command line argument"`
	Args                []string                      `json:"args,omitempty" jsonschema_description:"Command line arguments"`
	ArgsTruncated       bool                          `json:"args_truncated,omitempty" jsonschema_description:"Indicator of arguments truncation"`
	Envs                []string                      `json:"envs,omitempty" jsonschema_description:"Environment variables of the process"`
	EnvsTruncated       bool                          `json:"envs_truncated,omitempty" jsonschema_description:"Indicator of environments variable truncation"`
	IsThread            bool                          `json:"is_thread,omitempty" jsonschema_description:"Indicates whether the process is considered a thread (that is, a child process that hasn't executed another program)"`
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
	Async    bool   `json:"async,omitempty" jsonschema_description:"True if the event was asynchronous"`
}

// ProcessContextSerializer serializes a process context to JSON
// easyjson:json
type ProcessContextSerializer struct {
	*ProcessSerializer
	Parent    *ProcessSerializer   `json:"parent,omitempty" jsonschema_description:"Parent process"`
	Ancestors []*ProcessSerializer `json:"ancestors,omitempty" jsonschema_description:"Ancestor processes"`
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
// easyjson:json
type MMapEventSerializer struct {
	Address    string `json:"address" jsonschema_description:"memory segment address"`
	Offset     uint64 `json:"offset" jsonschema_description:"file offset"`
	Len        uint32 `json:"length" jsonschema_description:"memory segment length"`
	Protection string `json:"protection" jsonschema_description:"memory segment protection"`
	Flags      string `json:"flags" jsonschema_description:"memory segment flags"`
}

// MProtectEventSerializer serializes a mmap event to JSON
// easyjson:json
type MProtectEventSerializer struct {
	VMStart       string `json:"vm_start" jsonschema_description:"memory segment start address"`
	VMEnd         string `json:"vm_end" jsonschema_description:"memory segment end address"`
	VMProtection  string `json:"vm_protection" jsonschema_description:"initial memory segment protection"`
	ReqProtection string `json:"req_protection" jsonschema_description:"new memory segment protection"`
}

// PTraceEventSerializer serializes a mmap event to JSON
// easyjson:json
type PTraceEventSerializer struct {
	Request string                    `json:"request" jsonschema_description:"ptrace request"`
	Address string                    `json:"address" jsonschema_description:"address at which the ptrace request was executed"`
	Tracee  *ProcessContextSerializer `json:"tracee,omitempty" jsonschema_description:"process context of the tracee"`
}

// SignalEventSerializer serializes a signal event to JSON
// easyjson:json
type SignalEventSerializer struct {
	Type   string                    `json:"type" jsonschema_description:"signal type"`
	PID    uint32                    `json:"pid" jsonschema_description:"signal target pid"`
	Target *ProcessContextSerializer `json:"target,omitempty" jsonschema_description:"process context of the signal target"`
}

// NetworkDeviceSerializer serializes the network device context to JSON
// easyjson:json
type NetworkDeviceSerializer struct {
	NetNS   uint32 `json:"netns" jsonschema_description:"netns is the interface ifindex"`
	IfIndex uint32 `json:"ifindex" jsonschema_description:"ifindex is the network interface ifindex"`
	IfName  string `json:"ifname" jsonschema_description:"ifname is the network interface name"`
}

// IPPortSerializer is used to serialize an IP and Port context to JSON
// easyjson:json
type IPPortSerializer struct {
	IP   string `json:"ip" jsonschema_description:"IP address"`
	Port uint16 `json:"port" jsonschema_description:"Port number"`
}

// IPPortFamilySerializer is used to serialize an IP, Port and address family context to JSON
// easyjson:json
type IPPortFamilySerializer struct {
	Family string `json:"family" jsonschema_description:"Address family"`
	IP     string `json:"ip" jsonschema_description:"IP address"`
	Port   uint16 `json:"port" jsonschema_description:"Port number"`
}

// NetworkContextSerializer serializes the network context to JSON
// easyjson:json
type NetworkContextSerializer struct {
	Device *NetworkDeviceSerializer `json:"device,omitempty" jsonschema_description:"device is the network device on which the event was captured"`

	L3Protocol  string            `json:"l3_protocol" jsonschema_description:"l3_protocol is the layer 3 procotocol name"`
	L4Protocol  string            `json:"l4_protocol" jsonschema_description:"l4_protocol is the layer 4 procotocol name"`
	Source      *IPPortSerializer `json:"source" jsonschema_description:"source is the emitter of the network event"`
	Destination *IPPortSerializer `json:"destination" jsonschema_description:"destination is the receiver of the network event"`
	Size        uint32            `json:"size" jsonschema_description:"size is the size in bytes of the network event"`
}

// DNSQuestionSerializer serializes a DNS question to JSON
// easyjson:json
type DNSQuestionSerializer struct {
	Class string `json:"class" jsonschema_description:"class is the class looked up by the DNS question"`
	Type  string `json:"type" jsonschema_description:"type is a two octet code which specifies the DNS question type"`
	Name  string `json:"name" jsonschema_description:"name is the queried domain name"`
	Size  uint16 `json:"size" jsonschema_description:"size is the total DNS request size in bytes"`
	Count uint16 `json:"count" jsonschema_description:"count is the total count of questions in the DNS request"`
}

// DNSEventSerializer serializes a dns event to JSON
// easyjson:json
type DNSEventSerializer struct {
	ID       uint16                 `json:"id" jsonschema_description:"id is the unique identifier of the DNS request"`
	Question *DNSQuestionSerializer `json:"question,omitempty" jsonschema_description:"question is a DNS question for the DNS request"`
}

// DDContextSerializer serializes a span context to JSON
// easyjson:json
type DDContextSerializer struct {
	SpanID  uint64 `json:"span_id,omitempty" jsonschema_description:"Span ID used for APM correlation"`
	TraceID uint64 `json:"trace_id,omitempty" jsonschema_description:"Trace ID used for APM correlation"`
}

// ModuleEventSerializer serializes a module event to JSON
// easyjson:json
type ModuleEventSerializer struct {
	Name             string `json:"name" jsonschema_description:"module name"`
	LoadedFromMemory *bool  `json:"loaded_from_memory,omitempty" jsonschema_description:"indicates if a module was loaded from memory, as opposed to a file"`
}

// SpliceEventSerializer serializes a splice event to JSON
// easyjson:json
type SpliceEventSerializer struct {
	PipeEntryFlag string `json:"pipe_entry_flag" jsonschema_description:"Entry flag of the fd_out pipe passed to the splice syscall"`
	PipeExitFlag  string `json:"pipe_exit_flag" jsonschema_description:"Exit flag of the fd_out pipe passed to the splice syscall"`
}

// BindEventSerializer serializes a bind event to JSON
// easyjson:json
type BindEventSerializer struct {
	Addr *IPPortFamilySerializer `json:"addr" jsonschema_description:"Bound address (if any)"`
}

// ExitEventSerializer serializes an exit event to JSON
// easyjson:json
type ExitEventSerializer struct {
	Cause string `json:"cause" jsonschema_description:"Cause of the process termination (one of EXITED, SIGNALED, COREDUMPED)"`
	Code  uint32 `json:"code" jsonschema_description:"Exit code of the process or number of the signal that caused the process to terminate"`
}

// EventSerializer serializes an event to JSON
// easyjson:json
type EventSerializer struct {
	EventContextSerializer      `json:"evt,omitempty"`
	*FileEventSerializer        `json:"file,omitempty"`
	*SELinuxEventSerializer     `json:"selinux,omitempty"`
	*BPFEventSerializer         `json:"bpf,omitempty"`
	*MMapEventSerializer        `json:"mmap,omitempty"`
	*MProtectEventSerializer    `json:"mprotect,omitempty"`
	*PTraceEventSerializer      `json:"ptrace,omitempty"`
	*ModuleEventSerializer      `json:"module,omitempty"`
	*SignalEventSerializer      `json:"signal,omitempty"`
	*SpliceEventSerializer      `json:"splice,omitempty"`
	*DNSEventSerializer         `json:"dns,omitempty"`
	*NetworkContextSerializer   `json:"network,omitempty"`
	*BindEventSerializer        `json:"bind,omitempty"`
	*ExitEventSerializer        `json:"exit,omitempty"`
	*UserContextSerializer      `json:"usr,omitempty"`
	*ProcessContextSerializer   `json:"process,omitempty"`
	*DDContextSerializer        `json:"dd,omitempty"`
	*ContainerContextSerializer `json:"container,omitempty"`
	Date                        utils.EasyjsonTime `json:"date,omitempty"`
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

func getTimeIfNotZero(t time.Time) *utils.EasyjsonTime {
	if t.IsZero() {
		return nil
	}
	tt := utils.NewEasyjsonTime(t)
	return &tt
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

func newProcessSerializer(ps *model.Process, e *Event) *ProcessSerializer {
	argv, argvTruncated := e.resolvers.ProcessResolver.GetProcessScrubbedArgv(ps)
	envs, EnvsTruncated := e.resolvers.ProcessResolver.GetProcessEnvs(ps)
	argv0, _ := e.resolvers.ProcessResolver.GetProcessArgv0(ps)

	psSerializer := &ProcessSerializer{
		ForkTime: getTimeIfNotZero(ps.ForkTime),
		ExecTime: getTimeIfNotZero(ps.ExecTime),
		ExitTime: getTimeIfNotZero(ps.ExitTime),

		Pid:           ps.Pid,
		Tid:           ps.Tid,
		PPid:          ps.PPid,
		Comm:          ps.Comm,
		TTY:           ps.TTYName,
		Executable:    newFileSerializer(&ps.FileEvent, e),
		Argv0:         argv0,
		Args:          argv,
		ArgsTruncated: argvTruncated,
		Envs:          envs,
		EnvsTruncated: EnvsTruncated,
		IsThread:      ps.IsThread,
	}

	credsSerializer := newCredentialsSerializer(&ps.Credentials)
	// Populate legacy user / group fields
	psSerializer.UID = credsSerializer.UID
	psSerializer.User = credsSerializer.User
	psSerializer.GID = credsSerializer.GID
	psSerializer.Group = credsSerializer.Group
	psSerializer.Credentials = &ProcessCredentialsSerializer{
		CredentialsSerializer: credsSerializer,
	}

	if len(ps.ContainerID) != 0 {
		psSerializer.Container = &ContainerContextSerializer{
			ID: ps.ContainerID,
		}
	}
	return psSerializer
}

func newDDContextSerializer(e *Event) *DDContextSerializer {
	s := &DDContextSerializer{
		SpanID:  e.SpanContext.SpanID,
		TraceID: e.SpanContext.TraceID,
	}
	if s.SpanID != 0 || s.TraceID != 0 {
		return s
	}

	ctx := eval.NewContext(e.GetPointer())
	it := &model.ProcessAncestorsIterator{}
	ptr := it.Front(ctx)

	for ptr != nil {
		pce := (*model.ProcessCacheEntry)(ptr)

		if pce.SpanID != 0 || pce.TraceID != 0 {
			s.SpanID = pce.SpanID
			s.TraceID = pce.TraceID
			break
		}

		ptr = it.Next()
	}

	return s
}

func newUserContextSerializer(e *Event) *UserContextSerializer {
	return &UserContextSerializer{
		User:  e.ProcessContext.User,
		Group: e.ProcessContext.Group,
	}
}

func newProcessContextSerializer(pc *model.ProcessContext, e *Event, r *Resolvers) *ProcessContextSerializer {
	if pc == nil || pc.Pid == 0 {
		return nil
	}

	var ps ProcessContextSerializer

	if e == nil {
		// custom events create an empty event
		e = NewEvent(r, nil, nil)
		e.ProcessContext = pc
	}

	ps = ProcessContextSerializer{
		ProcessSerializer: newProcessSerializer(&pc.Process, e),
	}

	ctx := eval.NewContext(e.GetPointer())

	it := &model.ProcessAncestorsIterator{}
	ptr := it.Front(ctx)

	var ancestor *model.ProcessCacheEntry
	var prev *ProcessSerializer

	first := true

	for ptr != nil {
		pce := (*model.ProcessCacheEntry)(ptr)

		s := newProcessSerializer(&pce.Process, e)
		ps.Ancestors = append(ps.Ancestors, s)

		if first {
			ps.Parent = s
		}
		first = false

		// dedup args/envs
		if ancestor != nil && ancestor.ArgsEntry == pce.ArgsEntry {
			prev.Args, prev.ArgsTruncated = prev.Args[0:0], false
			prev.Envs, prev.EnvsTruncated = prev.Envs[0:0], false
			prev.Argv0 = ""
		}
		ancestor = pce
		prev = s

		ptr = it.Next()
	}
	return &ps
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
	return &PTraceEventSerializer{
		Request: model.PTraceRequest(e.PTrace.Request).String(),
		Address: fmt.Sprintf("0x%x", e.PTrace.Address),
		Tracee:  newProcessContextSerializer(e.PTrace.Tracee, e, e.resolvers),
	}
}

func newLoadModuleEventSerializer(e *Event) *ModuleEventSerializer {
	loadedFromMemory := e.LoadModule.LoadedFromMemory
	return &ModuleEventSerializer{
		Name:             e.LoadModule.Name,
		LoadedFromMemory: &loadedFromMemory,
	}
}

func newUnloadModuleEventSerializer(e *Event) *ModuleEventSerializer {
	return &ModuleEventSerializer{
		Name: e.UnloadModule.Name,
	}
}

func newSignalEventSerializer(e *Event) *SignalEventSerializer {
	ses := &SignalEventSerializer{
		Type:   model.Signal(e.Signal.Type).String(),
		PID:    e.Signal.PID,
		Target: newProcessContextSerializer(e.Signal.Target, e, e.resolvers),
	}
	return ses
}

func newSpliceEventSerializer(e *Event) *SpliceEventSerializer {
	return &SpliceEventSerializer{
		PipeEntryFlag: model.PipeBufFlag(e.Splice.PipeEntryFlag).String(),
		PipeExitFlag:  model.PipeBufFlag(e.Splice.PipeExitFlag).String(),
	}
}

func newDNSQuestionSerializer(d *model.DNSEvent) *DNSQuestionSerializer {
	return &DNSQuestionSerializer{
		Class: model.QClass(d.Class).String(),
		Type:  model.QType(d.Type).String(),
		Name:  d.Name,
		Size:  d.Size,
		Count: d.Count,
	}
}

func newDNSEventSerializer(d *model.DNSEvent) *DNSEventSerializer {
	return &DNSEventSerializer{
		ID:       d.ID,
		Question: newDNSQuestionSerializer(d),
	}
}

func newIPPortSerializer(c *model.IPPortContext) *IPPortSerializer {
	return &IPPortSerializer{
		IP:   c.IPNet.IP.String(),
		Port: c.Port,
	}
}

func newIPPortFamilySerializer(c *model.IPPortContext, family string) *IPPortFamilySerializer {
	return &IPPortFamilySerializer{
		IP:     c.IPNet.IP.String(),
		Port:   c.Port,
		Family: family,
	}
}

func newNetworkDeviceSerializer(e *Event) *NetworkDeviceSerializer {
	return &NetworkDeviceSerializer{
		NetNS:   e.NetworkContext.Device.NetNS,
		IfIndex: e.NetworkContext.Device.IfIndex,
		IfName:  e.ResolveNetworkDeviceIfName(&e.NetworkContext.Device),
	}
}

func newNetworkContextSerializer(e *Event) *NetworkContextSerializer {
	return &NetworkContextSerializer{
		Device:      newNetworkDeviceSerializer(e),
		L3Protocol:  model.L3Protocol(e.NetworkContext.L3Protocol).String(),
		L4Protocol:  model.L4Protocol(e.NetworkContext.L4Protocol).String(),
		Source:      newIPPortSerializer(&e.NetworkContext.Source),
		Destination: newIPPortSerializer(&e.NetworkContext.Destination),
		Size:        e.NetworkContext.Size,
	}
}

func newBindEventSerializer(e *Event) *BindEventSerializer {
	bes := &BindEventSerializer{
		Addr: newIPPortFamilySerializer(&e.Bind.Addr, model.AddressFamily(e.Bind.AddrFamily).String()),
	}
	return bes
}

func newExitEventSerializer(e *Event) *ExitEventSerializer {
	return &ExitEventSerializer{
		Cause: model.ExitCause(e.Exit.Cause).String(),
		Code:  e.Exit.Code,
	}
}

func serializeSyscallRetval(retval int64) string {
	switch {
	case retval < 0:
		if syscall.Errno(-retval) == syscall.EACCES || syscall.Errno(-retval) == syscall.EPERM {
			return "Refused"
		}
		return "Error"
	default:
		return "Success"
	}
}

// NewEventSerializer creates a new event serializer based on the event type
func NewEventSerializer(event *Event) *EventSerializer {
	var pc model.ProcessContext
	if entry := event.ResolveProcessCacheEntry(); entry != nil {
		pc = entry.ProcessContext
	}

	s := &EventSerializer{
		EventContextSerializer: EventContextSerializer{
			Name: model.EventType(event.Type).String(),
		},
		ProcessContextSerializer: newProcessContextSerializer(&pc, event, event.resolvers),
		DDContextSerializer:      newDDContextSerializer(event),
		UserContextSerializer:    newUserContextSerializer(event),
		Date:                     utils.NewEasyjsonTime(event.ResolveEventTimestamp()),
	}

	if id := event.ResolveContainerID(&event.ContainerContext); id != "" {
		s.ContainerContextSerializer = &ContainerContextSerializer{
			ID: id,
		}
	}

	eventType := model.EventType(event.Type)

	s.Category = model.GetEventTypeCategory(eventType.String())

	if s.Category == model.NetworkCategory {
		s.NetworkContextSerializer = newNetworkContextSerializer(event)
	}

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
		s.Async = event.Async
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
		s.Async = event.Async
	case model.FileMkdirEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Mkdir.File, event),
			Destination: &FileSerializer{
				Mode: &event.Mkdir.Mode,
			},
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Mkdir.Retval)
		s.Async = event.Async
	case model.FileRmdirEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Rmdir.File, event),
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Rmdir.Retval)
		s.Async = event.Async
	case model.FileUnlinkEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Unlink.File, event),
		}
		s.FileSerializer.Flags = model.UnlinkFlags(event.Unlink.Flags).StringArray()
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Unlink.Retval)
		s.Async = event.Async
	case model.FileRenameEventType:
		// use the new inode as the old one is a fake inode
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Rename.Old, event, event.Rename.New.Inode),
			Destination:    newFileSerializer(&event.Rename.New, event),
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Rename.Retval)
		s.Async = event.Async
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
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.ProcessContext.Process.FileEvent, event),
		}
		s.ExitEventSerializer = newExitEventSerializer(event)
		s.EventContextSerializer.Outcome = serializeSyscallRetval(0)
	case model.ExecEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.ProcessContext.Process.FileEvent, event),
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
	case model.LoadModuleEventType:
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.LoadModule.Retval)
		if !event.LoadModule.LoadedFromMemory {
			s.FileEventSerializer = &FileEventSerializer{
				FileSerializer: *newFileSerializer(&event.LoadModule.File, event),
			}
		}
		s.ModuleEventSerializer = newLoadModuleEventSerializer(event)
	case model.UnloadModuleEventType:
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.UnloadModule.Retval)
		s.ModuleEventSerializer = newUnloadModuleEventSerializer(event)
	case model.SignalEventType:
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Signal.Retval)
		s.SignalEventSerializer = newSignalEventSerializer(event)
	case model.SpliceEventType:
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Splice.Retval)
		s.SpliceEventSerializer = newSpliceEventSerializer(event)
		if event.Splice.File.Inode != 0 {
			s.FileEventSerializer = &FileEventSerializer{
				FileSerializer: *newFileSerializer(&event.Splice.File, event),
			}
		}
	case model.DNSEventType:
		s.EventContextSerializer.Outcome = serializeSyscallRetval(0)
		s.DNSEventSerializer = newDNSEventSerializer(&event.DNS)
	case model.BindEventType:
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Bind.Retval)
		s.BindEventSerializer = newBindEventSerializer(event)
	}

	return s
}
