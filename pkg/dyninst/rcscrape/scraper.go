// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package rcscrape

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procmon"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Scraper is a component that scrapes remote config files from processes.
// It coalesces updates and reports them via the GetUpdates method.
type Scraper struct {
	tenant ActuatorTenant
	mu     struct {
		sync.Mutex

		sinks     map[ir.ProgramID]*scraperSink
		processes map[actuator.ProcessID]*trackedProcess
		debouncer debouncer
	}
}

type trackedProcess struct {
	pu procmon.ProcessUpdate
	// The runtimeID reported by the tracer inside this process.
	runtimeID string
}

// defaultIdlePeriod is the default idle period for the debouncer. This value
// is somewhat arbitrary. Hopefully it is long enough that a process won't
// pause for this long and thus we'd see an incomplete update, but also short
// enough that we don't wait too long for an update.
const defaultIdlePeriod = 250 * time.Millisecond

// Actuator is an interface that enables the Scraper to create a new tenant.
type Actuator[T ActuatorTenant] interface {
	NewTenant(
		name string,
		reporter actuator.Reporter,
		irGenerator actuator.IRGenerator,
	) T
}

// ActuatorTenant is an interface that enables the Scraper to handle updates
type ActuatorTenant interface {
	HandleUpdate(update actuator.ProcessesUpdate)
}

// NewScraper creates a new Scraper.
func NewScraper[A Actuator[T], T ActuatorTenant](
	a A,
) *Scraper {
	v := &Scraper{}
	v.mu.sinks = make(map[ir.ProgramID]*scraperSink)
	v.mu.debouncer = makeDebouncer(defaultIdlePeriod)
	v.mu.processes = make(map[actuator.ProcessID]*trackedProcess)
	v.tenant = a.NewTenant(
		"rc-scrape",
		(*scraperReporter)(v),
		irGenerator{},
	)
	return v
}

// ProcessUpdate represents the current state of a process, which may have
// changed since the last time the Scraper returned information about that
// process. An update doesn't tell you exactly what changed, and ProcessUpdates
// can be produced even when nothing at all changed.
type ProcessUpdate struct {
	procmon.ProcessUpdate
	RuntimeID         string
	Probes            []ir.ProbeDefinition
	ShouldUploadSymDB bool
}

// GetUpdates returns the current state of processes that have pending updates
// (i.e. updates not previously returned by GetUpdates).
func (s *Scraper) GetUpdates() []ProcessUpdate {
	s.mu.Lock()
	defer s.mu.Unlock()
	updates := s.mu.debouncer.getUpdates(time.Now())
	res := make([]ProcessUpdate, 0, len(updates))
	for _, u := range updates {
		p, ok := s.mu.processes[u.procID]
		if !ok {
			log.Warnf("bug: debouncer update for untracked process: %v", u.procID)
			continue
		}
		res = append(res, ProcessUpdate{
			ProcessUpdate:     p.pu,
			RuntimeID:         p.runtimeID,
			Probes:            u.probes,
			ShouldUploadSymDB: u.symdbEnabled,
		})
	}

	slices.SortFunc(res, func(a, b ProcessUpdate) int {
		return cmp.Compare(a.ProcessID.PID, b.ProcessID.PID)
	})
	return res
}

// AsProcMonHandler returns a procmon.Handler attached to the Scraper.
func (s *Scraper) AsProcMonHandler() procmon.Handler {
	return (*procMonHandler)(s)
}

// AddUpdate accumulates a remote config update for a process, to be returned
// later by GetUpdates.
func (s *Scraper) AddUpdate(
	now time.Time,
	processID actuator.ProcessID,
	file remoteConfigFile,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.mu.processes[processID]
	if !ok {
		// Update corresponds to an untracked process. This can happen because
		// notifications about process removal can race with reading updates
		// from the BPF ringbuf.
		return
	}
	s.checkRuntimeIDLocked(p, file.RuntimeID)

	s.mu.debouncer.addUpdate(now, processID, file)

	if log.ShouldLog(log.TraceLvl) {
		log.Tracef(
			"rcscrape: process %v: got update for %s",
			p.pu.ProcessID, file.ConfigPath,
		)
	}
}

// AddSymdbEnabled accumulates a SymDB enablement update for a process, to be
// returned later by GetUpdates.
func (s *Scraper) AddSymdbEnabled(
	now time.Time,
	processID actuator.ProcessID,
	runtimeID string,
	symdbEnabled bool,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.mu.processes[processID]
	if !ok {
		// Update corresponds to an untracked process.
		return
	}
	s.checkRuntimeIDLocked(p, runtimeID)

	s.mu.debouncer.addSymdbEnabled(now, processID, symdbEnabled)

	if log.ShouldLog(log.TraceLvl) {
		log.Tracef(
			"rcscrape: process %v: SymDB enabled: %t",
			p.pu.ProcessID, symdbEnabled,
		)
	}
}

func (s *Scraper) checkRuntimeIDLocked(p *trackedProcess, runtimeID string) {
	if p.runtimeID != "" && p.runtimeID != runtimeID {
		log.Warnf(
			"rcscrape: process %v: runtime ID mismatch: %s != %s",
			p.pu.ProcessID, p.runtimeID, runtimeID,
		)
		s.mu.debouncer.clear(p.pu.ProcessID)
	}
	p.runtimeID = runtimeID
}

type procMonHandler Scraper

