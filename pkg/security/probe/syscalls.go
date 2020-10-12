// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

//go:generate stringer -type Syscall -output syscalls_string_linux.go

package probe

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
)

// Syscall represents a syscall identifier
type Syscall int

// Linux syscall identifiers
const (
	SysRead Syscall = iota
	SysWrite
	SysOpen
	SysClose
	SysStat
	SysFstat
	SysLstat
	SysPoll
	SysLseek
	SysMmap
	SysMprotect
	SysMunmap
	SysBrk
	SysRtSigaction
	SysRtSigprocmask
	SysRtSigreturn
	SysIoctl
	SysPread64
	SysPwrite64
	SysReadv
	SysWritev
	SysAccess
	SysPipe
	SysSelect
	SysSchedYield
	SysMremap
	SysMsync
	SysMincore
	SysMadvise
	SysShmget
	SysShmat
	SysShmctl
	SysDup
	SysDup2
	SysPause
	SysNanosleep
	SysGetitimer
	SysAlarm
	SysSetitimer
	SysGetpid
	SysSendfile
	SysSocket
	SysConnect
	SysAccept
	SysSendto
	SysRecvfrom
	SysSendmsg
	SysRecvmsg
	SysShutdown
	SysBind
	SysListen
	SysGetsockname
	SysGetpeername
	SysSocketpair
	SysSetsockopt
	SysGetsockopt
	SysClone
	SysFork
	SysVfork
	SysExecve
	SysExit
	SysWait4
	SysKill
	SysUname
	SysSemget
	SysSemop
	SysSemctl
	SysShmdt
	SysMsgget
	SysMsgsnd
	SysMsgrcv
	SysMsgctl
	SysFcntl
	SysFlock
	SysFsync
	SysFdatasync
	SysTruncate
	SysFtruncate
	SysGetdents
	SysGetcwd
	SysChdir
	SysFchdir
	SysRename
	SysMkdir
	SysRmdir
	SysCreat
	SysLink
	SysUnlink
	SysSymlink
	SysReadlink
	SysChmod
	SysFchmod
	SysChown
	SysFchown
	SysLchown
	SysUmask
	SysGettimeofday
	SysGetrlimit
	SysGetrusage
	SysSysinfo
	SysTimes
	SysPtrace
	SysGetuid
	SysSyslog
	SysGetgid
	SysSetuid
	SysSetgid
	SysGeteuid
	SysGetegid
	SysSetpgid
	SysGetppid
	SysGetpgrp
	SysSetsid
	SysSetreuid
	SysSetregid
	SysGetgroups
	SysSetgroups
	SysSetresuid
	SysGetresuid
	SysSetresgid
	SysGetresgid
	SysGetpgid
	SysSetfsuid
	SysSetfsgid
	SysGetsid
	SysCapget
	SysCapset
	SysRtSigpending
	SysRtSigtimedwait
	SysRtSigqueueinfo
	SysRtSigsuspend
	SysSigaltstack
	SysUtime
	SysMknod
	SysUselib
	SysPersonality
	SysUstat
	SysStatfs
	SysFstatfs
	SysSysfs
	SysGetpriority
	SysSetpriority
	SysSchedSetparam
	SysSchedGetparam
	SysSchedSetscheduler
	SysSchedGetscheduler
	SysSchedGetPriorityMax
	SysSchedGetPriorityMin
	SysSchedRrGetInterval
	SysMlock
	SysMunlock
	SysMlockall
	SysMunlockall
	SysVhangup
	SysModifyLdt
	SysPivotRoot
	SysSysctl
	SysPrctl
	SysArchPrctl
	SysAdjtimex
	SysSetrlimit
	SysChroot
	SysSync
	SysAcct
	SysSettimeofday
	SysMount
	SysUmount2
	SysSwapon
	SysSwapoff
	SysReboot
	SysSethostname
	SysSetdomainname
	SysIopl
	SysIoperm
	SysCreateModule
	SysInitModule
	SysDeleteModule
	SysGetKernelSyms
	SysQueryModule
	SysQuotactl
	SysNfsservctl
	SysGetpmsg
	SysPutpmsg
	SysAfsSyscall
	SysTuxcall
	SysSecurity
	SysGettid
	SysReadahead
	SysSetxattr
	SysLsetxattr
	SysFsetxattr
	SysGetxattr
	SysLgetxattr
	SysFgetxattr
	SysListxattr
	SysLlistxattr
	SysFlistxattr
	SysRemovexattr
	SysLremovexattr
	SysFremovexattr
	SysTkill
	SysTime
	SysFutex
	SysSchedSetaffinity
	SysSchedGetaffinity
	SysSetThreadArea
	SysIoSetup
	SysIoDestroy
	SysIoGetevents
	SysIoSubmit
	SysIoCancel
	SysGetThreadArea
	SysLookupDcookie
	SysEpollCreate
	SysEpollCtlOld
	SysEpollWaitOld
	SysRemapFilePages
	SysGetdents64
	SysSetTidAddress
	SysRestartSyscall
	SysSemtimedop
	SysFadvise64
	SysTimerCreate
	SysTimerSettime
	SysTimersysReadGettime
	SysTimerGetoverrun
	SysTimerDelete
	SysClockSettime
	SysClockGettime
	SysClockGetres
	SysClockNanosleep
	SysExitGroup
	SysEpollWait
	SysEpollCtl
	SysTgkill
	SysUtimes
	SysVserver
	SysMbind
	SysSetMempolicy
	SysGetMempolicy
	SysMqOpen
	SysMqUnlink
	SysMqTimedsend
	SysMqTimedreceive
	SysMqNotify
	SysMqGetsetattr
	SysKexecLoad
	SysWaitid
	SysAddKey
	SysRequestKey
	SysKeyctl
	SysIoprioSet
	SysIoprioGet
	SysInotifyInit
	SysInotifyAddWatch
	SysInotifyRmWatch
	SysMigratePages
	SysOpenat
	SysMkdirat
	SysMknodat
	SysFchownat
	SysFutimesat
	SysNewfstatat
	SysUnlinkat
	SysRenameat
	SysLinkat
	SysSymlinkat
	SysReadlinkat
	SysFchmodat
	SysFaccessat
	SysPselect6
	SysPpoll
	SysUnshare
	SysSetRobustList
	SysGetRobustList
	SysSplice
	SysTee
	SysSyncFileRange
	SysVmsplice
	SysMovePages
	SysUtimensat
	SysEpollPwait
	SysSignalfd
	SysTimerfdCreate
	SysEventfd
	SysFallocate
	SysTimerfdSettime
	SysTimerfdGettime
	SysAccept4
	SysSignalfd4
	SysEventfd2
	SysEpollCreate1
	SysDup3
	SysPipe2
	SysInotifyInit1
	SysPreadv
	SysPwritev
	SysRtTgsigqueueinfo
	SysPerfEventOpen
	SysRecvmmsg
	SysFanotifyInit
	SysFanotifyMark
	SysPrlimit64
)

