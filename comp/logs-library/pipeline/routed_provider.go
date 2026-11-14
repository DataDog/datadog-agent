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
	routedGroupCapacity     = 32
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
	Groups                    []routing.Group
	// Generation identifies checkpointed records on shared ingress before processing.
	// SourceInput supplies its bound generation instead. Anonymous records need none.
	// The callback must be local and bounded, as must Resolve.
	Generation func(*message.Message) (uint64, error)
	// Resolve sees the commonly processed record before rendering or encoding.
	Resolve             func(*message.Message) (routing.Selection, error)
	OnError             func(error)
	DestinationsContext *client.DestinationsContext
	Hostname            hostnameinterface.Component
	Config              pkgconfigmodel.Reader
	Compression         logscompression.Component
	Secrets             secrets.Component
}

type routedStream struct {
	pressureFlush bool // Single-record batches already flush immediately.
	groups        []string
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
	opts             RoutedOptions
	destinationsCtx  *client.DestinationsContext
	defs             *routing.Definitions
	progress         *routing.Progress
	monitor          metrics.PipelineMonitor
	ingress          chan *message.Message
	inputs           []chan *message.Message
	sourceInputs     map[sourceInputKey]chan *message.Message
	inputKeys        map[chan *message.Message]sourceInputKey
	processed        chan *message.Message
	processors       []*processor.Processor
	streams          map[string]*routedStream
	senders          map[string]*sender.Sender
	groupInputs      map[string]chan *message.Payload
	sinks            map[string]*routing.GroupSink
	ctx              context.Context
	cancel           context.CancelFunc
	ingressDone      sync.WaitGroup
	processedDone    chan struct{}
	dispatchDone     sync.WaitGroup
	groupDone        sync.WaitGroup
	sourceDispatchMu sync.Mutex
	sourceDispatch   map[string]*routedSourceDispatch
	sourceDone       sync.WaitGroup
	lifecycle        sync.RWMutex
	stopOnce         sync.Once
	stopRequested    chan struct{}
	started          bool
	stopped          bool
	pressureFlush    bool
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
	context.AfterFunc(ctx, progress.StopForwarding)
	p := &RoutedProvider{
		opts: opts, defs: defs, progress: progress,
		destinationsCtx: client.NewDestinationsContext(),
		ingress:         make(chan *message.Message, routedIngressCapacity),
		processed:       make(chan *message.Message, routedProcessedCapacity),
		streams:         make(map[string]*routedStream), senders: make(map[string]*sender.Sender),
		groupInputs: make(map[string]chan *message.Payload),
		sinks:       make(map[string]*routing.GroupSink), ctx: ctx, cancel: cancel,
		sourceDispatch: make(map[string]*routedSourceDispatch),
		sourceInputs:   make(map[sourceInputKey]chan *message.Message),
		inputKeys:      make(map[chan *message.Message]sourceInputKey),
		processedDone:  make(chan struct{}),
		stopRequested:  make(chan struct{}),
	}
	p.inputs = append(p.inputs, p.ingress)
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
		p.senders[group.ID] = httpsender.NewHTTPSender(opts.Config, groupSink, bufferSize, sender.NewServerlessMeta(false), group.Endpoints, p.destinationsCtx, "logs-routing-"+group.ID, httpclient.JSONContentType, "", sender.DefaultQueuesCount, sender.DefaultWorkersPerQueue, concurrency, concurrency, opts.Secrets, metrics.NewTelemetryPipelineMonitor())
		p.groupInputs[group.ID] = make(chan *message.Payload, routedGroupCapacity)
		key := group.Stream
		if key == "" {
			key = "group:" + group.ID
		} else {
			key = "stream:" + key
		}
		stream := p.streams[key]
		if stream == nil {
			stream = &routedStream{input: make(chan *message.Message, routedStreamCapacity), output: make(chan *message.Payload, routedPayloadCapacity), flush: make(chan struct{}), pressureFlush: group.Endpoints.BatchMaxSize > 1}
			p.pressureFlush = p.pressureFlush || stream.pressureFlush
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
	for _, group := range p.defs.Groups() {
		p.senders[group.ID].Start()
		p.groupDone.Add(1)
		go p.dispatchGroup(group.ID)
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
	request := func(lane *routedSourceDispatch) {
		if lane.flushAfter.Load() >= lane.submitted {
			return
		}
		lane.flushAfter.Store(lane.submitted)
		select {
		case lane.flushRequested <- struct{}{}:
		default:
		}
	}
	if identifier != "" {
		if lane := p.sourceDispatch["\x01"+identifier]; lane != nil {
			request(lane)
		}
		return
	}
	for _, lane := range p.sourceDispatch {
		request(lane)
	}
}

// flushSource sends one ordered marker per pending stream. A shared stream gets
// one marker, and a blocked stream cannot prevent offering markers to healthy
// streams. Checkpoints still depend exclusively on terminal outcomes.
func (p *RoutedProvider) flushSource(identifier string) {
	groups := p.progress.PendingGroups(identifier)
	seen := make(map[*routedStream]struct{}, len(groups))
	offers := make([]channelOffer[*message.Message], 0, len(groups))
	for _, id := range groups {
		group, _ := p.defs.Group(id)
		key := "group:" + id
		if group.Stream != "" {
			key = "stream:" + group.Stream
		}
		stream := p.streams[key]
		if !stream.pressureFlush {
			continue
		}
		if _, duplicate := seen[stream]; duplicate {
			continue
		}
		seen[stream] = struct{}{}
		marker := &message.Message{}
		marker.SetRoutingBarrier(make(chan struct{}))
		offers = append(offers, channelOffer[*message.Message]{input: stream.input, value: marker})
	}
	offerToChannels(p.ctx, offers)
}

// selectProcessed never waits for delivery or allocates a second reservation.
func (p *RoutedProvider) selectProcessed(msg *message.Message) processor.RoutingDecision {
	selection, err := p.opts.Resolve(msg)
	var groups []string
	if err == nil {
		groups, err = p.defs.Resolve(selection)
	}
	if err == nil {
		err = p.progress.Attach(msg.RoutingToken(), selection, groups)
	}
	if err != nil {
		p.progress.Invalidate(msg.RoutingToken())
		p.report(err)
		return processor.RoutingUnresolved
	}
	if selection.Excluded {
		return processor.RoutingExcluded
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
	for _, stream := range p.streams {
		ack := make(chan struct{})
		marker := &message.Message{}
		marker.SetRoutingBarrier(ack)
		marker.SetRoutingBarrierContext(ctx)
		select {
		case stream.input <- marker:
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
			for _, lane := range p.sourceDispatch {
				targets = append(targets, sourceTarget{lane, lane.submitted})
			}
			p.sourceDispatchMu.Unlock()
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
	var flushed, stalled uint64
	flushWhileStalled := func() {
		// Only a blocked fanout needs an early partial flush. Healthy windows
		// reach their sequence boundary and flush together.
		if target := lane.flushAfter.Load(); target > flushed && target > stalled {
			p.flushSource(identifier)
			stalled = target
		}
	}
	for {
		select {
		case msg, open := <-lane.input:
			if !open {
				return
			}
			groups := p.progress.Recipients(msg.RoutingToken())
			seen := make(map[string]struct{}, len(groups))
			offers := make([]channelOffer[*message.Message], 0, len(groups))
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
				offers = append(offers, channelOffer[*message.Message]{input: p.streams[key].input, value: msg})
			}
			offerToChannelsWithFlush(p.ctx, offers, lane.flushRequested, flushWhileStalled)
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

func (p *RoutedProvider) dispatchPayloads(stream *routedStream) {
	defer p.dispatchDone.Done()
	for payload := range stream.output {
		offerPayloadToGroups(p.ctx, payload, stream.groups, p.groupInputs)
	}
}

// offerPayloadToGroups preserves one independent copy for each selected group.
// Queue admission is tracked by offerToChannels and never resolves delivery.
func offerPayloadToGroups(ctx context.Context, payload *message.Payload, groups []string, inputs map[string]chan *message.Payload) {
	offers := make([]channelOffer[*message.Payload], 0, len(groups))
	for _, id := range groups {
		copyForGroup := *payload // Encoded remains immutable and shared.
		offers = append(offers, channelOffer[*message.Payload]{input: inputs[id], value: &copyForGroup})
	}
	offerToChannels(ctx, offers)
}

type channelOffer[T any] struct {
	input chan T
	value T
}

// offerToChannels first offers all selected recipients without waiting. It
// then waits for whichever pending queue has room. Removing a successful send
// from pending prevents duplicate admission without per-record goroutines.
func offerToChannels[T any](ctx context.Context, offers []channelOffer[T]) {
	offerToChannelsWithFlush(ctx, offers, nil, nil)
}

// offerToChannelsWithFlush permits a source pressure request to flush the
// available streams while fanout is blocked on a different selected stream.
func offerToChannelsWithFlush[T any](ctx context.Context, offers []channelOffer[T], pressure <-chan struct{}, flush func()) {
	pending := make([]channelOffer[T], 0, len(offers))
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
		cases := make([]reflect.SelectCase, 2, len(pending)+2)
		cases[0] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ctx.Done())}
		cases[1] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(pressure)}
		for _, offer := range pending {
			cases = append(cases, reflect.SelectCase{Dir: reflect.SelectSend, Chan: reflect.ValueOf(offer.input), Send: reflect.ValueOf(offer.value)})
		}
		chosen, _, _ := reflect.Select(cases)
		if chosen == 0 {
			return // Every unsent group remains unresolved on cancellation.
		}
		if chosen == 1 {
			flush()
			continue
		}
		index := chosen - 2
		pending[index] = pending[len(pending)-1]
		pending = pending[:len(pending)-1]
	}
}

func (p *RoutedProvider) dispatchGroup(id string) {
	defer p.groupDone.Done()
	for payload := range p.groupInputs[id] {
		select {
		case p.senders[id].In() <- payload:
		case <-p.ctx.Done():
			// Cancellation releases the copy without acknowledging its group.
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
	p.dispatchDone.Wait()
	for _, input := range p.groupInputs {
		close(input)
	}
	p.groupDone.Wait()
	for _, group := range p.defs.Groups() {
		p.senders[group.ID].Stop()
		p.sinks[group.ID].Stop()
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
// prevents source-owned writes from bypassing outstanding group obligations.
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
