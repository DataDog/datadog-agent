// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probes

import (
	"fmt"

	manager "github.com/DataDog/ebpf-manager"
	"k8s.io/utils/strings/slices"
)

type syscallDefinition struct {
	name       string
	flag       int
	compatMode bool
}

var allSyscalls = []syscallDefinition{
	// Already probed syscalls for system-probe purpose
	{"bind", Entry, false},
	{"bpf", Entry, false},
	{"capset", Entry, false},
	{"chmod", Entry, false},
	{"chown", Entry, false},
	{"chown16", Entry, false},
	{"clone", Entry, false},
	{"clone3", Entry, false},
	{"creat", Entry, false},
	{"delete_module", Entry, false},
	{"execve", Entry, false},
	{"execveat", Entry, false},
	{"fchmod", Entry, false},
	{"fchmodat", Entry, false},
	{"fchown", Entry, false},
	{"fchown16", Entry, false},
	{"fchownat", Entry, false},
	{"finit_module", Entry, false},
	{"fork", Entry, false},
	{"fremovexattr", Entry, false},
	{"fsetxattr", Entry, false},
	{"futimesat", Entry | ExpandTime32, false}, // ExpandTime32
	{"futimesat", Entry, true},                 // compat
	{"init_module", Entry, false},
	{"kill", Entry, false},
	{"lchown", Entry, false},
	{"lchown16", Entry, false},
	{"link", Entry, false},
	{"linkat", Entry, false},
	{"lremovexattr", Entry, false},
	{"lsetxattr", Entry, false},
	{"mkdir", Entry, false},
	{"mkdirat", Entry, false},
	{"mount", Entry, true}, // comapt
	{"mprotect", Entry, false},
	{"open", Entry, true},              // compat
	{"open_by_handle_at", Entry, true}, // compat
	{"openat", Entry, true},            // compat
	{"openat2", Entry, false},
	{"ptrace", Entry, false},
	{"removexattr", Entry, false},
	{"rename", Entry, false},
	{"renameat", Entry, false},
	{"renameat2", Entry, false},
	{"rmdir", Entry, false},
	{"setfsgid", Entry, false},
	{"setfsgid16", Entry, false},
	{"setfsuid", Entry, false},
	{"setfsuid16", Entry, false},
	{"setgid", Entry, false},
	{"setgid16", Entry, false},
	{"setregid", Entry, false},
	{"setregid16", Entry, false},
	{"setresgid", Entry, false},
	{"setresgid16", Entry, false},
	{"setresuid", Entry, false},
	{"setresuid16", Entry, false},
	{"setreuid", Entry, false},
	{"setreuid16", Entry, false},
	{"setuid", Entry, false},
	{"setuid16", Entry, false},
	{"setxattr", Entry, false},
	{"splice", Entry, false},
	{"truncate", Entry, true}, // compat
	{"umount", Entry, false},
	{"unlink", Entry, false},
	{"unlinkat", Entry, false},
	{"utime", Entry, true}, // compat
	{"utime32", Entry, false},
	{"utimensat", Entry | ExpandTime32, false}, // ExpandTime32
	{"utimensat", Entry, true},                 // compat
	{"utimes", Entry | ExpandTime32, false},    // ExpandTime32
	{"utimes", Entry, true},                    // compat
	{"vfork", Entry, false},

	// Other syscalls that we want to be able to block
	{"_llseek", Entry, false},
	{"_newselect", Entry, false},
	{"_sysctl", Entry, false},
	{"accept", Entry, false},
	{"accept4", Entry, false},
	{"access", Entry, false},
	{"acct", Entry, false},
	{"add_key", Entry, false},
	{"adjtimex", Entry, false},
	{"afs_syscall", Entry, false},
	{"alarm", Entry, false},
	{"arch_prctl", Entry, false},
	{"arm_fadvise64_64", Entry, false},
	{"arm_sync_file_range", Entry, false},
	{"bdflush", Entry, false},
	{"break", Entry, false},
	{"brk", Entry, false},
	{"capget", Entry, false},
	{"chdir", Entry, false},
	{"chown32", Entry, false},
	{"chroot", Entry, false},
	{"clock_adjtime", Entry, false},
	{"clock_getres", Entry, false},
	{"clock_gettime", Entry, false},
	{"clock_nanosleep", Entry, false},
	{"clock_settime", Entry, false},
	{"close", Entry, false},
	{"connect", Entry, false},
	{"copy_file_range", Entry, false},
	{"create_module", Entry, false},
	{"dup", Entry, false},
	{"dup2", Entry, false},
	{"dup3", Entry, false},
	{"epoll_create", Entry, false},
	{"epoll_create1", Entry, false},
	{"epoll_ctl", Entry, false},
	{"epoll_ctl_old", Entry, false},
	{"epoll_pwait", Entry, false},
	{"epoll_wait", Entry, false},
	{"epoll_wait_old", Entry, false},
	{"eventfd", Entry, false},
	{"eventfd2", Entry, false},
	{"faccessat", Entry, false},
	{"fadvise64", Entry, false},
	{"fadvise64_64", Entry, false},
	{"fallocate", Entry, false},
	{"fanotify_init", Entry, false},
	{"fanotify_mark", Entry, false},
	{"fchdir", Entry, false},
	{"fchown32", Entry, false},
	{"fcntl", Entry, false},
	{"fcntl64", Entry, false},
	{"fdatasync", Entry, false},
	{"fgetxattr", Entry, false},
	{"flistxattr", Entry, false},
	{"flock", Entry, false},
	{"fstat", Entry, false},
	{"fstat64", Entry, false},
	{"fstatat64", Entry, false},
	{"fstatfs", Entry, false},
	{"fstatfs64", Entry, false},
	{"fsync", Entry, false},
	{"ftime", Entry, false},
	{"ftruncate", Entry, false},
	{"ftruncate64", Entry, false},
	{"futex", Entry, false},
	{"get_kernel_syms", Entry, false},
	{"get_mempolicy", Entry, false},
	{"get_robust_list", Entry, false},
	{"get_thread_area", Entry, false},
	{"getcpu", Entry, false},
	{"getcwd", Entry, false},
	{"getdents", Entry, false},
	{"getdents64", Entry, false},
	{"getegid", Entry, false},
	{"getegid32", Entry, false},
	{"geteuid", Entry, false},
	{"geteuid32", Entry, false},
	{"getgid", Entry, false},
	{"getgid32", Entry, false},
	{"getgroups", Entry, false},
	{"getgroups32", Entry, false},
	{"getitimer", Entry, false},
	{"getpeername", Entry, false},
	{"getpgid", Entry, false},
	{"getpgrp", Entry, false},
	{"getpid", Entry, false},
	{"getpmsg", Entry, false},
	{"getppid", Entry, false},
	{"getpriority", Entry, false},
	{"getrandom", Entry, false},
	{"getresgid", Entry, false},
	{"getresgid32", Entry, false},
	{"getresuid", Entry, false},
	{"getresuid32", Entry, false},
	{"getrlimit", Entry, false},
	{"getrusage", Entry, false},
	{"getsid", Entry, false},
	{"getsockname", Entry, false},
	{"getsockopt", Entry, false},
	{"gettid", Entry, false},
	{"gettimeofday", Entry, false},
	{"getuid", Entry, false},
	{"getuid32", Entry, false},
	{"getxattr", Entry, false},
	{"gtty", Entry, false},
	{"idle", Entry, false},
	{"inotify_add_watch", Entry, false},
	{"inotify_init", Entry, false},
	{"inotify_init1", Entry, false},
	{"inotify_rm_watch", Entry, false},
	{"io_cancel", Entry, false},
	{"io_destroy", Entry, false},
	{"io_getevents", Entry, false},
	{"io_setup", Entry, false},
	{"io_submit", Entry, false},
	{"ioctl", Entry, false},
	{"ioperm", Entry, false},
	{"iopl", Entry, false},
	{"ioprio_get", Entry, false},
	{"ioprio_set", Entry, false},
	{"ipc", Entry, false},
	{"kcmp", Entry, false},
	{"kexec_file_load", Entry, false},
	{"kexec_load", Entry, false},
	{"keyctl", Entry, false},
	{"lchown32", Entry, false},
	{"lgetxattr", Entry, false},
	{"listen", Entry, false},
	{"listxattr", Entry, false},
	{"llistxattr", Entry, false},
	{"lock", Entry, false},
	{"lookup_dcookie", Entry, false},
	{"lseek", Entry, false},
	{"lstat", Entry, false},
	{"lstat64", Entry, false},
	{"madvise", Entry, false},
	{"mbind", Entry, false},
	{"membarrier", Entry, false},
	{"memfd_create", Entry, false},
	{"migrate_pages", Entry, false},
	{"mincore", Entry, false},
	{"mknod", Entry, false},
	{"mknodat", Entry, false},
	{"mlock", Entry, false},
	{"mlock2", Entry, false},
	{"mlockall", Entry, false},
	{"mmap", Entry, false},
	{"mmap2", Entry, false},
	{"modify_ldt", Entry, false},
	{"move_pages", Entry, false},
	{"mpx", Entry, false},
	{"mq_getsetattr", Entry, false},
	{"mq_notify", Entry, false},
	{"mq_open", Entry, false},
	{"mq_timedreceive", Entry, false},
	{"mq_timedsend", Entry, false},
	{"mq_unlink", Entry, false},
	{"mremap", Entry, false},
	{"msgctl", Entry, false},
	{"msgget", Entry, false},
	{"msgrcv", Entry, false},
	{"msgsnd", Entry, false},
	{"msync", Entry, false},
	{"munlock", Entry, false},
	{"munlockall", Entry, false},
	{"munmap", Entry, false},
	{"name_to_handle_at", Entry, false},
	{"nanosleep", Entry, false},
	{"newfstatat", Entry, false},
	{"nfsservctl", Entry, false},
	{"nice", Entry, false},
	{"oldfstat", Entry, false},
	{"oldlstat", Entry, false},
	{"oldolduname", Entry, false},
	{"oldstat", Entry, false},
	{"olduname", Entry, false},
	{"pause", Entry, false},
	{"pciconfig_iobase", Entry, false},
	{"pciconfig_read", Entry, false},
	{"pciconfig_write", Entry, false},
	{"perf_event_open", Entry, false},
	{"personality", Entry, false},
	{"pipe", Entry, false},
	{"pipe2", Entry, false},
	{"pivot_root", Entry, false},
	{"pkey_alloc", Entry, false},
	{"pkey_free", Entry, false},
	{"pkey_mprotect", Entry, false},
	{"poll", Entry, false},
	{"ppoll", Entry, false},
	{"prctl", Entry, false},
	{"pread64", Entry, false},
	{"preadv", Entry, false},
	{"preadv2", Entry, false},
	{"prlimit64", Entry, false},
	{"process_vm_readv", Entry, false},
	{"process_vm_writev", Entry, false},
	{"prof", Entry, false},
	{"profil", Entry, false},
	{"pselect6", Entry, false},
	{"putpmsg", Entry, false},
	{"pwrite64", Entry, false},
	{"pwritev", Entry, false},
	{"pwritev2", Entry, false},
	{"query_module", Entry, false},
	{"quotactl", Entry, false},
	{"read", Entry, false},
	{"readahead", Entry, false},
	{"readdir", Entry, false},
	{"readlink", Entry, false},
	{"readlinkat", Entry, false},
	{"readv", Entry, false},
	{"reboot", Entry, false},
	{"recv", Entry, false},
	{"recvfrom", Entry, false},
	{"recvmmsg", Entry, false},
	{"recvmsg", Entry, false},
	{"remap_file_pages", Entry, false},
	{"request_key", Entry, false},
	{"restart_syscall", Entry, false},
	{"rt_sigaction", Entry, false},
	{"rt_sigpending", Entry, false},
	{"rt_sigprocmask", Entry, false},
	{"rt_sigqueueinfo", Entry, false},
	{"rt_sigreturn", Entry, false},
	{"rt_sigsuspend", Entry, false},
	{"rt_sigtimedwait", Entry, false},
	{"rt_tgsigqueueinfo", Entry, false},
	{"sched_get_priority_max", Entry, false},
	{"sched_get_priority_min", Entry, false},
	{"sched_getaffinity", Entry, false},
	{"sched_getattr", Entry, false},
	{"sched_getparam", Entry, false},
	{"sched_getscheduler", Entry, false},
	{"sched_rr_get_interval", Entry, false},
	{"sched_setaffinity", Entry, false},
	{"sched_setattr", Entry, false},
	{"sched_setparam", Entry, false},
	{"sched_setscheduler", Entry, false},
	{"sched_yield", Entry, false},
	{"seccomp", Entry, false},
	{"security", Entry, false},
	{"select", Entry, false},
	{"semctl", Entry, false},
	{"semget", Entry, false},
	{"semop", Entry, false},
	{"semtimedop", Entry, false},
	{"send", Entry, false},
	{"sendfile", Entry, false},
	{"sendfile64", Entry, false},
	{"sendmmsg", Entry, false},
	{"sendmsg", Entry, false},
	{"sendto", Entry, false},
	{"set_mempolicy", Entry, false},
	{"set_robust_list", Entry, false},
	{"set_thread_area", Entry, false},
	{"set_tid_address", Entry, false},
	{"setdomainname", Entry, false},
	{"setfsgid32", Entry, false},
	{"setfsuid32", Entry, false},
	{"setgid32", Entry, false},
	{"setgroups", Entry, false},
	{"setgroups32", Entry, false},
	{"sethostname", Entry, false},
	{"setitimer", Entry, false},
	{"setns", Entry, false},
	{"setpgid", Entry, false},
	{"setpriority", Entry, false},
	{"setregid32", Entry, false},
	{"setresgid32", Entry, false},
	{"setresuid32", Entry, false},
	{"setreuid32", Entry, false},
	{"setrlimit", Entry, false},
	{"setsid", Entry, false},
	{"setsockopt", Entry, false},
	{"settimeofday", Entry, false},
	{"setuid32", Entry, false},
	{"sgetmask", Entry, false},
	{"shmat", Entry, false},
	{"shmctl", Entry, false},
	{"shmdt", Entry, false},
	{"shmget", Entry, false},
	{"shutdown", Entry, false},
	{"sigaction", Entry, false},
	{"sigaltstack", Entry, false},
	{"signal", Entry, false},
	{"signalfd", Entry, false},
	{"signalfd4", Entry, false},
	{"sigpending", Entry, false},
	{"sigprocmask", Entry, false},
	{"sigreturn", Entry, false},
	{"sigsuspend", Entry, false},
	{"socket", Entry, false},
	{"socketcall", Entry, false},
	{"socketpair", Entry, false},
	{"ssetmask", Entry, false},
	{"stat", Entry, false},
	{"stat64", Entry, false},
	{"statfs", Entry, false},
	{"statfs64", Entry, false},
	{"statx", Entry, false},
	{"stime", Entry, false},
	{"stty", Entry, false},
	{"swapoff", Entry, false},
	{"swapon", Entry, false},
	{"symlink", Entry, false},
	{"symlinkat", Entry, false},
	{"sync", Entry, false},
	{"sync_file_range", Entry, false},
	{"sync_file_range2", Entry, false},
	{"syncfs", Entry, false},
	{"sysfs", Entry, false},
	{"sysinfo", Entry, false},
	{"syslog", Entry, false},
	{"tee", Entry, false},
	{"tgkill", Entry, false},
	{"time", Entry, false},
	{"timer_create", Entry, false},
	{"timer_delete", Entry, false},
	{"timer_getoverrun", Entry, false},
	{"timer_gettime", Entry, false},
	{"timer_settime", Entry, false},
	{"timerfd_create", Entry, false},
	{"timerfd_gettime", Entry, false},
	{"timerfd_settime", Entry, false},
	{"times", Entry, false},
	{"tkill", Entry, false},
	{"truncate64", Entry, false},
	{"tuxcall", Entry, false},
	{"ugetrlimit", Entry, false},
	{"ulimit", Entry, false},
	{"umask", Entry, false},
	{"umount2", Entry, false},
	{"uname", Entry, false},
	{"uselib", Entry, false},
	{"userfaultfd", Entry, false},
	{"ustat", Entry, false},
	{"vhangup", Entry, false},
	{"vm86", Entry, false},
	{"vm86old", Entry, false},
	{"vmsplice", Entry, false},
	{"vserver", Entry, false},
	{"wait4", Entry, false},
	{"waitid", Entry, false},
	{"waitpid", Entry, false},
	{"write", Entry, false},
	{"writev", Entry, false},

	// we should not block exit, since it will just .. exit.
	// {"exit", Entry, false},
	// {"exit_group", Entry, false},

	// don't work, dunno why yet
	// {"unshare", Entry, false},
}

