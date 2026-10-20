// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package ntfsmft

import (
	"bytes"
	"container/heap"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// FindQuery is one filename-matching find request.
//
//   - Type "ext":   Value is a comma-separated list of extensions. Leading
//     dots and case are ignored (e.g. ".dmp,.etl,DMP").
//   - Type "glob":  Value is a single filepath.Match pattern matched against
//     the basename (no path separators).
//   - Type "regex": Value is an RE2 expression matched against the basename.
//
// Limit caps this query's result block; 0 selects a sensible default (100).
// Label is opaque, carried into FindResultBlock for caller attribution.
type FindQuery struct {
	Type  string
	Value string
	Limit int
	Label string
}

// FindResultBlock pairs a FindQuery with the files it matched, sorted by
// size descending then basename ascending and trimmed to the query's Limit.
type FindResultBlock struct {
	Query   FindQuery
	Matches []FileEntry
}

// matchSlot is one per-query predicate + heap. Each FindQuery passed to
// newMatchSet becomes one slot.
type matchSlot struct {
	query FindQuery
	// Exactly one of the following is populated, per query.Type:
	exts  [][]byte       // ext: pre-normalized lowercased extensions, no dot
	glob  string         // glob: filepath.Match pattern
	regex *regexp.Regexp // regex: compiled RE2
	heap  fileHeap
	cap   int
}

// matchSet evaluates per-file predicates against a list of independent
// FindQuery slots. Each slot retains its own top-Cap candidates via a
// min-heap.
//
// Hot-path cost when matchSet is nil: a single nil check per file. When
// multiple slots need the same input (extension or decoded basename),
// that work is done at most once per file via lazy locals in consider.
type matchSet struct {
	slots []*matchSlot
	// minSize is the file-size floor: files strictly smaller are not
	// considered for any query, matching the top-files floor semantics.
	minSize int64
}

// newMatchSet builds a matcher from a list of FindQuery. Returns (nil, nil)
// when queries is empty. Returns an error if any query is malformed (empty
// value, unknown type, bad glob, bad regex). minSize excludes files smaller
// than the threshold from all queries (0 = no floor).
func newMatchSet(queries []FindQuery, minSize int64) (*matchSet, error) {
	if len(queries) == 0 {
		return nil, nil
	}
	m := &matchSet{minSize: minSize}
	for i, q := range queries {
		if q.Value == "" {
			return nil, fmt.Errorf("find[%d]: value must not be empty", i)
		}
		c := q.Limit
		if c <= 0 {
			c = 100
		}
		slot := &matchSlot{query: q, cap: c, heap: make(fileHeap, 0, c)}
		switch q.Type {
		case "ext":
			for _, e := range splitAndNormalizeExts(q.Value) {
				slot.exts = append(slot.exts, []byte(e))
			}
			if len(slot.exts) == 0 {
				return nil, fmt.Errorf("find[%d]: ext value %q yielded no extensions", i, q.Value)
			}
		case "glob":
			// Validate eagerly via probe; filepath.Match only reports
			// ErrBadPattern when it encounters the bad character.
			if _, err := filepath.Match(q.Value, "x"); err != nil {
				return nil, fmt.Errorf("find[%d]: invalid glob %q: %w", i, q.Value, err)
			}
			slot.glob = q.Value
		case "regex":
			re, err := regexp.Compile(q.Value)
			if err != nil {
				return nil, fmt.Errorf("find[%d]: invalid regex %q: %w", i, q.Value, err)
			}
			slot.regex = re
		default:
			return nil, fmt.Errorf("find[%d]: unknown type %q (want \"ext\", \"glob\", or \"regex\")", i, q.Type)
		}
		m.slots = append(m.slots, slot)
	}
	return m, nil
}

// splitAndNormalizeExts parses ".dmp,.etl,DMP" into ["dmp", "etl", "dmp"]
// (lowercased, leading dot stripped, blanks dropped).
func splitAndNormalizeExts(csv string) []string {
	if csv == "" {
		return nil
	}
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.TrimPrefix(p, ".")
		p = strings.ToLower(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

// consider runs in the pass-2 hot path. Evaluates each slot's predicate
// against the file and pushes to the slot's heap on match. Per-file work
// (extension extraction, name decode) happens at most once, lazily.
func (m *matchSet) consider(idx uint64, e *mftEntry, sz int64) {
	if m == nil {
		return
	}
	// Size floor: skip files below --min before any predicate work, so
	// --find surfaces only large matches (e.g. "find large .dmp files").
	if sz < m.minSize {
		return
	}

	// Lazy extension extraction. The stack buffer covers every legal ASCII
	// $FILE_NAME extension; non-ASCII suffixes decode only if an ext query
	// actually needs to inspect them.
	var extBuf [255]byte
	var extN int
	extEvaluated := false
	getExt := func() ([]byte, bool) {
		if !extEvaluated {
			extEvaluated = true
			extN = extractAsciiExtension(e.nameBytes, extBuf[:])
		}
		if extN <= 0 {
			return nil, false
		}
		return extBuf[:extN], true
	}
	var decodedExt string
	decodedExtEvaluated := false
	getDecodedExt := func() string {
		if !decodedExtEvaluated {
			decodedExtEvaluated = true
			decodedExt = decodedExtension(e.nameBytes)
		}
		return decodedExt
	}

	// Lazy name decode. decodeUTF16Name returns a heap-allocated string safe to
	// retain past this call (reused for matching and, on a match, as the
	// candidate basename).
	var name string
	nameEvaluated := false
	getName := func() string {
		if !nameEvaluated {
			nameEvaluated = true
			name = decodeUTF16Name(e.nameBytes)
		}
		return name
	}

	for _, s := range m.slots {
		matched := false
		switch {
		case len(s.exts) > 0:
			ext, ok := getExt()
			if ok {
				for _, want := range s.exts {
					if bytes.Equal(ext, want) {
						matched = true
						break
					}
				}
			} else if extN == nonASCIIExtension {
				for _, want := range s.exts {
					if getDecodedExt() == string(want) {
						matched = true
						break
					}
				}
			}
		case s.glob != "":
			if ok, _ := filepath.Match(s.glob, getName()); ok {
				matched = true
			}
		case s.regex != nil:
			if s.regex.MatchString(getName()) {
				matched = true
			}
		}
		if !matched {
			continue
		}
		// push needs a basename. Reuse the one a glob/regex slot already
		// decoded; the pure-ext match path leaves pushName empty so push can
		// decode lazily (avoiding the decode when the heap rejects the file).
		pushName := ""
		if nameEvaluated {
			pushName = name
		}
		s.push(idx, e, sz, pushName)
	}
}

// push inserts a matched candidate into the slot's heap. If the heap is
// at capacity, the smallest entry is evicted unless the new entry is also
// smaller — in which case nothing happens. The basename is decoded lazily
// here when the caller didn't already have one (extension match path),
// avoiding the decode for files we end up evicting.
func (s *matchSlot) push(idx uint64, e *mftEntry, sz int64, name string) {
	if len(s.heap) >= s.cap && sz <= s.heap[0].size {
		return
	}
	if name == "" {
		name = decodeUTF16Name(e.nameBytes)
	}
	cand := fileCandidate{idx: idx, sequence: e.sequence, size: sz, basename: name}
	if len(s.heap) < s.cap {
		heap.Push(&s.heap, cand)
		return
	}
	s.heap[0] = cand
	heap.Fix(&s.heap, 0)
}

// drained returns one block per slot, in input-query order. Each block is
// sorted by size desc, then basename asc.
func (m *matchSet) drained() [][]fileCandidate {
	if m == nil {
		return nil
	}
	out := make([][]fileCandidate, len(m.slots))
	for i, s := range m.slots {
		blk := make([]fileCandidate, len(s.heap))
		copy(blk, s.heap)
		sort.Slice(blk, func(a, b int) bool {
			if blk[a].size != blk[b].size {
				return blk[a].size > blk[b].size
			}
			return blk[a].basename < blk[b].basename
		})
		out[i] = blk
	}
	return out
}

// queries returns the original FindQuery slice in slot order. Used by
// callers to pair drained() output back to the input queries.
func (m *matchSet) queries() []FindQuery {
	if m == nil {
		return nil
	}
	out := make([]FindQuery, len(m.slots))
	for i, s := range m.slots {
		out[i] = s.query
	}
	return out
}
