// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compliance

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/security/common"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// LogReporter is responsible for sending compliance logs to DataDog backends.
type LogReporter struct {
	hostname         string
	pipelineProvider pipeline.Provider
	auditor          *auditor.RegistryAuditor
	logSource        *sources.LogSource
	logChan          chan *message.Message
	endpoints        *config.Endpoints
	tags             []string
}

// NewLogReporter instantiates a new log LogReporter
func NewLogReporter(hostname string, sourceName, sourceType, runPath string, endpoints *config.Endpoints, dstcontext *client.DestinationsContext) *LogReporter {
	health := health.RegisterLiveness(sourceType)

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

	tags := []string{
		common.QueryAccountIDTag(),
		fmt.Sprintf("host:%s", hostname),
	}

	// merge tags from config
	for _, tag := range configUtils.GetConfiguredTags(coreconfig.Datadog, true) {
		if strings.HasPrefix(tag, "host") {
			continue
		}
		tags = append(tags, tag)
	}

	return &LogReporter{
		hostname:         hostname,
		pipelineProvider: pipelineProvider,
		auditor:          auditor,
		logSource:        logSource,
		logChan:          logChan,
		endpoints:        endpoints,
		tags:             tags,
	}
}

// Stop stops the LogReporter
func (r *LogReporter) Stop() {
	r.pipelineProvider.Stop()
	r.auditor.Stop()
}

// Endpoints returns the endpoints associated with the log reporter.
func (r *LogReporter) Endpoints() *config.Endpoints {
	return r.endpoints
}

// ReportEvent should be used to send an event to the backend.
func (r *LogReporter) ReportEvent(event interface{}) {
	buf, err := json.Marshal(event)
	if err != nil {
		log.Errorf("failed to serialize compliance event: %v", err)
		return
	}
	origin := message.NewOrigin(r.logSource)
	origin.SetTags(r.tags)
	msg := message.NewMessage(buf, origin, message.StatusInfo, time.Now().UnixNano())
	msg.Hostname = r.hostname
	r.logChan <- msg
}
