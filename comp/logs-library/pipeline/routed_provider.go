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

type sourceInputKey struct {
	identifier string
	generation uint64
}

type RoutedProvider struct {
	opts             RoutedOptions
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
	sourceDispatch   map[string]chan *message.Message
	sourceDone       sync.WaitGroup
	lifecycle        sync.RWMutex
	stopOnce         sync.Once
	stopRequested    chan struct{}
	started          bool
	stopped          bool
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
		ingress:   make(chan *message.Message, routedIngressCapacity),
		processed: make(chan *message.Message, routedProcessedCapacity),
		streams:   make(map[string]*routedStream), senders: make(map[string]*sender.Sender),
		groupInputs: make(map[string]chan *message.Payload),
		sinks:       make(map[string]*routing.GroupSink), ctx: ctx, cancel: cancel,
		sourceDispatch: make(map[string]chan *message.Message),
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
		p.senders[group.ID] = httpsender.NewHTTPSender(opts.Config, groupSink, bufferSize, sender.NewServerlessMeta(false), group.Endpoints, opts.DestinationsContext, "logs-routing-"+group.ID, httpclient.JSONContentType, "", sender.DefaultQueuesCount, sender.DefaultWorkersPerQueue, concurrency, concurrency, opts.Secrets, metrics.NewTelemetryPipelineMonitor())
		p.groupInputs[group.ID] = make(chan *message.Payload, routedGroupCapacity)
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
		selection, err := p.opts.Resolve(msg)
		if err != nil {
			p.report(err)
			if holdErr := p.progress.HoldInvalid(p.ctx, msg); holdErr != nil {
				p.report(holdErr)
			}
			continue
		}
		if bound != nil && (msg.Origin == nil || msg.Origin.Identifier != bound.identifier || selection.Generation != bound.generation) {
			p.report(routing.ErrInvalidSelection)
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
		if bound == nil {
			index++
		}
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
func (p *RoutedProvider) flushPrior(ctx context.Context, done chan struct{}) {
	defer close(done)
	var acknowledgements []chan struct{}
	for _, proc := range p.processors {
		ack := make(chan struct{})
		marker := &message.Message{}
		marker.SetRoutingBarrier(ack)
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
			p.sourceDispatchMu.Lock()
			acks := make([]chan struct{}, 0, len(p.sourceDispatch))
			for _, input := range p.sourceDispatch {
				ack := make(chan struct{})
				marker := &message.Message{}
				marker.SetRoutingBarrier(ack)
				select {
				case input <- marker:
					acks = append(acks, ack)
				case <-p.ctx.Done():
				}
			}
			p.sourceDispatchMu.Unlock()
			for _, ack := range acks {
				select {
				case <-ack:
				case <-p.ctx.Done():
				}
			}
			close(done)
			continue
		}
		identifier := ""
		if msg.Origin != nil {
			identifier = msg.Origin.Identifier
		}
		// Anonymous records have no source checkpoint and share one bounded lane.
		if identifier == "" {
			identifier = "\x00"
		} else {
			identifier = "\x01" + identifier
		}
		p.sourceDispatchMu.Lock()
		input := p.sourceDispatch[identifier]
		if input == nil {
			input = make(chan *message.Message, routing.SourcePendingLimit)
			p.sourceDispatch[identifier] = input
			p.sourceDone.Add(1)
			go p.dispatchSource(input)
		}
		p.sourceDispatchMu.Unlock()
		select {
		case input <- msg:
		case <-p.ctx.Done():
		}
	}
	p.sourceDispatchMu.Lock()
	for id, input := range p.sourceDispatch {
		close(input)
		delete(p.sourceDispatch, id)
	}
	p.sourceDispatchMu.Unlock()
	p.sourceDone.Wait()
}

func (p *RoutedProvider) dispatchSource(input <-chan *message.Message) {
	defer p.sourceDone.Done()
	for msg := range input {
		if done := msg.RoutingBarrier(); done != nil {
			close(done)
			continue
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
		offerToChannels(p.ctx, offers)
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
	for len(pending) != 0 {
		cases := make([]reflect.SelectCase, 1, len(pending)+1)
		cases[0] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ctx.Done())}
		for _, offer := range pending {
			cases = append(cases, reflect.SelectCase{Dir: reflect.SelectSend, Chan: reflect.ValueOf(offer.input), Send: reflect.ValueOf(offer.value)})
		}
		chosen, _, _ := reflect.Select(cases)
		if chosen == 0 {
			return // Every unsent group remains unresolved on cancellation.
		}
		index := chosen - 1
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
	p.stopOnce.Do(func() { close(p.stopRequested) })
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
	stopComplete := make(chan struct{})
	defer close(stopComplete)
	go func() {
		grace := time.NewTimer(routedStopGrace)
		defer grace.Stop()
		select {
		case <-grace.C:
			p.cancel() // Release blocked workers without acknowledging their records.
		case <-stopComplete:
		}
	}()
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

func (p *RoutedProvider) NextPipelineChan() chan *message.Message { return p.ingress }

// GuardRegistry returns the registry to give routed source launchers. It
// prevents source-owned writes from bypassing outstanding group obligations.
func (p *RoutedProvider) GuardRegistry(registry auditor.Registry) auditor.Registry {
	return p.progress.GuardRegistry(registry)
}

// RetireSource releases a producer's generation ownership after it stops and
// all of its decoder output has been admitted.
func (p *RoutedProvider) RetireSource(identifier string, generation uint64) error {
	p.sourceDispatchMu.Lock()
	if err := p.progress.RetireSource(identifier, generation); err != nil {
		p.sourceDispatchMu.Unlock()
		return err
	}
	if !p.progress.OwnsSource(identifier) {
		key := "\x01" + identifier
		if input := p.sourceDispatch[key]; input != nil {
			close(input)
			delete(p.sourceDispatch, key)
		}
	}
	p.sourceDispatchMu.Unlock()
	key := sourceInputKey{identifier, generation}
	p.lifecycle.Lock()
	if input := p.sourceInputs[key]; input != nil && !p.stopped {
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
	p.lifecycle.Unlock()
	return nil
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
