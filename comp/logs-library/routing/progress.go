// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package routing

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/comp/logs-library/sender"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

var (
	ErrObsoleteGeneration = errors.New("log routing source generation retired")
	ErrStopped            = errors.New("log routing progress stopped")
	ErrSourcePending      = errors.New("log routing source still has pending records")
	ErrSourceCapacity     = errors.New("log routing source ownership capacity exhausted")
	ErrUnknownGeneration  = errors.New("log routing source generation not owned")
)

// SourcePendingLimit bounds retained records from one checkpointed source.
// It is smaller than the routed stream input so one stalled source cannot
// fill that stage on its own.
const SourcePendingLimit = 64

// Progress coordinates terminal outcomes for original records. Its capacity
// counts record references, not log bodies. Full capacity backpressures callers
// before ownership transfers; it never acknowledges a rejected registration.
type Progress struct {
	mu           sync.Mutex
	limit        int
	sourceLimit  int
	next         uint64
	records      map[uint64]*record
	sources      map[string]*sourceProgress
	histories    int
	space        chan struct{} // Allocated when admission must wait; guarded by mu.
	ready        chan *message.Payload
	shutdown     chan struct{}
	shutdownOnce sync.Once
	finished     chan struct{}
	sink         sender.Sink
	stopped      bool
}

type record struct {
	meta                   *message.MessageMetadata
	identifier             string
	generation             uint64
	reliable               []string
	optional               []string
	remaining              map[string]struct{}
	reliableSelectorFailed bool
	reliableDelivered      chan struct{} // Only multi-endpoint sets can leave sibling offers waiting.
	selectionPending       bool
	done                   bool
}

type sourceProgress struct {
	currentGeneration  uint64
	seenGenerations    map[uint64]bool // true after that producer retires
	queue              []*record
	hasAdmittedRecords bool // Remains true after the queue drains.
	invalidSelection   bool // Checkpoints cannot advance past an invalid barrier.
}

// NewProgress creates bounded in-memory progress state and a bounded queue to
// the existing auditor. A nil sink channel disables checkpoint publication, as
// in the default sender. Start a real auditor before processing records. Call
// Close after all endpoint senders and sinks have stopped.
func NewProgress(limit int, sink sender.Sink) (*Progress, error) {
	if limit < 1 || sink == nil {
		return nil, errors.New("invalid routing progress capacity or sink")
	}
	p := &Progress{
		limit: limit, sourceLimit: min(limit, SourcePendingLimit), records: make(map[uint64]*record),
		sources: make(map[string]*sourceProgress),
		ready:   make(chan *message.Payload, limit), shutdown: make(chan struct{}), finished: make(chan struct{}), sink: sink,
	}
	go p.forward()
	return p, nil
}

func (p *Progress) forward() {
	defer close(p.finished)
	for payload := range p.ready {
		// The Agent constructs its provider before it starts the auditor. This
		// channel is acquired only when terminal progress is ready to publish.
		output := p.sink.Channel()
		if output == nil {
			continue // Match the default sender's no-op sink behavior.
		}
		select {
		case output <- payload:
		case <-p.shutdown:
			return // A blocked sink must not prevent shutdown.
		}
	}
}

