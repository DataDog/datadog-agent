// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package reporter

import (
	"time"

	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	logsconfig "github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	seccommon "github.com/DataDog/datadog-agent/pkg/security/common"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

type RuntimeReporter struct {
	logSource *sources.LogSource
	logChan   chan *message.Message
}

func (r *RuntimeReporter) ReportRaw(content []byte, service string, tags ...string) {
	origin := message.NewOrigin(r.logSource)
	origin.SetTags(tags)
	origin.SetService(service)
	msg := message.NewMessage(content, origin, message.StatusInfo, time.Now().UnixNano())
	r.logChan <- msg
}

func NewCWSReporter(runPath string, stopper startstop.Stopper, endpoints *logsconfig.Endpoints, context *client.DestinationsContext) (seccommon.RawReporter, error) {
	return newReporter(runPath, stopper, "runtime-security-agent", "runtime-security", endpoints, context)
}

func newReporter(runPath string, stopper startstop.Stopper, sourceName, sourceType string, endpoints *logsconfig.Endpoints, context *client.DestinationsContext) (seccommon.RawReporter, error) {
	health := health.RegisterLiveness("runtime-security")

	// setup the auditor
	auditor := auditor.New(runPath, "runtime-security-registry.json", pkgconfig.DefaultAuditorTTL, health)
	auditor.Start()
	stopper.Add(auditor)

	// setup the pipeline provider that provides pairs of processor and sender
	pipelineProvider := pipeline.NewProvider(logsconfig.NumberOfPipelines, auditor, &diagnostic.NoopMessageReceiver{}, nil, endpoints, context)
	pipelineProvider.Start()
	stopper.Add(pipelineProvider)

	logSource := sources.NewLogSource(
		sourceName,
		&logsconfig.LogsConfig{
			Type:   sourceType,
			Source: sourceName,
		},
	)
	logChan := pipelineProvider.NextPipelineChan()
	return &RuntimeReporter{
		logSource: logSource,
		logChan:   logChan,
	}, nil
}
