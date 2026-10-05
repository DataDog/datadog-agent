// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package patch constructs request-local, explicit Pod admission patches.
// Typed snapshots are for decisions only; existing objects are never rebuilt
// from their typed representation.
package patch

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	jsonpatch "github.com/evanphx/json-patch/v5"
	corev1 "k8s.io/api/core/v1"
)

type operation struct {
	Op    string          `json:"op"`
	Path  string          `json:"path"`
	Value json.RawMessage `json:"value,omitempty"`
}

// PodSession owns the original document, current document and operation journal.
// It is single-threaded and must not be shared across requests or reinvocations.
type PodSession struct {
	original []byte
	current  []byte
	journal  []operation
	revision uint64
	snapshot *corev1.Pod
	err      error
	active   *batch
}

// NewPodSession validates and owns an admission Pod document.
func NewPodSession(raw []byte) (*PodSession, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, errors.New("admission Pod must be a JSON object")
	}
	var pod corev1.Pod
	if err := json.Unmarshal(raw, &pod); err != nil {
		return nil, errors.New("failed to decode admission Pod")
	}
	original := bytes.Clone(raw)
	return &PodSession{original: original, current: original, snapshot: &pod}, nil
}

// Snapshot returns a detached typed view of the current document.
func (s *PodSession) Snapshot() (*corev1.Pod, error) {
	if s.err != nil {
		return nil, s.err
	}
	if err := s.flushBatch(); err != nil {
		return nil, err
	}
	if s.snapshot == nil {
		var pod corev1.Pod
		if err := json.Unmarshal(s.current, &pod); err != nil {
			return nil, s.fail(errors.New("failed to decode patch snapshot"))
		}
		s.snapshot = &pod
	}
	return s.snapshot.DeepCopy(), nil
}

// Err returns the first construction or application error, including ignored errors.
func (s *PodSession) Err() error { return s.err }

// HasOperations reports whether the journal contains successful edits.
func (s *PodSession) HasOperations() bool { return len(s.journal) != 0 }

// JSONPatch returns exactly the operations applied locally, or the first error.
func (s *PodSession) JSONPatch() ([]byte, error) {
	if s.active != nil {
		return nil, s.fail(errors.New("cannot export a journal during a container batch"))
	}
	if s.err != nil {
		return nil, s.err
	}
	if len(s.journal) == 0 {
		return []byte("[]"), nil
	}
	result, err := json.Marshal(s.journal)
	if err != nil {
		return nil, s.fail(fmt.Errorf("failed to encode admission patch: %w", err))
	}
	return result, nil
}

func (s *PodSession) fail(err error) error {
	if s.err == nil {
		s.err = err
	}
	return s.err
}

// apply commits a complete semantic batch only after all operations validate.
func (s *PodSession) apply(ops []operation) error {
	if s.err != nil {
		return s.err
	}
	if len(ops) == 0 {
		return nil
	}
	wire, err := json.Marshal(ops)
	if err != nil {
		return s.fail(fmt.Errorf("failed to encode patch batch: %w", err))
	}
	p, err := jsonpatch.DecodePatch(wire)
	if err != nil {
		return s.fail(fmt.Errorf("failed to decode patch batch: %w", err))
	}
	options := jsonpatch.NewApplyOptions()
	options.SupportNegativeIndices = false
	options.AllowMissingPathOnRemove = false
	options.EnsurePathExistsOnAdd = false
	candidate, err := p.ApplyWithOptions(s.current, options)
	if err != nil {
		// Library errors can include operation values. Do not expose them through
		// wrapper logging, admission status or metrics.
		return s.fail(errors.New("failed to apply admission patch batch"))
	}
	s.current = candidate
	s.journal = append(s.journal, ops...)
	s.revision++
	s.snapshot = nil
	return nil
}

func pointer(path []string) string {
	var b strings.Builder
	for _, segment := range path {
		b.WriteByte('/')
		b.WriteString(strings.ReplaceAll(strings.ReplaceAll(segment, "~", "~0"), "/", "~1"))
	}
	return b.String()
}

// batch inspects only requested ancestors. Created parents are tracked so a
// batch never emits repeated parent initialization. Inspection preserves raw
// number tokens and never serializes existing ancestors as operation values.
type batch struct {
	s          *PodSession
	ops        []operation
	values     map[string]json.RawMessage
	objects    map[string]map[string]json.RawMessage
	lists      map[string][]json.RawMessage
	containers map[ContainerKind]map[string]int
	views      map[string]*corev1.Container
}

