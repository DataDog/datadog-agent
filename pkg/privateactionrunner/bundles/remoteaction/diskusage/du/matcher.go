//go:build windows

package du

import (
	"bytes"
	"container/heap"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unsafe"
)

// matchSet evaluates per-file predicates (extension, glob, regex) and
// retains the top-Cap largest matches via a min-heap.
//
// Predicates OR together: a file matches if it satisfies any one of the
// configured filters. The heap is min-on-size so larger matches evict
// smaller ones once Cap is reached. After the scan, drained() returns
// candidates in descending-size order and resolveCandidatePaths() turns
// them into file paths.
//
// Hot-path cost when matchSet is nil: a single nil check per file. When
// only an extension predicate is set, evaluation is allocation-free
// (reuses extractAsciiExtension into a stack buffer). Glob/regex paths
// decode the UTF-16 basename once per file; the decoded string is reused
// for heap insertion if the file qualifies.
type matchSet struct {
	// Pre-normalized lowercased ASCII extensions, no leading dot.
	// Empty means no extension filter.
	extensions [][]byte
	// Glob patterns (filepath.Match syntax). Empty means no glob filter.
	globs []string
	// Compiled regex. Nil means no regex filter.
	regex *regexp.Regexp
	// Min-heap on size, capacity Cap.
	heap fileHeap
	cap  int

	// fast, when true, decodes ASCII filenames in-place into nameBuf and
	// passes a non-allocating string view (unsafe.String) to filepath.Match
	// and regexp.MatchString. Non-ASCII filenames fall back to the heap
	// allocator. The string MUST NOT escape the predicate evaluation —
	// nameBuf is reused on the next file.
	fast    bool
	nameBuf [512]byte
}

// newMatchSet builds a matcher from CLI-style strings. Returns (nil, nil)
// when no predicate is configured. Returns an error if a glob or regex
// fails to compile.
func newMatchSet(extsCSV string, globs []string, regex string, cap int, fast bool) (*matchSet, error) {
	exts := splitAndNormalizeExts(extsCSV)
	if len(exts) == 0 && len(globs) == 0 && regex == "" {
		return nil, nil
	}
	if cap <= 0 {
		cap = 100
	}
	m := &matchSet{cap: cap, heap: make(fileHeap, 0, cap), fast: fast}
	for _, e := range exts {
		m.extensions = append(m.extensions, []byte(e))
	}
	for _, g := range globs {
		if g == "" {
			continue
		}
		// Validate eagerly — filepath.Match only reports ErrBadPattern when
		// it encounters the bad character at evaluation, but probing with a
		// dummy input surfaces it now.
		if _, err := filepath.Match(g, "x"); err != nil {
			return nil, fmt.Errorf("invalid glob %q: %w", g, err)
		}
		m.globs = append(m.globs, g)
	}
	if regex != "" {
		re, err := regexp.Compile(regex)
		if err != nil {
			return nil, fmt.Errorf("invalid regex %q: %w", regex, err)
		}
		m.regex = re
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

// consider runs in the pass-2 hot path. Cheap when nil; cheap when only
// the extension predicate fires; allocates a string only when a glob /
// regex predicate is configured (decoded name is reused on heap push).
func (m *matchSet) consider(idx uint64, e *mftEntry, sz int64) {
	if m == nil {
		return
	}

	// Extension predicate — no allocation. extractAsciiExtension returns
	// the lowercased tail in a stack buffer; we compare against the
	// pre-lowercased filter list directly.
	if len(m.extensions) > 0 {
		var buf [8]byte
		n := extractAsciiExtension(e.nameBytes, buf[:])
		if n > 0 {
			for _, ext := range m.extensions {
				if bytes.Equal(buf[:n], ext) {
					m.push(idx, e, sz, "")
					return
				}
			}
		}
	}

	// Glob / regex predicates require the decoded name. With fast-name
	// decode enabled, ASCII filenames go through an in-place buffer +
	// unsafe.String view (zero alloc); non-ASCII falls back to a real
	// string. With fast off, every file allocates a real string.
	if len(m.globs) == 0 && m.regex == nil {
		return
	}
	var name string     // for predicate eval — may be unsafe view
	var realName string // copied basename for heap insertion
	if m.fast {
		n := utf16ToASCIIFast(e.nameBytes, m.nameBuf[:])
		if n >= 0 {
			// unsafe.String creates a read-only view of nameBuf; it MUST
			// NOT outlive this function call. filepath.Match and regexp's
			// MatchString consume their string argument synchronously and
			// do not retain it.
			name = unsafe.String(&m.nameBuf[0], n)
		} else {
			realName = decodeUTF16Name(e.nameBytes)
			name = realName
		}
	} else {
		realName = decodeUTF16Name(e.nameBytes)
		name = realName
	}
	matched := false
	for _, g := range m.globs {
		if ok, _ := filepath.Match(g, name); ok {
			matched = true
			break
		}
	}
	if !matched && m.regex != nil && m.regex.MatchString(name) {
		matched = true
	}
	if !matched {
		return
	}
	// On match, push needs a real (heap-allocated) basename — the unsafe
	// view of nameBuf will be clobbered by the next file. realName is set
	// either because we hit non-ASCII fallback above, or because fast was
	// off; if neither (ASCII + fast on + actual match), allocate now.
	if realName == "" {
		realName = decodeUTF16Name(e.nameBytes)
	}
	m.push(idx, e, sz, realName)
}

// utf16ToASCIIFast copies a UTF-16LE filename into out as ASCII bytes.
// Returns the number of bytes written, or -1 if the name contains any
// non-ASCII code unit (high byte != 0). No allocations.
func utf16ToASCIIFast(nameUTF16, out []byte) int {
	if len(nameUTF16)%2 != 0 {
		return -1
	}
	n := len(nameUTF16) / 2
	if n > len(out) {
		return -1
	}
	for i := 0; i < n; i++ {
		hi := nameUTF16[i*2+1]
		if hi != 0 {
			return -1
		}
		out[i] = nameUTF16[i*2]
	}
	return n
}

// push inserts a matched candidate into the heap. If the heap is at
// capacity, the smallest entry is evicted unless the new entry is also
// smaller — in which case nothing happens. The basename is decoded
// lazily here when the caller didn't already have one (extension match
// path), avoiding the decode for files we end up evicting.
func (m *matchSet) push(idx uint64, e *mftEntry, sz int64, name string) {
	if len(m.heap) >= m.cap && sz <= m.heap[0].size {
		return
	}
	if name == "" {
		name = decodeUTF16Name(e.nameBytes)
	}
	cand := fileCandidate{idx: idx, sequence: e.sequence, size: sz, basename: name}
	if len(m.heap) < m.cap {
		heap.Push(&m.heap, cand)
		return
	}
	m.heap[0] = cand
	heap.Fix(&m.heap, 0)
}

// drained returns matches sorted by size desc, then basename asc.
func (m *matchSet) drained() []fileCandidate {
	if m == nil {
		return nil
	}
	out := make([]fileCandidate, len(m.heap))
	copy(out, m.heap)
	sort.Slice(out, func(i, j int) bool {
		if out[i].size != out[j].size {
			return out[i].size > out[j].size
		}
		return out[i].basename < out[j].basename
	})
	return out
}
