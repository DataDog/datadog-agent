// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probes

import (
	"github.com/DataDog/datadog-agent/pkg/security/model"
	"github.com/DataDog/ebpf/manager"
)

func getSysExitTailCallRoutes() []manager.TailCallRoute {
	return []manager.TailCallRoute{
		{
			ProgArrayName: "sys_exit_progs",
			Key:           uint32(model.FileChmodEventType),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				Section: "tracepoint/handle_sys_chmod_exit",
			},
		},
		{
			ProgArrayName: "sys_exit_progs",
			Key:           uint32(model.FileChownEventType),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				Section: "tracepoint/handle_sys_chown_exit",
			},
		},
		{
			ProgArrayName: "sys_exit_progs",
			Key:           uint32(model.FileLinkEventType),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				Section: "tracepoint/handle_sys_link_exit",
			},
		},
		{
			ProgArrayName: "sys_exit_progs",
			Key:           uint32(model.FileMkdirEventType),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				Section: "tracepoint/handle_sys_mkdir_exit",
			},
		},
		{
			ProgArrayName: "sys_exit_progs",
			Key:           uint32(model.FileMountEventType),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				Section: "tracepoint/handle_sys_mount_exit",
			},
		},
		{
			ProgArrayName: "sys_exit_progs",
			Key:           uint32(model.FileOpenEventType),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				Section: "tracepoint/handle_sys_open_exit",
			},
		},
		{
			ProgArrayName: "sys_exit_progs",
			Key:           uint32(model.FileRenameEventType),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				Section: "tracepoint/handle_sys_rename_exit",
			},
		},
		{
			ProgArrayName: "sys_exit_progs",
			Key:           uint32(model.FileRmdirEventType),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				Section: "tracepoint/handle_sys_rmdir_exit",
			},
		},
		{
			ProgArrayName: "sys_exit_progs",
			Key:           uint32(model.FileSetXAttrEventType),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				Section: "tracepoint/handle_sys_setxattr_exit",
			},
		},
		{
			ProgArrayName: "sys_exit_progs",
			Key:           uint32(model.FileRemoveXAttrEventType),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				Section: "tracepoint/handle_sys_removexattr_exit",
			},
		},
		{
			ProgArrayName: "sys_exit_progs",
			Key:           uint32(model.FileUmountEventType),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				Section: "tracepoint/handle_sys_umount_exit",
			},
		},
		{
			ProgArrayName: "sys_exit_progs",
			Key:           uint32(model.FileUnlinkEventType),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				Section: "tracepoint/handle_sys_unlink_exit",
			},
		},
		{
			ProgArrayName: "sys_exit_progs",
			Key:           uint32(model.FileUtimeEventType),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				Section: "tracepoint/handle_sys_utimes_exit",
			},
		},
		{
			ProgArrayName: "sys_exit_progs",
			Key:           uint32(model.SetuidEventType),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				Section: "tracepoint/handle_sys_commit_creds_exit",
			},
		},
		{
			ProgArrayName: "sys_exit_progs",
			Key:           uint32(model.SetgidEventType),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				Section: "tracepoint/handle_sys_commit_creds_exit",
			},
		},
		{
			ProgArrayName: "sys_exit_progs",
			Key:           uint32(model.CapsetEventType),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				Section: "tracepoint/handle_sys_commit_creds_exit",
			},
		},
	}
}
