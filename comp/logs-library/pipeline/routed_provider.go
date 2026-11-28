// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package pipeline

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

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
	routedEndpointCapacity  = 32
	routedOutcomeCapacity   = 128
	routedProgressCapacity  = 4096
	routedDestinationBuffer = 128
	routedHTTPRequests      = 64
	routedStopGrace         = 5 * time.Second
)

// RoutedOptions is the injection boundary for the normal HTTP routing
// mechanism. No Agent startup path constructs this provider automatically.
type RoutedOptions struct {
	NumberOfPipelines         int
	Sink                      sender.Sink
	DiagnosticMessageReceiver diagnostic.MessageReceiver
	ProcessingRules           []*config.ProcessingRule
	TagFilters                *tagfilter.Filters
	// Endpoints configures endpoint-level filtering with independent reliable
	// and unreliable targets.
	Endpoints []routing.EndpointTarget
	// Generation identifies checkpointed records on shared ingress before processing.
	// SourceInput supplies its bound generation instead. Anonymous records need none.
	// The callback must be local and bounded.
	Generation          func(*message.Message) (uint64, error)
	OnError             func(error)
	DestinationsContext *client.DestinationsContext
	Hostname            hostnameinterface.Component
	Config              pkgconfigmodel.Reader
	Compression         logscompression.Component
	Secrets             secrets.Component
}

type routedStream struct {
	pressureFlush bool // Single-record batches already flush immediately.
	input         chan *message.Message
	output        chan *message.Payload
	flush         chan struct{}
	strategy      sender.Strategy
}

// routedSourceDispatch tracks batching-stage progress without putting flush
// markers in the bounded data queue. Cancelled flushes leave no queue entries.
type routedSourceDispatch struct {
	input          chan *message.Message
	submitted      uint64 // Accessed only by dispatchProcessed.
	completed      atomic.Uint64
	advanced       chan struct{} // Coalesced notifications for the single dispatcher.
	flushAfter     atomic.Uint64
	flushRequested chan struct{} // Coalesced pressure boundaries, outside the data queue.
}

// requestFlush records a batch boundary behind the records already submitted.
// Only dispatchProcessed may call it.
func (s *routedSourceDispatch) requestFlush() {
	if s.flushAfter.Load() >= s.submitted {
		return
	}
	s.flushAfter.Store(s.submitted)
	select {
	case s.flushRequested <- struct{}{}:
	default:
	}
}

func (s *routedSourceDispatch) wait(ctx, providerCtx context.Context, target uint64) {
	for s.completed.Load() < target {
		select {
		case <-s.advanced:
		case <-ctx.Done():
			return
		case <-providerCtx.Done():
			return
		}
	}
}

type sourceInputKey struct {
	identifier string
	generation uint64
}

type RoutedProvider struct {
	opts                 RoutedOptions
	destinationsCtx      *client.DestinationsContext
	endpointDefs         *routing.EndpointDefinitions
	progress             *routing.Progress
	monitor              metrics.PipelineMonitor
	ingress              chan *message.Message
	inputs               []chan *message.Message
	sourceInputs         map[sourceInputKey]chan *message.Message
	inputKeys            map[chan *message.Message]sourceInputKey
	processed            chan *message.Message
	processors           []*processor.Processor
	streams              map[string]*routedStream
	optionalStreams      map[string]*routedStream
	senders              map[string]*sender.Sender
	optionalSenders      map[string]*httpsender.OptionalSender
	endpointInputs       map[string]chan *message.Payload
	endpointSinks        map[string]*routing.EndpointSink
	ctx                  context.Context
	cancel               context.CancelFunc
	ingressDone          sync.WaitGroup
	processedDone        chan struct{}
	dispatchDone         sync.WaitGroup
	optionalDispatchDone sync.WaitGroup
	endpointDone         sync.WaitGroup
	sourceDispatchMu     sync.Mutex
	sourceDispatch       map[string]*routedSourceDispatch
	sourceDone           sync.WaitGroup
	lifecycle            sync.RWMutex
	stopOnce             sync.Once
	stopRequested        chan struct{}
	started              bool
	stopped              bool
	pressureFlush        bool
}