// MarshalText maps the syscall identifier to UTF-8-encoded text and returns the result
func (s Syscall) MarshalText() ([]byte, error) {
	return []byte(strings.ToLower(strings.TrimPrefix(s.String(), "Sys"))), nil
}

// cache of the syscall prefix depending on kernel version
var syscallPrefix string
var ia32SyscallPrefix string

func getSyscallFnName(name string) string {
	if syscallPrefix == "" {
		syscall, err := ebpf.GetSyscallFnName("open")
		if err != nil {
			panic(err)
		}
		syscallPrefix = strings.TrimSuffix(syscall, "open")
		if syscallPrefix != "SyS_" {
			ia32SyscallPrefix = "__ia32_"
		} else {
			ia32SyscallPrefix = "compat_"
		}
	}

	return strings.ToLower(syscallPrefix) + name
}

func getIA32SyscallFnName(name string) string {
	return ia32SyscallPrefix + "sys_" + name
}

func getCompatSyscallFnName(name string) string {
	return ia32SyscallPrefix + "compat_sys_" + name
}

func syscallKprobe(name string, compat ...bool) []*ebpf.KProbe {
	kprobes := []*ebpf.KProbe{
		{
			Name:      getSyscallFnName(name),
			EntryFunc: "kprobe/" + getSyscallFnName(name),
			ExitFunc:  "kretprobe/" + getSyscallFnName(name),
		},
	}

	if ebpf.RuntimeArch == "x64" {
		if len(compat) > 0 && syscallPrefix != "SyS_" {
			kprobes = append(kprobes, &ebpf.KProbe{
				Name:      getCompatSyscallFnName(name),
				EntryFunc: "kprobe/" + getCompatSyscallFnName(name),
				ExitFunc:  "kretprobe/" + getCompatSyscallFnName(name),
			})
		} else {
			kprobes = append(kprobes, &ebpf.KProbe{
				Name:      getIA32SyscallFnName(name),
				EntryFunc: "kprobe/" + getIA32SyscallFnName(name),
				ExitFunc:  "kretprobe/" + getIA32SyscallFnName(name),
			})
		}
	}

	return kprobes
}
