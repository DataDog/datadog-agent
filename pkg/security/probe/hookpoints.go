// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

import (
	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

// HookPoint represents
type HookPoint struct {
	Name       string
	KProbes    []*ebpf.KProbe
	Tracepoint string
	Optional   bool
	EventTypes []eval.EventType
}

var allHookPoints = []*HookPoint{
	{
		Name: "security_inode_setattr",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/security_inode_setattr",
		}},
		EventTypes: []eval.EventType{"chmod", "chown", "utimes"},
	},
	{
		Name:       "sys_chmod",
		KProbes:    syscallKprobe("chmod"),
		EventTypes: []eval.EventType{"chmod"},
	},
	{
		Name:       "sys_fchmod",
		KProbes:    syscallKprobe("fchmod"),
		EventTypes: []eval.EventType{"chmod"},
	},
	{
		Name:       "sys_fchmodat",
		KProbes:    syscallKprobe("fchmodat"),
		EventTypes: []eval.EventType{"chmod"},
	},
	{
		Name:       "sys_chown",
		KProbes:    syscallKprobe("chown"),
		EventTypes: []eval.EventType{"chown"},
	},
	{
		Name:       "sys_fchown",
		KProbes:    syscallKprobe("fchown"),
		EventTypes: []eval.EventType{"chown"},
	},
	{
		Name:       "sys_fchownat",
		KProbes:    syscallKprobe("fchownat"),
		EventTypes: []eval.EventType{"chown"},
	},
	{
		Name:       "sys_lchown",
		KProbes:    syscallKprobe("lchown"),
		EventTypes: []eval.EventType{"chown"},
	},
	{
		Name:       "sys_setxattr",
		KProbes:    syscallKprobe("setxattr"),
		EventTypes: []eval.EventType{"setxattr"},
	},
	{
		Name:       "sys_fsetxattr",
		KProbes:    syscallKprobe("fsetxattr"),
		EventTypes: []eval.EventType{"setxattr"},
	},
	{
		Name:       "sys_lsetxattr",
		KProbes:    syscallKprobe("lsetxattr"),
		EventTypes: []eval.EventType{"setxattr"},
	},
	{
		Name: "vfs_setxattr",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/vfs_setxattr",
		}},
		EventTypes: []eval.EventType{"setxattr"},
	},
	{
		Name:       "sys_removexattr",
		KProbes:    syscallKprobe("removexattr"),
		EventTypes: []eval.EventType{"removexattr"},
	},
	{
		Name:       "sys_fremovexattr",
		KProbes:    syscallKprobe("fremovexattr"),
		EventTypes: []eval.EventType{"removexattr"},
	},
	{
		Name:       "sys_lremovexattr",
		KProbes:    syscallKprobe("lremovexattr"),
		EventTypes: []eval.EventType{"removexattr"},
	},
	{
		Name: "vfs_removexattr",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/vfs_removexattr",
		}},
		EventTypes: []eval.EventType{"removexattr"},
	},
	{
		Name: "mnt_want_write",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/mnt_want_write",
		}},
		EventTypes: []eval.EventType{"utimes", "chmod", "chown", "rmdir", "unlink", "rename", "setxattr", "removexattr"},
	},
	{
		Name: "mnt_want_write_file",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/mnt_want_write_file",
		}},
		EventTypes: []eval.EventType{"chown", "setxattr", "removexattr"},
	},
	{
		Name: "mnt_want_write_file_path", // used on old kernels (RHEL 7)
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/mnt_want_write_file_path",
		}},
		EventTypes: []eval.EventType{"chown", "setxattr", "removexattr"},
		Optional:   true,
	},
	{
		Name:       "sys_utime",
		KProbes:    syscallKprobe("utime"),
		EventTypes: []eval.EventType{"utimes"},
	},
	{
		Name:       "sys_utimes",
		KProbes:    syscallKprobe("utimes"),
		EventTypes: []eval.EventType{"utimes"},
	},
	{
		Name:       "sys_utimensat",
		KProbes:    syscallKprobe("utimensat"),
		EventTypes: []eval.EventType{"utimes"},
	},
	{
		Name:       "sys_futimesat",
		KProbes:    syscallKprobe("futimesat"),
		EventTypes: []eval.EventType{"utimes"},
	},
	{
		Name: "vfs_mkdir",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/vfs_mkdir",
		}},
		EventTypes: []eval.EventType{"mkdir"},
	},
	{
		Name: "filename_create",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/filename_create",
		}},
		EventTypes: []eval.EventType{"mkdir", "link"},
	},
	{
		Name:       "sys_mkdir",
		KProbes:    syscallKprobe("mkdir"),
		EventTypes: []eval.EventType{"mkdir"},
	},
	{
		Name:       "sys_mkdirat",
		KProbes:    syscallKprobe("mkdirat"),
		EventTypes: []eval.EventType{"mkdir"},
	},
	{
		Name: "vfs_rmdir",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/vfs_rmdir",
		}},
		EventTypes: []eval.EventType{"rmdir", "unlink"},
	},
	{
		Name:       "sys_rmdir",
		KProbes:    syscallKprobe("rmdir"),
		EventTypes: []eval.EventType{"rmdir"},
	},
	{
		Name: "vfs_rename",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/vfs_rename",
		}},
		EventTypes: []eval.EventType{"rename"},
	},
	{
		Name:       "sys_rename",
		KProbes:    syscallKprobe("rename"),
		EventTypes: []eval.EventType{"rename"},
	},
	{
		Name:       "sys_renameat",
		KProbes:    syscallKprobe("renameat"),
		EventTypes: []eval.EventType{"rename"},
	},
	{
		Name:       "sys_renameat2",
		KProbes:    syscallKprobe("renameat2"),
		EventTypes: []eval.EventType{"rename"},
	},
	{
		Name: "vfs_link",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/vfs_link",
		}},
		EventTypes: []eval.EventType{"link"},
	},
	{
		Name:       "sys_link",
		KProbes:    syscallKprobe("link"),
		EventTypes: []eval.EventType{"link"},
	},
	{
		Name:       "sys_linkat",
		KProbes:    syscallKprobe("linkat"),
		EventTypes: []eval.EventType{"link"},
	},
}

func init() {
	allHookPoints = append(allHookPoints, openHookPoints...)
	allHookPoints = append(allHookPoints, mountHookPoints...)
	allHookPoints = append(allHookPoints, execHookPoints...)
	allHookPoints = append(allHookPoints, UnlinkHookPoints...)
}
