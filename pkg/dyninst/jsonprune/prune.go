// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package jsonprune

import (
	"bytes"
	"errors"
	"io"
	"sort"
	"sync"

	"github.com/go-json-experiment/json/jsontext"
)

// MaxSnapshotBytes is the per-snapshot size budget used by the module when
// invoking Prune. It is 1 MiB minus a 4 KiB reserve for upload-path framing
// so the final shipped log message fits within the Event Platform's 1 MiB
// per-message limit.
const MaxSnapshotBytes = 1<<20 - 4096

// minPruneLevel is the first JSON nesting depth at which captured values
// appear in the snapshot schema. Objects at shallower levels are envelope
// and must never be pruned.
//
// Levels 0-4: root, debugger.snapshot, captures, entry/return/lines...
// Level  5+: individual captured values (arguments, locals entries).
const minPruneLevel = 5

// Placeholder byte sequences. When pruning an object we replace its full
// byte range with placeholderWithTypePrefix + type-value bytes +
// placeholderWithTypeSuffix, or just placeholderNoType if the original
// object lacked a "type" field.
var (
	placeholderWithTypePrefix = []byte(`{"type":`)
	placeholderWithTypeSuffix = []byte(`,"notCapturedReason":"pruned"}`)
	placeholderNoType         = []byte(`{"notCapturedReason":"pruned"}`)
)

// Prune returns a byte slice no longer than maxSize whenever pruning is
// possible. If the input already fits, Prune returns the input unchanged.
// If the input is malformed, or has no pruneable captured values, Prune
// returns the input unchanged even when it exceeds maxSize — the caller
// must decide what to do with an unshrinkable message.
//
// The returned slice either aliases input (fast path) or is freshly
// allocated (pruned path). The caller must not mutate the returned slice
// while Prune may still be reading input.
func Prune(input []byte, maxSize int) []byte {
	if len(input) <= maxSize {
		return input
	}
	s := scratchPool.Get().(*scratch)
	defer releaseScratch(s)

	if err := s.parse(input); err != nil {
		return input
	}
	if !s.prune(len(input), maxSize) {
		return input
	}
	return s.emit(input)
}

// flag bits held in node.flags.
const (
	flagNotCaptured uint8 = 1 << 0 // object has a "notCapturedReason" key
	flagDepthReason uint8 = 1 << 1 // its value is the string "depth"
	flagPruned      uint8 = 1 << 2 // chosen for pruning
	flagDominated   uint8 = 1 << 3 // an ancestor is already pruned
	flagInHeap      uint8 = 1 << 4 // currently referenced by the heap
	flagNoEmit      uint8 = 1 << 5 // pruned for promotion only; keep original bytes
)

// nodeID indexes into scratch.nodes. 0 is reserved as an invalid sentinel
// so struct zero-values clearly denote "no node".
type nodeID uint32

const invalidNodeID nodeID = 0

// node is a single JSON object recorded during parsing. Children form an
// intrusive singly-linked list so no per-node child slice is allocated.
type node struct {
	start, end           uint32
	parent               nodeID
	firstChild           nodeID
	nextSibling          nodeID
	typeStart, typeEnd   uint32
	level                uint16
	unprunedChildObjects uint16
	flags                uint8
}

// isLeafObject reports whether the node has no child objects.
func (n *node) isLeafObject() bool { return n.firstChild == invalidNodeID }

// size is the byte length of the object in the input.
func (n *node) size() uint32 { return n.end - n.start }

// scratch holds the per-Prune state, pooled via scratchPool.
type scratch struct {
	nodes  []node
	heap   []nodeID
	pruned []nodeID
}

// scratchMaxRetain caps the capacity retained in the pool between calls.
const scratchMaxRetain = 1 << 16 // 64k nodes

var scratchPool = sync.Pool{
	New: func() any {
		return &scratch{
			nodes: make([]node, 1, 64), // index 0 is sentinel
		}
	},
}

