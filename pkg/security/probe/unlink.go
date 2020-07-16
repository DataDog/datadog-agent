package probe

import (
	"fmt"

	eprobe "github.com/DataDog/datadog-agent/pkg/ebpf/probe"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/pkg/errors"
)

// UnlinkTables - eBPF tables used by unlink's kProbes
var UnlinkTables = []KTable{
	{
		Name: "unlink_prefix_discarders",
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
		OnNewDiscarders: func(rs *rules.RuleSet, probe *Probe, discarder Discarder) error {
			switch discarder.Field {
			case "unlink.filename":
				prefix := discarder.Value.(string)
				if len(prefix) > 32 {
					prefix = prefix[0:32]
				}

				var event Event
				event.Event.Type = uint64(FileUnlinkEventType)

				if rs.IsDiscarder(&event, "unlink.filename", prefix) {
					fmt.Printf("->>>>>>>>>>>>>>>>>>>>> DISCARD DANS TA FACE: %s\n", prefix)

					key, err := StringToKey(prefix, UNLINK_PREFIX_FILTER_SIZE)
					if err != nil {
						return fmt.Errorf("unable to generate a key for `%s`: %s", prefix, err)
					}

					table := probe.Table("unlink_prefix_discarders")
					if err = table.Set(key, zeroInt8); err != nil {
						return err
					}
				}

			default:
				return errors.New("field unknown")
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
