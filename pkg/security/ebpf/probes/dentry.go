// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package probes

import manager "github.com/DataDog/ebpf-manager"

// getDentryResolverTailCallRoutes is the list of routes used during the dentry resolution process
func getDentryResolverTailCallRoutes(ERPCDentryResolutionEnabled, supportMmapableMaps bool) []manager.TailCallRoute {
	routes := []manager.TailCallRoute{
		// activity dump filter programs
		{
			ProgArrayName: "dentry_resolver_kprobe_progs",
			Key:           ActivityDumpFilterKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "kprobe_dentry_resolver_ad_filter",
			},
		},
		{
			ProgArrayName: "dentry_resolver_tracepoint_progs",
			Key:           ActivityDumpFilterTracepointKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tracepoint_dentry_resolver_ad_filter",
			},
		},

		// dentry resolver programs
		{
			ProgArrayName: "dentry_resolver_kprobe_progs",
			Key:           DentryResolverKernKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "kprobe_dentry_resolver_kern",
			},
		},
		{
			ProgArrayName: "dentry_resolver_tracepoint_progs",
			Key:           DentryResolverKernTracepointKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tracepoint_dentry_resolver_kern",
			},
		},

		// dentry resolver kprobe callbacks
		{
			ProgArrayName: "dentry_resolver_kprobe_callbacks",
			Key:           DentryResolverOpenCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "kprobe_dr_open_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_kprobe_callbacks",
			Key:           DentryResolverSetAttrCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "kprobe_dr_setattr_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_kprobe_callbacks",
			Key:           DentryResolverMkdirCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "kprobe_dr_mkdir_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_kprobe_callbacks",
			Key:           DentryResolverMountCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "kprobe_dr_mount_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_kprobe_callbacks",
			Key:           DentryResolverSecurityInodeRmdirCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "kprobe_dr_security_inode_rmdir_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_kprobe_callbacks",
			Key:           DentryResolverSetXAttrCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "kprobe_dr_setxattr_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_kprobe_callbacks",
			Key:           DentryResolverUnlinkCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "kprobe_dr_unlink_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_kprobe_callbacks",
			Key:           DentryResolverLinkSrcCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "kprobe_dr_link_src_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_kprobe_callbacks",
			Key:           DentryResolverLinkDstCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "kprobe_dr_link_dst_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_kprobe_callbacks",
			Key:           DentryResolverRenameCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "kprobe_dr_rename_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_kprobe_callbacks",
			Key:           DentryResolverSELinuxCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "kprobe_dr_selinux_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_kprobe_callbacks",
			Key:           DentryResolverUnshareMntNSStageOneCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "kprobe_dr_unshare_mntns_stage_one_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_kprobe_callbacks",
			Key:           DentryResolverUnshareMntNSStageTwoCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "kprobe_dr_unshare_mntns_stage_two_callback",
			},
		},

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
			Key:           DentryResolverMountCallbackTracepointKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tracepoint_dr_mount_callback",
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
	}

	// add routes for programs with the bpf_probe_write_user only if necessary
	if ERPCDentryResolutionEnabled {
		ebpfSuffix := "_mmap"
		if !supportMmapableMaps {
			ebpfSuffix = "_write_user"
		}

		routes = append(routes, []manager.TailCallRoute{
			{
				ProgArrayName: "dentry_resolver_kprobe_progs",
				Key:           DentryResolverERPCKey,
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "kprobe_dentry_resolver_erpc" + ebpfSuffix,
				},
			},
			{
				ProgArrayName: "dentry_resolver_kprobe_progs",
				Key:           DentryResolverParentERPCKey,
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "kprobe_dentry_resolver_parent_erpc" + ebpfSuffix,
				},
			},
			{
				ProgArrayName: "dentry_resolver_kprobe_progs",
				Key:           DentryResolverSegmentERPCKey,
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "kprobe_dentry_resolver_segment_erpc" + ebpfSuffix,
				},
			},
		}...)
	}

	return routes
}
