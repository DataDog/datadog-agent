// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probes

import "github.com/DataDog/ebpf/manager"

// getDentryResolverTailCallRoutes is the list of routes used during the dentry resolution process
func getDentryResolverTailCallRoutes() []manager.TailCallRoute {
	return []manager.TailCallRoute{
		// dentry resolver programs
		{
			ProgArrayName: "dentry_resolver_kprobe_progs",
			Key:           DentryResolverERPCKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				Section: "kprobe/dentry_resolver_erpc",
			},
		},
		{
			ProgArrayName: "dentry_resolver_kprobe_progs",
			Key:           DentryResolverKernKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				Section: "kprobe/dentry_resolver_kern",
			},
		},
		{
			ProgArrayName: "dentry_resolver_tracepoint_progs",
			Key:           DentryResolverKernTracepointKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				Section: "tracepoint/dentry_resolver_kern",
			},
		},

		// dentry resolver kprobe callbacks
		{
			ProgArrayName: "dentry_resolver_kprobe_callbacks",
			Key:           DentryResolverOpenCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				Section: "kprobe/dr_open_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_kprobe_callbacks",
			Key:           DentryResolverSetAttrCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				Section: "kprobe/dr_setattr_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_kprobe_callbacks",
			Key:           DentryResolverMkdirCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				Section: "kprobe/dr_mkdir_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_kprobe_callbacks",
			Key:           DentryResolverMountCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				Section: "kprobe/dr_mount_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_kprobe_callbacks",
			Key:           DentryResolverSecurityInodeRmdirCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				Section: "kprobe/dr_security_inode_rmdir_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_kprobe_callbacks",
			Key:           DentryResolverSetXAttrCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				Section: "kprobe/dr_setxattr_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_kprobe_callbacks",
			Key:           DentryResolverUnlinkCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				Section: "kprobe/dr_unlink_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_kprobe_callbacks",
			Key:           DentryResolverLinkSrcCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				Section: "kprobe/dr_link_src_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_kprobe_callbacks",
			Key:           DentryResolverLinkDstCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				Section: "kprobe/dr_link_dst_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_kprobe_callbacks",
			Key:           DentryResolverRenameCallbackKprobeKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				Section: "kprobe/dr_rename_callback",
			},
		},

		// dentry resolver tracepoint callbacks
		{
			ProgArrayName: "dentry_resolver_tracepoint_callbacks",
			Key:           DentryResolverOpenCallbackTracepointKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				Section: "tracepoint/dr_open_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_tracepoint_callbacks",
			Key:           DentryResolverMkdirCallbackTracepointKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				Section: "tracepoint/dr_mkdir_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_tracepoint_callbacks",
			Key:           DentryResolverMountCallbackTracepointKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				Section: "tracepoint/dr_mount_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_tracepoint_callbacks",
			Key:           DentryResolverLinkDstCallbackTracepointKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				Section: "tracepoint/dr_link_dst_callback",
			},
		},
		{
			ProgArrayName: "dentry_resolver_tracepoint_callbacks",
			Key:           DentryResolverRenameCallbackTracepointKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				Section: "tracepoint/dr_rename_callback",
			},
		},
	}
}
