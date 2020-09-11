// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

//go:generate go run github.com/DataDog/datadog-agent/pkg/security/secl/generators/accessors -tags linux -output model_accessors.go

package probe

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os/user"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/google/uuid"
	"github.com/pkg/errors"
)

var (
	byteOrder              = ebpf.ByteOrder
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
		value, ok := field.Value.(string)
		if ok {
			if value != path.Clean(value) || !path.IsAbs(value) {
				return fmt.Errorf("invalid path `%s`, all the path have to be absolute", value)
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

// BaseEvent contains common fields for all the event
type BaseEvent struct {
	TimestampRaw uint64    `field:"-"`
	Timestamp    time.Time `field:"-"`
	Retval       int64     `field:"retval"`
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *BaseEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 16 {
		return 0, ErrNotEnoughData
	}
	e.TimestampRaw = byteOrder.Uint64(data[0:8])
	e.Retval = int64(byteOrder.Uint64(data[8:16]))
	return 16, nil
}

func (e *BaseEvent) marshalJSON(eventType EventType, resolvers *Resolvers) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteRune('{')
	fmt.Fprintf(&buf, `"type":"%s",`, eventType.String())
	fmt.Fprintf(&buf, `"timestamp":"%s",`, e.ResolveMonotonicTimestamp(resolvers))
	fmt.Fprintf(&buf, `"retval":%d`, e.Retval)
	buf.WriteRune('}')

	return buf.Bytes(), nil
}

// ResolveMonotonicTimestamp resolves the monolitic kernel timestamp to an absolute time
func (e *BaseEvent) ResolveMonotonicTimestamp(resolvers *Resolvers) time.Time {
	if (e.Timestamp.Equal(time.Time{})) {
		e.Timestamp = resolvers.TimeResolver.ResolveMonotonicTimestamp(e.TimestampRaw)
	}
	return e.Timestamp
}

// BinaryUnmarshaler interface implemented by every event type
type BinaryUnmarshaler interface {
	UnmarshalBinary(data []byte) (int, error)
}

// FileEvent is the common file event type
type FileEvent struct {
	MountID         uint32 `field:"-"`
	Inode           uint64 `field:"inode"`
	OverlayNumLower int32  `field:"overlay_numlower"`
	PathnameStr     string `field:"filename" handler:"ResolveInode,string"`
	ContainerPath   string `field:"container_path" handler:"ResolveContainerPath,string"`
	BasenameStr     string `field:"basename" handler:"ResolveBasename,string"`
}

// ResolveInode resolves the inode to a full path
func (e *FileEvent) ResolveInode(resolvers *Resolvers) string {
	if len(e.PathnameStr) == 0 {
		e.PathnameStr = resolvers.DentryResolver.Resolve(e.MountID, e.Inode)
		_, mountPath, rootPath, err := resolvers.MountResolver.GetMountPath(e.MountID, e.OverlayNumLower)
		if err == nil {
			if strings.HasPrefix(e.PathnameStr, rootPath) && rootPath != "/" {
				e.PathnameStr = strings.Replace(e.PathnameStr, rootPath, "", 1)
			}
			e.PathnameStr = path.Join(mountPath, e.PathnameStr)
		}
	}
	return e.PathnameStr
}

// ResolveContainerPath resolves the inode to a path relative to the container
func (e *FileEvent) ResolveContainerPath(resolvers *Resolvers) string {
	if len(e.ContainerPath) == 0 {
		containerPath, _, _, err := resolvers.MountResolver.GetMountPath(e.MountID, e.OverlayNumLower)
		if err == nil {
			e.ContainerPath = containerPath
		}
		if len(containerPath) == 0 && len(e.PathnameStr) == 0 {
			// The container path might be included in the pathname. The container path will be set there.
			_ = e.ResolveInode(resolvers)
		}
	}
	return e.ContainerPath
}

// ResolveBasename resolves the inode to a filename
func (e *FileEvent) ResolveBasename(resolvers *Resolvers) string {
	if len(e.BasenameStr) == 0 {
		e.BasenameStr = resolvers.DentryResolver.GetName(e.MountID, e.Inode)
	}
	return e.BasenameStr
}

func (e *FileEvent) marshalJSON(resolvers *Resolvers) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteRune('{')
	fmt.Fprintf(&buf, `"filename":"%s",`, e.ResolveInode(resolvers))
	fmt.Fprintf(&buf, `"container_path":"%s",`, e.ResolveContainerPath(resolvers))
	fmt.Fprintf(&buf, `"inode":%d,`, e.Inode)
	fmt.Fprintf(&buf, `"mount_id":%d,`, e.MountID)
	fmt.Fprintf(&buf, `"overlay_numlower":%d`, e.OverlayNumLower)
	buf.WriteRune('}')

	return buf.Bytes(), nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *FileEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 16 {
		return 0, ErrNotEnoughData
	}
	e.Inode = byteOrder.Uint64(data[0:8])
	e.MountID = byteOrder.Uint32(data[8:12])
	e.OverlayNumLower = int32(byteOrder.Uint32(data[12:16]))
	return 16, nil
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

// ChmodEvent represents a chmod event
type ChmodEvent struct {
	BaseEvent
	FileEvent
	Mode uint32 `field:"mode"`
}

func (e *ChmodEvent) marshalJSON(resolvers *Resolvers) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteRune('{')
	fmt.Fprintf(&buf, `"filename":"%s",`, e.ResolveInode(resolvers))
	fmt.Fprintf(&buf, `"container_path":"%s",`, e.ResolveContainerPath(resolvers))
	fmt.Fprintf(&buf, `"inode":%d,`, e.Inode)
	fmt.Fprintf(&buf, `"mount_id":%d,`, e.MountID)
	fmt.Fprintf(&buf, `"overlay_numlower":%d,`, e.OverlayNumLower)
	fmt.Fprintf(&buf, `"mode":%d`, e.Mode)
	buf.WriteRune('}')

	return buf.Bytes(), nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *ChmodEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := unmarshalBinary(data, &e.BaseEvent, &e.FileEvent)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 4 {
		return n, ErrNotEnoughData
	}

	e.Mode = byteOrder.Uint32(data[0:4])
	return n + 4, nil
}

// ChownEvent represents a chown event
type ChownEvent struct {
	BaseEvent
	FileEvent
	UID int32 `field:"uid"`
	GID int32 `field:"gid"`
}

func (e *ChownEvent) marshalJSON(resolvers *Resolvers) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteRune('{')
	fmt.Fprintf(&buf, `"filename":"%s",`, e.ResolveInode(resolvers))
	fmt.Fprintf(&buf, `"container_path":"%s",`, e.ResolveContainerPath(resolvers))
	fmt.Fprintf(&buf, `"inode":%d,`, e.Inode)
	fmt.Fprintf(&buf, `"mount_id":%d,`, e.MountID)
	fmt.Fprintf(&buf, `"overlay_numlower":%d,`, e.OverlayNumLower)
	fmt.Fprintf(&buf, `"uid":%d,`, e.UID)
	fmt.Fprintf(&buf, `"gid":%d`, e.GID)
	buf.WriteRune('}')

	return buf.Bytes(), nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *ChownEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := unmarshalBinary(data, &e.BaseEvent, &e.FileEvent)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 8 {
		return n, ErrNotEnoughData
	}

	e.UID = int32(byteOrder.Uint32(data[0:4]))
	e.GID = int32(byteOrder.Uint32(data[4:8]))
	return n + 8, nil
}

