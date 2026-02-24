// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package reporter holds reporter related files
package reporter

import (
	"sync"
	"time"

	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	compression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	seccommon "github.com/DataDog/datadog-agent/pkg/security/common"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

// RuntimeReporter represents a CWS reporter, used to send events to the intake
type RuntimeReporter struct {
	hostname  string
	logSource *sources.LogSource
	logChan   chan *message.Message
	done      chan struct{}
	once      sync.Once
}

// ReportRaw reports raw (bytes) events to the intake
func (r *RuntimeReporter) ReportRaw(content []byte, service string, timestamp time.Time, tags ...string) {
	origin := message.NewOrigin(r.logSource)
	origin.SetTags(tags)
	origin.SetService(service)
	msg := message.NewMessage(content, origin, message.StatusInfo, timestamp.UnixNano())
	msg.Hostname = r.hostname

	select {
	case r.logChan <- msg:
	case <-r.done:
	}
}

// Stop signals the reporter to stop accepting new messages.
// Must be called before the underlying pipeline is stopped.
func (r *RuntimeReporter) Stop() {
	r.once.Do(func() { close(r.done) })
}

// NewCWSReporter returns a new CWS reported based on the fields necessary to communicate with the intake
func NewCWSReporter(hostname string, stopper startstop.Stopper, endpoints *logsconfig.Endpoints, context *client.DestinationsContext, compression compression.Component) (seccommon.RawReporter, error) {
	return newReporter(hostname, stopper, "runtime-security-agent", "runtime-security", endpoints, context, compression)
}

func newReporter(hostname string, stopper startstop.Stopper, sourceName, sourceType string, endpoints *logsconfig.Endpoints, context *client.DestinationsContext, compression compression.Component) (seccommon.RawReporter, error) {
	// setup the pipeline provider that provides pairs of processor and sender
	cfg := pkgconfigsetup.Datadog()
	pipelineProvider := pipeline.NewProvider(
		4,
		&sender.NoopSink{},
		&diagnostic.NoopMessageReceiver{},
		nil,
		endpoints,
		context,
		&seccommon.NoopStatusProvider{},
		seccommon.NewStaticHostnameService(hostname),
		cfg,
		compression,
		cfg.GetBool("logs_config.disable_distributed_senders"),
		false, // serverless
	)
	pipelineProvider.Start()

	logSource := sources.NewLogSource(
		sourceName,
		&logsconfig.LogsConfig{
			Type:   sourceType,
			Source: sourceName,
		},
	)
	logChan := pipelineProvider.NextPipelineChan()
	r := &RuntimeReporter{
		hostname:  hostname,
		logSource: logSource,
		logChan:   logChan,
		done:      make(chan struct{}),
	}

	// Stop the reporter before the pipeline to ensure no sends happen on the
	// closed pipeline channel.
	stopper.Add(r)
	stopper.Add(pipelineProvider)

	return r, nil
}
