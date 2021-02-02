// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

//go:generate go run github.com/DataDog/datadog-agent/pkg/security/secl/generators/accessors -tags linux -output model_accessors.go

package probe

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

var (
	dentryInvalidDiscarder = []interface{}{dentryPathKeyNotFound}
)

// InvalidDiscarders exposes list of values that are not discarders
var InvalidDiscarders = map[eval.Field][]interface{}{
	"open.filename":        dentryInvalidDiscarder,
	"unlink.filename":      dentryInvalidDiscarder,
	"chmod.filename":       dentryInvalidDiscarder,
	"chown.filename":       dentryInvalidDiscarder,
	"mkdir.filename":       dentryInvalidDiscarder,
	"rmdir.filename":       dentryInvalidDiscarder,
	"rename.old.filename":  dentryInvalidDiscarder,
	"rename.new.filename":  dentryInvalidDiscarder,
	"utimes.filename":      dentryInvalidDiscarder,
	"link.source.filename": dentryInvalidDiscarder,
	"link.target.filename": dentryInvalidDiscarder,
	"process.filename":     dentryInvalidDiscarder,
	"setxattr.filename":    dentryInvalidDiscarder,
	"removexattr.filename": dentryInvalidDiscarder,
}

// ErrNotEnoughData is returned when the buffer is too small to unmarshal the event
var ErrNotEnoughData = errors.New("not enough data")

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

// SyscallEvent contains common fields for all the event
type SyscallEvent struct {
	Retval int64 `field:"retval"`
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *SyscallEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 8 {
		return 0, ErrNotEnoughData
	}
	e.Retval = int64(ebpf.ByteOrder.Uint64(data[0:8]))
	return 8, nil
}

// BinaryUnmarshaler interface implemented by every event type
type BinaryUnmarshaler interface {
	UnmarshalBinary(data []byte) (int, error)
}

// FileEvent is the common file event type
type FileEvent struct {
	MountID         uint32 `field:"-"`
	Inode           uint64 `field:"inode"`
	PathID          uint32 `field:"-"`
	OverlayNumLower int32  `field:"overlay_numlower"`
	PathnameStr     string `field:"filename" handler:"ResolveInode,string"`
	ContainerPath   string `field:"container_path" handler:"ResolveContainerPath,string"`
	BasenameStr     string `field:"basename" handler:"ResolveBasename,string"`

	pathResolutionError error `field:"-"`
}

// GetPathResolutionError returns the path resolution error as a string if there is one
func (e *FileEvent) GetPathResolutionError() string {
	if e.pathResolutionError != nil {
		return e.pathResolutionError.Error()
	}
	return ""
}

// ResolveInode resolves the inode to a full path
func (e *FileEvent) ResolveInode(event *Event) string {
	path, err := e.resolveInode(event.resolvers)
	if err != nil {
		if _, ok := err.(ErrTruncatedSegment); ok {
			event.SetPathResolutionError(err)
		} else if _, ok := err.(ErrTruncatedParents); ok {
			event.SetPathResolutionError(err)
		}
	}
	return path
}

// ResolveInodeWithResolvers resolves the inode to a full path. Returns the path and true if it was entirely resolved
func (e *FileEvent) ResolveInodeWithResolvers(resolvers *Resolvers) string {
	path, _ := e.resolveInode(resolvers)
	return path
}

// resolveInode resolves the inode to a full path. Returns the path and true if it was entirely resolved
func (e *FileEvent) resolveInode(resolvers *Resolvers) (string, error) {
	if len(e.PathnameStr) == 0 {
		e.PathnameStr, e.pathResolutionError = resolvers.DentryResolver.Resolve(e.MountID, e.Inode, e.PathID)
		if e.PathnameStr == dentryPathKeyNotFound {
			return e.PathnameStr, e.pathResolutionError
		}

		_, mountPath, rootPath, err := resolvers.MountResolver.GetMountPath(e.MountID)
		if err == nil {
			if strings.HasPrefix(e.PathnameStr, rootPath) && rootPath != "/" {
				e.PathnameStr = strings.Replace(e.PathnameStr, rootPath, "", 1)
			}
			e.PathnameStr = path.Join(mountPath, e.PathnameStr)
		}
	}

	return e.PathnameStr, e.pathResolutionError
}

// ResolveContainerPath resolves the inode to a path relative to the container
func (e *FileEvent) ResolveContainerPath(event *Event) string {
	return e.ResolveContainerPathWithResolvers(event.resolvers)
}

