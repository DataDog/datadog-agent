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

// UnlinkHookPoints list of unlink's kProbes
var UnlinkHookPoints = []*HookPoint{
	{
		Name: "vfs_unlink",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/vfs_unlink",
		}},
		EventTypes: []eval.EventType{"unlink"},
	},
	{
		Name:       "sys_unlink",
		KProbes:    syscallKprobe("unlink"),
		EventTypes: []eval.EventType{"unlink"},
	},
	{
		Name:       "sys_unlinkat",
		KProbes:    syscallKprobe("unlinkat"),
		EventTypes: []eval.EventType{"unlink"},
	},
}
