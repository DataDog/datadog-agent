// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package probes

import (
	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// NetworkNFNatSelectors is the list of probes that should be activated if the `nf_nat` module is loaded
var NetworkNFNatSelectors = []manager.ProbesSelector{
	&manager.OneOf{Selectors: []manager.ProbesSelector{
		&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kprobe_nf_nat_manip_pkt"}},
		&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kprobe_nf_nat_packet"}},
	}},
}

// NetworkVethSelectors is the list of probes that should be activated if the `veth` module is loaded
var NetworkVethSelectors = []manager.ProbesSelector{
	&manager.AllOf{Selectors: []manager.ProbesSelector{
		&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kprobe_rtnl_create_link"}},
	}},
}

// NetworkSelectors is the list of probes that should be activated when the network is enabled
func NetworkSelectors(fentry bool) []manager.ProbesSelector {
	return []manager.ProbesSelector{
		// flow classification probes
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			kprobeOrFentry("security_socket_bind", fentry),
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kprobe_security_sk_classify_flow"}},
			kprobeOrFentry("path_get", fentry),
			kprobeOrFentry("proc_fd_link", fentry),
		}},

		// network device probes
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kprobe_register_netdevice"}},
			&manager.OneOf{Selectors: []manager.ProbesSelector{
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kprobe_dev_change_net_namespace"}},
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kprobe___dev_change_net_namespace"}},
			}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kretprobe_register_netdevice"}},
		}},
		&manager.BestEffort{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kprobe_dev_get_valid_name"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kprobe_dev_new_index"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kretprobe_dev_new_index"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kprobe___dev_get_by_index"}},
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kprobe___dev_get_by_name"}},
		}},
	}
}

// SyscallMonitorSelectors is the list of probes that should be activated for the syscall monitor feature
var SyscallMonitorSelectors = []manager.ProbesSelector{
	&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "sys_enter"}},
}

// SnapshotSelectors selectors required during the snapshot
func SnapshotSelectors(fentry bool) []manager.ProbesSelector {
	return []manager.ProbesSelector{
		// required to stat /proc/.../exe
		kprobeOrFentry("security_inode_getattr", fentry),
	}
}

var selectorsPerEventTypeStore map[eval.EventType][]manager.ProbesSelector

