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

var openCapabilities = Capabilities{
	"open.filename": {
		PolicyFlags:     PolicyFlagBasename,
		FieldValueTypes: eval.ScalarValueType,
	},
	"open.basename": {
		PolicyFlags:     PolicyFlagBasename,
		FieldValueTypes: eval.ScalarValueType,
	},
	"open.flags": {
		PolicyFlags:     PolicyFlagFlags,
		FieldValueTypes: eval.ScalarValueType | eval.BitmaskValueType,
	},
	"process.filename": {
		PolicyFlags:     PolicyFlagProcessInode,
		FieldValueTypes: eval.ScalarValueType,
	},
}

// openHookPoints holds the list of open's kProbes
var openHookPoints = []*HookPoint{
	{
		Name:       "sys_open",
		KProbes:    syscallKprobe("open"),
		EventTypes: []eval.EventType{"open"},
	},
	{
		Name:       "sys_creat",
		KProbes:    syscallKprobe("creat"),
		EventTypes: []eval.EventType{"open"},
	},
	{
		Name:       "sys_open_by_handle_at",
		KProbes:    syscallKprobe("open_by_handle_at"),
		EventTypes: []eval.EventType{"open"},
	},
	{
		Name:       "sys_truncate",
		KProbes:    syscallKprobe("truncate"),
		EventTypes: []eval.EventType{"open"},
	},
	{
		Name:       "sys_openat",
		KProbes:    syscallKprobe("openat"),
		EventTypes: []eval.EventType{"open"},
	},
	{
		Name: "vfs_open",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/vfs_open",
		}},
		EventTypes: []eval.EventType{"open"},
	},
	{
		Name: "vfs_truncate",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/vfs_truncate",
		}},
		EventTypes: []eval.EventType{"open"},
	},
}
