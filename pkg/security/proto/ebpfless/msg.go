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
	SyscallTypeUnknown SyscallType = 0
	// SyscallTypeExec exec type
	SyscallTypeExec SyscallType = 1
	// SyscallTypeFork fork type
	SyscallTypeFork SyscallType = 2
	// SyscallTypeOpen open type
	SyscallTypeOpen SyscallType = 3
	// SyscallTypeExit exit type
	SyscallTypeExit SyscallType = 4
	// SyscallTypeFcntl fcntl type
	SyscallTypeFcntl SyscallType = 5
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

// ExecSyscallMsg defines an exec message
type ExecSyscallMsg struct {
	Filename string
	Args     []string
	Envs     []string
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
	Dup              *DupSyscallFakeMsg
	Chdir            *ChdirSyscallFakeMsg
}

// String returns string representation
func (s SyscallMsg) String() string {
	b, _ := json.Marshal(s)
	return string(b)
}
