// +build linux_bpf

package probe

import (
	"os"
	"path"
	"syscall"

	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

// openTables is the list of eBPF tables used by open's kProbes
var openTables = []string{
	"open_policy",
	"open_basename_approvers",
	"open_basename_discarders",
	"open_flags_approvers",
	"open_flags_discarders",
	"open_process_inode_approvers",
}

// openHookPoints holds the list of open's kProbes
var openHookPoints = []*HookPoint{
	{
		Name:    "sys_open",
		KProbes: syscallKprobe("open"),
		EventTypes: map[string]Capabilities{
			"open": {},
		},
	},
	{
		Name:    "sys_openat",
		KProbes: syscallKprobe("openat"),
		EventTypes: map[string]Capabilities{
			"open": {},
		},
	},
	{
		Name: "vfs_open",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/vfs_open",
		}},
		EventTypes: map[string]Capabilities{
			"open": {
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
			},
		},
		PolicyTable: "open_policy",
		OnNewApprovers: func(probe *Probe, approvers rules.Approvers) error {
			stringValues := func(fvs rules.FilterValues) []string {
				var values []string
				for _, v := range fvs {
					values = append(values, v.Value.(string))
				}
				return values
			}

			intValues := func(fvs rules.FilterValues) []int {
				var values []int
				for _, v := range fvs {
					values = append(values, v.Value.(int))
				}
				return values
			}

			for field, values := range approvers {
				switch field {
				case "process.filename":
					if err := handleProcessFilename(probe, true, stringValues(values)...); err != nil {
						return err
					}

				case "open.basename":
					if err := handleBasenameFilters(probe, true, stringValues(values)...); err != nil {
						return err
					}

				case "open.filename":
					if err := handleFilenameFilters(probe, true, stringValues(values)...); err != nil {
						return err
					}

				case "open.flags":
					if err := handleFlagsFilters(probe, true, intValues(values)...); err != nil {
						return err
					}

				default:
					return errors.New("field unknown")
				}
			}

			return nil
		},
		OnNewDiscarders: func(probe *Probe, discarder Discarder) error {
			switch discarder.Field {
			case "process.filename":
				return handleProcessFilename(probe, false, discarder.Value.(string))

			case "open.basename":
				return handleBasenameFilters(probe, false, discarder.Value.(string))

			case "open.filename":
				return handleFilenameFilters(probe, false, discarder.Value.(string))

			case "open.flags":
				return handleFlagsFilters(probe, false, discarder.Value.(int))

			default:
				return errors.New("field unknown")
			}
		},
	},
}

func handleProcessFilename(probe *Probe, approve bool, values ...string) error {
	if !approve {
		return errors.New("process.filename discarders not supported")
	}

	for _, value := range values {
		fileinfo, err := os.Stat(value)
		if err != nil {
			return err
		}
		stat, _ := fileinfo.Sys().(*syscall.Stat_t)
		key := ebpf.Uint64TableItem(stat.Ino)

		table := probe.Table("open_process_inode_approvers")
		if err := table.Set(key, ebpf.ZeroUint8TableItem); err != nil {
			return err
		}
	}

	return nil
}

func handleFlagsFilters(probe *Probe, approve bool, values ...int) error {
	var kFilter ebpf.Uint32TableItem

	for _, value := range values {
		kFilter |= ebpf.Uint32TableItem(value)
	}

	var err error
	if kFilter != 0 {
		if approve {
			table := probe.Table("open_flags_approvers")
			err = table.Set(ebpf.ZeroUint32TableItem, kFilter)
		} else {
			table := probe.Table("open_flags_discarders")
			err = table.Set(ebpf.ZeroUint32TableItem, kFilter)
		}
	}

	return err
}

func handleBasenameFilter(probe *Probe, approver bool, basename string) error {
	var table *ebpf.Table
	key := ebpf.NewStringTableItem(basename, BasenameFilterSize)

	if approver {
		table = probe.Table("open_basename_approvers")
	} else {
		table = probe.Table("open_basename_discarders")
	}

	return table.Set(key, ebpf.ZeroUint8TableItem)
}

func handleBasenameFilters(probe *Probe, approve bool, values ...string) error {
	for _, value := range values {
		if err := handleBasenameFilter(probe, approve, value); err != nil {
			return err
		}
	}
	return nil
}

func handleFilenameFilters(probe *Probe, approve bool, values ...string) error {
	if !approve {
		return errors.New("open.filename discarders not supported")
	}

	for _, value := range values {
		// do not use dentry error placeholder as filter
		if value == dentryPathKeyNotFound {
			continue
		}

		basename := path.Base(value)
		if err := handleBasenameFilter(probe, approve, basename); err != nil {
			return err
		}
	}
	return nil
}
