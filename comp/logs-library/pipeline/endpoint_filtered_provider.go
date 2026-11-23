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
	"math/bits"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
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
	optionalStreamCapacity = 128
)

var (
	tlmEndpointFilterEvalErrors = telemetryimpl.GetCompatComponent().NewCounter(
		"logs_endpoint_filter", "eval_errors", []string{"endpoint", "reliable"},
		"Number of endpoint selector evaluation errors",
	)
	tlmEndpointFilterExcluded = telemetryimpl.GetCompatComponent().NewCounter(
		"logs_endpoint_filter", "excluded", []string{},
		"Number of logs excluded because no endpoint matched",
	)
	tlmEndpointFilterOptionalOnly = telemetryimpl.GetCompatComponent().NewCounter(
		"logs_endpoint_filter", "optional_only", []string{},
		"Number of logs matching only optional (unreliable) endpoints",
	)
	tlmEndpointFilterOptionalDropped = telemetryimpl.GetCompatComponent().NewCounter(
		"logs_endpoint_filter", "optional_messages_dropped", []string{"endpoint"},
		"Number of best-effort messages dropped because an optional endpoint queue was full",
	)
	tlmEndpointFilterOptionalPayloadsDropped = telemetryimpl.GetCompatComponent().NewCounter(
		"logs_endpoint_filter", "optional_payloads_dropped", []string{"endpoint"},
		"Number of best-effort payloads dropped because an optional endpoint sender queue was full",
	)
	tlmEndpointFilterReliableQueueDropped = telemetryimpl.GetCompatComponent().NewCounter(
		"logs_endpoint_filter", "reliable_messages_dropped", []string{"endpoint"},
		"Number of messages skipped for a full reliable endpoint while another reliable endpoint accepted",
	)
)

// EndpointFilteredOptions configures normal-mode Logs delivery with per-endpoint
// filtering using Layout B batching (one independent batch stream per endpoint).
type EndpointFilteredOptions struct {
	NumberOfPipelines         int
	Sink                      sender.Sink
	DiagnosticMessageReceiver diagnostic.MessageReceiver
	ProcessingRules           []*config.ProcessingRule
	TagFilters                *tagfilter.Filters
	Endpoints                 *config.Endpoints
	// Selectors aligns by index with Endpoints.Endpoints. Nil entries mean
	// unfiltered (always match, subject to MRF gating for MRF endpoints).
	Selectors []routing.EndpointSelector
	// Generation optionally supplies the producer generation for checkpointed
	// records arriving on shared ingress (NextPipelineChan). If nil, shared
	// ingress defaults checkpointed sources to generation 1.
	Generation          func(*message.Message) (uint64, error)
	OnError             func(error)
	DestinationsContext *client.DestinationsContext
	Hostname            hostnameinterface.Component
	Config              pkgconfigmodel.Reader
	Compression         logscompression.Component
	Secrets             secrets.Component
}

type endpointSelection struct {
	reliableMask uint64
	optionalMask uint64
}

// managedInput coordinates safe single-owner closure of a bound or shared
// ingress channel across concurrent Flush, RetireSource, and Stop calls.
type managedInput struct {
	ch       chan *message.Message
	mu       sync.Mutex
	retiring bool
	closed   bool
	senders  int
	idle     chan struct{}
}

func newManagedInput(ch chan *message.Message) *managedInput {
	return &managedInput{ch: ch}
}

func (m *managedInput) acquire() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.retiring || m.closed {
		return false
	}
	m.senders++
	return true
}

func (m *managedInput) release() {
	m.mu.Lock()
	m.senders--
	if m.senders == 0 && m.idle != nil {
		close(m.idle)
		m.idle = nil
	}
	m.mu.Unlock()
}

func (m *managedInput) claimRetire() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.retiring || m.closed {
		return false
	}
	m.retiring = true
	return true
}

func (m *managedInput) waitSendersAndClose() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	var waitCh chan struct{}
	if m.senders > 0 {
		if m.idle == nil {
			m.idle = make(chan struct{})
		}
		waitCh = m.idle
	}
	m.mu.Unlock()
	if waitCh != nil {
		<-waitCh
	}
	m.closeOnce()
}

func (m *managedInput) closeOnce() {
	m.mu.Lock()
	if !m.closed {
		m.closed = true
		close(m.ch)
	}
	m.mu.Unlock()
}

type sharedSourcePark struct {
	queue  chan *message.Message
	queued int // Guarded by sharedParkMu.
}

type endpointStream struct {
	index         int
	id            string
	reliable      bool
	isMRF         bool
	pressureFlush bool
	input         chan *message.Message
	output        chan *message.Payload
	flush         chan struct{}
	strategy      sender.Strategy

	// Reliable endpoint delivery components (non-nil iff reliable).
	reliableSender *sender.Sender
	groupSink      *routing.GroupSink

	// Optional endpoint delivery components (non-nil iff !reliable).
	optionalEP          config.Endpoint
	optionalConcurrency int
	optionalBufSize     int
	optionalDest        *sender.DestinationSender
	optionalNoop        chan *message.Payload
}

