// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

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
				EBPFSection:  "kprobe/dentry_resolver_ad_filter",
				EBPFFuncName: "kprobe_dentry_resolver_ad_filter",
			},
		},
		{
			ProgArrayName: "dentry_resolver_tracepoint_progs",
			Key:           ActivityDumpFilterTracepointKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFSection:  "tracepoint/dentry_resolver_ad_filter",
				EBPFFuncName: "tracepoint_dentry_resolver_ad_filter",
			},
		},

		// dentry resolver programs
		{
			ProgArrayName: "dentry_resolver_kprobe_progs",
			Key:           DentryResolverKernKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFSection:  "kprobe/dentry_resolver_kern",
				EBPFFuncName: "kprobe_dentry_resolver_kern",
			},
		},
		{
			ProgArrayName: "dentry_resolver_tracepoint_progs",
			Key:           DentryResolverKernTracepointKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFSection:  "tracepoint/dentry_resolver_kern",
				EBPFFuncName: "tracepoint_dentry_resolver_kern",
			},
		},

		// dentry resolver kprobe callbacks
		{
			ProgArrayName: "dentry_resolver_kprobe_callbacks",
			Key:           DentryResolverOpenCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFSection:  "kprobe/dr_open_callback",
				EBPFFuncName: "kprobe_dr_open_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_kprobe_callbacks",
			Key:           DentryResolverSetAttrCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFSection:  "kprobe/dr_setattr_callback",
				EBPFFuncName: "kprobe_dr_setattr_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_kprobe_callbacks",
			Key:           DentryResolverMkdirCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFSection:  "kprobe/dr_mkdir_callback",
				EBPFFuncName: "kprobe_dr_mkdir_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_kprobe_callbacks",
			Key:           DentryResolverMountCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFSection:  "kprobe/dr_mount_callback",
				EBPFFuncName: "kprobe_dr_mount_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_kprobe_callbacks",
			Key:           DentryResolverSecurityInodeRmdirCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFSection:  "kprobe/dr_security_inode_rmdir_callback",
				EBPFFuncName: "kprobe_dr_security_inode_rmdir_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_kprobe_callbacks",
			Key:           DentryResolverSetXAttrCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFSection:  "kprobe/dr_setxattr_callback",
				EBPFFuncName: "kprobe_dr_setxattr_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_kprobe_callbacks",
			Key:           DentryResolverUnlinkCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFSection:  "kprobe/dr_unlink_callback",
				EBPFFuncName: "kprobe_dr_unlink_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_kprobe_callbacks",
			Key:           DentryResolverLinkSrcCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFSection:  "kprobe/dr_link_src_callback",
				EBPFFuncName: "kprobe_dr_link_src_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_kprobe_callbacks",
			Key:           DentryResolverLinkDstCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFSection:  "kprobe/dr_link_dst_callback",
				EBPFFuncName: "kprobe_dr_link_dst_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_kprobe_callbacks",
			Key:           DentryResolverRenameCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFSection:  "kprobe/dr_rename_callback",
				EBPFFuncName: "kprobe_dr_rename_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_kprobe_callbacks",
			Key:           DentryResolverSELinuxCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFSection:  "kprobe/dr_selinux_callback",
				EBPFFuncName: "kprobe_dr_selinux_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_kprobe_callbacks",
			Key:           DentryResolverUnshareMntNSStageOneCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFSection:  "kprobe/dr_unshare_mntns_stage_one_callback",
				EBPFFuncName: "kprobe_dr_unshare_mntns_stage_one_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_kprobe_callbacks",
			Key:           DentryResolverUnshareMntNSStageTwoCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFSection:  "kprobe/dr_unshare_mntns_stage_two_callback",
				EBPFFuncName: "kprobe_dr_unshare_mntns_stage_two_callback",
			},
		},

		// dentry resolver tracepoint callbacks
		{
			ProgArrayName: "dentry_resolver_tracepoint_callbacks",
			Key:           DentryResolverOpenCallbackTracepointKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFSection:  "tracepoint/dr_open_callback",
				EBPFFuncName: "tracepoint_dr_open_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_tracepoint_callbacks",
			Key:           DentryResolverMkdirCallbackTracepointKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFSection:  "tracepoint/dr_mkdir_callback",
				EBPFFuncName: "tracepoint_dr_mkdir_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_tracepoint_callbacks",
			Key:           DentryResolverMountCallbackTracepointKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFSection:  "tracepoint/dr_mount_callback",
				EBPFFuncName: "tracepoint_dr_mount_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_tracepoint_callbacks",
			Key:           DentryResolverLinkDstCallbackTracepointKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFSection:  "tracepoint/dr_link_dst_callback",
				EBPFFuncName: "tracepoint_dr_link_dst_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_tracepoint_callbacks",
			Key:           DentryResolverRenameCallbackTracepointKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFSection:  "tracepoint/dr_rename_callback",
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
					EBPFSection:  "kprobe/dentry_resolver_erpc" + ebpfSuffix,
					EBPFFuncName: "kprobe_dentry_resolver_erpc" + ebpfSuffix,
				},
			},
			{
				ProgArrayName: "dentry_resolver_kprobe_progs",
				Key:           DentryResolverParentERPCKey,
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  "kprobe/dentry_resolver_parent_erpc" + ebpfSuffix,
					EBPFFuncName: "kprobe_dentry_resolver_parent_erpc" + ebpfSuffix,
				},
			},
			{
				ProgArrayName: "dentry_resolver_kprobe_progs",
				Key:           DentryResolverSegmentERPCKey,
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  "kprobe/dentry_resolver_segment_erpc" + ebpfSuffix,
					EBPFFuncName: "kprobe_dentry_resolver_segment_erpc" + ebpfSuffix,
				},
			},
		}...)
	}

	return routes
}
