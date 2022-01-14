// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

import "github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"

// EventType describes the type of an event sent from the kernel
type EventType uint64

const (
	// UnknownEventType unknow event
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
	// InvalidateDentryEventType Dentry invalidated event
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
	// NetDeviceEventType is sent for events on net devices
	NetDeviceEventType
	// VethPairEventType is sent when a new veth pair is created
	VethPairEventType
	// MaxKernelEventType is used internally to get the maximum number of kernel events.
	MaxKernelEventType

	// FirstDiscarderEventType first event that accepts discarders
	FirstDiscarderEventType = FileOpenEventType

	// LastDiscarderEventType last event that accepts discarders
	LastDiscarderEventType = FileRemoveXAttrEventType

	// CustomLostReadEventType is the custom event used to report lost events detected in user space
	CustomLostReadEventType = iota
	// CustomLostWriteEventType is the custom event used to report lost events detected in kernel space
	CustomLostWriteEventType
	// CustomRulesetLoadedEventType is the custom event used to report that a new ruleset was loaded
	CustomRulesetLoadedEventType
	// CustomNoisyProcessEventType is the custom event used to report the detection of a noisy process
	CustomNoisyProcessEventType
	// CustomForkBombEventType is the custom event used to report the detection of a fork bomb
	CustomForkBombEventType
	// CustomTruncatedParentsEventType is the custom event used to report that the parents of a path were truncated
	CustomTruncatedParentsEventType
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
	case NetDeviceEventType:
		return "net_device"
	case VethPairEventType:
		return "veth_pair"

	case CustomLostReadEventType:
		return "lost_events_read"
	case CustomLostWriteEventType:
		return "lost_events_write"
	case CustomRulesetLoadedEventType:
		return "ruleset_loaded"
	case CustomNoisyProcessEventType:
		return "noisy_process"
	case CustomForkBombEventType:
		return "fork_bomb"
	case CustomTruncatedParentsEventType:
		return "truncated_parents"
	default:
		return "unknown"
	}
}

// ParseEvalEventType convert a eval.EventType (string) to its uint64 representation
// the current algorithm is not efficient but allows us to reduce the number of conversion functions
//nolint:deadcode,unused
func ParseEvalEventType(eventType eval.EventType) EventType {
	for i := uint64(0); i != uint64(MaxAllEventType); i++ {
		if EventType(i).String() == eventType {
			return EventType(i)
		}
	}

	return UnknownEventType
}

var (
	eventTypeStrings = map[string]EventType{}
)

func init() {
	var eventType EventType
	for i := uint64(0); i != uint64(MaxEventType); i++ {
		eventType = EventType(i)
		eventTypeStrings[eventType.String()] = eventType
	}
}

// ParseEventTypeStringSlice converts a list
func ParseEventTypeStringSlice(eventTypes []string) []EventType {
	var output []EventType
	for _, eventTypeStr := range eventTypes {
		if eventType := eventTypeStrings[eventTypeStr]; eventType != UnknownEventType {
			output = append(output, eventType)
		}
	}
	return output
}