// ResolveContainerPathWithResolvers resolves the inode to a path relative to the container
func (e *FileEvent) ResolveContainerPathWithResolvers(resolvers *Resolvers) string {
	if len(e.ContainerPath) == 0 {
		containerPath, _, _, err := resolvers.MountResolver.GetMountPath(e.MountID)
		if err == nil {
			e.ContainerPath = containerPath
		}
		if len(containerPath) == 0 && len(e.PathnameStr) == 0 {
			// The container path might be included in the pathname. The container path will be set there.
			_ = e.ResolveInodeWithResolvers(resolvers)
		}
	}
	return e.ContainerPath
}

// ResolveBasename resolves the inode to a filename
func (e *FileEvent) ResolveBasename(event *Event) string {
	if len(e.BasenameStr) == 0 {
		if e.PathnameStr != "" {
			e.BasenameStr = path.Base(e.PathnameStr)
		} else {
			e.BasenameStr = event.resolvers.DentryResolver.GetName(e.MountID, e.Inode, e.PathID)
		}
	}
	return e.BasenameStr
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *FileEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 24 {
		return 0, ErrNotEnoughData
	}
	e.Inode = ebpf.ByteOrder.Uint64(data[0:8])
	e.MountID = ebpf.ByteOrder.Uint32(data[8:12])
	e.OverlayNumLower = int32(ebpf.ByteOrder.Uint32(data[12:16]))
	e.PathID = ebpf.ByteOrder.Uint32(data[16:20])

	return 24, nil
}

func unmarshalBinary(data []byte, binaryUnmarshalers ...BinaryUnmarshaler) (int, error) {
	read := 0
	for _, marshaler := range binaryUnmarshalers {
		n, err := marshaler.UnmarshalBinary(data[read:])
		read += n
		if err != nil {
			return read, err
		}
	}
	return read, nil
}

// Bytes returns a binary representation of itself
func (e *FileEvent) Bytes() []byte {
	b := make([]byte, 16)
	ebpf.ByteOrder.PutUint64(b[0:8], e.Inode)
	ebpf.ByteOrder.PutUint32(b[8:12], e.MountID)
	ebpf.ByteOrder.PutUint32(b[12:16], uint32(e.OverlayNumLower))
	ebpf.ByteOrder.PutUint32(b[16:20], e.PathID)
	return b
}

// ChmodEvent represents a chmod event
type ChmodEvent struct {
	SyscallEvent
	FileEvent
	Mode uint32 `field:"mode"`
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *ChmodEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := unmarshalBinary(data, &e.SyscallEvent, &e.FileEvent)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 4 {
		return n, ErrNotEnoughData
	}

	e.Mode = ebpf.ByteOrder.Uint32(data[0:4])
	return n + 4, nil
}

// ChownEvent represents a chown event
type ChownEvent struct {
	SyscallEvent
	FileEvent
	UID int32 `field:"uid"`
	GID int32 `field:"gid"`
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *ChownEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := unmarshalBinary(data, &e.SyscallEvent, &e.FileEvent)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 8 {
		return n, ErrNotEnoughData
	}

	e.UID = int32(ebpf.ByteOrder.Uint32(data[0:4]))
	e.GID = int32(ebpf.ByteOrder.Uint32(data[4:8]))
	return n + 8, nil
}

// SetXAttrEvent represents an extended attributes event
type SetXAttrEvent struct {
	SyscallEvent
	FileEvent
	Namespace string `field:"namespace" handler:"GetNamespace,string"`
	Name      string `field:"name" handler:"GetName,string"`

	NameRaw [200]byte
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *SetXAttrEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := unmarshalBinary(data, &e.SyscallEvent, &e.FileEvent)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 200 {
		return n, ErrNotEnoughData
	}
	utils.SliceToArray(data[0:200], unsafe.Pointer(&e.NameRaw))

	return n + 200, nil
}

// GetName returns the string representation of the extended attribute name
func (e *SetXAttrEvent) GetName(event *Event) string {
	if len(e.Name) == 0 {
		e.Name = string(bytes.Trim(e.NameRaw[:], "\x00"))
	}
	return e.Name
}