// RegisterSource reserves bounded generation ownership before producer input is
// accepted. This includes producers with no valid records yet. A new generation
// becomes current here; older registered producers may continue draining.
func (p *Progress) RegisterSource(identifier string, generation uint64) error {
	if identifier == "" || generation == 0 {
		return ErrInvalidSelection
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped {
		return ErrStopped
	}
	source := p.sources[identifier]
	if source != nil {
		if retired, seen := source.seenGenerations[generation]; seen {
			if retired {
				return ErrObsoleteGeneration
			}
			return nil
		}
	}
	if (source == nil && len(p.sources) >= p.limit) || p.histories >= p.limit {
		return ErrSourceCapacity
	}
	if source == nil {
		source = &sourceProgress{seenGenerations: make(map[uint64]bool)}
		p.sources[identifier] = source
	}
	source.currentGeneration = generation
	source.seenGenerations[generation] = false
	p.histories++
	return nil
}

func (p *Progress) reserve(ctx context.Context, msg *message.Message, generation uint64, validSource bool) (uint64, error) {
	if msg == nil || msg.Origin == nil {
		return 0, fmt.Errorf("%w: missing source metadata", ErrInvalidSelection)
	}
	identifier := msg.Origin.Identifier
	if validSource && identifier != "" && (generation == 0 || msg.Origin.LogSource == nil) {
		return 0, fmt.Errorf("%w: checkpointed source needs a generation and source", ErrInvalidSelection)
	}
	for {
		p.mu.Lock()
		if p.stopped {
			p.mu.Unlock()
			return 0, ErrStopped
		}
		source := p.sources[identifier]
		if identifier != "" && validSource && source != nil {
			if retired, seen := source.seenGenerations[generation]; seen && retired {
				p.mu.Unlock()
				return 0, fmt.Errorf("%w: %d", ErrObsoleteGeneration, generation)
			}
		}
		needsOwner := identifier != "" && source == nil
		needsHistory := identifier != "" && validSource
		if source != nil {
			_, seen := source.seenGenerations[generation]
			needsHistory = validSource && !seen
		}
		sourceHasRoom := identifier == "" || source == nil || len(source.queue) < p.sourceLimit
		if len(p.records) < p.limit && sourceHasRoom && (!needsOwner || len(p.sources) < p.limit) && (!needsHistory || p.histories < p.limit) {
			break
		}
		if p.space == nil {
			p.space = make(chan struct{})
		}
		space := p.space
		p.mu.Unlock()
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-space:
		}
	}
	p.next++
	token := p.next
	meta := msg.MessageMetadata
	meta.SetRoutingToken(token)
	msg.SetRoutingToken(token)
	r := &record{meta: &meta, identifier: identifier, generation: generation, selectionPending: validSource}
	p.records[token] = r
	if identifier != "" {
		source := p.sources[identifier]
		if source == nil {
			source = &sourceProgress{seenGenerations: make(map[uint64]bool)}
			p.sources[identifier] = source
		}
		if validSource {
			if _, seen := source.seenGenerations[generation]; !seen {
				p.histories++
				source.currentGeneration = generation
				source.seenGenerations[generation] = false
			}
		}
		source.hasAdmittedRecords = true
		if r.generation == 0 {
			source.invalidSelection = true
		}
		source.queue = append(source.queue, r)
	}
	p.mu.Unlock()
	return token, nil
}

// HoldInvalid reserves an unresolved source barrier after a selection failure.
// Later valid records cannot move a checkpoint past this record. Generation
// zero distinguishes this source barrier from every valid producer generation.
func (p *Progress) HoldInvalid(ctx context.Context, msg *message.Message) error {
	if msg == nil || msg.Origin == nil || msg.Origin.Identifier == "" {
		return nil
	}
	_, err := p.reserve(ctx, msg, 0, false)
	return err
}

// Reserve claims capacity and source order before processing or selection. No
// delivery or local completion is inferred while selection is pending. A
// checkpointed source must supply a nonzero generation; older, unretired
// producers can drain without reclaiming current checkpoint ownership.
func (p *Progress) Reserve(ctx context.Context, msg *message.Message, generation uint64) (uint64, error) {
	return p.reserve(ctx, msg, generation, true)
}

// AtCapacity reports pressure on the source window and aggregate record budget.
// Only terminal outcomes can release reservations.
func (p *Progress) AtCapacity(identifier string) (sourceFull, aggregateFull bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped {
		return false, false
	}
	source := p.sources[identifier]
	return identifier != "" && source != nil && len(source.queue) == p.sourceLimit, len(p.records) == p.limit
}

// PendingReliableEndpoints returns this source's unresolved recipients for a pressure
// flush. Pending selection and invalid barriers do not imply recipients.
func (p *Progress) PendingReliableEndpoints(identifier string) []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	seen := make(map[string]struct{})
	var endpoints []string
	add := func(r *record) {
		if r.done || r.selectionPending {
			return
		}
		for id := range r.remaining {
			if _, duplicate := seen[id]; !duplicate {
				seen[id] = struct{}{}
				endpoints = append(endpoints, id)
			}
		}
	}
	if identifier != "" {
		if source := p.sources[identifier]; source != nil {
			for _, r := range source.queue {
				add(r)
			}
		}
	} else {
		for _, r := range p.records {
			if r.identifier == "" {
				add(r)
			}
		}
	}
	return endpoints
}

// Invalidate turns the same pending reservation into a source-owned barrier.
// Generation zero lets the producer retire without releasing the source barrier.
func (p *Progress) Invalidate(token uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if r := p.records[token]; r != nil && r.selectionPending && !r.done {
		if r.identifier == "" {
			// Anonymous failures have no checkpoint to protect, as with HoldInvalid.
			delete(p.records, token)
			p.notifySpace()
			return
		}
		r.generation = 0
		r.selectionPending = false
		p.sources[r.identifier].invalidSelection = true
		p.advance(r)
	}
}

