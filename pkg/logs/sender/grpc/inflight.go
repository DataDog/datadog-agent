// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/statefulpb"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

// inflightTracker is a bounded FIFO queue that tracks payloads in two regions:
// 1. Sent but awaiting acknowledgment (head to sentTail)
// 2. Buffered but not yet sent to the network (sentTail to tail)
//
// Queue Layout:
// [--sent awaiting ack--][--buffered not sent--]
// ^                      ^                      ^
// head                 sentTail                 tail
//
// BatchID tracking:
// - Sent payloads have sequential batchIDs: [headBatchID, headBatchID+1, ..., headBatchID+sentSize-1]
// - Only tracks headBatchID (oldest sent) and nextBatchID (next to be assigned)
//
// Snapshot State:
// - Maintains accumulated state changes for stream bootstrapping
// - Represents the state "before" the first payload in the queue
// - Updated when payloads are acknowledged (popped)
type inflightTracker struct {
	workerID       string
	items          []*message.Payload
	head           int             // Index of the oldest sent item (awaiting ack)
	sentTail       int             // Index of the first buffered item that's not yet sent
	tail           int             // Index of the next available slot for new buffered items
	cap            int             // Maximum total capacity of the tracker
	headBatchID    uint32          // BatchID of the oldest sent payload (at head)
	batchIDCounter uint32          // Next batchID to be assigned when markSent is called
	snapshot       *snapshotState  // Accumulated state for new streams
	streamSent     stateReferences // State definitions already sent on the current stream
}

// newInflightTracker creates a new bounded inflight tracker with the given capacity
// Allocates capacity+1 slots to implement the "waste one slot" ring buffer pattern
func newInflightTracker(workerID string, capacity int) *inflightTracker {
	return &inflightTracker{
		workerID: workerID,
		items:    make([]*message.Payload, capacity+1),
		cap:      capacity,
		snapshot: newSnapshotState(),
		streamSent: stateReferences{
			dictEntryIDs: make(map[uint64]struct{}),
			patternIDs:   make(map[uint64]struct{}),
		},
	}
}

// hasSpace returns true if there is at least one free slot
func (t *inflightTracker) hasSpace() bool {
	return t.totalCount() < t.cap
}

// append adds a new payload to the buffered region (not yet sent)
// Returns true if the payload was added, false if the tracker is full
func (t *inflightTracker) append(payload *message.Payload) bool {
	if !t.hasSpace() {
		return false
	}
	t.items[t.tail] = payload
	tlmWorkerInflightSize.Add(float64(len(payload.Encoded)), t.workerID)
	t.tail = (t.tail + 1) % len(t.items)
	return true
}

// pop removes and returns the oldest sent payload (at head) after receiving an ack
// Returns nil if there are no sent payloads
// Also applies any state changes from the payload to the snapshot state
func (t *inflightTracker) pop() *message.Payload {
	if t.head == t.sentTail {
		return nil
	}
	payload := t.items[t.head]
	t.items[t.head] = nil // Allow GC
	t.head = (t.head + 1) % len(t.items)

	tlmWorkerInflightSize.Sub(float64(len(payload.Encoded)), t.workerID)

	// Apply state changes from this payload to snapshot
	if payload.StatefulExtra != nil {
		if extra, ok := payload.StatefulExtra.(*StatefulExtra); ok {
			t.snapshot.apply(extra)
		}
	}

	// Advance headBatchID for the next payload
	if t.head != t.sentTail {
		t.headBatchID++
	}

	return payload
}

// hasUnacked returns true if there are sent payloads awaiting acknowledgment
func (t *inflightTracker) hasUnacked() bool {
	return t.head != t.sentTail
}

// hasUnSent returns true if there are buffered payloads not yet sent
func (t *inflightTracker) hasUnSent() bool {
	return t.sentTail != t.tail
}