func isProbeAlreadyPresent(probes []*manager.Probe, name string) bool {
	for _, probe := range probes {
		if probe.EBPFSection == name {
			return true
		}
	}
	return false
}

// getOtherBlockingProbes will return the list of syscall kprobes that are not yet defined (basically,
// the list of "Other syscalls that we want to be able to block")
func getOtherBlockingProbes(currentProbes []*manager.Probe) []*manager.Probe {
	res := []*manager.Probe{}

	for _, syscall := range allSyscalls {
		probes := ExpandSyscallProbes(&manager.Probe{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID: SecurityAgentUID,
			},
			SyscallFuncName: syscall.name,
		}, syscall.flag, syscall.compatMode)
		for _, probe := range probes {
			if !isProbeAlreadyPresent(currentProbes, probe.EBPFSection) {
				fmt.Printf("getOtherBlockingProbes adding %s (%s)\n", syscall.name, probe.EBPFSection)
				res = append(res, probe)
			}
		}
	}
	return res
}

// allSyscallsIDs map the syscalls names with their ID, filled up by GetSyscallsIdConstants
var allSyscallsIDs map[string][]int

// GetSyscallsIdConstants return the list of constants for ALL of syscalls, also, generate string to id table
func GetSyscallsIdConstants() []manager.ConstantEditor {
	var syscallIDs []manager.ConstantEditor

	allSyscallsIDs = make(map[string][]int)
	for id, syscall := range allSyscalls {
		fmt.Printf("GetSyscallsIdConstants adding constant (%d) for syscall (%s)\n", id+1, syscall.name)
		allSyscallsIDs[syscall.name] = append(allSyscallsIDs[syscall.name], id+1)
		syscallIDs = append(syscallIDs, manager.ConstantEditor{
			Name:                     "syscall_id",
			Value:                    uint64(id + 1), // +1 to avoid an id of 0, because LOAD_CONSTANT will return 0 if it doesnt find any
			ProbeIdentificationPairs: ExpandSyscallProbePair(SecurityAgentUID, syscall.name, syscall.flag, syscall.compatMode),
		})
	}
	return syscallIDs
}

