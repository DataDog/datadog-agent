// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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
func (m *Model) ValidateField(field eval.Field, fieldValue eval.FieldValue) error {
	// check that all path are absolute
	if strings.HasSuffix(field, "path") {
		if fieldValue.Type == eval.RegexpValueType {
			return fmt.Errorf("regexp not supported on path `%s`", field)
		}

		if value, ok := fieldValue.Value.(string); ok {
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

	switch field {

	case "event.retval":
		if value := fieldValue.Value; value != -int(syscall.EPERM) && value != -int(syscall.EACCES) {
			return errors.New("return value can only be tested against EPERM or EACCES")
		}
	}

	return nil
}

// ChmodEvent represents a chmod event
type ChmodEvent struct {
	SyscallEvent
	File FileEvent `field:"file"`
	Mode uint32    `field:"file.destination.mode" field:"file.destination.rights"`
}

// ChownEvent represents a chown event
type ChownEvent struct {
	SyscallEvent
	File  FileEvent `field:"file"`
	UID   uint32    `field:"file.destination.uid"`
	User  string    `field:"file.destination.user,ResolveChownUID"`
	GID   uint32    `field:"file.destination.gid"`
	Group string    `field:"file.destination.group,ResolveChownGID"`
}

// ContainerContext holds the container context of an event
type ContainerContext struct {
	ID string `field:"id,ResolveContainerID"`
}

// Event represents an event sent from the kernel
// genaccessors
type Event struct {
	ID           string    `field:"-"`
	Type         uint64    `field:"-"`
	TimestampRaw uint64    `field:"-"`
	Timestamp    time.Time `field:"timestamp"`

	ProcessContext   ProcessContext   `field:"process" event:"*"`
	ContainerContext ContainerContext `field:"container"`

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

	SetUID SetuidEvent `field:"setuid" event:"setuid"`
	SetGID SetgidEvent `field:"setgid" event:"setgid"`
	Capset CapsetEvent `field:"capset" event:"capset"`

	Mount            MountEvent            `field:"-"`
	Umount           UmountEvent           `field:"-"`
	InvalidateDentry InvalidateDentryEvent `field:"-"`
	ArgsEnvs         ArgsEnvsEvent         `field:"-"`
	MountReleased    MountReleasedEvent    `field:"-"`
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

// SetuidEvent represents a setuid event
type SetuidEvent struct {
	UID    uint32 `field:"uid"`
	User   string `field:"user,ResolveSetuidUser"`
	EUID   uint32 `field:"euid"`
	EUser  string `field:"euser,ResolveSetuidEUser"`
	FSUID  uint32 `field:"fsuid"`
	FSUser string `field:"fsuser,ResolveSetuidFSUser"`
}

// SetgidEvent represents a setgid event
type SetgidEvent struct {
	GID     uint32 `field:"gid"`
	Group   string `field:"group,ResolveSetgidGroup"`
	EGID    uint32 `field:"egid"`
	EGroup  string `field:"egroup,ResolveSetgidEGroup"`
	FSGID   uint32 `field:"fsgid"`
	FSGroup string `field:"fsgroup,ResolveSetgidFSGroup"`
}

// CapsetEvent represents a capset event
type CapsetEvent struct {
	CapEffective uint64 `field:"cap_effective"`
	CapPermitted uint64 `field:"cap_permitted"`
}

// Credentials represents the kernel credentials of a process
type Credentials struct {
	UID   uint32 `field:"uid,ResolveCredentialsUID"`
	GID   uint32 `field:"gid,ResolveCredentialsGID"`
	User  string `field:"user,ResolveCredentialsUser"`
	Group string `field:"group,ResolveCredentialsGroup"`

	EUID   uint32 `field:"euid,ResolveCredentialsEUID"`
	EGID   uint32 `field:"egid,ResolveCredentialsEGID"`
	EUser  string `field:"euser,ResolveCredentialsEUser"`
	EGroup string `field:"egroup,ResolveCredentialsEGroup"`

	FSUID   uint32 `field:"fsuid,ResolveCredentialsFSUID"`
	FSGID   uint32 `field:"fsgid,ResolveCredentialsFSGID"`
	FSUser  string `field:"fsuser,ResolveCredentialsFSUser"`
	FSGroup string `field:"fsgroup,ResolveCredentialsFSGroup"`

	CapEffective uint64 `field:"cap_effective,ResolveCredentialsCapEffective"`
	CapPermitted uint64 `field:"cap_permitted,ResolveCredentialsCapPermitted"`
}

// GetPathResolutionError returns the path resolution error as a string if there is one
func (e *Process) GetPathResolutionError() string {
	if e.PathResolutionError != nil {
		return e.PathResolutionError.Error()
	}
	return ""
}

// Process represents a process
type Process struct {
	// proc_cache_t
	// (container context is parsed in Event.Container)
	FileFields FileFields `field:"file"`

	PathnameStr         string `field:"file.path,ResolveProcessInode"`
	ContainerPath       string `field:"file.container_path,ResolveProcessContainerPath"`
	BasenameStr         string `field:"file.name,ResolveProcessBasename"`
	Filesystem          string `field:"file.filesystem,ResolveProcessFilesystem"`
	PathResolutionError error  `field:"-"`

	ExecTimestamp uint64    `field:"-"`
	ExecTime      time.Time `field:"-"`

	TTYName string `field:"tty_name,ResolveProcessTTY"`
	Comm    string `field:"comm,ResolveProcessComm"`

	// pid_cache_t
	ForkTimestamp uint64    `field:"-"`
	ForkTime      time.Time `field:"-"`

	ExitTimestamp uint64    `field:"-"`
	ExitTime      time.Time `field:"-"`

	Cookie uint32 `field:"cookie,ResolveProcessCookie"`
	PPid   uint32 `field:"ppid,ResolveProcessPPID"`

	// credentials_t section of pid_cache_t
	Credentials

	ArgsArray     []string `field:"-"`
	ArgsTruncated bool     `field:"-"`
	EnvsArray     []string `field:"-"`
	EnvsTruncated bool     `field:"-"`

	ArgsID uint32 `field:"-"`
	EnvsID uint32 `field:"-"`
}

// ExecEvent represents a exec event
type ExecEvent struct {
	Process

	Args          string   `field:"args,ResolveExecArgs"`
	ArgsTruncated bool     `field:"args_truncated"`
	Envs          []string `field:"envs,ResolveExecEnvs"`
	EnvsTruncated bool     `field:"envs_truncated"`
}

// FileFields holds the information required to identify a file
type FileFields struct {
	UID   uint32    `field:"uid"`
	User  string    `field:"user,ResolveUser"`
	GID   uint32    `field:"gid"`
	Group string    `field:"group,ResolveGroup"`
	Mode  uint16    `field:"mode" field:"rights,ResolveRights"`
	CTime time.Time `field:"-"`
	MTime time.Time `field:"-"`

	MountID uint32 `field:"mount_id"`
	Inode   uint64 `field:"inode"`
	PathID  uint32 `field:"-"`
	Flags   int32  `field:"-"`
}

// GetInLowerLayer returns whether a file is in a lower layer
func (f *FileFields) GetInLowerLayer() bool {
	return f.Flags&LowerLayer != 0
}

// GetInUpperLayer returns whether a file is in the upper layer
func (f *FileFields) GetInUpperLayer() bool {
	return f.Flags&UpperLayer != 0
}

// FileEvent is the common file event type
type FileEvent struct {
	FileFields
	PathnameStr   string `field:"path,ResolveFileInode"`
	ContainerPath string `field:"container_path,ResolveFileContainerPath"`
	BasenameStr   string `field:"name,ResolveFileBasename"`
	Filesytem     string `field:"filesystem,ResolveFileFilesystem"`
	InUpperLayer  bool   `field:"in_upper_layer,ResolveFileInUpperLayer"`

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

// MountReleasedEvent defines a mount released event
type MountReleasedEvent struct {
	MountID           uint32
	DiscarderRevision uint32
}

// LinkEvent represents a link event
type LinkEvent struct {
	SyscallEvent
	Source FileEvent `field:"file"`
	Target FileEvent `field:"file.destination"`
}

// MkdirEvent represents a mkdir event
type MkdirEvent struct {
	SyscallEvent
	File FileEvent `field:"file"`
	Mode uint32    `field:"file.destination.mode" field:"file.destination.rights"`
}

// ArgsEnvsEvent defines a args/envs event
type ArgsEnvsEvent struct {
	ID          uint32
	Size        uint32
	Values      []string
	ValuesRaw   [128]byte
	IsTruncated bool
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
	File  FileEvent `field:"file"`
	Flags uint32    `field:"flags"`
	Mode  uint32    `field:"file.destination.mode"`
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
	if front := (*Event)(ctx.Object).ProcessContext.Ancestor; front != nil {
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
	Process

	Pid uint32 `field:"pid"`
	Tid uint32 `field:"tid"`

	Ancestor *ProcessCacheEntry `field:"ancestors,,ProcessAncestorsIterator"`
}

// RenameEvent represents a rename event
type RenameEvent struct {
	SyscallEvent
	Old               FileEvent `field:"file"`
	New               FileEvent `field:"file.destination"`
	DiscarderRevision uint32    `field:"-"`
}

// RmdirEvent represents a rmdir event
type RmdirEvent struct {
	SyscallEvent
	File              FileEvent `field:"file"`
	DiscarderRevision uint32    `field:"-"`
}

// SetXAttrEvent represents an extended attributes event
type SetXAttrEvent struct {
	SyscallEvent
	File      FileEvent `field:"file"`
	Namespace string    `field:"file.destination.namespace,GetXAttrNamespace"`
	Name      string    `field:"file.destination.name,GetXAttrName"`

	NameRaw [200]byte
}

// SyscallEvent contains common fields for all the event
type SyscallEvent struct {
	Retval int64 `field:"retval"`
}

// UnlinkEvent represents an unlink event
type UnlinkEvent struct {
	SyscallEvent
	File              FileEvent `field:"file"`
	Flags             uint32    `field:"-"`
	DiscarderRevision uint32    `field:"-"`
}

// UmountEvent represents an umount event
type UmountEvent struct {
	SyscallEvent
	MountID uint32
}

// UtimesEvent represents a utime event
type UtimesEvent struct {
	SyscallEvent
	File  FileEvent `field:"file"`
	Atime time.Time `field:"-"`
	Mtime time.Time `field:"-"`
}
