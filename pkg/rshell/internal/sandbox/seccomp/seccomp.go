// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package seccomp installs the syscall restrictions used by the privileged
// rshell worker.
package seccomp

import "errors"

// ErrUnsupported is returned when syscall filtering is unavailable on the
// current operating system.
var ErrUnsupported = errors.New("seccomp is not supported on this platform")

// defaultDenylist is deliberately a blocklist, rather than an allowlist. The
// rshell worker and Go runtime need a broad set of ordinary syscalls, while the
// operations below are not part of executing rshell builtins.
//
// Keep this list explicit and reviewed. In particular:
//   - clone is denied unless its flags exactly match the Go runtime's thread
//     creation flags; clone3 is denied unconditionally;
//   - setresuid remains allowed for the helper's controlled UID elevation;
//   - prctl and ioctl are denied only after no_new_privs, parent-death signal,
//     and other worker setup is complete.
var defaultDenylist = []string{
	// Create processes or replace the worker image. clone is filtered by flags.
	"clone",
	"clone3",
	"fork",
	"vfork",
	"execve",
	"execveat",
	"kill",

	// Change credentials or capabilities. setresuid is intentionally omitted.
	"setuid",
	"setgid",
	"setreuid",
	"setregid",
	"setresgid",
	"setgroups",
	"setfsuid",
	"setfsgid",
	"capset",
	"prctl",

	// Change namespaces, roots, or mount topology, including the new mount API.
	"unshare",
	"setns",
	"mount",
	"umount2",
	"pivot_root",
	"chroot",
	"open_tree",
	"move_mount",
	"fsopen",
	"fsconfig",
	"fsmount",
	"mount_setattr",
	"sethostname",
	"setdomainname",
	"settimeofday",
	"adjtimex",
	"clock_settime",
	"clock_adjtime",

	// Reach privileged kernel instrumentation or another process's memory.
	"bpf",
	"perf_event_open",
	"ptrace",
	"process_vm_readv",
	"process_vm_writev",
	"process_madvise",
	"process_mrelease",
	"pidfd_getfd",
	"pidfd_send_signal",
	"kcmp",

	// Access the kernel keyring.
	"keyctl",
	"add_key",
	"request_key",

	// Load, unload, or replace kernel code.
	"init_module",
	"finit_module",
	"delete_module",
	"create_module",
	"query_module",
	"get_kernel_syms",
	"kexec_load",
	"kexec_file_load",
	"reboot",

	// Create device nodes or modify global kernel/filesystem configuration.
	"mknod",
	"mknodat",
	"ioctl",
	"chmod",
	"fchmod",
	"fchmodat",
	"fchmodat2",
	"chown",
	"fchown",
	"lchown",
	"fchownat",
	"setxattr",
	"lsetxattr",
	"fsetxattr",
	"removexattr",
	"lremovexattr",
	"fremovexattr",
	"utime",
	"utimes",
	"futimesat",
	"utimensat",
	"swapon",
	"swapoff",
	"acct",
	"quotactl",
	"quotactl_fd",
	"setpriority",
	"sched_setparam",
	"sched_setscheduler",
	"sched_setattr",
	"sched_setaffinity",
	"ioprio_set",

	// APIs that expose broad kernel attack surface or bypass normal path opens.
	"io_uring_setup",
	"io_uring_enter",
	"io_uring_register",
	"userfaultfd",
	"open_by_handle_at",
	"name_to_handle_at",
	"fanotify_init",
	"fanotify_mark",
	"iopl",
	"ioperm",
	"syslog",
	"lookup_dcookie",
	"vhangup",
}

// DefaultDenylist returns a copy of the reviewed privileged-worker denylist.
// Callers may safely modify the returned slice.
func DefaultDenylist() []string {
	return append([]string(nil), defaultDenylist...)
}

// RestrictDefault installs the reviewed privileged-worker denylist.
func RestrictDefault() error {
	return Restrict(DefaultDenylist())
}
