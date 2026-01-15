// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build serverless

package agentimpl

import (
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers/channel"
	filelauncher "github.com/DataDog/datadog-agent/pkg/logs/launchers/file"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/schedulers"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers/file"
	"github.com/DataDog/datadog-agent/pkg/logs/types"
	"github.com/DataDog/datadog-agent/pkg/logs/util/opener"
	"github.com/DataDog/datadog-agent/pkg/serverless/streamlogs"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Note: Building the logs-agent for serverless separately removes the
// dependency on autodiscovery, file launchers, and some schedulers
// thereby decreasing the binary size.

// SetupPipeline returns a Logs Agent instance to run in a serverless environment.
// The Serverless Logs Agent has only one input being the channel to receive the logs to process.
// It is using a NullAuditor because we've nothing to do after having sent the logs to the intake.
func (a *logAgent) SetupPipeline(
	processingRules []*config.ProcessingRule,
	wmeta option.Option[workloadmeta.Component],
	_ integrations.Component,
	fingerprintConfig types.FingerprintConfig,
) {
	diagnosticMessageReceiver := diagnostic.NewBufferedMessageReceiver(streamlogs.Formatter{}, a.hostname)
	destinationsCtx := client.NewDestinationsContext()

	// setup the pipeline provider that provides pairs of processor and sender
	pipelineProvider := pipeline.NewProvider(
		a.config.GetInt("logs_config.pipelines"),
		a.auditor,
		diagnosticMessageReceiver,
		processingRules, a.endpoints,
		destinationsCtx,
		NewStatusProvider(),
		a.hostname,
		a.config,
		a.compression,
		true, // disable distributed sending for serverless
		true, // serverless
	)

	lnchrs := launchers.NewLaunchers(a.sources, pipelineProvider, a.auditor, a.tracker)
	lnchrs.AddLauncher(channel.NewLauncher())

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
	a.schedulers = schedulers.NewSchedulers(a.sources, a.services)
	a.destinationsCtx = destinationsCtx
	a.pipelineProvider = pipelineProvider
	a.launchers = lnchrs
	a.diagnosticMessageReceiver = diagnosticMessageReceiver
}

// buildEndpoints builds endpoints for the logs agent
func buildEndpoints(coreConfig model.Reader) (*config.Endpoints, error) {
	config, err := config.BuildServerlessEndpoints(coreConfig, intakeTrackType, config.DefaultIntakeProtocol)
	if err != nil {
		return nil, err
	}
	return config, nil
}
