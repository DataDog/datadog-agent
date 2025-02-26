// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ebpfless holds msgpack messages
package ebpfless

import (
	"encoding/json"
	"net"

	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model/sharedconsts"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model/utils"
)

// Mode defines ptrace mode
type Mode string

const (
	// UnknownMode unknown mode
	UnknownMode Mode = "unknown"
	// WrappedMode ptrace wrapping the binary
	WrappedMode Mode = "wrapped"
	// AttachedMode ptrace attached to a pid
	AttachedMode = "attached"
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
	// MessageTypeGoodbye event type
	MessageTypeGoodbye
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
	// SyscallTypeChdir chdir/fchdir type
	SyscallTypeChdir
	// SyscallTypeMount mount type
	SyscallTypeMount
	// SyscallTypeUmount umount/umount2 type
	SyscallTypeUmount
	// SyscallTypeAccept accept
	SyscallTypeAccept
	// SyscallTypeConnect connect
	SyscallTypeConnect
	// SyscallTypeBind bind
	SyscallTypeBind
)

// ContainerContext defines a container context
type ContainerContext struct {
	ID        containerutils.ContainerID
	CreatedAt uint64
}

// FcntlSyscallMsg defines a fcntl message
type FcntlSyscallMsg struct {
	Fd  uint32
	Cmd uint32
}

// Credentials defines process credentials
type Credentials struct {
	User   string
	EUser  string
	Group  string
	EGroup string
	UID    uint32
	EUID   uint32
	GID    uint32
	EGID   uint32
}

// ExecSyscallMsg defines an exec message
type ExecSyscallMsg struct {
	File          FileSyscallMsg
	Credentials   *Credentials
	TTY           string
	Args          []string
	Envs          []string
	PPID          uint32
	ArgsTruncated bool
	EnvsTruncated bool
	FromProcFS    bool
}

// ForkSyscallMsg defines a fork message
type ForkSyscallMsg struct {
	PPID uint32
}

// ExitSyscallMsg defines an exit message
type ExitSyscallMsg struct {
	Code  uint32
	Cause sharedconsts.ExitCause
}

// FileSyscallMsg defines a file message
type FileSyscallMsg struct {
	Credentials *Credentials
	Filename    string
	CTime       uint64
	MTime       uint64
	Inode       uint64
	Mode        uint32
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

// PipeSyscallFakeMsg defines a pipe message
type PipeSyscallFakeMsg struct {
	FdsPtr uint64
}

// SocketSyscallFakeMsg represents the socket message
type SocketSyscallFakeMsg struct {
	AddressFamily uint16
	Protocol      uint16
}

// ChdirSyscallMsg defines a chdir message
type ChdirSyscallMsg struct {
	Dir FileSyscallMsg
}

// SetUIDSyscallMsg defines a setreuid message
type SetUIDSyscallMsg struct {
	User  string
	EUser string
	UID   int32
	EUID  int32
}

// SetGIDSyscallMsg defines a setregid message
type SetGIDSyscallMsg struct {
	Group  string
	EGroup string
	GID    int32
	EGID   int32
}

// SetFSUIDSyscallMsg defines a setfsuid message
type SetFSUIDSyscallMsg struct {
	FSUser string
	FSUID  int32
}

// SetFSGIDSyscallMsg defines a setfsgid message
type SetFSGIDSyscallMsg struct {
	FSGroup string
	FSGID   int32
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
	Target FileSyscallMsg
	Link   FileSyscallMsg
	Type   LinkType
}

// ChmodSyscallMsg defines a chmod/fchmod/fchmodat/fchmodat2 message
type ChmodSyscallMsg struct {
	File FileSyscallMsg
	Mode uint32
}

// ChownSyscallMsg defines a chown/fchown/lchown/fchownat/fchownat2 message
type ChownSyscallMsg struct {
	File  FileSyscallMsg
	User  string
	Group string
	UID   int32
	GID   int32
}

// LoadModuleSyscallMsg defines a init_module/finit_module message
type LoadModuleSyscallMsg struct {
	File             FileSyscallMsg
	Name             string
	Args             string
	LoadedFromMemory bool
}

// UnloadModuleSyscallMsg defines a delete_module message
type UnloadModuleSyscallMsg struct {
	Name string
}

// SpanContext stores a span context (if any)
type SpanContext struct {
	SpanID  uint64
	TraceID utils.TraceID
}

// MountSyscallMsg defines a mount message
type MountSyscallMsg struct {
	Source string
	Target string
	FSType string
}

// UmountSyscallMsg defines a mount message
type UmountSyscallMsg struct {
	Path string
}

// MsgSocketInfo defines the base information for a socket message
type MsgSocketInfo struct {
	Addr          net.IP
	AddressFamily uint16
	Port          uint16
}

// BindSyscallMsg defines a bind message
type BindSyscallMsg struct {
	MsgSocketInfo
	Protocol uint16
}

// ConnectSyscallMsg defines a connect message
type ConnectSyscallMsg struct {
	MsgSocketInfo
	Protocol uint16
}

// AcceptSyscallMsg defines an accept message
type AcceptSyscallMsg struct {
	MsgSocketInfo
	SocketFd int32
}

// SyscallMsg defines a syscall message
type SyscallMsg struct {
	SpanContext  *SpanContext            `json:",omitempty"`
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
	Chdir        *ChdirSyscallMsg        `json:",omitempty"`
	Mount        *MountSyscallMsg        `json:",omitempty"`
	Umount       *UmountSyscallMsg       `json:",omitempty"`
	Bind         *BindSyscallMsg         `json:",omitempty"`
	Connect      *ConnectSyscallMsg      `json:",omitempty"`
	Accept       *AcceptSyscallMsg       `json:",omitempty"`

	// internals
	Dup         *DupSyscallFakeMsg    `json:",omitempty"`
	Pipe        *PipeSyscallFakeMsg   `json:",omitempty"`
	Socket      *SocketSyscallFakeMsg `json:",omitempty"`
	ContainerID containerutils.ContainerID
	Timestamp   uint64
	Retval      int64
	Type        SyscallType
	PID         uint32
}

// String returns string representation
func (s SyscallMsg) String() string {
	b, _ := json.Marshal(s)
	return string(b)
}

// HelloMsg defines a hello message
type HelloMsg struct {
	ContainerContext *ContainerContext
	Mode             Mode
	EntrypointArgs   []string
	NSID             uint64
}

// Message defines a message
type Message struct {
	Hello   *HelloMsg   `json:",omitempty"`
	Syscall *SyscallMsg `json:",omitempty"`
	Type    MessageType
}

// String returns string representation
func (m Message) String() string {
	b, _ := json.Marshal(m)
	return string(b)
}

// Reset resets a message
func (m *Message) Reset() {
	m.Type = MessageTypeUnknown
	m.Hello = nil
	m.Syscall = nil
}