// GetNamespace returns the string representation of the extended attribute namespace
func (e *SetXAttrEvent) GetNamespace(event *Event) string {
	if len(e.Namespace) == 0 {
		fragments := strings.Split(e.GetName(event), ".")
		if len(fragments) > 0 {
			e.Namespace = fragments[0]
		}
	}
	return e.Namespace
}

// OpenEvent represents an open event
type OpenEvent struct {
	SyscallEvent
	FileEvent
	Flags uint32 `field:"flags"`
	Mode  uint32 `field:"mode"`
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *OpenEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := unmarshalBinary(data, &e.SyscallEvent, &e.FileEvent)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 8 {
		return n, ErrNotEnoughData
	}

	e.Flags = ebpf.ByteOrder.Uint32(data[0:4])
	e.Mode = ebpf.ByteOrder.Uint32(data[4:8])
	return n + 8, nil
}

// MkdirEvent represents a mkdir event
type MkdirEvent struct {
	SyscallEvent
	FileEvent
	Mode uint32 `field:"mode"`
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *MkdirEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := unmarshalBinary(data, &e.SyscallEvent, &e.FileEvent)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 4 {
		return n, ErrNotEnoughData
	}

	e.Mode = ebpf.ByteOrder.Uint32(data[0:4])
	return n + 4, nil
}

// RmdirEvent represents a rmdir event
type RmdirEvent struct {
	SyscallEvent
	FileEvent
	DiscarderRevision uint32 `field:"-"`
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *RmdirEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := unmarshalBinary(data, &e.SyscallEvent, &e.FileEvent)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 8 {
		return 0, ErrNotEnoughData
	}

	e.DiscarderRevision = ebpf.ByteOrder.Uint32(data[0:4])
	// padding

	return n + 8, nil
}

// UnlinkEvent represents an unlink event
type UnlinkEvent struct {
	SyscallEvent
	FileEvent
	Flags             uint32 `field:"flags"`
	DiscarderRevision uint32 `field:"-"`
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *UnlinkEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := unmarshalBinary(data, &e.SyscallEvent, &e.FileEvent)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 8 {
		return 0, ErrNotEnoughData
	}

	e.Flags = ebpf.ByteOrder.Uint32(data[0:4])
	e.DiscarderRevision = ebpf.ByteOrder.Uint32(data[4:8])

	return n + 8, nil
}

// RenameEvent represents a rename event
type RenameEvent struct {
	SyscallEvent
	Old               FileEvent `field:"old"`
	New               FileEvent `field:"new"`
	DiscarderRevision uint32    `field:"-"`
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *RenameEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := unmarshalBinary(data, &e.SyscallEvent, &e.Old, &e.New)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 8 {
		return 0, ErrNotEnoughData
	}

	e.DiscarderRevision = ebpf.ByteOrder.Uint32(data[0:4])
	// padding

	return n + 8, nil
}

// UtimesEvent represents a utime event
type UtimesEvent struct {
	SyscallEvent
	FileEvent
	Atime time.Time
	Mtime time.Time
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *UtimesEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := unmarshalBinary(data, &e.SyscallEvent, &e.FileEvent)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 32 {
		return 0, ErrNotEnoughData
	}

	timeSec := ebpf.ByteOrder.Uint64(data[0:8])
	timeNsec := ebpf.ByteOrder.Uint64(data[8:16])
	e.Atime = time.Unix(int64(timeSec), int64(timeNsec))

	timeSec = ebpf.ByteOrder.Uint64(data[16:24])
	timeNsec = ebpf.ByteOrder.Uint64(data[24:32])
	e.Mtime = time.Unix(int64(timeSec), int64(timeNsec))

	return n + 32, nil
}

