package probe

import (
	"fmt"
	"path"

	eprobe "github.com/DataDog/datadog-agent/pkg/ebpf/probe"
	"github.com/DataDog/datadog-agent/pkg/ebpf/probe/types"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/pkg/errors"
)

var OpenTables = []*types.Table{
	{
		Name: "open_policy",
	},
	{
		Name: "open_basename_approvers",
	},
	{
		Name: "open_basename_discarders",
	},
}

var OpenKProbes = []*KProbe{
	{
		KProbe: &eprobe.KProbe{
			Name:      "sys_open",
			EntryFunc: "kprobe/" + getSyscallFnName("open"),
			ExitFunc:  "kretprobe/" + getSyscallFnName("open"),
		},
		EventTypes: map[string][]eval.FilteringCapability{
			"open": []eval.FilteringCapability{
				{Field: "open.filename", Types: eval.ScalarValueType},
			},
		},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:      "sys_openat",
			EntryFunc: "kprobe/" + getSyscallFnName("openat"),
			ExitFunc:  "kretprobe/" + getSyscallFnName("openat"),
		},
		EventTypes: map[string][]eval.FilteringCapability{
			"open": []eval.FilteringCapability{
				{Field: "open.filename", Types: eval.ScalarValueType},
			},
		},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:      "vfs_open",
			EntryFunc: "kprobe/vfs_open",
		},
		EventTypes: map[string][]eval.FilteringCapability{
			"open": []eval.FilteringCapability{
				{Field: "open.filename", Types: eval.ScalarValueType},
			},
		},
		SetFilterPolicy: func(probe *Probe, mode PolicyMode) error {
			table := probe.Table("open_policy")
			key, err := Int32ToKey(0)
			if err != nil {
				return errors.New("unable to set policy")
			}

			policy := FilterPolicy{
				Mode:  mode,
				Flags: BASENAME_FLAG,
			}

			table.Set(key, policy.Bytes())

			return nil
		},
		OnNewFilter: func(probe *Probe, field string, filters []eval.Filter) error {
			switch field {
			case "open.filename":
				for _, filter := range filters {
					basename := path.Base(filter.Value.(string))
					key, err := StringToKey(basename, BASENAME_FILTER_SIZE)
					if err != nil {
						return fmt.Errorf("unable to generate a key for `%s`: %s", basename, err)
					}

					var kFilter Filter

					if filter.Not {
						table := probe.Table("open_basename_discarders")
						table.Set(key, kFilter.Bytes())
					} else {
						table := probe.Table("open_basename_approvers")
						table.Set(key, kFilter.Bytes())
					}
				}
			default:
				return errors.New("field unknown")
			}

			return nil
		},
	},
}
