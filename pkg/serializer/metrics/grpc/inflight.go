// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	"sort"

	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/statefulpb"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// BatchID constants. PoC uses non-lazy snapshot per contract.md D6 — batch_id=0
// always carries the FULL accumulated dict as the snapshot bootstrap, and batch
// IDs reset to (0, then 1, 2, ...) on every stream rotation.
const (
	snapshotBatchID  uint32 = 0
	firstRealBatchID uint32 = 1
)

// inflightTracker is a bounded FIFO queue that tracks payloads in two regions:
//
//	[--sent awaiting ack--][--buffered not sent--]
//	^                      ^                      ^
//	head                 sentTail                 tail
//
// Sent payloads have sequential batchIDs starting at headBatchID and counting
// up. The tracker also owns the per-stream snapshotState — the accumulated set
// of dict definitions that have been acknowledged by intake. The snapshot is
// updated when payloads are popped on ack; on stream rotation, the full
// snapshot is serialized as batch_id=0.
//
// Compared to the logs equivalent (pkg/logs/sender/grpc/inflight.go), this is
// substantially simpler: the lazy-snapshot machinery introduced in commit
// a9142703ca7 is NOT adopted (see phase3-proto-proposal.md §9 / contract.md
// D6). No streamSent tracking, no nextToSendEncoded prefix injection, no
// reference-walking. The PoC accepts the cost of full-dict re-emission on
// rotation (~6 B/s amortized) in exchange for simplicity and decoupling from
// the un-reviewed lazy work.
type inflightTracker struct {
	laneID         string
	items          []*Payload
	head           int    // oldest sent item (awaiting ack)
	sentTail       int    // first buffered item not yet sent
	tail           int    // next free slot
	cap            int    // capacity (waste-one-slot, so items is cap+1 long)
	headBatchID    uint32 // batchID of the payload at head
	batchIDCounter uint32 // next batchID to assign when markSent is called
	snapshot       *snapshotState
}

func newInflightTracker(laneID string, capacity int) *inflightTracker {
	return &inflightTracker{
		laneID:         laneID,
		items:          make([]*Payload, capacity+1),
		cap:            capacity,
		snapshot:       newSnapshotState(),
		headBatchID:    firstRealBatchID,
		batchIDCounter: firstRealBatchID,
	}
}

// hasSpace returns true if there is at least one free slot for a new buffered
// payload.
func (t *inflightTracker) hasSpace() bool {
	return t.totalCount() < t.cap
}

// hasUnacked returns true if there are sent payloads awaiting ack.
func (t *inflightTracker) hasUnacked() bool {
	return t.head != t.sentTail
}

// hasUnSent returns true if there are buffered payloads not yet sent.
func (t *inflightTracker) hasUnSent() bool {
	return t.sentTail != t.tail
}

// sentCount returns the number of sent payloads awaiting ack.
func (t *inflightTracker) sentCount() int {
	return (t.sentTail - t.head + len(t.items)) % len(t.items)
}

// totalCount returns the total number of tracked payloads (sent + buffered).
func (t *inflightTracker) totalCount() int {
	return (t.tail - t.head + len(t.items)) % len(t.items)
}

// getHeadBatchID returns the expected batchID at the head (oldest sent).
// Caller must check hasUnacked() first.
func (t *inflightTracker) getHeadBatchID() uint32 {
	return t.headBatchID
}

// nextBatchID returns the batchID that will be assigned to the next-sent
// item. Idempotent peek.
func (t *inflightTracker) nextBatchID() uint32 {
	return t.batchIDCounter
}

// append adds a new payload to the buffered region. Returns true if added,
// false if the tracker is full.
func (t *inflightTracker) append(payload *Payload) bool {
	if !t.hasSpace() {
		return false
	}
	t.items[t.tail] = payload
	tlmInflightSize.Add(float64(len(payload.Encoded)), t.laneID)
	t.tail = (t.tail + 1) % len(t.items)
	return true
}

// markSent moves the next buffered payload to the sent region and assigns it
// the next batchID. Returns true on success, false if there are no buffered
// payloads.
func (t *inflightTracker) markSent() bool {
	if t.sentTail == t.tail {
		return false
	}
	// If this is the first sent item (head == sentTail before increment),
	// align headBatchID with the counter we're about to consume.
	if t.head == t.sentTail {
		t.headBatchID = t.batchIDCounter
	}
	t.sentTail = (t.sentTail + 1) % len(t.items)
	t.batchIDCounter++
	return true
}

// nextToSend returns the next buffered payload (peek; does not mutate).
// Returns nil if there are no buffered payloads.
func (t *inflightTracker) nextToSend() *Payload {
	if t.sentTail == t.tail {
		return nil
	}
	return t.items[t.sentTail]
}

