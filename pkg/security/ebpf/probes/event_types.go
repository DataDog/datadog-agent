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
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// NetworkNFNatSelectors is the list of probes that should be activated if the `nf_nat` module is loaded
func NetworkNFNatSelectors() []manager.ProbesSelector {
	return []manager.ProbesSelector{
		&manager.OneOf{Selectors: []manager.ProbesSelector{
			kprobeOrFentry("nf_nat_manip_pkt"),
			kprobeOrFentry("nf_nat_packet"),
		}},
	}
}

// NetworkVethSelectors is the list of probes that should be activated if the `veth` module is loaded
func NetworkVethSelectors() []manager.ProbesSelector {
	return []manager.ProbesSelector{
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			kprobeOrFentry("rtnl_create_link"),
		}},
	}
}

// NetworkSelectors is the list of probes that should be activated when the network is enabled
func NetworkSelectors() []manager.ProbesSelector {
	return []manager.ProbesSelector{
		// flow classification probes
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			kprobeOrFentry("security_socket_bind"),
			kprobeOrFentry("security_socket_connect"),
			kprobeOrFentry("security_sk_classify_flow"),
			kprobeOrFentry("path_get"),
			kprobeOrFentry("proc_fd_link"),
		}},

		// network device probes
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			kprobeOrFentry("register_netdevice"),
			kretprobeOrFexit("register_netdevice"),
			&manager.OneOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("dev_change_net_namespace"),
				kprobeOrFentry("__dev_change_net_namespace"),
			}},
		}},
		&manager.BestEffort{Selectors: []manager.ProbesSelector{
			kprobeOrFentry("dev_get_valid_name"),
			kprobeOrFentry("dev_new_index"),
			kretprobeOrFexit("dev_new_index"),
			kprobeOrFentry("__dev_get_by_index"),
		}},
	}
}

// SyscallMonitorSelectors is the list of probes that should be activated for the syscall monitor feature
var SyscallMonitorSelectors = []manager.ProbesSelector{
	&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "sys_enter"}},
}

