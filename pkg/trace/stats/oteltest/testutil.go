// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package oteltest

import (
	"context"
	"net/http"
	"runtime"
	"sync"
	"time"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	traceconfig "github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/stats"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/trace/timing"
	"github.com/DataDog/datadog-go/v5/statsd"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// This is a copy of https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/internal/datadog/agent.go and
// https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/internal/datadog/metrics_client.go.
// This fake agent is only used in tests.

// traceAgent specifies a minimal trace agent instance that is able to process traces and output stats.
type traceAgent struct {
	*agent.Agent

	// pchan specifies the channel that will be used to output Datadog Trace Agent API Payloads
	// resulting from ingested OpenTelemetry spans.
	pchan chan *api.Payload

	// wg waits for all goroutines to exit.
	wg sync.WaitGroup

	// exit signals the agent to shut down.
	exit chan struct{}
}

// newAgentWithConfig creates a new traceagent with the given config cfg. Used in tests; use newAgent instead.
func newAgentWithConfig(ctx context.Context, cfg *traceconfig.AgentConfig, out chan *pb.StatsPayload, now time.Time) *traceAgent {
	// disable the HTTP receiver
	cfg.ReceiverPort = 0
	// set the API key to succeed startup; it is never used nor needed
	cfg.Endpoints[0].APIKey = "skip_check"
	// set the default hostname to the translator's placeholder; in the case where no hostname
	// can be deduced from incoming traces, we don't know the default hostname (because it is set
	// in the exporter). In order to avoid duplicating the hostname setting in the processor and
	// exporter, we use a placeholder and fill it in later (in the Datadog Exporter or Agent OTLP
	// Ingest). This gives a better user experience.
	cfg.Hostname = "__unset__"
	pchan := make(chan *api.Payload, 1000)
	metricsClient := &statsd.NoOpClient{}
	a := agent.NewAgent(ctx, cfg, telemetry.NewNoopCollector(), metricsClient)
	// replace the Concentrator (the component which computes and flushes APM Stats from incoming
	// traces) with our own, which uses the 'out' channel.
	a.Concentrator = stats.NewConcentrator(cfg, out, now, metricsClient)
	// ...and the same for the ClientStatsAggregator; we don't use it here, but it is also a source
	// of stats which should be available to us.
	a.ClientStatsAggregator = stats.NewClientStatsAggregator(cfg, out, metricsClient)
	// lastly, initialize the OTLP receiver, which will be used to introduce ResourceSpans into the traceagent,
	// so that we can transform them to Datadog spans and receive stats.
	timingReporter := timing.New(metricsClient)
	a.OTLPReceiver = api.NewOTLPReceiver(pchan, cfg, metricsClient, timingReporter)
	return &traceAgent{
		Agent: a,
		exit:  make(chan struct{}),
		pchan: pchan,
	}
}

// Start starts the traceagent, making it ready to ingest spans.
func (p *traceAgent) Start() {
	// we don't need to start the full agent, so we only start a set of minimal
	// components needed to compute stats:
	for _, starter := range []interface{ Start() }{
		p.Concentrator,
		p.ClientStatsAggregator,
		// we don't need the samplers' nor the processor's functionalities;
		// but they are used by the agent nevertheless, so they need to be
		// active and functioning.
		p.PrioritySampler,
		p.ErrorsSampler,
		p.NoPrioritySampler,
		p.EventProcessor,
	} {
		starter.Start()
	}

	p.goDrain()
	p.goProcess()
}

// Stop stops the traceagent, making it unable to ingest spans. Do not call Ingest after Stop.
func (p *traceAgent) Stop() {
	for _, stopper := range []interface{ Stop() }{
		p.Concentrator,
		p.ClientStatsAggregator,
		p.PrioritySampler,
		p.ErrorsSampler,
		p.NoPrioritySampler,
		p.EventProcessor,
	} {
		stopper.Stop()
	}
	close(p.exit)
	p.wg.Wait()
}

// goDrain drains the TraceWriter channel, ensuring it won't block. We don't need the traces,
// nor do we have a running TraceWrite. We just want the outgoing stats.
func (p *traceAgent) goDrain() {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		for {
			select {
			case <-p.TraceWriter.In:
				// we don't write these traces anywhere; drain the channel
			case <-p.exit:
				return
			}
		}
	}()
}

// Ingest processes the given spans within the traceagent and outputs stats through the output channel
// provided to newAgent. Do not call Ingest on an unstarted or stopped traceagent.
func (p *traceAgent) Ingest(ctx context.Context, traces ptrace.Traces) {
	rspanss := traces.ResourceSpans()
	for i := 0; i < rspanss.Len(); i++ {
		rspans := rspanss.At(i)
		p.OTLPReceiver.ReceiveResourceSpans(ctx, rspans, http.Header{})
		// ...the call transforms the OTLP Spans into a Datadog payload and sends the result
		// down the p.pchan channel

	}
}

// goProcesses runs the main loop which takes incoming payloads, processes them and generates stats.
// It then picks up those stats and converts them to metrics.
func (p *traceAgent) goProcess() {
	for i := 0; i < runtime.NumCPU(); i++ {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			for {
				select {
				case payload := <-p.pchan:
					p.Process(payload)
					// ...the call processes the payload and outputs stats via the 'out' channel
					// provided to newAgent
				case <-p.exit:
					return
				}
			}
		}()
	}
}