// pop removes and returns the oldest sent payload (the one at head) after
// receiving an ack. Applies the payload's StateChanges to the snapshot
// before returning. Returns nil if there are no sent payloads.
func (t *inflightTracker) pop() *Payload {
	if t.head == t.sentTail {
		return nil
	}
	payload := t.items[t.head]
	t.items[t.head] = nil // allow GC
	t.head = (t.head + 1) % len(t.items)

	tlmInflightSize.Sub(float64(len(payload.Encoded)), t.laneID)

	// Apply state changes from this payload to the snapshot. This is what
	// makes the snapshot "definitely-delivered state": we only accumulate
	// state from payloads intake has acked.
	if len(payload.StateChanges) > 0 {
		t.snapshot.apply(payload.StateChanges)
		t.reportDictGauges()
	}

	// Advance headBatchID for the next-acked payload.
	if t.head != t.sentTail {
		t.headBatchID++
	}
	return payload
}

// resetOnRotation moves all unacked payloads back to the buffered region
// (so they'll be retransmitted on the new stream) and resets batchIDs.
// batch_id=0 is reserved for the snapshot batch; batch_id=1 is the first
// real batch on the new stream.
func (t *inflightTracker) resetOnRotation() {
	t.sentTail = t.head
	t.headBatchID = firstRealBatchID
	t.batchIDCounter = firstRealBatchID
	log.Infof("Lane %s: inflight.resetOnRotation — batchIDCounter reset to %d (next real batch_id; batch_id=0 will be snapshot)",
		t.laneID, firstRealBatchID)
}

// getSnapshot returns the serialized full snapshot of the per-stream
// dictionary state as a MetricDatumSequence. Used as batch_id=0 on every
// new stream (including post-rotation).
//
// PoC behavior is NON-LAZY per contract.md D6: this method dumps the entire
// snapshot regardless of what the inflight queue references. Lazy snapshot
// (subset-by-reference) is deferred — see phase3-proto-proposal.md §9.
//
// Returns nil if the snapshot is empty (no dict defined yet — typical on
// initial stream creation before any data has flowed).
func (t *inflightTracker) getSnapshot() ([]byte, error) {
	return t.snapshot.serialize()
}

// reportDictGauges updates the per-kind dict-size gauges. Called after each
// snapshot mutation. Cheap — the maps are bounded by the agent's dict
// cardinality, typically hundreds of entries.
func (t *inflightTracker) reportDictGauges() {
	tlmDictSize.Set(float64(len(t.snapshot.nameMap)), t.laneID, "name_define")
	tlmDictSize.Set(float64(len(t.snapshot.tagStringMap)), t.laneID, "tag_string_define")
	tlmDictSize.Set(float64(len(t.snapshot.sourceTypeNameMap)), t.laneID, "source_type_name_define")
	tlmDictSize.Set(float64(len(t.snapshot.resourceStringMap)), t.laneID, "resource_string_define")
	tlmDictSize.Set(float64(len(t.snapshot.resourceMap)), t.laneID, "resource_define")
	tlmDictSize.Set(float64(len(t.snapshot.originMap)), t.laneID, "origin_define")
	tlmDictSize.Set(float64(len(t.snapshot.tagsetMap)), t.laneID, "tagset_define")
}

// --------------------------------------------------------------------------
// snapshotState
// --------------------------------------------------------------------------

// snapshotState mirrors the intake's per-stream dictionary state. Each of the
// seven maps holds the accumulated MetricXxxDefine datums keyed by their id.
// State is updated when payloads are popped (acked) — see inflightTracker.pop.
type snapshotState struct {
	nameMap           map[uint64]*statefulpb.MetricNameDefine
	tagStringMap      map[uint64]*statefulpb.MetricTagStringDefine
	sourceTypeNameMap map[uint64]*statefulpb.MetricSourceTypeNameDefine
	resourceStringMap map[uint64]*statefulpb.MetricResourceStringDefine
	resourceMap       map[uint64]*statefulpb.MetricResourceDefine
	originMap         map[uint64]*statefulpb.MetricOriginDefine
	tagsetMap         map[uint64]*statefulpb.MetricTagsetDefine
}

func newSnapshotState() *snapshotState {
	return &snapshotState{
		nameMap:           make(map[uint64]*statefulpb.MetricNameDefine),
		tagStringMap:      make(map[uint64]*statefulpb.MetricTagStringDefine),
		sourceTypeNameMap: make(map[uint64]*statefulpb.MetricSourceTypeNameDefine),
		resourceStringMap: make(map[uint64]*statefulpb.MetricResourceStringDefine),
		resourceMap:       make(map[uint64]*statefulpb.MetricResourceDefine),
		originMap:         make(map[uint64]*statefulpb.MetricOriginDefine),
		tagsetMap:         make(map[uint64]*statefulpb.MetricTagsetDefine),
	}
}

