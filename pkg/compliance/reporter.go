// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compliance

import (
	"encoding/json"
	"time"

	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/security/common"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

type LogReporter struct {
	logSource *sources.LogSource
	logChan   chan *message.Message
	endpoints *config.Endpoints
	tags      []string
}

// NewLogReporter instantiates a new log LogReporter
func NewLogReporter(stopper startstop.Stopper, sourceName, sourceType, runPath string, endpoints *config.Endpoints, context *client.DestinationsContext) (*LogReporter, error) {
	health := health.RegisterLiveness(sourceType)

	// setup the auditor
	auditor := auditor.New(runPath, sourceType+"-registry.json", coreconfig.DefaultAuditorTTL, health)
	auditor.Start()

	// setup the pipeline provider that provides pairs of processor and sender
	pipelineProvider := pipeline.NewProvider(config.NumberOfPipelines, auditor, &diagnostic.NoopMessageReceiver{}, nil, endpoints, context)
	pipelineProvider.Start()

	stopper.Add(pipelineProvider)
	stopper.Add(auditor)

	logSource := sources.NewLogSource(
		sourceName,
		&config.LogsConfig{
			Type:    sourceType,
			Service: sourceName,
			Source:  sourceName,
		},
	)
	logChan := pipelineProvider.NextPipelineChan()
	tags := []string{common.QueryAccountIdTag()}

	return &LogReporter{
		logSource: logSource,
		logChan:   logChan,
		endpoints: endpoints,
		tags:      tags,
	}, nil
}

func (r *LogReporter) Endpoints() *config.Endpoints {
	return r.endpoints
}

func (r *LogReporter) ReportEvent(event interface{}) {
	buf, err := json.Marshal(event)
	if err != nil {
		log.Errorf("failed to serialize compliance event: %v", err)
		return
	}
	origin := message.NewOrigin(r.logSource)
	origin.SetTags(r.tags)
	msg := message.NewMessage(buf, origin, message.StatusInfo, time.Now().UnixNano(), "")
	r.logChan <- msg
}
