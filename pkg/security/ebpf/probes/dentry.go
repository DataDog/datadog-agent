// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package probes

import (
	"fmt"

	manager "github.com/DataDog/ebpf-manager"
)

// getDentryResolverTailCallRoutes is the list of routes used during the dentry resolution process
func getDentryResolverTailCallRoutes(ERPCDentryResolutionEnabled, supportMmapableMaps bool, fentry bool) []manager.TailCallRoute {
	routes := []manager.TailCallRoute{
		// activity dump filter programs
		{
			ProgArrayName: "dentry_resolver_kprobe_progs",
			Key:           ActivityDumpFilterKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "kprobe_dentry_resolver_ad_filter",
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
			ProgArrayName: "dentry_resolver_kprobe_progs",
			Key:           DentryResolverKernKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "kprobe_dentry_resolver_kern",
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

	if fentry {
		routes = append(routes, []manager.TailCallRoute{
			{
				ProgArrayName: "dentry_resolver_fentry_progs",
				Key:           ActivityDumpFilterKey,
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "fentry_dentry_resolver_ad_filter",
				},
			},
			{
				ProgArrayName: "dentry_resolver_fentry_progs",
				Key:           DentryResolverKernKey,
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "fentry_dentry_resolver_kern",
				},
			},
		}...)
	}

	prefixes := []string{"kprobe"}
	if fentry {
		prefixes = append(prefixes, "fentry")
	}

	for _, prefix := range prefixes {
		progArrayName := fmt.Sprintf("dentry_resolver_%s_callbacks", prefix)

		routes = append(routes, []manager.TailCallRoute{
			// dentry resolver kprobe callbacks
			{
				ProgArrayName: progArrayName,
				Key:           DentryResolverOpenCallbackKprobeKey,
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: prefix + "_dr_open_callback",
				},
			},
			{
				ProgArrayName: progArrayName,
				Key:           DentryResolverSetAttrCallbackKprobeKey,
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: prefix + "_dr_setattr_callback",
				},
			},
			{
				ProgArrayName: progArrayName,
				Key:           DentryResolverMkdirCallbackKprobeKey,
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: prefix + "_dr_mkdir_callback",
				},
			},
			{
				ProgArrayName: progArrayName,
				Key:           DentryResolverMountCallbackKprobeKey,
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: prefix + "_dr_mount_callback",
				},
			},
			{
				ProgArrayName: progArrayName,
				Key:           DentryResolverSecurityInodeRmdirCallbackKprobeKey,
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: prefix + "_dr_security_inode_rmdir_callback",
				},
			},
			{
				ProgArrayName: progArrayName,
				Key:           DentryResolverSetXAttrCallbackKprobeKey,
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: prefix + "_dr_setxattr_callback",
				},
			},
			{
				ProgArrayName: progArrayName,
				Key:           DentryResolverUnlinkCallbackKprobeKey,
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: prefix + "_dr_unlink_callback",
				},
			},
			{
				ProgArrayName: progArrayName,
				Key:           DentryResolverLinkSrcCallbackKprobeKey,
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: prefix + "_dr_link_src_callback",
				},
			},
			{
				ProgArrayName: progArrayName,
				Key:           DentryResolverLinkDstCallbackKprobeKey,
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: prefix + "_dr_link_dst_callback",
				},
			},
			{
				ProgArrayName: progArrayName,
				Key:           DentryResolverRenameCallbackKprobeKey,
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: prefix + "_dr_rename_callback",
				},
			},
			{
				ProgArrayName: progArrayName,
				Key:           DentryResolverSELinuxCallbackKprobeKey,
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: prefix + "_dr_selinux_callback",
				},
			},
			{
				ProgArrayName: progArrayName,
				Key:           DentryResolverUnshareMntNSStageOneCallbackKprobeKey,
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: prefix + "_dr_unshare_mntns_stage_one_callback",
				},
			},
			{
				ProgArrayName: progArrayName,
				Key:           DentryResolverUnshareMntNSStageTwoCallbackKprobeKey,
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: prefix + "_dr_unshare_mntns_stage_two_callback",
				},
			},
		}...)
	}

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
	}...)

	for _, prefix := range prefixes {
		// add routes for programs with the bpf_probe_write_user only if necessary
		if ERPCDentryResolutionEnabled {
			ebpfSuffix := "_mmap"
			if !supportMmapableMaps {
				ebpfSuffix = "_write_user"
			}

			progArrayName := fmt.Sprintf("dentry_resolver_%s_progs", prefix)

			routes = append(routes, []manager.TailCallRoute{
				{
					ProgArrayName: progArrayName,
					Key:           DentryResolverERPCKey,
					ProbeIdentificationPair: manager.ProbeIdentificationPair{
						EBPFFuncName: prefix + "_dentry_resolver_erpc" + ebpfSuffix,
					},
				},
				{
					ProgArrayName: progArrayName,
					Key:           DentryResolverParentERPCKey,
					ProbeIdentificationPair: manager.ProbeIdentificationPair{
						EBPFFuncName: prefix + "_dentry_resolver_parent_erpc" + ebpfSuffix,
					},
				},
				{
					ProgArrayName: progArrayName,
					Key:           DentryResolverSegmentERPCKey,
					ProbeIdentificationPair: manager.ProbeIdentificationPair{
						EBPFFuncName: prefix + "_dentry_resolver_segment_erpc" + ebpfSuffix,
					},
				},
			}...)
		}
	}

	return routes
}