func isProbeSelectorAlreadyPresent(probes []manager.ProbesSelector, probe manager.ProbesSelector) bool {
	for _, p := range probes {
		if probe == p {
			return true
		}
	}
	return false
}

// GetYetUnregisteredButNeedProbes will return the list of needed probes to be loaded to be able to block the list of
// specified syscalls
func GetYetUnregisteredButNeedProbes(currentProbes []manager.ProbesSelector, blockedSyscalls []string) []manager.ProbesSelector {
	var probesToAdd []manager.ProbesSelector

	// If "all" is supplied we have to return load them all
	if slices.Contains(blockedSyscalls, "all") {
		for _, syscall := range allSyscalls {
			probe := &manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, syscall.name, syscall.flag, syscall.compatMode)}
			if !isProbeSelectorAlreadyPresent(currentProbes, probe) {
				fmt.Printf("GetYetUnregisteredButNeedProbes adding probe: %s\n", syscall.name)
				probesToAdd = append(probesToAdd, probe)
			}
		}
	} else {
		for _, syscall := range allSyscalls {
			if slices.Contains(blockedSyscalls, syscall.name) {
				probe := &manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, syscall.name, syscall.flag, syscall.compatMode)}
				// We add the needed probes only if they are not yet pre-loaded
				if !isProbeSelectorAlreadyPresent(currentProbes, probe) {
					fmt.Printf("GetYetUnregisteredButNeedProbes adding probe: %s\n", syscall.name)
					probesToAdd = append(probesToAdd, probe)
				}
			}
		}
	}
	return probesToAdd
}

// GetSyscallsIDs returns the list of IDs for given list of syscall names.
// NB: a syscall can have multiple instance, and so multiple IDs (cf utimes).
func GetSyscallsIDs(syscalls []string) []int {
	var res []int

	for _, s := range syscalls {
		var ids []int
		ids, found := allSyscallsIDs[s]
		if found {
			res = append(res, ids...)
		}
	}
	return res
}
