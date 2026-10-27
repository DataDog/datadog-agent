// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package pipeline

import (
	"context"
	"errors"
	"fmt"
	"sync"

	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	"github.com/DataDog/datadog-agent/comp/logs-library/client"
	httpclient "github.com/DataDog/datadog-agent/comp/logs-library/client/http"
	"github.com/DataDog/datadog-agent/comp/logs-library/diagnostic"
	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs-library/processor"
	"github.com/DataDog/datadog-agent/comp/logs-library/routing"
	"github.com/DataDog/datadog-agent/comp/logs-library/sender"
	httpsender "github.com/DataDog/datadog-agent/comp/logs-library/sender/http"
	"github.com/DataDog/datadog-agent/comp/logs-library/tagfilter"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/def"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	compressioncommon "github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	routedIngressCapacity   = 256
	routedProcessedCapacity = 256
	routedStreamCapacity    = 128
	routedPayloadCapacity   = 4
	routedOutcomeCapacity   = 128
	routedProgressCapacity  = 4096
	routedDestinationBuffer = 128
	routedHTTPRequests      = 64
)

// RoutedOptions is the injection boundary for the normal HTTP routing
// mechanism. No Agent startup path constructs this provider automatically.
type RoutedOptions struct {
	NumberOfPipelines         int
	Sink                      sender.Sink
	DiagnosticMessageReceiver diagnostic.MessageReceiver
	ProcessingRules           []*config.ProcessingRule
	TagFilters                *tagfilter.Filters
	Groups                    []routing.Group
	Resolve                   func(*message.Message) (routing.Selection, error)
	OnError                   func(error)
	DestinationsContext       *client.DestinationsContext
	Hostname                  hostnameinterface.Component
	Config                    pkgconfigmodel.Reader
	Compression               logscompression.Component
	Secrets                   secrets.Component
}

type routedStream struct {
	groups   []string
	input    chan *message.Message
	output   chan *message.Payload
	flush    chan struct{}
	strategy sender.Strategy
}

type RoutedProvider struct {
	opts          RoutedOptions
	defs          *routing.Definitions
	progress      *routing.Progress
	monitor       metrics.PipelineMonitor
	ingress       chan *message.Message
	processed     chan *message.Message
	processors    []*processor.Processor
	streams       map[string]*routedStream
	senders       map[string]*sender.Sender
	sinks         map[string]*routing.GroupSink
	ctx           context.Context
	cancel        context.CancelFunc
	ingressDone   chan struct{}
	processedDone chan struct{}
	dispatchDone  sync.WaitGroup
	lifecycle     sync.RWMutex
	stopOnce      sync.Once
	stopRequested chan struct{}
	started       bool
	stopped       bool
}

