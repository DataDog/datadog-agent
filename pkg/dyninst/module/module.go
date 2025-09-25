// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/dispatcher"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procmon"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcscrape"
	"github.com/DataDog/datadog-agent/pkg/dyninst/uploader"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Module is the dynamic instrumentation system probe module.
type Module struct {
	tenant    ActuatorTenant
	symdb     symdbManagerInterface
	rcScraper Scraper

	store       *processStore
	diagnostics *diagnosticsManager

	testingKnobs testingKnobs

	shutdown struct {
		sync.Once
		realDependencies realDependencies
		unsubscribeExec  context.CancelFunc
		unsubscribeExit  context.CancelFunc
		cancelTasks      context.CancelFunc
		processMonitor   *procmon.ProcessMonitor
	}
}

// NewModule creates a new dynamic instrumentation module.
func NewModule(
	config *Config, subscriber ProcessSubscriber,
) (_ *Module, err error) {
	realDeps, err := makeRealDependencies(config)
	if err != nil {
		return nil, err
	}
	m := newUnstartedModule(realDeps.asDependencies())
	m.shutdown.realDependencies = realDeps
	procMon := procmon.NewProcessMonitor(&processHandler{
		module:         m,
		scraperHandler: realDeps.scraper.AsProcMonHandler(),
	})
	m.shutdown.processMonitor = procMon
	m.shutdown.unsubscribeExec = subscriber.SubscribeExec(procMon.NotifyExec)
	m.shutdown.unsubscribeExit = subscriber.SubscribeExit(procMon.NotifyExit)

	ctx, cancel := context.WithCancel(context.Background())
	m.shutdown.cancelTasks = cancel
	if !config.ProcessSyncDisabled {
		go m.runProcessSync(ctx, procMon)
	}
	const defaultInterval = 200 * time.Millisecond
	go m.run(ctx, defaultInterval)
	return m, nil
}

func newUnstartedModule(deps dependencies) *Module {
	// A zero-value symdbManager is valid and disabled.
	if deps.symdbManager == nil {
		deps.symdbManager = &symdbManager{}
	}
	store := newProcessStore()
	logsUploader := logsUploaderFactoryImpl[LogsUploader]{factory: deps.LogsFactory}
	diagnostics := newDiagnosticsManager(deps.DiagnosticsUploader)
	runtime := &runtimeImpl{
		store:                    store,
		diagnostics:              diagnostics,
		decoderFactory:           deps.DecoderFactory,
		irGenerator:              deps.IRGenerator,
		programCompiler:          deps.ProgramCompiler,
		kernelLoader:             deps.KernelLoader,
		attacher:                 deps.Attacher,
		dispatcher:               deps.Dispatcher,
		logsFactory:              logsUploader,
		procRuntimeIDbyProgramID: &sync.Map{},
	}
	tenant := deps.Actuator.NewTenant("dyninst", runtime)

	m := &Module{
		rcScraper:    deps.Scraper,
		store:        store,
		diagnostics:  diagnostics,
		symdb:        deps.symdbManager,
		tenant:       tenant,
		testingKnobs: testingKnobs{},
	}
	return m
}

type realDependencies struct {
	logUploader     *uploader.LogsUploaderFactory
	diagsUploader   *uploader.DiagnosticsUploader
	actuator        *actuator.Actuator
	dispatcher      *dispatcher.Dispatcher
	loader          *loader.Loader
	attacher        *defaultAttacher
	scraper         *rcscrape.Scraper
	symdbManager    *symdbManager
	decoderFactory  decoderFactory
	programCompiler *stackMachineCompiler

	objectLoader object.Loader
	irGenerator  IRGenerator
}

func (c *realDependencies) asDependencies() dependencies {
	return dependencies{
		Actuator:            &erasedActuator[*actuator.Actuator, *actuator.Tenant]{a: c.actuator},
		Scraper:             c.scraper,
		Dispatcher:          c.dispatcher,
		DecoderFactory:      c.decoderFactory,
		IRGenerator:         c.irGenerator,
		ProgramCompiler:     c.programCompiler,
		KernelLoader:        c.loader,
		Attacher:            c.attacher,
		LogsFactory:         logsUploaderFactoryImpl[*uploader.LogsUploader]{factory: c.logUploader},
		DiagnosticsUploader: c.diagsUploader,
		ObjectLoader:        c.objectLoader,
		symdbManager:        c.symdbManager,
	}
}

