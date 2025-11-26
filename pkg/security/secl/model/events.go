// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// EventType describes the type of an event sent from the kernel
type EventType uint32

const (
	// UnknownEventType unknown event
	UnknownEventType EventType = iota
	// FileOpenEventType File open event
	FileOpenEventType
	// FileMkdirEventType Folder creation event
	FileMkdirEventType
	// FileLinkEventType Hard link creation event
	FileLinkEventType
	// FileRenameEventType File or folder rename event
	FileRenameEventType
	// FileUnlinkEventType Unlink event
	FileUnlinkEventType
	// FileRmdirEventType Rmdir event
	FileRmdirEventType
	// FileChmodEventType Chmod event
	FileChmodEventType
	// FileChownEventType Chown event
	FileChownEventType
	// FileUtimesEventType Utime event
	FileUtimesEventType
	// FileSetXAttrEventType Setxattr event
	FileSetXAttrEventType
	// FileRemoveXAttrEventType Removexattr event
	FileRemoveXAttrEventType
	// FileChdirEventType chdir event
	FileChdirEventType
	// FileMountEventType Mount event
	FileMountEventType
	// FileUmountEventType Umount event
	FileUmountEventType
	// ForkEventType Fork event
	ForkEventType
	// ExecEventType Exec event
	ExecEventType
	// ExitEventType Exit event
	ExitEventType
	// InvalidateDentryEventType Dentry invalidated event (DEPRECATED)
	InvalidateDentryEventType
	// SetuidEventType setuid event
	SetuidEventType
	// SetgidEventType setgid event
	SetgidEventType
	// CapsetEventType capset event
	CapsetEventType
	// ArgsEnvsEventType args and envs event
	ArgsEnvsEventType
	// MountReleasedEventType sent when a mount point is released
	MountReleasedEventType
	// SELinuxEventType selinux event
	SELinuxEventType
	// BPFEventType bpf event
	BPFEventType
	// PTraceEventType PTrace event
	PTraceEventType
	// MMapEventType MMap event
	MMapEventType
	// MProtectEventType MProtect event
	MProtectEventType
	// LoadModuleEventType LoadModule event
	LoadModuleEventType
	// UnloadModuleEventType UnloadModule evnt
	UnloadModuleEventType
	// SignalEventType Signal event
	SignalEventType
	// SpliceEventType Splice event
	SpliceEventType
	// CgroupTracingEventType is sent when a new cgroup is being traced
	CgroupTracingEventType
	// DNSEventType DNS event
	DNSEventType
	// ShortDNSResponseEventType DNS Response event
	ShortDNSResponseEventType
	// FullDNSResponseEventType DNS Response event
	FullDNSResponseEventType
	// NetDeviceEventType is sent for events on net devices
	NetDeviceEventType
	// VethPairEventType is sent when a new veth pair is created
	VethPairEventType
	// VethPairNsEventType is sent when a veth pair is moved to a new network namespace
	VethPairNsEventType
	// AcceptEventType Accept event
	AcceptEventType
	// BindEventType Bind event
	BindEventType
	// ConnectEventType Connect event
	ConnectEventType
	// UnshareMountNsEventType is sent when a new mount is created from a mount namespace copy
	UnshareMountNsEventType
	// SyscallsEventType Syscalls event
	SyscallsEventType
	// IMDSEventType is sent when an IMDS request or answer is captured
	IMDSEventType
	// OnDemandEventType is sent for on-demand events
	OnDemandEventType
	// LoginUIDWriteEventType is sent for login_uid write events
	LoginUIDWriteEventType
	// CgroupWriteEventType is sent when a new cgroup was created
	CgroupWriteEventType
	// RawPacketFilterEventType raw packet filter event
	RawPacketFilterEventType
	// NetworkFlowMonitorEventType is sent to monitor network activity
	NetworkFlowMonitorEventType
	// PrCtlEventType is sent when a prctl event is captured
	PrCtlEventType
	// StatEventType stat event (used kernel side only)
	StatEventType
	// SysCtlEventType sysctl event
	SysCtlEventType
	// SetrlimitEventType setrlimit event
	SetrlimitEventType
	// SetSockOptEventType is sent when a socket option is set
	SetSockOptEventType
	// FileFsmountEventType Mount event
	FileFsmountEventType
	// FileOpenTreeEventType Open Tree event
	FileOpenTreeEventType
	// RawPacketActionEventType raw packet action event
	RawPacketActionEventType
	// CapabilitiesEventType is used to track capabilities usage
	CapabilitiesEventType
	// FileMoveMountEventType Move Mount even
	FileMoveMountEventType
	// FailedDNSEventType Failed DNS
	FailedDNSEventType
	// TracerMemfdCreateEventType memfd_create event (used kernel side only)
	TracerMemfdCreateEventType
	// TracerMemfdSealEventType Tracer memfd seal event
	TracerMemfdSealEventType
	// SocketEventType is sent when a socket is created
	SocketEventType
	// MaxKernelEventType is used internally to get the maximum number of kernel events.
	MaxKernelEventType

	// FirstEventType is the first valid event type
	FirstEventType = FileOpenEventType

	// LastEventType is the last valid event type
	LastEventType = SyscallsEventType

	// FirstDiscarderEventType first event that accepts discarders
	FirstDiscarderEventType = FileOpenEventType

	// LastDiscarderEventType last event that accepts discarders
	LastDiscarderEventType = FileChdirEventType

	// LastApproverEventType is the last event that accepts approvers
	LastApproverEventType = SpliceEventType

	// CustomEventType represents a custom event type
	CustomEventType EventType = iota

	// CreateNewFileEventType event
	CreateNewFileEventType EventType = iota
	// DeleteFileEventType event
	DeleteFileEventType
	// WriteFileEventType event
	WriteFileEventType
	// CreateRegistryKeyEventType event
	CreateRegistryKeyEventType
	// OpenRegistryKeyEventType event
	OpenRegistryKeyEventType
	// SetRegistryKeyValueEventType event
	SetRegistryKeyValueEventType
	// DeleteRegistryKeyEventType event
	DeleteRegistryKeyEventType
	// ChangePermissionEventType event
	ChangePermissionEventType

	// FirstWindowsEventType is the first Windows event type
	FirstWindowsEventType = CreateNewFileEventType
	// LastWindowsEventType is the last Windows event type
	LastWindowsEventType = ChangePermissionEventType

	// MaxAllEventType is used internally to get the maximum number of events.
	MaxAllEventType
)