// getHeadBatchID returns the expected batchID at the head (oldest sent payload)
// Caller must check hasUnacked() first to ensure there are sent payloads
func (t *inflightTracker) getHeadBatchID() uint32 {
	return t.headBatchID
}

// nextBatchID returns the batchID that will be assigned to the next sent item
// This is a peek operation (idempotent, no mutation)
func (t *inflightTracker) nextBatchID() uint32 {
	return t.batchIDCounter
}

// markSent moves a buffered payload to the sent region and assigns it a batchID
// Returns true if successful, false if there are no buffered payloads
func (t *inflightTracker) markSent() bool {
	if t.sentTail == t.tail {
		return false
	}

	t.markStateSent(t.items[t.sentTail])

	// If this is the first sent item, set headBatchID
	if t.head == t.sentTail {
		t.headBatchID = t.batchIDCounter
	}

	t.sentTail = (t.sentTail + 1) % len(t.items)
	t.batchIDCounter++ // Increment counter for next batch
	return true
}

// nextToSend returns the next buffered payload ready to be sent (without removing it)
// Returns nil if there are no buffered payloads
func (t *inflightTracker) nextToSend() *message.Payload {
	if t.sentTail == t.tail {
		return nil
	}
	return t.items[t.sentTail]
}

func (t *inflightTracker) nextToSendEncoded(compressor compression.Compressor) ([]byte, error) {
	payload := t.nextToSend()
	if payload == nil {
		return nil, nil
	}

	extra, ok := payload.StatefulExtra.(*StatefulExtra)
	if !ok || extra == nil || len(extra.WireDatums) == 0 {
		return payload.Encoded, nil
	}

	prefix := t.missingSnapshotDefines(extra.WireDatums)
	if len(prefix) == 0 {
		return payload.Encoded, nil
	}

	sync := replayDeltaEncodingSync(extra.WireDatums)
	datums := make([]*statefulpb.Datum, 0, len(prefix)+len(extra.WireDatums)+1)
	datums = append(datums, prefix...)
	if sync != nil {
		datums = append(datums, sync)
	}
	datums = append(datums, extra.WireDatums...)

	serialized, err := proto.Marshal(&statefulpb.DatumSequence{Data: datums})
	if err != nil {
		return nil, err
	}
	return compressor.Compress(serialized)
}

