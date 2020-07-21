package probe

import (
	eprobe "github.com/DataDog/datadog-agent/pkg/ebpf/probe"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

// UnlinkTables - eBPF tables used by unlink's kProbes
var UnlinkTables = []KTable{
	{
		Name: "unlink_path_inode_discarders",
	},
}

// UnlinkHookPoints - list of unlink's kProbes
var UnlinkHookPoints = []*HookPoint{
	{
		Name: "vfs_unlink",
		KProbes: []*eprobe.KProbe{{
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
					discardInode(probe, fsEvent.MountID, fsEvent.Inode, table)
				}

			default:
				return DiscarderNotSupported
			}

			return nil
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
