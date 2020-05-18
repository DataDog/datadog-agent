package probe

import (
	"bytes"
	"encoding/binary"
	"path"

	"github.com/iovisor/gobpf/elf"

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
	handler       EventHandler
	kernelFilters *KernelFilters
	resolvers     *Resolvers
}

type KProbe struct {
	*eprobe.KProbe
	EventTypes       map[string][]eval.FilteringCapability
	OnApproversFound func(probe *Probe, field string, approvers []eval.FieldApprover)
	OnDiscarderFound func(probe *Probe, event *Event, field string)
	SetFilterPolicy  func(probe *Probe, deny bool)
}

func getSyscallFnName(name string) string {
	syscall, err := elf.GetSyscallFnName(name)
	if err != nil {
		panic(err)
	}
	return syscall
}

var AllKProbes = []*KProbe{
	{
		KProbe: &eprobe.KProbe{
			Name:      "sys_mkdir",
			EntryFunc: "kprobe/" + getSyscallFnName("mkdir"),
			ExitFunc:  "kretprobe/" + getSyscallFnName("mkdir"),
		},
		EventTypes: map[string][]eval.FilteringCapability{
			"mkdir": []eval.FilteringCapability{},
		},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:      "sys_mkdirat",
			EntryFunc: "kprobe/" + getSyscallFnName("mkdirat"),
			ExitFunc:  "kretprobe/" + getSyscallFnName("mkdirat"),
		},
		EventTypes: map[string][]eval.FilteringCapability{
			"mkdir": []eval.FilteringCapability{},
		},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:      "vfs_mkdir",
			EntryFunc: "kprobe/vfs_mkdir",
		},
		EventTypes: map[string][]eval.FilteringCapability{
			"mkdir": []eval.FilteringCapability{},
		},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:      "sys_rmdir",
			EntryFunc: "kprobe/" + getSyscallFnName("rmdir"),
			ExitFunc:  "kretprobe/" + getSyscallFnName("rmdir"),
		},
		EventTypes: map[string][]eval.FilteringCapability{
			"rmdir": []eval.FilteringCapability{},
		},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:      "vfs_rmdir",
			EntryFunc: "kprobe/vfs_rmdir",
		},
		EventTypes: map[string][]eval.FilteringCapability{
			"rmdir": []eval.FilteringCapability{},
		},
	},
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
		SetFilterPolicy: func(probe *Probe, deny bool) {
			if deny {
				table := probe.Table("open_policy")
				table.Set([]byte{0, 0, 0, 0}, []byte{1})
			}
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
		OnApproversFound: func(probe *Probe, field string, approvers []eval.FieldApprover) {
			switch field {
			case "open.filename":
				for _, approver := range approvers {
					basename := path.Base(approver.Value.(string))

					buffer := new(bytes.Buffer)
					if err := binary.Write(buffer, byteOrder, []byte(basename)); err != nil {
						return
					}
					key := make([]byte, 32)
					copy(key, buffer.Bytes())

					table := probe.Table("open_basename_approvers")
					table.Set(key, []byte{1})
				}
			}
		},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:      "sys_unlink",
			EntryFunc: "kprobe/" + getSyscallFnName("unlink"),
			ExitFunc:  "kretprobe/" + getSyscallFnName("unlink"),
		},
		EventTypes: map[string][]eval.FilteringCapability{
			"unlink": []eval.FilteringCapability{},
		},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:      "sys_unlinkat",
			EntryFunc: "kprobe/" + getSyscallFnName("unlinkat"),
			ExitFunc:  "kretprobe/" + getSyscallFnName("unlinkat"),
		},
		EventTypes: map[string][]eval.FilteringCapability{
			"unlink": []eval.FilteringCapability{},
		},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:      "vfs_unlink",
			EntryFunc: "kprobe/vfs_unlink",
		},
		EventTypes: map[string][]eval.FilteringCapability{
			"unlink": []eval.FilteringCapability{},
		},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:      "sys_rename",
			EntryFunc: "kprobe/" + getSyscallFnName("rename"),
			ExitFunc:  "kretprobe/" + getSyscallFnName("rename"),
		},
		EventTypes: map[string][]eval.FilteringCapability{
			"rename": []eval.FilteringCapability{},
		},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:      "sys_renameat",
			EntryFunc: "kprobe/" + getSyscallFnName("renameat"),
			ExitFunc:  "kretprobe/" + getSyscallFnName("renameat"),
		},
		EventTypes: map[string][]eval.FilteringCapability{
			"rename": []eval.FilteringCapability{},
		},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:      "sys_renameat2",
			EntryFunc: "kprobe/" + getSyscallFnName("renameat2"),
			ExitFunc:  "kretprobe/" + getSyscallFnName("renameat2"),
		},
		EventTypes: map[string][]eval.FilteringCapability{
			"rename": []eval.FilteringCapability{},
		},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:      "vfs_rename",
			EntryFunc: "kprobe/vfs_rename",
		},
		EventTypes: map[string][]eval.FilteringCapability{
			"rename": []eval.FilteringCapability{},
		},
	},
}

func (p *Probe) NewRuleSet(opts eval.Opts) *eval.RuleSet {
	eventCtor := func() eval.Event {
		return NewEvent(p.resolvers)
	}

	return eval.NewRuleSet(&Model{}, eventCtor, opts)
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
			{
				Name: "open_policy",
			},
			{
				Name: "open_basename_approvers",
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

	return p, nil
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

func (p *Probe) Setup(rs *eval.RuleSet) error {
	already := make(map[*KProbe]bool)

	for _, kprobe := range AllKProbes {
		for eventType, capabilities := range kprobe.EventTypes {
			if rs.HasRulesForEventType(eventType) {
				if _, ok := already[kprobe]; !ok {
					if err := p.Module.RegisterKprobe(kprobe.KProbe); err != nil {
						return err
					}
					already[kprobe] = true
				}

				eventApprovers, err := rs.GetEventApprovers(eventType, capabilities...)
				if err != nil {
					if kprobe.SetFilterPolicy != nil {
						log.Infof("Setting in-kernel filter policy to `pass` for `%s`", eventType)
						kprobe.SetFilterPolicy(p, false)
					}
					continue
				}

				if kprobe.SetFilterPolicy != nil {
					log.Infof("Setting in-kernel filter policy to `deny` for `%s`", eventType)
					kprobe.SetFilterPolicy(p, true)
				}

				for field, approvers := range eventApprovers {
					if kprobe.OnApproversFound != nil {
						kprobe.OnApproversFound(p, field, approvers)
					}
				}
			}
		}
	}

	return nil
}
