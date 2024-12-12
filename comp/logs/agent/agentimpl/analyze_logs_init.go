// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package agentimpl

import (
	"time"

	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/comp/logs/agent/flare"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers"
	filelauncher "github.com/DataDog/datadog-agent/pkg/logs/launchers/file"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// SetUpLaunchers creates intializes the launcher. The launchers schedule the tailers to read the log files provided by the analyze-logs command
func SetUpLaunchers(conf configComponent.Component, sourceProvider *sources.ConfigSources) (chan *message.Message, *launchers.Launchers, pipeline.Provider) {
	processingRules, err := config.GlobalProcessingRules(conf)
	if err != nil {
		log.Errorf("Error while getting processing rules from config: %v", err)
		return nil, nil, nil
	}

	diagnosticMessageReceiver := diagnostic.NewBufferedMessageReceiver(nil, nil)
	pipelineProvider := pipeline.NewProcessorOnlyProvider(diagnosticMessageReceiver, processingRules, conf, nil)

	// setup the launchers
	lnchrs := launchers.NewLaunchers(nil, pipelineProvider, nil, nil)
	fileLimits := pkgconfigsetup.Datadog().GetInt("logs_config.open_files_limit")
	fileValidatePodContainer := pkgconfigsetup.Datadog().GetBool("logs_config.validate_pod_container_id")
	fileScanPeriod := time.Duration(pkgconfigsetup.Datadog().GetFloat64("logs_config.file_scan_period") * float64(time.Second))
	fileWildcardSelectionMode := pkgconfigsetup.Datadog().GetString("logs_config.file_wildcard_selection_mode")
	fileLauncher := filelauncher.NewLauncher(
		fileLimits,
		filelauncher.DefaultSleepDuration,
		fileValidatePodContainer,
		fileScanPeriod,
		fileWildcardSelectionMode,
		flare.NewFlareController(),
		nil)
	tracker := tailers.NewTailerTracker()

	a := auditor.NewNullAuditor()
	pipelineProvider.Start()
	fileLauncher.Start(sourceProvider, pipelineProvider, a, tracker)
	lnchrs.AddLauncher(fileLauncher)
	outputChan := pipelineProvider.GetOutputChan()
	return outputChan, lnchrs, pipelineProvider
}
