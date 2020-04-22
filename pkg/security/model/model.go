//go:generate go run github.com/DataDog/datadog-agent/pkg/security/generators/accessors -output ../secl/eval/model_accessors.go

package model

import (
	"C"

	"bytes"
	"encoding/binary"

	"github.com/iovisor/gobpf/bcc"
)

var byteOrder binary.ByteOrder

type OpenSyscall struct {
	Filename string `yaml:"filename" field:"filename" tags:"fs"`
	Flags    int    `yaml:"flags" field:"flags" tags:"fs"`
	Mode     int    `yaml:"mode" field:"mode" tags:"fs"`
}

type UnlinkSyscall struct {
	Filename string `yaml:"filename" field:"filename" tags:"fs"`
}

type RenameSyscall struct {
	OldName string `yaml:"oldname" field:"oldname" tags:"fs"`
	NewName string `yaml:"newname" field:"newname" tags:"fs"`
}

type Process struct {
	UID  int    `yaml:"UID" field:"uid" tags:"process"`
	GID  int    `yaml:"GID" field:"gid" tags:"process"`
	PID  int    `yaml:"PID" field:"pid" tags:"process"`
	Name string `yaml:"name" field:"name" tags:"process"`
}

type Container struct {
	ID     string   `yaml:"id" field:"id" tags:"container"`
	Labels []string `yaml:"labels" field:"labels" tags:"container"`
}

// DentryEvent - Dentry event definition
type DentryEvent struct {
	EventBase
	*DentryEventRaw
	TTYName        string `json:"tty_name,omitempty" field:"tty"`
	SrcFilename    string `json:"src_filename,omitempty" field:"source_filename"`
	TargetFilename string `json:"target_filename,omitempty" field:"target_filename"`
}

// genaccessors
type Event struct {
	*DentryEvent

	Process   Process       `yaml:"process" field:"process"`
	Container Container     `yaml:"container" field:"container"`
	Syscall   string        `yaml:"syscall" field:"syscall"`
	Open      OpenSyscall   `yaml:"open" field:"open"`
	Unlink    UnlinkSyscall `yaml:"unlink" field:"unlink"`
	Rename    RenameSyscall `yaml:"rename" field:"rename"`
}

func (e *DentryEventRaw) UnmarshalBinary(data []byte) error {
	e.Pidns = byteOrder.Uint64(data[0:8])
	e.TimestampRaw = byteOrder.Uint64(data[8:16])
	binary.Read(bytes.NewBuffer(data[16:80]), bcc.GetHostByteOrder(), &e.TTYNameRaw)
	e.Pid = byteOrder.Uint32(data[80:84])
	e.Tid = byteOrder.Uint32(data[84:88])
	e.UID = byteOrder.Uint32(data[88:92])
	e.GID = byteOrder.Uint32(data[92:96])
	e.Flags = int32(byteOrder.Uint32(data[96:100]))
	e.Mode = int32(byteOrder.Uint32(data[100:104]))
	e.SrcInode = byteOrder.Uint32(data[104:108])
	e.SrcPathnameKey = byteOrder.Uint32(data[108:112])
	e.SrcMountID = int32(byteOrder.Uint32(data[112:116]))
	e.TargetInode = byteOrder.Uint32(data[116:120])
	e.TargetPathnameKey = byteOrder.Uint32(data[120:124])
	e.TargetMountID = int32(byteOrder.Uint32(data[124:128]))
	e.Retval = int32(byteOrder.Uint32(data[128:132]))
	e.Event = byteOrder.Uint32(data[132:136])
	return nil
}

func (e *SetAttrRaw) UnmarshalBinary(data []byte) error {
	e.Pidns = byteOrder.Uint64(data[0:8])
	e.TimestampRaw = byteOrder.Uint64(data[8:16])
	binary.Read(bytes.NewBuffer(data[16:80]), bcc.GetHostByteOrder(), &e.TTYNameRaw)
	e.Pid = byteOrder.Uint32(data[80:84])
	e.Tid = byteOrder.Uint32(data[84:88])
	e.UID = byteOrder.Uint32(data[88:92])
	e.GID = byteOrder.Uint32(data[92:96])
	e.Inode = byteOrder.Uint32(data[96:100])
	e.PathnameKey = byteOrder.Uint32(data[100:104])
	e.MountID = int32(byteOrder.Uint32(data[104:108]))
	e.Flags = byteOrder.Uint32(data[108:112])
	e.Mode = byteOrder.Uint32(data[112:116])
	e.NewUID = byteOrder.Uint32(data[116:120])
	e.NewGID = byteOrder.Uint32(data[120:124])
	e.Padding = byteOrder.Uint32(data[124:128])
	e.Retval = int32(byteOrder.Uint32(data[180:184]))
	return nil
}

func init() {
	byteOrder = bcc.GetHostByteOrder()
}
