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
func getDentryResolverTailCallRoutes(ERPCDentryResolutionEnabled, supportMmapableMaps bool) []manager.TailCallRoute {
	dentryResolverProgs := "dentry_resolver_kprobe_or_fentry_progs"
	dentryCallbackProgs := "dentry_resolver_kprobe_or_fentry_callbacks"

	routes := []manager.TailCallRoute{
		// activity dump filter programs
		{
			ProgArrayName: dentryResolverProgs,
			Key:           ActivityDumpFilterKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tail_call_target_dentry_resolver_ad_filter",
			},
		},
		{
			ProgArrayName: "dentry_resolver_tracepoint_progs",
			Key:           ActivityDumpFilterKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tracepoint_dentry_resolver_ad_filter",
			},
		},

		// dentry resolver programs
		{
			ProgArrayName: dentryResolverProgs,
			Key:           DentryResolverKernKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tail_call_target_dentry_resolver_kern",
			},
		},
		{
			ProgArrayName: "dentry_resolver_tracepoint_progs",
			Key:           DentryResolverKernKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tracepoint_dentry_resolver_kern",
			},
		},
	}

	routes = append(routes, []manager.TailCallRoute{
		// dentry resolver kprobe callbacks
		{
			ProgArrayName: dentryCallbackProgs,
			Key:           DentryResolverOpenCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tail_call_target_dr_open_callback",
			},
		},
		{
			ProgArrayName: dentryCallbackProgs,
			Key:           DentryResolverSetAttrCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tail_call_target_dr_setattr_callback",
			},
		},
		{
			ProgArrayName: dentryCallbackProgs,
			Key:           DentryResolverMkdirCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tail_call_target_dr_mkdir_callback",
			},
		},
		{
			ProgArrayName: dentryCallbackProgs,
			Key:           DentryResolverMountStageOneCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tail_call_target_dr_mount_stage_one_callback",
			},
		},
		{
			ProgArrayName: dentryCallbackProgs,
			Key:           DentryResolverMountStageTwoCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tail_call_target_dr_mount_stage_two_callback",
			},
		},
		{
			ProgArrayName: dentryCallbackProgs,
			Key:           DentryResolverSecurityInodeRmdirCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tail_call_target_dr_security_inode_rmdir_callback",
			},
		},
		{
			ProgArrayName: dentryCallbackProgs,
			Key:           DentryResolverSetXAttrCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tail_call_target_dr_setxattr_callback",
			},
		},
		{
			ProgArrayName: dentryCallbackProgs,
			Key:           DentryResolverUnlinkCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tail_call_target_dr_unlink_callback",
			},
		},
		{
			ProgArrayName: dentryCallbackProgs,
			Key:           DentryResolverLinkSrcCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tail_call_target_dr_link_src_callback",
			},
		},
		{
			ProgArrayName: dentryCallbackProgs,
			Key:           DentryResolverLinkDstCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tail_call_target_dr_link_dst_callback",
			},
		},
		{
			ProgArrayName: dentryCallbackProgs,
			Key:           DentryResolverRenameCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tail_call_target_dr_rename_callback",
			},
		},
		{
			ProgArrayName: dentryCallbackProgs,
			Key:           DentryResolverSELinuxCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tail_call_target_dr_selinux_callback",
			},
		},
	}...)

	routes = append(routes, []manager.TailCallRoute{
		// dentry resolver tracepoint callbacks
		{
			ProgArrayName: "dentry_resolver_tracepoint_callbacks",
			Key:           DentryResolverOpenCallbackTracepointKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tracepoint_dr_open_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_tracepoint_callbacks",
			Key:           DentryResolverMkdirCallbackTracepointKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tracepoint_dr_mkdir_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_tracepoint_callbacks",
			Key:           DentryResolverMountStageOneCallbackTracepointKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tracepoint_dr_mount_stage_one_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_tracepoint_callbacks",
			Key:           DentryResolverMountStageTwoCallbackTracepointKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tracepoint_dr_mount_stage_two_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_tracepoint_callbacks",
			Key:           DentryResolverLinkDstCallbackTracepointKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tracepoint_dr_link_dst_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_tracepoint_callbacks",
			Key:           DentryResolverRenameCallbackTracepointKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tracepoint_dr_rename_callback",
			},
		},
	}...)

	// add routes for programs with the bpf_probe_write_user only if necessary
	if ERPCDentryResolutionEnabled {
		ebpfSuffix := "_mmap"
		if !supportMmapableMaps {
			ebpfSuffix = "_write_user"
		}

		routes = append(routes, []manager.TailCallRoute{
			{
				ProgArrayName: dentryResolverProgs,
				Key:           DentryResolverERPCKey,
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "tail_call_target_dentry_resolver_erpc" + ebpfSuffix,
				},
			},
			{
				ProgArrayName: dentryResolverProgs,
				Key:           DentryResolverParentERPCKey,
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "tail_call_target_dentry_resolver_parent_erpc" + ebpfSuffix,
				},
			},
			{
				ProgArrayName: dentryResolverProgs,
				Key:           DentryResolverSegmentERPCKey,
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "tail_call_target_dentry_resolver_segment_erpc" + ebpfSuffix,
				},
			},
		}...)
	}

	return routes
}