// EndpointFilteredProvider implements pipeline.Provider for normal-mode HTTP
// Logs delivery with per-endpoint filtering and Layout B batching.
//
// Each endpoint maintains its own independent batcher and HTTP sender so
// primary batches are never split by sparse secondary matches. Reliable
// endpoints use CompletionRequireAny (at least one matching reliable endpoint
// completing satisfies the reliable-set rule), while optional endpoints use
// bounded non-blocking queues and never advance or block source checkpoints.
type EndpointFilteredProvider struct {
	opts             EndpointFilteredOptions
	destinationsCtx  *client.DestinationsContext
	policy           *routing.EndpointFilterPolicy
	progress         *routing.Progress
	monitor          metrics.PipelineMonitor
	ingress          *managedInput
	inputs           []*managedInput
	sourceInputs     map[sourceInputKey]*managedInput
	inputKeys        map[*managedInput]sourceInputKey
	processed        chan *message.Message
	processors       []*processor.Processor
	streams          []*endpointStream
	selectionsMu     sync.Mutex
	selections       map[uint64]endpointSelection
	sharedParkMu     sync.Mutex
	sharedParks      map[string]*sharedSourcePark
	sharedParkWg     sync.WaitGroup
	ctx              context.Context
	cancel           context.CancelFunc
	ingressDone      sync.WaitGroup
	processedDone    chan struct{}
	dispatchDone     sync.WaitGroup
	sourceDispatchMu sync.Mutex
	sourceDispatch   map[string]*routedSourceDispatch
	sourceDone       sync.WaitGroup
	opsWg            sync.WaitGroup
	lifecycle        sync.RWMutex
	stopOnce         sync.Once
	stopRequested    chan struct{}
	started          bool
	stopped          bool
	pressureFlush    bool
	evalWarnBudget   atomic.Int64
}

