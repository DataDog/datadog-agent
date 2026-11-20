// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package agentimpl

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/afero"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/logs-library/client"
	"github.com/DataDog/datadog-agent/comp/logs-library/client/http"
	"github.com/DataDog/datadog-agent/comp/logs-library/diagnostic"
	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs-library/pipeline"
	"github.com/DataDog/datadog-agent/comp/logs-library/routing"
	"github.com/DataDog/datadog-agent/comp/logs-library/tagfilter"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/def"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers/container"
	filelauncher "github.com/DataDog/datadog-agent/pkg/logs/launchers/file"
	integrationLauncher "github.com/DataDog/datadog-agent/pkg/logs/launchers/integration"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers/journald"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers/listener"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers/windowsevent"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/schedulers"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers/file"
	"github.com/DataDog/datadog-agent/pkg/logs/types"
	"github.com/DataDog/datadog-agent/pkg/logs/util/opener"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

func (a *logAgent) configureTagFilters() {
	// Invalid patterns degrade to filtering less and must not block startup.
	tagFilters, report, err := config.GlobalTagFilters(a.config)
	if err != nil {
		report.Warnings = append(report.Warnings, fmt.Sprintf("Invalid tag_filters setting: %v", err))
	}
	for _, warning := range report.Warnings {
		a.log.Warn(warning)
	}
	for _, rejected := range report.Rejected {
		a.log.Warnf("tag_filters: %s", rejected.Reason)
	}
	// status warnings are registered after status.Init in startTagFiltering.
	a.tagFilterReport = report
	a.tagFilters = tagFilters
}

func (a *logAgent) startTagFiltering() {
	a.reportTagFilterWarnings()
	a.tagFilterSubscriberDone = make(chan struct{})
	startTagFilterSubscriber(a.sources, a.tagFilters, a.tagFilterSubscriberDone)
}

// NewAgent returns a new Logs Agent
func (a *logAgent) SetupPipeline(processingRules []*config.ProcessingRule, tagFilters *tagfilter.Filters, wmeta option.Option[workloadmeta.Component], integrationsLogs integrations.Component, fingerprintConfig types.FingerprintConfig) {
	destinationsCtx := client.NewDestinationsContext()
	diagnosticMessageReceiver := diagnostic.NewBufferedMessageReceiver(nil, a.hostname, a.config)

	// setup the pipeline provider that Provides pairs of processor and sender
	pipelineProvider := buildPipelineProvider(a, processingRules, tagFilters, diagnosticMessageReceiver, destinationsCtx)

	// setup the launchers (guard direct checkpoint writes when routed provider is active)
	lnchrs := launchers.NewLaunchers(a.sources, pipelineProvider, guardRegistryForProvider(pipelineProvider, a.auditor), a.tracker)

	a.addLauncherInstances(lnchrs, wmeta, integrationsLogs, fingerprintConfig)

	a.schedulers = schedulers.NewSchedulers(a.sources, a.services)
	a.destinationsCtx = destinationsCtx
	a.pipelineProvider = pipelineProvider
	a.launchers = lnchrs
	a.diagnosticMessageReceiver = diagnosticMessageReceiver
}

// buildEndpoints builds endpoints for the logs agent, either HTTP or TCP,
// dependent on configuration and connectivity
func buildEndpoints(coreConfig model.Reader) (*config.Endpoints, error) {
	httpConnectivity := config.HTTPConnectivityFailure
	if endpoints, err := buildHTTPEndpointsForConnectivityCheck(coreConfig); err == nil {
		httpConnectivity = http.CheckConnectivity(endpoints.Main, coreConfig)
	}

	// Publish HTTP connectivity check metric
	if httpConnectivity == config.HTTPConnectivitySuccess {
		metrics.TlmHTTPConnectivityCheck.Inc("success")
	} else {
		metrics.TlmHTTPConnectivityCheck.Inc("failure")
	}

	return config.BuildEndpointsWithVectorOverride(coreConfig, httpConnectivity, intakeTrackType, config.AgentJSONIntakeProtocol, config.DefaultIntakeOrigin)
}

// buildHTTPEndpointsForConnectivityCheck builds HTTP endpoints for connectivity testing only.
// Uses BuildEndpointsForDiagnostic to avoid registering config update callbacks since these
// endpoints are transient and will be discarded after the connectivity check.
func buildHTTPEndpointsForConnectivityCheck(coreConfig model.Reader) (*config.Endpoints, error) {
	return config.BuildEndpointsForDiagnostic(
		coreConfig,
		config.DefaultLogsConfigKeysWithVectorOverride(coreConfig),
		config.DefaultDiagnosticPrefix,
		config.DiagnosticHTTP,
		intakeTrackType,
		config.AgentJSONIntakeProtocol,
		config.DefaultIntakeOrigin,
	)
}

