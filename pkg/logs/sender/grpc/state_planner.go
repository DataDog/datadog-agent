// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/statefulpb"
)

// statePlanner owns the state-encoding rules between canonical datums and a stream.
type statePlanner struct {
	snapshot    *snapshotState
	streamKnown stateReferences
}

func newStatePlanner() *statePlanner {
	return &statePlanner{
		snapshot:    newSnapshotState(),
		streamKnown: newStateReferences(),
	}
}

func (p *statePlanner) resetStream() {
	p.streamKnown = newStateReferences()
}

func (p *statePlanner) applyAcked(extra *StatefulExtra, protectedRefs *stateReferences) {
	if protectedRefs == nil {
		p.snapshot.apply(extra)
		return
	}
	expanded := p.withJsonSchemaDependencies(*protectedRefs)
	p.snapshot.applyWithProtectedRefs(extra, &expanded)
}

func (p *statePlanner) hasDeferredDeletes() bool {
	return p.snapshot.hasDeferredDeletes()
}

func (p *statePlanner) snapshotBytes(refs *stateReferences) ([]byte, stateReferences) {
	var snapshotRefs *stateReferences
	if refs != nil {
		expanded := p.withJsonSchemaDependencies(*refs)
		snapshotRefs = &expanded
	}
	serialized, sent := p.snapshot.serialize(snapshotRefs)
	p.streamKnown = sent
	return serialized, sent
}

func (p *statePlanner) planWireDatums(datums []*statefulpb.LogDatum) ([]*statefulpb.LogDatum, bool) {
	prefix := p.missingSnapshotDefines(datums)
	if len(prefix) == 0 {
		return datums, false
	}

	planned := make([]*statefulpb.LogDatum, 0, len(prefix)+len(datums))
	planned = append(planned, prefix...)
	planned = append(planned, datums...)
	return planned, true
}

func (p *statePlanner) markSent(datums []*statefulpb.LogDatum) {
	for _, datum := range p.missingSnapshotDefines(datums) {
		switch d := datum.Data.(type) {
		case *statefulpb.LogDatum_PatternDefine:
			p.streamKnown.addPattern(d.PatternDefine.PatternId)
		case *statefulpb.LogDatum_DictEntryDefine:
			p.streamKnown.addDictEntry(d.DictEntryDefine.Id)
		case *statefulpb.LogDatum_JsonSchemaDefine:
			p.streamKnown.addJsonSchema(d.JsonSchemaDefine.SchemaId)
		}
	}
	p.streamKnown.applyStateChanges(datums)
}

func (p *statePlanner) withJsonSchemaDependencies(refs stateReferences) stateReferences {
	expanded := refs.clone()
	p.addJsonSchemaDictEntryReferences(expanded)
	return expanded
}

func (p *statePlanner) addJsonSchemaDictEntryReferences(refs stateReferences) {
	for id := range refs.jsonSchemaIDs {
		addJsonSchemaReferences(refs, p.snapshot.jsonSchemaMap[id])
	}
}