func replayDeltaEncodingSync(datums []*statefulpb.Datum) *statefulpb.Datum {
	var sync statefulpb.DeltaEncodingSync
	var hasSync bool
	var currentPatternID uint64
	var currentTags *statefulpb.TagSet
	var currentFlatLogStatus uint64
	var currentFlatLogService uint64
	var currentFlatLogTags uint64
	var currentFlatLogJSONSchema uint64
	var syncedPattern bool
	var syncedTags bool
	var syncedFlatLogStatus bool
	var syncedFlatLogService bool
	var syncedFlatLogTags bool
	var syncedFlatLogJSONSchema bool

	for _, datum := range datums {
		if datum == nil {
			continue
		}
		switch d := datum.Data.(type) {
		case *statefulpb.Datum_PatternDefine:
			if d.PatternDefine == nil {
				continue
			}
			currentPatternID = d.PatternDefine.PatternId
		case *statefulpb.Datum_DeltaEncodingSync:
			if d.DeltaEncodingSync == nil {
				continue
			}
			if d.DeltaEncodingSync.PatternId != 0 {
				currentPatternID = d.DeltaEncodingSync.PatternId
			}
			if d.DeltaEncodingSync.Tags != nil {
				currentTags = d.DeltaEncodingSync.Tags
				currentFlatLogTags = flatLogTagSetDictIndex(d.DeltaEncodingSync.Tags)
			}
			if d.DeltaEncodingSync.Status != 0 {
				currentFlatLogStatus = d.DeltaEncodingSync.Status
			}
			if d.DeltaEncodingSync.Service != 0 {
				currentFlatLogService = d.DeltaEncodingSync.Service
			}
			if d.DeltaEncodingSync.FlatLogTags != 0 {
				currentFlatLogTags = d.DeltaEncodingSync.FlatLogTags
			}
			if d.DeltaEncodingSync.JsonSchemaId != 0 {
				currentFlatLogJSONSchema = d.DeltaEncodingSync.JsonSchemaId
			}
		case *statefulpb.Datum_Logs:
			logDatum := d.Logs
			if logDatum == nil {
				continue
			}
			if !syncedPattern {
				if structured := logDatum.GetStructured(); structured != nil {
					if structured.PatternId == 0 {
						if currentPatternID != 0 {
							sync.PatternId = currentPatternID
							hasSync = true
							syncedPattern = true
						}
					} else {
						currentPatternID = structured.PatternId
					}
				}
			}
			if !syncedTags {
				if logDatum.Tags == nil || logDatum.Tags.Tagset == nil {
					if currentTags != nil {
						sync.Tags = currentTags
						hasSync = true
						syncedTags = true
					}
				} else {
					currentTags = logDatum.Tags
				}
			}
		case *statefulpb.Datum_FlatLog:
			logDatum := d.FlatLog
			if logDatum == nil {
				continue
			}
			if !syncedPattern && logDatum.RawLog == "" {
				if logDatum.PatternId == 0 {
					if currentPatternID != 0 {
						sync.PatternId = currentPatternID
						hasSync = true
						syncedPattern = true
					}
				} else {
					currentPatternID = logDatum.PatternId
				}
			}
			if !syncedFlatLogTags {
				if logDatum.Tags == 0 {
					if currentFlatLogTags != 0 {
						sync.FlatLogTags = currentFlatLogTags
						hasSync = true
						syncedFlatLogTags = true
					}
				} else {
					currentFlatLogTags = logDatum.Tags
				}
			}
			currentFlatLogStatus = setDeltaEncodingSyncField(logDatum.Status, currentFlatLogStatus, &sync.Status, &syncedFlatLogStatus, &hasSync)
			currentFlatLogService = setDeltaEncodingSyncField(logDatum.Service, currentFlatLogService, &sync.Service, &syncedFlatLogService, &hasSync)
			currentFlatLogJSONSchema = setDeltaEncodingSyncField(logDatum.JsonSchemaId, currentFlatLogJSONSchema, &sync.JsonSchemaId, &syncedFlatLogJSONSchema, &hasSync)
		}
	}

	if !hasSync {
		return nil
	}
	return &statefulpb.Datum{
		Data: &statefulpb.Datum_DeltaEncodingSync{
			DeltaEncodingSync: &sync,
		},
	}
}

func setDeltaEncodingSyncField(value uint64, current uint64, syncField *uint64, synced *bool, hasSync *bool) uint64 {
	if *synced {
		return current
	}
	if value == 0 {
		if current != 0 {
			*syncField = current
			*hasSync = true
			*synced = true
		}
		return current
	}
	return value
}

// sentCount returns the number of sent payloads awaiting ack
func (t *inflightTracker) sentCount() int {
	return (t.sentTail - t.head + len(t.items)) % len(t.items)
}

// totalCount returns the total number of tracked payloads
func (t *inflightTracker) totalCount() int {
	return (t.tail - t.head + len(t.items)) % len(t.items)
}

// resetOnRotation set any un-acked payload as un-sent and reset the batchID.
func (t *inflightTracker) resetOnRotation() {
	// Move all sent items back to buffered region by resetting sentTail to head
	// This makes all items [head, tail) buffered again
	t.sentTail = t.head

	// Reset batchID counter for the new stream
	// Make the first batchID be 1, 0 is reserved for the snapshot state
	t.headBatchID = 1
	t.batchIDCounter = 1
	t.streamSent = newStateReferences()
}