// NewRoutedProvider validates injected endpoints before constructing workers.
// It leaves the default provider unchanged.
func NewRoutedProvider(opts RoutedOptions) (*RoutedProvider, error) {
	if opts.Config == nil || opts.Sink == nil || opts.Compression == nil || opts.DestinationsContext == nil || opts.Secrets == nil || opts.DiagnosticMessageReceiver == nil || opts.NumberOfPipelines < 1 || opts.NumberOfPipelines > 16 {
		return nil, errors.New("invalid routed provider dependencies")
	}
	endpointDefs, err := routing.NewEndpointDefinitions(opts.Endpoints)
	if err != nil {
		return nil, err
	}
	progress, err := routing.NewProgress(routedProgressCapacity, opts.Sink)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	context.AfterFunc(ctx, progress.StopForwarding)
	p := &RoutedProvider{
		opts: opts, endpointDefs: endpointDefs, progress: progress,
		destinationsCtx: client.NewDestinationsContext(),
		ingress:         make(chan *message.Message, routedIngressCapacity),
		processed:       make(chan *message.Message, routedProcessedCapacity),
		streams:         make(map[string]*routedStream),
		optionalStreams: make(map[string]*routedStream),
		senders:         make(map[string]*sender.Sender),
		optionalSenders: make(map[string]*httpsender.OptionalSender),
		endpointInputs:  make(map[string]chan *message.Payload),
		endpointSinks:   make(map[string]*routing.EndpointSink),
		ctx:             ctx, cancel: cancel,
		sourceDispatch: make(map[string]*routedSourceDispatch),
		sourceInputs:   make(map[sourceInputKey]chan *message.Message),
		inputKeys:      make(map[chan *message.Message]sourceInputKey),
		processedDone:  make(chan struct{}),
		stopRequested:  make(chan struct{}),
	}
	p.inputs = append(p.inputs, p.ingress)
	targets := endpointDefs.Targets()
	perEndpointCap := routedHTTPRequests / len(targets)
	bufferSize := opts.Config.GetInt("logs_config.payload_channel_size")
	if bufferSize < 1 {
		bufferSize = 1
	} else if bufferSize > routedDestinationBuffer {
		bufferSize = routedDestinationBuffer
	}
	for _, target := range targets {
		concurrency := maxConcurrencyPerPipeline
		if target.Endpoints.BatchMaxConcurrentSend > 0 {
			concurrency = target.Endpoints.BatchMaxConcurrentSend
		}
		if concurrency < 1 {
			concurrency = 1
		} else if concurrency > perEndpointCap {
			concurrency = perEndpointCap
		}
		compressor := opts.Compression.NewCompressor(compressioncommon.NoneKind, 0)
		if target.Endpoints.Main.UseCompression {
			compressor = opts.Compression.NewCompressor(target.Endpoints.Main.CompressionKind, target.Endpoints.Main.CompressionLevel)
		}
		if target.Endpoints.Main.IsReliable() {
			endpointSink := progress.NewEndpointSink(target.ID, routedOutcomeCapacity)
			p.endpointSinks[target.ID] = endpointSink
			p.senders[target.ID] = httpsender.NewHTTPSender(opts.Config, endpointSink, bufferSize, sender.NewServerlessMeta(false), target.Endpoints, p.destinationsCtx, "logs-routing-"+target.ID, httpclient.JSONContentType, "", sender.DefaultQueuesCount, sender.DefaultWorkersPerQueue, concurrency, concurrency, opts.Secrets, metrics.NewTelemetryPipelineMonitor())
			p.endpointInputs[target.ID] = make(chan *message.Payload, routedEndpointCapacity)
			stream := &routedStream{
				pressureFlush: target.Endpoints.BatchMaxSize > 1,
				input:         make(chan *message.Message, routedStreamCapacity),
				output:        make(chan *message.Payload, routedPayloadCapacity),
				flush:         make(chan struct{}, 1),
			}
			p.pressureFlush = p.pressureFlush || stream.pressureFlush
			targetID := target.ID
			stream.strategy = sender.NewBatchStrategyWithTerminalLoss(stream.input, stream.output, stream.flush, sender.NewServerlessMeta(false), target.Endpoints.BatchWait, target.Endpoints.BatchMaxSize, target.Endpoints.BatchMaxContentSize, "logs-routing", compressor, p.senders[target.ID].PipelineMonitor(), "reliable:"+target.ID, func(metas []*message.MessageMetadata) {
				p.progress.CompleteReliableTerminalLoss(targetID, message.NewPayload(metas, nil, "", 0))
			}, sender.DrainQueuedOnFlush, sender.SkipSourceLatency)
			p.streams[target.ID] = stream
			if p.monitor == nil {
				p.monitor = p.senders[target.ID].PipelineMonitor()
			}
		} else {
			optSender := httpsender.NewOptionalHTTPSender(opts.Config, target.ID, target.Endpoints.Main, p.destinationsCtx, "logs-routing-opt-"+target.ID, httpclient.JSONContentType, bufferSize, concurrency, opts.Secrets, metrics.NewTelemetryPipelineMonitor())
			p.optionalSenders[target.ID] = optSender
			stream := &routedStream{
				input:  make(chan *message.Message, routedStreamCapacity),
				output: make(chan *message.Payload, routedPayloadCapacity),
				flush:  make(chan struct{}),
			}
			stream.strategy = sender.NewBatchStrategyWithTerminalLoss(stream.input, stream.output, stream.flush, sender.NewServerlessMeta(false), target.Endpoints.BatchWait, target.Endpoints.BatchMaxSize, target.Endpoints.BatchMaxContentSize, "logs-routing-opt", compressor, optSender.PipelineMonitor(), "optional:"+target.ID, func([]*message.MessageMetadata) {}, sender.SkipSourceLatency)
			p.optionalStreams[target.ID] = stream
		}
	}
	for i := 0; i < opts.NumberOfPipelines; i++ {
		input := make(chan *message.Message, routedStreamCapacity)
		proc := processor.NewForRouting(opts.Config, input, p.processed, opts.ProcessingRules, opts.TagFilters, processor.NewJSONEncoder(opts.Config.GetBool("logs_config.use_container_timestamp")), opts.DiagnosticMessageReceiver, opts.Hostname, p.monitor, fmt.Sprintf("routed-%d", i), func(msg *message.Message) {
			p.progress.CompleteLocal(msg.RoutingToken())
		}, p.selectProcessed)
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
	p.destinationsCtx.StartWithContext(p.ctx)
	p.started = true
	go func() {
		select {
		case <-p.opts.DestinationsContext.Context().Done():
			p.cancel()
		case <-p.ctx.Done():
		}
	}()
	for id, s := range p.senders {
		s.Start()
		p.endpointDone.Add(1)
		go p.dispatchEndpoint(id)
	}
	for _, s := range p.optionalSenders {
		s.Start()
	}
	for id, stream := range p.streams {
		stream.strategy.Start()
		p.dispatchDone.Add(1)
		go p.dispatchPayloads(id, stream)
	}
	for id, stream := range p.optionalStreams {
		stream.strategy.Start()
		p.optionalDispatchDone.Add(1)
		go p.dispatchOptionalPayloads(id, stream)
	}
	for _, proc := range p.processors {
		proc.Start()
	}
	go p.dispatchProcessed()
	for _, input := range p.inputs {
		if key, bound := p.inputKeys[input]; bound {
			p.ingressDone.Add(1)
			go p.admit(input, &key)
			continue
		}
		p.ingressDone.Add(1)
		go p.admit(input, nil)
	}
}

func (p *RoutedProvider) admit(input <-chan *message.Message, bound *sourceInputKey) {
	defer p.ingressDone.Done()
	var index uint64
	if bound != nil {
		h := fnv.New32a()
		_, _ = h.Write([]byte(bound.identifier))
		index = uint64(h.Sum32()) % uint64(len(p.processors))
	}
	for msg := range input {
		if done := msg.RoutingBarrier(); done != nil {
			close(done)
			continue
		}
		if p.ctx.Err() != nil {
			continue
		}
		var generation uint64
		var err error
		if bound != nil {
			generation = bound.generation
			if msg.Origin == nil || msg.Origin.Identifier != bound.identifier {
				err = routing.ErrInvalidSelection
			}
		} else if msg.Origin != nil && msg.Origin.Identifier != "" {
			if p.opts.Generation == nil {
				err = routing.ErrInvalidSelection
			} else {
				generation, err = p.opts.Generation(msg)
			}
		}
		if err == nil {
			_, err = p.progress.Reserve(p.ctx, msg, generation)
		}
		if err != nil {
			p.report(err)
			if errors.Is(err, routing.ErrObsoleteGeneration) {
				continue
			}
			if holdErr := p.progress.HoldInvalid(p.ctx, msg); holdErr != nil {
				p.report(holdErr)
			} else if msg.Origin != nil {
				p.flushAtCapacity(msg.Origin.Identifier, nil)
			}
			continue
		}
		proc := p.processors[index%uint64(len(p.processors))]
		if bound == nil {
			index++
		}
		select {
		case proc.InputChan() <- msg:
			if bound != nil {
				p.flushAtCapacity(msg.Origin.Identifier, proc)
			} else {
				p.flushAtCapacity(msg.Origin.Identifier, nil)
			}
		case <-p.ctx.Done():
			// The reservation remains unresolved and will be replayed from the
			// last safe checkpoint after restart.
		}
	}
}

// flushAtCapacity orders a pressure boundary behind the admitted window.
// Bound sources use one processing lane. Shared ingress and aggregate pressure
// require markers on every lane so slow processing cannot miss the boundary.
func (p *RoutedProvider) flushAtCapacity(identifier string, proc *processor.Processor) {
	if !p.pressureFlush {
		return
	}
	sourceFull, aggregateFull := p.progress.AtCapacity(identifier)
	if !sourceFull && !aggregateFull {
		return
	}
	if aggregateFull {
		identifier = "" // Flush each source independently, without waiting across sources.
		proc = nil
	}
	targets := p.processors
	if proc != nil {
		targets = []*processor.Processor{proc}
	}
	for _, target := range targets {
		marker := &message.Message{}
		marker.Origin = &message.Origin{Identifier: identifier}
		marker.SetRoutingBarrier(make(chan struct{}))
		select {
		case target.InputChan() <- marker:
		case <-p.ctx.Done():
			return
		}
	}
}

// requestSourceFlush runs only in dispatchProcessed. A pressure marker has an
// origin; ordinary Flush markers do not. Record sequence numbers keep pressure
// notifications out of the bounded per-source data queue.
func (p *RoutedProvider) requestSourceFlush(identifier string) {
	p.sourceDispatchMu.Lock()
	defer p.sourceDispatchMu.Unlock()
	if identifier != "" {
		if lane := p.sourceDispatch["\x01"+identifier]; lane != nil {
			lane.requestFlush()
		}
		return
	}
	for _, lane := range p.sourceDispatch {
		lane.requestFlush()
	}
}

// flushSource signals each pending reliable batcher without waiting for a data slot.
// Checkpoints still depend exclusively on terminal outcomes.
func (p *RoutedProvider) flushSource(identifier string) {
	for _, id := range p.progress.PendingReliableEndpoints(identifier) {
		stream := p.streams[id]
		if stream == nil || !stream.pressureFlush {
			continue
		}
		select {
		case stream.flush <- struct{}{}:
		default:
		}
	}
}

// selectProcessed never waits for delivery or allocates a second reservation.
func (p *RoutedProvider) selectProcessed(msg *message.Message) processor.RoutingDecision {
	reliable, optional, reliableError, err := p.endpointDefs.Evaluate(routing.NewRecordView(msg))
	if err != nil {
		p.report(err)
	}
	token := msg.RoutingToken()
	if len(reliable) == 0 && reliableError {
		p.progress.Invalidate(token)
		return processor.RoutingUnresolved
	}
	if len(reliable)+len(optional) == 0 {
		p.progress.CompleteLocal(token)
		return processor.RoutingExcluded
	}
	if err := p.progress.AttachEndpointSelection(token, reliable, optional, reliableError); err != nil {
		p.progress.Invalidate(token)
		p.report(err)
		return processor.RoutingUnresolved
	}
	return processor.RoutingContinue
}

// flushPrior sends ordered markers through every processing lane and stream.
// Each marker can pass only after all earlier records in that stage.
func (p *RoutedProvider) flushPrior(ctx context.Context, done chan struct{}) {
	defer close(done)
	var acknowledgements []chan struct{}
	for _, proc := range p.processors {
		ack := make(chan struct{})
		marker := &message.Message{}
		marker.SetRoutingBarrier(ack)
		marker.SetRoutingBarrierContext(ctx)
		select {
		case proc.InputChan() <- marker:
			acknowledgements = append(acknowledgements, ack)
		case <-ctx.Done():
			return
		case <-p.stopRequested:
			return
		case <-p.ctx.Done():
			return
		}
	}
	for _, ack := range acknowledgements {
		select {
		case <-ack:
		case <-ctx.Done():
			return
		case <-p.stopRequested:
			return
		case <-p.ctx.Done():
			return
		}
	}
	acknowledgements = acknowledgements[:0]
	var endpointOffers []channelOffer
	for _, stream := range p.streams {
		ack := make(chan struct{})
		marker := &message.Message{}
		marker.SetRoutingBarrier(ack)
		marker.SetRoutingBarrierContext(ctx)
		endpointOffers = append(endpointOffers, channelOffer{input: stream.input, value: marker})
		acknowledgements = append(acknowledgements, ack)
	}
	for _, stream := range p.optionalStreams {
		marker := &message.Message{}
		marker.SetRoutingBarrier(make(chan struct{}))
		marker.SetRoutingBarrierContext(ctx)
		// Neither optional queue admission nor optional batch handoff gates Flush.
		select {
		case stream.input <- marker:
		default:
		}
	}
	if len(endpointOffers) != 0 {
		offerToChannelsWithFlush(ctx, endpointOffers, nil, nil, p.stopRequested)
	}
	for _, ack := range acknowledgements {
		select {
		case <-ack:
		case <-ctx.Done():
			return
		case <-p.stopRequested:
			return
		case <-p.ctx.Done():
			return
		}
	}
}

func (p *RoutedProvider) dispatchProcessed() {
	defer close(p.processedDone)
	for msg := range p.processed {
		if done := msg.RoutingBarrier(); done != nil {
			if msg.Origin != nil {
				p.requestSourceFlush(msg.Origin.Identifier)
				close(done)
				continue
			}
			ctx := msg.RoutingBarrierContext()
			if ctx == nil {
				ctx = context.Background()
			}
			type sourceTarget struct {
				lane     *routedSourceDispatch
				sequence uint64
			}
			p.sourceDispatchMu.Lock()
			targets := make([]sourceTarget, 0, len(p.sourceDispatch))
			pending := false
			for _, lane := range p.sourceDispatch {
				if ctx.Err() == nil && lane.completed.Load() < lane.submitted {
					// A pending offer can need a batch to send before this barrier can drain.
					lane.requestFlush()
					pending = true
				}
				targets = append(targets, sourceTarget{lane, lane.submitted})
			}
			p.sourceDispatchMu.Unlock()
			if pending && ctx.Err() == nil {
				// A completed source can still have a partial batch. Signal reliable
				// streams before a different source delays the final ordered markers.
				for _, stream := range p.streams {
					if stream.pressureFlush {
						select {
						case stream.flush <- struct{}{}:
						default:
						}
					}
				}
			}
			for _, target := range targets {
				target.lane.wait(ctx, p.ctx, target.sequence)
			}
			close(done)
			continue
		}
		identifier := ""
		if msg.Origin != nil {
			identifier = msg.Origin.Identifier
		}
		sourceIdentifier := identifier
		// Anonymous records have no source checkpoint and share one bounded lane.
		if identifier == "" {
			identifier = "\x00"
		} else {
			identifier = "\x01" + identifier
		}
		p.sourceDispatchMu.Lock()
		lane := p.sourceDispatch[identifier]
		if lane == nil {
			capacity := routing.SourcePendingLimit
			if sourceIdentifier == "" {
				// Anonymous admission uses the aggregate budget. Its shared lane
				// must hold that window without blocking other source dispatch.
				capacity = routedProgressCapacity
			}
			lane = &routedSourceDispatch{input: make(chan *message.Message, capacity), advanced: make(chan struct{}, 1), flushRequested: make(chan struct{}, 1)}
			p.sourceDispatch[identifier] = lane
			p.sourceDone.Add(1)
			go p.dispatchSource(sourceIdentifier, lane)
		}
		lane.submitted++
		p.sourceDispatchMu.Unlock()
		select {
		case lane.input <- msg:
		case <-p.ctx.Done():
		}
	}
	p.sourceDispatchMu.Lock()
	for id, lane := range p.sourceDispatch {
		close(lane.input)
		delete(p.sourceDispatch, id)
	}
	p.sourceDispatchMu.Unlock()
	p.sourceDone.Wait()
}

func (p *RoutedProvider) dispatchSource(identifier string, lane *routedSourceDispatch) {
	defer p.sourceDone.Done()
	var flushed uint64
	flushWhileStalled := func() {
		// Only a blocked fanout needs an early partial flush. Healthy windows
		// reach their sequence boundary and flush together.
		if target := lane.flushAfter.Load(); target > flushed {
			p.flushSource(identifier)
		}
	}
	for {
		select {
		case msg, open := <-lane.input:
			if !open {
				return
			}
			reliable, optional, delivered := p.progress.EndpointRecipients(msg.RoutingToken())
			if (len(reliable) != 0 || len(optional) != 0) && msg.Origin != nil && msg.Origin.LogSource != nil {
				msg.Origin.LogSource.LatencyStats.Add(msg.GetLatency())
			}
			for _, id := range optional {
				if stream := p.optionalStreams[id]; stream != nil {
					select {
					case stream.input <- msg:
					default:
						// Count the lost copy without waiting for queue space or compression.
						p.optionalSenders[id].RecordInputDrop()
					}
				}
			}
			if len(reliable) == 0 && len(optional) != 0 {
				p.progress.CompleteLocal(msg.RoutingToken())
			}
			offers := make([]channelOffer, 0, len(reliable))
			for _, id := range reliable {
				if stream := p.streams[id]; stream != nil {
					offers = append(offers, channelOffer{input: stream.input, value: msg})
				}
			}
			offerToChannelsWithFlush(p.ctx, offers, lane.flushRequested, flushWhileStalled, delivered)
			lane.completed.Add(1)
			select {
			case lane.advanced <- struct{}{}:
			default:
			}
		case <-lane.flushRequested:
		}
		if target := lane.flushAfter.Load(); target > flushed && lane.completed.Load() >= target {
			p.flushSource(identifier)
			flushed = target
		}
	}
}

func (p *RoutedProvider) dispatchPayloads(id string, stream *routedStream) {
	defer p.dispatchDone.Done()
	input := p.endpointInputs[id]
	for payload := range stream.output {
		select {
		case input <- payload:
		case <-p.ctx.Done():
		}
	}
}

func (p *RoutedProvider) dispatchOptionalPayloads(id string, stream *routedStream) {
	defer p.optionalDispatchDone.Done()
	optSender := p.optionalSenders[id]
	for payload := range stream.output {
		optSender.TrySend(payload)
	}
}

type channelOffer struct {
	input chan *message.Message
	value *message.Message
}

// offerToChannelsWithFlush permits a source pressure request to flush the
// available streams while fanout is blocked on a different selected stream.
// A reliable delivery signal ends the wait only after all initial offers.
func offerToChannelsWithFlush(ctx context.Context, offers []channelOffer, pressure <-chan struct{}, flush func(), delivered <-chan struct{}) {
	pending := make([]channelOffer, 0, len(offers))
	for _, offer := range offers {
		if ctx.Err() != nil {
			return
		}
		select {
		case offer.input <- offer.value:
		default:
			pending = append(pending, offer)
		}
	}
	if len(pending) != 0 && flush != nil {
		flush()
	}
	for len(pending) != 0 {
		cases := make([]reflect.SelectCase, 3, len(pending)+3)
		cases[0] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ctx.Done())}
		cases[1] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(pressure)}
		cases[2] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(delivered)}
		for _, offer := range pending {
			cases = append(cases, reflect.SelectCase{Dir: reflect.SelectSend, Chan: reflect.ValueOf(offer.input), Send: reflect.ValueOf(offer.value)})
		}
		chosen, _, _ := reflect.Select(cases)
		if chosen == 0 {
			return // Every unsent endpoint remains unresolved on cancellation.
		}
		if chosen == 1 {
			flush()
			continue
		}
		if chosen == 2 {
			return // Only copies still waiting for queue space are skipped.
		}
		index := chosen - 3
		pending[index] = pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if len(pending) != 0 && delivered != nil && flush != nil {
			// This stream can now hold a partial batch while another offer waits.
			flush()
		}
	}
}