func (s *PodSession) batch(fn func(*batch) error) error {
	if s.err != nil {
		return s.err
	}
	b := s.active
	if b == nil {
		b = s.newBatch()
	}
	count := len(b.ops)
	if err := fn(b); err != nil {
		return s.fail(err)
	}
	if s.active != nil {
		if len(b.ops) != count {
			b.updateContainerViews(b.ops[count:])
			s.revision++
			s.snapshot = nil
		}
		return nil
	}
	return s.apply(b.ops)
}

// ForContainers is a transactional semantic batch for an ordered container
// stage. Its callback executes once per identity; snapshots observe prior edits.
// The outer boundary validates and commits the complete journal through the
// same patch library. It must not be used to retry external decisions.
func (s *PodSession) ForContainers(ids []ContainerID, plan func(ContainerID) error) error {
	if s.err != nil {
		return s.err
	}
	if s.active != nil {
		for _, id := range ids {
			if err := plan(id); err != nil {
				return err
			}
		}
		return s.err
	}
	before, snapshot, revision, count := s.current, s.snapshot, s.revision, len(s.journal)
	s.active = s.newBatch()
	var err error
	for _, id := range ids {
		if err = plan(id); err != nil {
			break
		}
		if err = s.err; err != nil {
			break
		}
	}
	if err == nil {
		err = s.apply(s.active.ops)
	}
	s.active = nil
	if err != nil {
		s.current, s.snapshot, s.revision = before, snapshot, revision
		s.journal = s.journal[:count]
		return err
	}
	return nil
}

func (s *PodSession) flushBatch() error {
	if s.active != nil && len(s.active.ops) > 0 {
		if err := s.apply(s.active.ops); err != nil {
			return err
		}
		s.active = s.newBatch()
	}
	return nil
}

// Aggregate readers need raw entries that include any earlier descendant edits.
// Flush only when the selected field has pending operations; scalar readers can
// keep using the inspector's current values without another document application.
func (s *PodSession) flushContainerField(id ContainerID, field string) error {
	if s.active == nil {
		return nil
	}
	path, err := s.active.container(id)
	if err != nil {
		return s.fail(err)
	}
	prefix := pointer(append(path, field))
	for _, op := range s.active.ops {
		if op.Path == prefix || strings.HasPrefix(op.Path, prefix+"/") {
			return s.flushBatch()
		}
	}
	return nil
}

func (b *batch) removeIndex(path []string, index int) error {
	list, err := b.list(path)
	if err != nil {
		return err
	}
	if index < 0 || index >= len(list) {
		return errors.New("invalid selected array occurrence")
	}
	b.ops = append(b.ops, operation{Op: "remove", Path: pointer(append(append([]string(nil), path...), strconv.Itoa(index)))})
	key := pointer(path)
	b.invalidateChildren(key)
	b.lists[key] = append(list[:index:index], list[index+1:]...)
	if len(path) == 2 && path[0] == "spec" {
		delete(b.containers, ContainerKind(path[1]))
	}
	return nil
}

func (s *PodSession) newBatch() *batch {
	return &batch{s: s, values: map[string]json.RawMessage{"": s.current},
		objects: make(map[string]map[string]json.RawMessage), lists: make(map[string][]json.RawMessage), containers: make(map[ContainerKind]map[string]int), views: make(map[string]*corev1.Container)}
}

func (b *batch) get(path []string) (json.RawMessage, error) {
	key := pointer(path)
	if raw, ok := b.values[key]; ok {
		return raw, nil
	}
	parent, err := b.get(path[:len(path)-1])
	if err != nil || len(parent) == 0 || bytes.Equal(bytes.TrimSpace(parent), []byte("null")) {
		return nil, err
	}
	var raw json.RawMessage
	switch bytes.TrimSpace(parent)[0] {
	case '{':
		parentKey := pointer(path[:len(path)-1])
		object, ok := b.objects[parentKey]
		if !ok {
			if err := json.Unmarshal(parent, &object); err != nil {
				return nil, fmt.Errorf("invalid object at %s", parentKey)
			}
			b.objects[parentKey] = object
		}
		raw = object[path[len(path)-1]]
	case '[':
		list, err := b.list(path[:len(path)-1])
		if err != nil {
			return nil, err
		}
		i, err := strconv.Atoi(path[len(path)-1])
		if err != nil || i < 0 || i >= len(list) {
			return nil, fmt.Errorf("invalid array position at %s", key)
		}
		raw = list[i]
	default:
		return nil, fmt.Errorf("incompatible parent at %s", pointer(path[:len(path)-1]))
	}
	b.values[key] = raw
	return raw, nil
}