// getSnapshot returns the current snapshot state for stream bootstrapping
// Returns serialized bytes (marshaled DatumSequence) or nil if empty
func (t *inflightTracker) getSnapshot() []byte {
	refs, ok := t.inflightReferences()
	if !ok {
		serialized, sent := t.snapshot.serialize(nil)
		t.streamSent = sent
		return serialized
	}

	serialized, sent := t.snapshot.serialize(&refs)
	t.streamSent = sent
	return serialized
}

func (t *inflightTracker) resetStreamSent() {
	t.streamSent = newStateReferences()
}

// snapshotState maintains the accumulated state changes for stream bootstrapping
// It represents the state "before" the first payload in the inflight queue
type snapshotState struct {
	dictMap       map[uint64]*statefulpb.DictEntryDefine
	patternMap    map[uint64]*statefulpb.PatternDefine
	jsonSchemaMap map[uint64]*statefulpb.JsonSchemaDefine
}

type stateReferences struct {
	dictEntryIDs  map[uint64]struct{}
	patternIDs    map[uint64]struct{}
	jsonSchemaIDs map[uint64]struct{}
}

func newStateReferences() stateReferences {
	return stateReferences{
		dictEntryIDs:  make(map[uint64]struct{}),
		patternIDs:    make(map[uint64]struct{}),
		jsonSchemaIDs: make(map[uint64]struct{}),
	}
}

func (r stateReferences) clone() stateReferences {
	clone := newStateReferences()
	for id := range r.dictEntryIDs {
		clone.dictEntryIDs[id] = struct{}{}
	}
	for id := range r.patternIDs {
		clone.patternIDs[id] = struct{}{}
	}
	for id := range r.jsonSchemaIDs {
		clone.jsonSchemaIDs[id] = struct{}{}
	}
	return clone
}

func (r stateReferences) hasDictEntry(id uint64) bool {
	_, ok := r.dictEntryIDs[id]
	return ok
}

func (r stateReferences) hasPattern(id uint64) bool {
	_, ok := r.patternIDs[id]
	return ok
}

func (r stateReferences) hasJsonSchema(id uint64) bool {
	_, ok := r.jsonSchemaIDs[id]
	return ok
}

func (r stateReferences) addDictEntry(id uint64) {
	if id == 0 {
		return
	}
	r.dictEntryIDs[id] = struct{}{}
}

func (r stateReferences) addPattern(id uint64) {
	if id == 0 {
		return
	}
	r.patternIDs[id] = struct{}{}
}

func (r stateReferences) addJsonSchema(id uint64) {
	if id == 0 || id == flatLogEmptyDictIndex {
		return
	}
	r.jsonSchemaIDs[id] = struct{}{}
}

func (r stateReferences) deleteDictEntry(id uint64) {
	delete(r.dictEntryIDs, id)
}

func (r stateReferences) deletePattern(id uint64) {
	delete(r.patternIDs, id)
}

func (r stateReferences) deleteJsonSchema(id uint64) {
	delete(r.jsonSchemaIDs, id)
}

// newSnapshotState creates a new empty snapshot state
func newSnapshotState() *snapshotState {
	return &snapshotState{
		dictMap:       make(map[uint64]*statefulpb.DictEntryDefine),
		patternMap:    make(map[uint64]*statefulpb.PatternDefine),
		jsonSchemaMap: make(map[uint64]*statefulpb.JsonSchemaDefine),
	}
}

