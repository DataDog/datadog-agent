// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package ptracer holds the start command of CWS injector
package ptracer

import (
	"encoding/binary"
	"os"
	"slices"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/proto/ebpfless"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"golang.org/x/sys/unix"
)

func (ctx *CWSPtracerCtx) sendSyscallMsg(process *Process, msg *ebpfless.SyscallMsg) {
	if msg == nil {
		return
	}
	msg.PID = uint32(process.Tgid)
	msg.Timestamp = uint64(time.Now().UnixNano())
	msg.ContainerID = ctx.containerID
	_ = ctx.sendMsg(&ebpfless.Message{
		Type:    ebpfless.MessageTypeSyscall,
		Syscall: msg,
	})
}

func (ctx *CWSPtracerCtx) handlePreHooks(nr int, pid int, regs syscall.PtraceRegs, process *Process, handler syscallHandler) {
	syscallMsg := &ebpfless.SyscallMsg{}
	if nr == ExecveatNr {
		// special case: sometimes, execveat returns as execve, to handle that, we force
		// the msg to be put in ExecveNr
		process.Nr[ExecveNr] = syscallMsg
	} else {
		process.Nr[nr] = syscallMsg
	}

	if handler.Func != nil {
		err := handler.Func(&ctx.Tracer, process, syscallMsg, regs, ctx.opts.StatsDisabled)
		if err != nil {
			return
		}
	}

	// if available, gather span
	syscallMsg.SpanContext = fillSpanContext(&ctx.Tracer, process.Tgid, pid, ctx.processCache.GetSpan(process.Tgid))

	/* internal special cases */
	switch nr {
	case ExecveNr:
		// Top level pids, add ctx.opts.Creds. For the other PIDs the creds will be propagated at the probe side
		for _, pid := range ctx.PIDs {
			if process.Pid == pid {
				var uid, gid uint32

				if ctx.opts.Creds.UID != nil {
					uid = *ctx.opts.Creds.UID
				} else {
					uid = uint32(os.Getuid())
				}
				if ctx.opts.Creds.GID != nil {
					gid = *ctx.opts.Creds.GID
				} else {
					gid = uint32(os.Getgid())
				}
				syscallMsg.Exec.Credentials = &ebpfless.Credentials{
					UID:  uid,
					EUID: uid,
					GID:  gid,
					EGID: gid,
				}
				if !ctx.opts.StatsDisabled {
					syscallMsg.Exec.Credentials.User = getUserFromUID(&ctx.Tracer, int32(syscallMsg.Exec.Credentials.UID))
					syscallMsg.Exec.Credentials.EUser = getUserFromUID(&ctx.Tracer, int32(syscallMsg.Exec.Credentials.EUID))
					syscallMsg.Exec.Credentials.Group = getGroupFromGID(&ctx.Tracer, int32(syscallMsg.Exec.Credentials.GID))
					syscallMsg.Exec.Credentials.EGroup = getGroupFromGID(&ctx.Tracer, int32(syscallMsg.Exec.Credentials.EGID))
				}
			}
		}

		// special case for exec since the pre reports the pid while the post reports the tgid
		if process.Pid != process.Tgid {
			ctx.processCache.Add(process.Tgid, process)
		}
	case ExecveatNr:
		// special case for exec since the pre reports the pid while the post reports the tgid
		if process.Pid != process.Tgid {
			ctx.processCache.Add(process.Tgid, process)
		}
	case IoctlNr:
		req := handleERPC(&ctx.Tracer, process, regs)
		if len(req) != 0 {
			if isTLSRegisterRequest(req) {
				ctx.processCache.SetSpanTLS(process.Tgid, handleTLSRegister(req))
			}
		}
	}
}

func (ctx *CWSPtracerCtx) handleClone(flags uint64, process *Process, ppid int) {
	ctx.processCache.shareResources(process, ppid, flags)

	if flags&unix.CLONE_THREAD == 0 {
		if parent := ctx.processCache.Get(ppid); parent != nil {
			ctx.sendSyscallMsg(process, &ebpfless.SyscallMsg{
				Type: ebpfless.SyscallTypeFork,
				Fork: &ebpfless.ForkSyscallMsg{
					PPID: uint32(parent.Tgid),
				},
			})
		}
	}
}

