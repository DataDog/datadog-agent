// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

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

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/dispatcher"
	"github.com/DataDog/datadog-agent/pkg/dyninst/eventbuf"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
	"github.com/DataDog/datadog-agent/pkg/dyninst/module/tombstone"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/process"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procsubscribe"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcjson"
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
	config       *Config

	cancel context.CancelFunc

	// metrics reports the values of metric probes. It is nil when the module
	// has no client to report them to.
	metrics *metricsReporter

	shutdown struct {
		sync.Once
		realDependencies  realDependencies
		metricProbeStatsd ddgostatsd.ClientInterface
	}
}

// Option configures a Module.
type Option func(*options)

type options struct {
	statsd            ddgostatsd.ClientInterface
	metricProbeStatsd ddgostatsd.ClientInterface
	workloadMeta      workloadmeta.Component
}

// WithStatsd sets the statsd client that the module reports metrics to. Without
// one, no metrics are reported.
func WithStatsd(client ddgostatsd.ClientInterface) Option {
	return func(o *options) { o.statsd = client }
}

// WithMetricProbeStatsd sets the statsd client that the values of metric probes
// are reported to. Without one, metric probes are installed but never report
// anything. The module closes the client when it shuts down.
func WithMetricProbeStatsd(client ddgostatsd.ClientInterface) Option {
	return func(o *options) { o.metricProbeStatsd = client }
}

// WithWorkloadMeta sets the workloadmeta store used to find the Kubernetes pods
// of the processes that metric probes report about. Without one, metric probe
// metrics are not tagged with the target's pod.
func WithWorkloadMeta(wmeta workloadmeta.Component) Option {
	return func(o *options) { o.workloadMeta = wmeta }
}