// apply updates the snapshot state by processing state changes from a payload
func (s *snapshotState) apply(extra *StatefulExtra) {
	if extra == nil {
		return
	}

	for _, datum := range extra.StateChanges {
		switch d := datum.Data.(type) {
		case *statefulpb.Datum_PatternDefine:
			s.patternMap[d.PatternDefine.PatternId] = d.PatternDefine
		case *statefulpb.Datum_PatternDelete:
			delete(s.patternMap, d.PatternDelete.PatternId)
		case *statefulpb.Datum_DictEntryDefine:
			s.dictMap[d.DictEntryDefine.Id] = d.DictEntryDefine
		case *statefulpb.Datum_DictEntryDelete:
			delete(s.dictMap, d.DictEntryDelete.Id)
		case *statefulpb.Datum_JsonSchemaDefine:
			s.jsonSchemaMap[d.JsonSchemaDefine.SchemaId] = d.JsonSchemaDefine
		case *statefulpb.Datum_JsonSchemaDelete:
			delete(s.jsonSchemaMap, d.JsonSchemaDelete.SchemaId)
		}
	}
}

// serialize returns the current snapshot state as serialized bytes.
// If refs is non-nil, only definitions referenced by queued inflight data are included.
// Used to send snapshot on new stream creation
func (s *snapshotState) serialize(refs *stateReferences) ([]byte, stateReferences) {
	sent := newStateReferences()
	datums := make([]*statefulpb.Datum, 0, len(s.patternMap)+len(s.dictMap)+len(s.jsonSchemaMap))

	for id, pattern := range s.patternMap {
		if refs != nil && !refs.hasPattern(id) {
			continue
		}
		datums = append(datums, &statefulpb.Datum{
			Data: &statefulpb.Datum_PatternDefine{PatternDefine: pattern},
		})
		sent.addPattern(id)
	}
	for id, entry := range s.dictMap {
		if refs != nil && !refs.hasDictEntry(id) {
			continue
		}
		datums = append(datums, &statefulpb.Datum{
			Data: &statefulpb.Datum_DictEntryDefine{DictEntryDefine: entry},
		})
		sent.addDictEntry(id)
	}
	for id, schema := range s.jsonSchemaMap {
		if refs != nil && !refs.hasJsonSchema(id) {
			continue
		}
		datums = append(datums, &statefulpb.Datum{
			Data: &statefulpb.Datum_JsonSchemaDefine{JsonSchemaDefine: schema},
		})
		sent.addJsonSchema(id)
	}

	if len(datums) == 0 {
		return nil, sent
	}

	datumSeq := &statefulpb.DatumSequence{
		Data: datums,
	}

	serialized, _ := proto.Marshal(datumSeq)
	return serialized, sent
}

func (t *inflightTracker) inflightReferences() (stateReferences, bool) {
	refs := newStateReferences()
	for count, index := 0, t.head; count < t.totalCount(); count++ {
		payload := t.items[index]
		extra, ok := payload.StatefulExtra.(*StatefulExtra)
		if !ok || extra == nil || extra.WireDatums == nil {
			return stateReferences{}, false
		}
		addDatumReferences(refs, extra.WireDatums)
		index = (index + 1) % len(t.items)
	}
	return refs, true
}