// GetSelectorsPerEventType returns the list of probes that should be activated for each event
func GetSelectorsPerEventType(fentry bool) map[eval.EventType][]manager.ProbesSelector {
	if selectorsPerEventTypeStore != nil {
		return selectorsPerEventTypeStore
	}

	selectorsPerEventTypeStore = map[eval.EventType][]manager.ProbesSelector{
		// The following probes will always be activated, regardless of the loaded rules
		"*": {
			// Exec probes
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "sys_exit"}},
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "sched_process_fork"}},
				kprobeOrFentry("do_exit", fentry),
				&manager.BestEffort{Selectors: []manager.ProbesSelector{
					kprobeOrFentry("prepare_binprm", fentry, withSkipIfFentry(true)),
					kprobeOrFentry("bprm_execve", fentry),
					kprobeOrFentry("security_bprm_check", fentry, withSkipIfFentry(true)),
				}},
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kprobe_setup_new_exec_interp"}},
				kprobeOrFentry("setup_new_exec_args_envs", fentry, withUid(SecurityAgentUID+"_a")),
				kprobeOrFentry("setup_arg_pages", fentry),
				kprobeOrFentry("mprotect_fixup", fentry),
				kprobeOrFentry("exit_itimers", fentry),
				kprobeOrFentry("vfs_open", fentry),
				kprobeOrFentry("do_dentry_open", fentry),
				kprobeOrFentry("commit_creds", fentry),
				kprobeOrFentry("switch_task_namespaces", fentry),
				kprobeOrFentry("do_coredump", fentry),
			}},
			&manager.OneOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("cgroup_procs_write", fentry),
				kprobeOrFentry("cgroup1_procs_write", fentry),
			}},
			&manager.OneOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("_do_fork", fentry, withSkipIfFentry(true)),
				kprobeOrFentry("do_fork", fentry, withSkipIfFentry(true)),
				kprobeOrFentry("kernel_clone", fentry),
			}},
			&manager.OneOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("cgroup_tasks_write", fentry, withSkipIfFentry(true)),
				kprobeOrFentry("cgroup1_tasks_write", fentry, withSkipIfFentry(true)),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "execve", fentry, Entry|SupportFentry)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "execveat", fentry, Entry|SupportFentry)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setuid", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setuid16", fentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setgid", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setgid16", fentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setfsuid", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setfsuid16", fentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setfsgid", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setfsgid16", fentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setreuid", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setreuid16", fentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setregid", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setregid16", fentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setresuid", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setresuid16", fentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setresgid", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setresgid16", fentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "capset", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "fork", fentry, Entry)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "vfork", fentry, Entry)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "clone", fentry, Entry|SupportFentry)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "clone3", fentry, Entry|SupportFentry)},

			// File Attributes
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kprobe_security_inode_setattr"}},

			// Open probes
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("vfs_truncate", fentry),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "open", fentry, EntryAndExit|SupportFentry, true)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "creat", fentry, EntryAndExit|SupportFentry)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "truncate", fentry, EntryAndExit|SupportFentry, true)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "openat", fentry, EntryAndExit|SupportFentry, true)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "openat2", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "open_by_handle_at", fentry, EntryAndExit|SupportFentry, true)},
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("io_openat", fentry),
				kprobeOrFentry("io_openat2", fentry),
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kretprobe_io_openat2"}},
			}},
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("filp_close", fentry),
			}},

			// iouring
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "io_uring_create"}},
				&manager.OneOf{Selectors: []manager.ProbesSelector{
					kprobeOrFentry("io_allocate_scq_urings", fentry),
					kprobeOrFentry("io_sq_offload_start", fentry, withSkipIfFentry(true)),
					kretprobeOrFexit("io_ring_ctx_alloc", fentry),
				}},
			}},

			// Mount probes
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kprobe_attach_recursive_mnt"}},
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kprobe_propagate_mnt"}},
				kprobeOrFentry("security_sb_umount", fentry),
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kprobe_clone_mnt"}},
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "mount", fentry, EntryAndExit|SupportFentry, true)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "umount", fentry, Exit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "unshare", fentry, EntryAndExit|SupportFentry)},
			&manager.OneOf{Selectors: []manager.ProbesSelector{
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kprobe___attach_mnt"}},
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kprobe_attach_mnt"}},
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kprobe_mnt_set_mountpoint"}},
			}},

			// Rename probes
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kprobe_vfs_rename"}},
				kprobeOrFentry("mnt_want_write", fentry),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "rename", fentry, EntryAndExit|SupportFentry)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "renameat", fentry, EntryAndExit|SupportFentry)},
			&manager.BestEffort{Selectors: append(
				[]manager.ProbesSelector{
					kprobeOrFentry("do_renameat2", fentry),
					&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kretprobe_do_renameat2"}}},
				ExpandSyscallProbesSelector(SecurityAgentUID, "renameat2", fentry, EntryAndExit|SupportFentry)...)},

			// unlink rmdir probes
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("mnt_want_write", fentry),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "unlinkat", fentry, EntryAndExit|SupportFentry)},
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("do_unlinkat", fentry),
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kretprobe_do_unlinkat"}},
			}},

			// Rmdir probes
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kprobe_security_inode_rmdir"}},
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "rmdir", fentry, EntryAndExit|SupportFentry)},
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("do_rmdir", fentry),
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kretprobe_do_rmdir"}},
			}},

			// Unlink probes
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kprobe_vfs_unlink"}},
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "unlink", fentry, EntryAndExit|SupportFentry)},
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("do_linkat", fentry),
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kretprobe_do_linkat"}},
			}},

			// ioctl probes
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("do_vfs_ioctl", fentry),
			}},

			// Link
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kprobe_vfs_link"}},
				kprobeOrFentry("filename_create", fentry),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "link", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "linkat", fentry, EntryAndExit|SupportFentry|SupportFexit)},

			// selinux
			// This needs to be best effort, as sel_write_disable is in the process to be removed
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kprobe_sel_write_disable"}},
			}},
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kprobe_sel_write_enforce"}},
			}},
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kprobe_sel_write_bool"}},
			}},
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kprobe_sel_commit_bools_write"}},
			}},

			// pipes
			// This is needed to skip FIM events relatives to pipes (avoiding abnormal path events)
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("mntget", fentry),
			}}},

		// List of probes required to capture chmod events
		"chmod": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("mnt_want_write", fentry),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "chmod", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "fchmod", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "fchmodat", fentry, EntryAndExit|SupportFentry|SupportFexit)},
		},

		// List of probes required to capture chown events
		"chown": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("mnt_want_write", fentry),
			}},
			&manager.OneOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("mnt_want_write_file", fentry),
				kprobeOrFentry("mnt_want_write_file_path", fentry, withSkipIfFentry(true)),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "chown", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "chown16", fentry, EntryAndExit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "fchown", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "fchown16", fentry, EntryAndExit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "fchownat", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "lchown", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "lchown16", fentry, EntryAndExit)},
		},

		// List of probes required to capture mkdir events
		"mkdir": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("vfs_mkdir", fentry),
				kprobeOrFentry("filename_create", fentry),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "mkdir", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "mkdirat", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kprobe_do_mkdirat"}},
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kretprobe_do_mkdirat"}},
			}}},

		// List of probes required to capture removexattr events
		"removexattr": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kprobe_vfs_removexattr"}},
				kprobeOrFentry("mnt_want_write", fentry),
			}},
			&manager.OneOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("mnt_want_write_file", fentry),
				kprobeOrFentry("mnt_want_write_file_path", fentry, withSkipIfFentry(true)),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "removexattr", fentry, EntryAndExit|SupportFentry)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "fremovexattr", fentry, EntryAndExit|SupportFentry)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "lremovexattr", fentry, EntryAndExit|SupportFentry)},
		},

		// List of probes required to capture setxattr events
		"setxattr": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kprobe_vfs_setxattr"}},
				kprobeOrFentry("mnt_want_write", fentry),
			}},
			&manager.OneOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("mnt_want_write_file", fentry),
				kprobeOrFentry("mnt_want_write_file_path", fentry, withSkipIfFentry(true)),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setxattr", fentry, EntryAndExit|SupportFentry)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "fsetxattr", fentry, EntryAndExit|SupportFentry)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "lsetxattr", fentry, EntryAndExit|SupportFentry)},
		},

		// List of probes required to capture utimes events
		"utimes": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("mnt_want_write", fentry),
			}},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "utime", fentry, EntryAndExit, true)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "utime32", fentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "utimes", fentry, EntryAndExit, true)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "utimes", fentry, EntryAndExit|ExpandTime32)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "utimensat", fentry, EntryAndExit, true)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "utimensat", fentry, EntryAndExit|ExpandTime32)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "futimesat", fentry, EntryAndExit, true)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "futimesat", fentry, EntryAndExit|ExpandTime32)},
		},

		// List of probes required to capture bpf events
		"bpf": {
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("security_bpf_map", fentry),
				kprobeOrFentry("security_bpf_prog", fentry),
				kprobeOrFentry("check_helper_call", fentry),
			}},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "bpf", fentry, EntryAndExit|SupportFentry|SupportFexit)},
		},

		// List of probes required to capture ptrace events
		"ptrace": {
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "ptrace", fentry, EntryAndExit|SupportFentry)},
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("ptrace_check_attach", fentry),
			}},
		},

		// List of probes required to capture mmap events
		"mmap": {
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "mmap", fentry, Exit|SupportFexit)},
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "tracepoint_syscalls_sys_enter_mmap"}},
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kretprobe_fget"}},
			}}},

		// List of probes required to capture mprotect events
		"mprotect": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("security_file_mprotect", fentry),
			}},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "mprotect", fentry, EntryAndExit|SupportFentry)},
		},

		// List of probes required to capture kernel load_module events
		"load_module": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				&manager.OneOf{Selectors: []manager.ProbesSelector{
					&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kprobe_security_kernel_read_file"}},
					&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kprobe_security_kernel_module_from_file"}},
				}},
				&manager.OneOf{Selectors: []manager.ProbesSelector{
					kprobeOrFentry("do_init_module", fentry),
					kprobeOrFentry("module_put", fentry),
				}},
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kprobe_parse_args"}},
			}},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "init_module", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "finit_module", fentry, EntryAndExit|SupportFentry|SupportFexit)},
		},

		// List of probes required to capture kernel unload_module events
		"unload_module": {
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "delete_module", fentry, EntryAndExit|SupportFentry|SupportFexit)},
		},

		// List of probes required to capture signal events
		"signal": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kretprobeOrFexit("check_kill_permission", fentry),
				kprobeOrFentry("kill_pid_info", fentry),
			}},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "kill", fentry, Entry|SupportFentry)},
		},

		// List of probes required to capture splice events
		"splice": {
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "splice", fentry, EntryAndExit|SupportFentry)},
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("get_pipe_info", fentry),
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "kretprobe_get_pipe_info"}},
			}}},

		// List of probes required to capture bind events
		"bind": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("security_socket_bind", fentry),
			}},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "bind", fentry, EntryAndExit|SupportFentry|SupportFexit)},
		},

		// List of probes required to capture DNS events
		"dns": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				&manager.AllOf{Selectors: NetworkSelectors(fentry)},
				&manager.AllOf{Selectors: NetworkVethSelectors},
				kprobeOrFentry("security_socket_bind", fentry),
			}},
		},
	}

	// add probes depending on loaded modules
	loadedModules, err := utils.FetchLoadedModules()
	if err == nil {
		if _, ok := loadedModules["nf_nat"]; ok {
			selectorsPerEventTypeStore["dns"] = append(selectorsPerEventTypeStore["dns"], NetworkNFNatSelectors...)
		}
	}

	if ShouldUseModuleLoadTracepoint() {
		selectorsPerEventTypeStore["load_module"] = append(selectorsPerEventTypeStore["load_module"], &manager.BestEffort{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "module_load"}},
		}})
	}

	return selectorsPerEventTypeStore
}