// NewEndpointFilteredProvider validates endpoints and selectors and constructs
// a Layout B endpoint-filtered pipeline provider.
func NewEndpointFilteredProvider(opts EndpointFilteredOptions) (*EndpointFilteredProvider, error) {
	if opts.Config == nil || opts.Sink == nil || opts.Compression == nil || opts.DestinationsContext == nil || opts.Secrets == nil || opts.DiagnosticMessageReceiver == nil || opts.NumberOfPipelines < 1 || opts.NumberOfPipelines > 16 {
		return nil, errors.New("invalid endpoint-filtered provider dependencies")
	}
	policy, err := routing.NewEndpointFilterPolicy(opts.Endpoints, opts.Selectors)
	if err != nil {
		return nil, err
	}
	progress, err := routing.NewProgressWithMode(routedProgressCapacity, opts.Sink, routing.CompletionRequireAny)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	context.AfterFunc(ctx, progress.StopForwarding)

	p := &EndpointFilteredProvider{
		opts:            opts,
		destinationsCtx: client.NewDestinationsContext(),
		policy:          policy,
		progress:        progress,
		ingress:         newManagedInput(make(chan *message.Message, routedIngressCapacity)),
		processed:       make(chan *message.Message, routedProcessedCapacity),
		streams:         make([]*endpointStream, policy.EndpointCount()),
		selections:      make(map[uint64]endpointSelection),
		sharedParks:     make(map[string]*sharedSourcePark),
		ctx:             ctx,
		cancel:          cancel,
		sourceDispatch:  make(map[string]*routedSourceDispatch),
		sourceInputs:    make(map[sourceInputKey]*managedInput),
		inputKeys:       make(map[*managedInput]sourceInputKey),
		processedDone:   make(chan struct{}),
		stopRequested:   make(chan struct{}),
	}
	p.evalWarnBudget.Store(20)
	p.inputs = append(p.inputs, p.ingress)

	endpointCount := policy.EndpointCount()
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

	baseConcurrency := maxConcurrencyPerPipeline
	if opts.Endpoints.BatchMaxConcurrentSend > 0 {
		baseConcurrency = opts.Endpoints.BatchMaxConcurrentSend
	}
	if baseConcurrency < 1 {
		baseConcurrency = 1
	} else if baseConcurrency > perEndpointCap {
		baseConcurrency = perEndpointCap
	}

	p.monitor = metrics.NewTelemetryPipelineMonitor()

	for i := 0; i < endpointCount; i++ {
		ep := policy.Endpoint(i)
		epID := policy.EndpointID(i)

		streamCap := optionalStreamCapacity
		if ep.IsReliable() {
			streamCap = routedStreamCapacity
		}
		stream := &endpointStream{
			index:         i,
			id:            epID,
			reliable:      ep.IsReliable(),
			isMRF:         ep.IsMRF,
			pressureFlush: ep.IsReliable() && opts.Endpoints.BatchMaxSize > 1,
			input:         make(chan *message.Message, streamCap),
			output:        make(chan *message.Payload, routedPayloadCapacity),
			flush:         make(chan struct{}, 1),
		}
		p.pressureFlush = p.pressureFlush || stream.pressureFlush

		compressor := opts.Compression.NewCompressor(compressioncommon.NoneKind, 0)
		if ep.UseCompression {
			compressor = opts.Compression.NewCompressor(ep.CompressionKind, ep.CompressionLevel)
		}

		if ep.IsReliable() {
			groupSink := progress.NewGroupSink(epID, routedOutcomeCapacity)
			stream.groupSink = groupSink

			// MRF eligibility is gated before selection by EndpointFilterPolicy.
			// Clear IsMRF on the dedicated sender copy so records already
			// selected while failover was enabled cannot be silently dropped
			// without a terminal outcome if failover toggles off mid-flight.
			senderEP := ep
			senderEP.IsMRF = false
			singleEP := config.NewEndpointsWithBatchSettings(
				senderEP, nil, false, true,
				opts.Endpoints.BatchWait,
				opts.Endpoints.BatchMaxConcurrentSend,
				opts.Endpoints.BatchMaxSize,
				opts.Endpoints.BatchMaxContentSize,
				opts.Endpoints.InputChanSize,
			)
			stream.reliableSender = httpsender.NewHTTPSender(
				opts.Config,
				groupSink,
				bufferSize,
				sender.NewServerlessMeta(false),
				singleEP,
				p.destinationsCtx,
				"logs-endpoint-filter-"+epID,
				httpclient.JSONContentType,
				"",
				sender.DefaultQueuesCount,
				sender.DefaultWorkersPerQueue,
				baseConcurrency,
				baseConcurrency,
				opts.Secrets,
				p.monitor,
			)
			id := epID
			stream.strategy = sender.NewBatchStrategyWithTerminalLoss(
				stream.input,
				stream.output,
				stream.flush,
				sender.NewServerlessMeta(false),
				opts.Endpoints.BatchWait,
				opts.Endpoints.BatchMaxSize,
				opts.Endpoints.BatchMaxContentSize,
				"logs-endpoint-filter",
				compressor,
				p.monitor,
				id,
				func(metas []*message.MessageMetadata) {
					p.progress.FailGroupLocal(id, message.NewPayload(metas, nil, "", 0))
				},
			)
		} else {
			// Optional (unreliable) endpoint: destination and sender are started
			// in Start() once destinationsCtx is initialized.
			stream.optionalEP = ep
			stream.optionalConcurrency = baseConcurrency
			stream.optionalBufSize = bufferSize
			stream.strategy = sender.NewBatchStrategyWithTerminalLoss(
				stream.input,
				stream.output,
				stream.flush,
				sender.NewServerlessMeta(false),
				opts.Endpoints.BatchWait,
				opts.Endpoints.BatchMaxSize,
				opts.Endpoints.BatchMaxContentSize,
				"logs-endpoint-filter",
				compressor,
				p.monitor,
				epID,
				nil, // Optional batch loss does not touch source progress.
			)
		}
		p.streams[i] = stream
	}

	for i := 0; i < opts.NumberOfPipelines; i++ {
		input := make(chan *message.Message, routedStreamCapacity)
		proc := processor.NewForEndpointFiltering(
			opts.Config,
			input,
			p.processed,
			opts.ProcessingRules,
			opts.TagFilters,
			processor.NewJSONEncoder(opts.Config.GetBool("logs_config.use_container_timestamp")),
			opts.DiagnosticMessageReceiver,
			opts.Hostname,
			p.monitor,
			fmt.Sprintf("endpoint-filter-%d", i),
			func(msg *message.Message) {
				token := msg.RoutingToken()
				p.deleteSelection(token)
				p.progress.CompleteLocal(token)
			},
			p.selectProcessed,
		)
		p.processors = append(p.processors, proc)
	}

	return p, nil
}

func (p *EndpointFilteredProvider) report(err error) {
	if p.opts.OnError != nil {
		p.opts.OnError(err)
		return
	}
	if p.evalWarnBudget.Add(-1) >= 0 {
		log.Warnf("logs endpoint filtering: %v", err)
	}
}

func (p *EndpointFilteredProvider) setSelection(token uint64, sel endpointSelection) {
	p.selectionsMu.Lock()
	p.selections[token] = sel
	p.selectionsMu.Unlock()
}

func (p *EndpointFilteredProvider) popSelection(token uint64) (endpointSelection, bool) {
	p.selectionsMu.Lock()
	sel, ok := p.selections[token]
	if ok {
		delete(p.selections, token)
	}
	p.selectionsMu.Unlock()
	return sel, ok
}

