//go:generate go run github.com/mailru/easyjson/easyjson -gen_build_flags=-mod=mod -no_std_marshalers -build_tags linux $GOFILE
//go:generate go run github.com/mailru/easyjson/easyjson -gen_build_flags=-mod=mod -no_std_marshalers -build_tags linux -output_filename serializers_base_linux_easyjson.go serializers_base.go
//go:generate go run github.com/DataDog/datadog-agent/pkg/security/generators/backend_doc -output ../../../docs/cloud-workload-security/backend_linux.schema.json

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package serializers holds serializers related files
package serializers

import (
	"fmt"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/events"
	sprocess "github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

const processTreeMaxDepth = 200

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
	// List of cryptographic hashes of the file
	Hashes []string `json:"hashes,omitempty"`
	// State of the hashes or reason why they weren't computed
	HashState string `json:"hash_state,omitempty"`
	// MountPath path of the mount
	MountPath string `json:"mount_path,omitempty"`
	// MountSource source of the mount
	MountSource string `json:"mount_source,omitempty"`
	// MountOrigin origin of the mount
	MountOrigin string `json:"mount_origin,omitempty"`
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
	// Login UID
	AUID int `json:"auid"`
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

// UserSessionContextSerializer serializes the user session context to JSON
// easyjson:json
type UserSessionContextSerializer struct {
	// Unique identifier of the user session on the host
	ID string `json:"id,omitempty"`
	// Type of the user session
	SessionType string `json:"session_type,omitempty"`
	// Username of the Kubernetes "kubectl exec" session
	K8SUsername string `json:"k8s_username,omitempty"`
	// UID of the Kubernetes "kubectl exec" session
	K8SUID string `json:"k8s_uid,omitempty"`
	// Groups of the Kubernetes "kubectl exec" session
	K8SGroups []string `json:"k8s_groups,omitempty"`
	// Extra of the Kubernetes "kubectl exec" session
	K8SExtra map[string][]string `json:"k8s_extra,omitempty"`
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
	// Context of the user session for this event
	UserSession *UserSessionContextSerializer `json:"user_session,omitempty"`
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
	// Indicates whether the process is an exec following another exec
	IsExecExec bool `json:"is_exec_child,omitempty"`
	// Process source
	Source string `json:"source,omitempty"`
	// List of syscalls captured to generate the event
	Syscalls *SyscallsEventSerializer `json:"syscalls,omitempty"`
	// List of AWS Security Credentials that the process had access to
	AWSSecurityCredentials []*AWSSecurityCredentialsSerializer `json:"aws_security_credentials,omitempty"`
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
	// Device associated with the file
	Device uint32 `json:"device,omitempty"`
	// Filesystem type
	FSType string `json:"fstype,omitempty"`
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
	Len uint64 `json:"length"`
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
	Addr IPPortFamilySerializer `json:"addr"`
}

// ConnectEventSerializer serializes a connect event to JSON
// easyjson:json
type ConnectEventSerializer struct {
	Addr IPPortFamilySerializer `json:"addr"`
}

// MountEventSerializer serializes a mount event to JSON
// easyjson:json
type MountEventSerializer struct {
	// Mount point file information
	MountPoint *FileSerializer `json:"mp,omitempty"`
	// Root file information
	Root *FileSerializer `json:"root,omitempty"`
	// Mount ID of the new mount
	MountID uint32 `json:"mount_id"`
	// Mount ID of the parent mount
	ParentMountID uint32 `json:"parent_mount_id"`
	// Mount ID of the source of a bind mount
	BindSrcMountID uint32 `json:"bind_src_mount_id"`
	// Device associated with the file
	Device uint32 `json:"device"`
	// Filesystem type
	FSType string `json:"fs_type,omitempty"`
	// Mount point path
	MountPointPath string `json:"mountpoint.path,omitempty"`
	// Mount source path
	MountSourcePath string `json:"source.path,omitempty"`
	// Mount point path error
	MountRootPathResolutionError string `json:"mountpoint.path_error,omitempty"`
	// Mount source path error
	MountSourcePathResolutionError string `json:"source.path_error,omitempty"`
}

// SecurityProfileContextSerializer serializes the security profile context in an event
// easyjson:json
type SecurityProfileContextSerializer struct {
	// Name of the security profile
	Name string `json:"name"`
	// Version of the profile in use
	Version string `json:"version"`
	// List of tags associated to this profile
	Tags []string `json:"tags"`
	// True if the corresponding event is part of this profile
	EventInProfile bool `json:"event_in_profile"`
	// State of the event type in this profile
	EventTypeState string `json:"event_type_state"`
}

// SyscallSerializer serializes a syscall
type SyscallSerializer struct {
	// Name of the syscall
	Name string `json:"name"`
	// ID of the syscall in the host architecture
	ID int `json:"id"`
}

// SyscallsEventSerializer serializes the syscalls from a syscalls event
type SyscallsEventSerializer []SyscallSerializer

// AnomalyDetectionSyscallEventSerializer serializes an anomaly detection for a syscall event
// easyjson:json
type AnomalyDetectionSyscallEventSerializer struct {
	// Name of the syscall that triggered the anomaly detection event
	Syscall string `json:"syscall"`
}

// SyscallArgsSerializer args serializer
// easyjson:json
type SyscallArgsSerializer struct {
	// Path argument
	Path *string `json:"path,omitempty"`
	// Flags argument
	Flags *int `json:"flags,omitempty"`
	// Mode argument
	Mode *int `json:"mode,omitempty"`
	// UID argument
	UID *int `json:"uid,omitempty"`
	// GID argument
	GID *int `json:"gid,omitempty"`
	// Directory file descriptor argument
	DirFd *int `json:"dirfd,omitempty"`
	// Destination path argument
	DestinationPath *string `json:"destination_path,omitempty"`
	// File system type argument
	FSType *string `json:"fs_type,omitempty"`
}

func newSyscallArgsSerializer(sc *model.SyscallContext, e *model.Event) *SyscallArgsSerializer {

	switch e.GetEventType() {
	case model.FileChmodEventType:
		path := e.FieldHandlers.ResolveSyscallCtxArgsStr1(e, sc)
		mode := e.FieldHandlers.ResolveSyscallCtxArgsInt2(e, sc)
		return &SyscallArgsSerializer{
			Path: &path,
			Mode: &mode,
		}
	case model.FileChdirEventType, model.ExecEventType, model.FileUtimesEventType:
		path := e.FieldHandlers.ResolveSyscallCtxArgsStr1(e, sc)
		return &SyscallArgsSerializer{
			Path: &path,
		}
	case model.FileOpenEventType:
		path := e.FieldHandlers.ResolveSyscallCtxArgsStr1(e, sc)
		flags := e.FieldHandlers.ResolveSyscallCtxArgsInt2(e, sc)
		mode := e.FieldHandlers.ResolveSyscallCtxArgsInt3(e, sc)
		return &SyscallArgsSerializer{
			Path:  &path,
			Flags: &flags,
			Mode:  &mode,
		}
	case model.FileChownEventType:
		path := e.FieldHandlers.ResolveSyscallCtxArgsStr1(e, sc)
		uid := e.FieldHandlers.ResolveSyscallCtxArgsInt2(e, sc)
		gid := e.FieldHandlers.ResolveSyscallCtxArgsInt3(e, sc)
		return &SyscallArgsSerializer{
			Path: &path,
			UID:  &uid,
			GID:  &gid,
		}
	case model.FileUnlinkEventType:
		dirfd := e.FieldHandlers.ResolveSyscallCtxArgsInt1(e, sc)
		path := e.FieldHandlers.ResolveSyscallCtxArgsStr2(e, sc)
		flags := e.FieldHandlers.ResolveSyscallCtxArgsInt3(e, sc)
		return &SyscallArgsSerializer{
			DirFd: &dirfd,
			Path:  &path,
			Flags: &flags,
		}
	case model.FileLinkEventType, model.FileRenameEventType:
		path := e.FieldHandlers.ResolveSyscallCtxArgsStr1(e, sc)
		destinationPath := e.FieldHandlers.ResolveSyscallCtxArgsStr2(e, sc)
		return &SyscallArgsSerializer{
			Path:            &path,
			DestinationPath: &destinationPath,
		}
	case model.FileMountEventType:
		sourcePath := e.FieldHandlers.ResolveSyscallCtxArgsStr1(e, sc)
		mountPointPath := e.FieldHandlers.ResolveSyscallCtxArgsStr2(e, sc)
		fstype := e.FieldHandlers.ResolveSyscallCtxArgsStr3(e, sc)
		return &SyscallArgsSerializer{
			Path:            &sourcePath,
			DestinationPath: &mountPointPath,
			FSType:          &fstype,
		}
	}

	return nil
}

// SyscallContextSerializer serializes syscall context
// easyjson:json
type SyscallContextSerializer struct {
	Chmod  *SyscallArgsSerializer `json:"chmod,omitempty"`
	Chown  *SyscallArgsSerializer `json:"chown,omitempty"`
	Chdir  *SyscallArgsSerializer `json:"chdir,omitempty"`
	Exec   *SyscallArgsSerializer `json:"exec,omitempty"`
	Open   *SyscallArgsSerializer `json:"open,omitempty"`
	Unlink *SyscallArgsSerializer `json:"unlink,omitempty"`
	Link   *SyscallArgsSerializer `json:"link,omitempty"`
	Rename *SyscallArgsSerializer `json:"rename,omitempty"`
	Utimes *SyscallArgsSerializer `json:"utimes,omitempty"`
	Mount  *SyscallArgsSerializer `json:"mount,omitempty"`
}

func newSyscallContextSerializer(sc *model.SyscallContext, e *model.Event, attachEventypeCb func(*SyscallContextSerializer, *SyscallArgsSerializer)) *SyscallContextSerializer {
	e.FieldHandlers.ResolveSyscallCtxArgs(e, sc)
	if !sc.Resolved {
		return nil
	}

	syscallArgsSerializer := newSyscallArgsSerializer(sc, e)
	if syscallArgsSerializer == nil {
		return nil
	}

	syscallContextSerializer := &SyscallContextSerializer{}
	attachEventypeCb(syscallContextSerializer, syscallArgsSerializer)

	return syscallContextSerializer
}

// EventSerializer serializes an event to JSON
// easyjson:json
type EventSerializer struct {
	*BaseEventSerializer

	*NetworkContextSerializer         `json:"network,omitempty"`
	*DDContextSerializer              `json:"dd,omitempty"`
	*SecurityProfileContextSerializer `json:"security_profile,omitempty"`

	*SELinuxEventSerializer   `json:"selinux,omitempty"`
	*BPFEventSerializer       `json:"bpf,omitempty"`
	*MMapEventSerializer      `json:"mmap,omitempty"`
	*MProtectEventSerializer  `json:"mprotect,omitempty"`
	*PTraceEventSerializer    `json:"ptrace,omitempty"`
	*ModuleEventSerializer    `json:"module,omitempty"`
	*SignalEventSerializer    `json:"signal,omitempty"`
	*SpliceEventSerializer    `json:"splice,omitempty"`
	*DNSEventSerializer       `json:"dns,omitempty"`
	*IMDSEventSerializer      `json:"imds,omitempty"`
	*BindEventSerializer      `json:"bind,omitempty"`
	*ConnectEventSerializer   `json:"connect,omitempty"`
	*MountEventSerializer     `json:"mount,omitempty"`
	*SyscallsEventSerializer  `json:"syscalls,omitempty"`
	*UserContextSerializer    `json:"usr,omitempty"`
	*SyscallContextSerializer `json:"syscall,omitempty"`
	*RawPacketSerializer      `json:"packet,omitempty"`
}

func newSyscallsEventSerializer(e *model.SyscallsEvent) *SyscallsEventSerializer {
	ses := SyscallsEventSerializer{}
	for _, s := range e.Syscalls {
		ses = append(ses, SyscallSerializer{
			ID:   int(s),
			Name: s.String(),
		})
	}
	return &ses
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

	fs := &FileSerializer{
		Path:                e.FieldHandlers.ResolveFilePath(e, fe),
		PathResolutionError: fe.GetPathResolutionError(),
		Name:                e.FieldHandlers.ResolveFileBasename(e, fe),
		Inode:               createNumPointer(inode),
		MountID:             createNumPointer(fe.MountID),
		Filesystem:          e.FieldHandlers.ResolveFileFilesystem(e, fe),
		Mode:                createNumPointer(uint32(fe.FileFields.Mode)), // only used by open events
		UID:                 int64(fe.UID),
		GID:                 int64(fe.GID),
		User:                e.FieldHandlers.ResolveFileFieldsUser(e, &fe.FileFields),
		Group:               e.FieldHandlers.ResolveFileFieldsGroup(e, &fe.FileFields),
		Mtime:               utils.NewEasyjsonTimeIfNotZero(time.Unix(0, int64(fe.MTime))),
		Ctime:               utils.NewEasyjsonTimeIfNotZero(time.Unix(0, int64(fe.CTime))),
		InUpperLayer:        getInUpperLayer(&fe.FileFields),
		PackageName:         e.FieldHandlers.ResolvePackageName(e, fe),
		PackageVersion:      e.FieldHandlers.ResolvePackageVersion(e, fe),
		HashState:           fe.HashState.String(),
		MountPath:           fe.MountPath,
		MountSource:         model.MountSourceToString(fe.MountSource),
		MountOrigin:         model.MountOriginToString(fe.MountOrigin),
	}

	// lazy hash serialization: we don't want to hash files for every event
	if fe.HashState == model.Done {
		fs.Hashes = fe.Hashes
	} else if e.IsAnomalyDetectionEvent() {
		fs.Hashes = e.FieldHandlers.ResolveHashesFromEvent(e, fe)
		fs.HashState = fe.HashState.String()
	}
	return fs
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
		AUID:         int(ce.AUID),
		CapEffective: model.KernelCapability(ce.CapEffective).StringArray(),
		CapPermitted: model.KernelCapability(ce.CapPermitted).StringArray(),
	}
}

