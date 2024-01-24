// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package ptracer holds the start command of CWS injector
package ptracer

import (
	"encoding/binary"
	"errors"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/security/proto/ebpfless"
)

func registerProcessHandlers(handlers map[int]syscallHandler) []string {
	processHandlers := []syscallHandler{
		{
			IDs:        []syscallID{{ID: ExecveNr, Name: "execve"}},
			Func:       handleExecve,
			ShouldSend: shouldSendAlways,
			RetFunc:    nil,
		},
		{
			IDs:        []syscallID{{ID: ExecveatNr, Name: "execveat"}},
			Func:       handleExecveAt,
			ShouldSend: shouldSendAlways,
			RetFunc:    nil,
		},
		{
			IDs:        []syscallID{{ID: ChdirNr, Name: "chdir"}},
			Func:       handleChdir,
			ShouldSend: nil,
			RetFunc:    handleChdirRet,
		},
		{
			IDs:        []syscallID{{ID: FchdirNr, Name: "fchdir"}},
			Func:       handleFchdir,
			ShouldSend: nil,
			RetFunc:    handleChdirRet,
		},
		{
			IDs:        []syscallID{{ID: SetuidNr, Name: "setuid"}},
			Func:       handleSetuid,
			ShouldSend: isAcceptedRetval,
			RetFunc:    nil,
		},
		{
			IDs:        []syscallID{{ID: SetgidNr, Name: "setgid"}},
			Func:       handleSetgid,
			ShouldSend: isAcceptedRetval,
			RetFunc:    nil,
		},
		{
			IDs:        []syscallID{{ID: SetreuidNr, Name: "setreuid"}, {ID: SetresuidNr, Name: "setresuid"}},
			Func:       handleSetreuid,
			ShouldSend: isAcceptedRetval,
			RetFunc:    nil,
		},
		{
			IDs:        []syscallID{{ID: SetregidNr, Name: "setregid"}, {ID: SetresgidNr, Name: "setresgid"}},
			Func:       handleSetregid,
			ShouldSend: isAcceptedRetval,
			RetFunc:    nil,
		},
		{
			IDs:        []syscallID{{ID: SetfsuidNr, Name: "setfsuid"}},
			Func:       handleSetfsuid,
			ShouldSend: shouldSendAlways,
			RetFunc:    nil,
		},
		{
			IDs:        []syscallID{{ID: SetfsgidNr, Name: "setfsgid"}},
			Func:       handleSetfsgid,
			ShouldSend: shouldSendAlways,
			RetFunc:    nil,
		},
		{
			IDs:        []syscallID{{ID: CapsetNr, Name: "capset"}},
			Func:       handleCapset,
			ShouldSend: isAcceptedRetval,
			RetFunc:    nil,
		},
	}

	syscallList := []string{}
	for _, h := range processHandlers {
		for _, id := range h.IDs {
			if id.ID >= 0 { // insert only available syscalls
				handlers[id.ID] = h
				syscallList = append(syscallList, id.Name)
			}
		}
	}
	return syscallList
}

//
// handlers called on syscall entrance
//

func handleExecveAt(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs, disableStats bool) error {
	fd := tracer.ReadArgInt32(regs, 0)

	filename, err := tracer.ReadArgString(process.Pid, regs, 1)
	if err != nil {
		return err
	}

	if filename == "" { // in this case, dirfd defines directly the file's FD
		var exists bool
		if filename, exists = process.Res.Fd[fd]; !exists || filename == "" {
			return errors.New("can't find related file path")
		}
	} else {
		filename, err = getFullPathFromFd(process, filename, fd)
		if err != nil {
			return err
		}
	}

	args, err := tracer.ReadArgStringArray(process.Pid, regs, 2)
	if err != nil {
		return err
	}
	args, argsTruncated := truncateArgs(args)

	envs, err := tracer.ReadArgStringArray(process.Pid, regs, 3)
	if err != nil {
		return err
	}
	envs, envsTruncated := truncateEnvs(envs)

	msg.Type = ebpfless.SyscallTypeExec
	msg.Exec = &ebpfless.ExecSyscallMsg{
		File: ebpfless.OpenSyscallMsg{
			Filename: filename,
		},
		Args:          args,
		ArgsTruncated: argsTruncated,
		Envs:          envs,
		EnvsTruncated: envsTruncated,
		TTY:           getPidTTY(process.Pid),
	}
	// special case for execveat: we store ALSO the msg in execve bucket (see cws.go)
	process.Nr[ExecveNr] = msg
	return fillFileMetadata(filename, &msg.Exec.File, disableStats)
}

