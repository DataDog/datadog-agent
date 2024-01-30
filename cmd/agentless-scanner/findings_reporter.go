// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	logshttp "github.com/DataDog/datadog-agent/pkg/logs/client/http"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// LogReporter is responsible for sending compliance logs to Datadog backends.
type LogReporter struct {
	logSource        *sources.LogSource
	logChan          chan *message.Message
	endpoints        *config.Endpoints
	dstcontext       *client.DestinationsContext
	auditor          *auditor.RegistryAuditor
	pipelineProvider pipeline.Provider
}

func newFindingsReporter() (*LogReporter, error) {
	const intakeTrackType = "compliance"
	const sourceName = "compliance-agent"
	const sourceType = "compliance"
	const endpointPrefix = "cspm-intake."
	runPath := coreconfig.Datadog.GetString("compliance_config.run_path")

	health := health.RegisterLiveness(sourceType)

	logsConfigComplianceKeys := config.NewLogsConfigKeys("compliance_config.endpoints.", coreconfig.Datadog)
	endpoints, err := config.BuildHTTPEndpointsWithConfig(coreconfig.Datadog, logsConfigComplianceKeys, endpointPrefix, intakeTrackType, config.AgentJSONIntakeProtocol, config.DefaultIntakeOrigin)
	if err != nil {
		endpoints, err = config.BuildHTTPEndpoints(coreconfig.Datadog, intakeTrackType, config.AgentJSONIntakeProtocol, config.DefaultIntakeOrigin)
		if err == nil {
			httpConnectivity := logshttp.CheckConnectivity(endpoints.Main)
			endpoints, err = config.BuildEndpoints(coreconfig.Datadog, httpConnectivity, intakeTrackType, config.AgentJSONIntakeProtocol, config.DefaultIntakeOrigin)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("Invalid endpoints: %w", err)
	}

	dstcontext := client.NewDestinationsContext()
	dstcontext.Start()

	// setup the auditor
	auditor := auditor.New(runPath, sourceType+"-registry.json", coreconfig.DefaultAuditorTTL, health)
	auditor.Start()

	// setup the pipeline provider that provides pairs of processor and sender
	pipelineProvider := pipeline.NewProvider(config.NumberOfPipelines, auditor, &diagnostic.NoopMessageReceiver{}, nil, endpoints, dstcontext)
	pipelineProvider.Start()

	logSource := sources.NewLogSource(
		sourceName,
		&config.LogsConfig{
			Type:    sourceType,
			Service: sourceName,
			Source:  sourceName,
		},
	)
	logChan := pipelineProvider.NextPipelineChan()

	// merge tags from config

	reporter := &LogReporter{
		logSource:        logSource,
		logChan:          logChan,
		endpoints:        endpoints,
		dstcontext:       dstcontext,
		auditor:          auditor,
		pipelineProvider: pipelineProvider,
	}

	return reporter, nil
}

// Stop stops the LogReporter.
func (r *LogReporter) Stop() {
	r.dstcontext.Stop()
	r.auditor.Stop()
	r.pipelineProvider.Stop()
}

// ReportEvent reports event.
func (r *LogReporter) ReportEvent(event interface{}, tags ...string) {
	buf, err := json.Marshal(event)
	if err != nil {
		log.Errorf("failed to serialize compliance event: %v", err)
		return
	}
	origin := message.NewOrigin(r.logSource)
	origin.SetTags(tags)
	msg := message.NewMessage(buf, origin, message.StatusInfo, time.Now().UnixNano())
	r.logChan <- msg
}