func (p *statePlanner) missingSnapshotDefines(datums []*statefulpb.LogDatum) []*statefulpb.LogDatum {
	known := p.streamKnown.clone()
	missing := newStateReferences()

	for _, datum := range datums {
		switch d := datum.Data.(type) {
		case *statefulpb.LogDatum_PatternDefine:
			known.addPattern(d.PatternDefine.PatternId)
		case *statefulpb.LogDatum_PatternDelete:
			known.deletePattern(d.PatternDelete.PatternId)
		case *statefulpb.LogDatum_DictEntryDefine:
			known.addDictEntry(d.DictEntryDefine.Id)
		case *statefulpb.LogDatum_DictEntryDelete:
			known.deleteDictEntry(d.DictEntryDelete.Id)
		case *statefulpb.LogDatum_JsonSchemaDefine:
			p.addMissingJsonSchemaDefineReferences(missing, known, d.JsonSchemaDefine)
			known.addJsonSchema(d.JsonSchemaDefine.SchemaId)
		case *statefulpb.LogDatum_JsonSchemaDelete:
			known.deleteJsonSchema(d.JsonSchemaDelete.SchemaId)
		case *statefulpb.LogDatum_Log:
			p.addMissingLogReferences(missing, known, d.Log)
		case *statefulpb.LogDatum_DeltaEncodingSync:
			p.addMissingDeltaEncodingSyncReferences(missing, known, d.DeltaEncodingSync)
		}
	}

	prefix := make([]*statefulpb.LogDatum, 0, len(missing.patternIDs)+len(missing.dictEntryIDs)+len(missing.jsonSchemaIDs))
	for id := range missing.patternIDs {
		pattern := p.snapshot.patternMap[id]
		if pattern == nil {
			continue
		}
		prefix = append(prefix, &statefulpb.LogDatum{
			Data: &statefulpb.LogDatum_PatternDefine{PatternDefine: pattern},
		})
	}
	for id := range missing.dictEntryIDs {
		entry := p.snapshot.dictMap[id]
		if entry == nil {
			continue
		}
		prefix = append(prefix, &statefulpb.LogDatum{
			Data: &statefulpb.LogDatum_DictEntryDefine{DictEntryDefine: entry},
		})
	}
	for id := range missing.jsonSchemaIDs {
		schema := p.snapshot.jsonSchemaMap[id]
		if schema == nil {
			continue
		}
		prefix = append(prefix, &statefulpb.LogDatum{
			Data: &statefulpb.LogDatum_JsonSchemaDefine{JsonSchemaDefine: schema},
		})
	}
	return prefix
}

func (p *statePlanner) addMissingLogReferences(missing stateReferences, known stateReferences, log *statefulpb.Log) {
	if log == nil {
		return
	}
	p.addMissingDictEntryIDReference(missing, known, log.Status)
	p.addMissingDictEntryIDReference(missing, known, log.Service)
	p.addMissingDictEntryIDReference(missing, known, log.Tags)
	if log.RawLog == "" {
		p.addMissingPatternReference(missing, known, log.PatternId)
		p.addMissingDynamicValueReferences(missing, known, log.DynamicValues)
	}
	p.addMissingJsonSchemaReference(missing, known, log.JsonSchemaId)
	for _, dictID := range log.JsonContextDictValues {
		p.addMissingDictEntryReference(missing, known, dictID)
	}
}

func (p *statePlanner) addMissingDeltaEncodingSyncReferences(missing stateReferences, known stateReferences, sync *statefulpb.DeltaEncodingSync) {
	if sync == nil {
		return
	}
	p.addMissingPatternReference(missing, known, sync.PatternId)
	p.addMissingDictEntryIDReference(missing, known, sync.Tags)
	p.addMissingDictEntryIDReference(missing, known, sync.Status)
	p.addMissingDictEntryIDReference(missing, known, sync.Service)
	p.addMissingJsonSchemaReference(missing, known, sync.JsonSchemaId)
}

func (p *statePlanner) addMissingDynamicValueReferences(missing stateReferences, known stateReferences, values []*statefulpb.DynamicValue) {
	for _, value := range values {
		p.addMissingDynamicValueReference(missing, known, value)
	}
}

func (p *statePlanner) addMissingDynamicValueReference(missing stateReferences, known stateReferences, value *statefulpb.DynamicValue) {
	if value == nil {
		return
	}
	p.addMissingDictEntryReference(missing, known, value.GetDictIndex())
}

func (p *statePlanner) addMissingDictEntryIDReference(missing stateReferences, known stateReferences, id uint64) {
	if isEmptyDictIndex(id) {
		return
	}
	p.addMissingDictEntryReference(missing, known, id)
}

func (p *statePlanner) addMissingDictEntryReference(missing stateReferences, known stateReferences, id uint64) {
	if id == 0 || known.hasDictEntry(id) || p.snapshot.dictMap[id] == nil {
		return
	}
	missing.addDictEntry(id)
	known.addDictEntry(id)
}

