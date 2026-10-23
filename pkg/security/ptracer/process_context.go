// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package ptracer

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v4/process"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/DataDog/datadog-agent/pkg/security/proto/ebpfless"
)

// collectProcessContext reads a best-effort procfs snapshot while the emitting
// tracee is stopped. It reuses the existing syscall/exec representation; the
// snapshot is never sent as an exec event. Keep partial data, but not data read
// across PID reuse.
func (ctx *CWSPtracerCtx) collectProcessContext(identity processIdentity) (snapshot *ebpfless.SyscallMsg, err error) {
	pid := int(identity.tgid)
	ppid, startTime, err := readProcessStat(pid)
	if err != nil {
		return nil, err
	}
	if identity.startTime != 0 && identity.startTime != startTime {
		return nil, fmt.Errorf("process %d changed generation before its context lookup", pid)
	}
	defer func() {
		_, currentStartTime, statErr := readProcessStat(pid)
		if statErr != nil {
			snapshot, err = nil, statErr
		} else if currentStartTime != startTime {
			snapshot, err = nil, fmt.Errorf("process %d changed generation during its context lookup", pid)
		}
	}()

	snapshot = &ebpfless.SyscallMsg{
		Type: ebpfless.SyscallTypeExec, PID: identity.tgid, StartTime: startTime,
		ContainerID: ctx.containerID, CGroupID: ctx.cgroupID,
		Exec: &ebpfless.ExecSyscallMsg{PPID: ppid, FromProcFS: true},
	}
	proc, err := process.NewProcess(int32(pid))
	if err != nil {
		return snapshot, err
	}
	image := snapshot.Exec
	birth, birthErr := proc.CreateTime()
	if birthErr == nil && birth > 0 {
		image.CreateTime = uint64(time.UnixMilli(birth).UnixNano())
	}
	uids, uidErr := proc.Uids()
	gids, gidErr := proc.Gids()
	if uidErr == nil && len(uids) < 2 {
		uidErr = fmt.Errorf("missing UIDs for process %d", pid)
	}
	if gidErr == nil && len(gids) < 2 {
		gidErr = fmt.Errorf("missing GIDs for process %d", pid)
	}
	if uidErr == nil && gidErr == nil {
		image.Credentials = &ebpfless.Credentials{
			UID: uint32(uids[0]), EUID: uint32(uids[1]), GID: uint32(gids[0]), EGID: uint32(gids[1]),
		}
	}
	filename, fileErr := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if filename == "/ (deleted)" {
		filename, fileErr = "", fmt.Errorf("unavailable executable for process %d", pid)
	}
	image.File.Filename = procExecutableName(filename)
	args, argsErr := proc.CmdlineSlice()
	image.Args, image.ArgsTruncated = truncateArgs(args)
	image.ArgsTruncated = image.ArgsTruncated || argsErr != nil
	var envErr error
	image.Envs, image.EnvsTruncated, envErr = collectPIDEnvVars(int32(pid))
	image.EnvsTruncated = image.EnvsTruncated || envErr != nil
	image.TTY = getPidTTY(pid)
	containerID, cgroupID, cgroupErr := getProcContainerContext(pid)
	if containerID != "" {
		snapshot.ContainerID = containerID
	}
	if cgroupID != "" {
		snapshot.CGroupID = cgroupID
	}
	ctx.fillProcessContextStats(pid, image)
	// Unreadable argv/envp or cgroups must not discard a valid image,
	// credentials and PPID. No part of this lookup is retried on later events.
	return snapshot, errors.Join(birthErr, uidErr, gidErr, fileErr, argsErr, envErr, cgroupErr)
}

// fillProcessContextStats adds optional names and file attributes without
// depending on an executable path in the tracer's mount namespace.
func (ctx *CWSPtracerCtx) fillProcessContextStats(pid int, image *ebpfless.ExecSyscallMsg) {
	if ctx.opts == nil || ctx.opts.StatsDisabled {
		return
	}
	if creds := image.Credentials; creds != nil {
		creds.User = getUserFromUID(&ctx.Tracer, int32(creds.UID))
		creds.EUser = getUserFromUID(&ctx.Tracer, int32(creds.EUID))
		creds.Group = getGroupFromGID(&ctx.Tracer, int32(creds.GID))
		creds.EGroup = getGroupFromGID(&ctx.Tracer, int32(creds.EGID))
	}
	// The executable can have been unlinked since exec; stat through procfs.
	if info, err := os.Stat(fmt.Sprintf("/proc/%d/exe", pid)); err == nil {
		stat := info.Sys().(*syscall.Stat_t)
		file := &image.File
		file.CTime, file.MTime = uint64(stat.Ctim.Nano()), uint64(stat.Mtim.Nano())
		file.Inode, file.Mode = stat.Ino, stat.Mode
		file.Credentials = &ebpfless.Credentials{
			UID: stat.Uid, User: getUserFromUID(&ctx.Tracer, int32(stat.Uid)),
			GID: stat.Gid, Group: getGroupFromGID(&ctx.Tracer, int32(stat.Gid)),
		}
	}
}

// processContextForEvent reserves one procfs lookup per unknown generation,
// including failures. New threads of an initialized process need no lookup.
func (ctx *CWSPtracerCtx) processContextForEvent(identity processIdentity) *ebpfless.SyscallMsg {
	guard := ctx.lifecycleGuard
	if !guard.beginProcessContext(identity) {
		return nil
	}
	reader := ctx.processContextReader
	if reader == nil {
		reader = ctx.collectProcessContext
	}
	snapshot, err := reader(identity)
	valid := snapshot != nil && snapshot.PID == identity.tgid && snapshot.Exec != nil &&
		(identity.startTime == 0 || snapshot.StartTime == 0 || snapshot.StartTime == identity.startTime)
	if err != nil || !valid {
		guard.contextFallbackFailures++
	} else {
		guard.contextFallbacks++
	}
	if !valid {
		return nil
	}
	if state := guard.states[identity]; state != nil {
		state.contextPPID = snapshot.Exec.PPID
	}
	return snapshot
}

// marshalSyscallMessage bounds optional procfs metadata, not real exec payloads.
// The metadata travels in Exec alongside the triggering event, so queue overflow
// cannot drop it separately. Oversized snapshots lose argv/envp before metadata.
func marshalSyscallMessage(message *ebpfless.Message) ([]byte, error) {
	event := message.Syscall
	data, err := msgpack.Marshal(message)
	if err != nil || len(data) <= ebpfless.MaxMessageSize || event.Type == ebpfless.SyscallTypeExec ||
		event.Exec == nil || !event.Exec.FromProcFS {
		return data, err
	}

	image := *event.Exec
	image.Args, image.Envs = nil, nil
	image.ArgsTruncated, image.EnvsTruncated = true, true
	event.Exec = &image
	data, err = msgpack.Marshal(message)
	if err == nil && len(data) > ebpfless.MaxMessageSize {
		// Best-effort metadata must not poison every reconnect retry of an
		// otherwise valid event. Fork's optional fields describe that metadata.
		event.Exec = nil
		if event.Type == ebpfless.SyscallTypeFork && event.Fork != nil {
			fork := *event.Fork
			fork.ParentStartTime, fork.ParentContainerID, fork.ParentCGroupID = 0, "", ""
			event.Fork = &fork
		}
		return msgpack.Marshal(message)
	}
	return data, err
}
