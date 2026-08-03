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
			hookFunc("hook_nf_nat_manip_pkt"),
			hookFunc("hook_nf_nat_packet"),
			hookFunc("hook_nf_ct_delete"),
		}},
	}
}

// NetworkVethSelectors is the list of probes that should be activated if the `veth` module is loaded
func NetworkVethSelectors() []manager.ProbesSelector {
	return []manager.ProbesSelector{
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			hookFunc("hook_rtnl_create_link"),
		}},
	}
}

// NetworkSelectors is the list of probes that should be activated when the network is enabled
func NetworkSelectors(hasCgroupSocket bool) []manager.ProbesSelector {
	ps := []manager.ProbesSelector{
		// flow classification probes
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			hookFunc("hook_security_socket_bind"),
			hookFunc("hook_security_socket_connect"),
			hookFunc("hook_security_sk_classify_flow"),
			hookFunc("hook_inet_release"),
			hookFunc("hook_inet_csk_destroy_sock"),
			hookFunc("hook_sk_destruct"),
			hookFunc("hook_inet_put_port"),
			hookFunc("hook_inet_shutdown"),
			hookFunc("hook_inet_bind"),
			hookFunc("rethook_inet_bind"),
			hookFunc("hook_sk_common_release"),
			hookFunc("hook_path_get"),
			hookFunc("hook_proc_fd_link"),
		}},

		&manager.BestEffort{Selectors: []manager.ProbesSelector{
			hookFunc("hook_inet6_bind"),
			hookFunc("rethook_inet6_bind"),
		}},

		// network device probes
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			hookFunc("hook_register_netdevice"),
			hookFunc("rethook_register_netdevice"),
			&manager.OneOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_dev_change_net_namespace"),
				hookFunc("hook___dev_change_net_namespace"),
			}},
		}},
		&manager.BestEffort{Selectors: []manager.ProbesSelector{
			hookFunc("hook_dev_get_valid_name"),
			// dev_new_index was replaced by dev_index_reserve in kernel 6.6; both are best-effort
			// alternatives used to resolve the ifindex of a newly registered device
			hookFunc("hook_dev_new_index"),
			hookFunc("rethook_dev_new_index"),
			hookFunc("hook_dev_index_reserve"),
			hookFunc("rethook_dev_index_reserve"),
			hookFunc("hook___dev_get_by_index"),
		}},
	}

	if hasCgroupSocket {
		ps = append(ps, &manager.BestEffort{Selectors: []manager.ProbesSelector{
			hookFunc("hook_sock_create"),
			hookFunc("hook_sock_release"),
		}})
	}

	return ps
}

// SyscallMonitorSelectors is the list of probes that should be activated for the syscall monitor feature
func SyscallMonitorSelectors() []manager.ProbesSelector {
	return []manager.ProbesSelector{
		&manager.ProbeSelector{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "sys_enter",
			},
		},
	}
}

// SnapshotSelectors selectors required during the snapshot
func SnapshotSelectors(fentry bool) []manager.ProbesSelector {
	procsOpen := hookFunc("hook_cgroup_procs_open")
	tasksOpen := hookFunc("hook_cgroup_tasks_open")
	return []manager.ProbesSelector{
		&manager.BestEffort{Selectors: []manager.ProbesSelector{procsOpen, tasksOpen}},

		// required to stat /proc/.../exe
		hookFunc("hook_security_inode_getattr"),
		&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "newfstatat", fentry, EntryAndExit)},
	}
}

// GetCapabilitiesMonitoringSelectors returns the list of probes that should be activated for capabilities monitoring
func GetCapabilitiesMonitoringSelectors() []manager.ProbesSelector {
	return []manager.ProbesSelector{
		&manager.AllOf{
			Selectors: []manager.ProbesSelector{
				hookFunc("hook_security_capable"),
				hookFunc("rethook_security_capable"),
				&manager.ProbeSelector{
					ProbeIdentificationPair: manager.ProbeIdentificationPair{
						UID:          SecurityAgentUID,
						EBPFFuncName: "capabilities_usage_ticker",
					},
				},
				// override_creds/revert_creds are inlined since kernel 6.13, so they are best-effort:
				// where attachable (< 6.13, including kernels without BTF) they drive the override
				// depth counter; on 6.13+ the cred/real_cred comparison is used instead
				&manager.BestEffort{Selectors: []manager.ProbesSelector{
					hookFunc("hook_override_creds"),
					hookFunc("hook_revert_creds"),
				}},
			},
		},
	}
}