// NewRoutedProvider validates injected groups before constructing workers. It
// reads the group limit once at startup and leaves the default provider alone.
func NewRoutedProvider(opts RoutedOptions) (*RoutedProvider, error) {
	if opts.Config == nil || opts.Sink == nil || opts.Resolve == nil || opts.Compression == nil || opts.DestinationsContext == nil || opts.Secrets == nil || opts.DiagnosticMessageReceiver == nil || opts.NumberOfPipelines < 1 || opts.NumberOfPipelines > 16 {
		return nil, errors.New("invalid routed provider dependencies")
	}
	defs, err := routing.NewDefinitions(opts.Groups, opts.Config.GetInt("logs_config.routing.max_groups"))
	if err != nil {
		return nil, err
	}
	progress, err := routing.NewProgress(routedProgressCapacity, opts.Sink)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &RoutedProvider{
		opts: opts, defs: defs, progress: progress,
		ingress:   make(chan *message.Message, routedIngressCapacity),
		processed: make(chan *message.Message, routedProcessedCapacity),
		streams:   make(map[string]*routedStream), senders: make(map[string]*sender.Sender),
		sinks: make(map[string]*routing.GroupSink), ctx: ctx, cancel: cancel,
		ingressDone: make(chan struct{}), processedDone: make(chan struct{}),
		stopRequested: make(chan struct{}),
	}
	endpointCount := 0
	for _, group := range defs.Groups() {
		endpointCount += len(group.Endpoints.Endpoints)
	}
	perEndpointCap := routedHTTPRequests / endpointCount
	if perEndpointCap < 1 {
		perEndpointCap = 1
	}
	bufferSize := opts.Config.GetInt("logs_config.payload_channel_size")
	if bufferSize < 1 {
		bufferSize = 1
	} else if bufferSize > routedDestinationBuffer {
		bufferSize = routedDestinationBuffer
	}
	for _, group := range defs.Groups() {
		groupSink := progress.NewGroupSink(group.ID, routedOutcomeCapacity)
		p.sinks[group.ID] = groupSink
		// One sender and its reliable-first optional endpoint behavior per group.
		// The cap includes every primary and optional endpoint across all groups.
		concurrency := maxConcurrencyPerPipeline
		if group.Endpoints.BatchMaxConcurrentSend > 0 {
			concurrency = group.Endpoints.BatchMaxConcurrentSend
		}
		if concurrency < 1 {
			concurrency = 1
		} else if concurrency > perEndpointCap {
			concurrency = perEndpointCap
		}
		p.senders[group.ID] = httpsender.NewHTTPSender(opts.Config, groupSink, bufferSize, sender.NewServerlessMeta(false), group.Endpoints, opts.DestinationsContext, "logs-routing-"+group.ID, httpclient.JSONContentType, "", sender.DefaultQueuesCount, sender.DefaultWorkersPerQueue, concurrency, concurrency, opts.Secrets, metrics.NewTelemetryPipelineMonitor())
		key := group.Stream
		if key == "" {
			key = "group:" + group.ID
		} else {
			key = "stream:" + key
		}
		stream := p.streams[key]
		if stream == nil {
			stream = &routedStream{input: make(chan *message.Message, routedStreamCapacity), output: make(chan *message.Payload, routedPayloadCapacity), flush: make(chan struct{})}
			p.streams[key] = stream
		}
		stream.groups = append(stream.groups, group.ID)
	}
	for key, stream := range p.streams {
		first, _ := defs.Group(stream.groups[0])
		endpoints := first.Endpoints
		compressor := opts.Compression.NewCompressor(compressioncommon.NoneKind, 0)
		if endpoints.Main.UseCompression {
			compressor = opts.Compression.NewCompressor(endpoints.Main.CompressionKind, endpoints.Main.CompressionLevel)
		}
		groups := append([]string(nil), stream.groups...)
		stream.strategy = sender.NewBatchStrategyWithTerminalLoss(stream.input, stream.output, stream.flush, sender.NewServerlessMeta(false), endpoints.BatchWait, endpoints.BatchMaxSize, endpoints.BatchMaxContentSize, "logs-routing", compressor, p.senders[groups[0]].PipelineMonitor(), key, func(metas []*message.MessageMetadata) {
			payload := message.NewPayload(metas, nil, "", 0)
			for _, id := range groups {
				p.progress.CompleteGroup(id, payload)
			}
		})
	}
	p.monitor = p.senders[defs.Groups()[0].ID].PipelineMonitor()
	for i := 0; i < opts.NumberOfPipelines; i++ {
		input := make(chan *message.Message, routedStreamCapacity)
		proc := processor.NewForRouting(opts.Config, input, p.processed, opts.ProcessingRules, opts.TagFilters, processor.NewJSONEncoder(opts.Config.GetBool("logs_config.use_container_timestamp")), opts.DiagnosticMessageReceiver, opts.Hostname, p.monitor, fmt.Sprintf("routed-%d", i), func(msg *message.Message) {
			p.progress.CompleteLocal(msg.RoutingToken())
		})
		p.processors = append(p.processors, proc)
	}
	return p, nil
}

func (p *RoutedProvider) report(err error) {
	if p.opts.OnError != nil {
		p.opts.OnError(err)
	} else {
		log.Warnf("logs routing: %v", err)
	}
}

func (p *RoutedProvider) Start() {
	p.lifecycle.Lock()
	defer p.lifecycle.Unlock()
	if p.started || p.stopped {
		return
	}
	if p.opts.DestinationsContext.Context() == nil {
		p.opts.DestinationsContext.Start()
	}
	p.started = true
	go func() {
		select {
		case <-p.opts.DestinationsContext.Context().Done():
			p.cancel()
		case <-p.ctx.Done():
		}
	}()
	for _, group := range p.defs.Groups() {
		p.senders[group.ID].Start()
	}
	for _, stream := range p.streams {
		stream.strategy.Start()
		p.dispatchDone.Add(1)
		go p.dispatchPayloads(stream)
	}
	for _, proc := range p.processors {
		proc.Start()
	}
	go p.dispatchProcessed()
	go p.admit()
}

func (p *RoutedProvider) admit() {
	defer close(p.ingressDone)
	var index uint64
	for msg := range p.ingress {
		if done := msg.RoutingBarrier(); done != nil {
			p.flushPrior(done)
			continue
		}
		if p.ctx.Err() != nil {
			continue
		}
		selection, err := p.opts.Resolve(msg)
		if err != nil {
			p.report(err)
			if holdErr := p.progress.HoldInvalid(p.ctx, msg); holdErr != nil {
				p.report(holdErr)
			}
			continue
		}
		groups, err := p.defs.Resolve(selection)
		if err != nil {
			p.report(err)
			if holdErr := p.progress.HoldInvalid(p.ctx, msg); holdErr != nil {
				p.report(holdErr)
			}
			continue
		}
		if _, err := p.progress.Register(p.ctx, msg, selection, groups); err != nil {
			p.report(err)
			if holdErr := p.progress.HoldInvalid(p.ctx, msg); holdErr != nil {
				p.report(holdErr)
			}
			continue
		}
		if selection.Excluded {
			continue
		}
		proc := p.processors[index%uint64(len(p.processors))]
		index++
		select {
		case proc.InputChan() <- msg:
		case <-p.ctx.Done():
			// The reservation remains unresolved and will be replayed from the
			// last safe checkpoint after restart.
		}
	}
}

