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
func (m *Model) ValidateField(key string, field eval.FieldValue) error {
	// check that all path are absolute
	if strings.HasSuffix(key, "path") {
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
	File FileEvent `field:"file"`
	Mode uint32    `field:"file.destination.mode"`
}

// ChownEvent represents a chown event
type ChownEvent struct {
	SyscallEvent
	File  FileEvent `field:"file"`
	UID   uint32    `field:"file.destination.uid"`
	User  string    `field:"file.destination.user" handler:"ResolveChownUID,string"`
	GID   uint32    `field:"file.destination.gid"`
	Group string    `field:"file.destination.group" handler:"ResolveChownGID,string"`
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
	User   string `field:"user" handler:"ResolveSetuidUser,string"`
	EUID   uint32 `field:"euid"`
	EUser  string `field:"euser" handler:"ResolveSetuidEUser,string"`
	FSUID  uint32 `field:"fsuid"`
	FSUser string `field:"fsuser" handler:"ResolveSetuidFSUser,string"`
}

// SetgidEvent represents a setgid event
type SetgidEvent struct {
	GID     uint32 `field:"gid"`
	Group   string `field:"group" handler:"ResolveSetgidGroup,string"`
	EGID    uint32 `field:"egid"`
	EGroup  string `field:"egroup" handler:"ResolveSetgidEGroup,string"`
	FSGID   uint32 `field:"fsgid"`
	FSGroup string `field:"fsgroup" handler:"ResolveSetgidFSGroup,string"`
}

// CapsetEvent represents a capset event
type CapsetEvent struct {
	CapEffective uint64 `field:"cap_effective"`
	CapPermitted uint64 `field:"cap_permitted"`
}

// Credentials represents the kernel credentials of a process
type Credentials struct {
	UID   uint32 `field:"uid" handler:"ResolveCredentialsUID,int"`
	GID   uint32 `field:"gid" handler:"ResolveCredentialsGID,int"`
	User  string `field:"user" handler:"ResolveCredentialsUser,string"`
	Group string `field:"group" handler:"ResolveCredentialsGroup,string"`

	EUID   uint32 `field:"euid" handler:"ResolveCredentialsEUID,int"`
	EGID   uint32 `field:"egid" handler:"ResolveCredentialsEGID,int"`
	EUser  string `field:"euser" handler:"ResolveCredentialsEUser,string"`
	EGroup string `field:"egroup" handler:"ResolveCredentialsEGroup,string"`

	FSUID   uint32 `field:"fsuid" handler:"ResolveCredentialsFSUID,int"`
	FSGID   uint32 `field:"fsgid" handler:"ResolveCredentialsFSGID,int"`
	FSUser  string `field:"fsuser" handler:"ResolveCredentialsFSUser,string"`
	FSGroup string `field:"fsgroup" handler:"ResolveCredentialsFSGroup,string"`

	CapEffective uint64 `field:"cap_effective" handler:"ResolveCredentialsCapEffective,int"`
	CapPermitted uint64 `field:"cap_permitted" handler:"ResolveCredentialsCapPermitted,int"`
}

// ExecArgsIterator represents an exec args iterator
type ExecArgsIterator struct {
	args  []string
	index int
}

// Front returns the first arg
func (e *ExecArgsIterator) Front(ctx *eval.Context) unsafe.Pointer {
	e.args = (*Event)(ctx.Object).ProcessContext.Args
	if len(e.args) > 0 {
		e.index = 1
		return unsafe.Pointer(&e.args[0])
	}
	return nil
}

// Next returns the next arg
func (e *ExecArgsIterator) Next() unsafe.Pointer {
	if e.index < len(e.args) {
		value := e.args[e.index]
		e.index++
		return unsafe.Pointer(&value)
	}

	return nil
}

// ExecEnvsIterator represents an exec envs iterator
type ExecEnvsIterator struct {
	envs  []string
	index int
}

// Front returns the first env variable
func (e *ExecEnvsIterator) Front(ctx *eval.Context) unsafe.Pointer {
	e.envs = (*Event)(ctx.Object).ProcessContext.Envs
	if len(e.envs) > 0 {
		e.index = 1
		return unsafe.Pointer(&e.envs[0])
	}
	return nil
}

// Next returns the next env variable
func (e *ExecEnvsIterator) Next() unsafe.Pointer {
	if e.index < len(e.envs) {
		value := e.envs[e.index]
		e.index++
		return unsafe.Pointer(&value)
	}

	return nil
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

	PathnameStr         string `field:"file.path" handler:"ResolveProcessInode,string"`
	ContainerPath       string `field:"file.container_path" handler:"ResolveProcessContainerPath,string"`
	BasenameStr         string `field:"file.name" handler:"ResolveProcessBasename,string"`
	Filesystem          string `field:"file.filesystem" handler:"ResolveProcessFilesystem,string"`
	PathResolutionError error  `field:"-"`

	ExecTimestamp uint64    `field:"-"`
	ExecTime      time.Time `field:"-"`

	TTYName string `field:"tty_name" handler:"ResolveProcessTTY,string"`
	Comm    string `field:"comm" handler:"ResolveProcessComm,string"`

	// pid_cache_t
	ForkTimestamp uint64    `field:"-"`
	ForkTime      time.Time `field:"-"`

	ExitTimestamp uint64    `field:"-"`
	ExitTime      time.Time `field:"-"`

	Cookie uint32 `field:"cookie" handler:"ResolveProcessCookie,int"`
	PPid   uint32 `field:"ppid" handler:"ResolveProcessPPID,int"`

	// credentials_t section of pid_cache_t
	Credentials

	Args          []string `field:"-"`
	ArgsTruncated bool     `field:"-"`
	Envs          []string `field:"-"`
	EnvsTruncated bool     `field:"-"`

	ArgsID uint32 `field:"-"`
	EnvsID uint32 `field:"-"`
}

// ExecEvent represents a exec event
type ExecEvent struct {
	Process

	// override Process fields so that SECL can expose them
	Args          []string `field:"args" iterator:"ExecArgsIterator"`
	ArgsTruncated bool     `field:"args_truncated"`
	Envs          []string `field:"envs" iterator:"ExecEnvsIterator"`
	EnvsTruncated bool     `field:"envs_truncated"`
}

// FileFields holds the information required to identify a file
type FileFields struct {
	UID   uint32    `field:"uid"`
	User  string    `field:"user" handler:"ResolveUser,string"`
	GID   uint32    `field:"gid"`
	Group string    `field:"group" handler:"ResolveGroup,string"`
	Mode  uint16    `field:"mode"`
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
	PathnameStr   string `field:"path" handler:"ResolveFileInode,string"`
	ContainerPath string `field:"container_path" handler:"ResolveFileContainerPath,string"`
	BasenameStr   string `field:"name" handler:"ResolveFileBasename,string"`
	Filesytem     string `field:"filesystem" handler:"ResolveFileFilesystem,string"`
	InUpperLayer  bool   `field:"in_upper_layer" handler:"ResolveFileInUpperLayer,bool"`

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
	Source FileEvent `field:"file"`
	Target FileEvent `field:"file.destination"`
}

// MkdirEvent represents a mkdir event
type MkdirEvent struct {
	SyscallEvent
	File FileEvent `field:"file"`
	Mode uint32    `field:"file.destination.mode"`
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

	Ancestor *ProcessCacheEntry `field:"ancestors" iterator:"ProcessAncestorsIterator"`
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
	Namespace string    `field:"file.destination.namespace" handler:"GetXAttrNamespace,string"`
	Name      string    `field:"file.destination.name" handler:"GetXAttrName,string"`

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
	MountID           uint32
	DiscarderRevision uint32 `field:"-"`
}

// UtimesEvent represents a utime event
type UtimesEvent struct {
	SyscallEvent
	File  FileEvent `field:"file"`
	Atime time.Time `field:"-"`
	Mtime time.Time `field:"-"`
}