// HandleUpdate is called by the actuator to track the current set of processes.
func (h *procMonHandler) HandleUpdate(update procmon.ProcessesUpdate) {
	(*Scraper)(h).handleProcmonUpdates(update)
	updates := make([]actuator.ProcessUpdate, 0, len(update.Processes))
	for i := range update.Processes {
		process := &update.Processes[i]
		updates = append(updates, actuator.ProcessUpdate{
			ProcessID:  process.ProcessID,
			Executable: process.Executable,
			Probes: []ir.ProbeDefinition{
				probeDefinitionV1{},
				probeDefinitionV2{},
				symdbProbeDefinition{},
			},
		})
	}
	h.tenant.HandleUpdate(actuator.ProcessesUpdate{
		Processes: updates,
		Removals:  update.Removals,
	})
}

type scraperSink struct {
	// This is baking in on a deep level that the program is 1:1 with the
	// process ID.
	programID  ir.ProgramID
	processID  actuator.ProcessID
	executable actuator.Executable
	scraper    *Scraper
	decoder    *decoder
}

var sinkErrLogLimiter = rate.NewLimiter(rate.Every(10*time.Minute), 10)

func (s *scraperSink) HandleEvent(ev output.Event) (retErr error) {
	// We don't want to fail out the actuator if we can't decode an event.
	// Instead, we log the error and continue, but we'll make sure to clear
	// the debouncer state for the process.
	defer func() {
		if retErr == nil {
			return
		}
		if sinkErrLogLimiter.Allow() {
			log.Errorf("rcscrape: error in HandleEvent: %v", retErr)
		} else {
			log.Debugf("rcscrape: error in HandleEvent: %v", retErr)
		}
		retErr = nil
		s.scraper.mu.Lock()
		defer s.scraper.mu.Unlock()
		s.scraper.mu.debouncer.clear(s.processID)
	}()
	now := time.Now()
	d, err := s.decoder.getEventDecoder(ev)
	if err != nil {
		return err
	}
	switch d := d.(type) {
	case *remoteConfigEventDecoder:
		rcFile, err := d.decodeRemoteConfigFile(ev)
		if err != nil {
			return err
		}
		s.scraper.AddUpdate(now, s.processID, rcFile)
		return nil
	case *symdbEventDecoder:
		runtimeID, symdbEnabled, err := d.decodeSymdbEnabled(ev)
		if err != nil {
			return err
		}
		s.scraper.AddSymdbEnabled(now, s.processID, runtimeID, symdbEnabled)
		return nil
	default:
		return fmt.Errorf("unknown event decoder: %T", d)
	}
}

func (s *scraperSink) Close() {
	s.scraper.mu.Lock()
	defer s.scraper.mu.Unlock()
	delete(s.scraper.mu.sinks, s.programID)
}

func (s *Scraper) handleProcmonUpdates(update procmon.ProcessesUpdate) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, proc := range update.Processes {
		s.mu.processes[proc.ProcessID] = &trackedProcess{
			pu:        proc,
			runtimeID: "",
		}
	}
	for _, pid := range update.Removals {
		s.untrackLocked(pid)
	}
}

func (s *Scraper) untrack(pid actuator.ProcessID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.untrackLocked(pid)
}

func (s *Scraper) untrackLocked(pid actuator.ProcessID) {
	s.mu.debouncer.clear(pid)
	delete(s.mu.processes, pid)
}

type scraperReporter Scraper

// ReportAttached implements actuator.Reporter.
func (s *scraperReporter) ReportAttached(
	procID actuator.ProcessID,
	_ *ir.Program,
) {
	log.Debugf("rcscrape: attached to process %v", procID)
}

// ReportAttachingFailed implements actuator.Reporter.
func (s *scraperReporter) ReportAttachingFailed(
	procID actuator.ProcessID,
	_ *ir.Program,
	err error,
) {
	log.Infof("rcscrape: failed to attach probes to process %v: %v", procID, err)
	s.untrack(procID)
}

// ReportDetached implements actuator.Reporter.
func (s *scraperReporter) ReportDetached(
	procID actuator.ProcessID,
	_ *ir.Program,
) {
	log.Tracef("rcscrape: detached from process %v", procID)
	s.untrack(procID)
}

var noSuccessProbesError = &actuator.NoSuccessfulProbesError{}

// ReportIRGenFailed implements actuator.Reporter.
func (s *scraperReporter) ReportIRGenFailed(
	processID actuator.ProcessID,
	err error,
	_ []ir.ProbeDefinition,
) {
	if errors.Is(err, noSuccessProbesError) {
		log.Tracef(
			"rcscrape: process %v has no successful probes, skipping", processID,
		)
	} else {
		log.Errorf("rcscrape: failed to generate IR for process %v: %v", processID, err)
	}
	s.untrack(processID)
}

// ReportLoaded implements actuator.Reporter.
func (s *scraperReporter) ReportLoaded(
	processID actuator.ProcessID,
	executable actuator.Executable,
	program *ir.Program,
) (actuator.Sink, error) {
	decoder, err := newDecoder(program)
	if err != nil {
		return nil, err
	}
	sd := &scraperSink{
		programID:  program.ID,
		scraper:    (*Scraper)(s),
		decoder:    decoder,
		processID:  processID,
		executable: executable,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mu.sinks[program.ID] = sd
	return sd, nil
}

// ReportLoadingFailed implements actuator.Reporter.
func (s *scraperReporter) ReportLoadingFailed(
	processID actuator.ProcessID,
	_ *ir.Program,
	err error,
) {
	log.Errorf("rcscrape: failed to load program for process %v: %v", processID, err)
	s.untrack(processID)
}

func (s *scraperReporter) untrack(processID actuator.ProcessID) {
	((*Scraper)(s)).untrack(processID)
}

var _ actuator.Reporter = (*scraperReporter)(nil)
