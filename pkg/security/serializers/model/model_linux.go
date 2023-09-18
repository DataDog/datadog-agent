//go:generate easyjson -gen_build_flags=-mod=mod -gen_build_goos=$GEN_GOOS -no_std_marshalers -build_tags linux_tmp $GOFILE
//go:generate easyjson -gen_build_flags=-mod=mod -gen_build_goos=$GEN_GOOS -no_std_marshalers -build_tags linux_tmp -output_filename model_base_linux_easyjson.go model_base.go
//go:generate go run github.com/DataDog/datadog-agent/pkg/security/probe/doc_generator -output ../../../../docs/cloud-workload-security/backend.schema.json

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package model holds serializers related files
package model

import "github.com/DataDog/datadog-agent/pkg/security/utils"

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
	// Indicates wether the process is an exec child of its parent
	IsExecChild bool `json:"is_exec_child,omitempty"`
	// Process source
	Source string `json:"source,omitempty"`
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

// MountEventSerializer serializes a mount event to JSON
// easyjson:json
type MountEventSerializer struct {
	MountPoint                     *FileSerializer `json:"mp,omitempty"`                    // Mount point file information
	Root                           *FileSerializer `json:"root,omitempty"`                  // Root file information
	MountID                        uint32          `json:"mount_id"`                        // Mount ID of the new mount
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

// EventSerializer serializes an event to JSON
// easyjson:json
type EventSerializer struct {
	*BaseEventSerializer

	*SELinuxEventSerializer                 `json:"selinux,omitempty"`
	*BPFEventSerializer                     `json:"bpf,omitempty"`
	*MMapEventSerializer                    `json:"mmap,omitempty"`
	*MProtectEventSerializer                `json:"mprotect,omitempty"`
	*PTraceEventSerializer                  `json:"ptrace,omitempty"`
	*ModuleEventSerializer                  `json:"module,omitempty"`
	*SignalEventSerializer                  `json:"signal,omitempty"`
	*SpliceEventSerializer                  `json:"splice,omitempty"`
	*DNSEventSerializer                     `json:"dns,omitempty"`
	*BindEventSerializer                    `json:"bind,omitempty"`
	*MountEventSerializer                   `json:"mount,omitempty"`
	*AnomalyDetectionSyscallEventSerializer `json:"anomaly_detection_syscall,omitempty"`
	*UserContextSerializer                  `json:"usr,omitempty"`
}
