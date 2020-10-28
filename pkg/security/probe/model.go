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
	"os/user"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/google/uuid"
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

func (e *SyscallEvent) marshalJSON(eventType EventType, resolvers *Resolvers) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteRune('{')
	fmt.Fprintf(&buf, `"type":"%s",`, eventType.String())
	fmt.Fprintf(&buf, `"retval":%d`, e.Retval)
	buf.WriteRune('}')

	return buf.Bytes(), nil
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
}

// ResolveInode resolves the inode to a full path
func (e *FileEvent) ResolveInode(resolvers *Resolvers) string {
	if len(e.PathnameStr) == 0 {
		e.PathnameStr = resolvers.DentryResolver.Resolve(e.MountID, e.Inode, e.PathID)
		if e.PathnameStr == dentryPathKeyNotFound {
			return e.PathnameStr
		}

		_, mountPath, rootPath, err := resolvers.MountResolver.GetMountPath(e.MountID)
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
		containerPath, _, _, err := resolvers.MountResolver.GetMountPath(e.MountID)
		if err == nil {
			e.ContainerPath = containerPath
		}
		if len(containerPath) == 0 && len(e.PathnameStr) == 0 {
			// The container path might be included in the pathname. The container path will be set there.
			e.ResolveInode(resolvers)
		}
	}
	return e.ContainerPath
}

// ResolveBasename resolves the inode to a filename
func (e *FileEvent) ResolveBasename(resolvers *Resolvers) string {
	if len(e.BasenameStr) == 0 {
		if e.PathnameStr != "" {
			e.BasenameStr = path.Base(e.PathnameStr)
		} else {
			e.BasenameStr = resolvers.DentryResolver.GetName(e.MountID, e.Inode, e.PathID)
		}
	}
	return e.BasenameStr
}

func (e *FileEvent) marshalJSONInode(resolvers *Resolvers, inode uint64) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteRune('{')
	fmt.Fprintf(&buf, `"filename":"%s",`, e.ResolveInode(resolvers))
	fmt.Fprintf(&buf, `"container_path":"%s",`, e.ResolveContainerPath(resolvers))
	fmt.Fprintf(&buf, `"inode":%d,`, inode)
	fmt.Fprintf(&buf, `"mount_id":%d,`, e.MountID)
	fmt.Fprintf(&buf, `"overlay_numlower":%d`, e.OverlayNumLower)
	buf.WriteRune('}')

	return buf.Bytes(), nil
}

func (e *FileEvent) marshalJSON(resolvers *Resolvers) ([]byte, error) {
	return e.marshalJSONInode(resolvers, e.Inode)
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
	SyscallEvent
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
	n, err := unmarshalBinary(data, &e.SyscallEvent, &e.FileEvent)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 4 {
		return n, ErrNotEnoughData
	}

	e.Mode = int32(ebpf.ByteOrder.Uint32(data[0:4]))
	return n + 4, nil
}

// RmdirEvent represents a rmdir event
type RmdirEvent struct {
	SyscallEvent
	FileEvent
}

func (e *RmdirEvent) marshalJSON(resolvers *Resolvers) ([]byte, error) {
	return e.FileEvent.marshalJSON(resolvers)
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *RmdirEvent) UnmarshalBinary(data []byte) (int, error) {
	return unmarshalBinary(data, &e.SyscallEvent, &e.FileEvent)
}

// UnlinkEvent represents an unlink event
type UnlinkEvent struct {
	SyscallEvent
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
	n, err := unmarshalBinary(data, &e.SyscallEvent, &e.FileEvent)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 4 {
		return 0, ErrNotEnoughData
	}

	e.Flags = ebpf.ByteOrder.Uint32(data[0:4])
	return n + 4, nil
}

// RenameEvent represents a rename event
type RenameEvent struct {
	SyscallEvent
	Old FileEvent `field:"old"`
	New FileEvent `field:"new"`
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *RenameEvent) UnmarshalBinary(data []byte) (int, error) {
	return unmarshalBinary(data, &e.SyscallEvent, &e.Old, &e.New)
}

func (e *RenameEvent) marshalJSON(resolvers *Resolvers) ([]byte, error) {
	var buf bytes.Buffer

	// use the new.inode as the old one is a fake one generated from the probe
	buf.WriteString(`"old":`)
	d, err := e.Old.marshalJSONInode(resolvers, e.New.Inode)
	if err != nil {
		return d, err
	}
	buf.Write(d)

	buf.WriteString(`,"new":`)
	d, err = e.New.marshalJSONInode(resolvers, e.New.Inode)
	if err != nil {
		return d, err
	}
	buf.Write(d)

	return buf.Bytes(), nil
}