func (p *EndpointFilteredProvider) deleteSelection(token uint64) {
	p.selectionsMu.Lock()
	delete(p.selections, token)
	p.selectionsMu.Unlock()
}

// GuardRegistry wraps direct source checkpoint writes through bounded progress.
func (p *EndpointFilteredProvider) GuardRegistry(registry routing.Registry) routing.Registry {
	return p.progress.GuardRegistry(registry)
}

// Start launches endpoint senders, Layout B batch strategies, processors, and admission workers.
func (p *EndpointFilteredProvider) Start() {
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

	for _, stream := range p.streams {
		if stream.reliable {
			stream.reliableSender.Start()
		} else {
			noopSink := make(chan *message.Payload, stream.optionalBufSize)
			go func(ch <-chan *message.Payload) {
				for range ch {
				}
			}(noopSink)
			stream.optionalNoop = noopSink

			destMeta := client.NewDestinationMetadata("logs-endpoint-filter-"+stream.id, "0", "unreliable", strconv.Itoa(stream.index), "")
			dest := httpclient.NewDestination(
				stream.optionalEP,
				httpclient.JSONContentType,
				p.destinationsCtx,
				false, // shouldRetry: unreliable endpoints drop on failure
				destMeta,
				p.opts.Config,
				stream.optionalConcurrency,
				stream.optionalConcurrency,
				p.monitor,
				"0",
				p.opts.Secrets,
			)
			stream.optionalDest = sender.NewDestinationSender(p.opts.Config, dest, noopSink, stream.optionalBufSize)
		}
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
			go p.admit(input.ch, &key)
			continue
		}
		p.ingressDone.Add(1)
		go p.admit(input.ch, nil)
	}
}

func (p *EndpointFilteredProvider) admit(input <-chan *message.Message, bound *sourceInputKey) {
	defer p.ingressDone.Done()
	var rrIndex uint64
	laneFor := func(identifier string) *processor.Processor {
		if identifier != "" {
			h := fnv.New32a()
			_, _ = h.Write([]byte(identifier))
			return p.processors[uint64(h.Sum32())%uint64(len(p.processors))]
		}
		proc := p.processors[rrIndex%uint64(len(p.processors))]
		rrIndex++
		return proc
	}

	dispatchToProc := func(msg *message.Message, identifier string) {
		proc := laneFor(identifier)
		select {
		case proc.InputChan() <- msg:
			if identifier != "" {
				p.flushAtCapacity(identifier, proc)
			} else {
				p.flushAtCapacity("", nil)
			}
		case <-p.ctx.Done():
		}
	}

	handleReserveErr := func(msg *message.Message, err error) {
		p.report(err)
		if errors.Is(err, routing.ErrObsoleteGeneration) {
			return
		}
		if holdErr := p.progress.HoldInvalid(p.ctx, msg); holdErr != nil {
			p.report(holdErr)
		} else if msg.Origin != nil {
			p.flushAtCapacity(msg.Origin.Identifier, nil)
		}
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
			// Accept deliberately uncheckpointed output (such as a draining
			// rotated file tailer emitting empty Identifier) as anonymous;
			// reject a different nonempty identifier.
			if msg.Origin == nil || msg.Origin.Identifier == "" {
				generation = 0
			} else if msg.Origin.Identifier != bound.identifier {
				err = routing.ErrInvalidSelection
			} else {
				generation = bound.generation
			}
		} else if msg.Origin != nil && msg.Origin.Identifier != "" {
			if p.opts.Generation != nil {
				generation, err = p.opts.Generation(msg)
			} else {
				generation = 1
			}
		}

		identifier := ""
		if msg.Origin != nil {
			identifier = msg.Origin.Identifier
		}

		// On shared ingress, avoid stalling unrelated sources if one source
		// has reached SourcePendingLimit by parking that source's backlog in a
		// bounded per-source waiter goroutine.
		if err == nil && bound == nil && identifier != "" {
			if p.parkIfStalled(identifier, msg, generation, dispatchToProc, handleReserveErr) {
				continue
			}
			_, reserved, tryErr := p.progress.TryReserveShared(msg, generation)
			if tryErr != nil {
				handleReserveErr(msg, tryErr)
				continue
			}
			if !reserved {
				p.enqueueSharedPark(identifier, msg, generation, dispatchToProc, handleReserveErr)
				continue
			}
			dispatchToProc(msg, identifier)
			continue
		}

		if err == nil {
			if bound != nil {
				_, err = p.progress.Reserve(p.ctx, msg, generation)
			} else {
				_, err = p.progress.ReserveShared(p.ctx, msg, generation)
			}
		}
		if err != nil {
			handleReserveErr(msg, err)
			continue
		}

		dispatchToProc(msg, identifier)
	}
}

