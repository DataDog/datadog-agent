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
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/dispatcher"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
	"github.com/DataDog/datadog-agent/pkg/dyninst/module/tombstone"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/process"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procsubscribe"
	"github.com/DataDog/datadog-agent/pkg/dyninst/uploader"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Module is the dynamic instrumentation system probe module.
type Module struct {
	actuator Actuator
	symdb    symdbManagerInterface

	store        *processStore
	diagnostics  *diagnosticsManager
	runtimeStats *runtimeStats

	cancel context.CancelFunc

	shutdown struct {
		sync.Once
		realDependencies realDependencies
	}
}

// NewModule creates a new dynamic instrumentation module.
func NewModule(
	config *Config,
	remoteConfigSubscriber procsubscribe.RemoteConfigSubscriber,
) (_ *Module, err error) {
	realDeps, err := makeRealDependencies(config, remoteConfigSubscriber)
	if err != nil {
		return nil, err
	}
	deps := realDeps.asDependencies()
	if override := config.TestingKnobs.ProcessSubscriberOverride; override != nil {
		deps.ProcessSubscriber = override(deps.ProcessSubscriber)
	}
	if override := config.TestingKnobs.IRGeneratorOverride; override != nil {
		deps.IRGenerator = override(deps.IRGenerator)
	}
	m := newUnstartedModule(deps, config.ProbeTombstoneFilePath)
	m.shutdown.realDependencies = realDeps

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel

	if deps.ProcessSubscriber != nil {
		// Start the subscriber in a separate goroutine since WaitOutTombstone()
		// may block.
		go func() {
			// Wait for a while if we're recovering from a crash.
			tombstone.WaitOutTombstone(ctx, config.ProbeTombstoneFilePath, config.TestingKnobs.TombstoneSleepKnobs)

			deps.ProcessSubscriber.Start()
		}()
	}
	return m, nil
}

// TODO: make this configurable.
const bufferedMessagesByteLimit = 512 << 10

// tombstoneFilePath is the path to the tombstone file left behind to detect
// crashes while loading programs. If empty, tombstone files are not
// created.
//
// tombstoneFilePath is the path to the tombstone file left behind to detect
// crashes while loading programs. If empty, tombstone files are not created.
func newUnstartedModule(deps dependencies, tombstoneFilePath string) *Module {
	// A zero-value symdbManager is valid and disabled.
	if deps.symdbManager == nil {
		deps.symdbManager = &symdbManager{}
	}
	store := newProcessStore()
	logsUploader := logsUploaderFactoryImpl[LogsUploader]{factory: deps.LogsFactory}
	diagnostics := newDiagnosticsManager(deps.DiagnosticsUploader)
	bufferedMessagesTracker := newBufferedMessageTracker(bufferedMessagesByteLimit)
	runtime := &runtimeImpl{
		store:                    store,
		diagnostics:              diagnostics,
		actuator:                 deps.Actuator,
		decoderFactory:           deps.DecoderFactory,
		irGenerator:              deps.IRGenerator,
		programCompiler:          deps.ProgramCompiler,
		kernelLoader:             deps.KernelLoader,
		attacher:                 deps.Attacher,
		dispatcher:               deps.Dispatcher,
		logsFactory:              logsUploader,
		procRuntimeIDbyProgramID: &sync.Map{},
		bufferedMessageTracker:   bufferedMessagesTracker,
		tombstoneFilePath:        tombstoneFilePath,
	}
	deps.Actuator.SetRuntime(runtime)
	m := &Module{
		store:        store,
		diagnostics:  diagnostics,
		symdb:        deps.symdbManager,
		actuator:     deps.Actuator,
		runtimeStats: &runtime.stats,
		cancel:       func() {}, // This gets overwritten in NewModule
	}
	if deps.ProcessSubscriber != nil {
		deps.ProcessSubscriber.Subscribe(m.handleProcessesUpdate)
	}
	return m
}

type realDependencies struct {
	logUploader    *uploader.LogsUploaderFactory
	diagsUploader  *uploader.DiagnosticsUploader
	actuator       *actuator.Actuator
	dispatcher     *dispatcher.Dispatcher
	loader         *loader.Loader
	attacher       *defaultAttacher
	symdbManager   *symdbManager
	procSubscriber *procsubscribe.Subscriber

	decoderFactory  decoderFactory
	programCompiler *stackMachineCompiler

	objectLoader object.Loader
	irGenerator  IRGenerator
}

func (c *realDependencies) asDependencies() dependencies {
	return dependencies{
		Actuator:            c.actuator,
		ProcessSubscriber:   c.procSubscriber,
		Dispatcher:          c.dispatcher,
		DecoderFactory:      c.decoderFactory,
		IRGenerator:         c.irGenerator,
		ProgramCompiler:     c.programCompiler,
		KernelLoader:        c.loader,
		Attacher:            c.attacher,
		LogsFactory:         logsUploaderFactoryImpl[*uploader.LogsUploader]{factory: c.logUploader},
		DiagnosticsUploader: c.diagsUploader,
		symdbManager:        c.symdbManager,
	}
}