// flushPrior sends ordered markers through every processing lane and stream.
// Each marker can pass only after all earlier records in that stage.
func (p *RoutedProvider) flushPrior(done chan struct{}) {
	defer close(done)
	var acknowledgements []chan struct{}
	for _, proc := range p.processors {
		ack := make(chan struct{})
		marker := &message.Message{}
		marker.SetRoutingBarrier(ack)
		select {
		case proc.InputChan() <- marker:
			acknowledgements = append(acknowledgements, ack)
		case <-p.ctx.Done():
			return
		}
	}
	for _, ack := range acknowledgements {
		select {
		case <-ack:
		case <-p.ctx.Done():
			return
		}
	}
	acknowledgements = acknowledgements[:0]
	for _, stream := range p.streams {
		ack := make(chan struct{})
		marker := &message.Message{}
		marker.SetRoutingBarrier(ack)
		select {
		case stream.input <- marker:
			acknowledgements = append(acknowledgements, ack)
		case <-p.ctx.Done():
			return
		}
	}
	for _, ack := range acknowledgements {
		select {
		case <-ack:
		case <-p.ctx.Done():
			return
		}
	}
}

func (p *RoutedProvider) dispatchProcessed() {
	defer close(p.processedDone)
	for msg := range p.processed {
		if done := msg.RoutingBarrier(); done != nil {
			close(done)
			continue
		}
		groups := p.progress.Recipients(msg.RoutingToken())
		seen := make(map[string]struct{}, len(groups))
		for _, id := range groups {
			group, _ := p.defs.Group(id)
			key := group.Stream
			if key == "" {
				key = "group:" + id
			} else {
				key = "stream:" + key
			}
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			select {
			case p.streams[key].input <- msg:
			case <-p.ctx.Done():
				// No completion follows cancellation or queue rejection.
			}
		}
	}
}

func (p *RoutedProvider) dispatchPayloads(stream *routedStream) {
	defer p.dispatchDone.Done()
	for payload := range stream.output {
		for _, id := range stream.groups {
			copyForGroup := *payload // Encoded remains immutable and shared.
			select {
			case p.senders[id].In() <- &copyForGroup:
			case <-p.ctx.Done():
				// Unsent obligations remain pending.
			}
		}
	}
}

func (p *RoutedProvider) Stop() {
	p.stopOnce.Do(func() { close(p.stopRequested) })
	p.lifecycle.Lock()
	if !p.started {
		if !p.stopped {
			p.stopped = true
			close(p.ingress)
			for _, sink := range p.sinks {
				sink.Stop()
			}
			p.progress.Close()
			p.cancel()
		}
		p.lifecycle.Unlock()
		return
	}
	p.started = false
	p.stopped = true
	close(p.ingress)
	p.lifecycle.Unlock()
	<-p.ingressDone
	for _, proc := range p.processors {
		proc.Stop()
	}
	close(p.processed)
	<-p.processedDone
	for _, stream := range p.streams {
		stream.strategy.Stop()
		close(stream.output)
	}
	p.dispatchDone.Wait()
	for _, group := range p.defs.Groups() {
		p.senders[group.ID].Stop()
		p.sinks[group.ID].Stop()
	}
	p.progress.Close()
	p.cancel()
}

func (p *RoutedProvider) NextPipelineChan() chan *message.Message { return p.ingress }

// GuardRegistry returns the registry to give routed source launchers. It
// prevents source-owned writes from bypassing outstanding group obligations.
func (p *RoutedProvider) GuardRegistry(registry auditor.Registry) auditor.Registry {
	return p.progress.GuardRegistry(registry)
}
func (p *RoutedProvider) GetOutputChan() chan *message.Message { return nil }
func (p *RoutedProvider) NextPipelineChanWithMonitor() (chan *message.Message, *metrics.CapacityMonitor) {
	return p.ingress, nil
}
func (p *RoutedProvider) GetPipelineMonitor() metrics.PipelineMonitor { return p.monitor }

func (p *RoutedProvider) Flush(ctx context.Context) {
	p.lifecycle.RLock()
	if !p.started {
		p.lifecycle.RUnlock()
		return
	}
	done := make(chan struct{})
	marker := &message.Message{}
	marker.SetRoutingBarrier(done)
	select {
	case p.ingress <- marker:
		p.lifecycle.RUnlock()
	case <-ctx.Done():
		p.lifecycle.RUnlock()
		return
	case <-p.ctx.Done():
		p.lifecycle.RUnlock()
		return
	case <-p.stopRequested:
		p.lifecycle.RUnlock()
		return
	}
	select {
	case <-done:
	case <-ctx.Done():
	}
}