// SetXAttrEvent represents an extended attributes event
type SetXAttrEvent struct {
	BaseEvent
	FileEvent
	Namespace string `field:"namespace" handler:"GetNamespace,string"`
	Name      string `field:"name" handler:"GetName,string"`

	NameRaw [200]byte
}

func (e *SetXAttrEvent) marshalJSON(resolvers *Resolvers) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteRune('{')
	fmt.Fprintf(&buf, `"filename":"%s",`, e.ResolveInode(resolvers))
	fmt.Fprintf(&buf, `"container_path":"%s",`, e.ResolveContainerPath(resolvers))
	fmt.Fprintf(&buf, `"inode":%d,`, e.Inode)
	fmt.Fprintf(&buf, `"mount_id":%d,`, e.MountID)
	fmt.Fprintf(&buf, `"overlay_numlower":%d,`, e.OverlayNumLower)
	fmt.Fprintf(&buf, `"attribute_name":"%s",`, e.GetName(resolvers))
	fmt.Fprintf(&buf, `"attribute_namespace":"%s"`, e.GetNamespace(resolvers))
	buf.WriteRune('}')

	return buf.Bytes(), nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *SetXAttrEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := unmarshalBinary(data, &e.BaseEvent, &e.FileEvent)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 200 {
		return n, ErrNotEnoughData
	}
	if err := binary.Read(bytes.NewBuffer(data[0:200]), byteOrder, &e.NameRaw); err != nil {
		return 0, err
	}
	return n + 200, nil
}