func (t *inflightTracker) missingSnapshotDefines(datums []*statefulpb.Datum) []*statefulpb.Datum {
	known := t.streamSent.clone()
	missing := newStateReferences()

	for _, datum := range datums {
		switch d := datum.Data.(type) {
		case *statefulpb.Datum_PatternDefine:
			known.addPattern(d.PatternDefine.PatternId)
		case *statefulpb.Datum_PatternDelete:
			known.deletePattern(d.PatternDelete.PatternId)
		case *statefulpb.Datum_DictEntryDefine:
			known.addDictEntry(d.DictEntryDefine.Id)
		case *statefulpb.Datum_DictEntryDelete:
			known.deleteDictEntry(d.DictEntryDelete.Id)
		case *statefulpb.Datum_JsonSchemaDefine:
			known.addJsonSchema(d.JsonSchemaDefine.SchemaId)
			addJsonSchemaReferences(known, d.JsonSchemaDefine)
		case *statefulpb.Datum_JsonSchemaDelete:
			known.deleteJsonSchema(d.JsonSchemaDelete.SchemaId)
		case *statefulpb.Datum_Logs:
			t.addMissingLogReferences(missing, known, d.Logs)
		case *statefulpb.Datum_FlatLog:
			t.addMissingFlatLogReferences(missing, known, d.FlatLog)
		case *statefulpb.Datum_DeltaEncodingSync:
			t.addMissingDeltaEncodingSyncReferences(missing, known, d.DeltaEncodingSync)
		}
	}

	prefix := make([]*statefulpb.Datum, 0, len(missing.patternIDs)+len(missing.dictEntryIDs)+len(missing.jsonSchemaIDs))
	for id := range missing.patternIDs {
		pattern := t.snapshot.patternMap[id]
		if pattern == nil {
			continue
		}
		prefix = append(prefix, &statefulpb.Datum{
			Data: &statefulpb.Datum_PatternDefine{PatternDefine: pattern},
		})
	}
	for id := range missing.dictEntryIDs {
		entry := t.snapshot.dictMap[id]
		if entry == nil {
			continue
		}
		prefix = append(prefix, &statefulpb.Datum{
			Data: &statefulpb.Datum_DictEntryDefine{DictEntryDefine: entry},
		})
	}
	for id := range missing.jsonSchemaIDs {
		schema := t.snapshot.jsonSchemaMap[id]
		if schema == nil {
			continue
		}
		prefix = append(prefix, &statefulpb.Datum{
			Data: &statefulpb.Datum_JsonSchemaDefine{JsonSchemaDefine: schema},
		})
	}
	return prefix
}

func (t *inflightTracker) addMissingLogReferences(missing stateReferences, known stateReferences, log *statefulpb.Log) {
	if log == nil {
		return
	}
	t.addMissingDynamicValueReference(missing, known, log.Status)
	t.addMissingDynamicValueReference(missing, known, log.Service)
	if log.Tags != nil {
		t.addMissingDynamicValueReference(missing, known, log.Tags.Tagset)
	}
	if structured := log.GetStructured(); structured != nil {
		t.addMissingPatternReference(missing, known, structured.PatternId)
		t.addMissingDynamicValueReferences(missing, known, structured.DynamicValues)
		t.addMissingDynamicValueReference(missing, known, structured.JsonMessageKey)
		t.addMissingDictEntryReference(missing, known, structured.JsonContextSchemaId)
		t.addMissingDynamicValueReferences(missing, known, structured.JsonContextValues)
	}
}

func (t *inflightTracker) addMissingFlatLogReferences(missing stateReferences, known stateReferences, log *statefulpb.FlatLog) {
	if log == nil {
		return
	}
	t.addMissingFlatLogDictEntryReference(missing, known, log.Status)
	t.addMissingFlatLogDictEntryReference(missing, known, log.Service)
	t.addMissingFlatLogDictEntryReference(missing, known, log.Tags)
	if log.RawLog == "" {
		t.addMissingPatternReference(missing, known, log.PatternId)
		t.addMissingDynamicValueReferences(missing, known, log.DynamicValues)
	}
	t.addMissingJsonSchemaReference(missing, known, log.JsonSchemaId)
	t.addMissingDynamicValueReferences(missing, known, log.JsonContextValues)
	for _, dictID := range log.JsonContextDictValues {
		t.addMissingDictEntryReference(missing, known, dictID)
	}
}

func (t *inflightTracker) addMissingDeltaEncodingSyncReferences(missing stateReferences, known stateReferences, sync *statefulpb.DeltaEncodingSync) {
	if sync == nil {
		return
	}
	t.addMissingPatternReference(missing, known, sync.PatternId)
	if sync.Tags != nil {
		t.addMissingDynamicValueReference(missing, known, sync.Tags.Tagset)
	}
	t.addMissingFlatLogDictEntryReference(missing, known, sync.Status)
	t.addMissingFlatLogDictEntryReference(missing, known, sync.Service)
	t.addMissingFlatLogDictEntryReference(missing, known, sync.FlatLogTags)
	t.addMissingJsonSchemaReference(missing, known, sync.JsonSchemaId)
}