// LinkEvent represents a link event
type LinkEvent struct {
	SyscallEvent
	Source FileEvent `field:"source"`
	Target FileEvent `field:"target"`
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *LinkEvent) UnmarshalBinary(data []byte) (int, error) {
	return unmarshalBinary(data, &e.SyscallEvent, &e.Source, &e.Target)
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

// GetRootPathResolutionError returns the root path resolution error as a string if there is one
func (e *MountEvent) GetRootPathResolutionError() string {
	if e.RootPathResolutionError != nil {
		return e.RootPathResolutionError.Error()
	}
	return ""
}

// GetMountPointPathResolutionError returns the mount point path resolution error as a string if there is one
func (e *MountEvent) GetMountPointPathResolutionError() string {
	if e.MountPointPathResolutionError != nil {
		return e.MountPointPathResolutionError.Error()
	}
	return ""
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *MountEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := unmarshalBinary(data, &e.SyscallEvent)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 56 {
		return 0, ErrNotEnoughData
	}

	e.MountID = ebpf.ByteOrder.Uint32(data[0:4])
	e.GroupID = ebpf.ByteOrder.Uint32(data[4:8])
	e.Device = ebpf.ByteOrder.Uint32(data[8:12])
	e.ParentMountID = ebpf.ByteOrder.Uint32(data[12:16])
	e.ParentInode = ebpf.ByteOrder.Uint64(data[16:24])
	e.RootInode = ebpf.ByteOrder.Uint64(data[24:32])
	e.RootMountID = ebpf.ByteOrder.Uint32(data[32:36])

	// Notes: bytes 36 to 40 are used to pad the structure

	utils.SliceToArray(data[40:56], unsafe.Pointer(&e.FSTypeRaw))

	return 56, nil
}

// ResolveMountPoint resolves the mountpoint to a full path
func (e *MountEvent) ResolveMountPoint(event *Event) string {
	if len(e.MountPointStr) == 0 {
		e.MountPointStr, e.MountPointPathResolutionError = event.resolvers.DentryResolver.Resolve(e.ParentMountID, e.ParentInode, 0)
	}
	return e.MountPointStr
}

// ResolveRoot resolves the mountpoint to a full path
func (e *MountEvent) ResolveRoot(event *Event) string {
	if len(e.RootStr) == 0 {
		e.RootStr, e.RootPathResolutionError = event.resolvers.DentryResolver.Resolve(e.RootMountID, e.RootInode, 0)
	}
	return e.RootStr
}

// GetFSType returns the filesystem type of the mountpoint
func (e *MountEvent) GetFSType() string {
	if len(e.FSType) == 0 {
		e.FSType = string(bytes.Trim(e.FSTypeRaw[:], "\x00"))
	}
	return e.FSType
}

// UmountEvent represents an umount event
type UmountEvent struct {
	SyscallEvent
	MountID           uint32
	DiscarderRevision uint32 `field:"-"`
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *UmountEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := unmarshalBinary(data, &e.SyscallEvent)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 8 {
		return 0, ErrNotEnoughData
	}

	e.MountID = ebpf.ByteOrder.Uint32(data[0:4])
	e.DiscarderRevision = ebpf.ByteOrder.Uint32(data[4:8])

	return 8, nil
}

// ContainerContext holds the container context of an event
type ContainerContext struct {
	ID string `field:"id" handler:"ResolveContainerID,string"`
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *ContainerContext) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 64 {
		return 0, ErrNotEnoughData
	}

	idRaw := [64]byte{}
	utils.SliceToArray(data[0:64], unsafe.Pointer(&idRaw))
	e.ID = string(bytes.Trim(idRaw[:], "\x00"))
	if len(e.ID) > 1 && len(e.ID) < 64 {
		e.ID = ""
	}

	return 64, nil
}

// Bytes returns a binary representation of itself
func (e *ContainerContext) Bytes() []byte {
	return utils.ContainerID(e.ID).Bytes()
}

// ResolveContainerID resolves the container ID of the event
func (e *ContainerContext) ResolveContainerID(event *Event) string {
	if len(e.ID) == 0 && event != nil {
		if entry := event.ResolveProcessCacheEntry(); entry != nil {
			e.ID = entry.ID
		}
	}
	return e.ID
}

