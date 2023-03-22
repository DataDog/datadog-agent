// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless
// +build !serverless

package logs

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery"
	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/client/http"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/launchers"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/launchers/container"
	filelauncher "github.com/DataDog/datadog-agent/pkg/logs/internal/launchers/file"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/launchers/journald"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/launchers/listener"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/launchers/windowsevent"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/schedulers"
	adScheduler "github.com/DataDog/datadog-agent/pkg/logs/schedulers/ad"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/status/health"
)

// NewAgent returns a new Logs Agent
func NewAgent(sources *sources.LogSources, services *service.Services, processingRules []*config.ProcessingRule, endpoints *config.Endpoints) *Agent {
	health := health.RegisterLiveness("logs-agent")

	// setup the auditor
	// We pass the health handle to the auditor because it's the end of the pipeline and the most
	// critical part. Arguably it could also be plugged to the destination.
	auditorTTL := time.Duration(coreConfig.Datadog.GetInt("logs_config.auditor_ttl")) * time.Hour
	auditor := auditor.New(coreConfig.Datadog.GetString("logs_config.run_path"), auditor.DefaultRegistryFilename, auditorTTL, health)
	destinationsCtx := client.NewDestinationsContext()
	diagnosticMessageReceiver := diagnostic.NewBufferedMessageReceiver()

	// setup the pipeline provider that provides pairs of processor and sender
	pipelineProvider := pipeline.NewProvider(config.NumberOfPipelines, auditor, diagnosticMessageReceiver, processingRules, endpoints, destinationsCtx)

	// setup the launchers
	lnchrs := launchers.NewLaunchers(sources, pipelineProvider, auditor)
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

// Start starts logs-agent
// getAC is a func returning the prepared AutoConfig. It is nil until
// the AutoConfig is ready, please consider using BlockUntilAutoConfigRanOnce
// instead of directly using it.
func Start(ac *autodiscovery.AutoConfig) (*Agent, error) {
	agent, err := start()
	if err != nil {
		return nil, err
	}
	if ac == nil {
		panic("AutoConfig must be initialized before logs-agent")
	}
	agent.AddScheduler(adScheduler.New(ac))
	return agent, nil
}

// buildEndpoints builds endpoints for the logs agent
func buildEndpoints() (*config.Endpoints, error) {
	httpConnectivity := config.HTTPConnectivityFailure
	if endpoints, err := config.BuildHTTPEndpointsWithVectorOverride(intakeTrackType, AgentJSONIntakeProtocol, config.DefaultIntakeOrigin); err == nil {
		httpConnectivity = http.CheckConnectivity(endpoints.Main)
	}
	return config.BuildEndpointsWithVectorOverride(httpConnectivity, intakeTrackType, AgentJSONIntakeProtocol, config.DefaultIntakeOrigin)
}
