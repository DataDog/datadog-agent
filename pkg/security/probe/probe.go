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
	resolvers     *Resolvers
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
				Name: "dentry_event_cache",
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
			&eprobe.KProbe{
				Name:       "vfs_rmdir",
				EntryFunc:  "kprobe/security_inode_rmdir",
				EntryEvent: "vfs_rmdir",
				ExitFunc:   "kretprobe/security_inode_rmdir",
				ExitEvent:  "vfs_rmdir",
			},
			&eprobe.KProbe{
				Name:       "vfs_rename",
				EntryFunc:  "kprobe/security_inode_rename",
				EntryEvent: "vfs_rename",
				ExitFunc:   "kretprobe/security_inode_rename",
				ExitEvent:  "vfs_rename",
			},
			&eprobe.KProbe{
				Name:       "vfs_unlink",
				EntryFunc:  "kprobe/security_inode_unlink",
				EntryEvent: "vfs_unlink",
				ExitFunc:   "kretprobe/security_inode_unlink",
				ExitEvent:  "vfs_unlink",
			},
			&eprobe.KProbe{
				Name:       "vfs_open",
				EntryFunc:  "kprobe/security_file_open",
				EntryEvent: "vfs_open",
				ExitFunc:   "kretprobe/security_file_open",
				ExitEvent:  "vfs_open",
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

func (p *Probe) AddKernelFilter(event *Event, field string) {
	switch field {
	case "process.name":
		processName := event.Process.GetComm()

		log.Infof("Push in-kernel process discriminator '%s'", processName)
		p.kernelFilters.Push("process_discriminators", CommTableKey(processName))
	}
}