// AttachEndpointSelection attaches endpoint-filtering recipients once after
// common processing. Only reliable endpoints enter remaining obligations;
// optional endpoints can never satisfy or release mandatory progress.
// The dispatcher completes optional-only records after it offers their copies.
// A reliable selector error leaves a possible match unknown.
func (p *Progress) AttachEndpointSelection(token uint64, reliable, optional []string, reliableSelectorFailed bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	r := p.records[token]
	if r == nil || r.done || !r.selectionPending || len(reliable)+len(optional) == 0 {
		return ErrInvalidSelection
	}
	r.reliableSelectorFailed = reliableSelectorFailed
	if len(reliable) > 1 {
		r.reliableDelivered = make(chan struct{})
	}
	r.reliable = append([]string(nil), reliable...)
	if len(optional) != 0 {
		r.optional = append([]string(nil), optional...)
	}
	r.remaining = make(map[string]struct{}, len(reliable))
	for _, id := range reliable {
		r.remaining[id] = struct{}{}
	}
	r.selectionPending = false
	return nil
}

// EndpointRecipients returns endpoint IDs and a signal for the first reliable
// delivery. For fewer than two reliable matches the signal is nil.
func (p *Progress) EndpointRecipients(token uint64) (reliable, optional []string, delivered <-chan struct{}) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if r := p.records[token]; r != nil {
		return append([]string(nil), r.reliable...), append([]string(nil), r.optional...), r.reliableDelivered
	}
	return nil, nil, nil
}

// CompleteReliableSuccess resolves a reliable-set record once any matching
// reliable endpoint succeeds ("at least one successful delivery satisfies the
// reliable-set rule").
func (p *Progress) CompleteReliableSuccess(endpointID string, payload *message.Payload) {
	if payload == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, meta := range payload.MessageMetas {
		if meta == nil {
			continue
		}
		r := p.records[meta.RoutingToken()]
		if r == nil || r.done || r.selectionPending {
			continue
		}
		if _, selected := r.remaining[endpointID]; !selected {
			continue
		}
		clear(r.remaining)
		r.done = true
		if r.reliableDelivered != nil {
			close(r.reliableDelivered)
		}
		p.advance(r)
	}
}

// CompleteReliableTerminalLoss records one matching endpoint's terminal loss.
// All losses complete the record as a drop only when reliable selection succeeded.
// A loss does not count as delivery success.
func (p *Progress) CompleteReliableTerminalLoss(endpointID string, payload *message.Payload) {
	if payload == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, meta := range payload.MessageMetas {
		if meta == nil {
			continue
		}
		r := p.records[meta.RoutingToken()]
		if r == nil || r.done || r.selectionPending {
			continue
		}
		if _, selected := r.remaining[endpointID]; !selected {
			continue
		}
		delete(r.remaining, endpointID)
		if len(r.remaining) == 0 && !r.reliableSelectorFailed {
			r.done = true
			p.advance(r)
		}
	}
}

// CompleteLocal resolves an intentional exclusion or a terminal processing
// loss before fanout. Repeated notifications are harmless.
func (p *Progress) CompleteLocal(token uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if r := p.records[token]; r != nil && !r.done {
		clear(r.remaining)
		r.done = true
		p.advance(r)
	}
}

// advance publishes only a completed source prefix. Records from an obsolete
// generation may release resources but cannot write the replacement checkpoint.
// Completed records behind an invalid barrier release capacity without publishing.
func (p *Progress) advance(r *record) {
	if r.identifier == "" {
		delete(p.records, r.meta.RoutingToken())
		p.notifySpace()
		return
	}
	source := p.sources[r.identifier]
	for len(source.queue) > 0 && source.queue[0].done {
		front := source.queue[0]
		source.queue[0] = nil
		source.queue = source.queue[1:]
		delete(p.records, front.meta.RoutingToken())
		if front.generation == source.currentGeneration {
			select {
			case p.ready <- message.NewPayload([]*message.MessageMetadata{front.meta}, nil, "", 0):
			case <-p.shutdown:
			}
		}
		p.notifySpace()
	}
	if !source.invalidSelection {
		return
	}
	// An invalid barrier can never resolve. Retain it and unfinished records,
	// but completed records after it can no longer contribute a checkpoint.
	blocked := false
	retained := source.queue[:0]
	for _, queued := range source.queue {
		if queued.generation == 0 {
			blocked = true
		}
		if blocked && queued.done {
			delete(p.records, queued.meta.RoutingToken())
			continue
		}
		retained = append(retained, queued)
	}
	if len(retained) != len(source.queue) {
		clear(source.queue[len(retained):])
		source.queue = retained
		p.notifySpace()
	}
}