// checkHTTPConnectivityStatus performs an HTTP connectivity check and returns the status
func checkHTTPConnectivityStatus(endpoint config.Endpoint, coreConfig model.Reader) config.HTTPConnectivity {
	return http.CheckConnectivity(endpoint, coreConfig)
}

// buildHTTPEndpointsForRestart builds HTTP endpoints for restart without connectivity check
// This is used when we've already verified HTTP connectivity and are upgrading from TCP
func buildHTTPEndpointsForRestart(coreConfig model.Reader) (*config.Endpoints, error) {
	// Force HTTP endpoints since we already confirmed connectivity
	return config.BuildEndpointsWithVectorOverride(coreConfig, config.HTTPConnectivitySuccess, intakeTrackType, config.AgentJSONIntakeProtocol, config.DefaultIntakeOrigin)
}

// validateRoutingConfig validates endpoint routing configuration and builds the
// endpoint policy when logs_config.routing.enabled is true.
func (a *logAgent) validateRoutingConfig() error {
	if !a.config.GetBool("logs_config.routing.enabled") {
		a.routingPolicy = nil
		return nil
	}
	if a.endpoints == nil || !a.endpoints.UseHTTP {
		return errors.New("logs_config.routing.enabled requires HTTP transport (set logs_config.use_http or logs_config.force_use_http)")
	}
	if a.endpoints.UseProto {
		return errors.New("logs_config.routing.enabled is not supported with protobuf transport")
	}
	if a.config.GetBool("logs_config.disable_distributed_senders") {
		return errors.New("logs_config.routing.enabled is not supported with logs_config.disable_distributed_senders")
	}
	if a.config.GetBool("multi_region_failover.enabled") {
		return errors.New("logs_config.routing.enabled is not supported with multi_region_failover.enabled")
	}
	policy, err := a.buildRoutingPolicy(a.endpoints)
	if err != nil {
		return err
	}
	a.routingPolicy = policy
	return nil
}

func (a *logAgent) buildRoutingPolicy(endpoints *config.Endpoints) (*routing.EndpointPolicy, error) {
	if a.routingPolicyBuilder != nil {
		return a.routingPolicyBuilder(endpoints)
	}
	maxEndpoints := a.config.GetInt("logs_config.routing.max_groups")
	if maxEndpoints < 1 {
		maxEndpoints = routing.MaxEndpoints
	}
	return routing.NewEndpointPolicy(routing.PolicyOptions{
		Endpoints:    endpoints,
		Compiler:     routing.StubCompiler{},
		MaxEndpoints: maxEndpoints,
	})
}

func guardRegistryForProvider(provider pipeline.Provider, registry auditor.Registry) auditor.Registry {
	if routed, ok := provider.(*pipeline.RoutedProvider); ok && registry != nil {
		return routed.GuardRegistry(registry)
	}
	return registry
}

// sharedIngressGenerations supplies stable per-identifier generations for
// launchers that still send checkpointed records over shared ingress.
type sharedIngressGenerations struct {
	mu   sync.Mutex
	next atomic.Uint64
	gens map[string]uint64
}

