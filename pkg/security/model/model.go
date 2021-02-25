// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

//go:generate go run github.com/DataDog/datadog-agent/pkg/security/secl/generators/accessors -mock -tags linux -output accessors.go
//go:generate go run github.com/DataDog/datadog-agent/pkg/security/secl/generators/accessors -tags linux -output ../probe/accessors.go

package model

import (
	"bytes"
	"fmt"
	"path"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

// Model describes the data model for the runtime security agent events
type Model struct{}

// NewEvent returns a new Event
func (m *Model) NewEvent() eval.Event {
	return &Event{}
}

// ValidateField validates the value of a field
func (m *Model) ValidateField(key string, field eval.FieldValue) error {
	// check that all path are absolute
	if strings.HasSuffix(key, "filename") || strings.HasSuffix(key, "_path") {
		if value, ok := field.Value.(string); ok {
			errAbs := fmt.Errorf("invalid path `%s`, all the path have to be absolute", value)
			errDepth := fmt.Errorf("invalid path `%s`, path depths have to be shorter than %d", value, MaxPathDepth)
			errSegment := fmt.Errorf("invalid path `%s`, each segment of a path must be shorter than %d", value, MaxSegmentLength)

			if value != path.Clean(value) {
				return errAbs
			}

			if matched, err := regexp.Match(`\.\.`, []byte(value)); err != nil || matched {
				return errAbs
			}

			if matched, err := regexp.Match(`^~`, []byte(value)); err != nil || matched {
				return errAbs
			}

			// check resolution limitations
			segments := strings.Split(value, "/")
			if len(segments) > MaxPathDepth {
				return errDepth
			}
			for _, segment := range segments {
				if len(segment) > MaxSegmentLength {
					return errSegment
				}
			}
		}
	}

	switch key {

	case "event.retval":
		if value := field.Value; value != -int(syscall.EPERM) && value != -int(syscall.EACCES) {
			return errors.New("return value can only be tested against EPERM or EACCES")
		}
	}

	return nil
}

// ChmodEvent represents a chmod event
type ChmodEvent struct {
	SyscallEvent
	FileEvent
	Mode uint32 `field:"mode"`
}

// ChownEvent represents a chown event
type ChownEvent struct {
	SyscallEvent
	FileEvent
	UID int32 `field:"uid"`
	GID int32 `field:"gid"`
}

// ContainerContext holds the container context of an event
type ContainerContext struct {
	ID string `field:"id" handler:"ResolveContainerID,string"`
}

// Event represents an event sent from the kernel
// genaccessors
type Event struct {
	ID           string    `field:"-"`
	Type         uint64    `field:"-"`
	TimestampRaw uint64    `field:"-"`
	Timestamp    time.Time `field:"timestamp"`

	Process   ProcessContext   `field:"process" event:"*"`
	Container ContainerContext `field:"container"`

	Chmod       ChmodEvent    `field:"chmod" event:"chmod"`
	Chown       ChownEvent    `field:"chown" event:"chown"`
	Open        OpenEvent     `field:"open" event:"open"`
	Mkdir       MkdirEvent    `field:"mkdir" event:"mkdir"`
	Rmdir       RmdirEvent    `field:"rmdir" event:"rmdir"`
	Rename      RenameEvent   `field:"rename" event:"rename"`
	Unlink      UnlinkEvent   `field:"unlink" event:"unlink"`
	Utimes      UtimesEvent   `field:"utimes" event:"utimes"`
	Link        LinkEvent     `field:"link" event:"link"`
	SetXAttr    SetXAttrEvent `field:"setxattr" event:"setxattr"`
	RemoveXAttr SetXAttrEvent `field:"removexattr" event:"removexattr"`
	Exec        ExecEvent     `field:"exec" event:"exec"`

	Mount            MountEvent            `field:"-"`
	Umount           UmountEvent           `field:"-"`
	InvalidateDentry InvalidateDentryEvent `field:"-"`
}

// GetType returns the event type
func (e *Event) GetType() string {
	return EventType(e.Type).String()
}

// GetEventType returns the event type of the event
func (e *Event) GetEventType() EventType {
	return EventType(e.Type)
}

// GetTags returns the list of tags specific to this event
func (e *Event) GetTags() []string {
	// TODO: add container tags once we collect them
	return []string{"type:" + e.GetType()}
}

// GetPointer return an unsafe.Pointer of the Event
func (e *Event) GetPointer() unsafe.Pointer {
	return unsafe.Pointer(e)
}

// ExecEvent represents a exec event
type ExecEvent struct {
	// proc_cache_t
	// (container context is parsed in Event.Container)
	FileFields

	PathnameStr         string `field:"filename" handler:"ResolveExecInode,string"`
	ContainerPath       string `field:"container_path" handler:"ResolveExecContainerPath,string"`
	BasenameStr         string `field:"name" handler:"ResolveExecBasename,string"`
	PathResolutionError error  `field:"-"`

	ExecTimestamp uint64    `field:"-"`
	ExecTime      time.Time `field:"-"`

	TTYName string `field:"tty_name" handler:"ResolveExecTTY,string"`
	Comm    string `field:"-" handler:"ResolveExecComm,string"`

	// pid_cache_t
	ForkTimestamp uint64    `field:"-"`
	ForkTime      time.Time `field:"-"`

	ExitTimestamp uint64    `field:"-"`
	ExitTime      time.Time `field:"-"`

	Cookie uint32 `field:"cookie" handler:"ResolveExecCookie,int"`
	PPid   uint32 `field:"ppid" handler:"ResolveExecPPID,int"`

	// The following fields should only be used here for evaluation
	UID   uint32 `field:"uid" handler:"ResolveExecUID,int"`
	GID   uint32 `field:"gid" handler:"ResolveExecGID,int"`
	User  string `field:"user" handler:"ResolveExecUser,string"`
	Group string `field:"group" handler:"ResolveExecGroup,string"`
}

// GetPathResolutionError returns the path resolution error as a string if there is one
func (e *ExecEvent) GetPathResolutionError() string {
	if e.PathResolutionError != nil {
		return e.PathResolutionError.Error()
	}
	return ""
}

// FileFields holds the information required to identify a file
type FileFields struct {
	MountID         uint32 `field:"-"`
	Inode           uint64 `field:"inode"`
	PathID          uint32 `field:"-"`
	OverlayNumLower int32  `field:"overlay_numlower"`
}

// FileEvent is the common file event type
type FileEvent struct {
	FileFields
	PathnameStr   string `field:"filename" handler:"ResolveFileInode,string"`
	ContainerPath string `field:"container_path" handler:"ResolveFileContainerPath,string"`
	BasenameStr   string `field:"basename" handler:"ResolveFileBasename,string"`

	PathResolutionError error `field:"-"`
}

// GetPathResolutionError returns the path resolution error as a string if there is one
func (e *FileEvent) GetPathResolutionError() string {
	if e.PathResolutionError != nil {
		return e.PathResolutionError.Error()
	}
	return ""
}

// InvalidateDentryEvent defines a invalidate dentry event
type InvalidateDentryEvent struct {
	Inode             uint64
	MountID           uint32
	DiscarderRevision uint32
}

// LinkEvent represents a link event
type LinkEvent struct {
	SyscallEvent
	Source FileEvent `field:"source"`
	Target FileEvent `field:"target"`
}

// MkdirEvent represents a mkdir event
type MkdirEvent struct {
	SyscallEvent
	FileEvent
	Mode uint32 `field:"mode"`
}

// MountEvent represents a mount event
type MountEvent struct {
	SyscallEvent
	MountID                       uint32
	GroupID                       uint32
	Device                        uint32
	ParentMountID                 uint32
	ParentInode                   uint64
	FSType                        string
	MountPointStr                 string
	MountPointPathResolutionError error
	RootMountID                   uint32
	RootInode                     uint64
	RootStr                       string
	RootPathResolutionError       error

	FSTypeRaw [16]byte
}

// GetFSType returns the filesystem type of the mountpoint
func (m *MountEvent) GetFSType() string {
	if len(m.FSType) == 0 {
		m.FSType = string(bytes.Trim(m.FSTypeRaw[:], "\x00"))
	}
	return m.FSType
}

// IsOverlayFS returns whether it is an overlay fs
func (m *MountEvent) IsOverlayFS() bool {
	return m.GetFSType() == "overlay"
}

// GetRootPathResolutionError returns the root path resolution error as a string if there is one
func (m *MountEvent) GetRootPathResolutionError() string {
	if m.RootPathResolutionError != nil {
		return m.RootPathResolutionError.Error()
	}
	return ""
}

// GetMountPointPathResolutionError returns the mount point path resolution error as a string if there is one
func (m *MountEvent) GetMountPointPathResolutionError() string {
	if m.MountPointPathResolutionError != nil {
		return m.MountPointPathResolutionError.Error()
	}
	return ""
}

// OpenEvent represents an open event
type OpenEvent struct {
	SyscallEvent
	FileEvent
	Flags uint32 `field:"flags"`
	Mode  uint32 `field:"mode"`
}

// ProcessCacheEntry this structure holds the container context that we keep in kernel for each process
type ProcessCacheEntry struct {
	ContainerContext
	ProcessContext
}

// ProcessAncestorsIterator defines an iterator of ancestors
type ProcessAncestorsIterator struct {
	prev *ProcessCacheEntry
}

// Front returns the first element
func (it *ProcessAncestorsIterator) Front(ctx *eval.Context) unsafe.Pointer {
	if front := (*Event)(ctx.Object).Process.Ancestor; front != nil {
		it.prev = front
		return unsafe.Pointer(front)
	}

	return nil
}

// Next returns the next element
func (it *ProcessAncestorsIterator) Next() unsafe.Pointer {
	if next := it.prev.Ancestor; next != nil {
		it.prev = next
		return unsafe.Pointer(next)
	}

	return nil
}

// ProcessContext holds the process context of an event
type ProcessContext struct {
	ExecEvent

	Pid uint32 `field:"pid"`
	Tid uint32 `field:"tid"`
	UID uint32 `field:"uid"`
	GID uint32 `field:"gid"`

	Ancestor *ProcessCacheEntry `field:"ancestors" iterator:"ProcessAncestorsIterator"`
}

// RenameEvent represents a rename event
type RenameEvent struct {
	SyscallEvent
	Old               FileEvent `field:"old"`
	New               FileEvent `field:"new"`
	DiscarderRevision uint32    `field:"-"`
}

// RmdirEvent represents a rmdir event
type RmdirEvent struct {
	SyscallEvent
	FileEvent
	DiscarderRevision uint32 `field:"-"`
}

// SetXAttrEvent represents an extended attributes event
type SetXAttrEvent struct {
	SyscallEvent
	FileEvent
	Namespace string `field:"namespace" handler:"GetXAttrNamespace,string"`
	Name      string `field:"name" handler:"GetXAttrName,string"`

	NameRaw [200]byte
}

// SyscallEvent contains common fields for all the event
type SyscallEvent struct {
	Retval int64 `field:"retval"`
}

// UnlinkEvent represents an unlink event
type UnlinkEvent struct {
	SyscallEvent
	FileEvent
	Flags             uint32 `field:"flags"`
	DiscarderRevision uint32 `field:"-"`
}

// UmountEvent represents an umount event
type UmountEvent struct {
	SyscallEvent
	MountID           uint32
	DiscarderRevision uint32 `field:"-"`
}

// UtimesEvent represents a utime event
type UtimesEvent struct {
	SyscallEvent
	FileEvent
	Atime time.Time
	Mtime time.Time
}