func (b *batch) parent(path []string, shape byte) error {
	raw, err := b.get(path)
	if err != nil {
		return err
	}
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		if len(path) > 0 {
			if err := b.parent(path[:len(path)-1], '{'); err != nil {
				return err
			}
		}
		value := json.RawMessage("{}")
		if shape == '[' {
			value = json.RawMessage("[]")
		}
		b.ops = append(b.ops, operation{Op: "add", Path: pointer(path), Value: value})
		b.invalidateChildren(pointer(path))
		b.values[pointer(path)] = value
		return nil
	}
	if bytes.TrimSpace(raw)[0] != shape {
		return fmt.Errorf("incompatible parent at %s", pointer(path))
	}
	return nil
}

func (b *batch) set(path []string, value any) error {
	if err := b.parent(path[:len(path)-1], '{'); err != nil {
		return err
	}
	raw, err := b.get(path)
	if err != nil {
		return err
	}
	v, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to encode patch value at %s", pointer(path))
	}
	if len(raw) > 0 && exactEqual(raw, v) {
		return nil
	}
	op := "add"
	if len(raw) > 0 {
		op = "replace"
	}
	b.ops = append(b.ops, operation{Op: op, Path: pointer(path), Value: v})
	b.invalidateChildren(pointer(path))
	b.values[pointer(path)] = v
	return nil
}

func (b *batch) remove(path []string) error {
	raw, err := b.get(path)
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		return nil
	}
	b.ops = append(b.ops, operation{Op: "remove", Path: pointer(path)})
	b.invalidateChildren(pointer(path))
	b.values[pointer(path)] = nil
	return nil
}

func (b *batch) list(path []string) ([]json.RawMessage, error) {
	key := pointer(path)
	if list, ok := b.lists[key]; ok {
		return list, nil
	}
	raw, err := b.get(path)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	var list []json.RawMessage
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("expected array at %s", pointer(path))
	}
	b.lists[key] = list
	return list, nil
}

func (b *batch) insert(path []string, value any, prepend bool) error {
	if err := b.parent(path, '['); err != nil {
		return err
	}
	v, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to encode array entry at %s", pointer(path))
	}
	position := "-"
	if prepend {
		position = "0"
	}
	b.ops = append(b.ops, operation{Op: "add", Path: pointer(append(path, position)), Value: v})
	// Keep inspection of this array consistent within the batch. These bytes
	// are never emitted as a replacement of the existing array.
	list, err := b.list(path)
	if err != nil {
		return err
	}
	if prepend {
		list = append([]json.RawMessage{v}, list...)
	} else {
		list = append(list, v)
	}
	key := pointer(path)
	// Invalidate positions beneath an inserted list; the inspector retains raw
	// entries without re-encoding the existing list or ancestors.
	for k := range b.values {
		if strings.HasPrefix(k, key+"/") {
			delete(b.values, k)
		}
	}
	for k := range b.objects {
		if strings.HasPrefix(k, key+"/") {
			delete(b.objects, k)
		}
	}
	for k := range b.lists {
		if strings.HasPrefix(k, key+"/") {
			delete(b.lists, k)
		}
	}
	b.lists[key] = list
	if len(path) == 2 && path[0] == "spec" {
		delete(b.containers, ContainerKind(path[1]))
	}
	return nil
}

func (b *batch) invalidateChildren(path string) {
	for key := range b.values {
		if strings.HasPrefix(key, path+"/") {
			delete(b.values, key)
		}
	}
	for key := range b.objects {
		if key == path || strings.HasPrefix(key, path+"/") {
			delete(b.objects, key)
		}
	}
	for key := range b.lists {
		if key == path || strings.HasPrefix(key, path+"/") {
			delete(b.lists, key)
		}
	}
}

func exactEqual(a, b []byte) bool {
	decode := func(raw []byte) (any, error) {
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		var value any
		err := decoder.Decode(&value)
		return value, err
	}
	x, err := decode(a)
	if err != nil {
		return false
	}
	y, err := decode(b)
	return err == nil && reflect.DeepEqual(x, y)
}