func handleExecve(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs, disableStats bool) error {
	filename, err := tracer.ReadArgString(process.Pid, regs, 0)
	if err != nil {
		return err
	}

	filename, err = getFullPathFromFilename(process, filename)
	if err != nil {
		return err
	}

	args, err := tracer.ReadArgStringArray(process.Pid, regs, 1)
	if err != nil {
		return err
	}
	args, argsTruncated := truncateArgs(args)

	envs, err := tracer.ReadArgStringArray(process.Pid, regs, 2)
	if err != nil {
		return err
	}
	envs, envsTruncated := truncateEnvs(envs)

	msg.Type = ebpfless.SyscallTypeExec
	msg.Exec = &ebpfless.ExecSyscallMsg{
		File: ebpfless.OpenSyscallMsg{
			Filename: filename,
		},
		Args:          args,
		ArgsTruncated: argsTruncated,
		Envs:          envs,
		EnvsTruncated: envsTruncated,
		TTY:           getPidTTY(process.Pid),
	}
	return fillFileMetadata(filename, &msg.Exec.File, disableStats)
}

func handleChdir(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs, _ bool) error {
	// using msg to temporary store arg0, as it will be erased by the return value on ARM64
	dirname, err := tracer.ReadArgString(process.Pid, regs, 0)
	if err != nil {
		return err
	}

	dirname, err = getFullPathFromFilename(process, dirname)
	if err != nil {
		process.Res.Cwd = ""
		return err
	}

	msg.Chdir = &ebpfless.ChdirSyscallFakeMsg{
		Path: dirname,
	}
	return nil
}

func handleFchdir(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs, _ bool) error {
	fd := tracer.ReadArgInt32(regs, 0)
	dirname, ok := process.Res.Fd[fd]
	if !ok {
		process.Res.Cwd = ""
		return nil
	}

	// using msg to temporary store arg0, as it will be erased by the return value on ARM64
	msg.Chdir = &ebpfless.ChdirSyscallFakeMsg{
		Path: dirname,
	}
	return nil
}

func handleSetuid(tracer *Tracer, _ *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs, _ bool) error {
	msg.Type = ebpfless.SyscallTypeSetUID
	msg.SetUID = &ebpfless.SetUIDSyscallMsg{
		UID:  tracer.ReadArgInt32(regs, 0),
		EUID: -1,
	}
	return nil
}

func handleSetgid(tracer *Tracer, _ *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs, _ bool) error {
	msg.Type = ebpfless.SyscallTypeSetGID
	msg.SetGID = &ebpfless.SetGIDSyscallMsg{
		GID:  tracer.ReadArgInt32(regs, 0),
		EGID: -1,
	}
	return nil
}

func handleSetreuid(tracer *Tracer, _ *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs, _ bool) error {
	msg.Type = ebpfless.SyscallTypeSetUID
	msg.SetUID = &ebpfless.SetUIDSyscallMsg{
		UID:  tracer.ReadArgInt32(regs, 0),
		EUID: tracer.ReadArgInt32(regs, 1),
	}
	return nil
}

func handleSetregid(tracer *Tracer, _ *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs, _ bool) error {
	msg.Type = ebpfless.SyscallTypeSetGID
	msg.SetGID = &ebpfless.SetGIDSyscallMsg{
		GID:  tracer.ReadArgInt32(regs, 0),
		EGID: tracer.ReadArgInt32(regs, 1),
	}
	return nil
}

func handleSetfsuid(tracer *Tracer, _ *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs, _ bool) error {
	msg.Type = ebpfless.SyscallTypeSetFSUID
	msg.SetFSUID = &ebpfless.SetFSUIDSyscallMsg{
		FSUID: tracer.ReadArgInt32(regs, 0),
	}
	return nil
}

func handleSetfsgid(tracer *Tracer, _ *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs, _ bool) error {
	msg.Type = ebpfless.SyscallTypeSetFSGID
	msg.SetFSGID = &ebpfless.SetFSGIDSyscallMsg{
		FSGID: tracer.ReadArgInt32(regs, 0),
	}
	return nil
}

func handleCapset(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs, _ bool) error {
	pCapsData, err := tracer.ReadArgData(process.Pid, regs, 1, 24 /*sizeof uint32 x3 x2*/)
	if err != nil {
		return err
	}

	// extract low bytes of effective caps
	effective := uint64(binary.NativeEndian.Uint32(pCapsData[0:4]))
	// extract high bytes of effective caps, merge them together
	effective |= uint64(binary.NativeEndian.Uint32(pCapsData[12:16])) << 32

	// extract low bytes of permitted caps
	permitted := uint64(binary.NativeEndian.Uint32(pCapsData[4:8]))
	// extract high bytes of permitted caps,  merge them together
	permitted |= uint64(binary.NativeEndian.Uint32(pCapsData[16:20])) << 32

	msg.Type = ebpfless.SyscallTypeCapset
	msg.Capset = &ebpfless.CapsetSyscallMsg{
		Effective: uint64(effective),
		Permitted: uint64(permitted),
	}
	return nil
}

//
// handlers called on syscall return
//

func handleChdirRet(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs, _ bool) error {
	if ret := tracer.ReadRet(regs); msg.Chdir != nil && ret >= 0 {
		process.Res.Cwd = msg.Chdir.Path
	}
	return nil
}