func (c *realDependencies) shutdown() {
	if c.logUploader != nil {
		c.logUploader.Stop()
	}
	if c.diagsUploader != nil {
		c.diagsUploader.Stop()
	}
	if c.actuator != nil {
		if err := c.actuator.Shutdown(); err != nil {
			log.Warnf("error shutting down actuator: %v", err)
		}
	}
	if c.dispatcher != nil {
		if err := c.dispatcher.Shutdown(); err != nil {
			log.Warnf("error shutting down dispatcher: %v", err)
		}
	}
	if c.loader != nil {
		c.loader.Close()
	}
	if c.symdbManager != nil {
		c.symdbManager.stop()
	}
}

func makeRealDependencies(config *Config) (_ realDependencies, retErr error) {
	var ret realDependencies
	defer func() {
		if retErr != nil {
			ret.shutdown()
		}
	}()

	logUploaderURL, err := url.Parse(config.LogUploaderURL)
	if err != nil {
		return ret, fmt.Errorf("error parsing log uploader URL: %w", err)
	}
	ret.logUploader = uploader.NewLogsUploaderFactory(uploader.WithURL(logUploaderURL))

	diagsUploaderURL, err := url.Parse(config.DiagsUploaderURL)
	if err != nil {
		return ret, fmt.Errorf("error parsing diagnostics uploader URL: %w", err)
	}
	diagsUploader := uploader.NewDiagnosticsUploader(uploader.WithURL(diagsUploaderURL))
	ret.diagsUploader = diagsUploader

	var symdbUploaderURL *url.URL
	if config.SymDBUploadEnabled {
		symdbUploaderURL, err = url.Parse(config.SymDBUploaderURL)
		if err != nil {
			return ret, fmt.Errorf("error parsing SymDB uploader URL: %w", err)
		}
	}
	ret.actuator = actuator.NewActuator()

	ret.loader, err = loader.NewLoader()
	if err != nil {
		return ret, fmt.Errorf("error creating loader: %w", err)
	}
	var irgenOptions []irgen.Option
	if config.DiskCacheEnabled {
		diskCache, err := object.NewDiskCache(config.DiskCacheConfig)
		if err != nil {
			return ret, fmt.Errorf("error creating disk cache: %w", err)
		}
		ret.objectLoader = diskCache
		irgenOptions = append(irgenOptions,
			irgen.WithOnDiskGoTypeIndexFactory(diskCache),
			irgen.WithObjectLoader(diskCache),
		)

	} else {
		ret.objectLoader = object.NewInMemoryLoader()
		irgenOptions = append(irgenOptions, irgen.WithObjectLoader(ret.objectLoader))
	}
	ret.irGenerator = irgen.NewGenerator(irgenOptions...)
	var ts unix.Timespec
	if err = unix.ClockGettime(unix.CLOCK_MONOTONIC, &ts); err != nil {
		return ret, fmt.Errorf("error getting monotonic time: %w", err)
	}
	ret.dispatcher = dispatcher.NewDispatcher(ret.loader.OutputReader())
	ret.scraper = rcscrape.NewScraper(ret.actuator, ret.dispatcher, ret.loader)

	approximateBootTime := time.Now().Add(time.Duration(-ts.Nano()))
	ret.decoderFactory = decoderFactory{approximateBootTime: approximateBootTime}
	ret.symdbManager = newSymdbManager(symdbUploaderURL, ret.objectLoader)
	ret.attacher = &defaultAttacher{}
	ret.programCompiler = &stackMachineCompiler{}
	return ret, nil
}