// ExecEvent represents a exec event
type ExecEvent struct {
	// proc_cache_t
	// (container context is parsed in Event.Container)
	FileEvent
	ExecTimestamp time.Time `field:"-"`
	TTYName       string    `field:"tty_name" handler:"ResolveTTY,string"`
	Name          string    `field:"name" handler:"ResolveName,string"`
	Comm          string    `field:"-" handler:"ResolveComm,string"`

	// pid_cache_t
	ForkTimestamp time.Time `field:"-"`
	ExitTimestamp time.Time `field:"-"`
	Cookie        uint32    `field:"cookie" handler:"ResolveCookie,int"`
	PPid          uint32    `field:"ppid" handler:"ResolvePPID,int"`

	// The following fields should only be used here for evaluation
	UID   uint32 `field:"uid" handler:"ResolveUID,int"`
	GID   uint32 `field:"gid" handler:"ResolveGID,int"`
	User  string `field:"user" handler:"ResolveUser,string"`
	Group string `field:"group" handler:"ResolveGroup,string"`
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *ExecEvent) UnmarshalBinary(data []byte, resolvers *Resolvers) (int, error) {
	if len(data) < 136 {
		return 0, ErrNotEnoughData
	}

	// Unmarshal proc_cache_t
	read, err := unmarshalBinary(data, &e.FileEvent)
	if err != nil {
		return read, err
	}

	e.ExecTimestamp = resolvers.TimeResolver.ResolveMonotonicTimestamp(ebpf.ByteOrder.Uint64(data[read : read+8]))
	read += 8

	var ttyRaw [64]byte
	utils.SliceToArray(data[read:read+64], unsafe.Pointer(&ttyRaw))
	e.TTYName = string(bytes.Trim(ttyRaw[:], "\x00"))
	read += 64

	var commRaw [16]byte
	utils.SliceToArray(data[read:read+16], unsafe.Pointer(&commRaw))
	e.Comm = string(bytes.Trim(commRaw[:], "\x00"))
	read += 16

	// Unmarshal pid_cache_t
	e.Cookie = ebpf.ByteOrder.Uint32(data[read : read+4])
	e.PPid = ebpf.ByteOrder.Uint32(data[read+4 : read+8])
	e.ForkTimestamp = resolvers.TimeResolver.ResolveMonotonicTimestamp(ebpf.ByteOrder.Uint64(data[read+8 : read+16]))
	e.ExitTimestamp = resolvers.TimeResolver.ResolveMonotonicTimestamp(ebpf.ByteOrder.Uint64(data[read+16 : read+24]))

	// resolve FileEvent now so that the dentry cache is up to date. Fork events might send a null inode if the parent
	// wasn't in the kernel cache, so only resolve if necessary
	if e.FileEvent.Inode != 0 && e.FileEvent.MountID != 0 {
		_ = e.FileEvent.ResolveInodeWithResolvers(resolvers)
		e.FileEvent.ResolveContainerPathWithResolvers(resolvers)
	}

	// ignore uid / gid, it has already been parsed in Event.Process
	// add 8 to the total

	return read + 32, nil
}

// UnmarshalEvent unmarshal an ExecEvent
func (e *ExecEvent) UnmarshalEvent(data []byte, event *Event) (int, error) {
	if len(data) < 136 {
		return 0, ErrNotEnoughData
	}

	// reset the process cache entry of the current event
	entry := NewProcessCacheEntry()
	entry.ContainerContext = event.Container
	entry.ProcessContext = ProcessContext{
		Pid: event.Process.Pid,
		Tid: event.Process.Tid,
		UID: event.Process.UID,
		GID: event.Process.GID,
	}
	event.processCacheEntry = entry

	return event.processCacheEntry.UnmarshalBinary(data, event.resolvers, false)
}

// ResolvePPID resolves the parent process ID
func (e *ExecEvent) ResolvePPID(event *Event) int {
	if e.PPid == 0 && event != nil {
		if entry := event.ResolveProcessCacheEntry(); entry != nil {
			e.PPid = entry.PPid
		}
	}
	return int(e.PPid)
}

// ResolveInode resolves the inode to a full path
func (e *ExecEvent) ResolveInode(event *Event) string {
	if len(e.PathnameStr) == 0 && event != nil {
		if entry := event.ResolveProcessCacheEntry(); entry != nil {
			e.PathnameStr = entry.PathnameStr
		}
	}
	return e.PathnameStr
}

// ResolveContainerPath resolves the inode to a path relative to the container
func (e *ExecEvent) ResolveContainerPath(event *Event) string {
	if len(e.ContainerPath) == 0 && event != nil {
		if entry := event.ResolveProcessCacheEntry(); entry != nil {
			e.ContainerPath = entry.ContainerPath
		}
	}
	return e.ContainerPath
}

// ResolveBasename resolves the inode to a filename
func (e *ExecEvent) ResolveBasename(event *Event) string {
	if len(e.BasenameStr) == 0 {
		if e.PathnameStr == "" {
			e.PathnameStr = e.ResolveInode(event)
		}

		e.BasenameStr = path.Base(e.PathnameStr)
	}
	return e.BasenameStr
}