func (p *RoutedProvider) dispatchEndpoint(id string) {
	defer p.endpointDone.Done()
	for payload := range p.endpointInputs[id] {
		select {
		case p.senders[id].In() <- payload:
		case <-p.ctx.Done():
			// Cancellation releases the copy without acknowledging delivery.
		}
	}
}

func (p *RoutedProvider) Stop() {
	stopComplete := make(chan struct{})
	defer close(stopComplete)
	p.stopOnce.Do(func() {
		close(p.stopRequested)
		go func() {
			grace := time.NewTimer(routedStopGrace)
			defer grace.Stop()
			select {
			case <-grace.C:
				p.cancel() // Also cancels this provider's HTTP destinations.
			case <-stopComplete:
			}
		}()
	})
	p.lifecycle.Lock()
	if !p.started {
		if !p.stopped {
			p.stopped = true
			for _, input := range p.inputs {
				close(input)
			}
			for _, sink := range p.endpointSinks {
				sink.Stop()
			}
			for _, optSender := range p.optionalSenders {
				optSender.Stop()
			}
			p.progress.Close()
			p.cancel()
		}
		p.lifecycle.Unlock()
		return
	}
	p.started = false
	p.stopped = true
	for _, input := range p.inputs {
		close(input)
	}
	p.lifecycle.Unlock()
	p.ingressDone.Wait()
	for _, proc := range p.processors {
		proc.Stop()
	}
	close(p.processed)
	<-p.processedDone
	for _, stream := range p.streams {
		stream.strategy.Stop()
		close(stream.output)
	}
	for _, stream := range p.optionalStreams {
		stream.strategy.Stop()
		close(stream.output)
	}
	p.dispatchDone.Wait()
	p.optionalDispatchDone.Wait()
	for _, input := range p.endpointInputs {
		close(input)
	}
	p.endpointDone.Wait()
	for id, s := range p.senders {
		s.Stop()
		p.endpointSinks[id].Stop()
	}
	for _, optSender := range p.optionalSenders {
		optSender.Stop()
	}
	p.progress.Close()
	p.cancel()
}

