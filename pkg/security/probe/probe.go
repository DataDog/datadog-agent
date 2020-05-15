package probe

import (
	"bytes"
	"sort"

	"github.com/DataDog/datadog-agent/pkg/ebpf/gobpf"
	eprobe "github.com/DataDog/datadog-agent/pkg/ebpf/probe"
	"github.com/DataDog/datadog-agent/pkg/ebpf/probe/types"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type EventHandler interface {
	HandleEvent(event *Event)
}

type Probe struct {
	*eprobe.Probe
	model         *Model
	handler       EventHandler
	kernelFilters *KernelFilters
	resolvers     *Resolvers
}

type KProbe struct {
	*eprobe.KProbe
	EventTypes []string
}

var AllKProbes = []*KProbe{
	{
		KProbe: &eprobe.KProbe{
			Name:       "sys_mkdir",
			EntryFunc:  "kprobe/__x64_sys_mkdir",
			EntryEvent: "sys_mkdir",
			ExitFunc:   "kretprobe/__x64_sys_mkdir",
			ExitEvent:  "sys_mkdir",
		},
		EventTypes: []string{"mkdir"},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:       "sys_mkdirat",
			EntryFunc:  "kprobe/__x64_sys_mkdirat",
			EntryEvent: "sys_mkdirat",
			ExitFunc:   "kretprobe/__x64_sys_mkdirat",
			ExitEvent:  "sys_mkdirat",
		},
		EventTypes: []string{"mkdir"},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:       "vfs_mkdir",
			EntryFunc:  "kprobe/vfs_mkdir",
			EntryEvent: "vfs_mkdir",
		},
		EventTypes: []string{"mkdir"},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:       "sys_rmdir",
			EntryFunc:  "kprobe/__x64_sys_rmdir",
			EntryEvent: "sys_rmdir",
			ExitFunc:   "kretprobe/__x64_sys_rmdir",
			ExitEvent:  "sys_rmdir",
		},
		EventTypes: []string{"rmdir"},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:       "vfs_rmdir",
			EntryFunc:  "kprobe/vfs_rmdir",
			EntryEvent: "vfs_rmdir",
		},
		EventTypes: []string{"rmdir"},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:       "sys_openat",
			EntryFunc:  "kprobe/__x64_sys_openat",
			EntryEvent: "sys_openat",
			ExitFunc:   "kretprobe/__x64_sys_openat",
			ExitEvent:  "sys_openat",
		},
		EventTypes: []string{"open"},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:       "vfs_open",
			EntryFunc:  "kprobe/vfs_open",
			EntryEvent: "vfs_open",
		},
		EventTypes: []string{"open"},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:       "sys_unlink",
			EntryFunc:  "kprobe/__x64_sys_unlink",
			EntryEvent: "sys_unlink",
			ExitFunc:   "kretprobe/__x64_sys_unlink",
			ExitEvent:  "sys_unlink",
		},
		EventTypes: []string{"unlink"},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:       "sys_unlinkat",
			EntryFunc:  "kprobe/__x64_sys_unlinkat",
			EntryEvent: "sys_unlinkat",
			ExitFunc:   "kretprobe/__x64_sys_unlinkat",
			ExitEvent:  "sys_unlinkat",
		},
		EventTypes: []string{"unlink"},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:       "vfs_unlink",
			EntryFunc:  "kprobe/vfs_unlink",
			EntryEvent: "vfs_unlink",
		},
		EventTypes: []string{"unlink"},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:       "sys_rename",
			EntryFunc:  "kprobe/__x64_sys_rename",
			EntryEvent: "sys_rename",
			ExitFunc:   "kretprobe/__x64_sys_rename",
			ExitEvent:  "sys_rename",
		},
		EventTypes: []string{"rename"},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:       "sys_renameat",
			EntryFunc:  "kprobe/__x64_sys_renameat",
			EntryEvent: "sys_renameat",
			ExitFunc:   "kretprobe/__x64_sys_renameat",
			ExitEvent:  "sys_renameat",
		},
		EventTypes: []string{"rename"},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:       "sys_renameat2",
			EntryFunc:  "kprobe/__x64_sys_renameat2",
			EntryEvent: "sys_renameat2",
			ExitFunc:   "kretprobe/__x64_sys_renameat2",
			ExitEvent:  "sys_renameat2",
		},
		EventTypes: []string{"rename"},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:       "vfs_rename",
			EntryFunc:  "kprobe/vfs_rename",
			EntryEvent: "vfs_rename",
		},
		EventTypes: []string{"rename"},
	},
}