// GetName returns the string representation of the extended attribute name
func (e *SetXAttrEvent) GetName(resolvers *Resolvers) string {
	if len(e.Name) == 0 {
		e.Name = string(bytes.Trim(e.NameRaw[:], "\x00"))
	}
	return e.Name
}

// GetNamespace returns the string representation of the extended attribute namespace
func (e *SetXAttrEvent) GetNamespace(resolvers *Resolvers) string {
	if len(e.Namespace) == 0 {
		fragments := strings.Split(e.GetName(resolvers), ".")
		if len(fragments) > 0 {
			e.Namespace = fragments[0]
		}
	}
	return e.Namespace
}

// OpenEvent represents an open event
type OpenEvent struct {
	BaseEvent
	FileEvent
	Flags uint32 `field:"flags"`
	Mode  uint32 `field:"mode"`
}

func (e *OpenEvent) marshalJSON(resolvers *Resolvers) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteRune('{')
	fmt.Fprintf(&buf, `"filename":"%s",`, e.ResolveInode(resolvers))
	fmt.Fprintf(&buf, `"container_path":"%s",`, e.ResolveContainerPath(resolvers))
	fmt.Fprintf(&buf, `"inode":%d,`, e.Inode)
	fmt.Fprintf(&buf, `"mount_id":%d,`, e.MountID)
	fmt.Fprintf(&buf, `"overlay_numlower":%d,`, e.OverlayNumLower)
	fmt.Fprintf(&buf, `"mode":%d,`, e.Mode)
	fmt.Fprintf(&buf, `"flags":"%s"`, OpenFlags(e.Flags))
	buf.WriteRune('}')

	return buf.Bytes(), nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *OpenEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := unmarshalBinary(data, &e.BaseEvent, &e.FileEvent)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 8 {
		return n, ErrNotEnoughData
	}

	e.Flags = byteOrder.Uint32(data[0:4])
	e.Mode = byteOrder.Uint32(data[4:8])
	return n + 8, nil
}

// MkdirEvent represents a mkdir event
type MkdirEvent struct {
	BaseEvent
	FileEvent
	Mode int32 `field:"mode"`
}

func (e *MkdirEvent) marshalJSON(resolvers *Resolvers) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteRune('{')
	fmt.Fprintf(&buf, `"filename":"%s",`, e.ResolveInode(resolvers))
	fmt.Fprintf(&buf, `"container_path":"%s",`, e.ResolveContainerPath(resolvers))
	fmt.Fprintf(&buf, `"inode":%d,`, e.Inode)
	fmt.Fprintf(&buf, `"mount_id":%d,`, e.MountID)
	fmt.Fprintf(&buf, `"overlay_numlower":%d,`, e.OverlayNumLower)
	fmt.Fprintf(&buf, `"mode":%d`, e.Mode)
	buf.WriteRune('}')

	return buf.Bytes(), nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *MkdirEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := unmarshalBinary(data, &e.BaseEvent, &e.FileEvent)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 4 {
		return n, ErrNotEnoughData
	}

	e.Mode = int32(byteOrder.Uint32(data[0:4]))
	return n + 4, nil
}

