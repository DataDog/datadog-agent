// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && amd64

package syscalls

import "github.com/DataDog/datadog-agent/pkg/security/secl/model"

func init() {
	// default syscalls: the baseline a typical Linux userspace process is
	// expected to issue at startup or as part of normal operation (file I/O,
	// memory management, signal handling, scheduling/synchronisation, process
	// info, polling). These are noisy by nature and are filtered out so that
	// only the more meaningful syscalls remain in the event.
	//
	// Must stay in sync with is_default_syscall in
	// pkg/security/ebpf/c/include/hooks/raw_syscalls.h.
	defaultSyscallSerializers = makeSyscallSerializers([]model.Syscall{
		// process lifecycle
		model.SysExecve, model.SysExit, model.SysExitGroup,
		model.SysClone, model.SysFork, model.SysVfork, model.SysWait4,

		// file I/O
		model.SysRead, model.SysWrite, model.SysReadv, model.SysWritev,
		model.SysPread64, model.SysPwrite64,
		model.SysOpen, model.SysOpenat, model.SysOpenat2,
		model.SysClose, model.SysLseek,
		model.SysStat, model.SysFstat, model.SysLstat, model.SysNewfstatat,
		model.SysReadlink, model.SysReadlinkat,
		model.SysGetdents, model.SysGetdents64, model.SysGetcwd,
		model.SysFcntl, model.SysIoctl,

		// memory
		model.SysBrk, model.SysMmap, model.SysMunmap, model.SysMprotect,
		model.SysMremap,

		// signals
		model.SysRtSigaction, model.SysRtSigprocmask, model.SysRtSigreturn,

		// process / user info
		model.SysGetpid, model.SysGettid, model.SysGetppid,
		model.SysGetuid, model.SysGeteuid, model.SysGetgid, model.SysGetegid,

		// scheduling / time / synchronisation
		model.SysFutex, model.SysSchedYield,
		model.SysNanosleep, model.SysClockGettime,
		model.SysGetrandom,
	})

	defaultNetworkSyscallSerializers = makeSyscallSerializers([]model.Syscall{
		model.SysSendmsg, model.SysSendmmsg, model.SysSendto,
		model.SysRecvmsg, model.SysRecvmmsg, model.SysRecvfrom,
	})
}
