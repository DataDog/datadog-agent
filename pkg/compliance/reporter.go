// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compliance

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs-library/pipeline"
	"github.com/DataDog/datadog-agent/comp/logs-library/sender"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/security/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// LogReporter is responsible for sending compliance logs to DataDog backends.
type LogReporter struct {
	hostname         string
	pipelineProvider pipeline.Provider
	logSource        *sources.LogSource
	logChan          chan *message.Message
	endpoints        *config.Endpoints
	tags             []string
}

// NewLogReporter instantiates a new log LogReporter
func NewLogReporter(hostname string, sourceName, sourceType string, endpoints *config.Endpoints, dstcontext *client.DestinationsContext, compression logscompression.Component) *LogReporter {
	// setup the pipeline provider that provides pairs of processor and sender
	cfg := pkgconfigsetup.Datadog()
	pipelineProvider := pipeline.NewProvider(
		4,
		&sender.NoopSink{},
		&diagnostic.NoopMessageReceiver{},
		nil, // processingRules
		endpoints,
		dstcontext,
		&common.NoopStatusProvider{},
		common.NewStaticHostnameService(hostname),
		cfg,
		compression,
		cfg.GetBool("logs_config.disable_distributed_senders"),
		false, // serverless
	)
	pipelineProvider.Start()

	logSource := sources.NewLogSource(
		sourceName,
		&config.LogsConfig{
			Type:   sourceType,
			Source: sourceName,
		},
	)
	logChan := pipelineProvider.NextPipelineChan()

	tags := []string{
		common.QueryAccountIDTag(),
		"host:" + hostname,
	}

	// merge tags from config
	for _, tag := range configUtils.GetConfiguredTags(pkgconfigsetup.Datadog(), true) {
		if strings.HasPrefix(tag, "host") {
			continue
		}
		tags = append(tags, tag)
	}

	return &LogReporter{
		hostname:         hostname,
		pipelineProvider: pipelineProvider,
		logSource:        logSource,
		logChan:          logChan,
		endpoints:        endpoints,
		tags:             tags,
	}
}

// Stop stops the LogReporter
func (r *LogReporter) Stop() {
	r.pipelineProvider.Stop()
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