// RmdirEvent represents a rmdir event
type RmdirEvent struct {
	BaseEvent
	FileEvent
}

func (e *RmdirEvent) marshalJSON(resolvers *Resolvers) ([]byte, error) {
	return e.FileEvent.marshalJSON(resolvers)
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *RmdirEvent) UnmarshalBinary(data []byte) (int, error) {
	return unmarshalBinary(data, &e.BaseEvent, &e.FileEvent)
}

// UnlinkEvent represents an unlink event
type UnlinkEvent struct {
	BaseEvent
	FileEvent
	Flags uint32 `field:"flags"`
}

func (e *UnlinkEvent) marshalJSON(resolvers *Resolvers) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteRune('{')
	fmt.Fprintf(&buf, `"filename":"%s",`, e.ResolveInode(resolvers))
	fmt.Fprintf(&buf, `"flags":"%s",`, UnlinkFlags(e.Flags))
	fmt.Fprintf(&buf, `"container_path":"%s",`, e.ResolveContainerPath(resolvers))
	fmt.Fprintf(&buf, `"inode":%d,`, e.Inode)
	fmt.Fprintf(&buf, `"mount_id":%d,`, e.MountID)
	fmt.Fprintf(&buf, `"overlay_numlower":%d`, e.OverlayNumLower)
	buf.WriteRune('}')

	return buf.Bytes(), nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *UnlinkEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := unmarshalBinary(data, &e.BaseEvent, &e.FileEvent)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 4 {
		return 0, ErrNotEnoughData
	}

	e.Flags = byteOrder.Uint32(data[0:4])
	return n + 4, nil
}

// RenameEvent represents a rename event
type RenameEvent struct {
	BaseEvent
	Old FileEvent `field:"old"`
	New FileEvent `field:"new"`
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *RenameEvent) UnmarshalBinary(data []byte) (int, error) {
	return unmarshalBinary(data, &e.BaseEvent, &e.Old, &e.New)
}

// UtimesEvent represents a utime event
type UtimesEvent struct {
	BaseEvent
	FileEvent
	Atime time.Time
	Mtime time.Time
}

func (e *UtimesEvent) marshalJSON(resolvers *Resolvers) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteRune('{')
	fmt.Fprintf(&buf, `"filename":"%s",`, e.ResolveInode(resolvers))
	fmt.Fprintf(&buf, `"container_path":"%s",`, e.ResolveContainerPath(resolvers))
	fmt.Fprintf(&buf, `"inode":%d,`, e.Inode)
	fmt.Fprintf(&buf, `"mount_id":%d,`, e.MountID)
	fmt.Fprintf(&buf, `"overlay_numlower":%d`, e.OverlayNumLower)
	fmt.Fprintf(&buf, `"access_time":"%s"`, e.Atime)
	fmt.Fprintf(&buf, `"modification_time":"%s"`, e.Mtime)
	buf.WriteRune('}')

	return buf.Bytes(), nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *UtimesEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := unmarshalBinary(data, &e.BaseEvent, &e.FileEvent)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 32 {
		return 0, ErrNotEnoughData
	}

	timeSec := byteOrder.Uint64(data[0:8])
	timeNsec := byteOrder.Uint64(data[8:16])
	e.Atime = time.Unix(int64(timeSec), int64(timeNsec))

	timeSec = byteOrder.Uint64(data[16:24])
	timeNsec = byteOrder.Uint64(data[24:32])
	e.Mtime = time.Unix(int64(timeSec), int64(timeNsec))

	return n + 32, nil
}

// LinkEvent represents a link event
type LinkEvent struct {
	BaseEvent
	Source FileEvent `field:"source"`
	Target FileEvent `field:"target"`
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *LinkEvent) UnmarshalBinary(data []byte) (int, error) {
	return unmarshalBinary(data, &e.BaseEvent, &e.Source, &e.Target)
}

// MountEvent represents a mount event
type MountEvent struct {
	BaseEvent
	NewMountID    uint32
	NewGroupID    uint32
	NewDevice     uint32
	ParentMountID uint32
	ParentInode   uint64
	FSType        string
	MountPointStr string
	RootMountID   uint32
	RootInode     uint64
	RootStr       string

	FSTypeRaw [16]byte
}

