// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package usdt

import (
	"errors"
	"fmt"
	"sync"
)

// Names of the eBPF objects defined by pkg/ebpf/c/usdt.h.
const (
	// SpecIDConstant is the constant holding the id of the Spec of the probe
	// sites a program instance is attached to.
	SpecIDConstant = "usdt_spec_id"
	// SpecsMapName is the name of the array map holding the Specs, indexed
	// by id.
	SpecsMapName = "usdt_specs"
)

// ErrNoSpecID is returned by SpecRegistry.Acquire when all the spec ids are in use.
var ErrNoSpecID = errors.New("no USDT spec id available")

// SpecRegistry assigns ids to the distinct Specs in use, with reference
// counting, so that all the probe sites sharing a Spec share one entry of the
// usdt_specs eBPF map, and one program instance.
//
// Ids are in [1, MaxSpecs): id 0 is never allocated, so that a program whose
// "usdt_spec_id" constant was not edited cannot decode the arguments of
// another site. A released id can be reallocated to a different Spec.
//
// The zero value is an empty registry. A SpecRegistry is safe for concurrent
// use and must not be copied.
type SpecRegistry struct {
	mu  sync.Mutex
	ids map[Spec]uint32
	// entries[id] is nil when id is free
	entries [MaxSpecs]*specEntry
}

type specEntry struct {
	spec Spec
	refs uint32
}

// Acquire takes a reference on spec and returns its id. isNew is true if the
// id was allocated by this call: the caller must then store spec under id in
// the usdt_specs map (and set up the program instance for it) before
// attaching sites with it. Every successful Acquire must be balanced by a
// Release.
func (r *SpecRegistry) Acquire(spec Spec) (id uint32, isNew bool, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if id, ok := r.ids[spec]; ok {
		r.entries[id].refs++
		return id, false, nil
	}
	for id = 1; id < MaxSpecs; id++ {
		if r.entries[id] == nil {
			if r.ids == nil {
				r.ids = make(map[Spec]uint32)
			}
			r.ids[spec] = id
			r.entries[id] = &specEntry{spec: spec, refs: 1}
			return id, true, nil
		}
	}
	return 0, false, ErrNoSpecID
}

// Release drops a reference on the spec id. freed is true if it was the last
// one: the caller must then detach the program instance of id, since the id
// may be reallocated to another Spec by a later Acquire.
func (r *SpecRegistry) Release(id uint32) (freed bool, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if id >= MaxSpecs || r.entries[id] == nil {
		return false, fmt.Errorf("USDT spec id %d is not in use", id)
	}
	e := r.entries[id]
	e.refs--
	if e.refs > 0 {
		return false, nil
	}
	delete(r.ids, e.spec)
	r.entries[id] = nil
	return true, nil
}

// Len returns the number of distinct Specs in use.
func (r *SpecRegistry) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.ids)
}
