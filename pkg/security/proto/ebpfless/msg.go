// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ebpfless holds msgpack messages
package ebpfless

import (
	"encoding/json"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// MessageType defines the type of a message
type MessageType int32

const (
	// MessageTypeUnknown unknown type
	MessageTypeUnknown MessageType = iota
	// MessageTypeHello hello type
	MessageTypeHello
	// MessageTypeSyscall syscall type
	MessageTypeSyscall
)

// SyscallType defines the type of a syscall message
type SyscallType int32

const (
	// SyscallTypeUnknown unknown type
	SyscallTypeUnknown SyscallType = iota
	// SyscallTypeExec exec type
	SyscallTypeExec
	// SyscallTypeFork fork type
	SyscallTypeFork
	// SyscallTypeOpen open type
	SyscallTypeOpen
	// SyscallTypeExit exit type
	SyscallTypeExit
	// SyscallTypeSetUID setuid/setreuid type
	SyscallTypeSetUID
	// SyscallTypeSetGID setgid/setregid type
	SyscallTypeSetGID
	// SyscallTypeSetFSUID setfsuid type
	SyscallTypeSetFSUID
	// SyscallTypeSetFSGID setfsgid type
	SyscallTypeSetFSGID
	// SyscallTypeCapset capset type
	SyscallTypeCapset
	// SyscallTypeUnlink unlink/unlinkat type
	SyscallTypeUnlink
	// SyscallTypeRmdir rmdir type
	SyscallTypeRmdir
	// SyscallTypeRename rename/renameat/renameat2 type
	SyscallTypeRename
	// SyscallTypeMkdir mkdir/mkdirat type
	SyscallTypeMkdir
	// SyscallTypeUtimes utime/utimes/utimensat/futimesat type
	SyscallTypeUtimes
	// SyscallTypeLink link/linkat/symlink/symlinkat type
	SyscallTypeLink
	// SyscallTypeChmod chmod/fchmod/fchmodat/fchmodat2 type
	SyscallTypeChmod
	// SyscallTypeChown chown/fchown/lchown/fchownat/fchownat2 type
	SyscallTypeChown
	// SyscallTypeLoadModule init_module/finit_module type
	SyscallTypeLoadModule
	// SyscallTypeUnloadModule delete_module type
	SyscallTypeUnloadModule
)

// ContainerContext defines a container context
type ContainerContext struct {
	ID             string
	Name           string
	ImageShortName string
	ImageTag       string
	CreatedAt      uint64
}

// FcntlSyscallMsg defines a fcntl message
type FcntlSyscallMsg struct {
	Fd  uint32
	Cmd uint32
}

// Credentials defines process credentials
type Credentials struct {
	UID    uint32
	User   string
	EUID   uint32
	EUser  string
	GID    uint32
	Group  string
	EGID   uint32
	EGroup string
}

// ExecSyscallMsg defines an exec message
type ExecSyscallMsg struct {
	File          FileSyscallMsg
	Args          []string
	ArgsTruncated bool
	Envs          []string
	EnvsTruncated bool
	TTY           string
	Credentials   *Credentials
	PPID          uint32
}

// ForkSyscallMsg defines a fork message
type ForkSyscallMsg struct {
	PPID uint32
}

// ExitSyscallMsg defines an exit message
type ExitSyscallMsg struct {
	Code  uint32
	Cause model.ExitCause
}

// FileSyscallMsg defines a file message
type FileSyscallMsg struct {
	Filename    string
	CTime       uint64
	MTime       uint64
	Mode        uint32
	Credentials *Credentials
}

// OpenSyscallMsg defines an open message
type OpenSyscallMsg struct {
	FileSyscallMsg
	Flags uint32
}

// DupSyscallFakeMsg defines a dup message
type DupSyscallFakeMsg struct {
	OldFd int32
}

// ChdirSyscallFakeMsg defines a chdir message
type ChdirSyscallFakeMsg struct {
	Path string
}

// SetUIDSyscallMsg defines a setreuid message
type SetUIDSyscallMsg struct {
	UID   int32
	User  string
	EUID  int32
	EUser string
}

// SetGIDSyscallMsg defines a setregid message
type SetGIDSyscallMsg struct {
	GID    int32
	Group  string
	EGID   int32
	EGroup string
}

// SetFSUIDSyscallMsg defines a setfsuid message
type SetFSUIDSyscallMsg struct {
	FSUID  int32
	FSUser string
}

// SetFSGIDSyscallMsg defines a setfsgid message
type SetFSGIDSyscallMsg struct {
	FSGID   int32
	FSGroup string
}

// CapsetSyscallMsg defines a capset message
type CapsetSyscallMsg struct {
	Effective uint64
	Permitted uint64
}

// UnlinkSyscallMsg defines a unlink message
type UnlinkSyscallMsg struct {
	File FileSyscallMsg
}

// RmdirSyscallMsg defines a rmdir message
type RmdirSyscallMsg struct {
	File FileSyscallMsg
}

// RenameSyscallMsg defines a rename/renameat/renameat2 message
type RenameSyscallMsg struct {
	OldFile FileSyscallMsg
	NewFile FileSyscallMsg
}

// MkdirSyscallMsg defines a mkdir/mkdirat message
type MkdirSyscallMsg struct {
	Dir  FileSyscallMsg
	Mode uint32
}

// UtimesSyscallMsg defines a utime/utimes/utimensat/futimesat message
type UtimesSyscallMsg struct {
	File  FileSyscallMsg
	ATime uint64 // in nanoseconds
	MTime uint64 // in nanoseconds
}

// LinkType to handle the different link types
type LinkType uint8

const (
	// LinkTypeSymbolic defines a symbolic link type
	LinkTypeSymbolic LinkType = iota
	// LinkTypeHardlink defines an hard link type
	LinkTypeHardlink
)

// LinkSyscallMsg defines a link/linkat/symlink/symlinkat message
type LinkSyscallMsg struct {
	Type   LinkType
	Target FileSyscallMsg
	Link   FileSyscallMsg
}

// ChmodSyscallMsg defines a chmod/fchmod/fchmodat/fchmodat2 message
type ChmodSyscallMsg struct {
	File FileSyscallMsg
	Mode uint32
}

// ChownSyscallMsg defines a chown/fchown/lchown/fchownat/fchownat2 message
type ChownSyscallMsg struct {
	File  FileSyscallMsg
	UID   int32
	User  string
	GID   int32
	Group string
}

// LoadModuleSyscallMsg defines a init_module/finit_module message
type LoadModuleSyscallMsg struct {
	File             FileSyscallMsg
	LoadedFromMemory bool
	Name             string
	Args             string
}

// UnloadModuleSyscallMsg defines a delete_module message
type UnloadModuleSyscallMsg struct {
	Name string
}

// SyscallMsg defines a syscall message
type SyscallMsg struct {
	Type         SyscallType
	PID          uint32
	Timestamp    uint64
	Retval       int64
	Exec         *ExecSyscallMsg         `json:",omitempty"`
	Open         *OpenSyscallMsg         `json:",omitempty"`
	Fork         *ForkSyscallMsg         `json:",omitempty"`
	Exit         *ExitSyscallMsg         `json:",omitempty"`
	Fcntl        *FcntlSyscallMsg        `json:",omitempty"`
	SetUID       *SetUIDSyscallMsg       `json:",omitempty"`
	SetGID       *SetGIDSyscallMsg       `json:",omitempty"`
	SetFSUID     *SetFSUIDSyscallMsg     `json:",omitempty"`
	SetFSGID     *SetFSGIDSyscallMsg     `json:",omitempty"`
	Capset       *CapsetSyscallMsg       `json:",omitempty"`
	Unlink       *UnlinkSyscallMsg       `json:",omitempty"`
	Rmdir        *RmdirSyscallMsg        `json:",omitempty"`
	Rename       *RenameSyscallMsg       `json:",omitempty"`
	Mkdir        *MkdirSyscallMsg        `json:",omitempty"`
	Utimes       *UtimesSyscallMsg       `json:",omitempty"`
	Link         *LinkSyscallMsg         `json:",omitempty"`
	Chmod        *ChmodSyscallMsg        `json:",omitempty"`
	Chown        *ChownSyscallMsg        `json:",omitempty"`
	LoadModule   *LoadModuleSyscallMsg   `json:",omitempty"`
	UnloadModule *UnloadModuleSyscallMsg `json:",omitempty"`

	// internals
	Dup   *DupSyscallFakeMsg   `json:",omitempty"`
	Chdir *ChdirSyscallFakeMsg `json:",omitempty"`
}

// String returns string representation
func (s SyscallMsg) String() string {
	b, _ := json.Marshal(s)
	return string(b)
}

// HelloMsg defines a hello message
type HelloMsg struct {
	NSID             uint64
	ContainerContext *ContainerContext
	EntrypointArgs   []string
}

// Message defines a message
type Message struct {
	SeqNum  uint64
	Type    MessageType
	Hello   *HelloMsg   `json:",omitempty"`
	Syscall *SyscallMsg `json:",omitempty"`
}

// String returns string representation
func (m Message) String() string {
	b, _ := json.Marshal(m)
	return string(b)
}

// Reset resets a message
func (m *Message) Reset() {
	m.SeqNum = 0
	m.Type = MessageTypeUnknown
	m.Hello = nil
	m.Syscall = nil
}
