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
	space        chan struct{}
	ready        chan *message.Payload
	shutdown     chan struct{}
	shutdownOnce sync.Once
	finished     chan struct{}
	sink         sender.Sink
	stopped      bool
}

type record struct {
	meta       *message.MessageMetadata
	identifier string
	generation uint64
	groups     []string
	remaining  map[string]struct{}
	done       bool
}

type sourceProgress struct {
	currentGeneration uint64
	seenGenerations   map[uint64]bool // true after that producer retires
	queue             []*record
}

// NewProgress creates bounded in-memory progress state and a bounded queue to
// the existing auditor. Call Close after all group senders and sinks have stopped.
func NewProgress(limit int, sink sender.Sink) (*Progress, error) {
	if limit < 1 || sink == nil {
		return nil, errors.New("invalid routing progress capacity or sink")
	}
	p := &Progress{
		limit: limit, sourceLimit: min(limit, SourcePendingLimit), records: make(map[uint64]*record),
		sources: make(map[string]*sourceProgress), space: make(chan struct{}),
		ready: make(chan *message.Payload, limit), shutdown: make(chan struct{}), finished: make(chan struct{}), sink: sink,
	}
	go p.forward()
	return p, nil
}

func (p *Progress) forward() {
	defer close(p.finished)
	for payload := range p.ready {
		// The Agent constructs its provider before it starts the auditor. This
		// channel is acquired only when terminal progress is ready to publish.
		select {
		case p.sink.Channel() <- payload:
		case <-p.shutdown:
			return // A blocked sink must not prevent shutdown.
		}
	}
}

// Register reserves a record before processing lanes can reorder it. A
// checkpointed source must supply a nonzero generation. It returns only after
// capacity is reserved, or with no ownership transfer on error. Older, unretired
// producers may drain without reclaiming current checkpoint ownership; retired
// producers return ErrObsoleteGeneration.
func (p *Progress) Register(ctx context.Context, msg *message.Message, selection Selection, groups []string) (uint64, error) {
	return p.register(ctx, msg, selection, groups, true)
}