func (p *EndpointFilteredProvider) parkIfStalled(
	identifier string,
	msg *message.Message,
	generation uint64,
	dispatch func(*message.Message, string),
	onErr func(*message.Message, error),
) bool {
	p.sharedParkMu.Lock()
	park := p.sharedParks[identifier]
	if park == nil {
		p.sharedParkMu.Unlock()
		return false
	}
	park.queued++
	p.sharedParkMu.Unlock()
	select {
	case park.queue <- msg:
	case <-p.ctx.Done():
		p.sharedParkMu.Lock()
		park.queued--
		p.sharedParkMu.Unlock()
	}
	_ = generation
	_ = dispatch
	_ = onErr
	return true
}

func (p *EndpointFilteredProvider) enqueueSharedPark(
	identifier string,
	msg *message.Message,
	generation uint64,
	dispatch func(*message.Message, string),
	onErr func(*message.Message, error),
) {
	p.sharedParkMu.Lock()
	park := p.sharedParks[identifier]
	created := false
	if park == nil {
		park = &sharedSourcePark{queue: make(chan *message.Message, routing.SourcePendingLimit)}
		p.sharedParks[identifier] = park
		created = true
		p.sharedParkWg.Add(1)
	}
	park.queued++
	p.sharedParkMu.Unlock()

	if created {
		go func(id string, pk *sharedSourcePark, gen uint64) {
			defer p.sharedParkWg.Done()
			for {
				select {
				case m := <-pk.queue:
					if _, err := p.progress.ReserveShared(p.ctx, m, gen); err != nil {
						onErr(m, err)
					} else {
						dispatch(m, id)
					}
					p.sharedParkMu.Lock()
					pk.queued--
					if pk.queued == 0 && p.sharedParks[id] == pk {
						delete(p.sharedParks, id)
						p.sharedParkMu.Unlock()
						return
					}
					p.sharedParkMu.Unlock()
				case <-p.ctx.Done():
					p.sharedParkMu.Lock()
					if p.sharedParks[id] == pk {
						delete(p.sharedParks, id)
					}
					p.sharedParkMu.Unlock()
					return
				}
			}
		}(identifier, park, generation)
	}

	select {
	case park.queue <- msg:
	case <-p.ctx.Done():
		p.sharedParkMu.Lock()
		park.queued--
		p.sharedParkMu.Unlock()
	}
}

