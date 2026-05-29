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
	// fast, when true, decodes ASCII filenames in-place into nameBuf and
	// passes a non-allocating string view (unsafe.String) to filepath.Match
	// and regexp.MatchString. Non-ASCII filenames fall back to the heap
	// allocator. The string MUST NOT escape predicate evaluation — nameBuf
	// is reused on the next file.
	fast    bool
	nameBuf [512]byte
}

// newMatchSet builds a matcher from a list of FindQuery. Returns (nil, nil)
// when queries is empty. Returns an error if any query is malformed (empty
// value, unknown type, bad glob, bad regex).
func newMatchSet(queries []FindQuery, fast bool) (*matchSet, error) {
	if len(queries) == 0 {
		return nil, nil
	}
	m := &matchSet{fast: fast}
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

	// Lazy extension extraction (24-byte buffer covers all real-world
	// extensions, e.g. ".crdownload", ".application", ".compositions").
	var extBuf [24]byte
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

	// Lazy name decode. name is the value handed to predicate evaluators
	// (may be an unsafe view of nameBuf in fast+ASCII path). realName, if
	// set, is a heap-allocated copy safe to retain past this call.
	var name string
	var realName string
	nameEvaluated := false
	getName := func() string {
		if nameEvaluated {
			return name
		}
		nameEvaluated = true
		if m.fast {
			n := utf16ToASCIIFast(e.nameBytes, m.nameBuf[:])
			if n >= 0 {
				name = unsafe.String(&m.nameBuf[0], n)
				return name
			}
		}
		realName = decodeUTF16Name(e.nameBytes)
		name = realName
		return name
	}

	for _, s := range m.slots {
		matched := false
		switch {
		case len(s.exts) > 0:
			ext, ok := getExt()
			if !ok {
				continue
			}
			for _, want := range s.exts {
				if bytes.Equal(ext, want) {
					matched = true
					break
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
		// push needs a heap-allocated basename. realName is empty in the
		// fast+ASCII match path until we promote it here, and in the pure-
		// ext match path until push lazily decodes (push avoids the decode
		// if the heap rejects the candidate).
		pushName := realName
		if pushName == "" && nameEvaluated {
			realName = decodeUTF16Name(e.nameBytes)
			pushName = realName
		}
		s.push(idx, e, sz, pushName)
	}
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