func (t EventType) String() string {
	switch t {
	case FileOpenEventType:
		return "open"
	case FileMkdirEventType:
		return "mkdir"
	case FileLinkEventType:
		return "link"
	case FileRenameEventType:
		return "rename"
	case FileUnlinkEventType:
		return "unlink"
	case FileRmdirEventType:
		return "rmdir"
	case FileChmodEventType:
		return "chmod"
	case FileChownEventType:
		return "chown"
	case FileUtimesEventType:
		return "utimes"
	case FileMountEventType:
		return "mount"
	case FileUmountEventType:
		return "umount"
	case FileSetXAttrEventType:
		return "setxattr"
	case FileRemoveXAttrEventType:
		return "removexattr"
	case FileChdirEventType:
		return "chdir"
	case ForkEventType:
		return "fork"
	case ExecEventType:
		return "exec"
	case ExitEventType:
		return "exit"
	case InvalidateDentryEventType:
		return "invalidate_dentry"
	case SetuidEventType:
		return "setuid"
	case SetgidEventType:
		return "setgid"
	case CapsetEventType:
		return "capset"
	case ArgsEnvsEventType:
		return "args_envs"
	case MountReleasedEventType:
		return "mount_released"
	case SELinuxEventType:
		return "selinux"
	case BPFEventType:
		return "bpf"
	case PTraceEventType:
		return "ptrace"
	case MMapEventType:
		return "mmap"
	case MProtectEventType:
		return "mprotect"
	case LoadModuleEventType:
		return "load_module"
	case UnloadModuleEventType:
		return "unload_module"
	case SignalEventType:
		return "signal"
	case SpliceEventType:
		return "splice"
	case CgroupTracingEventType:
		return "cgroup_tracing"
	case DNSEventType:
		return "dns"
	case ShortDNSResponseEventType:
		return "dns_response_short"
	case NetDeviceEventType:
		return "net_device"
	case VethPairEventType:
		return "veth_pair"
	case VethPairNsEventType:
		return "veth_pair_ns"
	case BindEventType:
		return "bind"
	case AcceptEventType:
		return "accept"
	case ConnectEventType:
		return "connect"
	case UnshareMountNsEventType:
		return "unshare_mntns"
	case SyscallsEventType:
		return "syscalls"
	case IMDSEventType:
		return "imds"
	case OnDemandEventType:
		return "ondemand"
	case RawPacketFilterEventType:
		return "packet"
	case RawPacketActionEventType:
		return "packet_action"
	case NetworkFlowMonitorEventType:
		return "network_flow_monitor"
	case StatEventType:
		return "stat"
	case CustomEventType:
		return "custom_event"
	case CreateNewFileEventType:
		return "create"
	case DeleteFileEventType:
		return "delete"
	case WriteFileEventType:
		return "write"
	case CreateRegistryKeyEventType:
		return "create_key"
	case OpenRegistryKeyEventType:
		return "open_key"
	case SetRegistryKeyValueEventType:
		return "set_key_value"
	case DeleteRegistryKeyEventType:
		return "delete_key"
	case ChangePermissionEventType:
		return "change_permission"
	case FailedDNSEventType:
		return "failed_dns"
	case LoginUIDWriteEventType:
		return "login_uid_write"
	case CgroupWriteEventType:
		return "cgroup_write"
	case SysCtlEventType:
		return "sysctl"
	case SetrlimitEventType:
		return "setrlimit"
	case FullDNSResponseEventType:
		return "dns_response"
	case SetSockOptEventType:
		return "setsockopt"
	case CapabilitiesEventType:
		return "capabilities"
	case PrCtlEventType:
		return "prctl"
	case FileFsmountEventType:
		return "fsmount"
	case FileOpenTreeEventType:
		return "open_tree"
	case FileMoveMountEventType:
		return "move_mount"
	case TracerMemfdCreateEventType:
		return "tracer_memfd_create"
	case TracerMemfdSealEventType:
		return "tracer_memfd_seal"
	case SocketEventType:
		return "socket"
	default:
		return "unknown"
	}
}

// ParseEvalEventType convert a eval.EventType (string) to its uint64 representation
// the current algorithm is not efficient but allows us to reduce the number of conversion functions
func ParseEvalEventType(eventType eval.EventType) (EventType, error) {
	for i := uint64(0); i != uint64(MaxAllEventType); i++ {
		if EventType(i).String() == eventType {
			return EventType(i), nil
		}
	}

	return UnknownEventType, fmt.Errorf("unknown event type '%s'", eventType)
}
