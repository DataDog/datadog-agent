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

	"github.com/vmihailenco/msgpack/v5"

	"github.com/DataDog/datadog-agent/pkg/security/proto/ebpfless"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model/sharedconsts"
	"golang.org/x/sys/unix"
)

func (ctx *CWSPtracerCtx) resolveProcessContext(process *Process) {
	if process == nil || process.containerContextResolved {
		return
	}
	process.containerContextResolved = true

	containerID, cgroupID, err := getProcContainerContext(process.Pid)
	if err != nil {
		if process.CGroupID == "" {
			process.CGroupID = ctx.cgroupID
		}
		if process.ContainerID == "" {
			process.ContainerID = ctx.containerID
		}
		return
	}
	if process.CGroupID == "" {
		if cgroupID != "" {
			process.CGroupID = cgroupID
		} else {
			process.CGroupID = ctx.cgroupID
		}
	}
	if process.ContainerID == "" {
		if containerID != "" {
			process.ContainerID = containerID
		} else {
			process.ContainerID = ctx.containerID
		}
	}
}

func (ctx *CWSPtracerCtx) sendSyscallMsg(process *Process, msg *ebpfless.SyscallMsg) {
	if msg == nil {
		return
	}
	ctx.resolveProcessContext(process)
	msg.PID = uint32(process.Tgid)
	msg.Timestamp = uint64(time.Now().UnixNano())
	msg.ContainerID = process.ContainerID
	msg.CGroupID = process.CGroupID
	message := &ebpfless.Message{
		Type:    ebpfless.MessageTypeSyscall,
		Syscall: msg,
	}
	if ctx.lifecycleGuard == nil {
		_ = ctx.sendMsg(message)
		return
	}

	// Freeze the payload and capture its generation on the tracing thread.
	// Handlers can reuse their messages; the network sender receives bytes only.
	logger.Debugf("sending message: %s", message)
	data, err := msgpack.Marshal(message)
	if err != nil {
		logger.Errorf("unable to marshal message: %v", err)
		return
	}
	outgoing := serializedSyscall{
		owner:   process.identity(),
		kind:    msg.Type,
		payload: data,
	}
	if msg.Type == ebpfless.SyscallTypeFork {
		// A fork belongs to the parent's event stream but carries the child's
		// PID on the wire. Guard both generations without changing that protocol.
		outgoing.child = outgoing.owner
		if parent := ctx.processCache.Get(int(msg.Fork.PPID)); parent != nil {
			outgoing.owner = parent.identity()
		} else {
			_, startTime, _ := readProcessStat(int(msg.Fork.PPID))
			outgoing.owner = processIdentity{tgid: msg.Fork.PPID, startTime: startTime}
		}
	}
	ctx.lifecycleGuard.filterAndEnqueue(outgoing)
	ctx.lifecycleGuard.reportSuppressedForks()
}

func (ctx *CWSPtracerCtx) handlePreHooks(nr int, regs syscall.PtraceRegs, process *Process, handler syscallHandler) {
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

	/* internal special cases */
	switch nr {
	case ExecveNr:
		// Top level pids, add ctx.opts.Creds. For the other PIDs the creds will be propagated at the probe side
		for _, pid := range ctx.PIDs {
			if process.Pid == pid && !slices.Contains(ctx.traceesReported, pid) {
				// report the creds for the first time
				ctx.traceesReported = append(ctx.traceesReported, pid)

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
	}
}

func (ctx *CWSPtracerCtx) sendForkMsg(process *Process, creatorTID int) {
	if creator := ctx.processCache.Get(creatorTID); creator != nil {
		ctx.sendSyscallMsg(process, &ebpfless.SyscallMsg{
			Type: ebpfless.SyscallTypeFork,
			Fork: &ebpfless.ForkSyscallMsg{PPID: uint32(creator.Tgid)},
		})
	}
}

func (ctx *CWSPtracerCtx) handleClone(flags uint64, process *Process, creatorTID int) {
	if ctx.processCache.Get(creatorTID) == nil {
		return
	}
	ctx.processCache.shareResources(process, creatorTID, flags)

	if flags&unix.CLONE_THREAD == 0 {
		ctx.sendForkMsg(process, creatorTID)
	}
	// Mark ready only after inheritance and fork enqueueing. Threads skip the
	// process-level fork message but still need their shared resource pointers.
	process.creationHandled = true
}

func (ctx *CWSPtracerCtx) handlePostHooks(nr int, creatorTID int, regs syscall.PtraceRegs, process *Process, handler syscallHandler) {
	// A nonzero creatorTID denotes a creation event dispatched for its child,
	// not an ordinary syscall exit. The creator can itself be a nonleader TID.
	if creatorTID != 0 {
		if process.creationHandled {
			return
		}
		if process.creationWaitTimedOut {
			// The child may already have exec'd or changed its FD/CWD caches.
			// Never reapply inherited resources. The lifecycle guard drops the
			// fork if telemetry has started, but can forward it if nothing has
			// been reported for this child yet. Neither path waits again.
			if process.Pid == process.Tgid {
				ctx.sendForkMsg(process, creatorTID)
			}
			process.creationHandled = true
			return
		}
	}
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
	case CloneNr:
		ctx.handleClone(ctx.ReadArgUint64(regs, 0), process, creatorTID)
	case Clone3Nr:
		data, err := ctx.ReadArgData(process.Pid, regs, 0, 8 /*sizeof flags only*/)
		if err != nil {
			return
		}
		ctx.handleClone(binary.NativeEndian.Uint64(data), process, creatorTID)
	case ForkNr, VforkNr:
		ctx.handleClone(0, process, creatorTID)
	}
}

func (ctx *CWSPtracerCtx) handleExit(process *Process, waitStatus *syscall.WaitStatus) {
	// send exit only for process not threads
	if process.Pid == process.Tgid && waitStatus != nil {
		exitCtx := &ebpfless.ExitSyscallMsg{}
		if waitStatus.Exited() {
			exitCtx.Cause = sharedconsts.ExitExited
			exitCtx.Code = uint32(waitStatus.ExitStatus())
		} else if waitStatus.CoreDump() {
			exitCtx.Cause = sharedconsts.ExitCoreDumped
			exitCtx.Code = uint32(waitStatus.Signal())
		} else if waitStatus.Signaled() {
			exitCtx.Cause = sharedconsts.ExitSignaled
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

func (ctx *CWSPtracerCtx) handleHooks(cbType CallbackType, nr int, pid int, creatorTID int, regs syscall.PtraceRegs, waitStatus *syscall.WaitStatus) {
	handler, handlerFound := ctx.syscallHandlers[nr]
	if !handlerFound && !slices.Contains([]int{ExecveNr, ExecveatNr, CloneNr, Clone3Nr, ForkNr, VforkNr, ExitNr}, nr) {
		return
	}

	process := ctx.processCache.Get(pid)
	if process == nil {
		process = NewProcess(pid)
		ctx.processCache.Add(pid, process)
	}

	switch cbType {
	case CallbackPreType:
		ctx.handlePreHooks(nr, regs, process, handler)

	case CallbackPostType:
		ctx.handlePostHooks(nr, creatorTID, regs, process, handler)

	case CallbackExitType:
		ctx.handleExit(process, waitStatus)
	}
}
