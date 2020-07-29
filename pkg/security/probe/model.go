// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

//go:generate go run github.com/DataDog/datadog-agent/pkg/security/secl/generators/accessors -tags linux_bpf -output model_accessors.go

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

	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/google/uuid"
	"github.com/pkg/errors"
)

// ErrNotEnoughData is returned when the buffer is too small to unmarshal the event
var ErrNotEnoughData = errors.New("not enough data")

// Model describes the data model for the runtime security agent events
type Model struct {
	event *Event
}

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
			if value != path.Clean(value) || strings.HasPrefix(value, "..") {
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

// ChmodEvent represents a chmod event
type ChmodEvent struct {
	Mode            int32  `field:"mode"`
	MountID         uint32 `field:"-"`
	Inode           uint64 `field:"inode"`
	OverlayNumLower int32  `field:"overlay_num_lower"`
	PathnameStr     string `field:"filename" handler:"ResolveInode,string"`
	ContainerPath   string `field:"container_path" handler:"ResolveContainerPath,string"`
}

func (e *ChmodEvent) marshalJSON(resolvers *Resolvers) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteRune('{')
	fmt.Fprintf(&buf, `"filename":"%s",`, e.ResolveInode(resolvers))
	fmt.Fprintf(&buf, `"container_path":"%s",`, e.ResolveContainerPath(resolvers))
	fmt.Fprintf(&buf, `"inode":%d,`, e.Inode)
	fmt.Fprintf(&buf, `"mount_id":%d,`, e.MountID)
	fmt.Fprintf(&buf, `"overlay_numlower":%d,`, e.OverlayNumLower)
	fmt.Fprintf(&buf, `"mode":"%s"`, ChmodMode(e.Mode))
	buf.WriteRune('}')

	return buf.Bytes(), nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *ChmodEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 20 {
		return 0, ErrNotEnoughData
	}
	e.Mode = int32(byteOrder.Uint32(data[0:4]))
	e.MountID = byteOrder.Uint32(data[4:8])
	e.Inode = byteOrder.Uint64(data[8:16])
	e.OverlayNumLower = int32(byteOrder.Uint32(data[16:20]))
	return 20, nil
}

