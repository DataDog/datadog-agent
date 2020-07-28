package probe

import (
	"log"
	"time"

	"github.com/DataDog/datadog-agent/pkg/ebpf/probe/types"
)

type KProbe = types.KProbe

type PerfMap interface {
	Start() error
	Stop()
}

type Table interface {
	Get(key []byte) ([]byte, error)
	Set(key, value []byte)
	Delete(key []byte) error
}

type Module interface {
	RegisterKprobe(k *types.KProbe) error
	RegisterTable(t *types.Table) (Table, error)
	RegisterPerfMap(p *types.PerfMap) (PerfMap, error)
	Close() error
}

type Probe struct {
	// Source   string
	// Cflags   []string
	// Bytecode []byte
	Kprobes  []*types.KProbe
	Tables   []*types.Table
	PerfMaps []*types.PerfMap

	tablesMap   map[string]Table
	perfMapsMap map[string]PerfMap
	startTime   time.Time
	Module      Module
}

func (p *Probe) Table(name string) Table {
	return p.tablesMap[name]
}

func (p *Probe) StartTime() time.Time {
	return p.startTime
}

func (p *Probe) registerHooks() error {
	for _, kProbe := range p.Kprobes {
		if err := p.Module.RegisterKprobe(kProbe); err != nil {
			return err
		}
		log.Printf("Registered Kprobe %s", kProbe)
	}

	return nil
}

func (p *Probe) registerTables() error {
	p.tablesMap = make(map[string]Table)
	for _, table := range p.Tables {
		t, err := p.Module.RegisterTable(table)
		if err != nil {
			return err
		}
		p.tablesMap[table.Name] = t
		log.Printf("Registered table %s", table.Name)
	}

	return nil
}

func (p *Probe) Stop() {
	for _, perfMap := range p.perfMapsMap {
		perfMap.Stop()
	}

	if p.Module != nil {
		p.Module.Close()
	}
}

func (p *Probe) Start() error {
	log.Println("Starting perf maps")
	for _, perfMap := range p.perfMapsMap {
		if err := perfMap.Start(); err != nil {
			return err
		}
	}

	log.Println("Register eBPF hooks")
	if err := p.registerHooks(); err != nil {
		return err
	}

	p.startTime = time.Now()

	return nil
}

func (p *Probe) Load() error {
	log.Println("Register eBPF tables")
	if err := p.registerTables(); err != nil {
		return err
	}

	log.Println("Registering perf maps")
	p.perfMapsMap = make(map[string]PerfMap, len(p.PerfMaps))
	for _, perfMapDef := range p.PerfMaps {
		perfMap, err := p.Module.RegisterPerfMap(perfMapDef)
		if err != nil {
			return err
		}

		p.perfMapsMap[perfMapDef.Name] = perfMap
	}

	return nil
}
