package probe

import (
	"bytes"

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
				Name: "dentry_cache",
			},
		},
		Kprobes: []*types.KProbe{
			&eprobe.KProbe{
				Name:       "vfs_mkdir",
				EntryFunc:  "kprobe/security_inode_mkdir",
				EntryEvent: "vfs_mkdir",
				ExitFunc:   "kretprobe/security_inode_mkdir",
				ExitEvent:  "vfs_mkdir",
			},
		},
		PerfMaps: []*types.PerfMap{
			&types.PerfMap{
				Name:    "dentry_events",
				Handler: p.handleDentryEvent,
			},
		},
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

	p.model = &Model{dentryResolver: dentryResolver}

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

func (p *Probe) AddKernelFilter(event *Event, field string) {
	switch field {
	case "process.name":
		processName := event.Process.GetComm()

		log.Infof("Push in-kernel process discriminator '%s'", processName)
		p.kernelFilters.Push("process_discriminators", CommTableKey(processName))
	}
}