func (p *Progress) register(ctx context.Context, msg *message.Message, selection Selection, groups []string, requireCompleteSource bool) (uint64, error) {
	if msg == nil || msg.Origin == nil {
		return 0, fmt.Errorf("%w: missing source metadata", ErrInvalidSelection)
	}
	identifier := msg.Origin.Identifier
	if requireCompleteSource && identifier != "" && (selection.Generation == 0 || msg.Origin.LogSource == nil) {
		return 0, fmt.Errorf("%w: checkpointed source needs a generation and source", ErrInvalidSelection)
	}
	if selection.Excluded && len(groups) != 0 {
		return 0, ErrInvalidSelection
	}
	if !selection.Excluded && len(groups) == 0 {
		return 0, ErrInvalidSelection
	}
	remaining := make(map[string]struct{}, len(groups))
	for _, id := range groups {
		remaining[id] = struct{}{}
	}
	for {
		p.mu.Lock()
		if p.stopped {
			p.mu.Unlock()
			return 0, ErrStopped
		}
		source := p.sources[identifier]
		if identifier != "" && requireCompleteSource && source != nil {
			if retired, seen := source.seenGenerations[selection.Generation]; seen && retired {
				p.mu.Unlock()
				return 0, fmt.Errorf("%w: %d", ErrObsoleteGeneration, selection.Generation)
			}
		}
		needsOwner := identifier != "" && source == nil
		needsHistory := identifier != "" && requireCompleteSource
		if source != nil {
			_, seen := source.seenGenerations[selection.Generation]
			needsHistory = requireCompleteSource && !seen
		}
		sourceHasRoom := identifier == "" || source == nil || len(source.queue) < p.sourceLimit
		if len(p.records) < p.limit && sourceHasRoom && (!needsOwner || len(p.sources) < p.limit) && (!needsHistory || p.histories < p.limit) {
			break
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
	r := &record{meta: &meta, identifier: identifier, generation: selection.Generation, groups: append([]string(nil), groups...), remaining: remaining, done: selection.Excluded}
	p.records[token] = r
	if identifier != "" {
		source := p.sources[identifier]
		if source == nil {
			source = &sourceProgress{seenGenerations: make(map[uint64]bool)}
			p.sources[identifier] = source
		}
		if requireCompleteSource {
			if _, seen := source.seenGenerations[selection.Generation]; !seen {
				p.histories++
				source.currentGeneration = selection.Generation
				source.seenGenerations[selection.Generation] = false
			}
		}
		source.queue = append(source.queue, r)
	}
	if r.done {
		p.advance(r)
	}
	p.mu.Unlock()
	return token, nil
}

// HoldInvalid reserves an unresolved source barrier after a selection failure.
// Later valid records cannot move a checkpoint past this record. The empty
// sentinel is not a delivery group and no sender can resolve it. Generation
// zero distinguishes this source barrier from every valid producer generation.
func (p *Progress) HoldInvalid(ctx context.Context, msg *message.Message) error {
	if msg == nil || msg.Origin == nil || msg.Origin.Identifier == "" {
		return nil
	}
	_, err := p.register(ctx, msg, Selection{}, []string{""}, false)
	return err
}

// Recipients returns the immutable selection made before processor distribution.
func (p *Progress) Recipients(token uint64) []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if r := p.records[token]; r != nil {
		return append([]string(nil), r.groups...)
	}
	return nil
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

// CompleteGroup resolves only the named group's obligations from a terminal
// reliable sender output. It does not infer success from queue admission.
func (p *Progress) CompleteGroup(group string, payload *message.Payload) {
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
		if r == nil || r.done {
			continue
		}
		if _, selected := r.remaining[group]; !selected {
			continue
		}
		delete(r.remaining, group)
		if len(r.remaining) == 0 {
			r.done = true
			p.advance(r)
		}
	}
}

// advance releases only a completed source prefix. Records from an obsolete
// generation may release resources but cannot write the replacement checkpoint.
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
}

// RetireSource marks one producer stopped after all its decoder output has
// been admitted. A drained queue alone is insufficient evidence of this. Old
// generation retirement does not release newer ownership; history is released
// only when every generation for the identifier has retired and the queue is
// empty. Callers must not retire an active producer.
func (p *Progress) RetireSource(identifier string, generation uint64) error {
	if identifier == "" {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
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
		if record.generation == generation {
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
	close(p.space)
	p.space = make(chan struct{})
}

// Pending is the count of registered records retaining delivery obligations.
func (p *Progress) Pending() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.records)
}

// OwnsSource reports whether any active generation still owns this identifier.
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
// only after every group sink has stopped producing outcomes.
func (p *Progress) Close() {
	p.StopForwarding()
	p.mu.Lock()
	if !p.stopped {
		p.stopped = true
		close(p.space)
		close(p.ready)
		p.records = nil
		p.sources = nil
	}
	p.mu.Unlock()
	<-p.finished
}

// GroupSink intercepts terminal output from one group's reliable destinations.
// Multiple optional reliable endpoints can report the same payload; completion
// is idempotent within the group.
type GroupSink struct {
	group    string
	input    chan *message.Payload
	done     chan struct{}
	progress *Progress
}

// NewGroupSink creates a bounded completion adapter for one group.
func (p *Progress) NewGroupSink(group string, capacity int) *GroupSink {
	if capacity < 1 {
		capacity = 1
	}
	s := &GroupSink{group: group, input: make(chan *message.Payload, capacity), done: make(chan struct{}), progress: p}
	go func() {
		defer close(s.done)
		for payload := range s.input {
			p.CompleteGroup(group, payload)
		}
	}()
	return s
}

// Channel implements sender.Sink.
func (s *GroupSink) Channel() chan *message.Payload { return s.input }

// Stop drains outcomes after the corresponding sender has stopped.
func (s *GroupSink) Stop() {
	close(s.input)
	<-s.done
}