func (e *MountEvent) marshalJSON(resolvers *Resolvers) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteRune('{')
	fmt.Fprintf(&buf, `"mount_point":"%s",`, e.ResolveMountPoint(resolvers))
	fmt.Fprintf(&buf, `"parent_mount_id":%d,`, e.ParentMountID)
	fmt.Fprintf(&buf, `"parent_inode":%d,`, e.ParentInode)
	fmt.Fprintf(&buf, `"root_inode":%d,`, e.RootInode)
	fmt.Fprintf(&buf, `"root_mount_id":%d,`, e.RootInode)
	fmt.Fprintf(&buf, `"root":"%s",`, e.ResolveRoot(resolvers))
	fmt.Fprintf(&buf, `"new_mount_id":%d,`, e.NewMountID)
	fmt.Fprintf(&buf, `"new_group_id":%d,`, e.NewGroupID)
	fmt.Fprintf(&buf, `"new_device":%d,`, e.NewDevice)
	fmt.Fprintf(&buf, `"fstype":"%s"`, e.GetFSType())
	buf.WriteRune('}')

	return buf.Bytes(), nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *MountEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := unmarshalBinary(data, &e.BaseEvent)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 56 {
		return 0, ErrNotEnoughData
	}

	e.NewMountID = byteOrder.Uint32(data[0:4])
	e.NewGroupID = byteOrder.Uint32(data[4:8])
	e.NewDevice = byteOrder.Uint32(data[8:12])
	e.ParentMountID = byteOrder.Uint32(data[12:16])
	e.ParentInode = byteOrder.Uint64(data[16:24])
	e.RootInode = byteOrder.Uint64(data[24:32])
	e.RootMountID = byteOrder.Uint32(data[32:36])

	if err := binary.Read(bytes.NewBuffer(data[40:56]), byteOrder, &e.FSTypeRaw); err != nil {
		return 40, err
	}

	return 56, nil
}

// ResolveMountPoint resolves the mountpoint to a full path
func (e *MountEvent) ResolveMountPoint(resolvers *Resolvers) string {
	if len(e.MountPointStr) == 0 {
		e.MountPointStr = resolvers.DentryResolver.Resolve(e.ParentMountID, e.ParentInode)
	}
	return e.MountPointStr
}

// ResolveRoot resolves the mountpoint to a full path
func (e *MountEvent) ResolveRoot(resolvers *Resolvers) string {
	if len(e.RootStr) == 0 {
		e.RootStr = resolvers.DentryResolver.Resolve(e.RootMountID, e.RootInode)
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
	BaseEvent
	MountID uint32
}

func (e *UmountEvent) marshalJSON(resolvers *Resolvers) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteRune('{')
	fmt.Fprintf(&buf, `"mount_id":%d`, e.MountID)
	buf.WriteRune('}')

	return buf.Bytes(), nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *UmountEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := unmarshalBinary(data, &e.BaseEvent)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 4 {
		return 0, ErrNotEnoughData
	}

	e.MountID = byteOrder.Uint32(data[0:4])
	return 4, nil
}

// ContainerEvent holds the container context of an event
type ContainerEvent struct {
	ID string `field:"id" handler:"ResolveContainerID,string"`

	IDRaw [64]byte `field:"-"`
}

func (e *ContainerEvent) marshalJSON(resolvers *Resolvers) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteRune('{')
	if id := e.GetContainerID(); len(id) > 0 {
		fmt.Fprintf(&buf, `"container_id":"%s"`, id)
	}
	buf.WriteRune('}')

	return buf.Bytes(), nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *ContainerEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 64 {
		return 0, ErrNotEnoughData
	}
	if err := binary.Read(bytes.NewBuffer(data[0:64]), byteOrder, &e.IDRaw); err != nil {
		return 0, err
	}
	return 64, nil
}

// ResolveContainerID resolves the container ID of the event
func (e *ContainerEvent) ResolveContainerID(resolvers *Resolvers) string {
	return e.GetContainerID()
}

