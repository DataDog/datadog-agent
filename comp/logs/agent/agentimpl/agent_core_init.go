// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package agentimpl

import (
	"time"

	"github.com/spf13/afero"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/client/http"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers/container"
	filelauncher "github.com/DataDog/datadog-agent/pkg/logs/launchers/file"
	integrationLauncher "github.com/DataDog/datadog-agent/pkg/logs/launchers/integration"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers/journald"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers/listener"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers/windowsevent"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/schedulers"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers/file"
	"github.com/DataDog/datadog-agent/pkg/logs/types"
	"github.com/DataDog/datadog-agent/pkg/logs/util/opener"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// NewAgent returns a new Logs Agent
func (a *logAgent) SetupPipeline(processingRules []*config.ProcessingRule, wmeta option.Option[workloadmeta.Component], integrationsLogs integrations.Component, fingerprintConfig types.FingerprintConfig) {
	destinationsCtx := client.NewDestinationsContext()
	diagnosticMessageReceiver := diagnostic.NewBufferedMessageReceiver(nil, a.hostname)

	// setup the pipeline provider that provides pairs of processor and sender
	pipelineProvider := buildPipelineProvider(a, processingRules, diagnosticMessageReceiver, destinationsCtx)

	// setup the launchers
	lnchrs := launchers.NewLaunchers(a.sources, pipelineProvider, a.auditor, a.tracker)

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

// buildPipelineProvider builds a new pipeline provider with the given configuration
func buildPipelineProvider(a *logAgent, processingRules []*config.ProcessingRule, diagnosticMessageReceiver *diagnostic.BufferedMessageReceiver, destinationsCtx *client.DestinationsContext) pipeline.Provider {
	pipelineProvider := pipeline.NewProvider(
		a.config.GetInt("logs_config.pipelines"),
		a.auditor,
		diagnosticMessageReceiver,
		processingRules,
		a.endpoints,
		destinationsCtx,
		NewStatusProvider(),
		a.hostname,
		a.config,
		a.compression,
		a.config.GetBool("logs_config.disable_distributed_senders"), // legacy
		false, // serverless
	)
	return pipelineProvider
}

// rebuildTransientComponents recreates only the components that need to change during a restart.
//
// Components recreated (transient):
//   - destinationsCtx: New context for new transport connections
//   - pipelineProvider: New pipeline with updated endpoints and configuration
//   - launchers: New launchers connected to the new pipeline
func (a *logAgent) rebuildTransientComponents(processingRules []*config.ProcessingRule, wmeta option.Option[workloadmeta.Component], integrationsLogs integrations.Component, fingerprintConfig types.FingerprintConfig) {
	destinationsCtx := client.NewDestinationsContext()
	pipelineProvider := buildPipelineProvider(a, processingRules, a.diagnosticMessageReceiver, destinationsCtx)

	// recreate launchers with new pipelineProvider
	// use OLD: sources, auditor, tracker
	lnchers := launchers.NewLaunchers(a.sources, pipelineProvider, a.auditor, a.tracker)
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
	fingerprinter := file.NewFingerprinter(fingerprintConfig, fileOpener)
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
