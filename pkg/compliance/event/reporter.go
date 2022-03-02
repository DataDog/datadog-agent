// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package event

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
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

// Reporter defines an interface for reporting rule events
type Reporter interface {
	Report(event *Event)
	ReportRaw(content []byte, service string, tags ...string)
}

type reporter struct {
	logSource *config.LogSource
	logChan   chan *message.Message
}

// NewLogReporter instantiates a new log reporter
func NewLogReporter(stopper startstop.Stopper, sourceName, sourceType, runPath string, endpoints *config.Endpoints, context *client.DestinationsContext) (Reporter, error) {
	health := health.RegisterLiveness(sourceType)

	// setup the auditor
	auditor := auditor.New(runPath, sourceType+"-registry.json", coreconfig.DefaultAuditorTTL, health)
	auditor.Start()

	// setup the pipeline provider that provides pairs of processor and sender
	pipelineProvider := pipeline.NewProvider(config.NumberOfPipelines, auditor, &diagnostic.NoopMessageReceiver{}, nil, endpoints, context)
	pipelineProvider.Start()

	stopper.Add(pipelineProvider)
	stopper.Add(auditor)

	logSource := config.NewLogSource(
		sourceName,
		&config.LogsConfig{
			Type:    sourceType,
			Service: sourceName,
			Source:  sourceName,
		},
	)

	return NewReporter(logSource, pipelineProvider.NextPipelineChan()), nil
}

// NewReporter returns an instance of Reporter
func NewReporter(logSource *config.LogSource, logChan chan *message.Message) Reporter {
	return &reporter{
		logSource: logSource,
		logChan:   logChan,
	}
}

func (r *reporter) Report(event *Event) {
	buf, err := json.Marshal(event)
	if err != nil {
		log.Errorf("Failed to serialize rule event for rule %s", event.AgentRuleID)
		return
	}
	r.ReportRaw(buf, "")
}

func (r *reporter) ReportRaw(content []byte, service string, tags ...string) {
	origin := message.NewOrigin(r.logSource)
	origin.SetTags(tags)
	origin.SetService(service)
	msg := message.NewMessage(content, origin, message.StatusInfo, time.Now().UnixNano())
	r.logChan <- msg
}