// GetContainerID returns the container ID of the event
func (e *ContainerEvent) GetContainerID() string {
	if len(e.ID) == 0 {
		e.ID = string(bytes.Trim(e.IDRaw[:], "\x00"))
		if len(e.ID) > 1 && len(e.ID) < 64 {
			e.ID = ""
		}
	}
	return e.ID
}

// ProcessEvent holds the process context of an event
type ProcessEvent struct {
	FileEvent
	Pidns   uint64 `field:"pidns"`
	Comm    string `field:"name" handler:"ResolveComm,string"`
	TTYName string `field:"tty_name" handler:"ResolveTTY,string"`
	Pid     uint32 `field:"pid"`
	Tid     uint32 `field:"tid"`
	UID     uint32 `field:"uid"`
	GID     uint32 `field:"gid"`
	User    string `field:"user" handler:"ResolveUser,string"`
	Group   string `field:"group" handler:"ResolveGroup,string"`

	CommRaw    [16]byte `field:"-"`
	TTYNameRaw [64]byte `field:"-"`
}

func (p *ProcessEvent) marshalJSON(resolvers *Resolvers) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteRune('{')
	fmt.Fprintf(&buf, `"pidns":%d,`, p.Pidns)
	fmt.Fprintf(&buf, `"name":"%s",`, p.GetComm())
	if tty := p.GetTTY(); tty != "" {
		fmt.Fprintf(&buf, `"tty_name":"%s",`, tty)
	}
	fmt.Fprintf(&buf, `"pid":%d,`, p.Pid)
	fmt.Fprintf(&buf, `"tid":%d,`, p.Tid)
	fmt.Fprintf(&buf, `"uid":%d,`, p.UID)
	fmt.Fprintf(&buf, `"gid":%d`, p.GID)
	buf.WriteRune('}')

	return buf.Bytes(), nil
}

// ResolveTTY resolves the name of the process tty
func (p *ProcessEvent) ResolveTTY(resolvers *Resolvers) string {
	return p.GetTTY()
}

// GetTTY returns the name of the process tty
func (p *ProcessEvent) GetTTY() string {
	if len(p.TTYName) == 0 {
		p.TTYName = string(bytes.Trim(p.TTYNameRaw[:], "\x00"))
	}
	return p.TTYName
}

// ResolveComm resolves the comm of the process
func (p *ProcessEvent) ResolveComm(resolvers *Resolvers) string {
	return p.GetComm()
}

// GetComm returns the comm of the process
func (p *ProcessEvent) GetComm() string {
	if len(p.Comm) == 0 {
		p.Comm = string(bytes.Trim(p.CommRaw[:], "\x00"))
	}
	return p.Comm
}

// ResolveUser resolves the user id of the process to a username
func (p *ProcessEvent) ResolveUser(resolvers *Resolvers) string {
	u, err := user.LookupId(strconv.Itoa(int(p.UID)))
	if err == nil {
		p.User = u.Username
	}
	return p.User
}

// ResolveGroup resolves the group id of the process to a group name
func (p *ProcessEvent) ResolveGroup(resolvers *Resolvers) string {
	g, err := user.LookupGroupId(strconv.Itoa(int(p.GID)))
	if err == nil {
		p.Group = g.Name
	}
	return p.Group
}

// UnmarshalBinary unmarshals a binary representation of itself
func (p *ProcessEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 108 {
		return 0, ErrNotEnoughData
	}
	p.Pidns = byteOrder.Uint64(data[0:8])
	if err := binary.Read(bytes.NewBuffer(data[8:24]), byteOrder, &p.CommRaw); err != nil {
		return 8, err
	}
	if err := binary.Read(bytes.NewBuffer(data[24:88]), byteOrder, &p.TTYNameRaw); err != nil {
		return 8 + len(p.CommRaw), err
	}
	p.Pid = byteOrder.Uint32(data[88:92])
	p.Tid = byteOrder.Uint32(data[92:96])
	p.UID = byteOrder.Uint32(data[96:100])
	p.GID = byteOrder.Uint32(data[100:104])

	read, err := p.FileEvent.UnmarshalBinary(data[104:])
	if err != nil {
		return 104 + read, err
	}
	return 104 + read, nil
}