func NewProbe(config *config.Config) (*Probe, error) {
	bytecode, err := Asset("probe.o") // ioutil.ReadFile("pkg/security/ebpf/probe.o")
	if err != nil {
		return nil, err
	}

	module, err := gobpf.NewModuleFromReader(bytes.NewReader(bytecode))
	if err != nil {
		return nil, err
	}
	log.Infof("Loaded security agent eBPF module: %+v", module)

	p := &Probe{}

	ebpfProbe := &eprobe.Probe{
		Module: module,
		Tables: []*types.Table{
			{
				Name: "pathnames",
			},
			{
				Name: "process_discriminators",
			},
		},
		PerfMaps: []*types.PerfMap{
			{
				Name:    "events",
				Handler: p.handleEvent,
			},
		},
	}

	for _, kprobe := range AllKProbes {
		ebpfProbe.Kprobes = append(ebpfProbe.Kprobes, kprobe.KProbe)
	}

	if err := ebpfProbe.Load(); err != nil {
		return nil, err
	}
	p.Probe = ebpfProbe

	p.kernelFilters, err = NewKernelFilters(config.MaxKernelFilters, []string{
		"process_discriminators",
	}, ebpfProbe)
	if err != nil {
		return nil, err
	}

	dentryResolver, err := NewDentryResolver(ebpfProbe)
	if err != nil {
		return nil, err
	}

	p.resolvers = &Resolvers{
		DentryResolver: dentryResolver,
	}

	p.model = &Model{}

	return p, nil
}

func (p *Probe) GetModel() eval.Model {
	return p.model
}

func (p *Probe) SetEventHandler(handler EventHandler) {
	p.handler = handler
}

func (p *Probe) DispatchEvent(event *Event) {
	if p.handler != nil {
		p.handler.HandleEvent(event)
	}
}

func (p *Probe) handleEvent(data []byte) {
	log.Debugf("Handling dentry event (len %d)", len(data))

	offset := 0
	event := NewEvent(p.resolvers)

	read, err := event.Event.UnmarshalBinary(data)
	if err != nil {
		log.Errorf("failed to decode event")
		return
	}
	offset += read

	read, err = event.Process.UnmarshalBinary(data[offset:])
	if err != nil {
		log.Errorf("failed to decode process event")
		return
	}
	offset += read

	switch ProbeEventType(event.Event.Type) {
	case FileOpenEventType:
		if _, err := event.Open.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode open event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
	case FileMkdirEventType:
		if _, err := event.Mkdir.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode mkdir event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
	case FileRmdirEventType:
		if _, err := event.Rmdir.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode rmdir event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
	case FileUnlinkEventType:
		if _, err := event.Unlink.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode unlink event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
	case FileRenameEventType:
		if _, err := event.Rename.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode rename event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
	default:
		log.Errorf("Unsupported event type %d\n", event.Event.Type)
	}

	log.Debugf("Dispatching event %+v\n", event)
	p.DispatchEvent(event)
}

func (p *Probe) AddKernelFilter(event *Event, field string) {
	switch field {
	case "process.name":
		processName := event.Process.GetComm()

		log.Infof("Push in-kernel process discriminator '%s'", processName)
		p.kernelFilters.Push("process_discriminators", CommTableKey(processName))
	}
}

func (p *Probe) SetEventTypes(eventTypes []string) error {
	sort.Strings(eventTypes)

	for _, kprobe := range AllKProbes {
		enable := false
		for _, eventType := range kprobe.EventTypes {
			index := sort.SearchStrings(eventTypes, eventType)
			if index < len(eventTypes) && eventTypes[index] == eventType {
				enable = true
				break
			}
		}

		var err error
		if enable {
			err = p.Module.RegisterKprobe(kprobe.KProbe)
		} else {
			err = p.Module.UnregisterKprobe(kprobe.KProbe)
		}

		if err != nil {
			return err
		}
	}

	return nil
}