func (p *statePlanner) addMissingJsonSchemaReference(missing stateReferences, known stateReferences, id uint64) {
	if id == 0 || id == emptyDictIndex {
		return
	}
	schema := p.snapshot.jsonSchemaMap[id]
	if schema == nil {
		return
	}
	if !known.hasJsonSchema(id) {
		missing.addJsonSchema(id)
		known.addJsonSchema(id)
	}
	p.addMissingJsonSchemaDefineReferences(missing, known, schema)
}

func (p *statePlanner) addMissingJsonSchemaDefineReferences(missing stateReferences, known stateReferences, schema *statefulpb.JsonSchemaDefine) {
	if schema == nil {
		return
	}
	p.addMissingDictEntryReference(missing, known, schema.MessageKeyId)
	for _, keyID := range schema.Keys {
		p.addMissingDictEntryReference(missing, known, keyID)
	}
}

func (p *statePlanner) addMissingPatternReference(missing stateReferences, known stateReferences, id uint64) {
	if id == 0 || known.hasPattern(id) || p.snapshot.patternMap[id] == nil {
		return
	}
	missing.addPattern(id)
	known.addPattern(id)
}

// snapshotState maintains the accumulated state changes for stream bootstrapping.
// It represents the state before the first payload in the inflight queue.
type snapshotState struct {
	dictMap         map[uint64]*statefulpb.DictEntryDefine
	patternMap      map[uint64]*statefulpb.PatternDefine
	jsonSchemaMap   map[uint64]*statefulpb.JsonSchemaDefine
	deferredDeletes stateReferences
}

func newSnapshotState() *snapshotState {
	return &snapshotState{
		dictMap:         make(map[uint64]*statefulpb.DictEntryDefine),
		patternMap:      make(map[uint64]*statefulpb.PatternDefine),
		jsonSchemaMap:   make(map[uint64]*statefulpb.JsonSchemaDefine),
		deferredDeletes: newStateReferences(),
	}
}

func (s *snapshotState) apply(extra *StatefulExtra) {
	s.applyWithProtectedRefs(extra, nil)
}

func (s *snapshotState) applyWithProtectedRefs(extra *StatefulExtra, protectedRefs *stateReferences) {
	if extra == nil {
		return
	}
	s.pruneDeferredDeletes(protectedRefs)

	for _, datum := range extra.StateChanges {
		switch d := datum.Data.(type) {
		case *statefulpb.LogDatum_PatternDefine:
			s.patternMap[d.PatternDefine.PatternId] = d.PatternDefine
			s.deferredDeletes.deletePattern(d.PatternDefine.PatternId)
		case *statefulpb.LogDatum_PatternDelete:
			if protectedRefs != nil && protectedRefs.hasPattern(d.PatternDelete.PatternId) {
				s.deferredDeletes.addPattern(d.PatternDelete.PatternId)
			} else {
				delete(s.patternMap, d.PatternDelete.PatternId)
				s.deferredDeletes.deletePattern(d.PatternDelete.PatternId)
			}
		case *statefulpb.LogDatum_DictEntryDefine:
			s.dictMap[d.DictEntryDefine.Id] = d.DictEntryDefine
			s.deferredDeletes.deleteDictEntry(d.DictEntryDefine.Id)
		case *statefulpb.LogDatum_DictEntryDelete:
			if protectedRefs != nil && protectedRefs.hasDictEntry(d.DictEntryDelete.Id) {
				s.deferredDeletes.addDictEntry(d.DictEntryDelete.Id)
			} else {
				delete(s.dictMap, d.DictEntryDelete.Id)
				s.deferredDeletes.deleteDictEntry(d.DictEntryDelete.Id)
			}
		case *statefulpb.LogDatum_JsonSchemaDefine:
			s.jsonSchemaMap[d.JsonSchemaDefine.SchemaId] = d.JsonSchemaDefine
			s.deferredDeletes.deleteJsonSchema(d.JsonSchemaDefine.SchemaId)
		case *statefulpb.LogDatum_JsonSchemaDelete:
			if protectedRefs != nil && protectedRefs.hasJsonSchema(d.JsonSchemaDelete.SchemaId) {
				s.deferredDeletes.addJsonSchema(d.JsonSchemaDelete.SchemaId)
			} else {
				delete(s.jsonSchemaMap, d.JsonSchemaDelete.SchemaId)
				s.deferredDeletes.deleteJsonSchema(d.JsonSchemaDelete.SchemaId)
			}
		}
	}
	s.pruneDeferredDeletes(protectedRefs)
}