// NewModule creates a new dynamic instrumentation module.
func NewModule(
	config *Config,
	remoteConfigSubscriber procsubscribe.RemoteConfigSubscriber,
	opts ...Option,
) (_ *Module, err error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	realDeps, err := makeRealDependencies(config, remoteConfigSubscriber, o.statsd)
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
	var moduleOpts []newUnstartedModuleOption
	if o.metricProbeStatsd != nil {
		var lookupContainer func(string) (string, bool)
		if o.workloadMeta != nil {
			lookupContainer = podLookup(o.workloadMeta)
		}
		moduleOpts = append(moduleOpts, withMetricsClient(o.metricProbeStatsd, lookupContainer))
	}
	if k := config.TestingKnobs; k.SinkOverride != nil || k.OnProgramLoaded != nil {
		moduleOpts = append(moduleOpts, withRuntimeTestingKnobs(&runtimeTestingKnobs{
			sinkOverride:    k.SinkOverride,
			onProgramLoaded: k.OnProgramLoaded,
		}))
	}
	m := newUnstartedModule(deps, config.ProbeTombstoneFilePath, moduleOpts...)
	m.config = config
	m.shutdown.realDependencies = realDeps
	m.shutdown.metricProbeStatsd = o.metricProbeStatsd

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

// eventbufBudgetBytes is the per-process byte ceiling enforced across all
// per-program event buffers. Matches the pre-Buffer pairing-store budget.
// TODO: make this configurable.
const eventbufBudgetBytes = 512 << 18

// tombstoneFilePath is the path to the tombstone file left behind to detect
// crashes while loading programs. If empty, tombstone files are not
// created.
//
// tombstoneFilePath is the path to the tombstone file left behind to detect
// crashes while loading programs. If empty, tombstone files are not created.
// newUnstartedModuleOption configures a *Module / *runtimeImpl beyond
// the dependencies threaded through dependencies. Used to keep
// newUnstartedModule's positional arguments stable as test-only fields
// are added.
type newUnstartedModuleOption func(*runtimeImpl)

func withRuntimeTestingKnobs(k *runtimeTestingKnobs) newUnstartedModuleOption {
	return func(rt *runtimeImpl) { rt.testingKnobs = k }
}

// withMetricsClient makes the module report the values of metric probes to
// client. lookupContainer finds the pod of a container; it may be nil.
func withMetricsClient(
	client metricsClient, lookupContainer func(containerID string) (podUID string, ok bool),
) newUnstartedModuleOption {
	return func(rt *runtimeImpl) {
		rt.metrics = newMetricsReporter(
			client, defaultMetricsReportInterval, rt.setProbeMaybeEmitting, lookupContainer,
		)
	}
}

// podLookup returns a function that finds the Kubernetes pod of a container in
// workloadmeta.
func podLookup(wmeta workloadmeta.Component) func(containerID string) (podUID string, ok bool) {
	return func(containerID string) (string, bool) {
		container, err := wmeta.GetContainer(containerID)
		if err != nil {
			return "", false
		}
		if owner := container.Owner; owner != nil && owner.Kind == workloadmeta.KindKubernetesPod {
			return owner.ID, true
		}
		return "", true
	}
}

func newUnstartedModule(deps dependencies, tombstoneFilePath string, opts ...newUnstartedModuleOption) *Module {
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
		actuator:                 deps.Actuator,
		decoderFactory:           deps.DecoderFactory,
		irGenerator:              deps.IRGenerator,
		programCompiler:          deps.ProgramCompiler,
		kernelLoader:             deps.KernelLoader,
		attacher:                 deps.Attacher,
		dispatcher:               deps.Dispatcher,
		logsFactory:              logsUploader,
		procRuntimeIDbyProgramID: &sync.Map{},
		eventbufBudget:           eventbuf.NewBudget(eventbufBudgetBytes),
		tombstoneFilePath:        tombstoneFilePath,
	}
	for _, opt := range opts {
		opt(runtime)
	}
	deps.Actuator.SetRuntime(runtime)
	m := &Module{
		store:        store,
		diagnostics:  diagnostics,
		symdb:        deps.symdbManager,
		actuator:     deps.Actuator,
		runtimeStats: &runtime.stats,
		metrics:      runtime.metrics,
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
	statsd ddgostatsd.ClientInterface,
) (_ realDependencies, retErr error) {
	var ret realDependencies
	defer func() {
		if retErr != nil {
			ret.shutdown()
		}
	}()

	logsURL, err := url.Parse(config.LogUploaderURL)
	if err != nil {
		return ret, fmt.Errorf("error parsing log uploader URL: %w", err)
	}
	ret.logUploader = uploader.NewLogsUploaderFactory(logsURL)

	diagsUploaderURL, err := url.Parse(config.DiagsUploaderURL)
	if err != nil {
		return ret, fmt.Errorf("error parsing diagnostics uploader URL: %w", err)
	}
	diagsUploader := uploader.NewDiagnosticsUploader(diagsUploaderURL)
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
	if cb := config.TestingKnobs.OnLoaderReady; cb != nil {
		cb(ret.loader)
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
	ret.dispatcher = dispatcher.NewDispatcher(
		ret.loader.OutputReader(), ret.loader.DropNotifyReader())
	ret.procSubscriber = procsubscribe.NewSubscriber(
		remoteConfigSubscriber,
		procsubscribe.WithStatsd(statsd),
	)

	approximateBootTime := time.Now().Add(time.Duration(-ts.Nano()))
	ret.decoderFactory = decoderFactory{approximateBootTime: approximateBootTime}
	ret.symdbManager = newSymdbManager(
		symdbUploaderURL, ret.objectLoader, config.SymDBCacheDir,
		withStatsd(statsd),
	)
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
	if sub := m.shutdown.realDependencies.procSubscriber; sub != nil {
		stats["processDiscovery"] = sub.Stats()
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

	m.registerDebugEndpoints(router)

	return nil
}

// Close closes the Module.
func (m *Module) Close() {
	m.shutdown.Once.Do(func() {
		log.Debugf("closing dynamic instrumentation module")
		m.cancel()
		// Shutting down the actuator detaches the programs, which reports their
		// metric probes one last time, so the metrics reporter goes second.
		m.shutdown.realDependencies.shutdown()
		if m.metrics != nil {
			m.metrics.close()
		}
		if c := m.shutdown.metricProbeStatsd; c != nil {
			if err := c.Close(); err != nil {
				log.Warnf("error closing the metric probes statsd client: %v", err)
			}
		}
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
			m.diagnostics.retain(runtimeID, configuredProbes(update))
			probes := m.validProbes(runtimeID, update)
			actuatorUpdates = append(actuatorUpdates, actuator.ProcessUpdate{
				Info:   update.Info,
				Probes: probes,
			})
			for _, probe := range probes {
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

// configuredProbes returns all the probes configured for the process, whether
// or not they can be instrumented.
func configuredProbes(update *process.Config) []ir.ProbeIDer {
	probes := make([]ir.ProbeIDer, 0, len(update.Probes)+len(update.InvalidProbes))
	for _, probe := range update.Probes {
		probes = append(probes, probe)
	}
	for _, probe := range update.InvalidProbes {
		probes = append(probes, probe)
	}
	return probes
}

// validProbes reports an error for each probe configured for the process whose
// definition is invalid, and returns the probes that remain.
func (m *Module) validProbes(
	runtimeID procRuntimeID, update *process.Config,
) []ir.ProbeDefinition {
	for _, invalid := range update.InvalidProbes {
		m.reportInvalidProbe(runtimeID, invalid, invalid.Issue)
	}
	valid := make([]ir.ProbeDefinition, 0, len(update.Probes))
	for _, probe := range update.Probes {
		if err := validateProbe(probe); err != nil {
			m.reportInvalidProbe(runtimeID, probe, ir.Issue{
				Kind:    ir.IssueKindInvalidProbeDefinition,
				Message: err.Error(),
			})
			continue
		}
		valid = append(valid, probe)
	}
	return valid
}

func (m *Module) reportInvalidProbe(
	runtimeID procRuntimeID, probe ir.ProbeIDer, issue ir.Issue,
) {
	if m.diagnostics.reportError(
		runtimeID, probe, (*irIssueError)(&issue), issue.Kind.String(),
	) {
		log.Debugf(
			"reported issue %v for probe %v %v: %v",
			issue.Kind, probe.GetID(), probe.GetVersion(), issue.Message,
		)
	}
}

// validateProbe checks that a probe's definition is one that can be
// instrumented. Only probes defined in the Remote Config format are checked.
func validateProbe(probe ir.ProbeDefinition) error {
	if p, ok := probe.(rcjson.Probe); ok {
		return rcjson.Validate(p)
	}
	return nil
}