// SourceInput binds an input channel to one checkpointed producer generation.
// A routed source launcher must use this boundary to receive source isolation.
// Creation reserves generation ownership before the first record and may return
// ErrSourceCapacity when the bounded owner/history budget is exhausted.
func (p *RoutedProvider) SourceInput(identifier string, generation uint64) (chan *message.Message, error) {
	if identifier == "" || generation == 0 {
		return nil, routing.ErrInvalidSelection
	}
	p.lifecycle.Lock()
	defer p.lifecycle.Unlock()
	if p.stopped {
		return nil, routing.ErrStopped
	}
	key := sourceInputKey{identifier, generation}
	if input := p.sourceInputs[key]; input != nil {
		return input, nil
	}
	if len(p.sourceInputs) >= routedProgressCapacity {
		return nil, errors.New("too many routed source inputs")
	}
	if err := p.progress.RegisterSource(identifier, generation); err != nil {
		return nil, err
	}
	input := make(chan *message.Message, 1)
	p.inputs = append(p.inputs, input)
	p.sourceInputs[key] = input
	p.inputKeys[input] = key
	if p.started {
		p.ingressDone.Add(1)
		go p.admit(input, &key)
	}
	return input, nil
}

// NextPipelineChan returns shared ingress without per-source admission isolation.
// Checkpointed records require RoutedOptions.Generation for early reservation.
// A stalled source can block other sources on this channel. Routed launchers
// requiring independent source progress must use SourceInput instead.
func (p *RoutedProvider) NextPipelineChan() chan *message.Message { return p.ingress }

