// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

//go:generate go run github.com/DataDog/datadog-agent/pkg/security/secl/generators/accessors -mock -tags linux -output accessors.go
//go:generate go run github.com/DataDog/datadog-agent/pkg/security/secl/generators/accessors -tags linux -output ../probe/accessors.go
//go:generate go run github.com/DataDog/datadog-agent/pkg/security/secl/generators/accessors -doc -output secl_documentation.json

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
	ID   string   `field:"id,ResolveContainerID"`
	Tags []string `field:"tags,ResolveContainerTags:9999"`
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

	Chmod       ChmodEvent    `field:"chmod" event:"chmod"` // [7.27] chmod event definition
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

	SELinux SELinuxEvent `field:"selinux" event:"selinux"`

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
	tags := []string{"type:" + e.GetType()}

	// should already be resolved at this stage
	if len(e.ContainerContext.Tags) > 0 {
		tags = append(tags, e.ContainerContext.Tags...)
	}
	return tags
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
	UID   uint32 `field:"uid"`
	GID   uint32 `field:"gid"`
	User  string `field:"user"`
	Group string `field:"group"`

	EUID   uint32 `field:"euid"`
	EGID   uint32 `field:"egid"`
	EUser  string `field:"euser"`
	EGroup string `field:"egroup"`

	FSUID   uint32 `field:"fsuid"`
	FSGID   uint32 `field:"fsgid"`
	FSUser  string `field:"fsuser"`
	FSGroup string `field:"fsgroup"`

	CapEffective uint64 `field:"cap_effective"`
	CapPermitted uint64 `field:"cap_permitted"`
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
	FileFields FileFields `field:"file"`

	Pid uint32 `field:"pid"`
	Tid uint32 `field:"tid"`

	PathnameStr         string `field:"file.path"`
	BasenameStr         string `field:"file.name"`
	Filesystem          string `field:"file.filesystem"`
	PathResolutionError error  `field:"-"`

	ContainerID string `field:"container.id"`

	TTYName string `field:"tty_name"`
	Comm    string `field:"comm"`

	// pid_cache_t
	ForkTime time.Time `field:"-"`
	ExitTime time.Time `field:"-"`
	ExecTime time.Time `field:"-"`

	CreatedAt uint64 `field:"created_at,ResolveProcessCreatedAt"`

	Cookie uint32 `field:"cookie"`
	PPid   uint32 `field:"ppid"`

	// credentials_t section of pid_cache_t
	Credentials

	ArgsID uint32 `field:"-"`
	EnvsID uint32 `field:"-"`

	ArgsEntry     *ArgsEntry `field:"-"`
	EnvsEntry     *EnvsEntry `field:"-"`
	EnvsTruncated bool       `field:"-"`
	ArgsTruncated bool       `field:"-"`
}

// ExecEvent represents a exec event
type ExecEvent struct {
	Process

	// defined to generate accessors
	Args          string   `field:"args,ResolveExecArgs"`
	Argv          []string `field:"argv,ResolveExecArgv" field:"args_flags,ResolveExecArgsFlags" field:"args_options,ResolveExecArgsOptions"`
	ArgsTruncated bool     `field:"args_truncated,ResolveExecArgsTruncated"`
	Envs          []string `field:"envs,ResolveExecEnvs"`
	EnvsTruncated bool     `field:"envs_truncated,ResolveExecEnvsTruncated"`
}

// FileFields holds the information required to identify a file
type FileFields struct {
	UID   uint32 `field:"uid"`
	User  string `field:"user,ResolveFileFieldsUser"`
	GID   uint32 `field:"gid"`
	Group string `field:"group,ResolveFileFieldsGroup"`
	Mode  uint16 `field:"mode" field:"rights,ResolveRights"`
	CTime uint64 `field:"change_time"`
	MTime uint64 `field:"modification_time"`

	MountID      uint32 `field:"mount_id"`
	Inode        uint64 `field:"inode"`
	PathID       uint32 `field:"-"`
	Flags        int32  `field:"-"`
	InUpperLayer bool   `field:"in_upper_layer,ResolveFileFieldsInUpperLayer"`
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
	PathnameStr string `field:"path,ResolveFilePath"`
	BasenameStr string `field:"name,ResolveFileBasename"`
	Filesytem   string `field:"filesystem,ResolveFileFilesystem"`

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
	ArgsEnvs
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

// SELinuxEventKind represents the event kind for SELinux events
type SELinuxEventKind uint32

const (
	// SELinuxBoolChangeEventKind represents SELinux boolean change events
	SELinuxBoolChangeEventKind SELinuxEventKind = iota
	// SELinuxStatusChangeEventKind represents SELinux status change events
	SELinuxStatusChangeEventKind
	// SELinuxBoolCommitEventKind represents SELinux boolean commit events
	SELinuxBoolCommitEventKind
)

// SELinuxEvent represents a selinux event
type SELinuxEvent struct {
	File            FileEvent        `field:"-"`
	EventKind       SELinuxEventKind `field:"-"`
	BoolName        string           `field:"bool.name,ResolveSELinuxBoolName"`
	BoolChangeValue string           `field:"bool.state"`
	BoolCommitValue bool             `field:"bool_commit.state"`
	EnforceStatus   string           `field:"enforce.status"`
}

var zeroProcessContext ProcessContext

// ProcessCacheEntry this struct holds process context kept in the process tree
type ProcessCacheEntry struct {
	ProcessContext

	refCount  uint64                     `field:"-"`
	onRelease func(_ *ProcessCacheEntry) `field:"-"`
}

// Reset the entry
func (e *ProcessCacheEntry) Reset() {
	e.ProcessContext = zeroProcessContext
	e.refCount = 0
}

// Retain increment ref counter
func (e *ProcessCacheEntry) Retain() {
	e.refCount++
}

// Release decrement and eventually release the entry
func (e *ProcessCacheEntry) Release() {
	e.refCount--
	if e.refCount > 0 {
		return
	}

	if e.onRelease != nil {
		e.onRelease(e)
	}
}

// NewProcessCacheEntry returns a new process cache entry
func NewProcessCacheEntry(onRelease func(_ *ProcessCacheEntry)) *ProcessCacheEntry {
	return &ProcessCacheEntry{
		onRelease: onRelease,
	}
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
	Namespace string    `field:"file.destination.namespace,ResolveXAttrNamespace"`
	Name      string    `field:"file.destination.name,ResolveXAttrName"`

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
