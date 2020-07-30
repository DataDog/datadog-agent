// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package probe

import (
	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

var unlinkTables = []string{
	"unlink_path_inode_discarders",
}

// UnlinkHookPoints list of unlink's kProbes
var UnlinkHookPoints = []*HookPoint{
	{
		Name: "vfs_unlink",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/vfs_unlink",
		}},
		EventTypes: map[eval.EventType]Capabilities{
			"unlink": {},
		},
		OnNewDiscarders: func(rs *rules.RuleSet, event *Event, probe *Probe, discarder Discarder) error {
			field := discarder.Field

			switch field {
			case "unlink.filename":
				fsEvent := event.Unlink
				table := "unlink_path_inode_discarders"

				isDiscarded, err := discardParentInode(probe, rs, field, discarder.Value.(string), fsEvent.MountID, fsEvent.Inode, table)
				if !isDiscarded || err != nil {
					// not able to discard the parent then only discard the filename
					_, err = discardInode(probe, fsEvent.MountID, fsEvent.Inode, table)
				}

				return err
			}
			return &ErrDiscarderNotSupported{Field: field}
		},
	},
	{
		Name:    "sys_unlink",
		KProbes: syscallKprobe("unlink"),
		EventTypes: map[eval.EventType]Capabilities{
			"unlink": {},
		},
	},
	{
		Name:    "sys_unlinkat",
		KProbes: syscallKprobe("unlinkat"),
		EventTypes: map[eval.EventType]Capabilities{
			"unlink": {},
		},
	},
}