func newProcessSerializer(ps *model.Process, e *model.Event) *ProcessSerializer {
	if ps.IsNotKworker() {
		argv := e.FieldHandlers.ResolveProcessArgvScrubbed(e, ps)
		argvTruncated := e.FieldHandlers.ResolveProcessArgsTruncated(e, ps)
		envs := e.FieldHandlers.ResolveProcessEnvs(e, ps)
		envsTruncated := e.FieldHandlers.ResolveProcessEnvsTruncated(e, ps)
		argv0, _ := sprocess.GetProcessArgv0(ps)

		psSerializer := &ProcessSerializer{
			ForkTime: utils.NewEasyjsonTimeIfNotZero(ps.ForkTime),
			ExecTime: utils.NewEasyjsonTimeIfNotZero(ps.ExecTime),
			ExitTime: utils.NewEasyjsonTimeIfNotZero(ps.ExitTime),

			Pid:           ps.Pid,
			Tid:           ps.Tid,
			PPid:          createNumPointer(ps.PPid),
			Comm:          ps.Comm,
			TTY:           ps.TTYName,
			Executable:    newFileSerializer(&ps.FileEvent, e),
			Argv0:         argv0,
			Args:          argv,
			ArgsTruncated: argvTruncated,
			Envs:          envs,
			EnvsTruncated: envsTruncated,
			IsThread:      ps.IsThread,
			IsKworker:     ps.IsKworker,
			IsExecExec:    ps.IsExecExec,
			Source:        model.ProcessSourceToString(ps.Source),
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

		if ps.UserSession.ID != 0 {
			psSerializer.UserSession = newUserSessionContextSerializer(&ps.UserSession, e)
		}

		awsSecurityCredentials := e.FieldHandlers.ResolveAWSSecurityCredentials(e)
		if len(awsSecurityCredentials) > 0 {
			for _, creds := range awsSecurityCredentials {
				psSerializer.AWSSecurityCredentials = append(psSerializer.AWSSecurityCredentials, newAWSSecurityCredentialsSerializer(&creds))
			}
		}

		if len(ps.ContainerID) != 0 {
			psSerializer.Container = &ContainerContextSerializer{
				ID:        string(ps.ContainerID),
				CreatedAt: utils.NewEasyjsonTimeIfNotZero(time.Unix(0, int64(e.GetContainerCreatedAt()))),
			}
		}

		return psSerializer
	}
	return &ProcessSerializer{
		Pid:        ps.Pid,
		Tid:        ps.Tid,
		IsKworker:  ps.IsKworker,
		IsExecExec: ps.IsExecExec,
		Source:     model.ProcessSourceToString(ps.Source),
		Credentials: &ProcessCredentialsSerializer{
			CredentialsSerializer: &CredentialsSerializer{},
		},
	}
}

func newUserSessionContextSerializer(ctx *model.UserSessionContext, e *model.Event) *UserSessionContextSerializer {
	e.FieldHandlers.ResolveUserSessionContext(ctx)

	return &UserSessionContextSerializer{
		ID:          fmt.Sprintf("%x", ctx.ID),
		SessionType: ctx.SessionType.String(),
		K8SUsername: ctx.K8SUsername,
		K8SUID:      ctx.K8SUID,
		K8SGroups:   ctx.K8SGroups,
		K8SExtra:    ctx.K8SExtra,
	}
}

func newUserContextSerializer(e *model.Event) *UserContextSerializer {
	return &UserContextSerializer{
		User:  e.ProcessContext.User,
		Group: e.ProcessContext.Group,
	}
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

func newPTraceEventSerializer(e *model.Event) *PTraceEventSerializer {
	return &PTraceEventSerializer{
		Request: model.PTraceRequest(e.PTrace.Request).String(),
		Address: fmt.Sprintf("0x%x", e.PTrace.Address),
		Tracee:  newProcessContextSerializer(e.PTrace.Tracee, e),
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

func newSignalEventSerializer(e *model.Event) *SignalEventSerializer {
	ses := &SignalEventSerializer{
		Type:   model.Signal(e.Signal.Type).String(),
		PID:    e.Signal.PID,
		Target: newProcessContextSerializer(e.Signal.Target, e),
	}
	return ses
}

func newSpliceEventSerializer(e *model.Event) *SpliceEventSerializer {
	return &SpliceEventSerializer{
		PipeEntryFlag: model.PipeBufFlag(e.Splice.PipeEntryFlag).String(),
		PipeExitFlag:  model.PipeBufFlag(e.Splice.PipeExitFlag).String(),
	}
}

func newBindEventSerializer(e *model.Event) *BindEventSerializer {
	bes := &BindEventSerializer{
		Addr: newIPPortFamilySerializer(&e.Bind.Addr,
			model.AddressFamily(e.Bind.AddrFamily).String()),
	}
	return bes
}

func newConnectEventSerializer(e *model.Event) *ConnectEventSerializer {
	ces := &ConnectEventSerializer{
		Addr: newIPPortFamilySerializer(&e.Connect.Addr,
			model.AddressFamily(e.Connect.AddrFamily).String()),
	}
	return ces
}

func newMountEventSerializer(e *model.Event) *MountEventSerializer {
	fh := e.FieldHandlers

	//src, srcErr := , e.Mount.MountPointPathResolutionError
	//resolvers.PathResolver.ResolveMountRoot(e, &e.Mount.Mount)
	//dst, dstErr := resolvers.PathResolver.ResolveMountPoint(e, &e.Mount.Mount)
	mountPointPath := fh.ResolveMountPointPath(e, &e.Mount)
	mountSourcePath := fh.ResolveMountSourcePath(e, &e.Mount)

	mountSerializer := &MountEventSerializer{
		MountPoint: &FileSerializer{
			Path:    e.GetMountRootPath(),
			MountID: createNumPointer(e.Mount.ParentPathKey.MountID),
			Inode:   createNumPointer(e.Mount.ParentPathKey.Inode),
		},
		Root: &FileSerializer{
			Path:    e.GetMountMountpointPath(),
			MountID: createNumPointer(e.Mount.RootPathKey.MountID),
			Inode:   createNumPointer(e.Mount.RootPathKey.Inode),
		},
		MountID:         e.Mount.MountID,
		ParentMountID:   e.Mount.ParentPathKey.MountID,
		BindSrcMountID:  e.Mount.BindSrcMountID,
		Device:          e.Mount.Device,
		FSType:          e.Mount.GetFSType(),
		MountPointPath:  mountPointPath,
		MountSourcePath: mountSourcePath,
	}

	// potential errors retrieved from ResolveMountPointPath and ResolveMountSourcePath
	if e.Mount.MountRootPathResolutionError != nil {
		mountSerializer.MountRootPathResolutionError = e.Mount.MountRootPathResolutionError.Error()
	}
	if e.Mount.MountSourcePathResolutionError != nil {
		mountSerializer.MountSourcePathResolutionError = e.Mount.MountSourcePathResolutionError.Error()
	}

	return mountSerializer
}

func newNetworkDeviceSerializer(deviceCtx *model.NetworkDeviceContext, e *model.Event) *NetworkDeviceSerializer {
	return &NetworkDeviceSerializer{
		NetNS:   deviceCtx.NetNS,
		IfIndex: deviceCtx.IfIndex,
		IfName:  e.FieldHandlers.ResolveNetworkDeviceIfName(e, &e.NetworkContext.Device),
	}
}

func newRawPacketEventSerializer(rp *model.RawPacketEvent, e *model.Event) *RawPacketSerializer {
	return &RawPacketSerializer{
		NetworkContextSerializer: newNetworkContextSerializer(e, &rp.NetworkContext),
		TLSContext: &TLSContextSerializer{
			Version: model.TLSVersion(rp.TLSContext.Version).String(),
		},
	}
}

func serializeOutcome(retval int64) string {
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

func newProcessContextSerializer(pc *model.ProcessContext, e *model.Event) *ProcessContextSerializer {
	if pc == nil || pc.Pid == 0 || e == nil {
		return nil
	}

	ps := ProcessContextSerializer{
		ProcessSerializer: newProcessSerializer(&pc.Process, e),
	}

	// add the syscalls from the event only for the top level parent
	if e.GetEventType() == model.SyscallsEventType {
		ps.Syscalls = newSyscallsEventSerializer(&e.Syscalls)
	}

	ctx := eval.NewContext(e)

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
		if ancestor != nil && ancestor.ArgsEntry != nil && ancestor.ArgsEntry == pce.ArgsEntry {
			prev.Args, prev.ArgsTruncated = prev.Args[0:0], false
			prev.Envs, prev.EnvsTruncated = prev.Envs[0:0], false
			prev.Argv0 = ""
		}
		ancestor = pce
		prev = s

		ptr = it.Next()
	}

	// shrink the middle of the ancestors list if it is too long
	if len(ps.Ancestors) > processTreeMaxDepth {
		subLen := processTreeMaxDepth / 2
		// we add one extra element to the leaf slice in case processTreeMaxDepth is an odd number
		// this is to make sure the length of the resulting slice matches the value of processTreeMaxDepth
		extraElem := processTreeMaxDepth % 2
		ps.Ancestors = append(ps.Ancestors[0:subLen+extraElem], ps.Ancestors[len(ps.Ancestors)-subLen:]...)
		ps.TruncatedAncestors = true
	}

	return &ps
}

// DDContextSerializer serializes a span context to JSON
// easyjson:json
type DDContextSerializer struct {
	// Span ID used for APM correlation
	SpanID string `json:"span_id,omitempty"`
	// Trace ID used for APM correlation
	TraceID string `json:"trace_id,omitempty"`
}

func newDDContextSerializer(e *model.Event) *DDContextSerializer {
	s := &DDContextSerializer{}
	if e.SpanContext.SpanID != 0 && (e.SpanContext.TraceID.Hi != 0 || e.SpanContext.TraceID.Lo != 0) {
		s.SpanID = fmt.Sprint(e.SpanContext.SpanID)
		s.TraceID = fmt.Sprintf("%x%x", e.SpanContext.TraceID.Hi, e.SpanContext.TraceID.Lo)
		return s
	}

	ctx := eval.NewContext(e)
	it := &model.ProcessAncestorsIterator{}
	ptr := it.Front(ctx)

	for ptr != nil {
		pce := (*model.ProcessCacheEntry)(ptr)

		if pce.SpanID != 0 && (pce.TraceID.Hi != 0 || pce.TraceID.Lo != 0) {
			s.SpanID = fmt.Sprint(pce.SpanID)
			s.TraceID = fmt.Sprintf("%x%x", pce.TraceID.Hi, pce.TraceID.Lo)
			break
		}

		ptr = it.Next()
	}
	return s
}

// nolint: deadcode, unused
func newNetworkContextSerializer(e *model.Event, networkCtx *model.NetworkContext) *NetworkContextSerializer {
	return &NetworkContextSerializer{
		Device:      newNetworkDeviceSerializer(&networkCtx.Device, e),
		L3Protocol:  model.L3Protocol(networkCtx.L3Protocol).String(),
		L4Protocol:  model.L4Protocol(networkCtx.L4Protocol).String(),
		Source:      newIPPortSerializer(&networkCtx.Source),
		Destination: newIPPortSerializer(&networkCtx.Destination),
		Size:        networkCtx.Size,
	}
}

func newSecurityProfileContextSerializer(event *model.Event, e *model.SecurityProfileContext) *SecurityProfileContextSerializer {
	tags := make([]string, len(e.Tags))
	copy(tags, e.Tags)
	return &SecurityProfileContextSerializer{
		Name:           e.Name,
		Version:        e.Version,
		Tags:           tags,
		EventInProfile: event.IsInProfile(),
		EventTypeState: e.EventTypeState.String(),
	}
}

// ToJSON returns json
func (e *EventSerializer) ToJSON() ([]byte, error) {
	return utils.MarshalEasyJSON(e)
}

// MarshalJSON returns json
func (e *EventSerializer) MarshalJSON() ([]byte, error) {
	return utils.MarshalEasyJSON(e)
}

// MarshalEvent marshal the event
func MarshalEvent(event *model.Event, opts *eval.Opts) ([]byte, error) {
	s := NewEventSerializer(event, opts)
	return utils.MarshalEasyJSON(s)
}

// MarshalCustomEvent marshal the custom event
func MarshalCustomEvent(event *events.CustomEvent) ([]byte, error) {
	return event.MarshalJSON()
}

// NewEventSerializer creates a new event serializer based on the event type
func NewEventSerializer(event *model.Event, opts *eval.Opts) *EventSerializer {
	s := &EventSerializer{
		BaseEventSerializer:   NewBaseEventSerializer(event, opts),
		UserContextSerializer: newUserContextSerializer(event),
		DDContextSerializer:   newDDContextSerializer(event),
	}
	s.Async = event.FieldHandlers.ResolveAsync(event)

	if !event.NetworkContext.IsZero() {
		s.NetworkContextSerializer = newNetworkContextSerializer(event, &event.NetworkContext)
	}

	if event.SecurityProfileContext.Name != "" {
		s.SecurityProfileContextSerializer = newSecurityProfileContextSerializer(event, &event.SecurityProfileContext)
	}

	if ctx, exists := event.FieldHandlers.ResolveContainerContext(event); exists {
		s.ContainerContextSerializer = &ContainerContextSerializer{
			ID:        string(ctx.ContainerID),
			CreatedAt: utils.NewEasyjsonTimeIfNotZero(time.Unix(0, int64(ctx.CreatedAt))),
			Variables: newVariablesContext(event, opts, "container."),
		}
	}

	if cgroupID := event.FieldHandlers.ResolveCGroupID(event, &event.CGroupContext); cgroupID != "" {
		manager := event.FieldHandlers.ResolveCGroupManager(event, &event.CGroupContext)
		s.CGroupContextSerializer = &CGroupContextSerializer{
			ID:      string(event.CGroupContext.CGroupID),
			Manager: manager,
		}
	}

	eventType := model.EventType(event.Type)

	switch eventType {
	case model.FileChmodEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Chmod.File, event),
			Destination: &FileSerializer{
				Mode: createNumPointer(event.Chmod.Mode),
			},
		}
		s.EventContextSerializer.Outcome = serializeOutcome(event.Chmod.Retval)
		s.SyscallContextSerializer = newSyscallContextSerializer(&event.Chmod.SyscallContext, event, func(ctx *SyscallContextSerializer, args *SyscallArgsSerializer) {
			ctx.Chmod = args
		})
	case model.FileChownEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Chown.File, event),
			Destination: &FileSerializer{
				UID: event.Chown.UID,
				GID: event.Chown.GID,
			},
		}
		s.EventContextSerializer.Outcome = serializeOutcome(event.Chown.Retval)
		s.SyscallContextSerializer = newSyscallContextSerializer(&event.Chown.SyscallContext, event, func(ctx *SyscallContextSerializer, args *SyscallArgsSerializer) {
			ctx.Chown = args
		})
	case model.FileLinkEventType:
		// use the source inode as the target one is a fake inode
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Link.Source, event),
			Destination:    newFileSerializer(&event.Link.Target, event, event.Link.Source.Inode),
		}
		s.EventContextSerializer.Outcome = serializeOutcome(event.Link.Retval)
		s.SyscallContextSerializer = newSyscallContextSerializer(&event.Link.SyscallContext, event, func(ctx *SyscallContextSerializer, args *SyscallArgsSerializer) {
			ctx.Link = args
		})
	case model.FileOpenEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Open.File, event),
		}

		if event.Open.Flags&syscall.O_CREAT > 0 {
			s.FileEventSerializer.Destination = &FileSerializer{
				Mode: createNumPointer(event.Open.Mode),
			}
		}

		s.FileSerializer.Flags = model.OpenFlags(event.Open.Flags).StringArray()
		s.EventContextSerializer.Outcome = serializeOutcome(event.Open.Retval)
		s.SyscallContextSerializer = newSyscallContextSerializer(&event.Open.SyscallContext, event, func(ctx *SyscallContextSerializer, args *SyscallArgsSerializer) {
			ctx.Open = args
		})
	case model.FileMkdirEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Mkdir.File, event),
			Destination: &FileSerializer{
				Mode: createNumPointer(event.Mkdir.Mode),
			},
		}
		s.EventContextSerializer.Outcome = serializeOutcome(event.Mkdir.Retval)
	case model.FileRmdirEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Rmdir.File, event),
		}
		s.EventContextSerializer.Outcome = serializeOutcome(event.Rmdir.Retval)
	case model.FileChdirEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Chdir.File, event),
		}
		s.EventContextSerializer.Outcome = serializeOutcome(event.Chdir.Retval)
		s.SyscallContextSerializer = newSyscallContextSerializer(&event.Chdir.SyscallContext, event, func(ctx *SyscallContextSerializer, args *SyscallArgsSerializer) {
			ctx.Chdir = args
		})
	case model.FileUnlinkEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Unlink.File, event),
		}
		s.FileSerializer.Flags = model.UnlinkFlags(event.Unlink.Flags).StringArray()
		s.EventContextSerializer.Outcome = serializeOutcome(event.Unlink.Retval)
		s.SyscallContextSerializer = newSyscallContextSerializer(&event.Unlink.SyscallContext, event, func(ctx *SyscallContextSerializer, args *SyscallArgsSerializer) {
			ctx.Unlink = args
		})
	case model.FileRenameEventType:
		// use the new inode as the old one is a fake inode
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Rename.Old, event, event.Rename.New.Inode),
			Destination:    newFileSerializer(&event.Rename.New, event),
		}
		s.EventContextSerializer.Outcome = serializeOutcome(event.Rename.Retval)
		s.SyscallContextSerializer = newSyscallContextSerializer(&event.Rename.SyscallContext, event, func(ctx *SyscallContextSerializer, args *SyscallArgsSerializer) {
			ctx.Rename = args
		})
	case model.FileRemoveXAttrEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.RemoveXAttr.File, event),
			Destination: &FileSerializer{
				XAttrName:      event.FieldHandlers.ResolveXAttrName(event, &event.RemoveXAttr),
				XAttrNamespace: event.FieldHandlers.ResolveXAttrNamespace(event, &event.RemoveXAttr),
			},
		}
		s.EventContextSerializer.Outcome = serializeOutcome(event.RemoveXAttr.Retval)
	case model.FileSetXAttrEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.SetXAttr.File, event),
			Destination: &FileSerializer{
				XAttrName:      event.FieldHandlers.ResolveXAttrName(event, &event.SetXAttr),
				XAttrNamespace: event.FieldHandlers.ResolveXAttrNamespace(event, &event.SetXAttr),
			},
		}
		s.EventContextSerializer.Outcome = serializeOutcome(event.SetXAttr.Retval)
	case model.FileUtimesEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.Utimes.File, event),
			Destination: &FileSerializer{
				Atime: utils.NewEasyjsonTimeIfNotZero(event.Utimes.Atime),
				Mtime: utils.NewEasyjsonTimeIfNotZero(event.Utimes.Mtime),
			},
		}
		s.EventContextSerializer.Outcome = serializeOutcome(event.Utimes.Retval)
		s.SyscallContextSerializer = newSyscallContextSerializer(&event.Utimes.SyscallContext, event, func(ctx *SyscallContextSerializer, args *SyscallArgsSerializer) {
			ctx.Utimes = args
		})
	case model.FileMountEventType:
		s.MountEventSerializer = newMountEventSerializer(event)
		s.EventContextSerializer.Outcome = serializeOutcome(event.Mount.Retval)
		s.SyscallContextSerializer = newSyscallContextSerializer(&event.Mount.SyscallContext, event, func(ctx *SyscallContextSerializer, args *SyscallArgsSerializer) {
			ctx.Mount = args
		})
	case model.FileUmountEventType:
		s.FileEventSerializer = &FileEventSerializer{
			NewMountID: event.Umount.MountID,
		}
		s.EventContextSerializer.Outcome = serializeOutcome(event.Umount.Retval)
	case model.SetuidEventType:
		s.ProcessContextSerializer.Credentials.Destination = &SetuidSerializer{
			UID:    int(event.SetUID.UID),
			User:   event.FieldHandlers.ResolveSetuidUser(event, &event.SetUID),
			EUID:   int(event.SetUID.EUID),
			EUser:  event.FieldHandlers.ResolveSetuidEUser(event, &event.SetUID),
			FSUID:  int(event.SetUID.FSUID),
			FSUser: event.FieldHandlers.ResolveSetuidFSUser(event, &event.SetUID),
		}
		s.EventContextSerializer.Outcome = serializeOutcome(0)
	case model.SetgidEventType:
		s.ProcessContextSerializer.Credentials.Destination = &SetgidSerializer{
			GID:     int(event.SetGID.GID),
			Group:   event.FieldHandlers.ResolveSetgidGroup(event, &event.SetGID),
			EGID:    int(event.SetGID.EGID),
			EGroup:  event.FieldHandlers.ResolveSetgidEGroup(event, &event.SetGID),
			FSGID:   int(event.SetGID.FSGID),
			FSGroup: event.FieldHandlers.ResolveSetgidFSGroup(event, &event.SetGID),
		}
		s.EventContextSerializer.Outcome = serializeOutcome(0)
	case model.CapsetEventType:
		s.ProcessContextSerializer.Credentials.Destination = &CapsetSerializer{
			CapEffective: model.KernelCapability(event.Capset.CapEffective).StringArray(),
			CapPermitted: model.KernelCapability(event.Capset.CapPermitted).StringArray(),
		}
		s.EventContextSerializer.Outcome = serializeOutcome(0)
	case model.ForkEventType:
		s.EventContextSerializer.Outcome = serializeOutcome(0)
	case model.SELinuxEventType:
		s.EventContextSerializer.Outcome = serializeOutcome(0)
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.SELinux.File, event),
		}
		s.SELinuxEventSerializer = newSELinuxSerializer(event)
	case model.BPFEventType:
		s.EventContextSerializer.Outcome = serializeOutcome(0)
		s.BPFEventSerializer = newBPFEventSerializer(event)
	case model.MMapEventType:
		s.EventContextSerializer.Outcome = serializeOutcome(event.MMap.Retval)
		if event.MMap.Flags&unix.MAP_ANONYMOUS == 0 {
			s.FileEventSerializer = &FileEventSerializer{
				FileSerializer: *newFileSerializer(&event.MMap.File, event),
			}
		}
		s.MMapEventSerializer = newMMapEventSerializer(event)
	case model.MProtectEventType:
		s.EventContextSerializer.Outcome = serializeOutcome(event.MProtect.Retval)
		s.MProtectEventSerializer = newMProtectEventSerializer(event)
	case model.PTraceEventType:
		s.EventContextSerializer.Outcome = serializeOutcome(event.PTrace.Retval)
		s.PTraceEventSerializer = newPTraceEventSerializer(event)
	case model.LoadModuleEventType:
		s.EventContextSerializer.Outcome = serializeOutcome(event.LoadModule.Retval)
		if !event.LoadModule.LoadedFromMemory {
			s.FileEventSerializer = &FileEventSerializer{
				FileSerializer: *newFileSerializer(&event.LoadModule.File, event),
			}
		}
		s.ModuleEventSerializer = newLoadModuleEventSerializer(event)
	case model.UnloadModuleEventType:
		s.EventContextSerializer.Outcome = serializeOutcome(event.UnloadModule.Retval)
		s.ModuleEventSerializer = newUnloadModuleEventSerializer(event)
	case model.SignalEventType:
		s.EventContextSerializer.Outcome = serializeOutcome(event.Signal.Retval)
		s.SignalEventSerializer = newSignalEventSerializer(event)
	case model.SpliceEventType:
		s.EventContextSerializer.Outcome = serializeOutcome(event.Splice.Retval)
		s.SpliceEventSerializer = newSpliceEventSerializer(event)
		if event.Splice.File.Inode != 0 {
			s.FileEventSerializer = &FileEventSerializer{
				FileSerializer: *newFileSerializer(&event.Splice.File, event),
			}
		}
	case model.BindEventType:
		s.EventContextSerializer.Outcome = serializeOutcome(event.Bind.Retval)
		s.BindEventSerializer = newBindEventSerializer(event)
	case model.ConnectEventType:
		s.EventContextSerializer.Outcome = serializeOutcome(event.Connect.Retval)
		s.ConnectEventSerializer = newConnectEventSerializer(event)
	case model.SyscallsEventType:
		s.SyscallsEventSerializer = newSyscallsEventSerializer(&event.Syscalls)
	case model.DNSEventType:
		s.EventContextSerializer.Outcome = serializeOutcome(0)
		s.DNSEventSerializer = newDNSEventSerializer(&event.DNS)
	case model.IMDSEventType:
		s.EventContextSerializer.Outcome = serializeOutcome(0)
		s.IMDSEventSerializer = newIMDSEventSerializer(&event.IMDS)
	case model.ExecEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.ProcessContext.Process.FileEvent, event),
		}
		s.EventContextSerializer.Outcome = serializeOutcome(0)
		s.SyscallContextSerializer = newSyscallContextSerializer(&event.Exec.SyscallContext, event, func(ctx *SyscallContextSerializer, args *SyscallArgsSerializer) {
			ctx.Exec = args
		})
	case model.RawPacketEventType:
		s.RawPacketSerializer = newRawPacketEventSerializer(&event.RawPacket, event)
	}

	return s
}