func (t *inflightTracker) addMissingDynamicValueReferences(missing stateReferences, known stateReferences, values []*statefulpb.DynamicValue) {
	for _, value := range values {
		t.addMissingDynamicValueReference(missing, known, value)
	}
}

func (t *inflightTracker) addMissingDynamicValueReference(missing stateReferences, known stateReferences, value *statefulpb.DynamicValue) {
	if value == nil {
		return
	}
	t.addMissingDictEntryReference(missing, known, value.GetDictIndex())
}

func (t *inflightTracker) addMissingFlatLogDictEntryReference(missing stateReferences, known stateReferences, id uint64) {
	if isFlatLogEmptyDictIndex(id) {
		return
	}
	t.addMissingDictEntryReference(missing, known, id)
}

func (t *inflightTracker) addMissingDictEntryReference(missing stateReferences, known stateReferences, id uint64) {
	if id == 0 || known.hasDictEntry(id) || t.snapshot.dictMap[id] == nil {
		return
	}
	missing.addDictEntry(id)
	known.addDictEntry(id)
}

func (t *inflightTracker) addMissingJsonSchemaReference(missing stateReferences, known stateReferences, id uint64) {
	if id == 0 || id == flatLogEmptyDictIndex || known.hasJsonSchema(id) {
		return
	}
	schema := t.snapshot.jsonSchemaMap[id]
	if schema == nil {
		return
	}
	missing.addJsonSchema(id)
	known.addJsonSchema(id)
	t.addMissingDictEntryReference(missing, known, schema.MessageKeyId)
	for _, keyID := range schema.Keys {
		t.addMissingDictEntryReference(missing, known, keyID)
	}
}

func (t *inflightTracker) addMissingPatternReference(missing stateReferences, known stateReferences, id uint64) {
	if id == 0 || known.hasPattern(id) || t.snapshot.patternMap[id] == nil {
		return
	}
	missing.addPattern(id)
	known.addPattern(id)
}

func (t *inflightTracker) markStateSent(payload *message.Payload) {
	extra, ok := payload.StatefulExtra.(*StatefulExtra)
	if !ok || extra == nil || len(extra.WireDatums) == 0 {
		return
	}
	for _, datum := range t.missingSnapshotDefines(extra.WireDatums) {
		switch d := datum.Data.(type) {
		case *statefulpb.Datum_PatternDefine:
			t.streamSent.addPattern(d.PatternDefine.PatternId)
		case *statefulpb.Datum_DictEntryDefine:
			t.streamSent.addDictEntry(d.DictEntryDefine.Id)
		case *statefulpb.Datum_JsonSchemaDefine:
			t.streamSent.addJsonSchema(d.JsonSchemaDefine.SchemaId)
			addJsonSchemaReferences(t.streamSent, d.JsonSchemaDefine)
		}
	}
	t.streamSent.applyStateChanges(extra.WireDatums)
}

func (r stateReferences) applyStateChanges(datums []*statefulpb.Datum) {
	for _, datum := range datums {
		switch d := datum.Data.(type) {
		case *statefulpb.Datum_PatternDefine:
			r.addPattern(d.PatternDefine.PatternId)
		case *statefulpb.Datum_PatternDelete:
			r.deletePattern(d.PatternDelete.PatternId)
		case *statefulpb.Datum_DictEntryDefine:
			r.addDictEntry(d.DictEntryDefine.Id)
		case *statefulpb.Datum_DictEntryDelete:
			r.deleteDictEntry(d.DictEntryDelete.Id)
		case *statefulpb.Datum_JsonSchemaDefine:
			r.addJsonSchema(d.JsonSchemaDefine.SchemaId)
			addJsonSchemaReferences(r, d.JsonSchemaDefine)
		case *statefulpb.Datum_JsonSchemaDelete:
			r.deleteJsonSchema(d.JsonSchemaDelete.SchemaId)
		}
	}
}

