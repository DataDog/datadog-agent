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
		Name: "open_basename_discarders",
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
		Name: "open_process_inode_discarders",
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
				"open.filename": {
					PolicyFlags:     BASENAME_FLAG,
					FieldValueTypes: eval.ScalarValueType,
				},
				"open.basename": {
					PolicyFlags:     BASENAME_FLAG,
					FieldValueTypes: eval.ScalarValueType,
				},
				"open.flags": {
					PolicyFlags:     FLAGS_FLAG,
					FieldValueTypes: eval.ScalarValueType,
				},
				"process.filename": {
					PolicyFlags:     PROCESS_INODE,
					FieldValueTypes: eval.ScalarValueType,
				},
			},
		},
		PolicyTable: "open_policy",
		OnNewApprovers: func(probe *Probe, approvers eval.Approvers) error {
			stringValues := func(fvs eval.FilterValues) []string {
				var values []string
				for _, v := range fvs {
					values = append(values, v.Value.(string))
				}
				return values
			}

			intValues := func(fvs eval.FilterValues) []int {
				var values []int
				for _, v := range fvs {
					values = append(values, v.Value.(int))
				}
				return values
			}

			for field, values := range approvers {
				switch field {
				case "process.filename":
					return handleProcessFilename(probe, true, stringValues(values)...)

				case "open.basename":
					return handleBasenameFilters(probe, true, stringValues(values)...)

				case "open.filename":
					return handleFilenameFilters(probe, true, stringValues(values)...)

				case "open.flags":
					return handleFlagsFilters(probe, true, intValues(values)...)

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
				return errors.New("open.filename discarders not supported")

			case "open.flags":
				return handleFlagsFilters(probe, false, discarder.Value.(int))

			default:
				return errors.New("field unknown")
			}

			return nil
		},
	},
}

func handleProcessFilename(probe *Probe, approve bool, values ...string) error {
	for _, value := range values {
		fileinfo, err := os.Stat(value)
		if err != nil {
			return err
		}
		stat, _ := fileinfo.Sys().(*syscall.Stat_t)
		key := Int64ToKey(int64(stat.Ino))

		var kFilter Uint8KFilter
		if approve {
			table := probe.Table("open_process_inode_approvers")
			err = table.Set(key, kFilter.Bytes())
		} else {
			table := probe.Table("open_process_inode_discarders")
			err = table.Set(key, kFilter.Bytes())
		}

		if err != nil {
			return err
		}
	}

	return nil
}

func handleFlagsFilters(probe *Probe, approve bool, values ...int) error {
	var kFilter Uint32KFilter

	for _, value := range values {
		kFilter.value |= uint32(value)
	}

	key := Int32ToKey(0)

	var err error
	if kFilter.value != 0 {
		if approve {
			table := probe.Table("open_flags_approvers")
			err = table.Set(key, kFilter.Bytes())
		} else {
			table := probe.Table("open_flags_discarders")
			err = table.Set(key, kFilter.Bytes())
		}
	}

	return err
}

func handleBasenameFilter(probe *Probe, approver bool, basename string) error {
	key, err := StringToKey(basename, BASENAME_FILTER_SIZE)
	if err != nil {
		return fmt.Errorf("unable to generate a key for `%s`: %s", basename, err)
	}

	var kFilter Uint8KFilter

	if approver {
		table := probe.Table("open_basename_approvers")
		err = table.Set(key, kFilter.Bytes())
	} else {
		table := probe.Table("open_basename_discarders")
		err = table.Set(key, kFilter.Bytes())
	}

	return err
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
	for _, value := range values {
		basename := path.Base(value)
		if err := handleBasenameFilter(probe, approve, basename); err != nil {
			return err
		}
	}
	return nil
}
