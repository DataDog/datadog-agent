package probe

import (
	"fmt"
	"os"
	"path"
	"syscall"

	eprobe "github.com/DataDog/datadog-agent/pkg/ebpf/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/pkg/errors"
)

// OpenTables - eBPF tables used by open's kProbes
var OpenTables = []KTable{
	{
		Name: "open_policy",
	},
	{
		Name: "open_basename_approvers",
	},
	{
		Name:    "open_basename_discarders",
		LRUSize: 256,
	},
	{
		Name: "open_flags_approvers",
	},
	{
		Name: "open_flags_discarders",
	},
	{
		Name: "open_process_inode_approvers",
	},
	{
		Name:    "open_process_inode_discarders",
		LRUSize: 256,
	},
}

// OpenKProbes - list of open's kProbes
var OpenKProbes = []*KProbe{
	{
		KProbe: &eprobe.KProbe{
			Name:      "sys_open",
			EntryFunc: "kprobe/" + getSyscallFnName("open"),
			ExitFunc:  "kretprobe/" + getSyscallFnName("open"),
		},
		EventTypes: map[string]Capabilities{
			"open": Capabilities{},
		},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:      "sys_openat",
			EntryFunc: "kprobe/" + getSyscallFnName("openat"),
			ExitFunc:  "kretprobe/" + getSyscallFnName("openat"),
		},
		EventTypes: map[string]Capabilities{
			"open": Capabilities{},
		},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:      "vfs_open",
			EntryFunc: "kprobe/vfs_open",
		},
		EventTypes: map[string]Capabilities{
			"open": Capabilities{
				EvalCapabilities: []eval.FilteringCapability{
					{Field: "open.basename", Types: eval.ScalarValueType},
					{Field: "open.filename", Types: eval.ScalarValueType},
					{Field: "open.flags", Types: eval.ScalarValueType},
					{Field: "process.filename", Types: eval.ScalarValueType},
				},
				PolicyFlags: BASENAME_FLAG | FLAGS_FLAG | PROCESS_INODE,
			},
		},
		PolicyTable: "open_policy",
		OnNewFilter: func(probe *Probe, field string, filters ...eval.Filter) error {
			switch field {
			case "process.filename":
				for _, filter := range filters {
					fileinfo, err := os.Stat(filter.Value.(string))
					if err != nil {
						return err
					}
					stat, _ := fileinfo.Sys().(*syscall.Stat_t)
					key, err := Int64ToKey(int64(stat.Ino))
					if err != nil {
						return errors.New("unable to set policy")
					}

					var kFilter Uint8KFilter

					if filter.Not {
						table := probe.Table("open_process_inode_discarders")
						table.Set(key, kFilter.Bytes())
					} else {
						table := probe.Table("open_process_inode_approvers")
						table.Set(key, kFilter.Bytes())
					}
				}

			case "open.basename":
				for _, filter := range filters {
					handleBasenameFilter(probe, filter.Value.(string), filter.Not)
				}

			case "open.filename":
				for _, filter := range filters {
					if filter.Not {
						return errors.New("open.filename discarder not supported")
					}
				}

				for _, filter := range filters {
					basename := path.Base(filter.Value.(string))
					handleBasenameFilter(probe, basename, filter.Not)
				}
			case "open.flags":
				var kFilter, kNotFilter Uint32KFilter

				for _, filter := range filters {
					if filter.Not {
						kNotFilter.value |= uint32(filter.Value.(int))
					} else {
						kFilter.value |= uint32(filter.Value.(int))
					}
				}

				key, err := Int32ToKey(0)
				if err != nil {
					return errors.New("unable to set policy")
				}

				if kFilter.value != 0 {
					table := probe.Table("open_flags_approvers")
					table.Set(key, kFilter.Bytes())
				}
				if kNotFilter.value != 0 {
					table := probe.Table("open_flags_discarders")
					table.Set(key, kFilter.Bytes())
				}
			default:
				return errors.New("field unknown")
			}

			return nil
		},
	},
}

func handleBasenameFilter(probe *Probe, basename string, not bool) error {
	key, err := StringToKey(basename, BASENAME_FILTER_SIZE)
	if err != nil {
		return fmt.Errorf("unable to generate a key for `%s`: %s", basename, err)
	}

	var kFilter Uint8KFilter

	if not {
		table := probe.Table("open_basename_discarders")
		table.Set(key, kFilter.Bytes())
	} else {
		table := probe.Table("open_basename_approvers")
		table.Set(key, kFilter.Bytes())
	}

	return nil
}
