package probe

import (
	"log"
	"time"
)

type Hook interface {
	Register(m *Module) error
}

type Probe struct {
	Source    string
	Cflags    []string
	Hooks     []Hook
	Tables    map[string]*Table
	PerfMaps  []*PerfMap
	StartTime time.Time

	module *Module
}

func (p *Probe) registerHooks() error {
	for _, hook := range p.Hooks {
		if err := hook.Register(p.module); err != nil {
			return err
		}
	}

	return nil
}

func (p *Probe) registerTables() error {
	for name, table := range p.Tables {
		table.Name = name
		if err := table.Register(p.module); err != nil {
			return err
		}
	}

	return nil
}

func (p *Probe) Stop() {
	for _, perfMap := range p.PerfMaps {
		perfMap.Stop()
	}

	if p.module != nil {
		p.module.Close()
	}
}

func (p *Probe) Start() (err error) {
	p.module, err = NewModuleFromSource(p.Source, p.Cflags)
	if err != nil {
		return err
	}

	log.Println("Register eBPF tables")
	if err := p.registerTables(); err != nil {
		return err
	}

	log.Println("Starting perf maps")
	for _, perfMap := range p.PerfMaps {
		if err := perfMap.Start(p.module); err != nil {
			return err
		}
	}

	log.Println("Register eBPF hooks")
	if err := p.registerHooks(); err != nil {
		return err
	}

	p.StartTime = time.Now()

	return nil
}