// RetireSource marks one producer stopped after all its decoder output has
// been admitted. A drained queue alone is insufficient evidence of this. Old
// generation retirement does not release newer ownership; history is released
// only when every generation for the identifier has retired and the queue is
// empty. Invalid-selection barriers do not prevent producer retirement.
// Callers must not retire an active producer.
func (p *Progress) RetireSource(identifier string, generation uint64) error {
	if identifier == "" {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.retireSource(identifier, generation)
}

// TryRetireSource retires a producer without waiting for checkpoint publication
// to release the progress lock. ErrSourcePending means the caller should retry.
// On success, owned reports whether another producer or barrier still owns the
// identifier. Both retirement and the ownership check happen under the same lock.
func (p *Progress) TryRetireSource(identifier string, generation uint64) (owned bool, err error) {
	if identifier == "" {
		return false, nil
	}
	if !p.mu.TryLock() {
		return true, ErrSourcePending
	}
	defer p.mu.Unlock()
	err = p.retireSource(identifier, generation)
	return p.sources[identifier] != nil, err
}

// retireSource requires p.mu to be held.
func (p *Progress) retireSource(identifier string, generation uint64) error {
	if p.stopped {
		return ErrStopped
	}
	source := p.sources[identifier]
	if source == nil {
		return ErrUnknownGeneration
	}
	retired, seen := source.seenGenerations[generation]
	if !seen {
		return ErrUnknownGeneration
	}
	if retired {
		return nil
	}
	for _, record := range source.queue {
		if record.generation == generation && !record.done {
			return ErrSourcePending
		}
	}
	source.seenGenerations[generation] = true
	for _, retired := range source.seenGenerations {
		if !retired {
			return nil
		}
	}
	if len(source.queue) != 0 {
		return nil // An invalid-selection barrier still owns this identifier.
	}
	p.histories -= len(source.seenGenerations)
	delete(p.sources, identifier)
	p.notifySpace()
	return nil
}

func (p *Progress) notifySpace() {
	if p.space != nil {
		close(p.space)
		p.space = nil
	}
}

// Pending counts records retained for delivery or ordered checkpoint progress.
func (p *Progress) Pending() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.records)
}

// OwnsSource reports whether producer ownership or an unresolved source barrier
// still protects this identifier.
func (p *Progress) OwnsSource(identifier string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.sources[identifier] != nil
}

// StopForwarding interrupts checkpoint publication when the downstream auditor
// is blocked. Call it on provider cancellation, before waiting for workers that
// may be publishing outcomes. Unpublished checkpoints are left for replay.
func (p *Progress) StopForwarding() {
	p.shutdownOnce.Do(func() { close(p.shutdown) })
}

// Close releases unresolved reservations without acknowledging them. Call it
// only after every endpoint sink has stopped producing outcomes.
func (p *Progress) Close() {
	p.StopForwarding()
	p.mu.Lock()
	if !p.stopped {
		p.stopped = true
		p.notifySpace()
		close(p.ready)
		p.records = nil
		p.sources = nil
	}
	p.mu.Unlock()
	<-p.finished
}

// EndpointSink implements sender.OutcomeSink for one reliable endpoint target,
// keeping delivery success distinct from permanent HTTP rejection.
type EndpointSink struct {
	endpointID   string
	success      chan *message.Payload
	terminalLoss chan *message.Payload
	done         sync.WaitGroup
	progress     *Progress
}

// NewEndpointSink creates a bounded outcome adapter for one reliable endpoint.
func (p *Progress) NewEndpointSink(endpointID string, capacity int) *EndpointSink {
	if capacity < 1 {
		capacity = 1
	}
	s := &EndpointSink{
		endpointID:   endpointID,
		success:      make(chan *message.Payload, capacity),
		terminalLoss: make(chan *message.Payload, capacity),
		progress:     p,
	}
	s.done.Add(2)
	go func() {
		defer s.done.Done()
		for payload := range s.success {
			p.CompleteReliableSuccess(endpointID, payload)
		}
	}()
	go func() {
		defer s.done.Done()
		for payload := range s.terminalLoss {
			p.CompleteReliableTerminalLoss(endpointID, payload)
		}
	}()
	return s
}

// Channel implements sender.Sink for delivery successes.
func (s *EndpointSink) Channel() chan *message.Payload { return s.success }

// TerminalLossChannel implements sender.OutcomeSink for permanent rejections.
func (s *EndpointSink) TerminalLossChannel() chan *message.Payload { return s.terminalLoss }

// Stop drains both outcome channels after the corresponding sender has stopped.
func (s *EndpointSink) Stop() {
	close(s.success)
	close(s.terminalLoss)
	s.done.Wait()
}
