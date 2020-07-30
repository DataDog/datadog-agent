// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package ebpf

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// PerfMapDefinition holds the definition of a perf event array
type PerfMapDefinition struct {
	Name         string
	BufferLength int
	Handler      PerfMapHandler
	LostHandler  PerfMapLostHandler
}

// Probe describes a set composed of an eBPF module, maps and perf event arrays
type Probe struct {
	// Source   string
	// Cflags   []string
	// Bytecode []byte
	Tables   []string
	PerfMaps []*PerfMapDefinition

	tablesMap   map[string]*Table
	perfMapsMap map[string]*PerfMap
	startTime   time.Time
	Module      *Module
}

// Table returns the eBPF map with the specified name
func (p *Probe) Table(name string) *Table {
	return p.tablesMap[name]
}

// StartTime returns the probe starting time
func (p *Probe) StartTime() time.Time {
	return p.startTime
}

func (p *Probe) registerTables() error {
	p.tablesMap = make(map[string]*Table)
	for _, name := range p.Tables {
		t, err := p.Module.RegisterTable(name)
		if err != nil {
			return err
		}
		p.tablesMap[name] = t
		log.Debugf("Registered table %s", name)
	}

	return nil
}

// Stop the eBPF probe and its associated perf event arrays
func (p *Probe) Stop() {
	for _, perfMap := range p.perfMapsMap {
		perfMap.Stop()
	}

	if p.Module != nil {
		p.Module.Close()
	}
}

// Start the eBPF probe and its associated perf event arrays
func (p *Probe) Start() error {
	log.Debugf("Starting perf maps")
	for _, perfMap := range p.perfMapsMap {
		if err := perfMap.Start(); err != nil {
			return err
		}
	}

	p.startTime = time.Now()

	return nil
}

// Load the probe
func (p *Probe) Load() error {
	log.Debugf("Register eBPF tables")
	if err := p.registerTables(); err != nil {
		return err
	}

	log.Debugf("Registering perf maps")
	p.perfMapsMap = make(map[string]*PerfMap, len(p.PerfMaps))
	for _, perfMapDef := range p.PerfMaps {
		perfMap, err := p.Module.RegisterPerfMap(perfMapDef)
		if err != nil {
			return err
		}

		p.perfMapsMap[perfMapDef.Name] = perfMap
	}

	return nil
}
