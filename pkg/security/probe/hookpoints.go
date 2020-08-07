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
	Name            string
	KProbes         []*ebpf.KProbe
	Tracepoint      string
	Optional        bool
	EventTypes      map[eval.EventType]Capabilities
	OnNewApprovers  onApproversFnc
	OnNewDiscarders onDiscarderFnc
	PolicyTable     string
}

var allHookPoints = []*HookPoint{
	{
		Name: "security_inode_setattr",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/security_inode_setattr",
		}},
		EventTypes: map[eval.EventType]Capabilities{
			"chmod":  {},
			"chown":  {},
			"utimes": {},
		},
	},
	{
		Name:    "sys_chmod",
		KProbes: syscallKprobe("chmod"),
		EventTypes: map[eval.EventType]Capabilities{
			"chmod": {},
		},
	},
	{
		Name:    "sys_fchmod",
		KProbes: syscallKprobe("fchmod"),
		EventTypes: map[eval.EventType]Capabilities{
			"chmod": {},
		},
	},
	{
		Name:    "sys_fchmodat",
		KProbes: syscallKprobe("fchmodat"),
		EventTypes: map[eval.EventType]Capabilities{
			"chmod": {},
		},
	},
	{
		Name:    "sys_chown",
		KProbes: syscallKprobe("chown"),
		EventTypes: map[eval.EventType]Capabilities{
			"chown": {},
		},
	},
	{
		Name:    "sys_fchown",
		KProbes: syscallKprobe("fchown"),
		EventTypes: map[eval.EventType]Capabilities{
			"chown": {},
		},
	},
	{
		Name:    "sys_fchownat",
		KProbes: syscallKprobe("fchownat"),
		EventTypes: map[eval.EventType]Capabilities{
			"chown": {},
		},
	},
	{
		Name:    "sys_lchown",
		KProbes: syscallKprobe("lchown"),
		EventTypes: map[eval.EventType]Capabilities{
			"chown": {},
		},
	},
	{
		Name: "mnt_want_write",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/mnt_want_write",
		}},
		EventTypes: map[eval.EventType]Capabilities{
			"utimes": {},
			"chmod":  {},
			"chown":  {},
			"rmdir":  {},
			"unlink": {},
			"rename": {},
		},
	},
	{
		Name: "mnt_want_write_file",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/mnt_want_write_file",
		}},
		EventTypes: map[eval.EventType]Capabilities{
			"chown": {},
		},
	},
	{
		Name:    "sys_utime",
		KProbes: syscallKprobe("utime"),
		EventTypes: map[eval.EventType]Capabilities{
			"utimes": {},
		},
	},
	{
		Name:    "sys_utimes",
		KProbes: syscallKprobe("utimes"),
		EventTypes: map[eval.EventType]Capabilities{
			"utimes": {},
		},
	},
	{
		Name:    "sys_utimensat",
		KProbes: syscallKprobe("utimensat"),
		EventTypes: map[eval.EventType]Capabilities{
			"utimes": {},
		},
	},
	{
		Name:    "sys_futimesat",
		KProbes: syscallKprobe("futimesat"),
		EventTypes: map[string]Capabilities{
			"utimes": {},
		},
	},
	{
		Name: "vfs_mkdir",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/vfs_mkdir",
		}},
		EventTypes: map[eval.EventType]Capabilities{
			"mkdir": {},
		},
	},
	{
		Name: "filename_create",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/filename_create",
		}},
		EventTypes: map[string]Capabilities{
			"mkdir": {},
			"link":  {},
		},
	},
	{
		Name:    "sys_mkdir",
		KProbes: syscallKprobe("mkdir"),
		EventTypes: map[eval.EventType]Capabilities{
			"mkdir": {},
		},
	},
	{
		Name:    "sys_mkdirat",
		KProbes: syscallKprobe("mkdirat"),
		EventTypes: map[eval.EventType]Capabilities{
			"mkdir": {},
		},
	},
	{
		Name: "vfs_rmdir",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/vfs_rmdir",
		}},
		EventTypes: map[string]Capabilities{
			"rmdir":  {},
			"unlink": {},
		},
	},
	{
		Name:    "sys_rmdir",
		KProbes: syscallKprobe("rmdir"),
		EventTypes: map[eval.EventType]Capabilities{
			"rmdir": {},
		},
	},
	{
		Name: "vfs_rename",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/vfs_rename",
		}},
		EventTypes: map[eval.EventType]Capabilities{
			"rename": {},
		},
	},
	{
		Name:    "sys_rename",
		KProbes: syscallKprobe("rename"),
		EventTypes: map[string]Capabilities{
			"rename": {},
		},
	},
	{
		Name:    "sys_renameat",
		KProbes: syscallKprobe("renameat"),
		EventTypes: map[eval.EventType]Capabilities{
			"rename": {},
		},
	},
	{
		Name:    "sys_renameat2",
		KProbes: syscallKprobe("renameat2"),
		EventTypes: map[eval.EventType]Capabilities{
			"rename": {},
		},
	},
	{
		Name: "vfs_link",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/vfs_link",
		}},
		EventTypes: map[string]Capabilities{
			"link": {},
		},
	},
	{
		Name:    "sys_link",
		KProbes: syscallKprobe("link"),
		EventTypes: map[eval.EventType]Capabilities{
			"link": {},
		},
	},
	{
		Name:    "sys_linkat",
		KProbes: syscallKprobe("linkat"),
		EventTypes: map[eval.EventType]Capabilities{
			"link": {},
		},
	},
}

func init() {
	allHookPoints = append(allHookPoints, openHookPoints...)
	allHookPoints = append(allHookPoints, mountHookPoints...)
	allHookPoints = append(allHookPoints, execHookPoints...)
	allHookPoints = append(allHookPoints, UnlinkHookPoints...)
}