func (ctx *CWSPtracerCtx) handlePostHooks(nr int, ppid int, regs syscall.PtraceRegs, process *Process, handler syscallHandler) {
	syscallMsg, msgExists := process.Nr[nr]
	if msgExists {
		if handler.RetFunc != nil {
			err := handler.RetFunc(&ctx.Tracer, process, syscallMsg, regs, ctx.opts.StatsDisabled)
			if err != nil {
				return
			}
		}
		if handler.ShouldSend != nil {
			syscallMsg.Retval = ctx.ReadRet(regs)
			if handler.ShouldSend(syscallMsg) && syscallMsg.Type != ebpfless.SyscallTypeUnknown {
				ctx.sendSyscallMsg(process, syscallMsg)
			}
		}
	}

	/* internal special cases */
	switch nr {
	case ExecveNr, ExecveatNr:
		// now the pid is the tgid
		process.Pid = process.Tgid
		// remove previously registered TLS
		ctx.processCache.UnsetSpan(process.Tgid)
	case CloneNr:
		ctx.handleClone(ctx.ReadArgUint64(regs, 0), process, ppid)
	case Clone3Nr:
		data, err := ctx.ReadArgData(process.Pid, regs, 0, 8 /*sizeof flags only*/)
		if err != nil {
			return
		}
		ctx.handleClone(binary.NativeEndian.Uint64(data), process, ppid)
	case ForkNr, VforkNr:
		if parent := ctx.processCache.Get(ppid); parent != nil {
			ctx.sendSyscallMsg(process, &ebpfless.SyscallMsg{
				Type: ebpfless.SyscallTypeFork,
				Fork: &ebpfless.ForkSyscallMsg{
					PPID: uint32(parent.Tgid),
				},
			})
		}
	}
}

func (ctx *CWSPtracerCtx) handleExit(process *Process, waitStatus *syscall.WaitStatus) {
	// send exit only for process not threads
	if process.Pid == process.Tgid && waitStatus != nil {
		exitCtx := &ebpfless.ExitSyscallMsg{}
		if waitStatus.Exited() {
			exitCtx.Cause = model.ExitExited
			exitCtx.Code = uint32(waitStatus.ExitStatus())
		} else if waitStatus.CoreDump() {
			exitCtx.Cause = model.ExitCoreDumped
			exitCtx.Code = uint32(waitStatus.Signal())
		} else if waitStatus.Signaled() {
			exitCtx.Cause = model.ExitSignaled
			exitCtx.Code = uint32(waitStatus.Signal())
		} else {
			exitCtx.Code = uint32(waitStatus.Signal())
		}

		ctx.sendSyscallMsg(process, &ebpfless.SyscallMsg{
			Type: ebpfless.SyscallTypeExit,
			Exit: exitCtx,
		})
	}

	ctx.processCache.Remove(process)

}

func (ctx *CWSPtracerCtx) handleHooks(cbType CallbackType, nr int, pid int, ppid int, regs syscall.PtraceRegs, waitStatus *syscall.WaitStatus) {
	handler, handlerFound := ctx.syscallHandlers[nr]
	if !handlerFound && !slices.Contains([]int{ExecveNr, ExecveatNr, IoctlNr, CloneNr, Clone3Nr, ForkNr, VforkNr, ExitNr}, nr) {
		return
	}

	process := ctx.processCache.Get(pid)
	if process == nil {
		process = NewProcess(pid)
		ctx.processCache.Add(pid, process)
	}

	switch cbType {
	case CallbackPreType:
		ctx.handlePreHooks(nr, pid, regs, process, handler)

	case CallbackPostType:
		ctx.handlePostHooks(nr, ppid, regs, process, handler)

	case CallbackExitType:
		ctx.handleExit(process, waitStatus)
	}
}
