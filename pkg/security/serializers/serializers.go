//go:generate go run github.com/mailru/easyjson/easyjson -gen_build_flags=-mod=mod -no_std_marshalers -build_tags linux $GOFILE
//go:generate go run github.com/DataDog/datadog-agent/pkg/security/probe/doc_generator -output ../../../docs/cloud-workload-security/backend.schema.json

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package serializers

import (
	"fmt"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	jwriter "github.com/mailru/easyjson/jwriter"

	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// FileSerializer serializes a file to JSON
// easyjson:json
type FileSerializer struct {
	// File path
	Path string `json:"path,omitempty"`
	// File basename
	Name string `json:"name,omitempty"`
	// Error message from path resolution
	PathResolutionError string `json:"path_resolution_error,omitempty"`
	// File inode number
	Inode *uint64 `json:"inode,omitempty"`
	// File mode
	Mode *uint32 `json:"mode,omitempty"`
	// Indicator of file OverlayFS layer
	InUpperLayer *bool `json:"in_upper_layer,omitempty"`
	// File mount ID
	MountID *uint32 `json:"mount_id,omitempty"`
	// File filesystem name
	Filesystem string `json:"filesystem,omitempty"`
	// File User ID
	UID int64 `json:"uid"`
	// File Group ID
	GID int64 `json:"gid"`
	// File user
	User string `json:"user,omitempty"`
	// File group
	Group string `json:"group,omitempty"`
	// File extended attribute name
	XAttrName string `json:"attribute_name,omitempty"`
	// File extended attribute namespace
	XAttrNamespace string `json:"attribute_namespace,omitempty"`
	// File flags
	Flags []string `json:"flags,omitempty"`
	// File access time
	Atime *utils.EasyjsonTime `json:"access_time,omitempty"`
	// File modified time
	Mtime *utils.EasyjsonTime `json:"modification_time,omitempty"`
	// File change time
	Ctime *utils.EasyjsonTime `json:"change_time,omitempty"`
	// System package name
	PackageName string `json:"package_name,omitempty"`
	// System package version
	PackageVersion string `json:"package_version,omitempty"`
}

// UserContextSerializer serializes a user context to JSON
// easyjson:json
type UserContextSerializer struct {
	// User name
	User string `json:"id,omitempty"`
	// Group name
	Group string `json:"group,omitempty"`
}

// CredentialsSerializer serializes a set credentials to JSON
// easyjson:json
type CredentialsSerializer struct {
	// User ID
	UID int `json:"uid"`
	// User name
	User string `json:"user,omitempty"`
	// Group ID
	GID int `json:"gid"`
	// Group name
	Group string `json:"group,omitempty"`
	// Effective User ID
	EUID int `json:"euid"`
	// Effective User name
	EUser string `json:"euser,omitempty"`
	// Effective Group ID
	EGID int `json:"egid"`
	// Effective Group name
	EGroup string `json:"egroup,omitempty"`
	// Filesystem User ID
	FSUID int `json:"fsuid"`
	// Filesystem User name
	FSUser string `json:"fsuser,omitempty"`
	// Filesystem Group ID
	FSGID int `json:"fsgid"`
	// Filesystem Group name
	FSGroup string `json:"fsgroup,omitempty"`
	// Effective Capability set
	CapEffective []string `json:"cap_effective"`
	// Permitted Capability set
	CapPermitted []string `json:"cap_permitted"`
}

// SetuidSerializer serializes a setuid event
// easyjson:json
type SetuidSerializer struct {
	// User ID
	UID int `json:"uid"`
	// User name
	User string `json:"user,omitempty"`
	// Effective User ID
	EUID int `json:"euid"`
	// Effective User name
	EUser string `json:"euser,omitempty"`
	// Filesystem User ID
	FSUID int `json:"fsuid"`
	// Filesystem User name
	FSUser string `json:"fsuser,omitempty"`
}

// SetgidSerializer serializes a setgid event
// easyjson:json
type SetgidSerializer struct {
	// Group ID
	GID int `json:"gid"`
	// Group name
	Group string `json:"group,omitempty"`
	// Effective Group ID
	EGID int `json:"egid"`
	// Effective Group name
	EGroup string `json:"egroup,omitempty"`
	// Filesystem Group ID
	FSGID int `json:"fsgid"`
	// Filesystem Group name
	FSGroup string `json:"fsgroup,omitempty"`
}

// CapsetSerializer serializes a capset event
// easyjson:json
type CapsetSerializer struct {
	// Effective Capability set
	CapEffective []string `json:"cap_effective"`
	// Permitted Capability set
	CapPermitted []string `json:"cap_permitted"`
}

// ProcessCredentialsSerializer serializes the process credentials to JSON
// easyjson:json
type ProcessCredentialsSerializer struct {
	*CredentialsSerializer
	// Credentials after the operation
	Destination interface{} `json:"destination,omitempty"`
}

// ProcessSerializer serializes a process to JSON
// easyjson:json
type ProcessSerializer struct {
	// Process ID
	Pid uint32 `json:"pid,omitempty"`
	// Parent Process ID
	PPid *uint32 `json:"ppid,omitempty"`
	// Thread ID
	Tid uint32 `json:"tid,omitempty"`
	// User ID
	UID int `json:"uid"`
	// Group ID
	GID int `json:"gid"`
	// User name
	User string `json:"user,omitempty"`
	// Group name
	Group string `json:"group,omitempty"`
	// Description of an error in the path resolution
	PathResolutionError string `json:"path_resolution_error,omitempty"`
	// Command name
	Comm string `json:"comm,omitempty"`
	// TTY associated with the process
	TTY string `json:"tty,omitempty"`
	// Fork time of the process
	ForkTime *utils.EasyjsonTime `json:"fork_time,omitempty"`
	// Exec time of the process
	ExecTime *utils.EasyjsonTime `json:"exec_time,omitempty"`
	// Exit time of the process
	ExitTime *utils.EasyjsonTime `json:"exit_time,omitempty"`
	// Credentials associated with the process
	Credentials *ProcessCredentialsSerializer `json:"credentials,omitempty"`
	// File information of the executable
	Executable *FileSerializer `json:"executable,omitempty"`
	// File information of the interpreter
	Interpreter *FileSerializer `json:"interpreter,omitempty"`
	// Container context
	Container *ContainerContextSerializer `json:"container,omitempty"`
	// First command line argument
	Argv0 string `json:"argv0,omitempty"`
	// Command line arguments
	Args []string `json:"args,omitempty"`
	// Indicator of arguments truncation
	ArgsTruncated bool `json:"args_truncated,omitempty"`
	// Environment variables of the process
	Envs []string `json:"envs,omitempty"`
	// Indicator of environments variable truncation
	EnvsTruncated bool `json:"envs_truncated,omitempty"`
	// Indicates whether the process is considered a thread (that is, a child process that hasn't executed another program)
	IsThread bool `json:"is_thread,omitempty"`
	// Indicates whether the process is a kworker
	IsKworker bool `json:"is_kworker,omitempty"`
}

// ContainerContextSerializer serializes a container context to JSON
// easyjson:json
type ContainerContextSerializer struct {
	// Container ID
	ID string `json:"id,omitempty"`
	// Creation time of the container
	CreatedAt *utils.EasyjsonTime `json:"created_at,omitempty"`
}

// FileEventSerializer serializes a file event to JSON
// easyjson:json
type FileEventSerializer struct {
	FileSerializer
	// Target file information
	Destination *FileSerializer `json:"destination,omitempty"`

	// Specific to mount events

	// New Mount ID
	NewMountID uint32 `json:"new_mount_id,omitempty"`
	// Group ID
	GroupID uint32 `json:"group_id,omitempty"`
	// Device associated with the file
	Device uint32 `json:"device,omitempty"`
	// Filesystem type
	FSType string `json:"fstype,omitempty"`
}

// EventContextSerializer serializes an event context to JSON
// easyjson:json
type EventContextSerializer struct {
	// Event name
	Name string `json:"name,omitempty"`
	// Event category
	Category string `json:"category,omitempty"`
	// Event outcome
	Outcome string `json:"outcome,omitempty"`
	// True if the event was asynchronous
	Async bool `json:"async,omitempty"`
}

// ProcessContextSerializer serializes a process context to JSON
// easyjson:json
type ProcessContextSerializer struct {
	*ProcessSerializer
	// Parent process
	Parent *ProcessSerializer `json:"parent,omitempty"`
	// Ancestor processes
	Ancestors []*ProcessSerializer `json:"ancestors,omitempty"`
}

// SELinuxBoolChangeSerializer serializes a SELinux boolean change to JSON
// easyjson:json
type SELinuxBoolChangeSerializer struct {
	// SELinux boolean name
	Name string `json:"name,omitempty"`
	// SELinux boolean state ('on' or 'off')
	State string `json:"state,omitempty"`
}

// SELinuxEnforceStatusSerializer serializes a SELinux enforcement status change to JSON
// easyjson:json
type SELinuxEnforceStatusSerializer struct {
	// SELinux enforcement status (one of 'enforcing', 'permissive' or 'disabled')
	Status string `json:"status,omitempty"`
}

// SELinuxBoolCommitSerializer serializes a SELinux boolean commit to JSON
// easyjson:json
type SELinuxBoolCommitSerializer struct {
	// SELinux boolean commit operation
	State bool `json:"state,omitempty"`
}

// SELinuxEventSerializer serializes a SELinux context to JSON
// easyjson:json
type SELinuxEventSerializer struct {
	// SELinux boolean operation
	BoolChange *SELinuxBoolChangeSerializer `json:"bool,omitempty"`
	// SELinux enforcement change
	EnforceStatus *SELinuxEnforceStatusSerializer `json:"enforce,omitempty"`
	// SELinux boolean commit
	BoolCommit *SELinuxBoolCommitSerializer `json:"bool_commit,omitempty"`
}

// BPFMapSerializer serializes a BPF map to JSON
// easyjson:json
type BPFMapSerializer struct {
	// Name of the BPF map
	Name string `json:"name,omitempty"`
	// Type of the BPF map
	MapType string `json:"map_type,omitempty"`
}

// BPFProgramSerializer serializes a BPF map to JSON
// easyjson:json
type BPFProgramSerializer struct {
	// Name of the BPF program
	Name string `json:"name,omitempty"`
	// Hash (sha1) of the BPF program
	Tag string `json:"tag,omitempty"`
	// Type of the BPF program
	ProgramType string `json:"program_type,omitempty"`
	// Attach type of the BPF program
	AttachType string `json:"attach_type,omitempty"`
	// List of helpers used by the BPF program
	Helpers []string `json:"helpers,omitempty"`
}

// BPFEventSerializer serializes a BPF event to JSON
// easyjson:json
type BPFEventSerializer struct {
	// BPF command
	Cmd string `json:"cmd"`
	// BPF map
	Map *BPFMapSerializer `json:"map,omitempty"`
	// BPF program
	Program *BPFProgramSerializer `json:"program,omitempty"`
}

// MMapEventSerializer serializes a mmap event to JSON
// easyjson:json
type MMapEventSerializer struct {
	// memory segment address
	Address string `json:"address"`
	// file offset
	Offset uint64 `json:"offset"`
	// memory segment length
	Len uint32 `json:"length"`
	// memory segment protection
	Protection string `json:"protection"`
	// memory segment flags
	Flags string `json:"flags"`
}

// MProtectEventSerializer serializes a mmap event to JSON
// easyjson:json
type MProtectEventSerializer struct {
	// memory segment start address
	VMStart string `json:"vm_start"`
	// memory segment end address
	VMEnd string `json:"vm_end"`
	// initial memory segment protection
	VMProtection string `json:"vm_protection"`
	// new memory segment protection
	ReqProtection string `json:"req_protection"`
}

// PTraceEventSerializer serializes a mmap event to JSON
// easyjson:json
type PTraceEventSerializer struct {
	// ptrace request
	Request string `json:"request"`
	// address at which the ptrace request was executed
	Address string `json:"address"`
	// process context of the tracee
	Tracee *ProcessContextSerializer `json:"tracee,omitempty"`
}

// SignalEventSerializer serializes a signal event to JSON
// easyjson:json
type SignalEventSerializer struct {
	// signal type
	Type string `json:"type"`
	// signal target pid
	PID uint32 `json:"pid"`
	// process context of the signal target
	Target *ProcessContextSerializer `json:"target,omitempty"`
}

// NetworkDeviceSerializer serializes the network device context to JSON
// easyjson:json
type NetworkDeviceSerializer struct {
	// netns is the interface ifindex
	NetNS uint32 `json:"netns"`
	// ifindex is the network interface ifindex
	IfIndex uint32 `json:"ifindex"`
	// ifname is the network interface name
	IfName string `json:"ifname"`
}

// IPPortSerializer is used to serialize an IP and Port context to JSON
// easyjson:json
type IPPortSerializer struct {
	// IP address
	IP string `json:"ip"`
	// Port number
	Port uint16 `json:"port"`
}

// IPPortFamilySerializer is used to serialize an IP, port, and address family context to JSON
// easyjson:json
type IPPortFamilySerializer struct {
	// Address family
	Family string `json:"family"`
	// IP address
	IP string `json:"ip"`
	// Port number
	Port uint16 `json:"port"`
}

// NetworkContextSerializer serializes the network context to JSON
// easyjson:json
type NetworkContextSerializer struct {
	// device is the network device on which the event was captured
	Device *NetworkDeviceSerializer `json:"device,omitempty"`

	// l3_protocol is the layer 3 protocol name
	L3Protocol string `json:"l3_protocol"`
	// l4_protocol is the layer 4 protocol name
	L4Protocol string `json:"l4_protocol"`
	// source is the emitter of the network event
	Source *IPPortSerializer `json:"source"`
	// destination is the receiver of the network event
	Destination *IPPortSerializer `json:"destination"`
	// size is the size in bytes of the network event
	Size uint32 `json:"size"`
}

// DNSQuestionSerializer serializes a DNS question to JSON
// easyjson:json
type DNSQuestionSerializer struct {
	// class is the class looked up by the DNS question
	Class string `json:"class"`
	// type is a two octet code which specifies the DNS question type
	Type string `json:"type"`
	// name is the queried domain name
	Name string `json:"name"`
	// size is the total DNS request size in bytes
	Size uint16 `json:"size"`
	// count is the total count of questions in the DNS request
	Count uint16 `json:"count"`
}

// DNSEventSerializer serializes a DNS event to JSON
// easyjson:json
type DNSEventSerializer struct {
	// id is the unique identifier of the DNS request
	ID uint16 `json:"id"`
	// question is a DNS question for the DNS request
	Question *DNSQuestionSerializer `json:"question,omitempty"`
}

// DDContextSerializer serializes a span context to JSON
// easyjson:json
type DDContextSerializer struct {
	// Span ID used for APM correlation
	SpanID uint64 `json:"span_id,omitempty"`
	// Trace ID used for APM correlation
	TraceID uint64 `json:"trace_id,omitempty"`
}

// ModuleEventSerializer serializes a module event to JSON
// easyjson:json
type ModuleEventSerializer struct {
	// module name
	Name string `json:"name"`
	// indicates if a module was loaded from memory, as opposed to a file
	LoadedFromMemory *bool    `json:"loaded_from_memory,omitempty"`
	Argv             []string `json:"argv,omitempty"`
	ArgsTruncated    *bool    `json:"args_truncated,omitempty"`
}

// SpliceEventSerializer serializes a splice event to JSON
// easyjson:json
type SpliceEventSerializer struct {
	// Entry flag of the fd_out pipe passed to the splice syscall
	PipeEntryFlag string `json:"pipe_entry_flag"`
	// Exit flag of the fd_out pipe passed to the splice syscall
	PipeExitFlag string `json:"pipe_exit_flag"`
}

// BindEventSerializer serializes a bind event to JSON
// easyjson:json
type BindEventSerializer struct {
	// Bound address (if any)
	Addr *IPPortFamilySerializer `json:"addr"`
}

// ExitEventSerializer serializes an exit event to JSON
// easyjson:json
type ExitEventSerializer struct {
	// Cause of the process termination (one of EXITED, SIGNALED, COREDUMPED)
	Cause string `json:"cause"`
	// Exit code of the process or number of the signal that caused the process to terminate
	Code uint32 `json:"code"`
}

// MountEventSerializer serializes a mount event to JSON
// easyjson:json
type MountEventSerializer struct {
	MountPoint                     *FileSerializer `json:"mp,omitempty"`                    // Mount point file information
	Root                           *FileSerializer `json:"root,omitempty"`                  // Root file information
	MountID                        uint32          `json:"mount_id"`                        // Mount ID of the new mount
	GroupID                        uint32          `json:"group_id"`                        // ID of the peer group
	ParentMountID                  uint32          `json:"parent_mount_id"`                 // Mount ID of the parent mount
	BindSrcMountID                 uint32          `json:"bind_src_mount_id"`               // Mount ID of the source of a bind mount
	Device                         uint32          `json:"device"`                          // Device associated with the file
	FSType                         string          `json:"fs_type,omitempty"`               // Filesystem type
	MountPointPath                 string          `json:"mountpoint.path,omitempty"`       // Mount point path
	MountSourcePath                string          `json:"source.path,omitempty"`           // Mount source path
	MountPointPathResolutionError  string          `json:"mountpoint.path_error,omitempty"` // Mount point path error
	MountSourcePathResolutionError string          `json:"source.path_error,omitempty"`     // Mount source path error
}

// AnomalyDetectionSyscallEventSerializer serializes an anomaly detection for a syscall event
type AnomalyDetectionSyscallEventSerializer struct {
	// Name of the syscall that triggered the anomaly detection event
	Syscall string `json:"syscall"`
}

// SecurityProfileContextSerializer serializes the security profile context in an event
type SecurityProfileContextSerializer struct {
	// Name of the security profile
	Name string `json:"name"`
	// Status defines in which state the security profile was when the event was triggered
	Status string `json:"status"`
	// Version of the profile in use
	Version string `json:"version"`
	// List of tags associated to this profile
	Tags []string `json:"tags"`
}

// EventSerializer serializes an event to JSON
// easyjson:json
type EventSerializer struct {
	EventContextSerializer                  `json:"evt,omitempty"`
	*FileEventSerializer                    `json:"file,omitempty"`
	*SELinuxEventSerializer                 `json:"selinux,omitempty"`
	*BPFEventSerializer                     `json:"bpf,omitempty"`
	*MMapEventSerializer                    `json:"mmap,omitempty"`
	*MProtectEventSerializer                `json:"mprotect,omitempty"`
	*PTraceEventSerializer                  `json:"ptrace,omitempty"`
	*ModuleEventSerializer                  `json:"module,omitempty"`
	*SignalEventSerializer                  `json:"signal,omitempty"`
	*SpliceEventSerializer                  `json:"splice,omitempty"`
	*DNSEventSerializer                     `json:"dns,omitempty"`
	*NetworkContextSerializer               `json:"network,omitempty"`
	*BindEventSerializer                    `json:"bind,omitempty"`
	*ExitEventSerializer                    `json:"exit,omitempty"`
	*MountEventSerializer                   `json:"mount,omitempty"`
	*AnomalyDetectionSyscallEventSerializer `json:"anomaly_detection_syscall,omitempty"`
	*UserContextSerializer                  `json:"usr,omitempty"`
	*ProcessContextSerializer               `json:"process,omitempty"`
	*DDContextSerializer                    `json:"dd,omitempty"`
	*ContainerContextSerializer             `json:"container,omitempty"`
	*SecurityProfileContextSerializer       `json:"security_profile,omitempty"`
	Date                                    utils.EasyjsonTime `json:"date,omitempty"`
}

func newSecurityProfileContextSerializer(e *model.SecurityProfileContext) *SecurityProfileContextSerializer {
	tags := make([]string, len(e.Tags))
	copy(tags, e.Tags)
	return &SecurityProfileContextSerializer{
		Name:    e.Name,
		Version: e.Version,
		Status:  e.Status.String(),
		Tags:    tags,
	}
}

func newAnomalyDetectionSyscallEventSerializer(e *model.AnomalyDetectionSyscallEvent) *AnomalyDetectionSyscallEventSerializer {
	return &AnomalyDetectionSyscallEventSerializer{
		Syscall: e.SyscallID.String(),
	}
}

func getInUpperLayer(f *model.FileFields) *bool {
	lowerLayer := f.GetInLowerLayer()
	upperLayer := f.GetInUpperLayer()
	if !lowerLayer && !upperLayer {
		return nil
	}
	return &upperLayer
}

func newFileSerializer(fe *model.FileEvent, e *model.Event, forceInode ...uint64) *FileSerializer {
	inode := fe.Inode
	if len(forceInode) > 0 {
		inode = forceInode[0]
	}

	mode := uint32(fe.FileFields.Mode)
	return &FileSerializer{
		Path:                e.FieldHandlers.ResolveFilePath(e, fe),
		PathResolutionError: fe.GetPathResolutionError(),
		Name:                e.FieldHandlers.ResolveFileBasename(e, fe),
		Inode:               getUint64Pointer(&inode),
		MountID:             getUint32Pointer(&fe.MountID),
		Filesystem:          e.FieldHandlers.ResolveFileFilesystem(e, fe),
		Mode:                getUint32Pointer(&mode), // only used by open events
		UID:                 int64(fe.UID),
		GID:                 int64(fe.GID),
		User:                e.FieldHandlers.ResolveFileFieldsUser(e, &fe.FileFields),
		Group:               e.FieldHandlers.ResolveFileFieldsGroup(e, &fe.FileFields),
		Mtime:               getTimeIfNotZero(time.Unix(0, int64(fe.MTime))),
		Ctime:               getTimeIfNotZero(time.Unix(0, int64(fe.CTime))),
		InUpperLayer:        getInUpperLayer(&fe.FileFields),
		PackageName:         e.FieldHandlers.ResolvePackageName(e, fe),
		PackageVersion:      e.FieldHandlers.ResolvePackageVersion(e, fe),
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

func newProcessSerializer(ps *model.Process, e *model.Event, resolvers *resolvers.Resolvers) *ProcessSerializer {
	if ps.IsNotKworker() {
		argv, argvTruncated := resolvers.ProcessResolver.GetProcessScrubbedArgv(ps)
		envs, EnvsTruncated := resolvers.ProcessResolver.GetProcessEnvs(ps)
		argv0, _ := resolvers.ProcessResolver.GetProcessArgv0(ps)

		psSerializer := &ProcessSerializer{
			ForkTime: getTimeIfNotZero(ps.ForkTime),
			ExecTime: getTimeIfNotZero(ps.ExecTime),
			ExitTime: getTimeIfNotZero(ps.ExitTime),

			Pid:           ps.Pid,
			Tid:           ps.Tid,
			PPid:          getUint32Pointer(&ps.PPid),
			Comm:          ps.Comm,
			TTY:           ps.TTYName,
			Executable:    newFileSerializer(&ps.FileEvent, e),
			Argv0:         argv0,
			Args:          argv,
			ArgsTruncated: argvTruncated,
			Envs:          envs,
			EnvsTruncated: EnvsTruncated,
			IsThread:      ps.IsThread,
			IsKworker:     ps.IsKworker,
		}

		if ps.HasInterpreter() {
			psSerializer.Interpreter = newFileSerializer(&ps.LinuxBinprm.FileEvent, e)
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
			if cgroup, _ := resolvers.CGroupResolver.GetWorkload(ps.ContainerID); cgroup != nil {
				psSerializer.Container.CreatedAt = getTimeIfNotZero(time.Unix(0, int64(cgroup.CreationTime)))
			}
		}
		return psSerializer
	} else {
		return &ProcessSerializer{
			Pid:       ps.Pid,
			Tid:       ps.Tid,
			IsKworker: ps.IsKworker,
			Credentials: &ProcessCredentialsSerializer{
				CredentialsSerializer: &CredentialsSerializer{},
			},
		}
	}
}

func newDDContextSerializer(e *model.Event) *DDContextSerializer {
	s := &DDContextSerializer{
		SpanID:  e.SpanContext.SpanID,
		TraceID: e.SpanContext.TraceID,
	}
	if s.SpanID != 0 || s.TraceID != 0 {
		return s
	}

	ctx := eval.NewContext(e)
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

func newUserContextSerializer(e *model.Event) *UserContextSerializer {
	return &UserContextSerializer{
		User:  e.ProcessContext.User,
		Group: e.ProcessContext.Group,
	}
}

func newProcessContextSerializer(pc *model.ProcessContext, e *model.Event, resolvers *resolvers.Resolvers) *ProcessContextSerializer {
	if pc == nil || pc.Pid == 0 || e == nil {
		return nil
	}

	lastPid := pc.Pid

	ps := ProcessContextSerializer{
		ProcessSerializer: newProcessSerializer(&pc.Process, e, resolvers),
	}

	ctx := eval.NewContext(e)

	it := &model.ProcessAncestorsIterator{}
	ptr := it.Front(ctx)

	var ancestor *model.ProcessCacheEntry
	var prev *ProcessSerializer

	first := true

	for ptr != nil {
		pce := (*model.ProcessCacheEntry)(ptr)
		lastPid = pce.Pid

		s := newProcessSerializer(&pce.Process, e, resolvers)
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

	if lastPid != 1 {
		resolvers.ProcessResolver.CountBrokenLineage()
	}

	return &ps
}

func newSELinuxSerializer(e *model.Event) *SELinuxEventSerializer {
	switch e.SELinux.EventKind {
	case model.SELinuxBoolChangeEventKind:
		return &SELinuxEventSerializer{
			BoolChange: &SELinuxBoolChangeSerializer{
				Name:  e.FieldHandlers.ResolveSELinuxBoolName(e, &e.SELinux),
				State: e.SELinux.BoolChangeValue,
			},
		}
	case model.SELinuxStatusChangeEventKind:
		return &SELinuxEventSerializer{
			EnforceStatus: &SELinuxEnforceStatusSerializer{
				Status: e.SELinux.EnforceStatus,
			},
		}
	case model.SELinuxBoolCommitEventKind:
		return &SELinuxEventSerializer{
			BoolCommit: &SELinuxBoolCommitSerializer{
				State: e.SELinux.BoolCommitValue,
			},
		}
	default:
		return nil
	}
}

func newBPFMapSerializer(e *model.Event) *BPFMapSerializer {
	if e.BPF.Map.ID == 0 {
		return nil
	}
	return &BPFMapSerializer{
		Name:    e.BPF.Map.Name,
		MapType: model.BPFMapType(e.BPF.Map.Type).String(),
	}
}

func newBPFProgramSerializer(e *model.Event) *BPFProgramSerializer {
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

func newBPFEventSerializer(e *model.Event) *BPFEventSerializer {
	return &BPFEventSerializer{
		Cmd:     model.BPFCmd(e.BPF.Cmd).String(),
		Map:     newBPFMapSerializer(e),
		Program: newBPFProgramSerializer(e),
	}
}

func newMMapEventSerializer(e *model.Event) *MMapEventSerializer {
	return &MMapEventSerializer{
		Address:    fmt.Sprintf("0x%x", e.MMap.Addr),
		Offset:     e.MMap.Offset,
		Len:        e.MMap.Len,
		Protection: model.Protection(e.MMap.Protection).String(),
		Flags:      model.MMapFlag(e.MMap.Flags).String(),
	}
}

func newMProtectEventSerializer(e *model.Event) *MProtectEventSerializer {
	return &MProtectEventSerializer{
		VMStart:       fmt.Sprintf("0x%x", e.MProtect.VMStart),
		VMEnd:         fmt.Sprintf("0x%x", e.MProtect.VMEnd),
		VMProtection:  model.VMFlag(e.MProtect.VMProtection).String(),
		ReqProtection: model.VMFlag(e.MProtect.ReqProtection).String(),
	}
}

func newPTraceEventSerializer(e *model.Event, resolvers *resolvers.Resolvers) *PTraceEventSerializer {
	return &PTraceEventSerializer{
		Request: model.PTraceRequest(e.PTrace.Request).String(),
		Address: fmt.Sprintf("0x%x", e.PTrace.Address),
		Tracee:  newProcessContextSerializer(e.PTrace.Tracee, e, resolvers),
	}
}

func newLoadModuleEventSerializer(e *model.Event) *ModuleEventSerializer {
	loadedFromMemory := e.LoadModule.LoadedFromMemory
	argsTruncated := e.LoadModule.ArgsTruncated
	return &ModuleEventSerializer{
		Name:             e.LoadModule.Name,
		LoadedFromMemory: &loadedFromMemory,
		Argv:             e.FieldHandlers.ResolveModuleArgv(e, &e.LoadModule),
		ArgsTruncated:    &argsTruncated,
	}
}

func newUnloadModuleEventSerializer(e *model.Event) *ModuleEventSerializer {
	return &ModuleEventSerializer{
		Name: e.UnloadModule.Name,
	}
}

func newSignalEventSerializer(e *model.Event, resolvers *resolvers.Resolvers) *SignalEventSerializer {
	ses := &SignalEventSerializer{
		Type:   model.Signal(e.Signal.Type).String(),
		PID:    e.Signal.PID,
		Target: newProcessContextSerializer(e.Signal.Target, e, resolvers),
	}
	return ses
}

func newSpliceEventSerializer(e *model.Event) *SpliceEventSerializer {
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

func newNetworkDeviceSerializer(e *model.Event) *NetworkDeviceSerializer {
	return &NetworkDeviceSerializer{
		NetNS:   e.NetworkContext.Device.NetNS,
		IfIndex: e.NetworkContext.Device.IfIndex,
		IfName:  e.FieldHandlers.ResolveNetworkDeviceIfName(e, &e.NetworkContext.Device),
	}
}

func newNetworkContextSerializer(e *model.Event) *NetworkContextSerializer {
	return &NetworkContextSerializer{
		Device:      newNetworkDeviceSerializer(e),
		L3Protocol:  model.L3Protocol(e.NetworkContext.L3Protocol).String(),
		L4Protocol:  model.L4Protocol(e.NetworkContext.L4Protocol).String(),
		Source:      newIPPortSerializer(&e.NetworkContext.Source),
		Destination: newIPPortSerializer(&e.NetworkContext.Destination),
		Size:        e.NetworkContext.Size,
	}
}

func newBindEventSerializer(e *model.Event) *BindEventSerializer {
	bes := &BindEventSerializer{
		Addr: newIPPortFamilySerializer(&e.Bind.Addr,
			model.AddressFamily(e.Bind.AddrFamily).String()),
	}
	return bes
}

func newExitEventSerializer(e *model.Event) *ExitEventSerializer {
	return &ExitEventSerializer{
		Cause: model.ExitCause(e.Exit.Cause).String(),
		Code:  e.Exit.Code,
	}
}

func newMountEventSerializer(e *model.Event, resolvers *resolvers.Resolvers) *MountEventSerializer {
	fh := e.FieldHandlers

	src, srcErr := resolvers.PathResolver.ResolveMountRoot(e, &e.Mount.Mount)
	dst, dstErr := resolvers.PathResolver.ResolveMountPoint(e, &e.Mount.Mount)
	mountPointPath := fh.ResolveMountPointPath(e, &e.Mount)
	mountSourcePath := fh.ResolveMountSourcePath(e, &e.Mount)

	mountSerializer := &MountEventSerializer{
		MountPoint: &FileSerializer{
			Path:    dst,
			MountID: &e.Mount.ParentMountID,
			Inode:   &e.Mount.ParentInode,
		},
		Root: &FileSerializer{
			Path:    src,
			MountID: &e.Mount.RootMountID,
			Inode:   &e.Mount.RootInode,
		},
		MountID:         e.Mount.MountID,
		GroupID:         e.Mount.GroupID,
		ParentMountID:   e.Mount.ParentMountID,
		BindSrcMountID:  e.Mount.BindSrcMountID,
		Device:          e.Mount.Device,
		FSType:          e.Mount.GetFSType(),
		MountPointPath:  mountPointPath,
		MountSourcePath: mountSourcePath,
	}

	if srcErr != nil {
		mountSerializer.Root.PathResolutionError = srcErr.Error()
	}
	if dstErr != nil {
		mountSerializer.MountPoint.PathResolutionError = dstErr.Error()
	}
	// potential errors retrieved from ResolveMountPointPath and ResolveMountSourcePath
	if e.Mount.MountPointPathResolutionError != nil {
		mountSerializer.MountPointPathResolutionError = e.Mount.MountPointPathResolutionError.Error()
	}
	if e.Mount.MountSourcePathResolutionError != nil {
		mountSerializer.MountSourcePathResolutionError = e.Mount.MountSourcePathResolutionError.Error()
	}

	return mountSerializer
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

func MarshalEvent(event *model.Event, probe *resolvers.Resolvers) ([]byte, error) {
	s := NewEventSerializer(event, probe)
	w := &jwriter.Writer{
		Flags: jwriter.NilSliceAsEmpty | jwriter.NilMapAsEmpty,
	}
	s.MarshalEasyJSON(w)
	return w.BuildBytes()
}

func MarshalCustomEvent(event *events.CustomEvent) ([]byte, error) {
	w := &jwriter.Writer{
		Flags: jwriter.NilSliceAsEmpty | jwriter.NilMapAsEmpty,
	}
	event.MarshalEasyJSON(w)
	return w.BuildBytes()
}

// NewEventSerializer creates a new event serializer based on the event type
func NewEventSerializer(event *model.Event, resolvers *resolvers.Resolvers) *EventSerializer {
	var pc model.ProcessContext
	if entry, _ := event.FieldHandlers.ResolveProcessCacheEntry(event); entry != nil {
		pc = entry.ProcessContext
	}

	s := &EventSerializer{
		EventContextSerializer: EventContextSerializer{
			Name:  model.EventType(event.Type).String(),
			Async: event.FieldHandlers.ResolveAsync(event),
		},
		ProcessContextSerializer: newProcessContextSerializer(&pc, event, resolvers),
		DDContextSerializer:      newDDContextSerializer(event),
		UserContextSerializer:    newUserContextSerializer(event),
		Date:                     utils.NewEasyjsonTime(event.FieldHandlers.ResolveEventTimestamp(event)),
	}

	if id := event.FieldHandlers.ResolveContainerID(event, &event.ContainerContext); id != "" {
		var creationTime time.Time
		if cgroup, _ := resolvers.CGroupResolver.GetWorkload(id); cgroup != nil {
			creationTime = time.Unix(0, int64(cgroup.CreationTime))
		}
		s.ContainerContextSerializer = &ContainerContextSerializer{
			ID:        id,
			CreatedAt: getTimeIfNotZero(creationTime),
		}
	}

	eventType := model.EventType(event.Type)

	s.Category = model.GetEventTypeCategory(eventType.String())

	if s.Category == model.NetworkCategory {
		s.NetworkContextSerializer = newNetworkContextSerializer(event)
	}

	if profile.IsAnomalyDetectionEvent(eventType) {
		s.SecurityProfileContextSerializer = newSecurityProfileContextSerializer(&event.SecurityProfileContext)
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
				XAttrName:      event.FieldHandlers.ResolveXAttrName(event, &event.RemoveXAttr),
				XAttrNamespace: event.FieldHandlers.ResolveXAttrNamespace(event, &event.RemoveXAttr),
			},
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.RemoveXAttr.Retval)
	case model.FileSetXAttrEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.SetXAttr.File, event),
			Destination: &FileSerializer{
				XAttrName:      event.FieldHandlers.ResolveXAttrName(event, &event.SetXAttr),
				XAttrNamespace: event.FieldHandlers.ResolveXAttrNamespace(event, &event.SetXAttr),
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
		s.MountEventSerializer = newMountEventSerializer(event, resolvers)
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Mount.Retval)
	case model.FileUmountEventType:
		s.FileEventSerializer = &FileEventSerializer{
			NewMountID: event.Umount.MountID,
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(event.Umount.Retval)
	case model.SetuidEventType:
		s.ProcessContextSerializer.Credentials.Destination = &SetuidSerializer{
			UID:    int(event.SetUID.UID),
			User:   event.FieldHandlers.ResolveSetuidUser(event, &event.SetUID),
			EUID:   int(event.SetUID.EUID),
			EUser:  event.FieldHandlers.ResolveSetuidEUser(event, &event.SetUID),
			FSUID:  int(event.SetUID.FSUID),
			FSUser: event.FieldHandlers.ResolveSetuidFSUser(event, &event.SetUID),
		}
		s.EventContextSerializer.Outcome = serializeSyscallRetval(0)
	case model.SetgidEventType:
		s.ProcessContextSerializer.Credentials.Destination = &SetgidSerializer{
			GID:     int(event.SetGID.GID),
			Group:   event.FieldHandlers.ResolveSetgidGroup(event, &event.SetGID),
			EGID:    int(event.SetGID.EGID),
			EGroup:  event.FieldHandlers.ResolveSetgidEGroup(event, &event.SetGID),
			FSGID:   int(event.SetGID.FSGID),
			FSGroup: event.FieldHandlers.ResolveSetgidFSGroup(event, &event.SetGID),
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
		s.PTraceEventSerializer = newPTraceEventSerializer(event, resolvers)
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
		s.SignalEventSerializer = newSignalEventSerializer(event, resolvers)
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
	case model.AnomalyDetectionSyscallEventType:
		s.AnomalyDetectionSyscallEventSerializer = newAnomalyDetectionSyscallEventSerializer(&event.AnomalyDetectionSyscallEvent)
	}

	return s
}
