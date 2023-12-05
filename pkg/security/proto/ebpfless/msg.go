// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ebpfless

type SyscallType int32

const (
	SyscallType_Unknown SyscallType = 0
	SyscallType_Exec    SyscallType = 1
	SyscallType_Fork    SyscallType = 2
	SyscallType_Open    SyscallType = 3
	SyscallType_Exit    SyscallType = 4
	SyscallType_Fcntl   SyscallType = 5
)

type ContainerContext struct {
	ID        string
	Name      string
	Tag       string
	CreatedAt uint64
}

type FcntlSyscallMsg struct {
	Fd  uint32
	Cmd uint32
}

type ExecSyscallMsg struct {
	Filename string
	Args     []string
	Envs     []string
}

type ForkSyscallMsg struct {
	PPID uint32
}

type ExitSyscallMsg struct{}

type OpenSyscallMsg struct {
	Filename string
	Flags    uint32
	Mode     uint32
}

type DupSyscallFakeMsg struct {
	OldFd int32
}

type ChdirSyscallFakeMsg struct {
	Path string
}

type SyscallMsg struct {
	SeqNum           uint64
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