func (g *sharedIngressGenerations) generation(msg *message.Message) (uint64, error) {
	if msg == nil || msg.Origin == nil || msg.Origin.Identifier == "" {
		return 0, nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.gens == nil {
		g.gens = make(map[string]uint64)
	}
	if gen, ok := g.gens[msg.Origin.Identifier]; ok {
		return gen, nil
	}
	gen := g.next.Add(1)
	g.gens[msg.Origin.Identifier] = gen
	return gen, nil
}

// buildPipelineProvider builds a new pipeline provider with the given configuration
func buildPipelineProvider(a *logAgent, processingRules []*config.ProcessingRule, tagFilters *tagfilter.Filters, diagnosticMessageReceiver *diagnostic.BufferedMessageReceiver, destinationsCtx *client.DestinationsContext) pipeline.Provider {
	if a.config.GetBool("logs_config.routing.enabled") {
		policy := a.routingPolicy
		if policy == nil {
			var err error
			policy, err = a.buildRoutingPolicy(a.endpoints)
			if err != nil {
				a.log.Errorf("Failed to build logs routing policy: %v", err)
			}
		} else if len(a.endpoints.Endpoints) > 0 && a.routingPolicyBuilder == nil {
			// Rebuild when endpoints changed (e.g. intake routing update) while preserving policy validation.
			if refreshed, err := a.buildRoutingPolicy(a.endpoints); err == nil {
				policy = refreshed
				a.routingPolicy = refreshed
			}
		}
		if policy != nil {
			numPipelines := a.config.GetInt("logs_config.pipelines")
			if numPipelines < 1 {
				numPipelines = 1
			} else if numPipelines > 16 {
				numPipelines = 16
			}
			sharedGens := &sharedIngressGenerations{}
			routedProvider, err := pipeline.NewRoutedProvider(pipeline.RoutedOptions{
				NumberOfPipelines:         numPipelines,
				Sink:                      a.auditor,
				DiagnosticMessageReceiver: diagnosticMessageReceiver,
				ProcessingRules:           processingRules,
				TagFilters:                tagFilters,
				EndpointPolicy:            policy,
				Generation:                sharedGens.generation,
				DestinationsContext:       destinationsCtx,
				Hostname:                  a.hostname,
				Config:                    a.config,
				Compression:               a.compression,
				Secrets:                   a.secrets,
			})
			if err == nil {
				return routedProvider
			}
			a.log.Errorf("Failed to construct routed pipeline provider: %v", err)
		}
	}
	pipelineProvider := pipeline.NewProvider(
		a.config.GetInt("logs_config.pipelines"),
		a.auditor,
		diagnosticMessageReceiver,
		processingRules,
		tagFilters,
		a.endpoints,
		destinationsCtx,
		NewStatusProvider(),
		a.hostname,
		a.config,
		a.compression,
		a.config.GetBool("logs_config.disable_distributed_senders"), // legacy
		false, // serverless
		a.secrets,
	)
	return pipelineProvider
}

// rebuildTransientComponents recreates only the components that need to change during a restart.
//
// Components recreated (transient):
//   - destinationsCtx: New context for new transport connections
//   - pipelineProvider: New pipeline with updated endpoints and configuration
//   - launchers: New launchers connected to the new pipeline
func (a *logAgent) rebuildTransientComponents(processingRules []*config.ProcessingRule, tagFilters *tagfilter.Filters, wmeta option.Option[workloadmeta.Component], integrationsLogs integrations.Component, fingerprintConfig types.FingerprintConfig) {
	destinationsCtx := client.NewDestinationsContext()
	pipelineProvider := buildPipelineProvider(a, processingRules, tagFilters, a.diagnosticMessageReceiver, destinationsCtx)

	// recreate launchers with new pipelineProvider
	// use OLD: sources, auditor, tracker
	lnchers := launchers.NewLaunchers(a.sources, pipelineProvider, guardRegistryForProvider(pipelineProvider, a.auditor), a.tracker)
	a.addLauncherInstances(lnchers, wmeta, integrationsLogs, fingerprintConfig)

	// update agent with new components
	// schedulers, sources, tracker, auditor, diagnosticMessageReceiver remain unchanged
	a.destinationsCtx = destinationsCtx
	a.pipelineProvider = pipelineProvider
	a.launchers = lnchers
}

func (a *logAgent) addLauncherInstances(lnchrs *launchers.Launchers, wmeta option.Option[workloadmeta.Component], integrationsLogs integrations.Component, fingerprintConfig types.FingerprintConfig) {
	fileLimits := a.config.GetInt("logs_config.open_files_limit")
	fileValidatePodContainer := a.config.GetBool("logs_config.validate_pod_container_id")
	fileScanPeriod := time.Duration(a.config.GetFloat64("logs_config.file_scan_period") * float64(time.Second))
	fileWildcardSelectionMode := a.config.GetString("logs_config.file_wildcard_selection_mode")
	fileOpener := opener.NewFileOpener()
	fingerprinter := file.NewFingerprinterWithUnreliableMount(fingerprintConfig, fileOpener, config.UnreliableMountEnabled(a.config))
	lnchrs.AddLauncher(filelauncher.NewLauncher(
		fileLimits,
		filelauncher.DefaultSleepDuration,
		fileValidatePodContainer,
		fileScanPeriod,
		fileWildcardSelectionMode,
		a.flarecontroller,
		a.tagger,
		fileOpener,
		fingerprinter,
	))
	lnchrs.AddLauncher(listener.NewLauncher(a.config.GetInt("logs_config.frame_size")))
	lnchrs.AddLauncher(journald.NewLauncher(a.flarecontroller, a.tagger))
	lnchrs.AddLauncher(windowsevent.NewLauncher())
	lnchrs.AddLauncher(container.NewLauncher(a.sources, wmeta, a.tagger))
	lnchrs.AddLauncher(integrationLauncher.NewLauncher(
		afero.NewOsFs(),
		a.sources, integrationsLogs))

}