// ResolveCookie resolves the cookie of the process
func (e *ExecEvent) ResolveCookie(event *Event) int {
	if e.Cookie == 0 && event != nil {
		if entry := event.ResolveProcessCacheEntry(); entry != nil {
			e.Cookie = entry.Cookie
		}
	}
	return int(e.Cookie)
}

// ResolveTTY resolves the name of the process tty
func (e *ExecEvent) ResolveTTY(event *Event) string {
	if e.TTYName == "" && event != nil {
		if entry := event.ResolveProcessCacheEntry(); entry != nil {
			e.TTYName = entry.TTYName
		}
	}
	return e.TTYName
}

// ResolveComm resolves the comm of the process
func (e *ExecEvent) ResolveComm(event *Event) string {
	if len(e.Comm) == 0 && event != nil {
		if entry := event.ResolveProcessCacheEntry(); entry != nil {
			e.Comm = entry.Comm
		}
	}
	return e.Comm
}

// ResolveName resolves the basename of the process executable
func (e *ExecEvent) ResolveName(event *Event) string {
	return e.ResolveBasename(event)
}

// ResolveUID resolves the user id of the process
func (e *ExecEvent) ResolveUID(event *Event) int {
	if e.UID == 0 && event != nil {
		if entry := event.ResolveProcessCacheEntry(); entry != nil {
			e.UID = entry.UID
		}
	}
	return int(e.UID)
}

// ResolveGID resolves the group id of the process
func (e *ExecEvent) ResolveGID(event *Event) int {
	if e.GID == 0 && event != nil {
		if entry := event.ResolveProcessCacheEntry(); entry != nil {
			e.GID = entry.GID
		}
	}
	return int(e.GID)
}

// ResolveUser resolves the user id of the process to a username
func (e *ExecEvent) ResolveUser(event *Event) string {
	if len(e.User) == 0 && event != nil {
		e.User, _ = event.resolvers.UserGroupResolver.ResolveUser(int(event.Process.UID))
	}
	return e.User
}

// ResolveGroup resolves the group id of the process to a group name
func (e *ExecEvent) ResolveGroup(event *Event) string {
	if len(e.Group) == 0 && event != nil {
		e.Group, _ = event.resolvers.UserGroupResolver.ResolveGroup(int(event.Process.GID))
	}
	return e.Group
}

// ResolveForkTimestamp returns the fork timestamp of the process
func (e *ExecEvent) ResolveForkTimestamp(event *Event) time.Time {
	if e.ForkTimestamp.IsZero() && event != nil {
		if entry := event.ResolveProcessCacheEntry(); entry != nil {
			e.ForkTimestamp = entry.ForkTimestamp
		}
	}
	return e.ForkTimestamp
}

// ResolveExecTimestamp returns the execve timestamp of the process
func (e *ExecEvent) ResolveExecTimestamp(event *Event) time.Time {
	if e.ExecTimestamp.IsZero() && event != nil {
		if entry := event.ResolveProcessCacheEntry(); entry != nil {
			e.ExecTimestamp = entry.ExecTimestamp
		}
	}
	return e.ExecTimestamp
}

// ResolveExitTimestamp returns the exit timestamp of the process
func (e *ExecEvent) ResolveExitTimestamp(event *Event) time.Time {
	if e.ExitTimestamp.IsZero() && event != nil {
		if entry := event.ResolveProcessCacheEntry(); entry != nil {
			e.ExitTimestamp = entry.ExitTimestamp
		}
	}
	return e.ExitTimestamp
}

// InvalidateDentryEvent defines a invalidate dentry event
type InvalidateDentryEvent struct {
	Inode             uint64
	MountID           uint32
	DiscarderRevision uint32
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *InvalidateDentryEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 16 {
		return 0, ErrNotEnoughData
	}

	e.Inode = ebpf.ByteOrder.Uint64(data[0:8])
	e.MountID = ebpf.ByteOrder.Uint32(data[8:12])
	e.DiscarderRevision = ebpf.ByteOrder.Uint32(data[12:16])

	return 16, nil
}

// ProcessCacheEntry this structure holds the container context that we keep in kernel for each process
type ProcessCacheEntry struct {
	ContainerContext
	ProcessContext
}

