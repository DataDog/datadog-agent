// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build serverless

package agent

import (
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers/channel"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/schedulers"
	"github.com/DataDog/datadog-agent/pkg/status/health"
)

// Note: Building the logs-agent for serverless separately removes the
// dependency on autodiscovery, file launchers, and some schedulers
// thereby decreasing the binary size.

// NewAgent returns a Logs Agent instance to run in a serverless environment.
// The Serverless Logs Agent has only one input being the channel to receive the logs to process.
// It is using a NullAuditor because we've nothing to do after having sent the logs to the intake.
func (a *agent) SetupPipeline(
	processingRules []*config.ProcessingRule,
) {
	health := health.RegisterLiveness("logs-agent")

	diagnosticMessageReceiver := diagnostic.NewBufferedMessageReceiver(nil)

	// setup the a null auditor, not tracking data in any registry
	a.auditor = auditor.NewNullAuditor()
	destinationsCtx := client.NewDestinationsContext()

	// setup the pipeline provider that provides pairs of processor and sender
	pipelineProvider := pipeline.NewServerlessProvider(config.NumberOfPipelines, a.auditor, processingRules, a.endpoints, destinationsCtx)

	// setup the sole launcher for this agent
	lnchrs := launchers.NewLaunchers(a.sources, pipelineProvider, a.auditor, a.tracker)
	lnchrs.AddLauncher(channel.NewLauncher())

	a.schedulers = schedulers.NewSchedulers(a.sources, a.services)
	a.destinationsCtx = destinationsCtx
	a.pipelineProvider = pipelineProvider
	a.launchers = lnchrs
	a.health = health
	a.diagnosticMessageReceiver = diagnosticMessageReceiver
}

// buildEndpoints builds endpoints for the logs agent
func buildEndpoints(coreConfig pkgConfig.ConfigReader) (*config.Endpoints, error) {
	return config.BuildServerlessEndpoints(coreConfig, intakeTrackType, config.DefaultIntakeProtocol)
}