func (p *EndpointFilteredProvider) flushAtCapacity(identifier string, proc *processor.Processor) {
	if !p.pressureFlush {
		return
	}
	sourceFull, aggregateFull := p.progress.AtCapacity(identifier)
	if !sourceFull && !aggregateFull {
		return
	}
	if aggregateFull {
		identifier = ""
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

func (p *EndpointFilteredProvider) requestSourceFlush(identifier string) {
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

func (p *EndpointFilteredProvider) flushSource(identifier string) {
	groups := p.progress.PendingGroups(identifier)
	for _, id := range groups {
		for _, stream := range p.streams {
			if stream.id == id && stream.pressureFlush {
				// Out-of-band coalesced flush: never blocks on a full stalled stream.
				select {
				case stream.flush <- struct{}{}:
				default:
				}
				break
			}
		}
	}
}

// selectProcessed evaluates endpoint selectors once per record after common
// processing, tag filter resolution, and MRF classification, before rendering
// and JSON encoding.
func (p *EndpointFilteredProvider) selectProcessed(msg *message.Message) processor.RoutingDecision {
	reliableMask, optionalMask, evalErrs := p.policy.Evaluate(msg)
	for _, evalErr := range evalErrs {
		tlmEndpointFilterEvalErrors.Inc(p.policy.EndpointID(evalErr.EndpointIndex), strconv.FormatBool(evalErr.Reliable))
		p.report(evalErr)
	}

	token := msg.RoutingToken()
	if reliableMask == 0 && optionalMask == 0 {
		tlmEndpointFilterExcluded.Inc()
		if err := p.progress.Attach(token, routing.Selection{Excluded: true}, nil); err != nil {
			p.progress.CompleteLocal(token)
		}
		return processor.RoutingExcluded
	}

	if reliableMask == 0 {
		// Explicit optional-only path: render and encode once, dispatch
		// non-blockingly to matched optional queues, then complete locally.
		tlmEndpointFilterOptionalOnly.Inc()
		p.setSelection(token, endpointSelection{optionalMask: optionalMask})
		return processor.RoutingContinue
	}

	var idsBuf [8]string
	ids := idsBuf[:0]
	for m := reliableMask; m != 0; {
		idx := bits.TrailingZeros64(m)
		m &= m - 1
		ids = append(ids, p.streams[idx].id)
	}
	if err := p.progress.Attach(token, routing.Selection{}, ids); err != nil {
		p.progress.Invalidate(token)
		p.report(err)
		return processor.RoutingUnresolved
	}
	p.setSelection(token, endpointSelection{
		reliableMask: reliableMask,
		optionalMask: optionalMask,
	})
	return processor.RoutingContinue
}

func (p *EndpointFilteredProvider) dispatchProcessed() {
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
			// Wait off the dispatchProcessed goroutine so a stalled source
			// lane cannot halt dispatch for healthy sources during Flush.
			go func(targets []sourceTarget, waitCtx context.Context, ack chan struct{}) {
				for _, target := range targets {
					target.lane.wait(waitCtx, p.ctx, target.sequence)
				}
				close(ack)
			}(targets, ctx, done)
			continue
		}

		identifier := ""
		if msg.Origin != nil {
			identifier = msg.Origin.Identifier
		}
		sourceIdentifier := identifier
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
				capacity = routedProgressCapacity
			}
			lane = &routedSourceDispatch{
				input:          make(chan *message.Message, capacity),
				advanced:       make(chan struct{}, 1),
				flushRequested: make(chan struct{}, 1),
			}
			p.sourceDispatch[identifier] = lane
			p.sourceDone.Add(1)
			go p.dispatchSource(identifier, sourceIdentifier, lane)
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

func (p *EndpointFilteredProvider) dispatchSource(mapKey, identifier string, lane *routedSourceDispatch) {
	defer p.sourceDone.Done()
	var flushed, stalled uint64
	flushWhileStalled := func() {
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
			token := msg.RoutingToken()
			sel, ok := p.popSelection(token)
			if !ok {
				lane.completed.Add(1)
				select {
				case lane.advanced <- struct{}{}:
				default:
				}
				continue
			}

			// 1. Optional endpoints: strictly non-blocking best-effort dispatch.
			for m := sel.optionalMask; m != 0; {
				idx := bits.TrailingZeros64(m)
				m &= m - 1
				ep := p.streams[idx]
				select {
				case ep.input <- msg:
				default:
					tlmEndpointFilterOptionalDropped.Inc(ep.id)
				}
			}

			// 2. Explicit optional-only path: no reliable obligation, so
			// advance source progress immediately after non-blocking handoff.
			if sel.reliableMask == 0 {
				p.progress.CompleteLocal(token)
			} else {
				// 3. Reliable endpoints: Layout B independent per-endpoint streams
				// with reliable-set semantics (try all matching reliable endpoints;
				// block only when ALL matching reliable endpoints are full).
				p.dispatchReliable(msg, sel.reliableMask, lane.flushRequested, flushWhileStalled)
			}

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
		// Reclaim idle per-source dispatch lanes once all submitted records
		// have been dispatched so source churn cannot retain goroutines/queues.
		p.sourceDispatchMu.Lock()
		if lane.submitted == lane.completed.Load() && p.sourceDispatch[mapKey] == lane {
			delete(p.sourceDispatch, mapKey)
			p.sourceDispatchMu.Unlock()
			return
		}
		p.sourceDispatchMu.Unlock()
	}
}

func (p *EndpointFilteredProvider) dispatchReliable(msg *message.Message, reliableMask uint64, pressure <-chan struct{}, flush func()) {
	// Fast path: single matching reliable endpoint (e.g., US1 primary).
	if bits.OnesCount64(reliableMask) == 1 {
		idx := bits.TrailingZeros64(reliableMask)
		ep := p.streams[idx]
		select {
		case ep.input <- msg:
			return
		default:
		}
		if flush != nil {
			flush()
		}
		for {
			select {
			case ep.input <- msg:
				return
			case <-pressure:
				if flush != nil {
					flush()
				}
			case <-p.ctx.Done():
				return
			}
		}
	}

	// Multiple matching reliable endpoints: try all non-blockingly first.
	var pending []*endpointStream
	accepted := false
	for m := reliableMask; m != 0; {
		idx := bits.TrailingZeros64(m)
		m &= m - 1
		ep := p.streams[idx]
		select {
		case ep.input <- msg:
			accepted = true
		default:
			pending = append(pending, ep)
		}
	}

	if accepted {
		// At least one reliable endpoint accepted; drop full secondary reliable
		// copies so a stalled reliable endpoint cannot block a healthy one.
		if len(pending) > 0 {
			meta := msg.MessageMetadata
			payload := message.NewPayload([]*message.MessageMetadata{&meta}, nil, "", 0)
			for _, ep := range pending {
				tlmEndpointFilterReliableQueueDropped.Inc(ep.id)
				p.progress.FailGroupLocal(ep.id, payload)
			}
		}
		return
	}

	// All matching reliable endpoints are full: apply backpressure until at
	// least one reliable endpoint accepts.
	if flush != nil {
		flush()
	}
	for len(pending) > 0 {
		cases := make([]reflect.SelectCase, 2, len(pending)+2)
		cases[0] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(p.ctx.Done())}
		cases[1] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(pressure)}
		for _, ep := range pending {
			cases = append(cases, reflect.SelectCase{
				Dir:  reflect.SelectSend,
				Chan: reflect.ValueOf(ep.input),
				Send: reflect.ValueOf(msg),
			})
		}
		chosen, _, _ := reflect.Select(cases)
		if chosen == 0 {
			return
		}
		if chosen == 1 {
			if flush != nil {
				flush()
			}
			continue
		}
		// One reliable endpoint accepted; try the remaining ones non-blockingly
		// and release local obligations for any that are still full.
		acceptedIdx := chosen - 2
		meta := msg.MessageMetadata
		payload := message.NewPayload([]*message.MessageMetadata{&meta}, nil, "", 0)
		for i, ep := range pending {
			if i == acceptedIdx {
				continue
			}
			select {
			case ep.input <- msg:
			default:
				tlmEndpointFilterReliableQueueDropped.Inc(ep.id)
				p.progress.FailGroupLocal(ep.id, payload)
			}
		}
		return
	}
}