func addDatumReferences(refs stateReferences, datums []*statefulpb.Datum) {
	for _, datum := range datums {
		switch d := datum.Data.(type) {
		case *statefulpb.Datum_Logs:
			addLogReferences(refs, d.Logs)
		case *statefulpb.Datum_FlatLog:
			addFlatLogReferences(refs, d.FlatLog)
		case *statefulpb.Datum_DeltaEncodingSync:
			addDeltaEncodingSyncReferences(refs, d.DeltaEncodingSync)
		case *statefulpb.Datum_JsonSchemaDefine:
			refs.addJsonSchema(d.JsonSchemaDefine.SchemaId)
			addJsonSchemaReferences(refs, d.JsonSchemaDefine)
		}
	}
}

func addLogReferences(refs stateReferences, log *statefulpb.Log) {
	if log == nil {
		return
	}
	addDynamicValueReference(refs, log.Status)
	addDynamicValueReference(refs, log.Service)
	if log.Tags != nil {
		addDynamicValueReference(refs, log.Tags.Tagset)
	}
	if structured := log.GetStructured(); structured != nil {
		refs.addPattern(structured.PatternId)
		addDynamicValueReferences(refs, structured.DynamicValues)
		addDynamicValueReference(refs, structured.JsonMessageKey)
		refs.addDictEntry(structured.JsonContextSchemaId)
		addDynamicValueReferences(refs, structured.JsonContextValues)
	}
}

func addFlatLogReferences(refs stateReferences, log *statefulpb.FlatLog) {
	if log == nil {
		return
	}
	addFlatLogDictEntryReference(refs, log.Status)
	addFlatLogDictEntryReference(refs, log.Service)
	addFlatLogDictEntryReference(refs, log.Tags)
	if log.RawLog == "" {
		refs.addPattern(log.PatternId)
		addDynamicValueReferences(refs, log.DynamicValues)
	}
	addFlatLogJsonSchemaReference(refs, log.JsonSchemaId)
	addDynamicValueReferences(refs, log.JsonContextValues)
	for _, dictID := range log.JsonContextDictValues {
		refs.addDictEntry(dictID)
	}
}

func addDeltaEncodingSyncReferences(refs stateReferences, sync *statefulpb.DeltaEncodingSync) {
	if sync == nil {
		return
	}
	refs.addPattern(sync.PatternId)
	if sync.Tags != nil {
		addDynamicValueReference(refs, sync.Tags.Tagset)
	}
	addFlatLogDictEntryReference(refs, sync.Status)
	addFlatLogDictEntryReference(refs, sync.Service)
	addFlatLogDictEntryReference(refs, sync.FlatLogTags)
	addFlatLogJsonSchemaReference(refs, sync.JsonSchemaId)
}

func addJsonSchemaReferences(refs stateReferences, schema *statefulpb.JsonSchemaDefine) {
	if schema == nil {
		return
	}
	addFlatLogDictEntryReference(refs, schema.MessageKeyId)
	for _, keyID := range schema.Keys {
		addFlatLogDictEntryReference(refs, keyID)
	}
}

func addDynamicValueReferences(refs stateReferences, values []*statefulpb.DynamicValue) {
	for _, value := range values {
		addDynamicValueReference(refs, value)
	}
}

func addDynamicValueReference(refs stateReferences, value *statefulpb.DynamicValue) {
	if value == nil {
		return
	}
	refs.addDictEntry(value.GetDictIndex())
}

func addFlatLogDictEntryReference(refs stateReferences, id uint64) {
	if isFlatLogEmptyDictIndex(id) {
		return
	}
	refs.addDictEntry(id)
}

func addFlatLogJsonSchemaReference(refs stateReferences, id uint64) {
	if id == 0 || id == flatLogEmptyDictIndex {
		return
	}
	refs.addJsonSchema(id)
}