// GetNetworkSelectors returns the probes that track network interfaces and sockets.
// These probes must be loaded independently of the current ruleset or network filter actions as
// these are used to track resources that are needed if we later dynamically load network rules
// or network filter actions.
func GetNetworkSelectors(hasCgroupSocket bool) []manager.ProbesSelector {
	selectors := []manager.ProbesSelector{
		&manager.AllOf{Selectors: []manager.ProbesSelector{
			&manager.AllOf{Selectors: NetworkSelectors(hasCgroupSocket)},
			&manager.AllOf{Selectors: NetworkVethSelectors()},
		}},
	}

	// add probes depending on loaded modules
	if loadedModules, err := utils.FetchLoadedModules(); err == nil {
		if _, ok := loadedModules["nf_nat"]; ok {
			selectors = append(selectors, NetworkNFNatSelectors()...)
		}
	}

	return selectors
}

// GetSelectorsPerEventType returns the list of probes that should be activated for each event
func GetSelectorsPerEventType(hasFentry, haveIOURing bool) map[eval.EventType][]manager.ProbesSelector {
	linkIOUringProbes := []manager.ProbesSelector{}
	if haveIOURing {
		linkIOUringProbes = []manager.ProbesSelector{
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_do_linkat"),
				hookFunc("rethook_do_linkat"),
			}},
			// Since 7.0, do_linkat was removed from the kernel so we need to hook the filename_linkat function instead
			// It is also used by the io_uring code path
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_filename_linkat"),
				hookFunc("rethook_filename_linkat"),
			}},
		}
	}

	unlinkIOUringProbes := []manager.ProbesSelector{}
	if haveIOURing {
		unlinkIOUringProbes = []manager.ProbesSelector{
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_do_unlinkat"),
				hookFunc("rethook_do_unlinkat"),
			}},
			// Since 7.0, do_unlinkat was removed from the kernel so we need to hook the filename_unlinkat function instead
			// It is also used by the io_uring code path
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_filename_unlinkat"),
				hookFunc("rethook_filename_unlinkat"),
			}},
		}
	}

	rmdirIOUringProbes := []manager.ProbesSelector{}
	if haveIOURing {
		rmdirIOUringProbes = []manager.ProbesSelector{
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_do_rmdir"),
				hookFunc("rethook_do_rmdir"),
			}},
			// Since 7.0, do_rmdir was removed from the kernel so we need to hook the filename_rmdir function instead
			// It is also used by the io_uring code path
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_filename_rmdir"),
				hookFunc("rethook_filename_rmdir"),
			}},
		}
	}

	mkdirIOUringProbes := []manager.ProbesSelector{}
	if haveIOURing {
		mkdirIOUringProbes = []manager.ProbesSelector{
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_do_mkdirat"),
				hookFunc("rethook_do_mkdirat"),
			}},
			// Since 7.0, do_mkdirat was removed from the kernel so we need to hook the filename_mkdirat function instead
			// It is also used by the io_uring code path
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_filename_mkdirat"),
				hookFunc("rethook_filename_mkdirat"),
			}},
		}
	}

	renameIOUringProbes := []manager.ProbesSelector{}
	if haveIOURing {
		renameIOUringProbes = []manager.ProbesSelector{
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_do_renameat2"),
				hookFunc("rethook_do_renameat2"),
			}},
			// Since 7.0, do_renameat2 was removed from the kernel so we need to hook the filename_renameat2 function instead
			// It is also used by the io_uring code path
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_filename_renameat2"),
				hookFunc("rethook_filename_renameat2"),
			}},
		}
	}

	selectorsPerEventTypeStore := map[eval.EventType][]manager.ProbesSelector{
		// The following probes will always be activated, regardless of the loaded rules
		"*": {
			// Exec probes
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				&manager.OneOf{
					Selectors: []manager.ProbesSelector{
						&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "sched_process_fork"}},
						hookFunc("rethook_get_task_pid"),
					},
				},
				hookFunc("hook_do_exit"),
				&manager.BestEffort{Selectors: []manager.ProbesSelector{
					hookFunc("hook_prepare_binprm"),
					hookFunc("hook_bprm_execve"),
					hookFunc("hook_security_bprm_check"),
				}},
				hookFunc("hook_setup_new_exec_interp"),
				// kernels < 4.17 will rely on the tracefs events interface to attach kprobes, which requires event names to be unique
				// because the setup_new_exec_interp and setup_new_exec_args_envs probes are attached to the same function, we rely on using a secondary uid for that purpose
				hookFunc("hook_setup_new_exec_args_envs", withUID(SecurityAgentUID+"_a")),
				hookFunc("hook_setup_arg_pages"),
				hookFunc("hook_mprotect_fixup"),
				hookFunc("hook_exit_itimers"),
				hookFunc("hook_do_dentry_open"),
				hookFunc("hook_vfs_open"),
				hookFunc("hook_commit_creds"),
				hookFunc("hook_switch_task_namespaces"),
				&manager.OneOf{Selectors: []manager.ProbesSelector{
					hookFunc("hook_do_coredump"),
					hookFunc("hook_vfs_coredump"),
				}},
				hookFunc("hook_audit_set_loginuid"),
				hookFunc("rethook_audit_set_loginuid"),
				hookFunc("hook_security_inode_follow_link"),
			}},
			&manager.OneOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_cgroup_procs_write"),
				hookFunc("hook_cgroup1_procs_write"),
			}},
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				hookFunc("hook_cgroup_procs_open"),
			}},
			&manager.OneOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook__do_fork"),
				hookFunc("hook_do_fork"),
				hookFunc("hook_kernel_clone"),
				hookFunc("hook_kernel_thread"),
				hookFunc("hook_user_mode_thread"),
			}},
			&manager.OneOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_cgroup_tasks_write"),
				hookFunc("hook_cgroup1_tasks_write"),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "execve", hasFentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "execveat", hasFentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setuid", hasFentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setuid16", hasFentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setgid", hasFentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setgid16", hasFentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setfsuid", hasFentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setfsuid16", hasFentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setfsgid", hasFentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setfsgid16", hasFentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setreuid", hasFentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setreuid16", hasFentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setregid", hasFentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setregid16", hasFentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setresuid", hasFentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setresuid16", hasFentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setresgid", hasFentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setresgid16", hasFentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "capset", hasFentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setsid", hasFentry, EntryAndExit)},

			// File Attributes
			hookFunc("hook_security_inode_setattr"),

			// Open probes
			&manager.OneOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_security_path_truncate"),
				hookFunc("hook_security_file_truncate"),
				hookFunc("hook_vfs_truncate"),
				hookFunc("hook_do_truncate"),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "open", hasFentry, EntryAndExit, true)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "creat", hasFentry, EntryAndExit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "truncate", hasFentry, EntryAndExit, true)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "ftruncate", hasFentry, EntryAndExit, true)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "openat", hasFentry, EntryAndExit, true)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "openat2", hasFentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "open_by_handle_at", hasFentry, EntryAndExit, true)},
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				hookFunc("hook_io_openat"),
				hookFunc("hook_io_openat2"),
				hookFunc("rethook_io_openat2"),
				hookFunc("hook_io_ftruncate"),
				hookFunc("rethook_io_ftruncate"),
			}},
			&manager.OneOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_terminate_walk"),
			}},

			// iouring
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "io_uring_create"}},
				&manager.OneOf{Selectors: []manager.ProbesSelector{
					hookFunc("hook_io_allocate_scq_urings"),
					hookFunc("hook_io_sq_offload_start"),
					hookFunc("rethook_io_ring_ctx_alloc"),
				}},
			}},

			// Mount probes
			// The following functions may be inlined, partially inlined, or rewritten as ISRA clones.
			// A OneOf selector is insufficient here, as some symbols may still be present even when the
			// corresponding code has effectively been inlined, making the hook point ineffective.
			// Therefore, we use a best-effort selector to ensure that mount operations
			// are captured regardless of which hook point is used.
			// Event deduplication is handled in the C code to prevent the same mount operation from being
			// processed multiple times.
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				hookFunc("hook_attach_recursive_mnt"),
				hookFunc("hook_propagate_mnt"),
				hookFunc("hook_attach_mnt"),
				hookFunc("hook___attach_mnt"),
				hookFunc("hook_make_visible"),
				hookFunc("hook_mnt_set_mountpoint"),
			}},
			// The previous considerations do not apply to this mount hook point.
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_security_sb_umount"),
				hookFunc("hook_clone_mnt"),
				hookFunc("hook_mnt_change_mountpoint"),
				hookFunc("hook_cleanup_mnt"),
				hookFunc("rethook_clone_mnt"),
			}},
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				hookFunc("rethook_alloc_vfsmnt"),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "mount", hasFentry, EntryAndExit, true)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "fsmount", hasFentry, EntryAndExit, false)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "open_tree", hasFentry, EntryAndExit, false)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "move_mount", hasFentry, EntryAndExit, false)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "umount", hasFentry, Exit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "unshare", hasFentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "pivot_root", hasFentry, EntryAndExit)},

			// Rename probes
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_vfs_rename"),
				hookFunc("hook_mnt_want_write"),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "rename", hasFentry, EntryAndExit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "renameat", hasFentry, EntryAndExit)},
			&manager.BestEffort{Selectors: renameIOUringProbes},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "renameat2", hasFentry, EntryAndExit)},

			// unlink rmdir probes
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_mnt_want_write"),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "unlinkat", hasFentry, EntryAndExit)},
			&manager.OneOf{
				Selectors: []manager.ProbesSelector{
					&manager.AllOf{Selectors: []manager.ProbesSelector{
						hookFunc("hook_do_unlinkat"),
						hookFunc("rethook_do_unlinkat"),
					}},
					// Since 7.0, do_unlinkat was removed from the kernel so we need to hook the filename_unlinkat function instead
					// It is also used by the io_uring code path
					&manager.AllOf{Selectors: []manager.ProbesSelector{
						hookFunc("hook_filename_unlinkat"),
						hookFunc("rethook_filename_unlinkat"),
					}},
				},
			},

			// Rmdir probes
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_security_inode_rmdir"),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "rmdir", hasFentry, EntryAndExit)},
			&manager.BestEffort{Selectors: rmdirIOUringProbes},

			// Unlink probes
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_vfs_unlink"),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "unlink", hasFentry, EntryAndExit)},
			&manager.BestEffort{Selectors: unlinkIOUringProbes},

			// ioctl probes
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_security_file_ioctl"),
			}},

			// Link
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				// source dentry
				hookFunc("hook_complete_walk"),
				// target dentry
				&manager.OneOf{Selectors: []manager.ProbesSelector{
					hookFunc("rethook_filename_create"),
					hookFunc("rethook___lookup_hash"),
				}},
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "link", hasFentry, EntryAndExit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "linkat", hasFentry, EntryAndExit)},
			&manager.BestEffort{Selectors: linkIOUringProbes},

			// selinux
			// This needs to be best effort, as sel_write_disable is in the process of being removed
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				hookFunc("hook_sel_write_disable"),
				hookFunc("hook_sel_write_enforce"),
				hookFunc("hook_sel_write_bool"),
				hookFunc("hook_sel_commit_bools_write"),
			}}},

		// List of probes required to capture chmod events
		"chmod": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_mnt_want_write"),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "chmod", hasFentry, EntryAndExit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "fchmod", hasFentry, EntryAndExit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "fchmodat", hasFentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "fchmodat2", hasFentry, EntryAndExit)},
		},

		// List of probes required to capture chown events
		"chown": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_mnt_want_write"),
			}},
			&manager.OneOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_mnt_want_write_file"),
				hookFunc("hook_mnt_want_write_file_path"),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "chown", hasFentry, EntryAndExit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "chown16", hasFentry, EntryAndExit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "fchown", hasFentry, EntryAndExit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "fchown16", hasFentry, EntryAndExit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "fchownat", hasFentry, EntryAndExit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "lchown", hasFentry, EntryAndExit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "lchown16", hasFentry, EntryAndExit)},
		},

		// List of probes required to capture mkdir events
		"mkdir": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_vfs_mkdir"),
				&manager.OneOf{Selectors: []manager.ProbesSelector{
					hookFunc("hook_filename_create"),
					hookFunc("hook_security_path_mkdir"),
				}},
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "mkdir", hasFentry, EntryAndExit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "mkdirat", hasFentry, EntryAndExit)},
			&manager.BestEffort{Selectors: mkdirIOUringProbes}},

		// List of probes required to capture removexattr events
		"removexattr": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_vfs_removexattr"),
				hookFunc("hook_mnt_want_write"),
			}},
			&manager.OneOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_mnt_want_write_file"),
				hookFunc("hook_mnt_want_write_file_path"),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "removexattr", hasFentry, EntryAndExit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "fremovexattr", hasFentry, EntryAndExit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "lremovexattr", hasFentry, EntryAndExit)},
		},

		// List of probes required to capture setxattr events
		"setxattr": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_vfs_setxattr"),
				hookFunc("hook_mnt_want_write"),
			}},
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				hookFunc("hook_io_fsetxattr"),
				hookFunc("rethook_io_fsetxattr"),
				hookFunc("hook_io_setxattr"),
				hookFunc("rethook_io_setxattr"),
			}},
			&manager.OneOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_mnt_want_write_file"),
				hookFunc("hook_mnt_want_write_file_path"),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setxattr", hasFentry, EntryAndExit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "fsetxattr", hasFentry, EntryAndExit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "lsetxattr", hasFentry, EntryAndExit)},
		},

		// List of probes required to capture utimes events
		"utimes": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_mnt_want_write"),
			}},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "utime", hasFentry, EntryAndExit, true)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "utime32", hasFentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "utimes", hasFentry, EntryAndExit, true)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "utimes", hasFentry, EntryAndExit|ExpandTime32)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "utimensat", hasFentry, EntryAndExit, true)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "utimensat", hasFentry, EntryAndExit|ExpandTime32)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "futimesat", hasFentry, EntryAndExit, true)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "futimesat", hasFentry, EntryAndExit|ExpandTime32)},
		},

		// List of probes required to capture bpf events
		"bpf": {
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				hookFunc("hook_security_bpf_map"),
				hookFunc("hook_security_bpf_prog"),
				hookFunc("hook_check_helper_call"),
			}},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "bpf", hasFentry, EntryAndExit)},
		},

		// List of probes required to capture ptrace events
		"ptrace": {
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "ptrace", hasFentry, EntryAndExit)},
			&manager.OneOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_ptrace_check_attach"),
				hookFunc("hook_arch_ptrace"),
			}},
		},

		// List of probes required to capture mmap events
		"mmap": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_vm_mmap_pgoff"),
				hookFunc("rethook_vm_mmap_pgoff"),
				hookFunc("hook_security_mmap_file"),
			}},
			// get_unmapped_area is inlined since kernel 6.13; fall back to __get_unmapped_area,
			// which keeps pgoff in the same argument position, so we can still read the mmap offset
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				&manager.OneOf{Selectors: []manager.ProbesSelector{
					hookFunc("hook_get_unmapped_area"),
					hookFunc("hook___get_unmapped_area"),
				}},
			}},
		},

		// List of probes required to capture mprotect events
		"mprotect": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_security_file_mprotect"),
			}},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "mprotect", hasFentry, EntryAndExit)},
		},

		// List of probes required to capture kernel load_module events
		"load_module": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				&manager.OneOf{Selectors: []manager.ProbesSelector{
					hookFunc("hook_security_kernel_read_file"),
					hookFunc("hook_security_kernel_module_from_file"),
				}},
				&manager.OneOf{Selectors: []manager.ProbesSelector{
					hookFunc("hook_mod_sysfs_setup"),
					hookFunc("hook_module_param_sysfs_setup"),
				}},
			}},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "init_module", hasFentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "finit_module", hasFentry, EntryAndExit)},
		},

		// List of probes required to capture kernel unload_module events
		"unload_module": {
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "delete_module", hasFentry, EntryAndExit)},
		},

		// List of probes required to capture signal events
		"signal": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				hookFunc("rethook_check_kill_permission"),
				hookFunc("hook_check_kill_permission"),
			}},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "kill", hasFentry, Entry)},
		},

		// List of probes required to capture setsockopt events
		"setsockopt": {
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setsockopt", hasFentry, EntryAndExit)},
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_security_socket_setsockopt"),
				hookFunc("hook_sk_attach_filter"),
				hookFunc("hook_release_sock"),
				hookFunc("rethook_release_sock"),
			}},
		},

		// List of probes required to capture splice events
		"splice": {
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "splice", hasFentry, EntryAndExit)},
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_get_pipe_info"),
				hookFunc("rethook_get_pipe_info"),
			}},
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				hookFunc("hook_io_issue_sqe"),
				hookFunc("rethook_io_issue_sqe"),
			}}},

		// List of probes required to capture accept events
		"accept": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_accept"),
			}},
		},
		// List of probes required to capture bind events
		"bind": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_security_socket_bind"),
			}},
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				hookFunc("hook_io_bind"),
				hookFunc("rethook_io_bind"),
			}},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "bind", hasFentry, EntryAndExit)},
		},
		// List of probes required to capture connect events
		"connect": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_security_socket_connect"),
			}},
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				hookFunc("hook_io_connect"),
				hookFunc("rethook_io_connect"),
			}},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "connect", hasFentry, EntryAndExit)},
		},
		// List of probes required to capture socket events
		"socket": {
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "socket", hasFentry, EntryAndExit)},
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				hookFunc("hook_io_socket"),
				hookFunc("rethook_io_socket"),
			}},
		},

		// List of probes required to capture chdir events
		"chdir": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_set_fs_pwd"),
			}},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "chdir", hasFentry, EntryAndExit)},
			&manager.OneOf{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "fchdir", hasFentry, EntryAndExit)},
		},

		// List of probes required to capture network_flow_monitor events
		"network_flow_monitor": {
			// perf_event probes
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				&manager.ProbeSelector{
					ProbeIdentificationPair: manager.ProbeIdentificationPair{
						UID:          SecurityAgentUID,
						EBPFFuncName: "network_stats_worker",
					},
				},
			}},
		},

		// List of probes required to capture sysctl events
		"sysctl": {
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				&manager.ProbeSelector{
					ProbeIdentificationPair: manager.ProbeIdentificationPair{
						UID:          SecurityAgentUID,
						EBPFFuncName: SysCtlProbeFunctionName,
					},
				},
				hookFunc("hook_proc_sys_call_handler"),
			}},
		},
		"setrlimit": {
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "setrlimit", hasFentry, EntryAndExit)},
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "prlimit64", hasFentry, EntryAndExit)},
			&manager.AllOf{Selectors: []manager.ProbesSelector{
				hookFunc("hook_security_task_setrlimit"),
			}},
		},
		"prctl": {
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "prctl", hasFentry, EntryAndExit)},
		},
		"tracer_memfd_seal": {
			&manager.BestEffort{Selectors: ExpandSyscallProbesSelector(SecurityAgentUID, "memfd_create", hasFentry, EntryAndExit)},
			&manager.BestEffort{Selectors: []manager.ProbesSelector{
				hookFunc("hook_memfd_fcntl"),
				hookFunc("hook_shmem_fcntl"),
			}},
		},
	}

	// Register the network event types so they are correctly reflected in the enabled_events map when
	// requested by rules, activity dumps, security profiles or event sampling. The probes that track
	// network interfaces and sockets are activated in updateProbes whenever the network
	// feature is enabled (see GetNetworkSelectors), because they are required to track these resources.
	networkEventTypes := model.GetEventTypePerCategory(model.NetworkCategory)[model.NetworkCategory]
	for _, networkEventType := range networkEventTypes {
		if model.EventTypeDependsOnInterfaceTracking(networkEventType) {
			selectorsPerEventTypeStore[networkEventType] = []manager.ProbesSelector{}
		}
	}

	if ShouldUseModuleLoadTracepoint() {
		selectorsPerEventTypeStore["load_module"] = append(selectorsPerEventTypeStore["load_module"], &manager.BestEffort{Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: SecurityAgentUID, EBPFFuncName: "module_load"}},
		}})
	}

	return selectorsPerEventTypeStore
}
