// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package model

import "github.com/DataDog/datadog-agent/pkg/security/secl/eval"

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
	// FileUtimeEventType Utime event
	FileUtimeEventType
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
	// MaxEventType is used internally to get the maximum number of kernel events.
	MaxEventType

	// FirstDiscarderEventType first event that accepts discarders
	FirstDiscarderEventType = FileOpenEventType

	// LastDiscarderEventType last event that accepts discarders
	LastDiscarderEventType = FileRemoveXAttrEventType

	// CustomLostReadEventType is the custom event used to report lost events detected in user space
	CustomLostReadEventType EventType = iota
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
	// CustomTruncatedSegmentEventType is the custom event used to report that a segment of a path was truncated
	CustomTruncatedSegmentEventType
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
	case FileUtimeEventType:
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
	case CustomTruncatedSegmentEventType:
		return "truncated_segment"
	default:
		return "unknown"
	}
}

// ParseEvalEventType convert a eval.EventType (string) to its uint64 representation
// the current algorithm is not efficient but allows us to reduce the number of conversion functions
//nolint:deadcode,unused
func ParseEvalEventType(eventType eval.EventType) EventType {
	for i := uint64(0); i != uint64(MaxEventType); i++ {
		if EventType(i).String() == eventType {
			return EventType(i)
		}
	}

	return UnknownEventType
}