// ResolveInode resolves the inode to a full path
func (e *ChmodEvent) ResolveInode(resolvers *Resolvers) string {
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
func (e *ChmodEvent) ResolveContainerPath(resolvers *Resolvers) string {
	if len(e.ContainerPath) == 0 {
		containerPath, _, _, err := resolvers.MountResolver.GetMountPath(e.MountID, e.OverlayNumLower)
		if err == nil {
			e.ContainerPath = containerPath
		}
	}
	return e.ContainerPath
}

// ChownEvent represents a chown event
type ChownEvent struct {
	UID             int32  `field:"uid"`
	GID             int32  `field:"gid"`
	MountID         uint32 `field:"-"`
	Inode           uint64 `field:"inode"`
	OverlayNumLower int32  `field:"overlay_num_lower"`
	PathnameStr     string `field:"filename" handler:"ResolveInode,string"`
	ContainerPath   string `field:"container_path" handler:"ResolveContainerPath,string"`
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
	if len(data) < 28 {
		return 0, ErrNotEnoughData
	}
	e.UID = int32(byteOrder.Uint32(data[0:4]))
	e.GID = int32(byteOrder.Uint32(data[4:8]))
	e.MountID = byteOrder.Uint32(data[12:16])
	e.Inode = byteOrder.Uint64(data[16:24])
	e.OverlayNumLower = int32(byteOrder.Uint32(data[24:28]))
	return 28, nil
}

// ResolveInode resolves the inode to a full path
func (e *ChownEvent) ResolveInode(resolvers *Resolvers) string {
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
func (e *ChownEvent) ResolveContainerPath(resolvers *Resolvers) string {
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

// OpenEvent represents an open event
type OpenEvent struct {
	Flags           uint32 `field:"flags"`
	Mode            uint32 `field:"mode"`
	Inode           uint64 `field:"inode"`
	MountID         uint32 `field:"-"`
	OverlayNumLower int32  `field:"overlay_num_lower"`
	PathnameStr     string `field:"filename" handler:"ResolveInode,string"`
	ContainerPath   string `field:"container_path" handler:"ResolveContainerPath,string"`
	BasenameStr     string `field:"basename" handler:"ResolveBasename,string"`
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

// ResolveInode resolves the inode to a full path
func (e *OpenEvent) ResolveInode(resolvers *Resolvers) string {
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
func (e *OpenEvent) ResolveContainerPath(resolvers *Resolvers) string {
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
func (e *OpenEvent) ResolveBasename(resolvers *Resolvers) string {
	if len(e.BasenameStr) == 0 {
		e.BasenameStr = resolvers.DentryResolver.GetName(e.MountID, e.Inode)
	}
	return e.BasenameStr
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *OpenEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 24 {
		return 0, ErrNotEnoughData
	}
	e.Flags = byteOrder.Uint32(data[0:4])
	e.Mode = byteOrder.Uint32(data[4:8])
	e.Inode = byteOrder.Uint64(data[8:16])
	e.MountID = byteOrder.Uint32(data[16:20])
	e.OverlayNumLower = int32(byteOrder.Uint32(data[20:24]))
	return 24, nil
}

// MkdirEvent represents a mkdir event
type MkdirEvent struct {
	Mode            int32  `field:"mode"`
	MountID         uint32 `field:"-"`
	Inode           uint64 `field:"inode"`
	OverlayNumLower int32  `field:"overlay_num_lower"`
	PathnameStr     string `field:"filename" handler:"ResolveInode,string"`
	ContainerPath   string `field:"container_path" handler:"ResolveContainerPath,string"`
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
	if len(data) < 20 {
		return 0, ErrNotEnoughData
	}
	e.Mode = int32(byteOrder.Uint32(data[0:4]))
	e.MountID = byteOrder.Uint32(data[4:8])
	e.Inode = byteOrder.Uint64(data[8:16])
	e.OverlayNumLower = int32(byteOrder.Uint32(data[16:20]))
	return 20, nil
}

// ResolveInode resolves the inode to a full path
func (e *MkdirEvent) ResolveInode(resolvers *Resolvers) string {
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
func (e *MkdirEvent) ResolveContainerPath(resolvers *Resolvers) string {
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

// RmdirEvent represents a rmdir event
type RmdirEvent struct {
	MountID         uint32 `field:"-"`
	Inode           uint64 `field:"inode"`
	OverlayNumLower int32  `field:"overlay_num_lower"`
	PathnameStr     string `field:"filename" handler:"ResolveInode,string"`
	ContainerPath   string `field:"container_path" handler:"ResolveContainerPath,string"`
}

func (e *RmdirEvent) marshalJSON(resolvers *Resolvers) ([]byte, error) {
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

// ResolveInode resolves the inode to a full path
func (e *RmdirEvent) ResolveInode(resolvers *Resolvers) string {
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
func (e *RmdirEvent) ResolveContainerPath(resolvers *Resolvers) string {
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

// UnmarshalBinary unmarshals a binary representation of itself
func (e *RmdirEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 16 {
		return 0, ErrNotEnoughData
	}
	e.Inode = byteOrder.Uint64(data[0:8])
	e.MountID = byteOrder.Uint32(data[8:12])
	e.OverlayNumLower = int32(byteOrder.Uint32(data[12:16]))
	return 16, nil
}

// UnlinkEvent represents an unlink event
type UnlinkEvent struct {
	Inode           uint64 `field:"inode"`
	MountID         uint32 `field:"-"`
	OverlayNumLower int32  `field:"overlay_num_lower"`
	PathnameStr     string `field:"filename" handler:"ResolveInode,string"`
	ContainerPath   string `field:"container_path" handler:"ResolveContainerPath,string"`
	Flags           uint32 `field:"flags"`
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
	if len(data) < 20 {
		return 0, ErrNotEnoughData
	}
	e.Inode = byteOrder.Uint64(data[0:8])
	e.MountID = byteOrder.Uint32(data[8:12])
	e.OverlayNumLower = int32(byteOrder.Uint32(data[12:16]))
	e.Flags = byteOrder.Uint32(data[16:20])

	return 20, nil
}

// ResolveInode resolves the inode to a full path
func (e *UnlinkEvent) ResolveInode(resolvers *Resolvers) string {
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
func (e *UnlinkEvent) ResolveContainerPath(resolvers *Resolvers) string {
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

// RenameEvent represents a rename event
type RenameEvent struct {
	SrcMountID            uint32 `field:"-"`
	SrcInode              uint64 `field:"old_inode"`
	SrcRandomInode        uint64 `field:"-"`
	SrcPathnameStr        string `field:"old_filename" handler:"ResolveSrcInode,string"`
	SrcContainerPath      string `field:"src_container_path" handler:"ResolveSrcContainerPath,string"`
	SrcOverlayNumLower    int32  `field:"src_overlay_num_lower"`
	TargetMountID         uint32 `field:"-"`
	TargetInode           uint64 `field:"new_inode"`
	TargetPathnameStr     string `field:"new_filename" handler:"ResolveTargetInode,string"`
	TargetContainerPath   string `field:"target_container_path" handler:"ResolveTargetContainerPath,string"`
	TargetOverlayNumLower int32  `field:"target_overlay_num_lower"`
}

func (e *RenameEvent) marshalJSON(resolvers *Resolvers) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteRune('{')
	fmt.Fprintf(&buf, `"old_mount_id":%d,`, e.SrcMountID)
	fmt.Fprintf(&buf, `"old_inode":%d,`, e.SrcInode)
	fmt.Fprintf(&buf, `"old_random_inode":%d,`, e.SrcRandomInode)
	fmt.Fprintf(&buf, `"old_filename":"%s",`, e.ResolveSrcInode(resolvers))
	fmt.Fprintf(&buf, `"old_container_path":"%s",`, e.ResolveSrcContainerPath(resolvers))
	fmt.Fprintf(&buf, `"old_overlay_numlower":%d,`, e.SrcOverlayNumLower)
	fmt.Fprintf(&buf, `"new_mount_id":%d,`, e.TargetMountID)
	fmt.Fprintf(&buf, `"new_inode":%d,`, e.TargetInode)
	fmt.Fprintf(&buf, `"new_filename":"%s",`, e.ResolveTargetInode(resolvers))
	fmt.Fprintf(&buf, `"new_container_path":"%s",`, e.ResolveTargetContainerPath(resolvers))
	fmt.Fprintf(&buf, `"new_overlay_numlower":%d`, e.TargetOverlayNumLower)
	buf.WriteRune('}')

	return buf.Bytes(), nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *RenameEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 44 {
		return 0, ErrNotEnoughData
	}
	e.SrcMountID = byteOrder.Uint32(data[0:4])
	// padding
	e.SrcInode = byteOrder.Uint64(data[8:16])
	e.SrcRandomInode = byteOrder.Uint64(data[16:24])
	e.TargetInode = byteOrder.Uint64(data[24:32])
	e.TargetMountID = byteOrder.Uint32(data[32:36])
	e.SrcOverlayNumLower = int32(byteOrder.Uint32(data[36:40]))
	e.TargetOverlayNumLower = int32(byteOrder.Uint32(data[40:44]))
	return 44, nil
}

// ResolveSrcInode resolves the source inode to a full path
func (e *RenameEvent) ResolveSrcInode(resolvers *Resolvers) string {
	if len(e.SrcPathnameStr) == 0 {
		e.SrcPathnameStr = resolvers.DentryResolver.Resolve(e.SrcMountID, e.SrcRandomInode)
		_, mountPath, rootPath, err := resolvers.MountResolver.GetMountPath(e.SrcMountID, e.SrcOverlayNumLower)
		if err == nil {
			if strings.HasPrefix(e.SrcPathnameStr, rootPath) && rootPath != "/" {
				e.SrcPathnameStr = strings.Replace(e.SrcPathnameStr, rootPath, "", 1)
			}
			e.SrcPathnameStr = path.Join(mountPath, e.SrcPathnameStr)
		}
	}
	return e.SrcPathnameStr
}

// ResolveSrcContainerPath resolves the source inode to a path relative to the container
func (e *RenameEvent) ResolveSrcContainerPath(resolvers *Resolvers) string {
	if len(e.SrcContainerPath) == 0 {
		containerPath, _, _, err := resolvers.MountResolver.GetMountPath(e.SrcMountID, e.SrcOverlayNumLower)
		if err == nil {
			e.SrcContainerPath = containerPath
		}
		if len(containerPath) == 0 && len(e.SrcPathnameStr) == 0 {
			// The container path might be included in the pathname. The container path will be set there.
			_ = e.ResolveSrcInode(resolvers)
		}
	}
	return e.SrcContainerPath
}

// ResolveTargetInode resolves the target inode to a full path
func (e *RenameEvent) ResolveTargetInode(resolvers *Resolvers) string {
	if len(e.TargetPathnameStr) == 0 {
		e.TargetPathnameStr = resolvers.DentryResolver.Resolve(e.TargetMountID, e.TargetInode)
		_, mountPath, rootPath, err := resolvers.MountResolver.GetMountPath(e.TargetMountID, e.TargetOverlayNumLower)
		if err == nil {
			if strings.HasPrefix(e.TargetPathnameStr, rootPath) && rootPath != "/" {
				e.TargetPathnameStr = strings.Replace(e.TargetPathnameStr, rootPath, "", 1)
			}
			e.TargetPathnameStr = path.Join(mountPath, e.TargetPathnameStr)
		}
	}
	return e.TargetPathnameStr
}

// ResolveTargetContainerPath resolves the inode to a path relative to the container
func (e *RenameEvent) ResolveTargetContainerPath(resolvers *Resolvers) string {
	if len(e.TargetContainerPath) == 0 {
		containerPath, _, _, err := resolvers.MountResolver.GetMountPath(e.TargetMountID, e.TargetOverlayNumLower)
		if err == nil {
			e.TargetContainerPath = containerPath
		}
		if len(containerPath) == 0 && len(e.TargetPathnameStr) == 0 {
			// The container path might be included in the pathname. The container path will be set there.
			_ = e.ResolveTargetInode(resolvers)
		}
	}
	return e.TargetContainerPath
}

// UtimesEvent represents a utime event
type UtimesEvent struct {
	Atime           time.Time
	Mtime           time.Time
	Inode           uint64 `field:"inode"`
	MountID         uint32 `field:"-"`
	OverlayNumLower int32  `field:"overlay_num_lower"`
	PathnameStr     string `field:"filename" handler:"ResolveInode,string"`
	ContainerPath   string `field:"container_path" handler:"ResolveContainerPath,string"`
}

func (e *UtimesEvent) marshalJSON(resolvers *Resolvers) ([]byte, error) {
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
func (e *UtimesEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 52 {
		return 0, ErrNotEnoughData
	}

	timeSec := byteOrder.Uint64(data[0:8])
	timeNsec := byteOrder.Uint64(data[8:16])
	e.Atime = time.Unix(int64(timeSec), int64(timeNsec))

	timeSec = byteOrder.Uint64(data[16:24])
	timeNsec = byteOrder.Uint64(data[24:32])
	e.Mtime = time.Unix(int64(timeSec), int64(timeNsec))

	e.MountID = byteOrder.Uint32(data[36:40])
	e.Inode = byteOrder.Uint64(data[40:48])
	e.OverlayNumLower = int32(byteOrder.Uint32(data[48:52]))

	return 52, nil
}

// ResolveInode resolves the inode to a full path
func (e *UtimesEvent) ResolveInode(resolvers *Resolvers) string {
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
func (e *UtimesEvent) ResolveContainerPath(resolvers *Resolvers) string {
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

// LinkEvent represents a link event
type LinkEvent struct {
	SrcMountID         uint32 `field:"-"`
	SrcInode           uint64 `field:"src_inode"`
	SrcPathnameStr     string `field:"src_filename" handler:"ResolveSrcInode,string"`
	SrcContainerPath   string `field:"src_container_path" handler:"ResolveSrcContainerPath,string"`
	SrcOverlayNumLower int32  `field:"src_overlay_num_lower"`
	NewMountID         uint32 `field:"-"`
	NewInode           uint64 `field:"new_inode"`
	NewPathnameStr     string `field:"new_filename" handler:"ResolveNewInode,string"`
	NewContainerPath   string `field:"new_container_path" handler:"ResolveNewContainerPath,string"`
	NewOverlayNumLower int32  `field:"new_overlay_num_lower"`
}

func (e *LinkEvent) marshalJSON(resolvers *Resolvers) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteRune('{')
	fmt.Fprintf(&buf, `"src_mount_id":%d,`, e.SrcMountID)
	fmt.Fprintf(&buf, `"src_inode":%d,`, e.SrcInode)
	fmt.Fprintf(&buf, `"src_filename":"%s",`, e.ResolveSrcInode(resolvers))
	fmt.Fprintf(&buf, `"src_container_path":"%s",`, e.ResolveSrcContainerPath(resolvers))
	fmt.Fprintf(&buf, `"src_overlay_numlower":%d,`, e.SrcOverlayNumLower)
	fmt.Fprintf(&buf, `"new_mount_id":%d,`, e.NewMountID)
	fmt.Fprintf(&buf, `"new_inode":%d,`, e.NewInode)
	fmt.Fprintf(&buf, `"new_filename":"%s",`, e.ResolveNewInode(resolvers))
	fmt.Fprintf(&buf, `"new_container_path":"%s",`, e.ResolveNewContainerPath(resolvers))
	fmt.Fprintf(&buf, `"new_overlay_numlower":%d`, e.NewOverlayNumLower)
	buf.WriteRune('}')

	return buf.Bytes(), nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *LinkEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 36 {
		return 0, ErrNotEnoughData
	}
	e.SrcMountID = byteOrder.Uint32(data[0:4])
	// padding
	e.SrcInode = byteOrder.Uint64(data[8:16])
	e.NewInode = byteOrder.Uint64(data[16:24])
	e.NewMountID = byteOrder.Uint32(data[24:28])
	e.SrcOverlayNumLower = int32(byteOrder.Uint32(data[28:32]))
	e.NewOverlayNumLower = int32(byteOrder.Uint32(data[32:36]))
	return 36, nil
}

// ResolveSrcInode resolves the source inode to a full path
func (e *LinkEvent) ResolveSrcInode(resolvers *Resolvers) string {
	if len(e.SrcPathnameStr) == 0 {
		e.SrcPathnameStr = resolvers.DentryResolver.Resolve(e.SrcMountID, e.SrcInode)
		_, mountPath, rootPath, err := resolvers.MountResolver.GetMountPath(e.SrcMountID, e.SrcOverlayNumLower)
		if err == nil {
			if strings.HasPrefix(e.SrcPathnameStr, rootPath) && rootPath != "/" {
				e.SrcPathnameStr = strings.Replace(e.SrcPathnameStr, rootPath, "", 1)
			}
			e.SrcPathnameStr = path.Join(mountPath, e.SrcPathnameStr)
		}
	}
	return e.SrcPathnameStr
}

// ResolveSrcContainerPath resolves the source inode to a path relative to the container
func (e *LinkEvent) ResolveSrcContainerPath(resolvers *Resolvers) string {
	if len(e.SrcContainerPath) == 0 {
		containerPath, _, _, err := resolvers.MountResolver.GetMountPath(e.SrcMountID, e.SrcOverlayNumLower)
		if err == nil {
			e.SrcContainerPath = containerPath
		}
		if len(containerPath) == 0 && len(e.SrcPathnameStr) == 0 {
			// The container path might be included in the pathname. The container path will be set there.
			_ = e.ResolveSrcInode(resolvers)
		}
	}
	return e.SrcContainerPath
}

// ResolveNewInode resolves the target inode to a full path
func (e *LinkEvent) ResolveNewInode(resolvers *Resolvers) string {
	if len(e.NewPathnameStr) == 0 {
		e.NewPathnameStr = resolvers.DentryResolver.Resolve(e.NewMountID, e.NewInode)
		_, mountPath, rootPath, err := resolvers.MountResolver.GetMountPath(e.NewMountID, e.NewOverlayNumLower)
		if err == nil {
			if strings.HasPrefix(e.NewPathnameStr, rootPath) && rootPath != "/" {
				e.NewPathnameStr = strings.Replace(e.NewPathnameStr, rootPath, "", 1)
			}
			e.NewPathnameStr = path.Join(mountPath, e.NewPathnameStr)
		}
	}
	return e.NewPathnameStr
}

// ResolveNewContainerPath resolves the target inode to a path relative to the container
func (e *LinkEvent) ResolveNewContainerPath(resolvers *Resolvers) string {
	if len(e.NewContainerPath) == 0 {
		containerPath, _, _, err := resolvers.MountResolver.GetMountPath(e.NewMountID, e.NewOverlayNumLower)
		if err == nil {
			e.NewContainerPath = containerPath
		}
		if len(containerPath) == 0 && len(e.NewPathnameStr) == 0 {
			// The container path might be included in the pathname. The container path will be set there.
			_ = e.ResolveNewInode(resolvers)
		}
	}
	return e.NewContainerPath
}

// MountEvent represents a mount event
type MountEvent struct {
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
	if len(data) < 4 {
		return 0, ErrNotEnoughData
	}

	e.MountID = byteOrder.Uint32(data[0:4])

	return 8, nil
}

// ContainerEvent holds the container context of an event
type ContainerEvent struct {
	ID     string   `yaml:"id" field:"id" event:"container"`
	Labels []string `yaml:"labels" field:"labels" event:"container"`
}

// KernelEvent describes an event sent from the kernel
type KernelEvent struct {
	Type         uint64    `field:"type" handler:"ResolveType,string"`
	TimestampRaw uint64    `field:"-"`
	Timestamp    time.Time `field:"-"`
	Retval       int64     `field:"retval"`
}

// ResolveType resolves the type of the event to a name
func (k *KernelEvent) ResolveType(resolvers *Resolvers) string {
	return EventType(k.Type).String()
}

func (k *KernelEvent) marshalJSON(resolvers *Resolvers) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteRune('{')
	fmt.Fprintf(&buf, `"type":"%s",`, k.ResolveType(resolvers))
	fmt.Fprintf(&buf, `"timestamp":"%s",`, k.ResolveMonoliticTimestamp(resolvers))
	fmt.Fprintf(&buf, `"retval":%d`, k.Retval)
	if k.Retval < 0 {
		fmt.Fprintf(&buf, `,"error":"%s"`, RetValError(k.Retval))
	}
	buf.WriteRune('}')

	return buf.Bytes(), nil
}

// ResolveMonoliticTimestamp resolves the monolitic kernel timestamp to an absolute time
func (k *KernelEvent) ResolveMonoliticTimestamp(resolvers *Resolvers) time.Time {
	if (k.Timestamp.Equal(time.Time{})) {
		k.Timestamp = resolvers.TimeResolver.ResolveMonotonicTimestamp(k.TimestampRaw)
	}
	return k.Timestamp
}

// UnmarshalBinary unmarshals a binary representation of itself
func (k *KernelEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 24 {
		return 0, ErrNotEnoughData
	}
	k.Type = byteOrder.Uint64(data[0:8])
	k.TimestampRaw = byteOrder.Uint64(data[8:16])
	k.Retval = int64(byteOrder.Uint64(data[16:24]))
	return 24, nil
}

// ProcessEvent holds the process context of an event
type ProcessEvent struct {
	Pidns       uint64 `field:"pidns"`
	Comm        string `field:"name" handler:"ResolveComm,string"`
	TTYName     string `field:"tty_name" handler:"ResolveTTY,string"`
	Pid         uint32 `field:"pid"`
	Tid         uint32 `field:"tid"`
	UID         uint32 `field:"uid"`
	GID         uint32 `field:"gid"`
	User        string `field:"user" handler:"ResolveUser,string"`
	Group       string `field:"group" handler:"ResolveGroup,string"`
	PathnameStr string `field:"filename" handler:"ResolveInode,string"`

	CommRaw    [16]byte `field:"-"`
	TTYNameRaw [64]byte `field:"-"`
	Inode      uint64   `field:"-"`
}

// ResolveInode resolves the executable inode to a path
func (p *ProcessEvent) ResolveInode(resolvers *Resolvers) string {
	if len(p.PathnameStr) == 0 {
		p.PathnameStr = resolvers.DentryResolver.Resolve(0xffffffff, p.Inode)
	}
	return p.PathnameStr
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
	if len(data) < 104 {
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
	return 104, nil
}

// Event represents an event sent from the kernel
// genaccessors
type Event struct {
	ID        string         `yaml:"id" field:"-"`
	Event     KernelEvent    `yaml:"event" field:"event" event:"*"`
	Process   ProcessEvent   `yaml:"process" field:"process" event:"*"`
	Container ContainerEvent `yaml:"container" field:"container"`
	Chmod     ChmodEvent     `yaml:"chmod" field:"chmod" event:"chmod"`
	Chown     ChownEvent     `yaml:"chown" field:"chown" event:"chown"`
	Open      OpenEvent      `yaml:"open" field:"open" event:"open"`
	Mkdir     MkdirEvent     `yaml:"mkdir" field:"mkdir" event:"mkdir"`
	Rmdir     RmdirEvent     `yaml:"rmdir" field:"rmdir" event:"rmdir"`
	Rename    RenameEvent    `yaml:"rename" field:"rename" event:"rename"`
	Unlink    UnlinkEvent    `yaml:"unlink" field:"unlink" event:"unlink"`
	Utimes    UtimesEvent    `yaml:"utimes" field:"utimes" event:"utimes"`
	Link      LinkEvent      `yaml:"link" field:"link" event:"link"`
	Mount     MountEvent     `yaml:"mount" field:"-"`
	Umount    UmountEvent    `yaml:"umount" field:"-"`

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
	var buf bytes.Buffer
	buf.WriteRune('{')
	fmt.Fprintf(&buf, `"id":"%s",`, e.ID)

	entries := []eventMarshaler{
		{
			field:      "event",
			marshalFnc: e.Event.marshalJSON,
		},
		{
			field:      "process",
			marshalFnc: e.Process.marshalJSON,
		},
	}
	switch EventType(e.Event.Type) {
	case FileChmodEventType:
		entries = append(entries,
			eventMarshaler{
				field:      "file",
				marshalFnc: e.Chmod.marshalJSON,
			})
	case FileChownEventType:
		entries = append(entries,
			eventMarshaler{
				field:      "file",
				marshalFnc: e.Chown.marshalJSON,
			})
	case FileOpenEventType:
		entries = append(entries,
			eventMarshaler{
				field:      "file",
				marshalFnc: e.Open.marshalJSON,
			})
	case FileMkdirEventType:
		entries = append(entries,
			eventMarshaler{
				field:      "file",
				marshalFnc: e.Mkdir.marshalJSON,
			})
	case FileRmdirEventType:
		entries = append(entries,
			eventMarshaler{
				field:      "file",
				marshalFnc: e.Rmdir.marshalJSON,
			})
	case FileUnlinkEventType:
		entries = append(entries,
			eventMarshaler{
				field:      "file",
				marshalFnc: e.Unlink.marshalJSON,
			})
	case FileRenameEventType:
		entries = append(entries,
			eventMarshaler{
				field:      "file",
				marshalFnc: e.Rename.marshalJSON,
			})
	case FileUtimeEventType:
		entries = append(entries,
			eventMarshaler{
				field:      "file",
				marshalFnc: e.Utimes.marshalJSON,
			})
	case FileLinkEventType:
		entries = append(entries,
			eventMarshaler{
				field:      "file",
				marshalFnc: e.Link.marshalJSON,
			})
	case FileMountEventType:
		entries = append(entries,
			eventMarshaler{
				field:      "mount",
				marshalFnc: e.Mount.marshalJSON,
			})
	case FileUmountEventType:
		entries = append(entries,
			eventMarshaler{
				field:      "umount",
				marshalFnc: e.Umount.marshalJSON,
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
	return EventType(e.Event.Type).String()
}

// GetID returns the event identifier
func (e *Event) GetID() string {
	return e.ID
}

// GetPointer return an unsafe.Pointer of the Event
func (e *Event) GetPointer() unsafe.Pointer {
	return unsafe.Pointer(e)
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *Event) UnmarshalBinary(data []byte) (int, error) {
	offset, err := e.Process.UnmarshalBinary(data)
	if err != nil {
		return offset, err
	}

	return offset, nil
}

// NewEvent returns a new event
func NewEvent(resolvers *Resolvers) *Event {
	id, _ := uuid.NewRandom()
	return &Event{
		ID:        id.String(),
		resolvers: resolvers,
	}
}