// GuardRegistry returns the registry to give routed source launchers. It
// prevents source-owned writes from bypassing outstanding delivery obligations.
func (p *RoutedProvider) GuardRegistry(registry routing.Registry) routing.Registry {
	return p.progress.GuardRegistry(registry)
}

// RetireSource waits for admission before releasing a stopped producer's ownership.
// Callers must stop sending first and provide a shutdown-cancelled context or
// deadline. Full input queues or busy coordinator locks return ErrSourcePending.
// Cancelling the admission wait leaves ownership and unresolved progress intact
// for a later retry or Stop.
func (p *RoutedProvider) RetireSource(ctx context.Context, identifier string, generation uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	key := sourceInputKey{identifier, generation}
	if !p.lifecycle.TryRLock() {
		return routing.ErrSourcePending
	}
	if p.stopped {
		p.lifecycle.RUnlock()
		return routing.ErrStopped
	}
	var admitted chan struct{}
	if input := p.sourceInputs[key]; input != nil {
		if p.started {
			admitted = make(chan struct{})
			marker := &message.Message{}
			marker.SetRoutingBarrier(admitted)
			select {
			case input <- marker:
			default:
				p.lifecycle.RUnlock()
				return routing.ErrSourcePending
			}
		} else if len(input) != 0 {
			p.lifecycle.RUnlock()
			return routing.ErrSourcePending
		}
	}
	// Waiting for admission must not block replacement producers or shutdown.
	p.lifecycle.RUnlock()
	if admitted != nil {
		select {
		case <-admitted:
		case <-ctx.Done():
			return ctx.Err()
		case <-p.ctx.Done():
			return routing.ErrStopped
		case <-p.stopRequested:
			return routing.ErrStopped
		}
	}
	// Acquire every cleanup lock before changing ownership. A busy coordinator
	// must remain retryable, including when its auditor queue is backpressured.
	if !p.lifecycle.TryLock() {
		return routing.ErrSourcePending
	}
	defer p.lifecycle.Unlock()
	if p.stopped {
		return routing.ErrStopped
	}
	if !p.sourceDispatchMu.TryLock() {
		return routing.ErrSourcePending
	}
	defer p.sourceDispatchMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	owned, err := p.progress.TryRetireSource(identifier, generation)
	if err != nil {
		return err
	}
	if !owned {
		key := "\x01" + identifier
		if lane := p.sourceDispatch[key]; lane != nil {
			close(lane.input)
			delete(p.sourceDispatch, key)
		}
	}
	if input := p.sourceInputs[key]; input != nil {
		close(input)
		delete(p.sourceInputs, key)
		delete(p.inputKeys, input)
		for i, candidate := range p.inputs {
			if candidate == input {
				p.inputs[i] = p.inputs[len(p.inputs)-1]
				p.inputs = p.inputs[:len(p.inputs)-1]
				break
			}
		}
	}
	return nil
}

func (p *RoutedProvider) GetOutputChan() chan *message.Message { return nil }

// NextPipelineChanWithMonitor returns the same shared ingress as NextPipelineChan
// and has the same head-of-line blocking limitation. Use SourceInput for isolation.
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
	acks := make([]chan struct{}, 0, len(p.inputs))
	for _, input := range p.inputs {
		if input == nil {
			continue
		}
		ack := make(chan struct{})
		marker := &message.Message{}
		marker.SetRoutingBarrier(ack)
		select {
		case input <- marker:
			acks = append(acks, ack)
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
	}
	for _, ack := range acks {
		select {
		case <-ack:
		case <-ctx.Done():
			p.lifecycle.RUnlock()
			return
		case <-p.stopRequested:
			p.lifecycle.RUnlock()
			return
		case <-p.ctx.Done():
			p.lifecycle.RUnlock()
			return
		}
	}
	done := make(chan struct{})
	p.flushPrior(ctx, done)
	p.lifecycle.RUnlock()
}