func releaseScratch(s *scratch) {
	if cap(s.nodes) > scratchMaxRetain {
		return
	}
	s.nodes = s.nodes[:1]
	s.heap = s.heap[:0]
	s.pruned = s.pruned[:0]
	scratchPool.Put(s)
}

// parse walks input with a jsontext.Decoder, recording every object's
// [start, end) byte range, its "type" field value span (if any), and its
// "notCapturedReason" flags. Leaf objects at level >= minPruneLevel are
// pushed onto the heap at close time.
func (s *scratch) parse(input []byte) error {
	// AllowDuplicateNames skips jsontext's per-object key-uniqueness
	// tracking. We don't care about duplicate keys (the decoder
	// upstream already wrote well-formed output) and skipping the
	// bookkeeping avoids an allocation per object member.
	dec := jsontext.NewDecoder(
		bytes.NewReader(input),
		jsontext.AllowDuplicateNames(true),
	)
	var stack []nodeID

	for {
		kind := dec.PeekKind()
		if kind == 0 {
			// EOF. Consume the signalling read for error discipline.
			_, err := dec.ReadToken()
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		switch kind {
		case '{':
			if _, err := dec.ReadToken(); err != nil {
				return err
			}
			start := uint32(dec.InputOffset()) - 1
			id := nodeID(len(s.nodes))
			var parent nodeID
			level := uint16(0)
			if n := len(stack); n > 0 {
				parent = stack[n-1]
				level = s.nodes[parent].level + 1
			}
			s.nodes = append(s.nodes, node{
				start:  start,
				parent: parent,
				level:  level,
			})
			if parent != invalidNodeID {
				p := &s.nodes[parent]
				s.nodes[id].nextSibling = p.firstChild
				p.firstChild = id
				p.unprunedChildObjects++
			}
			stack = append(stack, id)

		case '}':
			if _, err := dec.ReadToken(); err != nil {
				return err
			}
			id := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			n := &s.nodes[id]
			n.end = uint32(dec.InputOffset())
			if n.isLeafObject() && n.level >= minPruneLevel {
				s.heapPush(id)
			}

		case '[', ']':
			if _, err := dec.ReadToken(); err != nil {
				return err
			}

		case '"':
			if len(stack) > 0 && isObjectKeyPosition(dec) {
				tok, err := dec.ReadToken()
				if err != nil {
					return err
				}
				key := tok.String()
				if err := s.handleKey(dec, input, stack[len(stack)-1], key); err != nil {
					return err
				}
			} else {
				if _, err := dec.ReadToken(); err != nil {
					return err
				}
			}

		default: // number, true, false, null
			if _, err := dec.ReadToken(); err != nil {
				return err
			}
		}
	}
}

// handleKey reads the value following a known key and, for the two keys
// we care about, records flags / offsets on the current node.
func (s *scratch) handleKey(
	dec *jsontext.Decoder, input []byte, id nodeID, key string,
) error {
	switch key {
	case "type":
		return s.readTypeValue(dec, input, id)
	case "notCapturedReason":
		return s.readNotCapturedReason(dec, id)
	}
	// Unknown key: fall back to the main loop, which will consume the
	// next token(s) as normal. No-op here.
	return nil
}

// readTypeValue records the byte span of the type field's string value
// (after the surrounding quotes are stripped). If the value is not a
// string, typeStart/typeEnd remain 0 and the placeholder will fall back
// to the no-type variant.
func (s *scratch) readTypeValue(
	dec *jsontext.Decoder, input []byte, id nodeID,
) error {
	if dec.PeekKind() != '"' {
		return nil
	}
	if _, err := dec.ReadToken(); err != nil {
		return err
	}
	end := uint32(dec.InputOffset())
	start := innerStringStart(input, end)
	if start == 0 {
		return nil
	}
	s.nodes[id].typeStart = start
	s.nodes[id].typeEnd = end - 1 // drop trailing '"'
	return nil
}

// readNotCapturedReason sets flags on the node based on the string value
// of a "notCapturedReason" field.
func (s *scratch) readNotCapturedReason(
	dec *jsontext.Decoder, id nodeID,
) error {
	if dec.PeekKind() != '"' {
		return nil
	}
	tok, err := dec.ReadToken()
	if err != nil {
		return err
	}
	s.nodes[id].flags |= flagNotCaptured
	if tok.String() == "depth" {
		s.nodes[id].flags |= flagDepthReason
	}
	return nil
}

// isObjectKeyPosition reports whether the next token will be read as an
// object key (as opposed to a value).
func isObjectKeyPosition(dec *jsontext.Decoder) bool {
	depth := dec.StackDepth()
	if depth == 0 {
		return false
	}
	kind, n := dec.StackIndex(depth)
	if kind != '{' {
		return false
	}
	return n%2 == 0
}

// innerStringStart returns the offset of the first byte *inside* the
// string literal ending at `end` (i.e., one past the opening quote).
// Returns 0 if the input is malformed.
func innerStringStart(input []byte, end uint32) uint32 {
	if end < 2 || end > uint32(len(input)) || input[end-1] != '"' {
		return 0
	}
	pos := int(end) - 2
	for pos >= 0 {
		if input[pos] != '"' {
			pos--
			continue
		}
		bs := 0
		for bs < pos && input[pos-1-bs] == '\\' {
			bs++
		}
		if bs%2 == 0 {
			return uint32(pos + 1)
		}
		pos--
	}
	return 0
}

// priorityKey packs a node's comparison key into a uint64. Layout:
//
//	bit 63       depthReason
//	bits 48..62  level (15 bits)
//	bit 47       notCaptured
//	bits 0..31   size in bytes
func (s *scratch) priorityKey(id nodeID) uint64 {
	n := &s.nodes[id]
	var k uint64
	if n.flags&flagDepthReason != 0 {
		k |= 1 << 63
	}
	k |= uint64(n.level&0x7FFF) << 48
	if n.flags&flagNotCaptured != 0 {
		k |= 1 << 47
	}
	k |= uint64(n.size())
	return k
}

func (s *scratch) heapPush(id nodeID) {
	s.nodes[id].flags |= flagInHeap
	s.heap = append(s.heap, id)
	s.siftUp(len(s.heap) - 1)
}

func (s *scratch) heapPop() (nodeID, bool) {
	if len(s.heap) == 0 {
		return invalidNodeID, false
	}
	top := s.heap[0]
	last := len(s.heap) - 1
	s.heap[0] = s.heap[last]
	s.heap = s.heap[:last]
	if last > 0 {
		s.siftDown(0)
	}
	s.nodes[top].flags &^= flagInHeap
	return top, true
}

func (s *scratch) siftUp(i int) {
	for i > 0 {
		p := (i - 1) / 2
		if s.priorityKey(s.heap[i]) <= s.priorityKey(s.heap[p]) {
			return
		}
		s.heap[i], s.heap[p] = s.heap[p], s.heap[i]
		i = p
	}
}

func (s *scratch) siftDown(i int) {
	n := len(s.heap)
	for {
		l := 2*i + 1
		if l >= n {
			return
		}
		best := l
		if r := l + 1; r < n && s.priorityKey(s.heap[r]) > s.priorityKey(s.heap[l]) {
			best = r
		}
		if s.priorityKey(s.heap[i]) >= s.priorityKey(s.heap[best]) {
			return
		}
		s.heap[i], s.heap[best] = s.heap[best], s.heap[i]
		i = best
	}
}

// prune repeatedly pops the top heap element and marks it pruned until
// the cumulative byte size drops to or below maxSize. It correctly
// accounts for descendants that had already been pruned: when a new
// node is selected, the effective "saving" attributable to any pruned
// descendant is replaced by the saving from pruning the ancestor.
// Returns true if at least one node was pruned.
func (s *scratch) prune(totalBytes, maxSize int) bool {
	currentSize := totalBytes
	anyPruned := false
	for currentSize > maxSize {
		id, ok := s.heapPop()
		if !ok {
			break
		}
		n := &s.nodes[id]
		if n.flags&flagPruned != 0 {
			continue
		}
		// Discount savings from any already-pruned descendants of this
		// node: they are about to become dominated and no longer
		// contribute their placeholder-delta to the output.
		refund := s.dominateAndSumSavings(id)
		currentSize += refund

		n.flags |= flagPruned
		anyPruned = true

		saving := int(n.size()) - placeholderSize(n)
		if saving <= 0 {
			// Placeholder would be the same size or larger than the
			// original. Don't emit a placeholder — just leaf-promote
			// so the parent can be considered next.
			n.flags |= flagNoEmit
		} else {
			s.pruned = append(s.pruned, id)
			currentSize -= saving
		}

		if n.parent != invalidNodeID {
			p := &s.nodes[n.parent]
			if p.unprunedChildObjects > 0 {
				p.unprunedChildObjects--
			}
			if p.unprunedChildObjects == 0 &&
				p.level >= minPruneLevel &&
				p.flags&flagPruned == 0 &&
				p.flags&flagInHeap == 0 {
				p.flags |= flagDepthReason
				s.heapPush(n.parent)
			}
		}
	}
	return anyPruned
}

// dominateAndSumSavings marks every descendant of id as dominated and
// returns the sum of savings those descendants had contributed to
// currentSize (so the caller can refund them). Descendants that were
// already dominated are skipped — their savings were refunded on the
// earlier call that dominated them.
func (s *scratch) dominateAndSumSavings(id nodeID) int {
	var sum int
	var walk func(nodeID)
	walk = func(id nodeID) {
		child := s.nodes[id].firstChild
		for child != invalidNodeID {
			n := &s.nodes[child]
			if n.flags&flagDominated == 0 {
				if n.flags&flagPruned != 0 && n.flags&flagNoEmit == 0 {
					sum += int(n.size()) - placeholderSize(n)
				}
				n.flags |= flagDominated
				walk(child)
			}
			child = n.nextSibling
		}
	}
	walk(id)
	return sum
}

// placeholderSize computes the exact byte length of the placeholder that
// will replace node n.
func placeholderSize(n *node) int {
	if n.typeEnd == n.typeStart {
		return len(placeholderNoType)
	}
	return len(placeholderWithTypePrefix) +
		2 + // surrounding quotes we emit around the type value
		int(n.typeEnd-n.typeStart) +
		len(placeholderWithTypeSuffix)
}

// emit walks the input in order, copying unpruned spans verbatim and
// substituting pruned spans with their placeholder.
func (s *scratch) emit(input []byte) []byte {
	// Filter out dominated nodes from the pruned list; sort by start.
	live := s.pruned[:0:cap(s.pruned)]
	for _, id := range s.pruned {
		if s.nodes[id].flags&flagDominated == 0 {
			live = append(live, id)
		}
	}
	sort.Slice(live, func(i, j int) bool {
		return s.nodes[live[i]].start < s.nodes[live[j]].start
	})

	// Precompute exact output size for a single allocation.
	outLen := len(input)
	for _, id := range live {
		n := &s.nodes[id]
		outLen -= int(n.size())
		outLen += placeholderSize(n)
	}

	out := make([]byte, 0, outLen)
	cursor := uint32(0)
	for _, id := range live {
		n := &s.nodes[id]
		if cursor < n.start {
			out = append(out, input[cursor:n.start]...)
		}
		if n.typeEnd == n.typeStart {
			out = append(out, placeholderNoType...)
		} else {
			out = append(out, placeholderWithTypePrefix...)
			out = append(out, '"')
			out = append(out, input[n.typeStart:n.typeEnd]...)
			out = append(out, '"')
			out = append(out, placeholderWithTypeSuffix...)
		}
		cursor = n.end
	}
	if int(cursor) < len(input) {
		out = append(out, input[cursor:]...)
	}
	return out
}
