// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probes

import (
	"github.com/DataDog/ebpf/manager"

	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

// SyscallMonitorSelectors is the list of probes that should be activated for the syscall monitor feature
var SyscallMonitorSelectors = []manager.ProbesSelector{
	&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "tracepoint/raw_syscalls/sys_enter"}},
	&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "tracepoint/sched/sched_process_exec"}},
}

// SelectorsPerEventType is the list of probes that should be activated for each event
var SelectorsPerEventType = map[eval.EventType][]manager.ProbesSelector{

	// The following events will always be activated, regardless of the rules loaded
	"*": {
		// Exec probes
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "tracepoint/raw_syscalls/sys_exit"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "tracepoint/sched/sched_process_fork"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/do_exit"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/security_bprm_committed_creds"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/exit_itimers"}},
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/prepare_binprm"}},
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/bprm_execve"}},
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/security_bprm_check"}},
			}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/vfs_open"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/do_dentry_open"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/commit_creds"}},
		}},
		&manager.OneOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/cgroup_procs_write"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/cgroup1_procs_write"}},
		}},
		&manager.OneOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/_do_fork"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/do_fork"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/kernel_clone"}},
		}},
		&manager.OneOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/cgroup_tasks_write"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/cgroup1_tasks_write"}},
		}},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "execve"}, Entry),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "execveat"}, Entry),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "setuid"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "setuid16"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "setgid"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "setgid16"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "seteuid"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "seteuid16"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "setegid"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "setegid16"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "setfsuid"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "setfsuid16"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "setfsgid"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "setfsgid16"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "setreuid"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "setreuid16"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "setregid"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "setregid16"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "setresuid"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "setresuid16"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "setresgid"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "setresgid16"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "capset"}, EntryAndExit),
		},

		// Open probes
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/vfs_truncate"}},
		}},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "open"}, EntryAndExit, true),
		},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "creat"}, EntryAndExit),
		},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "truncate"}, EntryAndExit, true),
		},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "openat"}, EntryAndExit, true),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "openat2"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "open_by_handle_at"}, EntryAndExit, true),
		},
		&manager.BestEffort{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/io_openat2"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kretprobe/io_openat2"}},
		}},
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/filp_close"}},
		}},

		// Mount probes
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/attach_recursive_mnt"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/propagate_mnt"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/security_sb_umount"}},
		}},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "mount"}, EntryAndExit, true),
		},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "umount"}, EntryAndExit),
		},

		// Rename probes
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/vfs_rename"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/mnt_want_write"}},
		}},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "rename"}, EntryAndExit),
		},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "renameat"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "renameat2"}, EntryAndExit),
		},

		// unlink rmdir probes
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/mnt_want_write"}},
		}},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "unlinkat"}, EntryAndExit),
		},

		// Rmdir probes
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/security_inode_rmdir"}},
		}},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "rmdir"}, EntryAndExit),
		},

		// Unlink probes
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/vfs_unlink"}},
		}},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "unlink"}, EntryAndExit),
		},

		// ioctl probes
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/do_vfs_ioctl"}},
		}},

		// snapshot
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/security_inode_getattr"}},
		}},
	},

	// List of probes to activate to capture chmod events
	"chmod": {
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/security_inode_setattr"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/mnt_want_write"}},
		}},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "chmod"}, EntryAndExit),
		},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "fchmod"}, EntryAndExit),
		},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "fchmodat"}, EntryAndExit),
		},
	},

	// List of probes to activate to capture chown events
	"chown": {
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/security_inode_setattr"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/mnt_want_write"}},
		}},
		&manager.OneOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/mnt_want_write_file"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/mnt_want_write_file_path"}},
		}},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "chown"}, EntryAndExit),
		},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "chown16"}, EntryAndExit),
		},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "fchown"}, EntryAndExit),
		},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "fchown16"}, EntryAndExit),
		},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "fchownat"}, EntryAndExit),
		},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "lchown"}, EntryAndExit),
		},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "lchown16"}, EntryAndExit),
		},
	},

	// List of probes to activate to capture link events
	"link": {
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/vfs_link"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/filename_create"}},
		}},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "link"}, EntryAndExit),
		},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "linkat"}, EntryAndExit),
		},
	},

	// List of probes to activate to capture mkdir events
	"mkdir": {
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/vfs_mkdir"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/filename_create"}},
		}},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "mkdir"}, EntryAndExit),
		},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "mkdirat"}, EntryAndExit),
		},
	},

	// List of probes to activate to capture removexattr events
	"removexattr": {
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/vfs_removexattr"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/mnt_want_write"}},
		}},
		&manager.OneOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/mnt_want_write_file"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/mnt_want_write_file_path"}},
		}},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "removexattr"}, EntryAndExit),
		},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "fremovexattr"}, EntryAndExit),
		},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "lremovexattr"}, EntryAndExit),
		},
	},

	// List of probes to activate to capture setxattr events
	"setxattr": {
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/vfs_setxattr"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/mnt_want_write"}},
		}},
		&manager.OneOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/mnt_want_write_file"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/mnt_want_write_file_path"}},
		}},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "setxattr"}, EntryAndExit),
		},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "fsetxattr"}, EntryAndExit),
		},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "lsetxattr"}, EntryAndExit),
		},
	},

	// List of probes to activate to capture utimes events
	"utimes": {
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/security_inode_setattr"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/mnt_want_write"}},
		}},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "utime"}, EntryAndExit, true),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "utime32"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "utimes"}, EntryAndExit, true),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "utimes"}, EntryAndExit|ExpandTime32),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "utimensat"}, EntryAndExit, true),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "utimensat"}, EntryAndExit|ExpandTime32),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "futimesat"}, EntryAndExit, true),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "futimesat"}, EntryAndExit|ExpandTime32),
		},
	},

	"selinux": {
		// This needs to be best effort, as sel_write_disable is in the process to be removed
		&manager.BestEffort{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/sel_write_disable"}},
		}},
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/sel_write_enforce"}},
		}},
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/sel_write_bool"}},
		}},
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, Section: "kprobe/sel_commit_bools_write"}},
		}},
	},
}