func (m *Module) runProcessSync(ctx context.Context, subscriber *procmon.ProcessMonitor) {
	const syncInterval = 30 * time.Second
	timer := time.NewTimer(0) // sync immediately on startup
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
		case <-ctx.Done():
			return
		}
		if err := subscriber.Sync(); err != nil {
			log.Errorf("error syncing process monitor: %v", err)
		}
		timer.Reset(jitter(syncInterval, 0.2))
	}
}

func (m *Module) run(ctx context.Context, interval time.Duration) {
	duration := func() time.Duration { return jitter(interval, 0.2) }
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			m.checkForUpdates()
			timer.Reset(duration())
		case <-ctx.Done():
			return
		}
	}
}

// GetStats returns the stats of the module
func (m *Module) GetStats() map[string]any {
	// m.controller.mu.Lock()
	// defer m.controller.mu.Unlock()

	return map[string]any{}
}

// Register registers the module to the router
func (m *Module) Register(router *module.Router) error {
	router.HandleFunc(
		"/check",
		utils.WithConcurrencyLimit(
			utils.DefaultMaxConcurrentRequests,
			func(w http.ResponseWriter, _ *http.Request) {
				utils.WriteAsJSON(
					w, json.RawMessage(`{"status":"ok"}`), utils.CompactOutput,
				)
			},
		),
	)
	return nil
}

// Close closes the module
func (m *Module) Close() {
	m.shutdown.Once.Do(func() {
		log.Debugf("closing dynamic instrumentation module")
		if m.shutdown.unsubscribeExec != nil {
			m.shutdown.unsubscribeExec()
		}
		if m.shutdown.unsubscribeExit != nil {
			m.shutdown.unsubscribeExit()
		}
		if m.shutdown.cancelTasks != nil {
			m.shutdown.cancelTasks()
		}
		if m.shutdown.processMonitor != nil {
			m.shutdown.processMonitor.Close()
		}
		m.shutdown.realDependencies.shutdown()
	})
}

func (m *Module) handleRemovals(removals []procmon.ProcessID) {
	m.store.remove(removals, m.diagnostics)
	if len(removals) > 0 && m.tenant != nil {
		m.tenant.HandleUpdate(actuator.ProcessesUpdate{Removals: removals})
	}
	for _, pid := range removals {
		m.symdb.removeUploadByPID(pid)
	}
}

func (m *Module) checkForUpdates() {
	updates := m.rcScraper.GetUpdates()
	if m.testingKnobs.scraperUpdatesCallback != nil && len(updates) > 0 {
		m.testingKnobs.scraperUpdatesCallback(updates)
	}
	if len(updates) == 0 {
		return
	}
	actuatorUpdates := make([]actuator.ProcessUpdate, 0, len(updates))
	for i := range updates {
		update := &updates[i]
		runtimeID := m.store.ensureExists(update)
		actuatorUpdates = append(actuatorUpdates, actuator.ProcessUpdate{
			ProcessID:  update.ProcessID,
			Executable: update.Executable,
			Probes:     update.Probes,
		})
		for _, probe := range update.Probes {
			m.diagnostics.reportReceived(runtimeID, probe)
		}
		if update.ShouldUploadSymDB {
			if err := m.symdb.queueUpload(runtimeID, update.Executable.Path); err != nil {
				log.Warnf("Failed to queue SymDB upload for process %v: %v", runtimeID.ProcessID, err)
			}
		} else {
			m.symdb.removeUpload(runtimeID)
		}
	}
	if m.tenant != nil {
		m.tenant.HandleUpdate(actuator.ProcessesUpdate{Processes: actuatorUpdates})
	}
}

// CheckForUpdates runs a single iteration of the update loop. Exposed for tests.
func (m *Module) CheckForUpdates() {
	m.checkForUpdates()
}

// HandleRemovals removes the provided processes. Exposed for tests.
func (m *Module) HandleRemovals(removals []procmon.ProcessID) {
	m.handleRemovals(removals)
}

func jitter(duration time.Duration, fraction float64) time.Duration {
	multiplier := 1 + ((rand.Float64()*2 - 1) * fraction)
	return time.Duration(float64(duration) * multiplier)
}
