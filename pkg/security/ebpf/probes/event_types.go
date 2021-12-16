// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probes

import (
	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// NetworkTrackingSelectors is the list of probes that should be activated when the network is enabled
var NetworkTrackingSelectors = []manager.ProbesSelector{
	// flow classification probes
	&manager.AllOf{Selectors: []manager.ProbesSelector{
		&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/security_socket_bind", EBPFFuncName: "kprobe_security_socket_bind"}},
		&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/security_sk_classify_flow", EBPFFuncName: "kprobe_security_sk_classify_flow"}},
	}},

	// network device probes
	&manager.AllOf{Selectors: []manager.ProbesSelector{
		&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/veth_newlink", EBPFFuncName: "kprobe_veth_newlink"}},
		&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/register_netdevice", EBPFFuncName: "kprobe_register_netdevice"}},
		&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/dev_change_net_namespace", EBPFFuncName: "kprobe_dev_change_net_namespace"}},
		&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kretprobe/register_netdevice", EBPFFuncName: "kretprobe_register_netdevice"}},
	}},
	&manager.BestEffort{Selectors: []manager.ProbesSelector{
		&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/dev_get_valid_name", EBPFFuncName: "kprobe_dev_get_valid_name"}},
		&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/dev_new_index", EBPFFuncName: "kprobe_dev_new_index"}},
		&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kretprobe/dev_new_index", EBPFFuncName: "kretprobe_dev_new_index"}},
		&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/__dev_get_by_index", EBPFFuncName: "kprobe___dev_get_by_index"}},
		&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/__dev_get_by_name", EBPFFuncName: "kprobe___dev_get_by_name"}},
	}},
}

// SyscallMonitorSelectors is the list of probes that should be activated for the syscall monitor feature
var SyscallMonitorSelectors = []manager.ProbesSelector{
	&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "tracepoint/raw_syscalls/sys_enter", EBPFFuncName: "sys_enter"}},
	&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "tracepoint/sched/sched_process_exec", EBPFFuncName: "sched_process_exec"}},
}

// SelectorsPerEventType is the list of probes that should be activated for each event
var SelectorsPerEventType = map[eval.EventType][]manager.ProbesSelector{

	// The following probes will always be activated, regardless of the loaded rules
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
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kretprobe/__task_pid_nr_ns", EBPFFuncName: "kretprobe__task_pid_nr_ns"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kretprobe/alloc_pid", EBPFFuncName: "kretprobe_alloc_pid"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/switch_task_namespaces", EBPFFuncName: "kprobe_switch_task_namespaces"}},
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

		// Link
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

		// selinux
		// This needs to be best effort, as sel_write_disable is in the process to be removed
		&manager.BestEffort{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/sel_write_disable", EBPFFuncName: "kprobe_sel_write_disable"}},
		}},
		&manager.BestEffort{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/sel_write_enforce", EBPFFuncName: "kprobe_sel_write_enforce"}},
		}},
		&manager.BestEffort{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/sel_write_bool", EBPFFuncName: "kprobe_sel_write_bool"}},
		}},
		&manager.BestEffort{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/sel_commit_bools_write", EBPFFuncName: "kprobe_sel_commit_bools_write"}},
		}},
	},

	// List of probes required to capture chmod events
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

	// List of probes required to capture chown events
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

	// List of probes required to capture mkdir events
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

	// List of probes required to capture removexattr events
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

	// List of probes required to capture setxattr events
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

	// List of probes required to capture utimes events
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

	// List of probes required to capture bpf events
	"bpf": {
		&manager.BestEffort{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/security_bpf_map", EBPFFuncName: "kprobe_security_bpf_map"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/security_bpf_prog", EBPFFuncName: "kprobe_security_bpf_prog"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/check_helper_call", EBPFFuncName: "kprobe_check_helper_call"}},
		}},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "bpf"}, EntryAndExit),
		},
	},

	// List of probes required to capture ptrace events
	"ptrace": {
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "ptrace"}, EntryAndExit),
		},
	},

	// List of probes required to capture mmap events
	"mmap": {
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "mmap"}, Exit),
		},
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "tracepoint/syscalls/sys_enter_mmap", EBPFFuncName: "tracepoint_syscalls_sys_enter_mmap"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kretprobe/fget", EBPFFuncName: "kretprobe_fget"}},
		}},
	},

	// List of probes required to capture mprotect events
	"mprotect": {
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/security_file_mprotect", EBPFFuncName: "kprobe_security_file_mprotect"}},
		}},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "mprotect"}, EntryAndExit),
		},
	},

	// List of probes required to capture kernel load_module events
	"load_module": {
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.OneOf{Selectors: []manager.ProbesSelector{
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/security_kernel_read_file", EBPFFuncName: "kprobe_security_kernel_read_file"}},
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/security_kernel_module_from_file", EBPFFuncName: "kprobe_security_kernel_module_from_file"}},
			}},
			&manager.OneOf{Selectors: []manager.ProbesSelector{
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/do_init_module", EBPFFuncName: "kprobe_do_init_module"}},
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/module_put", EBPFFuncName: "kprobe_module_put"}},
			}},
		}},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "init_module"}, EntryAndExit),
		},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "finit_module"}, EntryAndExit),
		},
	},

	// List of probes required to capture kernel unload_module events
	"unload_module": {
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "delete_module"}, EntryAndExit),
		},
	},

	// List of probes required to capture signal events
	"signal": {
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID,
				EBPFSection: "kretprobe/check_kill_permission", EBPFFuncName: "kretprobe_check_kill_permission"}},
		}},
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kill"}, Entry),
		},
	},

	// List of probes required to capture splice events
	"splice": {
		&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(
			manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "splice"}, EntryAndExit),
		},
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kprobe/get_pipe_info", EBPFFuncName: "kprobe_get_pipe_info"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFSection: "kretprobe/get_pipe_info", EBPFFuncName: "kretprobe_get_pipe_info"}},
		}},
	},
}
