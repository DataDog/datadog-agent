// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ebpfless holds msgpack messages
package ebpfless

import "encoding/json"

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
	// SyscallTypeFcntl fcntl type
	SyscallTypeFcntl
	// SyscallTypeSetUID setuid/setreuid type
	SyscallTypeSetUID
	// SyscallTypeSetGID setgid/setregid type
	SyscallTypeSetGID
)

// ContainerContext defines a container context
type ContainerContext struct {
	ID        string
	Name      string
	Tag       string
	CreatedAt uint64
}

// FcntlSyscallMsg defines a fcntl message
type FcntlSyscallMsg struct {
	Fd  uint32
	Cmd uint32
}

// Credentials defines process credentials
type Credentials struct {
	UID  uint32
	EUID uint32
	GID  uint32
	EGID uint32
}

// ExecSyscallMsg defines an exec message
type ExecSyscallMsg struct {
	Filename    string
	Args        []string
	Envs        []string
	Credentials *Credentials
}

// ForkSyscallMsg defines a fork message
type ForkSyscallMsg struct {
	PPID uint32
}

// ExitSyscallMsg defines an exit message
type ExitSyscallMsg struct{}

// OpenSyscallMsg defines an open message
type OpenSyscallMsg struct {
	Filename string
	Flags    uint32
	Mode     uint32
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
	UID  int32
	EUID int32
}

// SetGIDSyscallMsg defines a setregid message
type SetGIDSyscallMsg struct {
	GID  int32
	EGID int32
}

// SyscallMsg defines a syscall message
type SyscallMsg struct {
	SeqNum           uint64
	NSID             uint64
	Type             SyscallType
	PID              uint32
	ContainerContext *ContainerContext
	Exec             *ExecSyscallMsg
	Open             *OpenSyscallMsg
	Fork             *ForkSyscallMsg
	Exit             *ExitSyscallMsg
	Fcntl            *FcntlSyscallMsg
	SetUID           *SetUIDSyscallMsg
	SetGID           *SetGIDSyscallMsg

	// internals
	Dup   *DupSyscallFakeMsg
	Chdir *ChdirSyscallFakeMsg
}

// String returns string representation
func (s SyscallMsg) String() string {
	b, _ := json.Marshal(s)
	return string(b)
}