// UtimesEvent represents a utime event
type UtimesEvent struct {
	SyscallEvent
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
	fmt.Fprintf(&buf, `"overlay_numlower":%d,`, e.OverlayNumLower)
	fmt.Fprintf(&buf, `"access_time":"%s",`, e.Atime)
	fmt.Fprintf(&buf, `"modification_time":"%s"`, e.Mtime)
	buf.WriteRune('}')

	return buf.Bytes(), nil
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

func (e *LinkEvent) marshalJSON(resolvers *Resolvers) ([]byte, error) {
	var buf bytes.Buffer

	// use the source.inode as the target one is a fake one generated from the probe
	buf.WriteString(`"source":`)
	d, err := e.Source.marshalJSONInode(resolvers, e.Source.Inode)
	if err != nil {
		return d, err
	}
	buf.Write(d)

	buf.WriteString(`,"target":`)
	d, err = e.Target.marshalJSONInode(resolvers, e.Source.Inode)
	if err != nil {
		return d, err
	}
	buf.Write(d)

	return buf.Bytes(), nil
}

// MountEvent represents a mount event
type MountEvent struct {
	SyscallEvent
	MountID       uint32
	GroupID       uint32
	Device        uint32
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
	fmt.Fprintf(&buf, `"mount_id":%d,`, e.MountID)
	fmt.Fprintf(&buf, `"group_id":%d,`, e.GroupID)
	fmt.Fprintf(&buf, `"device":%d,`, e.Device)
	fmt.Fprintf(&buf, `"fstype":"%s"`, e.GetFSType())
	buf.WriteRune('}')

	return buf.Bytes(), nil
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
func (e *MountEvent) ResolveMountPoint(resolvers *Resolvers) string {
	if len(e.MountPointStr) == 0 {
		e.MountPointStr = resolvers.DentryResolver.Resolve(e.ParentMountID, e.ParentInode, 0)
	}
	return e.MountPointStr
}

// ResolveRoot resolves the mountpoint to a full path
func (e *MountEvent) ResolveRoot(resolvers *Resolvers) string {
	if len(e.RootStr) == 0 {
		e.RootStr = resolvers.DentryResolver.Resolve(e.RootMountID, e.RootInode, 0)
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
	n, err := unmarshalBinary(data, &e.SyscallEvent)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 4 {
		return 0, ErrNotEnoughData
	}

	e.MountID = ebpf.ByteOrder.Uint32(data[0:4])
	return 4, nil
}

// ContainerEvent holds the container context of an event
type ContainerEvent struct {
	ID string `field:"id" handler:"ResolveContainerID,string"`

	IDRaw [64]byte `field:"-"`
}

func (e *ContainerEvent) marshalJSON(resolvers *Resolvers) ([]byte, error) {
	if len(e.GetContainerID()) == 0 {
		return nil, nil
	}

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
	utils.SliceToArray(data[0:64], unsafe.Pointer(&e.IDRaw))

	return 64, nil
}

// Bytes returns a binary representation of itself
func (e *ContainerEvent) Bytes() []byte {
	return utils.ContainerID(e.ID).Bytes()
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

// ExecEvent represents a exec event
type ExecEvent struct {
	ProcessCacheEntry
	Pid uint32
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *ExecEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 96 {
		return 0, ErrNotEnoughData
	}

	var offset int
	read, err := e.ProcessCacheEntry.UnmarshalBinary(data)
	if err != nil {
		return read, err
	}
	offset += read

	e.Pid = ebpf.ByteOrder.Uint32(data[offset : offset+4])

	// 4 of padding
	return offset + 8, nil
}

// ExitEvent represents a exit event
type ExitEvent struct {
	Pid uint32
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *ExitEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 4 {
		return 0, ErrNotEnoughData
	}

	e.Pid = ebpf.ByteOrder.Uint32(data)

	return 4, nil
}

// InvalidateDentryEvent defines a invalidate dentry event
type InvalidateDentryEvent struct {
	Inode   uint64
	MountID uint32
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *InvalidateDentryEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 16 {
		return 0, ErrNotEnoughData
	}

	e.Inode = ebpf.ByteOrder.Uint64(data[0:8])
	e.MountID = ebpf.ByteOrder.Uint32(data[8:12])

	// 4 of padding

	return 16, nil
}

// ProcessEvent holds the process context of an event
type ProcessEvent struct {
	FileEvent
	Comm      string    `field:"name" handler:"ResolveComm,string"`
	TTYName   string    `field:"tty_name" handler:"ResolveTTY,string"`
	Pid       uint32    `field:"pid"`
	Tid       uint32    `field:"tid"`
	UID       uint32    `field:"uid"`
	GID       uint32    `field:"gid"`
	User      string    `field:"user" handler:"ResolveUser,string"`
	Group     string    `field:"group" handler:"ResolveGroup,string"`
	Timestamp time.Time `field:"-" handler:"ResolveTimestamp,string"`

	CommRaw [16]byte `field:"-"`
}

// ResolveTimestamp converts a raw timestamp to a time object
func (p *ProcessEvent) ResolveTimestamp(resolvers *Resolvers) time.Time {
	if p.Timestamp.IsZero() {
		entry := resolvers.ProcessResolver.Resolve(p.Pid)
		if entry == nil {
			return time.Time{}
		}

		p.Timestamp = entry.Timestamp
	}

	return p.Timestamp
}

// ResolveInode resolves the inode to a full path
func (p *ProcessEvent) ResolveInode(resolvers *Resolvers) string {
	if p.PathnameStr == "" {
		if entry := resolvers.ProcessResolver.Resolve(p.Pid); entry != nil {
			p.PathnameStr = entry.ResolveInode(resolvers)
		}
	}

	return p.PathnameStr
}

// ResolveContainerPath resolves the inode to a path relative to the container
func (p *ProcessEvent) ResolveContainerPath(resolvers *Resolvers) string {
	if p.ContainerPath == "" {
		if entry := resolvers.ProcessResolver.Resolve(p.Pid); entry != nil {
			p.ContainerPath = entry.ResolveContainerPath(resolvers)
		}
	}

	return p.ContainerPath
}

// ResolveBasename resolves the inode to a filename
func (p *ProcessEvent) ResolveBasename(resolvers *Resolvers) string {
	if len(p.BasenameStr) == 0 {
		if p.PathnameStr == "" {
			p.PathnameStr = p.ResolveInode(resolvers)
		}

		p.BasenameStr = path.Base(p.PathnameStr)
	}
	return p.BasenameStr
}

func (p *ProcessEvent) marshalJSON(resolvers *Resolvers) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteRune('{')
	fmt.Fprintf(&buf, `"name":"%s",`, p.ResolveComm(resolvers))
	if tty := p.ResolveTTY(resolvers); tty != "" {
		fmt.Fprintf(&buf, `"tty_name":"%s",`, tty)
	}
	fmt.Fprintf(&buf, `"pid":%d,`, p.Pid)
	fmt.Fprintf(&buf, `"tid":%d,`, p.Tid)
	fmt.Fprintf(&buf, `"uid":%d,`, p.UID)
	fmt.Fprintf(&buf, `"gid":%d,`, p.GID)
	fmt.Fprintf(&buf, `"filename":"%s",`, p.ResolveInode(resolvers))
	fmt.Fprintf(&buf, `"container_path":"%s",`, p.ResolveContainerPath(resolvers))
	fmt.Fprintf(&buf, `"inode":%d,`, p.Inode)
	fmt.Fprintf(&buf, `"mount_id":%d,`, p.MountID)
	fmt.Fprintf(&buf, `"overlay_numlower":%d,`, p.OverlayNumLower)
	fmt.Fprintf(&buf, `"timestamp":"%s"`, p.ResolveTimestamp(resolvers))
	buf.WriteRune('}')

	return buf.Bytes(), nil
}

// ResolveTTY resolves the name of the process tty
func (p *ProcessEvent) ResolveTTY(resolvers *Resolvers) string {
	if p.TTYName == "" {
		if entry := resolvers.ProcessResolver.Resolve(p.Pid); entry != nil {
			p.TTYName = entry.GetTTY()
		}
	}

	return p.TTYName
}

// ResolveComm resolves the comm of the process
func (p *ProcessEvent) ResolveComm(resolvers *Resolvers) string {
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
	if len(data) < 32 {
		return 0, ErrNotEnoughData
	}

	utils.SliceToArray(data[0:16], unsafe.Pointer(&p.CommRaw))

	p.Pid = ebpf.ByteOrder.Uint32(data[16:20])
	p.Tid = ebpf.ByteOrder.Uint32(data[20:24])
	p.UID = ebpf.ByteOrder.Uint32(data[24:28])
	p.GID = ebpf.ByteOrder.Uint32(data[28:32])

	return 32, nil
}

// Event represents an event sent from the kernel
// genaccessors
type Event struct {
	ID           string    `field:"-"`
	Type         uint64    `field:"-"`
	TimestampRaw uint64    `field:"-"`
	Timestamp    time.Time `field:"timestamp"`

	Process          ProcessEvent          `yaml:"process" field:"process" event:"*"`
	Container        ContainerEvent        `yaml:"container" field:"container"`
	Chmod            ChmodEvent            `yaml:"chmod" field:"chmod" event:"chmod"`
	Chown            ChownEvent            `yaml:"chown" field:"chown" event:"chown"`
	Open             OpenEvent             `yaml:"open" field:"open" event:"open"`
	Mkdir            MkdirEvent            `yaml:"mkdir" field:"mkdir" event:"mkdir"`
	Rmdir            RmdirEvent            `yaml:"rmdir" field:"rmdir" event:"rmdir"`
	Rename           RenameEvent           `yaml:"rename" field:"rename" event:"rename"`
	Unlink           UnlinkEvent           `yaml:"unlink" field:"unlink" event:"unlink"`
	Utimes           UtimesEvent           `yaml:"utimes" field:"utimes" event:"utimes"`
	Link             LinkEvent             `yaml:"link" field:"link" event:"link"`
	SetXAttr         SetXAttrEvent         `yaml:"setxattr" field:"setxattr" event:"setxattr"`
	RemoveXAttr      SetXAttrEvent         `yaml:"removexattr" field:"removexattr" event:"removexattr"`
	Mount            MountEvent            `field:"-"`
	Umount           UmountEvent           `field:"-"`
	Exec             ExecEvent             `field:"-"`
	Exit             ExitEvent             `field:"-"`
	InvalidateDentry InvalidateDentryEvent `field:"-"`

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

// ResolveMonotonicTimestamp resolves the monolitic kernel timestamp to an absolute time
func (e *Event) ResolveMonotonicTimestamp(resolvers *Resolvers) time.Time {
	if e.Timestamp.IsZero() {
		e.Timestamp = resolvers.TimeResolver.ResolveMonotonicTimestamp(e.TimestampRaw)
	}
	return e.Timestamp
}

// MarshalJSON returns the JSON encoding of the event
func (e *Event) MarshalJSON() ([]byte, error) {
	eventID, _ := uuid.NewRandom()

	var buf bytes.Buffer
	buf.WriteRune('{')
	fmt.Fprintf(&buf, `"id":"%s",`, eventID)
	fmt.Fprintf(&buf, `"timestamp":"%s"`, e.ResolveMonotonicTimestamp(e.resolvers))

	var entries []eventMarshaler

	eventType := EventType(e.Type)

	eventMarshalJSON := func(e *SyscallEvent) func(*Resolvers) ([]byte, error) {
		return func(resolvers *Resolvers) ([]byte, error) {
			return e.marshalJSON(eventType, resolvers)
		}
	}

	switch eventType {
	case FileChmodEventType:
		entries = append(entries,
			eventMarshaler{
				field:      "syscall",
				marshalFnc: eventMarshalJSON(&e.Chmod.SyscallEvent),
			},
			eventMarshaler{
				field:      "process",
				marshalFnc: e.Process.marshalJSON,
			},
			eventMarshaler{
				field:      "container",
				marshalFnc: e.Container.marshalJSON,
			},
			eventMarshaler{
				field:      "file",
				marshalFnc: e.Chmod.marshalJSON,
			})
	case FileChownEventType:
		entries = append(entries,
			eventMarshaler{
				field:      "syscall",
				marshalFnc: eventMarshalJSON(&e.Chown.SyscallEvent),
			},
			eventMarshaler{
				field:      "process",
				marshalFnc: e.Process.marshalJSON,
			},
			eventMarshaler{
				field:      "container",
				marshalFnc: e.Container.marshalJSON,
			},
			eventMarshaler{
				field:      "file",
				marshalFnc: e.Chown.marshalJSON,
			})
	case FileOpenEventType:
		entries = append(entries,
			eventMarshaler{
				field:      "syscall",
				marshalFnc: eventMarshalJSON(&e.Open.SyscallEvent),
			},
			eventMarshaler{
				field:      "process",
				marshalFnc: e.Process.marshalJSON,
			},
			eventMarshaler{
				field:      "container",
				marshalFnc: e.Container.marshalJSON,
			},
			eventMarshaler{
				field:      "file",
				marshalFnc: e.Open.marshalJSON,
			})
	case FileMkdirEventType:
		entries = append(entries,
			eventMarshaler{
				field:      "syscall",
				marshalFnc: eventMarshalJSON(&e.Mkdir.SyscallEvent),
			},
			eventMarshaler{
				field:      "process",
				marshalFnc: e.Process.marshalJSON,
			},
			eventMarshaler{
				field:      "container",
				marshalFnc: e.Container.marshalJSON,
			},
			eventMarshaler{
				field:      "file",
				marshalFnc: e.Mkdir.marshalJSON,
			})
	case FileRmdirEventType:
		entries = append(entries,
			eventMarshaler{
				field:      "syscall",
				marshalFnc: eventMarshalJSON(&e.Rmdir.SyscallEvent),
			},
			eventMarshaler{
				field:      "process",
				marshalFnc: e.Process.marshalJSON,
			},
			eventMarshaler{
				field:      "container",
				marshalFnc: e.Container.marshalJSON,
			},
			eventMarshaler{
				field:      "file",
				marshalFnc: e.Rmdir.marshalJSON,
			})
	case FileUnlinkEventType:
		entries = append(entries,
			eventMarshaler{
				field:      "syscall",
				marshalFnc: eventMarshalJSON(&e.Unlink.SyscallEvent),
			},
			eventMarshaler{
				field:      "process",
				marshalFnc: e.Process.marshalJSON,
			},
			eventMarshaler{
				field:      "container",
				marshalFnc: e.Container.marshalJSON,
			},
			eventMarshaler{
				field:      "file",
				marshalFnc: e.Unlink.marshalJSON,
			})
	case FileRenameEventType:
		entries = append(entries,
			eventMarshaler{
				field:      "syscall",
				marshalFnc: eventMarshalJSON(&e.Rename.SyscallEvent),
			},
			eventMarshaler{
				marshalFnc: e.Rename.marshalJSON,
			})
	case FileUtimeEventType:
		entries = append(entries,
			eventMarshaler{
				field:      "syscall",
				marshalFnc: eventMarshalJSON(&e.Utimes.SyscallEvent),
			},
			eventMarshaler{
				field:      "process",
				marshalFnc: e.Process.marshalJSON,
			},
			eventMarshaler{
				field:      "container",
				marshalFnc: e.Container.marshalJSON,
			},
			eventMarshaler{
				field:      "file",
				marshalFnc: e.Utimes.marshalJSON,
			})
	case FileLinkEventType:
		entries = append(entries,
			eventMarshaler{
				field:      "syscall",
				marshalFnc: eventMarshalJSON(&e.Link.SyscallEvent),
			},
			eventMarshaler{
				marshalFnc: e.Link.marshalJSON,
			})
	case FileMountEventType:
		entries = append(entries,
			eventMarshaler{
				field:      "syscall",
				marshalFnc: eventMarshalJSON(&e.Mount.SyscallEvent),
			},
			eventMarshaler{
				field:      "mount",
				marshalFnc: e.Mount.marshalJSON,
			})
	case FileUmountEventType:
		entries = append(entries,
			eventMarshaler{
				field:      "syscall",
				marshalFnc: eventMarshalJSON(&e.Umount.SyscallEvent),
			},
			eventMarshaler{
				field:      "umount",
				marshalFnc: e.Umount.marshalJSON,
			})
	case FileSetXAttrEventType:
		entries = append(entries,
			eventMarshaler{
				field:      "syscall",
				marshalFnc: eventMarshalJSON(&e.SetXAttr.SyscallEvent),
			},
			eventMarshaler{
				field:      "file",
				marshalFnc: e.SetXAttr.marshalJSON,
			})
	case FileRemoveXAttrEventType:
		entries = append(entries,
			eventMarshaler{
				field:      "syscall",
				marshalFnc: eventMarshalJSON(&e.RemoveXAttr.SyscallEvent),
			},
			eventMarshaler{
				field:      "file",
				marshalFnc: e.RemoveXAttr.marshalJSON,
			})
	}

	for _, entry := range entries {
		d, err := entry.marshalFnc(e.resolvers)
		if err != nil {
			return nil, errors.Wrapf(err, "in %s", entry.field)
		}
		if d != nil {
			buf.WriteRune(',')
			if entry.field != "" {
				buf.WriteString(`"` + entry.field + `":`)
			}
			buf.Write(d)
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
	if len(data) < 16 {
		return 0, ErrNotEnoughData
	}
	e.Type = ebpf.ByteOrder.Uint64(data[0:8])
	e.TimestampRaw = ebpf.ByteOrder.Uint64(data[8:16])

	return 16, nil
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
