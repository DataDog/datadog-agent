// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probes holds probes related files
package probes

import (
	manager "github.com/DataDog/ebpf-manager"
)

// getDentryResolverTailCallRoutes is the list of routes used during the dentry resolution process
func getDentryResolverTailCallRoutes(ERPCDentryResolutionEnabled, supportMmapableMaps bool, fentry bool) []manager.TailCallRoute {

	var routes []manager.TailCallRoute

	// tracepoint routes
	routes = append(routes, []manager.TailCallRoute{
		// skip index 0 as it is used for the DR_NO_CALLBACK check
		{
			ProgArrayName: "dr_tracepoint_progs",
			Key:           1,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tracepoint_dentry_resolver_entrypoint",
			},
		},
		{
			ProgArrayName: "dr_tracepoint_progs",
			Key:           2,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tracepoint_dentry_resolver_loop",
			},
		},
		{
			ProgArrayName: "dr_tracepoint_progs",
			Key:           3,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tracepoint_dr_open_callback",
			},
		},
		{
			ProgArrayName: "dr_tracepoint_progs",
			Key:           4,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tracepoint_dr_mkdir_callback",
			},
		},
		{
			ProgArrayName: "dr_tracepoint_progs",
			Key:           5,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tracepoint_dr_mount_stage_one_callback",
			},
		},
		{
			ProgArrayName: "dr_tracepoint_progs",
			Key:           6,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tracepoint_dr_mount_stage_two_callback",
			},
		},
		{
			ProgArrayName: "dr_tracepoint_progs",
			Key:           7,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tracepoint_dr_link_dst_callback",
			},
		},
		{
			ProgArrayName: "dr_tracepoint_progs",
			Key:           8,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tracepoint_dr_rename_callback",
			},
		},
	}...)

	// kprobe or fentry routes
	routes = append(routes, []manager.TailCallRoute{
		// skip index 0 as it is used for the DR_NO_CALLBACK check
		{
			ProgArrayName: "dr_kprobe_or_fentry_progs",
			Key:           1,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tail_call_target_dentry_resolver_entrypoint",
			},
		},
		{
			ProgArrayName: "dr_kprobe_or_fentry_progs",
			Key:           2,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tail_call_target_dentry_resolver_loop",
			},
		},
		{
			ProgArrayName: "dr_kprobe_or_fentry_progs",
			Key:           3,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tail_call_target_dr_open_callback",
			},
		},
		{
			ProgArrayName: "dr_kprobe_or_fentry_progs",
			Key:           4,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tail_call_target_dr_mkdir_callback",
			},
		},
		{
			ProgArrayName: "dr_kprobe_or_fentry_progs",
			Key:           5,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tail_call_target_dr_mount_stage_one_callback",
			},
		},
		{
			ProgArrayName: "dr_kprobe_or_fentry_progs",
			Key:           6,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tail_call_target_dr_mount_stage_two_callback",
			},
		},
		{
			ProgArrayName: "dr_kprobe_or_fentry_progs",
			Key:           7,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tail_call_target_dr_link_dst_callback",
			},
		},
		{
			ProgArrayName: "dr_kprobe_or_fentry_progs",
			Key:           8,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tail_call_target_dr_rename_callback",
			},
		},
		{
			ProgArrayName: "dr_kprobe_or_fentry_progs",
			Key:           9,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tail_call_target_dr_executable_path_cb",
			},
		},
		{
			ProgArrayName: "dr_kprobe_or_fentry_progs",
			Key:           10,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tail_call_target_dr_interpreter_path_cb",
			},
		},
		{
			ProgArrayName: "dr_kprobe_or_fentry_progs",
			Key:           11,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tail_call_target_dr_link_src_callback",
			},
		},
		{
			ProgArrayName: "dr_kprobe_or_fentry_progs",
			Key:           12,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tail_call_target_dr_rename_src_callback",
			},
		},
		{
			ProgArrayName: "dr_kprobe_or_fentry_progs",
			Key:           13,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tail_call_target_dr_security_inode_rmdir_callback",
			},
		},
		{
			ProgArrayName: "dr_kprobe_or_fentry_progs",
			Key:           14,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tail_call_target_dr_selinux_callback",
			},
		},
		{
			ProgArrayName: "dr_kprobe_or_fentry_progs",
			Key:           15,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tail_call_target_dr_setattr_callback",
			},
		},
		{
			ProgArrayName: "dr_kprobe_or_fentry_progs",
			Key:           16,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tail_call_target_dr_setxattr_callback",
			},
		},
		{
			ProgArrayName: "dr_kprobe_or_fentry_progs",
			Key:           17,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tail_call_target_dr_unlink_callback",
			},
		},
		{
			ProgArrayName: "dr_kprobe_or_fentry_progs",
			Key:           18,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tail_call_target_dr_init_module_callback",
			},
		},
	}...)

	if ERPCDentryResolutionEnabled {
		if !supportMmapableMaps {
			routes = append(routes, []manager.TailCallRoute{
				{
					ProgArrayName: "erpc_kprobe_or_fentry_progs",
					Key:           ERPCResolveParentDentryKey,
					ProbeIdentificationPair: manager.ProbeIdentificationPair{
						EBPFFuncName: "tail_call_target_erpc_resolve_parent_write_user",
					},
				},
				{
					ProgArrayName: "erpc_kprobe_or_fentry_progs",
					Key:           ERPCResolvePathWatermarkReaderKey,
					ProbeIdentificationPair: manager.ProbeIdentificationPair{
						EBPFFuncName: "tail_call_target_erpc_resolve_path_watermark_reader",
					},
				},
				{
					ProgArrayName: "erpc_kprobe_or_fentry_progs",
					Key:           ERPCResolvePathSegmentkReaderKey,
					ProbeIdentificationPair: manager.ProbeIdentificationPair{
						EBPFFuncName: "tail_call_target_erpc_resolve_path_segment_reader",
					},
				},
			}...)
		} else {
			routes = append(routes, []manager.TailCallRoute{
				{
					ProgArrayName: "erpc_kprobe_or_fentry_progs",
					Key:           ERPCResolveParentDentryKey,
					ProbeIdentificationPair: manager.ProbeIdentificationPair{
						EBPFFuncName: "tail_call_target_erpc_resolve_parent_mmap",
					},
				},
			}...)
		}
	}

	return routes
}
