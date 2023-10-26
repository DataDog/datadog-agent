// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probes holds probes related files
package probes

import (
	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// NetworkNFNatSelectors is the list of probes that should be activated if the `nf_nat` module is loaded
func NetworkNFNatSelectors(fentry bool) []manager.ProbesSelector {
	return []manager.ProbesSelector{
		&manager.OneOf{Selectors: []manager.ProbesSelector{
			kprobeOrFentry("nf_nat_manip_pkt", fentry),
			kprobeOrFentry("nf_nat_packet", fentry),
		}},
	}
}

// NetworkVethSelectors is the list of probes that should be activated if the `veth` module is loaded
func NetworkVethSelectors(fentry bool) []manager.ProbesSelector {
	return []manager.ProbesSelector{
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			kprobeOrFentry("rtnl_create_link", fentry),
		}},
	}
}

// NetworkSelectors is the list of probes that should be activated when the network is enabled
func NetworkSelectors(fentry bool) []manager.ProbesSelector {
	return []manager.ProbesSelector{
		// flow classification probes
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			kprobeOrFentry("security_socket_bind", fentry),
			kprobeOrFentry("security_sk_classify_flow", fentry),
			kprobeOrFentry("path_get", fentry),
			kprobeOrFentry("proc_fd_link", fentry),
		}},

		// network device probes
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			kprobeOrFentry("register_netdevice", fentry),
			kretprobeOrFexit("register_netdevice", fentry),
			&manager.OneOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("dev_change_net_namespace", fentry),
				kprobeOrFentry("__dev_change_net_namespace", fentry),
			}},
		}},
		&manager.BestEffort{Selectors: []manager.ProbesSelector{
			kprobeOrFentry("dev_get_valid_name", fentry),
			kprobeOrFentry("dev_new_index", fentry),
			kretprobeOrFexit("dev_new_index", fentry),
			kprobeOrFentry("__dev_get_by_index", fentry),
			kprobeOrFentry("__dev_get_by_name", fentry),
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
					kprobeOrFentry("prepare_binprm", fentry),
					kprobeOrFentry("bprm_execve", fentry),
					kprobeOrFentry("security_bprm_check", fentry),
				}},
				kprobeOrFentry("setup_new_exec_interp", fentry),
				kprobeOrFentry("setup_new_exec_args_envs", fentry, withUID(SecurityAgentUID+"_a")),
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
				kprobeOrFentry("_do_fork", fentry),
				kprobeOrFentry("do_fork", fentry),
				kprobeOrFentry("kernel_clone", fentry),
				kprobeOrFentry("kernel_thread", fentry),
				kprobeOrFentry("user_mode_thread", fentry),
			}},
			&manager.OneOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("cgroup_tasks_write", fentry),
				kprobeOrFentry("cgroup1_tasks_write", fentry),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "execve", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "execveat", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setuid", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setuid16", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setgid", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setgid16", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setfsuid", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setfsuid16", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setfsgid", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setfsgid16", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setreuid", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setreuid16", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setregid", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setregid16", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setresuid", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setresuid16", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setresgid", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setresgid16", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "capset", fentry, EntryAndExit|SupportFentry|SupportFexit)},

			// File Attributes
			kprobeOrFentry("security_inode_setattr", fentry),

			// Open probes
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("vfs_truncate", fentry),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "open", fentry, EntryAndExit|SupportFentry|SupportFexit, true)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "creat", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "truncate", fentry, EntryAndExit|SupportFentry|SupportFexit, true)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "openat", fentry, EntryAndExit|SupportFentry|SupportFexit, true)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "openat2", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "open_by_handle_at", fentry, EntryAndExit|SupportFentry|SupportFexit, true)},
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("io_openat", fentry),
				kprobeOrFentry("io_openat2", fentry),
				kretprobeOrFexit("io_openat2", fentry),
			}},
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("filp_close", fentry),
			}},

			// iouring
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "io_uring_create"}},
				&manager.OneOf{Selectors: []manager.ProbesSelector{
					kprobeOrFentry("io_allocate_scq_urings", fentry),
					kprobeOrFentry("io_sq_offload_start", fentry),
					kretprobeOrFexit("io_ring_ctx_alloc", fentry),
				}},
			}},

			// Mount probes
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("attach_recursive_mnt", fentry),
				kprobeOrFentry("propagate_mnt", fentry),
				kprobeOrFentry("security_sb_umount", fentry),
				kprobeOrFentry("clone_mnt", fentry),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "mount", fentry, EntryAndExit|SupportFentry|SupportFexit, true)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "umount", fentry, Exit|SupportFexit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "unshare", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.OneOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("attach_mnt", fentry),
				kprobeOrFentry("__attach_mnt", fentry),
				kprobeOrFentry("mnt_set_mountpoint", fentry),
			}},

			// Rename probes
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("vfs_rename", fentry),
				kprobeOrFentry("mnt_want_write", fentry),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "rename", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "renameat", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.BestEffort{Selectors: append(
				[]manager.ProbesSelector{
					kprobeOrFentry("do_renameat2", fentry),
					kretprobeOrFexit("do_renameat2", fentry),
				},
				ExpandSyscallProbesSelector(SecurityAgentUID, "renameat2", fentry, EntryAndExit|SupportFentry|SupportFexit)...)},

			// unlink rmdir probes
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("mnt_want_write", fentry),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "unlinkat", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("do_unlinkat", fentry),
				kretprobeOrFexit("do_unlinkat", fentry),
			}},

			// Rmdir probes
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("security_inode_rmdir", fentry),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "rmdir", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("do_rmdir", fentry),
				kretprobeOrFexit("do_rmdir", fentry),
			}},

			// Unlink probes
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("vfs_unlink", fentry),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "unlink", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("do_linkat", fentry),
				kretprobeOrFexit("do_linkat", fentry),
			}},

			// ioctl probes
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("do_vfs_ioctl", fentry),
			}},

			// Link
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("vfs_link", fentry),
				&manager.OneOf{Selectors: []manager.ProbesSelector{
					kprobeOrFentry("filename_create", fentry),
					kprobeOrFentry("security_path_link", fentry),
					kprobeOrFentry("security_path_mkdir", fentry),
				}},
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "link", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "linkat", fentry, EntryAndExit|SupportFentry|SupportFexit)},

			// selinux
			// This needs to be best effort, as sel_write_disable is in the process of being removed
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("sel_write_disable", fentry),
				kprobeOrFentry("sel_write_enforce", fentry),
				kprobeOrFentry("sel_write_bool", fentry),
				kprobeOrFentry("sel_commit_bools_write", fentry),
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
				kprobeOrFentry("mnt_want_write_file_path", fentry),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "chown", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "chown16", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "fchown", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "fchown16", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "fchownat", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "lchown", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "lchown16", fentry, EntryAndExit|SupportFentry|SupportFexit)},
		},

		// List of probes required to capture mkdir events
		"mkdir": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("vfs_mkdir", fentry),
				&manager.OneOf{Selectors: []manager.ProbesSelector{
					kprobeOrFentry("filename_create", fentry),
					kprobeOrFentry("security_path_link", fentry),
					kprobeOrFentry("security_path_mkdir", fentry),
				}},
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "mkdir", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "mkdirat", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("do_mkdirat", fentry),
				kretprobeOrFexit("do_mkdirat", fentry),
			}}},

		// List of probes required to capture removexattr events
		"removexattr": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("vfs_removexattr", fentry),
				kprobeOrFentry("mnt_want_write", fentry),
			}},
			&manager.OneOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("mnt_want_write_file", fentry),
				kprobeOrFentry("mnt_want_write_file_path", fentry),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "removexattr", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "fremovexattr", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "lremovexattr", fentry, EntryAndExit|SupportFentry|SupportFexit)},
		},

		// List of probes required to capture setxattr events
		"setxattr": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("vfs_setxattr", fentry),
				kprobeOrFentry("mnt_want_write", fentry),
			}},
			&manager.OneOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("mnt_want_write_file", fentry),
				kprobeOrFentry("mnt_want_write_file_path", fentry),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setxattr", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "fsetxattr", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "lsetxattr", fentry, EntryAndExit|SupportFentry|SupportFexit)},
		},

		// List of probes required to capture utimes events
		"utimes": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("mnt_want_write", fentry),
			}},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "utime", fentry, EntryAndExit|SupportFentry|SupportFexit, true)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "utime32", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "utimes", fentry, EntryAndExit|SupportFentry|SupportFexit, true)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "utimes", fentry, EntryAndExit|SupportFentry|SupportFexit|ExpandTime32)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "utimensat", fentry, EntryAndExit|SupportFentry|SupportFexit, true)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "utimensat", fentry, EntryAndExit|SupportFentry|SupportFexit|ExpandTime32)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "futimesat", fentry, EntryAndExit|SupportFentry|SupportFexit, true)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "futimesat", fentry, EntryAndExit|SupportFentry|SupportFexit|ExpandTime32)},
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
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "ptrace", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("ptrace_check_attach", fentry),
			}},
		},

		// List of probes required to capture mmap events
		"mmap": {
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "mmap", fentry, Exit|SupportFexit)},
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "tracepoint_syscalls_sys_enter_mmap"}},
				kretprobeOrFexit("fget", fentry),
			}}},

		// List of probes required to capture mprotect events
		"mprotect": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("security_file_mprotect", fentry),
			}},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "mprotect", fentry, EntryAndExit|SupportFentry|SupportFexit)},
		},

		// List of probes required to capture kernel load_module events
		"load_module": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				&manager.OneOf{Selectors: []manager.ProbesSelector{
					kprobeOrFentry("security_kernel_read_file", fentry),
					kprobeOrFentry("security_kernel_module_from_file", fentry),
				}},
				&manager.OneOf{Selectors: []manager.ProbesSelector{
					kprobeOrFentry("mod_sysfs_setup", fentry),
					kprobeOrFentry("module_param_sysfs_setup", fentry),
				}},
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
				kprobeOrFentry("check_kill_permission", fentry),
			}},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "kill", fentry, Entry|SupportFentry)},
		},

		// List of probes required to capture splice events
		"splice": {
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "splice", fentry, EntryAndExit|SupportFentry|SupportFexit)},
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("get_pipe_info", fentry),
				kretprobeOrFexit("get_pipe_info", fentry),
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
				&manager.AllOf{Selectors: NetworkVethSelectors(fentry)},
				kprobeOrFentry("security_socket_bind", fentry),
			}},
		},
	}

	// add probes depending on loaded modules
	loadedModules, err := utils.FetchLoadedModules()
	if err == nil {
		if _, ok := loadedModules["nf_nat"]; ok {
			selectorsPerEventTypeStore["dns"] = append(selectorsPerEventTypeStore["dns"], NetworkNFNatSelectors(fentry)...)
		}
	}

	if ShouldUseModuleLoadTracepoint() {
		selectorsPerEventTypeStore["load_module"] = append(selectorsPerEventTypeStore["load_module"], &manager.BestEffort{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "module_load"}},
		}})
	}

	return selectorsPerEventTypeStore
}
