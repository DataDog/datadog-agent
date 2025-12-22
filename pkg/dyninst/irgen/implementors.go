// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irgen

import (
	"container/heap"

	"github.com/DataDog/datadog-agent/pkg/dyninst/gotype"
)

// implementorIterator is an iterator that can enumerate the implementations of
// an interface.
type implementorIterator struct {
	idx     methodToGoTypeIndex
	current gotype.TypeID
	done    bool
	heap    methodHeap
}

// makeImplementorIterator creates a new implementor iterator using the given
// method index.
func makeImplementorIterator(idx methodToGoTypeIndex) implementorIterator {
	return implementorIterator{
		idx: idx,
	}
}

type imethodsToGoTypeIterator = iterator[[]gotype.IMethod, gotype.TypeID]

var _ imethodsToGoTypeIterator = (*implementorIterator)(nil)

// init initializes the implementor iterator with the given method set, and
// returns the first implementation.
func (ii *implementorIterator) seek(imethods []gotype.IMethod) {
	h := &ii.heap
	// Reuse the existing iterators if possible.
	h.iters = h.iters[:min(cap(h.iters), len(imethods))]
	for i := range imethods {
		if i < len(h.iters) {
			if h.iters[i] == nil {
				h.iters[i] = ii.idx.Iterator()
			}
		} else {
			h.iters = append(h.iters, ii.idx.Iterator())
		}
	}
	ii.done = true
	for i, im := range imethods {
		m := gotype.Method{
			Name: im.Name,
			Mtyp: im.Typ,
		}
		if h.iters[i].seek(m); !h.iters[i].valid() {
			return
		}
	}
	h.len = len(imethods)
	h.init()
	ii.done = false
	ii.next()
}

func (ii *implementorIterator) valid() bool { return !ii.done }
func (ii *implementorIterator) cur() gotype.TypeID {
	if ii.valid() {
		return ii.current
	}
	return 0
}

// next returns the next implementation of the interface.
func (ii *implementorIterator) next() {
	if !ii.valid() {
		return
	}
	h := &ii.heap
outer:
	for {
		// Add all the popped iterators back to the heap.
		for _, iter := range h.iters[h.len:] {
			if iter.next(); !iter.valid() {
				ii.done = true
				return
			}
			h.push(iter)
		}
		// Pop the next implementation from the heap and check if all the
		// iterators have the same implementation.
		ii.current = h.pop().cur()
		for h.len > 0 {
			if h.iters[0].cur() != ii.current {
				continue outer
			}
			h.pop()
		}
		return // matched
	}
}

type methodHeap struct {
	iters []methodToGoTypeIterator
	len   int
}

func (h *methodHeap) push(iter methodToGoTypeIterator) {
	h.iters[h.len] = iter
	h.len++
	heap.Push((*methodHeapI)(h), nil)
}
func (h *methodHeap) pop() methodToGoTypeIterator {
	heap.Pop((*methodHeapI)(h))
	h.len--
	return h.iters[h.len]
}
func (h *methodHeap) init() { heap.Init((*methodHeapI)(h)) }

type methodHeapI methodHeap

func (h *methodHeapI) Len() int           { return h.len }
func (h *methodHeapI) Less(i, j int) bool { return h.iters[i].cur() < h.iters[j].cur() }
func (h *methodHeapI) Swap(i, j int)      { h.iters[i], h.iters[j] = h.iters[j], h.iters[i] }
func (h *methodHeapI) Push(any)           {}
func (h *methodHeapI) Pop() any           { return nil }