// Event represents an event sent from the kernel
// genaccessors
type Event struct {
	ID   string `field:"-"`
	Type uint64 `field:"-"`

	Process     ProcessEvent   `yaml:"process" field:"process" event:"*"`
	Container   ContainerEvent `yaml:"container" field:"container"`
	Chmod       ChmodEvent     `yaml:"chmod" field:"chmod" event:"chmod"`
	Chown       ChownEvent     `yaml:"chown" field:"chown" event:"chown"`
	Open        OpenEvent      `yaml:"open" field:"open" event:"open"`
	Mkdir       MkdirEvent     `yaml:"mkdir" field:"mkdir" event:"mkdir"`
	Rmdir       RmdirEvent     `yaml:"rmdir" field:"rmdir" event:"rmdir"`
	Rename      RenameEvent    `yaml:"rename" field:"rename" event:"rename"`
	Unlink      UnlinkEvent    `yaml:"unlink" field:"unlink" event:"unlink"`
	Utimes      UtimesEvent    `yaml:"utimes" field:"utimes" event:"utimes"`
	Link        LinkEvent      `yaml:"link" field:"link" event:"link"`
	SetXAttr    SetXAttrEvent  `yaml:"setxattr" field:"setxattr" event:"setxattr"`
	RemoveXAttr SetXAttrEvent  `yaml:"removexattr" field:"removexattr" event:"removexattr"`
	Mount       MountEvent     `yaml:"mount" field:"-"`
	Umount      UmountEvent    `yaml:"umount" field:"-"`

	resolvers *Resolvers `field:"-"`
}

func (e *Event) String() string {
	d, err := json.Marshal(e)
	if err != nil {
		return err.Error()
	}
	return string(d)
}

type eventMarshaler struct {
	field      string
	marshalFnc func(resolvers *Resolvers) ([]byte, error)
}

