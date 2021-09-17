// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probes

import (
	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

// SyscallMonitorSelectors is the list of probes that should be activated for the syscall monitor feature
var SyscallMonitorSelectors = []manager.ProbesSelector{
	&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "tracepoint/raw_syscalls/sys_enter", EBPFFuncName: "sys_enter"}},
	&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "tracepoint/sched/sched_process_exec", EBPFFuncName: "sched_process_exec"}},
}

// SelectorsPerEventType is the list of probes that should be activated for each event
var SelectorsPerEventType = map[eval.EventType][]manager.ProbesSelector{

	// The following events will always be activated, regardless of the rules loaded
	"*": {
		// Exec probes
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "tracepoint/raw_syscalls/sys_exit", EBPFFuncName: "sys_exit"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "tracepoint/sched/sched_process_fork", EBPFFuncName: "sched_process_fork"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/do_exit", EBPFFuncName: "kprobe_do_exit"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/security_bprm_committed_creds", EBPFFuncName: "kprobe_security_bprm_committed_creds"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/exit_itimers", EBPFFuncName: "kprobe_exit_itimers"}},
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/prepare_binprm", EBPFFuncName: "kprobe_prepare_binprm"}},
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/bprm_execve", EBPFFuncName: "kprobe_bprm_execve"}},
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/security_bprm_check", EBPFFuncName: "kprobe_security_bprm_check"}},
			}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/vfs_open", EBPFFuncName: "kprobe_vfs_open"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/do_dentry_open", EBPFFuncName: "kprobe_do_dentry_open"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/commit_creds", EBPFFuncName: "kprobe_commit_creds"}},
		}},
		&manager.OneOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/cgroup_procs_write", EBPFFuncName: "kprobe_cgroup_procs_write"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/cgroup1_procs_write", EBPFFuncName: "kprobe_cgroup1_procs_write"}},
		}},
		&manager.OneOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/_do_fork", EBPFFuncName: "kprobe__do_fork"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/do_fork", EBPFFuncName: "kprobe_do_fork"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/kernel_clone", EBPFFuncName: "kprobe_kernel_clone"}},
		}},
		&manager.OneOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/cgroup_tasks_write", EBPFFuncName: "kprobe_cgroup_tasks_write"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/cgroup1_tasks_write", EBPFFuncName: "kprobe_cgroup1_tasks_write"}},
		}},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "execve"}, Entry),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "execveat"}, Entry),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "setuid"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "setuid16"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "setgid"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "setgid16"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "seteuid"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "seteuid16"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "setegid"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "setegid16"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "setfsuid"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "setfsuid16"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "setfsgid"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "setfsgid16"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "setreuid"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "setreuid16"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "setregid"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "setregid16"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "setresuid"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "setresuid16"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "setresgid"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "setresgid16"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "capset"}, EntryAndExit),
		},

		// Open probes
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/vfs_truncate", EBPFFuncName: "kprobe_vfs_truncate"}},
		}},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "open"}, EntryAndExit, true),
		},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "creat"}, EntryAndExit),
		},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "truncate"}, EntryAndExit, true),
		},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "openat"}, EntryAndExit, true),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "openat2"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "open_by_handle_at"}, EntryAndExit, true),
		},
		&manager.BestEffort{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/io_openat2", EBPFFuncName: "kprobe_io_openat2"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kretprobe/io_openat2", EBPFFuncName: "kretprobe_io_openat2"}},
		}},
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/filp_close", EBPFFuncName: "kprobe_filp_close"}},
		}},

		// Mount probes
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/attach_recursive_mnt", EBPFFuncName: "kprobe_attach_recursive_mnt"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/propagate_mnt", EBPFFuncName: "kprobe_propagate_mnt"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/security_sb_umount", EBPFFuncName: "kprobe_security_sb_umount"}},
		}},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "mount"}, EntryAndExit, true),
		},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "umount"}, EntryAndExit),
		},

		// Rename probes
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/vfs_rename", EBPFFuncName: "kprobe_vfs_rename"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/mnt_want_write", EBPFFuncName: "kprobe_mnt_want_write"}},
		}},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "rename"}, EntryAndExit),
		},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "renameat"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "renameat2"}, EntryAndExit),
		},

		// unlink rmdir probes
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/mnt_want_write", EBPFFuncName: "kprobe_mnt_want_write"}},
		}},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "unlinkat"}, EntryAndExit),
		},

		// Rmdir probes
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/security_inode_rmdir", EBPFFuncName: "kprobe_security_inode_rmdir"}},
		}},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "rmdir"}, EntryAndExit),
		},

		// Unlink probes
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/vfs_unlink", EBPFFuncName: "kprobe_vfs_unlink"}},
		}},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "unlink"}, EntryAndExit),
		},

		// ioctl probes
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/do_vfs_ioctl", EBPFFuncName: "kprobe_do_vfs_ioctl"}},
		}},

		// snapshot
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/security_inode_getattr", EBPFFuncName: "kprobe_security_inode_getattr"}},
		}},
	},

	// List of probes to activate to capture chmod events
	"chmod": {
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/security_inode_setattr", EBPFFuncName: "kprobe_security_inode_setattr"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/mnt_want_write", EBPFFuncName: "kprobe_mnt_want_write"}},
		}},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "chmod"}, EntryAndExit),
		},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "fchmod"}, EntryAndExit),
		},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "fchmodat"}, EntryAndExit),
		},
	},

	// List of probes to activate to capture chown events
	"chown": {
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/security_inode_setattr", EBPFFuncName: "kprobe_security_inode_setattr"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/mnt_want_write", EBPFFuncName: "kprobe_mnt_want_write"}},
		}},
		&manager.OneOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/mnt_want_write_file", EBPFFuncName: "kprobe_mnt_want_write_file"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/mnt_want_write_file_path", EBPFFuncName: "kprobe_mnt_want_write_file_path"}},
		}},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "chown"}, EntryAndExit),
		},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "chown16"}, EntryAndExit),
		},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "fchown"}, EntryAndExit),
		},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "fchown16"}, EntryAndExit),
		},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "fchownat"}, EntryAndExit),
		},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "lchown"}, EntryAndExit),
		},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "lchown16"}, EntryAndExit),
		},
	},

	// List of probes to activate to capture link events
	"link": {
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/vfs_link", EBPFFuncName: "kprobe_vfs_link"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/filename_create", EBPFFuncName: "kprobe_filename_create"}},
		}},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "link"}, EntryAndExit),
		},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "linkat"}, EntryAndExit),
		},
	},

	// List of probes to activate to capture mkdir events
	"mkdir": {
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/vfs_mkdir", EBPFFuncName: "kprobe_vfs_mkdir"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/filename_create", EBPFFuncName: "kprobe_filename_create"}},
		}},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "mkdir"}, EntryAndExit),
		},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "mkdirat"}, EntryAndExit),
		},
	},

	// List of probes to activate to capture removexattr events
	"removexattr": {
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/vfs_removexattr", EBPFFuncName: "kprobe_vfs_removexattr"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/mnt_want_write", EBPFFuncName: "kprobe_mnt_want_write"}},
		}},
		&manager.OneOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/mnt_want_write_file", EBPFFuncName: "kprobe_mnt_want_write_file"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/mnt_want_write_file_path", EBPFFuncName: "kprobe_mnt_want_write_file_path"}},
		}},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "removexattr"}, EntryAndExit),
		},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "fremovexattr"}, EntryAndExit),
		},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "lremovexattr"}, EntryAndExit),
		},
	},

	// List of probes to activate to capture setxattr events
	"setxattr": {
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/vfs_setxattr", EBPFFuncName: "kprobe_vfs_setxattr"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/mnt_want_write", EBPFFuncName: "kprobe_mnt_want_write"}},
		}},
		&manager.OneOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/mnt_want_write_file", EBPFFuncName: "kprobe_mnt_want_write_file"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/mnt_want_write_file_path", EBPFFuncName: "kprobe_mnt_want_write_file_path"}},
		}},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "setxattr"}, EntryAndExit),
		},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "fsetxattr"}, EntryAndExit),
		},
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "lsetxattr"}, EntryAndExit),
		},
	},

	// List of probes to activate to capture utimes events
	"utimes": {
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/security_inode_setattr", EBPFFuncName: "kprobe_security_inode_setattr"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/mnt_want_write", EBPFFuncName: "kprobe_mnt_want_write"}},
		}},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "utime"}, EntryAndExit, true),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "utime32"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "utimes"}, EntryAndExit, true),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "utimes"}, EntryAndExit|ExpandTime32),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "utimensat"}, EntryAndExit, true),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "utimensat"}, EntryAndExit|ExpandTime32),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "futimesat"}, EntryAndExit, true),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "futimesat"}, EntryAndExit|ExpandTime32),
		},
	},

	"selinux": {
		// This needs to be best effort, as sel_write_disable is in the process to be removed
		&manager.BestEffort{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/sel_write_disable", EBPFFuncName: "kprobe_sel_write_disable"}},
		}},
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/sel_write_enforce", EBPFFuncName: "kprobe_sel_write_enforce"}},
		}},
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/sel_write_bool", EBPFFuncName: "kprobe_sel_write_bool"}},
		}},
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/sel_commit_bools_write", EBPFFuncName: "kprobe_sel_commit_bools_write"}},
		}},
	},
}
