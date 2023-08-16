// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package logs

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
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
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
	"github.com/DataDog/datadog-agent/pkg/status/health"
)

// NewAgent returns a new Logs Agent
func NewAgent(sources *sources.LogSources, services *service.Services, tracker *tailers.TailerTracker, processingRules []*config.ProcessingRule, endpoints *config.Endpoints) *Agent {
	health := health.RegisterLiveness("logs-agent")

	// setup the auditor
	// We pass the health handle to the auditor because it's the end of the pipeline and the most
	// critical part. Arguably it could also be plugged to the destination.
	auditorTTL := time.Duration(coreConfig.Datadog.GetInt("logs_config.auditor_ttl")) * time.Hour
	auditor := auditor.New(coreConfig.Datadog.GetString("logs_config.run_path"), auditor.DefaultRegistryFilename, auditorTTL, health)
	destinationsCtx := client.NewDestinationsContext()
	diagnosticMessageReceiver := diagnostic.NewBufferedMessageReceiver(nil)

	// setup the pipeline provider that provides pairs of processor and sender
	pipelineProvider := pipeline.NewProvider(config.NumberOfPipelines, auditor, diagnosticMessageReceiver, processingRules, endpoints, destinationsCtx)

	// setup the launchers
	lnchrs := launchers.NewLaunchers(sources, pipelineProvider, auditor, tracker)
	lnchrs.AddLauncher(filelauncher.NewLauncher(
		coreConfig.Datadog.GetInt("logs_config.open_files_limit"),
		filelauncher.DefaultSleepDuration,
		coreConfig.Datadog.GetBool("logs_config.validate_pod_container_id"),
		time.Duration(coreConfig.Datadog.GetFloat64("logs_config.file_scan_period")*float64(time.Second)),
		coreConfig.Datadog.GetString("logs_config.file_wildcard_selection_mode")))
	lnchrs.AddLauncher(listener.NewLauncher(coreConfig.Datadog.GetInt("logs_config.frame_size")))
	lnchrs.AddLauncher(journald.NewLauncher())
	lnchrs.AddLauncher(windowsevent.NewLauncher())
	lnchrs.AddLauncher(container.NewLauncher(sources))

	return &Agent{
		sources:                   sources,
		services:                  services,
		schedulers:                schedulers.NewSchedulers(sources, services),
		auditor:                   auditor,
		destinationsCtx:           destinationsCtx,
		pipelineProvider:          pipelineProvider,
		launchers:                 lnchrs,
		health:                    health,
		diagnosticMessageReceiver: diagnosticMessageReceiver,
	}
}

// buildEndpoints builds endpoints for the logs agent
func buildEndpoints() (*config.Endpoints, error) {
	httpConnectivity := config.HTTPConnectivityFailure
	if endpoints, err := config.BuildHTTPEndpointsWithVectorOverride(coreConfig.Datadog, intakeTrackType, AgentJSONIntakeProtocol, config.DefaultIntakeOrigin); err == nil {
		httpConnectivity = http.CheckConnectivity(endpoints.Main)
	}
	return config.BuildEndpointsWithVectorOverride(coreConfig.Datadog, httpConnectivity, intakeTrackType, AgentJSONIntakeProtocol, config.DefaultIntakeOrigin)
}
