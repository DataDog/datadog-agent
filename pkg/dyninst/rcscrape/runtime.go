// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package rcscrape

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
	"github.com/DataDog/datadog-agent/pkg/dyninst/uprobe"
)

type scraperRuntime Scraper

// Close implements actuator.Runtime.
func (s *scraperRuntime) Close() error {
	return nil
}

// Load implements actuator.Runtime.
func (s *scraperRuntime) Load(
	programID ir.ProgramID,
	executable actuator.Executable,
	processID actuator.ProcessID,
	probes []ir.ProbeDefinition,
) (_ actuator.LoadedProgram, retErr error) {
	defer func() {
		if retErr != nil {
			((*Scraper)(s)).untrack(processID)
		}
	}()
	ir, err := s.irGenerator.GenerateIR(programID, executable.Path, probes)
	if err != nil {
		return nil, fmt.Errorf("failed to generate IR: %w", err)
	}
	smProgram, err := compiler.GenerateProgram(ir)
	if err != nil {
		return nil, fmt.Errorf("failed to generate program: %w", err)
	}
	lp, err := s.loader.Load(smProgram)
	if err != nil {
		return nil, fmt.Errorf("failed to load program: %w", err)
	}
	defer func() {
		if retErr != nil {
			lp.Close()
		}
	}()
	dec, err := newDecoder(ir)
	if err != nil {
		return nil, fmt.Errorf("failed to create decoder: %w", err)
	}
	sink := &scraperSink{
		programID: programID,
		processID: processID,
		scraper:   (*Scraper)(s),
		decoder:   dec,
	}
	s.dispatcher.RegisterSink(programID, sink)
	return &scraperLoadedProgram{
		ir:         ir,
		executable: executable,
		lp:         lp,
		d:          s.dispatcher,
		s:          (*Scraper)(s),
	}, nil
}

type scraperLoadedProgram struct {
	ir         *ir.Program
	executable actuator.Executable
	lp         *loader.Program
	d          Dispatcher
	s          *Scraper
}

func (s *scraperLoadedProgram) IR() *ir.Program {
	return s.ir
}

func (s *scraperLoadedProgram) Attach(
	processID actuator.ProcessID, executable actuator.Executable,
) (actuator.AttachedProgram, error) {
	v, err := uprobe.Attach(s.lp, executable, processID)
	if err != nil {
		s.s.untrack(processID)
		return nil, err
	}
	return v, nil
}

func (s *scraperLoadedProgram) Close() error {
	s.lp.Close()
	s.d.UnregisterSink(s.ir.ID)
	return nil
}

var _ actuator.Runtime = (*scraperRuntime)(nil)