// NewProcessCacheEntry returns an empty instance of ProcessCacheEntry
func NewProcessCacheEntry() *ProcessCacheEntry {
	return &ProcessCacheEntry{}
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

// UnmarshalBinary unmarshals a binary representation of itself
func (p *ProcessContext) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 16 {
		return 0, ErrNotEnoughData
	}

	p.Pid = ebpf.ByteOrder.Uint32(data[0:4])
	p.Tid = ebpf.ByteOrder.Uint32(data[4:8])
	p.UID = ebpf.ByteOrder.Uint32(data[8:12])
	p.GID = ebpf.ByteOrder.Uint32(data[12:16])

	return 16, nil
}

// ResolveUser resolves the user id of the process to a username
func (p *ProcessContext) ResolveUser(event *Event) string {
	return p.ResolveUserWithResolvers(event.resolvers)
}

// ResolveUserWithResolvers resolves the user id of the process to a username
func (p *ProcessContext) ResolveUserWithResolvers(resolvers *Resolvers) string {
	if len(p.User) == 0 {
		p.User, _ = resolvers.UserGroupResolver.ResolveUser(int(p.UID))
	}
	return p.User
}

// ResolveGroup resolves the group id of the process to a group name
func (p *ProcessContext) ResolveGroup(event *Event) string {
	return p.ResolveGroupWithResolvers(event.resolvers)
}

// ResolveGroupWithResolvers resolves the group id of the process to a group name
func (p *ProcessContext) ResolveGroupWithResolvers(resolvers *Resolvers) string {
	if len(p.Group) == 0 {
		p.Group, _ = resolvers.UserGroupResolver.ResolveGroup(int(p.GID))
	}
	return p.Group
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

	resolvers           *Resolvers         `field:"-"`
	processCacheEntry   *ProcessCacheEntry `field:"-"`
	pathResolutionError error              `field:"-"`
}

func (e *Event) String() string {
	d, err := json.Marshal(e)
	if err != nil {
		return err.Error()
	}
	return string(d)
}

// SetPathResolutionError sets the Event.pathResolutionError
func (e *Event) SetPathResolutionError(err error) {
	e.pathResolutionError = err
}

// GetPathResolutionError returns the path resolution error
func (e *Event) GetPathResolutionError() error {
	return e.pathResolutionError
}

// MarshalJSON returns the JSON encoding of the event
func (e *Event) MarshalJSON() ([]byte, error) {
	s := newEventSerializer(e)
	return json.Marshal(s)
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

// UnmarshalBinary unmarshals a binary representation of itself
func (e *Event) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 24 {
		return 0, ErrNotEnoughData
	}

	e.TimestampRaw = ebpf.ByteOrder.Uint64(data[8:16])
	e.Type = ebpf.ByteOrder.Uint64(data[16:24])

	return 24, nil
}

// ExtractEventInfo extracts cpu and timestamp from the raw data event
func ExtractEventInfo(data []byte) (uint64, uint64, error) {
	if len(data) < 16 {
		return 0, 0, ErrNotEnoughData
	}

	return ebpf.ByteOrder.Uint64(data[0:8]), ebpf.ByteOrder.Uint64(data[8:16]), nil
}

// ResolveEventTimestamp resolves the monolitic kernel event timestamp to an absolute time
func (e *Event) ResolveEventTimestamp() time.Time {
	if e.Timestamp.IsZero() {
		e.Timestamp = e.resolvers.TimeResolver.ResolveMonotonicTimestamp(e.TimestampRaw)
		if e.Timestamp.IsZero() {
			e.Timestamp = time.Now()
		}
	}
	return e.Timestamp
}

// ResolveProcessCacheEntry queries the ProcessResolver to retrieve the ProcessCacheEntry of the event
func (e *Event) ResolveProcessCacheEntry() *ProcessCacheEntry {
	if e.processCacheEntry == nil {
		e.processCacheEntry = e.resolvers.ProcessResolver.Resolve(e.Process.Pid)
		if e.processCacheEntry == nil {
			e.processCacheEntry = &ProcessCacheEntry{}
		}
	}
	e.Process.Ancestor = e.processCacheEntry.Ancestor
	return e.processCacheEntry
}

// updateProcessCachePointer updates the internal pointers of the event structure to the ProcessCacheEntry of the event
func (e *Event) updateProcessCachePointer(event *ProcessCacheEntry) {
	e.processCacheEntry = event
	e.Process.Ancestor = event.Ancestor
}

// Clone returns a copy on the event
func (e *Event) Clone() Event {
	return *e
}

// NewEvent returns a new event
func NewEvent(resolvers *Resolvers) *Event {
	return &Event{
		resolvers: resolvers,
	}
}
