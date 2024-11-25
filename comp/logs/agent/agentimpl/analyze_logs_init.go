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
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers"
	filelauncher "github.com/DataDog/datadog-agent/pkg/logs/launchers/file"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// SetUpLaunchers creates intializes the launcher. The launchers schedule the tailers to read the log files provided by the analyze-logs command
func SetUpLaunchers(conf configComponent.Component) chan *message.Message {
	processingRules, err := config.GlobalProcessingRules(conf)
	if err != nil {
		log.Errorf("Error while getting processing rules from config: %v", err)
		return nil
	}

	diagnosticMessageReceiver := diagnostic.NewBufferedMessageReceiver(nil, nil)
	pipelineProvider := pipeline.NewProcessorOnlyProvider(diagnosticMessageReceiver, processingRules, conf, nil)
	// setup the launchers

	lnchrs := launchers.NewLaunchers(nil, pipelineProvider, nil, nil)
	fileLimits := 500
	fileValidatePodContainer := false
	fileScanPeriod := time.Duration(0.5 * float64(time.Second))
	fileWildcardSelectionMode := "by_name"
	fileLauncher := filelauncher.NewLauncher(
		fileLimits,
		filelauncher.DefaultSleepDuration,
		fileValidatePodContainer,
		fileScanPeriod,
		fileWildcardSelectionMode,
		flare.NewFlareController(),
		nil)
	tracker := tailers.NewTailerTracker()

	DefaultAuditorTTL := 23
	defaultRunPath := "/opt/datadog-agent/run"
	health := health.RegisterLiveness("logs-agent")

	auditorTTL := time.Duration(DefaultAuditorTTL) * time.Hour
	auditor := auditor.New(defaultRunPath, auditor.DefaultRegistryFilename, auditorTTL, health)

	pipelineProvider.Start()
	sourceProvider := sources.GetInstance()
	fileLauncher.Start(sourceProvider, pipelineProvider, auditor, tracker)

	lnchrs.AddLauncher(fileLauncher)
	outputChan := pipelineProvider.GetOutputChan()
	return outputChan
}