func (p *EndpointFilteredProvider) dispatchPayloads(stream *endpointStream) {
	defer p.dispatchDone.Done()
	if stream.reliable {
		in := stream.reliableSender.In()
		for payload := range stream.output {
			select {
			case in <- payload:
			case <-p.ctx.Done():
				// Drain remaining batcher output without acknowledging so
				// strategy.Stop() never blocks on a full output channel.
				for range stream.output {
				}
				return
			}
		}
		return
	}
	for payload := range stream.output {
		if !stream.optionalDest.NonBlockingSend(payload) {
			tlmEndpointFilterOptionalPayloadsDropped.Inc(stream.id)
		}
	}
}

// SourceInput creates or returns an isolated input channel bound to one
// checkpointed source producer generation.
func (p *EndpointFilteredProvider) SourceInput(identifier string, generation uint64) (chan *message.Message, error) {
	if identifier == "" || generation == 0 {
		return nil, routing.ErrInvalidSelection
	}
	p.lifecycle.Lock()
	defer p.lifecycle.Unlock()
	if p.stopped {
		return nil, routing.ErrStopped
	}
	key := sourceInputKey{identifier: identifier, generation: generation}
	if mi, ok := p.sourceInputs[key]; ok {
		return mi.ch, nil
	}
	if err := p.progress.RegisterSource(identifier, generation); err != nil {
		return nil, err
	}
	mi := newManagedInput(make(chan *message.Message, routedIngressCapacity))
	p.sourceInputs[key] = mi
	p.inputKeys[mi] = key
	p.inputs = append(p.inputs, mi)
	if p.started {
		p.ingressDone.Add(1)
		go p.admit(mi.ch, &key)
	}
	return mi.ch, nil
}