// apply walks the supplied state-change datums (typically from a popped
// payload's StateChanges) and updates the corresponding maps. Define-datums
// for a given (kind, id) overwrite any existing entry — agents don't re-define
// IDs with different values, but if a future Delete mechanism is added this
// would be the place to handle removal.
func (s *snapshotState) apply(stateChanges []*statefulpb.MetricDatum) {
	for _, datum := range stateChanges {
		switch d := datum.Data.(type) {
		case *statefulpb.MetricDatum_MetricNameDefine:
			if d.MetricNameDefine != nil {
				s.nameMap[d.MetricNameDefine.Id] = d.MetricNameDefine
			}
		case *statefulpb.MetricDatum_MetricTagStringDefine:
			if d.MetricTagStringDefine != nil {
				s.tagStringMap[d.MetricTagStringDefine.Id] = d.MetricTagStringDefine
			}
		case *statefulpb.MetricDatum_MetricSourceTypeNameDefine:
			if d.MetricSourceTypeNameDefine != nil {
				s.sourceTypeNameMap[d.MetricSourceTypeNameDefine.Id] = d.MetricSourceTypeNameDefine
			}
		case *statefulpb.MetricDatum_MetricResourceStringDefine:
			if d.MetricResourceStringDefine != nil {
				s.resourceStringMap[d.MetricResourceStringDefine.Id] = d.MetricResourceStringDefine
			}
		case *statefulpb.MetricDatum_MetricResourceDefine:
			if d.MetricResourceDefine != nil {
				s.resourceMap[d.MetricResourceDefine.Id] = d.MetricResourceDefine
			}
		case *statefulpb.MetricDatum_MetricOriginDefine:
			if d.MetricOriginDefine != nil {
				s.originMap[d.MetricOriginDefine.Id] = d.MetricOriginDefine
			}
		case *statefulpb.MetricDatum_MetricTagsetDefine:
			if d.MetricTagsetDefine != nil {
				s.tagsetMap[d.MetricTagsetDefine.Id] = d.MetricTagsetDefine
			}
		}
	}
}

// serialize returns the entire snapshot as a serialized MetricDatumSequence.
// Datums are emitted in **dependency order** so the receiver's state machine
// can process them sequentially without forward refs:
//
//  1. All leaf-string defines (name, tag-string, source-type-name,
//     resource-string) — no inter-dict references.
//  2. All origin defines — no inter-dict references.
//  3. All resource defines — depend on resource-strings, all of which are
//     emitted in step 1.
//  4. All tagset defines, sorted by id ascending — depend on tag-strings
//     (emitted in step 1) AND possibly on prior tagsets via prefix_id. By
//     v3 interning invariant (the prefix is always interned first via
//     internTags1(0, t1) before internTags1(prefixID, t2)), a tagset's
//     prefix_id is always strictly less than its own id, so emitting in
//     ascending-id order satisfies the dependency.
//
// Within each kind, map iteration order is non-deterministic (Go map order).
// That's acceptable because dependencies are between kinds, not within.
//
// Returns (nil, nil) if the snapshot is empty.
func (s *snapshotState) serialize() ([]byte, error) {
	total := len(s.nameMap) + len(s.tagStringMap) + len(s.sourceTypeNameMap) +
		len(s.resourceStringMap) + len(s.resourceMap) + len(s.originMap) +
		len(s.tagsetMap)
	if total == 0 {
		return nil, nil
	}

	datums := make([]*statefulpb.MetricDatum, 0, total)

	// Step 1: leaf-string defines.
	for _, def := range s.nameMap {
		datums = append(datums, &statefulpb.MetricDatum{
			Data: &statefulpb.MetricDatum_MetricNameDefine{MetricNameDefine: def},
		})
	}
	for _, def := range s.tagStringMap {
		datums = append(datums, &statefulpb.MetricDatum{
			Data: &statefulpb.MetricDatum_MetricTagStringDefine{MetricTagStringDefine: def},
		})
	}
	for _, def := range s.sourceTypeNameMap {
		datums = append(datums, &statefulpb.MetricDatum{
			Data: &statefulpb.MetricDatum_MetricSourceTypeNameDefine{MetricSourceTypeNameDefine: def},
		})
	}
	for _, def := range s.resourceStringMap {
		datums = append(datums, &statefulpb.MetricDatum{
			Data: &statefulpb.MetricDatum_MetricResourceStringDefine{MetricResourceStringDefine: def},
		})
	}

	// Step 2: origin defines (independent of strings).
	for _, def := range s.originMap {
		datums = append(datums, &statefulpb.MetricDatum{
			Data: &statefulpb.MetricDatum_MetricOriginDefine{MetricOriginDefine: def},
		})
	}

	// Step 3: resource defines (depend on resource-strings from step 1).
	for _, def := range s.resourceMap {
		datums = append(datums, &statefulpb.MetricDatum{
			Data: &statefulpb.MetricDatum_MetricResourceDefine{MetricResourceDefine: def},
		})
	}

	// Step 4: tagset defines, sorted by id ascending to satisfy prefix_id < id.
	tagsetIDs := make([]uint64, 0, len(s.tagsetMap))
	for id := range s.tagsetMap {
		tagsetIDs = append(tagsetIDs, id)
	}
	sort.Slice(tagsetIDs, func(i, j int) bool { return tagsetIDs[i] < tagsetIDs[j] })
	for _, id := range tagsetIDs {
		datums = append(datums, &statefulpb.MetricDatum{
			Data: &statefulpb.MetricDatum_MetricTagsetDefine{MetricTagsetDefine: s.tagsetMap[id]},
		})
	}

	seq := &statefulpb.MetricDatumSequence{Data: datums}
	return proto.Marshal(seq)
}