func (s *snapshotState) pruneDeferredDeletes(protectedRefs *stateReferences) {
	if protectedRefs == nil {
		protectedRefs = &stateReferences{}
	}
	for id := range s.deferredDeletes.patternIDs {
		if !protectedRefs.hasPattern(id) {
			delete(s.patternMap, id)
			s.deferredDeletes.deletePattern(id)
		}
	}
	for id := range s.deferredDeletes.dictEntryIDs {
		if !protectedRefs.hasDictEntry(id) {
			delete(s.dictMap, id)
			s.deferredDeletes.deleteDictEntry(id)
		}
	}
	for id := range s.deferredDeletes.jsonSchemaIDs {
		if !protectedRefs.hasJsonSchema(id) {
			delete(s.jsonSchemaMap, id)
			s.deferredDeletes.deleteJsonSchema(id)
		}
	}
}

func (s *snapshotState) hasDeferredDeletes() bool {
	return len(s.deferredDeletes.patternIDs) > 0 ||
		len(s.deferredDeletes.dictEntryIDs) > 0 ||
		len(s.deferredDeletes.jsonSchemaIDs) > 0
}

func (s *snapshotState) serialize(refs *stateReferences) ([]byte, stateReferences) {
	sent := newStateReferences()
	datums := make([]*statefulpb.LogDatum, 0, len(s.patternMap)+len(s.dictMap)+len(s.jsonSchemaMap))

	for id, pattern := range s.patternMap {
		if refs != nil && !refs.hasPattern(id) {
			continue
		}
		datums = append(datums, &statefulpb.LogDatum{
			Data: &statefulpb.LogDatum_PatternDefine{PatternDefine: pattern},
		})
		sent.addPattern(id)
	}
	for id, entry := range s.dictMap {
		if refs != nil && !refs.hasDictEntry(id) {
			continue
		}
		datums = append(datums, &statefulpb.LogDatum{
			Data: &statefulpb.LogDatum_DictEntryDefine{DictEntryDefine: entry},
		})
		sent.addDictEntry(id)
	}
	for id, schema := range s.jsonSchemaMap {
		if refs != nil && !refs.hasJsonSchema(id) {
			continue
		}
		datums = append(datums, &statefulpb.LogDatum{
			Data: &statefulpb.LogDatum_JsonSchemaDefine{JsonSchemaDefine: schema},
		})
		sent.addJsonSchema(id)
	}

	if len(datums) == 0 {
		return nil, sent
	}

	serialized, _ := proto.Marshal(&statefulpb.LogDatumSequence{Data: datums})
	return serialized, sent
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
	if id == 0 || id == emptyDictIndex {
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

func (r stateReferences) applyStateChanges(datums []*statefulpb.LogDatum) {
	for _, datum := range datums {
		switch d := datum.Data.(type) {
		case *statefulpb.LogDatum_PatternDefine:
			r.addPattern(d.PatternDefine.PatternId)
		case *statefulpb.LogDatum_PatternDelete:
			r.deletePattern(d.PatternDelete.PatternId)
		case *statefulpb.LogDatum_DictEntryDefine:
			r.addDictEntry(d.DictEntryDefine.Id)
		case *statefulpb.LogDatum_DictEntryDelete:
			r.deleteDictEntry(d.DictEntryDelete.Id)
		case *statefulpb.LogDatum_JsonSchemaDefine:
			r.addJsonSchema(d.JsonSchemaDefine.SchemaId)
		case *statefulpb.LogDatum_JsonSchemaDelete:
			r.deleteJsonSchema(d.JsonSchemaDelete.SchemaId)
		}
	}
}

func splitStateAndWireDatums(datums []*statefulpb.LogDatum) (stateChanges []*statefulpb.LogDatum, wireDatums []*statefulpb.LogDatum) {
	wireDatums = make([]*statefulpb.LogDatum, 0, len(datums))
	for _, datum := range datums {
		if isStateDatum(datum) {
			stateChanges = append(stateChanges, datum)
		}
		if isWireStateDatum(datum) {
			wireDatums = append(wireDatums, datum)
		}
	}
	return stateChanges, wireDatums
}

func stateChangesContainDeletes(datums []*statefulpb.LogDatum) bool {
	for _, datum := range datums {
		switch datum.Data.(type) {
		case *statefulpb.LogDatum_PatternDelete,
			*statefulpb.LogDatum_DictEntryDelete,
			*statefulpb.LogDatum_JsonSchemaDelete:
			return true
		}
	}
	return false
}

func isStateDatum(datum *statefulpb.LogDatum) bool {
	switch datum.Data.(type) {
	case *statefulpb.LogDatum_PatternDefine, *statefulpb.LogDatum_PatternDelete,
		*statefulpb.LogDatum_DictEntryDefine, *statefulpb.LogDatum_DictEntryDelete,
		*statefulpb.LogDatum_JsonSchemaDefine, *statefulpb.LogDatum_JsonSchemaDelete:
		return true
	default:
		return false
	}
}

func isWireStateDatum(datum *statefulpb.LogDatum) bool {
	switch datum.Data.(type) {
	case *statefulpb.LogDatum_PatternDelete, *statefulpb.LogDatum_DictEntryDelete, *statefulpb.LogDatum_JsonSchemaDelete:
		return false
	default:
		return true
	}
}

func addDatumReferences(refs stateReferences, datums []*statefulpb.LogDatum) {
	for _, datum := range datums {
		switch d := datum.Data.(type) {
		case *statefulpb.LogDatum_Log:
			addLogReferences(refs, d.Log)
		case *statefulpb.LogDatum_DeltaEncodingSync:
			addDeltaEncodingSyncReferences(refs, d.DeltaEncodingSync)
		case *statefulpb.LogDatum_JsonSchemaDefine:
			refs.addJsonSchema(d.JsonSchemaDefine.SchemaId)
			addJsonSchemaReferences(refs, d.JsonSchemaDefine)
		}
	}
}

func addLogReferences(refs stateReferences, log *statefulpb.Log) {
	if log == nil {
		return
	}
	addDictEntryIDReference(refs, log.Status)
	addDictEntryIDReference(refs, log.Service)
	addDictEntryIDReference(refs, log.Tags)
	if log.RawLog == "" {
		refs.addPattern(log.PatternId)
		addDynamicValueReferences(refs, log.DynamicValues)
	}
	addJSONSchemaIDReference(refs, log.JsonSchemaId)
	for _, dictID := range log.JsonContextDictValues {
		refs.addDictEntry(dictID)
	}
}

func addDeltaEncodingSyncReferences(refs stateReferences, sync *statefulpb.DeltaEncodingSync) {
	if sync == nil {
		return
	}
	refs.addPattern(sync.PatternId)
	addDictEntryIDReference(refs, sync.Tags)
	addDictEntryIDReference(refs, sync.Status)
	addDictEntryIDReference(refs, sync.Service)
	addJSONSchemaIDReference(refs, sync.JsonSchemaId)
}

func addJsonSchemaReferences(refs stateReferences, schema *statefulpb.JsonSchemaDefine) {
	if schema == nil {
		return
	}
	addDictEntryIDReference(refs, schema.MessageKeyId)
	for _, keyID := range schema.Keys {
		addDictEntryIDReference(refs, keyID)
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

func addDictEntryIDReference(refs stateReferences, id uint64) {
	if isEmptyDictIndex(id) {
		return
	}
	refs.addDictEntry(id)
}

func addJSONSchemaIDReference(refs stateReferences, id uint64) {
	if id == 0 || id == emptyDictIndex {
		return
	}
	refs.addJsonSchema(id)
}