// RetireSource retires a bound producer generation after flushing its input
// channel. If records from this generation are still in flight, retirement is
// finalized automatically once they drain so generation slots never leak.
func (p *EndpointFilteredProvider) RetireSource(ctx context.Context, identifier string, generation uint64) error {
	if identifier == "" {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	p.lifecycle.RLock()
	if p.stopped {
		p.lifecycle.RUnlock()
		return routing.ErrStopped
	}
	key := sourceInputKey{identifier: identifier, generation: generation}
	mi := p.sourceInputs[key]
	started := p.started
	p.opsWg.Add(1)
	p.lifecycle.RUnlock()
	defer p.opsWg.Done()

	var claimed bool
	if mi != nil {
		claimed = mi.claimRetire()
		if !claimed {
			// Another concurrent RetireSource already owns this input.
			return nil
		}
		if started {
			done := make(chan struct{})
			marker := &message.Message{}
			marker.SetRoutingBarrier(done)
			select {
			case mi.ch <- marker:
			case <-ctx.Done():
				return ctx.Err()
			case <-p.stopRequested:
				return routing.ErrStopped
			case <-p.ctx.Done():
				return routing.ErrStopped
			}
			select {
			case <-done:
			case <-ctx.Done():
				return ctx.Err()
			case <-p.stopRequested:
				return routing.ErrStopped
			case <-p.ctx.Done():
				return routing.ErrStopped
			}
		}
	}

	err := p.progress.RetireSourceWhenDrained(identifier, generation)

	// Reclaim the retired bound input channel once existing Flush senders exit.
	if claimed {
		p.lifecycle.Lock()
		if !p.stopped {
			if existing, ok := p.sourceInputs[key]; ok && existing == mi {
				delete(p.sourceInputs, key)
				delete(p.inputKeys, mi)
				for i, entry := range p.inputs {
					if entry == mi {
						p.inputs = append(p.inputs[:i], p.inputs[i+1:]...)
						break
					}
				}
			}
		}
		p.lifecycle.Unlock()
		mi.waitSendersAndClose()
	}
	return err
}

// NextPipelineChan returns the shared ingress channel.
func (p *EndpointFilteredProvider) NextPipelineChan() chan *message.Message {
	return p.ingress.ch
}

// GetOutputChan returns nil (matching the default provider).
func (p *EndpointFilteredProvider) GetOutputChan() chan *message.Message {
	return nil
}

// NextPipelineChanWithMonitor returns the shared ingress channel and capacity monitor.
func (p *EndpointFilteredProvider) NextPipelineChanWithMonitor() (chan *message.Message, *metrics.CapacityMonitor) {
	return p.ingress.ch, p.monitor.GetCapacityMonitor(metrics.ProcessorTlmName, "endpoint-filter-0")
}

// GetPipelineMonitor returns the pipeline monitor for this provider.
func (p *EndpointFilteredProvider) GetPipelineMonitor() metrics.PipelineMonitor {
	return p.monitor
}

// Flush flushes all admitted records through processors and Layout B batchers.
func (p *EndpointFilteredProvider) Flush(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	p.lifecycle.RLock()
	if !p.started || p.stopped {
		p.lifecycle.RUnlock()
		return
	}
	inputs := append([]*managedInput(nil), p.inputs...)
	p.opsWg.Add(1)
	p.lifecycle.RUnlock()
	defer p.opsWg.Done()

	var acks []chan struct{}
	for _, mi := range inputs {
		if !mi.acquire() {
			continue
		}
		ack := make(chan struct{})
		marker := &message.Message{}
		marker.SetRoutingBarrier(ack)
		select {
		case mi.ch <- marker:
			mi.release()
			acks = append(acks, ack)
		case <-ctx.Done():
			mi.release()
			return
		case <-p.stopRequested:
			mi.release()
			return
		case <-p.ctx.Done():
			mi.release()
			return
		}
	}
	for _, ack := range acks {
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

	acks = acks[:0]
	for _, proc := range p.processors {
		ack := make(chan struct{})
		marker := &message.Message{}
		marker.SetRoutingBarrier(ack)
		marker.SetRoutingBarrierContext(ctx)
		select {
		case proc.InputChan() <- marker:
			acks = append(acks, ack)
		case <-ctx.Done():
			return
		case <-p.stopRequested:
			return
		case <-p.ctx.Done():
			return
		}
	}
	for _, ack := range acks {
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

	acks = acks[:0]
	for _, stream := range p.streams {
		ack := make(chan struct{})
		marker := &message.Message{}
		marker.SetRoutingBarrier(ack)
		marker.SetRoutingBarrierContext(ctx)
		select {
		case stream.input <- marker:
			acks = append(acks, ack)
		case <-ctx.Done():
			return
		case <-p.stopRequested:
			return
		case <-p.ctx.Done():
			return
		}
	}
	for _, ack := range acks {
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

// Stop gracefully drains and shuts down all processors, batchers, senders, and progress state.
func (p *EndpointFilteredProvider) Stop() {
	p.stopOnce.Do(func() {
		p.lifecycle.Lock()
		started := p.started
		p.stopped = true
		close(p.stopRequested)
		inputs := append([]*managedInput(nil), p.inputs...)
		p.lifecycle.Unlock()

		// Arm the grace timer before waiting on in-flight operations so a
		// blocked checkpoint sink holding progress.mu cannot prevent shutdown.
		timer := time.AfterFunc(routedStopGrace, p.cancel)
		defer timer.Stop()

		// Wait for any in-flight Flush or RetireSource calls to observe
		// stopRequested and exit before closing any channels they send on.
		p.opsWg.Wait()

		if !started {
			p.cancel()
			for _, stream := range p.streams {
				if stream.groupSink != nil {
					stream.groupSink.Stop()
				}
				if stream.optionalDest != nil {
					stream.optionalDest.Stop()
					close(stream.optionalNoop)
				}
			}
			p.progress.Close()
			p.selectionsMu.Lock()
			clear(p.selections)
			p.selectionsMu.Unlock()
			return
		}

		p.monitor.Stop()

		for _, mi := range inputs {
			mi.closeOnce()
		}
		p.ingressDone.Wait()
		p.sharedParkWg.Wait()

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

		for _, stream := range p.streams {
			if stream.reliable {
				stream.reliableSender.Stop()
				stream.groupSink.Stop()
			} else {
				stream.optionalDest.Stop()
				close(stream.optionalNoop)
			}
		}

		// Drain already-completed checkpoints to the auditor before cancelling
		// the context; the grace timer still interrupts forwarding if wedged.
		p.progress.DrainAndClose()
		p.cancel()
		p.destinationsCtx.Stop()
		p.selectionsMu.Lock()
		clear(p.selections)
		p.selectionsMu.Unlock()
	})
}