// SnapshotSelectors selectors required during the snapshot
func SnapshotSelectors() []manager.ProbesSelector {
	procsOpen := kprobeOrFentry("cgroup_procs_open")
	tasksOpen := kprobeOrFentry("cgroup_tasks_open")
	return []manager.ProbesSelector{
		// required to stat /proc/.../exe
		kprobeOrFentry("security_inode_getattr"),
		&manager.BestEffort{Selectors: []manager.ProbesSelector{procsOpen, tasksOpen}},
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
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "sched_process_fork"}},
				kprobeOrFentry("do_exit"),
				&manager.BestEffort{Selectors: []manager.ProbesSelector{
					kprobeOrFentry("prepare_binprm"),
					kprobeOrFentry("bprm_execve"),
					kprobeOrFentry("security_bprm_check"),
				}},
				kprobeOrFentry("setup_new_exec_interp"),
				kprobeOrFentry("setup_new_exec_args_envs", withUID(SecurityAgentUID+"_a")),
				kprobeOrFentry("setup_arg_pages"),
				kprobeOrFentry("mprotect_fixup"),
				kprobeOrFentry("exit_itimers"),
				kprobeOrFentry("vfs_open"),
				kprobeOrFentry("do_dentry_open"),
				kprobeOrFentry("commit_creds"),
				kprobeOrFentry("switch_task_namespaces"),
				kprobeOrFentry("do_coredump"),
				kprobeOrFentry("audit_set_loginuid"),
				kretprobeOrFexit("audit_set_loginuid"),
			}},
			&manager.OneOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("cgroup_procs_write"),
				kprobeOrFentry("cgroup1_procs_write"),
			}},
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("cgroup_procs_open"),
			}},
			&manager.OneOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("_do_fork"),
				kprobeOrFentry("do_fork"),
				kprobeOrFentry("kernel_clone"),
				kprobeOrFentry("kernel_thread"),
				kprobeOrFentry("user_mode_thread"),
			}},
			&manager.OneOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("cgroup_tasks_write"),
				kprobeOrFentry("cgroup1_tasks_write"),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "execve", fentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "execveat", fentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setuid", fentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setuid16", fentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setgid", fentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setgid16", fentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setfsuid", fentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setfsuid16", fentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setfsgid", fentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setfsgid16", fentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setreuid", fentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setreuid16", fentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setregid", fentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setregid16", fentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setresuid", fentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setresuid16", fentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setresgid", fentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setresgid16", fentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "capset", fentry, EntryAndExit)},

			// File Attributes
			kprobeOrFentry("security_inode_setattr"),

			// Open probes
			&manager.OneOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("security_path_truncate"),
				kprobeOrFentry("security_file_truncate"),
				kprobeOrFentry("vfs_truncate"),
				kprobeOrFentry("do_truncate"),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "open", fentry, EntryAndExit, true)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "creat", fentry, EntryAndExit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "truncate", fentry, EntryAndExit, true)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "ftruncate", fentry, EntryAndExit, true)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "openat", fentry, EntryAndExit, true)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "openat2", fentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "open_by_handle_at", fentry, EntryAndExit, true)},
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("io_openat"),
				kprobeOrFentry("io_openat2"),
				kretprobeOrFexit("io_openat2"),
			}},
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("filp_close"),
			}},

			// iouring
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "io_uring_create"}},
				&manager.OneOf{Selectors: []manager.ProbesSelector{
					kprobeOrFentry("io_allocate_scq_urings"),
					kprobeOrFentry("io_sq_offload_start"),
					kretprobeOrFexit("io_ring_ctx_alloc"),
				}},
			}},

			// Mount probes
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("attach_recursive_mnt"),
				kprobeOrFentry("propagate_mnt"),
				kprobeOrFentry("security_sb_umount"),
				kprobeOrFentry("clone_mnt"),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "mount", fentry, EntryAndExit, true)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "umount", fentry, Exit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "unshare", fentry, EntryAndExit)},
			&manager.OneOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("attach_mnt"),
				kprobeOrFentry("__attach_mnt"),
				kprobeOrFentry("mnt_set_mountpoint"),
			}},

			// Rename probes
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("vfs_rename"),
				kprobeOrFentry("mnt_want_write"),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "rename", fentry, EntryAndExit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "renameat", fentry, EntryAndExit)},
			&manager.BestEffort{Selectors: append(
				[]manager.ProbesSelector{
					kprobeOrFentry("do_renameat2"),
					kretprobeOrFexit("do_renameat2"),
				},
				ExpandSyscallProbesSelector(SecurityAgentUID, "renameat2", fentry, EntryAndExit)...)},

			// unlink rmdir probes
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("mnt_want_write"),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "unlinkat", fentry, EntryAndExit)},
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("do_unlinkat"),
				kretprobeOrFexit("do_unlinkat"),
			}},

			// Rmdir probes
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("security_inode_rmdir"),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "rmdir", fentry, EntryAndExit)},
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("do_rmdir"),
				kretprobeOrFexit("do_rmdir"),
			}},

			// Unlink probes
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("vfs_unlink"),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "unlink", fentry, EntryAndExit)},
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("do_linkat"),
				kretprobeOrFexit("do_linkat"),
			}},

			// ioctl probes
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("do_vfs_ioctl"),
			}},

			// Link
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("vfs_link"),
				&manager.OneOf{Selectors: []manager.ProbesSelector{
					kprobeOrFentry("filename_create"),
					kprobeOrFentry("security_path_link"),
					kprobeOrFentry("security_path_mkdir"),
				}},
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "link", fentry, EntryAndExit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "linkat", fentry, EntryAndExit)},

			// selinux
			// This needs to be best effort, as sel_write_disable is in the process of being removed
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("sel_write_disable"),
				kprobeOrFentry("sel_write_enforce"),
				kprobeOrFentry("sel_write_bool"),
				kprobeOrFentry("sel_commit_bools_write"),
			}}},

		// List of probes required to capture chmod events
		"chmod": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("mnt_want_write"),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "chmod", fentry, EntryAndExit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "fchmod", fentry, EntryAndExit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "fchmodat", fentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "fchmodat2", fentry, EntryAndExit)},
		},

		// List of probes required to capture chown events
		"chown": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("mnt_want_write"),
			}},
			&manager.OneOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("mnt_want_write_file"),
				kprobeOrFentry("mnt_want_write_file_path"),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "chown", fentry, EntryAndExit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "chown16", fentry, EntryAndExit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "fchown", fentry, EntryAndExit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "fchown16", fentry, EntryAndExit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "fchownat", fentry, EntryAndExit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "lchown", fentry, EntryAndExit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "lchown16", fentry, EntryAndExit)},
		},

		// List of probes required to capture mkdir events
		"mkdir": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("vfs_mkdir"),
				&manager.OneOf{Selectors: []manager.ProbesSelector{
					kprobeOrFentry("filename_create"),
					kprobeOrFentry("security_path_link"),
					kprobeOrFentry("security_path_mkdir"),
				}},
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "mkdir", fentry, EntryAndExit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "mkdirat", fentry, EntryAndExit)},
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("do_mkdirat"),
				kretprobeOrFexit("do_mkdirat"),
			}}},

		// List of probes required to capture removexattr events
		"removexattr": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("vfs_removexattr"),
				kprobeOrFentry("mnt_want_write"),
			}},
			&manager.OneOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("mnt_want_write_file"),
				kprobeOrFentry("mnt_want_write_file_path"),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "removexattr", fentry, EntryAndExit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "fremovexattr", fentry, EntryAndExit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "lremovexattr", fentry, EntryAndExit)},
		},

		// List of probes required to capture setxattr events
		"setxattr": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("vfs_setxattr"),
				kprobeOrFentry("mnt_want_write"),
			}},
			&manager.OneOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("mnt_want_write_file"),
				kprobeOrFentry("mnt_want_write_file_path"),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setxattr", fentry, EntryAndExit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "fsetxattr", fentry, EntryAndExit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "lsetxattr", fentry, EntryAndExit)},
		},

		// List of probes required to capture utimes events
		"utimes": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("mnt_want_write"),
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
				kprobeOrFentry("security_bpf_map"),
				kprobeOrFentry("security_bpf_prog"),
				kprobeOrFentry("check_helper_call"),
			}},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "bpf", fentry, EntryAndExit)},
		},

		// List of probes required to capture ptrace events
		"ptrace": {
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "ptrace", fentry, EntryAndExit)},
			&manager.OneOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("ptrace_check_attach"),
				kprobeOrFentry("arch_ptrace"),
			}},
		},

		// List of probes required to capture mmap events
		"mmap": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("vm_mmap_pgoff"),
				kretprobeOrFexit("vm_mmap_pgoff"),
				kprobeOrFentry("security_mmap_file"),
			}},
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("get_unmapped_area"),
			}},
		},

		// List of probes required to capture mprotect events
		"mprotect": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("security_file_mprotect"),
			}},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "mprotect", fentry, EntryAndExit)},
		},

		// List of probes required to capture kernel load_module events
		"load_module": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				&manager.OneOf{Selectors: []manager.ProbesSelector{
					kprobeOrFentry("security_kernel_read_file"),
					kprobeOrFentry("security_kernel_module_from_file"),
				}},
				&manager.OneOf{Selectors: []manager.ProbesSelector{
					kprobeOrFentry("mod_sysfs_setup"),
					kprobeOrFentry("module_param_sysfs_setup"),
				}},
			}},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "init_module", fentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "finit_module", fentry, EntryAndExit)},
		},

		// List of probes required to capture kernel unload_module events
		"unload_module": {
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "delete_module", fentry, EntryAndExit)},
		},

		// List of probes required to capture signal events
		"signal": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kretprobeOrFexit("check_kill_permission"),
				kprobeOrFentry("check_kill_permission"),
			}},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "kill", fentry, Entry)},
		},

		// List of probes required to capture splice events
		"splice": {
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "splice", fentry, EntryAndExit)},
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("get_pipe_info"),
				kretprobeOrFexit("get_pipe_info"),
			}}},

		// List of probes required to capture bind events
		"bind": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("security_socket_bind"),
			}},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "bind", fentry, EntryAndExit)},
		},
		// List of probes required to capture connect events
		"connect": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("security_socket_connect"),
			}},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "connect", fentry, EntryAndExit)},
		},

		// List of probes required to capture chdir events
		"chdir": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				kprobeOrFentry("set_fs_pwd"),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "chdir", fentry, EntryAndExit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "fchdir", fentry, EntryAndExit)},
		},
	}

	// Add probes required to track network interfaces and map network flows to processes
	// networkEventTypes: dns, imds, packet
	networkEventTypes := model.GetEventTypePerCategory(model.NetworkCategory)[model.NetworkCategory]
	for _, networkEventType := range networkEventTypes {
		selectorsPerEventTypeStore[networkEventType] = []manager.ProbesSelector{
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				&manager.AllOf{Selectors: NetworkSelectors()},
				&manager.AllOf{Selectors: NetworkVethSelectors()},
			}},
		}
	}

	// add probes depending on loaded modules
	loadedModules, err := utils.FetchLoadedModules()
	if err == nil {
		if _, ok := loadedModules["nf_nat"]; ok {
			for _, networkEventType := range networkEventTypes {
				selectorsPerEventTypeStore[networkEventType] = append(selectorsPerEventTypeStore[networkEventType], NetworkNFNatSelectors()...)
			}
		}
	}

	if ShouldUseModuleLoadTracepoint() {
		selectorsPerEventTypeStore["load_module"] = append(selectorsPerEventTypeStore["load_module"], &manager.BestEffort{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "module_load"}},
		}})
	}

	return selectorsPerEventTypeStore
}