func (c *realDependencies) shutdown() {
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
	if c.logUploader != nil {
		c.logUploader.Stop()
	}
	if c.diagsUploader != nil {
		c.diagsUploader.Stop()
	}
	if c.symdbManager != nil {
		c.symdbManager.stop()
	}
	if c.procSubscriber != nil {
		c.procSubscriber.Close()
	}
}

func makeRealDependencies(
	config *Config,
	remoteConfigSubscriber procsubscribe.RemoteConfigSubscriber,
) (_ realDependencies, retErr error) {
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
	ret.logUploader = uploader.NewLogsUploaderFactory(
		uploader.WithURL(logUploaderURL),
	)

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
	ret.actuator = actuator.NewActuator(config.ActuatorConfig)

	var loaderOpts []loader.Option
	if config.TestingKnobs.LoaderOptions != nil {
		loaderOpts = config.TestingKnobs.LoaderOptions
	}
	ret.loader, err = loader.NewLoader(loaderOpts...)
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
	ret.procSubscriber = procsubscribe.NewSubscriber(
		remoteConfigSubscriber,
	)

	approximateBootTime := time.Now().Add(time.Duration(-ts.Nano()))
	ret.decoderFactory = decoderFactory{approximateBootTime: approximateBootTime}
	ret.symdbManager = newSymdbManager(symdbUploaderURL, ret.objectLoader, config.SymDBCacheDir)
	ret.attacher = &defaultAttacher{}
	ret.programCompiler = &stackMachineCompiler{}
	return ret, nil
}

// GetStats returns the stats of the module.
func (m *Module) GetStats() map[string]any {
	stats := map[string]any{}
	if m.shutdown.realDependencies.actuator != nil {
		actuatorStats := m.shutdown.realDependencies.actuator.Stats()
		if actuatorStats != nil {
			stats["actuator"] = actuatorStats
		}
	}
	if m.runtimeStats != nil {
		stats["runtime"] = m.runtimeStats.asStats()
	}
	return stats
}

// Register registers the module to the router
func (m *Module) Register(router *module.Router) error {
	router.HandleFunc(
		"/check",
		utils.WithConcurrencyLimit(
			utils.DefaultMaxConcurrentRequests,
			func(w http.ResponseWriter, req *http.Request) {
				utils.WriteAsJSON(
					req, w, json.RawMessage(`{"status":"ok"}`), utils.CompactOutput,
				)
			},
		),
	)
	// Handler for printing debug information about the known Go processes with
	// the Datadog tracer. These processes are watched for Remote Config updates
	// related to Dynamic Instrumentation.
	router.HandleFunc(
		"/debug/goprocs",
		utils.WithConcurrencyLimit(
			utils.DefaultMaxConcurrentRequests,
			func(w http.ResponseWriter, req *http.Request) {
				if m.shutdown.realDependencies.procSubscriber == nil {
					utils.WriteAsJSON(req, w, nil, utils.PrettyPrint)
					return
				}

				report := m.shutdown.realDependencies.procSubscriber.GetReport()
				utils.WriteAsJSON(req, w, report, utils.PrettyPrint)
			},
		),
	)
	return nil
}

// Close closes the Module.
func (m *Module) Close() {
	m.shutdown.Once.Do(func() {
		log.Debugf("closing dynamic instrumentation module")
		m.cancel()
		m.shutdown.realDependencies.shutdown()
	})
}

func (m *Module) handleProcessesUpdate(update process.ProcessesUpdate) {
	if removals := update.Removals; len(removals) > 0 {
		m.store.remove(removals, m.diagnostics)
		if len(removals) > 0 && m.actuator != nil {
			m.actuator.HandleUpdate(actuator.ProcessesUpdate{Removals: removals})
		}
		for _, pid := range removals {
			m.symdb.removeUploadByPID(pid)
		}
	}
	if updates := update.Updates; len(updates) > 0 {
		actuatorUpdates := make([]actuator.ProcessUpdate, 0, len(updates))
		for i := range updates {
			update := &updates[i]
			runtimeID := m.store.ensureExists(update)
			actuatorUpdates = append(actuatorUpdates, actuator.ProcessUpdate{
				Info:   update.Info,
				Probes: update.Probes,
			})
			m.diagnostics.retain(runtimeID, update.Probes)
			for _, probe := range update.Probes {
				m.diagnostics.reportReceived(runtimeID, probe)
			}
			if update.ShouldUploadSymDB {
				// Perform the upload, unless it was already queued previously.
				if err := m.symdb.queueUpload(runtimeID, update.Executable.Path); err != nil {
					log.Warnf("Failed to queue SymDB upload for process %v: %v", runtimeID.ID, err)
				}
			}
			// NOTE: we don't do anything if ShouldUploadSymDB is false, even
			// when it switches from true to false. We could attempt to cancel
			// an upload if it's in progress, but then would we would probably
			// also want to re-attempt it if the flag switches back to true
			// later; this is complicated since, in other cases, we don't want
			// to re-upload (i.e. after a successful upload), so it's easier to
			// do nothing.
		}
		if m.actuator != nil {
			m.actuator.HandleUpdate(actuator.ProcessesUpdate{Processes: actuatorUpdates})
		}
	}
}
