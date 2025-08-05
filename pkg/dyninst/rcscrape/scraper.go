// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package rcscrape

import (
	"errors"
	"fmt"
	"sync"
	"time"

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
		debouncer debouncer
	}
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
	v.tenant = a.NewTenant(
		"rc-scrape",
		(*scraperReporter)(v),
		irGenerator{},
	)
	return v
}

// ProcessUpdate is a wrapper around an actuator.ProcessUpdate that includes
// the runtime ID of the process.
type ProcessUpdate struct {
	procmon.ProcessUpdate
	RuntimeID         string
	Probes            []ir.ProbeDefinition
	ShouldUploadSymDB bool
}

// GetUpdates returns the current set of updates.
func (s *Scraper) GetUpdates() []ProcessUpdate {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mu.debouncer.coalesceInFlight(time.Now())
}

// AsProcMonHandler returns a procmon.Handler attached to the Scraper.
func (s *Scraper) AsProcMonHandler() procmon.Handler {
	return (*procMonHandler)(s)
}

type procMonHandler Scraper

// HandleProcessesUpdate is called by the actuator to track the current set of
// processes.
func (h *procMonHandler) HandleUpdate(update procmon.ProcessesUpdate) {
	(*Scraper)(h).trackUpdate(update)
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

func (s *scraperSink) HandleEvent(ev output.Event) error {
	now := time.Now()
	d, err := s.decoder.getEventDecoder(ev)
	if err != nil {
		return err
	}
	switch d := d.(type) {
	case *remoteConfigEventDecoder:
		rcFile, err := d.decodeRemoteConfigFile(ev)
		log.Infof("decodeRemoteConfigFile: %+v, %+v", rcFile, err)
		if err != nil {
			return err
		}
		s.scraper.mu.Lock()
		defer s.scraper.mu.Unlock()
		s.scraper.mu.debouncer.addInFlight(now, s.processID, rcFile)
		return nil
	case *symdbEventDecoder:
		runtimeID, symdbEnabled, err := d.decodeSymdbEnabled(ev)
		if err != nil {
			return err
		}
		s.scraper.mu.Lock()
		defer s.scraper.mu.Unlock()
		s.scraper.mu.debouncer.addSymdbEnabled(
			now, s.processID, runtimeID, symdbEnabled,
		)
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

func (s *Scraper) trackUpdate(update procmon.ProcessesUpdate) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, process := range update.Processes {
		s.mu.debouncer.track(process)
	}
	for _, process := range update.Removals {
		s.mu.debouncer.untrack(process)
	}
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
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mu.debouncer.untrack(processID)
}

var _ actuator.Reporter = (*scraperReporter)(nil)