// MarshalJSON returns the JSON encoding of the event
func (e *Event) MarshalJSON() ([]byte, error) {
	eventID, _ := uuid.NewRandom()

	var buf bytes.Buffer
	buf.WriteRune('{')
	fmt.Fprintf(&buf, `"id":"%s",`, eventID)

	entries := []eventMarshaler{
		{
			field:      "process",
			marshalFnc: e.Process.marshalJSON,
		},
	}

	if len(e.Container.GetContainerID()) > 0 {
		entries = append(entries, eventMarshaler{
			field:      "container",
			marshalFnc: e.Container.marshalJSON,
		})
	}

	eventType := EventType(e.Type)

	eventMarshalJSON := func(e *BaseEvent) func(*Resolvers) ([]byte, error) {
		return func(resolvers *Resolvers) ([]byte, error) {
			return e.marshalJSON(eventType, resolvers)
		}
	}

	switch eventType {
	case FileChmodEventType:
		entries = append(entries,
			eventMarshaler{
				field:      "syscall",
				marshalFnc: eventMarshalJSON(&e.Chmod.BaseEvent),
			},
			eventMarshaler{
				field:      "file",
				marshalFnc: e.Chmod.marshalJSON,
			})
	case FileChownEventType:
		entries = append(entries,
			eventMarshaler{
				field:      "syscall",
				marshalFnc: eventMarshalJSON(&e.Chown.BaseEvent),
			},
			eventMarshaler{
				field:      "file",
				marshalFnc: e.Chown.marshalJSON,
			})
	case FileOpenEventType:
		entries = append(entries,
			eventMarshaler{
				field:      "syscall",
				marshalFnc: eventMarshalJSON(&e.Open.BaseEvent),
			},
			eventMarshaler{
				field:      "file",
				marshalFnc: e.Open.marshalJSON,
			})
	case FileMkdirEventType:
		entries = append(entries,
			eventMarshaler{
				field:      "syscall",
				marshalFnc: eventMarshalJSON(&e.Mkdir.BaseEvent),
			},
			eventMarshaler{
				field:      "file",
				marshalFnc: e.Mkdir.marshalJSON,
			})
	case FileRmdirEventType:
		entries = append(entries,
			eventMarshaler{
				field:      "syscall",
				marshalFnc: eventMarshalJSON(&e.Rmdir.BaseEvent),
			},
			eventMarshaler{
				field:      "file",
				marshalFnc: e.Rmdir.marshalJSON,
			})
	case FileUnlinkEventType:
		entries = append(entries,
			eventMarshaler{
				field:      "syscall",
				marshalFnc: eventMarshalJSON(&e.Unlink.BaseEvent),
			},
			eventMarshaler{
				field:      "file",
				marshalFnc: e.Unlink.marshalJSON,
			})
	case FileRenameEventType:
		entries = append(entries,
			eventMarshaler{
				field:      "syscall",
				marshalFnc: eventMarshalJSON(&e.Rename.BaseEvent),
			},
			eventMarshaler{
				field:      "old",
				marshalFnc: e.Rename.Old.marshalJSON,
			},
			eventMarshaler{
				field:      "new",
				marshalFnc: e.Rename.New.marshalJSON,
			})
	case FileUtimeEventType:
		entries = append(entries,
			eventMarshaler{
				field:      "syscall",
				marshalFnc: eventMarshalJSON(&e.Utimes.BaseEvent),
			},
			eventMarshaler{
				field:      "file",
				marshalFnc: e.Utimes.marshalJSON,
			})
	case FileLinkEventType:
		entries = append(entries,
			eventMarshaler{
				field:      "syscall",
				marshalFnc: eventMarshalJSON(&e.Link.BaseEvent),
			},
			eventMarshaler{
				field:      "source",
				marshalFnc: e.Link.Source.marshalJSON,
			},
			eventMarshaler{
				field:      "target",
				marshalFnc: e.Link.Target.marshalJSON,
			})
	case FileMountEventType:
		entries = append(entries,
			eventMarshaler{
				field:      "syscall",
				marshalFnc: eventMarshalJSON(&e.Mount.BaseEvent),
			},
			eventMarshaler{
				field:      "mount",
				marshalFnc: e.Mount.marshalJSON,
			})
	case FileUmountEventType:
		entries = append(entries,
			eventMarshaler{
				field:      "syscall",
				marshalFnc: eventMarshalJSON(&e.Umount.BaseEvent),
			},
			eventMarshaler{
				field:      "umount",
				marshalFnc: e.Umount.marshalJSON,
			})
	case FileSetXAttrEventType:
		entries = append(entries,
			eventMarshaler{
				field:      "syscall",
				marshalFnc: eventMarshalJSON(&e.SetXAttr.BaseEvent),
			},
			eventMarshaler{
				field:      "file",
				marshalFnc: e.SetXAttr.marshalJSON,
			})
	case FileRemoveXAttrEventType:
		entries = append(entries,
			eventMarshaler{
				field:      "syscall",
				marshalFnc: eventMarshalJSON(&e.RemoveXAttr.BaseEvent),
			},
			eventMarshaler{
				field:      "file",
				marshalFnc: e.RemoveXAttr.marshalJSON,
			})
	}

	var prev bool
	for _, entry := range entries {
		d, err := entry.marshalFnc(e.resolvers)
		if err != nil {
			return nil, errors.Wrapf(err, "in %s", entry.field)
		}
		if d != nil {
			if prev {
				buf.WriteRune(',')
			}
			buf.WriteString(`"` + entry.field + `":`)
			buf.Write(d)
			prev = true
		}
	}
	buf.WriteRune('}')

	return buf.Bytes(), nil
}

// GetType returns the event type
func (e *Event) GetType() string {
	return EventType(e.Type).String()
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
	if len(data) < 8 {
		return 0, ErrNotEnoughData
	}
	e.Type = byteOrder.Uint64(data[0:8])

	n, err := unmarshalBinary(data[8:], &e.Process, &e.Container)
	return n + 8, err
}

// NewEvent returns a new event
func NewEvent(resolvers *Resolvers) *Event {
	return &Event{
		resolvers: resolvers,
	}
}
