// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package agentimpl

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/client/http"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers/container"
	filelauncher "github.com/DataDog/datadog-agent/pkg/logs/launchers/file"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers/journald"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers/listener"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers/windowsevent"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/schedulers"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// NewAgent returns a new Logs Agent
func (a *logAgent) SetupPipeline(processingRules []*config.ProcessingRule, wmeta optional.Option[workloadmeta.Component]) {
	health := health.RegisterLiveness("logs-agent")

	// setup the auditor
	// We pass the health handle to the auditor because it's the end of the pipeline and the most
	// critical part. Arguably it could also be plugged to the destination.
	auditorTTL := time.Duration(a.config.GetInt("logs_config.auditor_ttl")) * time.Hour
	auditor := auditor.New(a.config.GetString("logs_config.run_path"), auditor.DefaultRegistryFilename, auditorTTL, health)
	destinationsCtx := client.NewDestinationsContext()
	diagnosticMessageReceiver := diagnostic.NewBufferedMessageReceiver(nil, a.hostname)

	// setup the pipeline provider that provides pairs of processor and sender
	pipelineProvider := pipeline.NewProvider(config.NumberOfPipelines, auditor, diagnosticMessageReceiver, processingRules, a.endpoints, destinationsCtx, NewStatusProvider(), a.hostname, a.config)

	// setup the launchers
	lnchrs := launchers.NewLaunchers(a.sources, pipelineProvider, auditor, a.tracker)
	lnchrs.AddLauncher(filelauncher.NewLauncher(
		a.config.GetInt("logs_config.open_files_limit"),
		filelauncher.DefaultSleepDuration,
		a.config.GetBool("logs_config.validate_pod_container_id"),
		time.Duration(a.config.GetFloat64("logs_config.file_scan_period")*float64(time.Second)),
		a.config.GetString("logs_config.file_wildcard_selection_mode"), a.flarecontroller))
	lnchrs.AddLauncher(listener.NewLauncher(a.config.GetInt("logs_config.frame_size")))
	lnchrs.AddLauncher(journald.NewLauncher(a.flarecontroller))
	lnchrs.AddLauncher(windowsevent.NewLauncher())
	lnchrs.AddLauncher(container.NewLauncher(a.sources, wmeta))

	a.schedulers = schedulers.NewSchedulers(a.sources, a.services)
	a.auditor = auditor
	a.destinationsCtx = destinationsCtx
	a.pipelineProvider = pipelineProvider
	a.launchers = lnchrs
	a.health = health
	a.diagnosticMessageReceiver = diagnosticMessageReceiver

}

// buildEndpoints builds endpoints for the logs agent
func buildEndpoints(coreConfig pkgConfig.Reader) (*config.Endpoints, error) {
	httpConnectivity := config.HTTPConnectivityFailure
	if endpoints, err := config.BuildHTTPEndpointsWithVectorOverride(coreConfig, intakeTrackType, config.AgentJSONIntakeProtocol, config.DefaultIntakeOrigin); err == nil {
		httpConnectivity = http.CheckConnectivity(endpoints.Main, coreConfig)
	}
	return config.BuildEndpointsWithVectorOverride(coreConfig, httpConnectivity, intakeTrackType, config.AgentJSONIntakeProtocol, config.DefaultIntakeOrigin)
}
