// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build openmetrics_differential

package differential

import (
	"bytes"
	"fmt"
	"math"
	"math/rand"
	"strings"
)

// Mutator generates structure-preserving (and sometimes structure-breaking)
// edits to Prometheus/OpenMetrics text payloads. It works line-by-line on the
// text representation — no AST — because the diff harness only cares whether
// both implementations agree on what to do with the mutated bytes, and
// line-oriented edits are sufficient to exercise the interesting parser and
// transformer paths.
//
// Mutator is deterministic: given the same seed and the same input, it
// produces the same output. That's the point: when a fuzz iteration finds a
// divergence, the seed alone is enough to reproduce it.
type Mutator struct {
	rng *rand.Rand
}

// NewMutator returns a mutator with its RNG seeded from `seed`.
func NewMutator(seed int64) *Mutator {
	return &Mutator{rng: rand.New(rand.NewSource(seed))}
}

// mutationOp is one editing primitive. Each op returns the (possibly modified)
// line slice; ops that don't apply to the current payload (e.g. "swap two
// sample lines" when there's only one) are no-ops, which the dispatcher
// detects by byte-comparing input and output.
type mutationOp func(m *Mutator, lines []string) []string

var allOps = []struct {
	name   string
	weight int
	fn     mutationOp
}{
	{"perturb_value", 6, opPerturbValue},
	{"duplicate_sample", 3, opDuplicateSample},
	{"drop_sample", 3, opDropSample},
	{"swap_samples", 2, opSwapSamples},
	{"mutate_label", 4, opMutateLabel},
	{"inject_junk_label", 2, opInjectJunkLabel},
	{"inject_reserved_label", 1, opInjectReservedLabel},
	{"drop_help", 1, opDropMetaLine("# HELP")},
	{"drop_type", 1, opDropMetaLine("# TYPE")},
	{"corrupt_type", 2, opCorruptTypeLine},
	{"non_monotonic_bucket", 2, opNonMonotonicBucket},
	{"inject_blank", 1, opInjectBlankLine},
	{"inject_comment", 1, opInjectComment},
	{"truncate_value", 1, opTruncateValue},
}

// Mutate applies up to numOps mutations to `input` and returns the resulting
// bytes. Each individual op may be a no-op (input shape didn't permit it); the
// total number of *applied* mutations may be less than numOps.
func (m *Mutator) Mutate(input []byte, numOps int) []byte {
	if numOps <= 0 {
		return append([]byte(nil), input...)
	}
	lines := splitLinesPreserveTrailing(string(input))
	for i := 0; i < numOps; i++ {
		op := m.pickOp()
		lines = op(m, lines)
	}
	return []byte(strings.Join(lines, "\n"))
}

func (m *Mutator) pickOp() mutationOp {
	totalWeight := 0
	for _, o := range allOps {
		totalWeight += o.weight
	}
	pick := m.rng.Intn(totalWeight)
	for _, o := range allOps {
		if pick < o.weight {
			return o.fn
		}
		pick -= o.weight
	}
	return allOps[0].fn // unreachable
}

// splitLinesPreserveTrailing splits on '\n' but keeps an empty final element
// when the input ends in '\n', so a round-trip via strings.Join preserves the
// trailing newline (which Prometheus parsers generally require).
func splitLinesPreserveTrailing(s string) []string {
	return strings.Split(s, "\n")
}

// ---- line classification ----------------------------------------------------

// classifyLine identifies what kind of line we're looking at. We intentionally
// don't fully parse — just enough to know if a mutation applies.
func classifyLine(line string) string {
	trim := strings.TrimSpace(line)
	switch {
	case trim == "":
		return "empty"
	case strings.HasPrefix(trim, "# HELP"):
		return "help"
	case strings.HasPrefix(trim, "# TYPE"):
		return "type"
	case strings.HasPrefix(trim, "# UNIT"):
		return "unit"
	case strings.HasPrefix(trim, "#"):
		return "comment"
	default:
		return "sample"
	}
}

// sampleIndices returns indices of all sample lines (i.e. metric points, not
// comments/HELP/TYPE).
func sampleIndices(lines []string) []int {
	var idx []int
	for i, l := range lines {
		if classifyLine(l) == "sample" {
			idx = append(idx, i)
		}
	}
	return idx
}

// parsedSample is a lightweight view of a sample line:  name{labels} value [ts]
type parsedSample struct {
	name   string
	labels string // raw `{a="b",c="d"}` including braces, or ""
	value  string // numeric token as it appears
	ts     string // optional timestamp token, or ""
}

func parseSample(line string) (parsedSample, bool) {
	p := parsedSample{}
	lbStart := strings.IndexByte(line, '{')
	var rest string
	if lbStart < 0 {
		// no labels
		parts := strings.Fields(line)
		if len(parts) < 2 {
			return p, false
		}
		p.name = parts[0]
		p.value = parts[1]
		if len(parts) > 2 {
			p.ts = parts[2]
		}
		return p, true
	}
	p.name = strings.TrimSpace(line[:lbStart])
	lbEnd := findLabelEnd(line, lbStart)
	if lbEnd < 0 {
		return p, false
	}
	p.labels = line[lbStart : lbEnd+1]
	rest = strings.TrimSpace(line[lbEnd+1:])
	parts := strings.Fields(rest)
	if len(parts) < 1 {
		return p, false
	}
	p.value = parts[0]
	if len(parts) > 1 {
		p.ts = parts[1]
	}
	return p, true
}

// findLabelEnd returns the byte index of the '}' that closes the label set
// opened at `start`. Naively respects backslash-escapes inside quoted values.
func findLabelEnd(line string, start int) int {
	inString := false
	escape := false
	for i := start + 1; i < len(line); i++ {
		c := line[i]
		if escape {
			escape = false
			continue
		}
		switch c {
		case '\\':
			escape = true
		case '"':
			inString = !inString
		case '}':
			if !inString {
				return i
			}
		}
	}
	return -1
}

func renderSample(p parsedSample) string {
	var b strings.Builder
	b.WriteString(p.name)
	b.WriteString(p.labels)
	b.WriteByte(' ')
	b.WriteString(p.value)
	if p.ts != "" {
		b.WriteByte(' ')
		b.WriteString(p.ts)
	}
	return b.String()
}

// ---- ops --------------------------------------------------------------------

var adversarialValues = []string{
	"NaN",
	"+Inf",
	"-Inf",
	"0",
	"-0",
	"1e308",
	"-1e308",
	"5e-324", // smallest positive subnormal double
	"9007199254740993", // 2^53 + 1, not exactly representable
	"1.7976931348623157e+308",
}

func opPerturbValue(m *Mutator, lines []string) []string {
	idx := sampleIndices(lines)
	if len(idx) == 0 {
		return lines
	}
	i := idx[m.rng.Intn(len(idx))]
	p, ok := parseSample(lines[i])
	if !ok {
		return lines
	}
	switch m.rng.Intn(3) {
	case 0:
		p.value = adversarialValues[m.rng.Intn(len(adversarialValues))]
	case 1:
		// negate
		if strings.HasPrefix(p.value, "-") {
			p.value = p.value[1:]
		} else {
			p.value = "-" + p.value
		}
	case 2:
		// add small noise; if parse fails, fall back to an adversarial value
		var f float64
		if _, err := fmt.Sscanf(p.value, "%g", &f); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
			p.value = fmt.Sprintf("%g", f*1.0001)
		} else {
			p.value = adversarialValues[m.rng.Intn(len(adversarialValues))]
		}
	}
	lines[i] = renderSample(p)
	return lines
}

func opDuplicateSample(m *Mutator, lines []string) []string {
	idx := sampleIndices(lines)
	if len(idx) == 0 {
		return lines
	}
	i := idx[m.rng.Intn(len(idx))]
	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:i+1]...)
	out = append(out, lines[i]) // duplicate the same line
	out = append(out, lines[i+1:]...)
	return out
}

func opDropSample(m *Mutator, lines []string) []string {
	idx := sampleIndices(lines)
	if len(idx) == 0 {
		return lines
	}
	i := idx[m.rng.Intn(len(idx))]
	out := make([]string, 0, len(lines)-1)
	out = append(out, lines[:i]...)
	out = append(out, lines[i+1:]...)
	return out
}

func opSwapSamples(m *Mutator, lines []string) []string {
	idx := sampleIndices(lines)
	if len(idx) < 2 {
		return lines
	}
	a := idx[m.rng.Intn(len(idx))]
	b := idx[m.rng.Intn(len(idx))]
	if a == b {
		return lines
	}
	lines[a], lines[b] = lines[b], lines[a]
	return lines
}

var adversarialLabelValues = []string{
	"",
	" ",
	"\"",
	"\\",
	"\\n",
	"\\\"",
	"a\\\"b",
	strings.Repeat("x", 1024),
	"éáíóú",
	"你好",
	"\xe2\x80\x8e", // LRM
	"a\u0000b",
}

func opMutateLabel(m *Mutator, lines []string) []string {
	idx := sampleIndices(lines)
	if len(idx) == 0 {
		return lines
	}
	i := idx[m.rng.Intn(len(idx))]
	p, ok := parseSample(lines[i])
	if !ok || p.labels == "" {
		return lines
	}
	// Pick the Nth label inside p.labels and replace its value.
	inner := p.labels[1 : len(p.labels)-1] // strip { }
	pairs := splitLabelPairs(inner)
	if len(pairs) == 0 {
		return lines
	}
	j := m.rng.Intn(len(pairs))
	eq := strings.IndexByte(pairs[j], '=')
	if eq < 0 {
		return lines
	}
	newVal := adversarialLabelValues[m.rng.Intn(len(adversarialLabelValues))]
	pairs[j] = pairs[j][:eq+1] + "\"" + newVal + "\""
	p.labels = "{" + strings.Join(pairs, ",") + "}"
	lines[i] = renderSample(p)
	return lines
}

func opInjectJunkLabel(m *Mutator, lines []string) []string {
	idx := sampleIndices(lines)
	if len(idx) == 0 {
		return lines
	}
	i := idx[m.rng.Intn(len(idx))]
	p, ok := parseSample(lines[i])
	if !ok {
		return lines
	}
	// Use a non-reserved name prefix (no leading underscores). The reserved
	// `__foo` namespace is exercised by opInjectReservedLabel separately so
	// that finding doesn't dominate every run.
	injected := fmt.Sprintf(`fuzz_%d="%d"`, m.rng.Intn(1000), m.rng.Intn(1000))
	if p.labels == "" {
		p.labels = "{" + injected + "}"
	} else {
		inner := p.labels[1 : len(p.labels)-1]
		if inner == "" {
			p.labels = "{" + injected + "}"
		} else {
			p.labels = "{" + inner + "," + injected + "}"
		}
	}
	lines[i] = renderSample(p)
	return lines
}

// opInjectReservedLabel injects a label whose name starts with `__`, which the
// OpenMetrics spec reserves for internal use. Python's prometheus_client
// rejects such labels strictly; Go currently accepts them. Low-weight op so
// this divergence class doesn't dominate every fuzz run — just gets
// occasional coverage.
func opInjectReservedLabel(m *Mutator, lines []string) []string {
	idx := sampleIndices(lines)
	if len(idx) == 0 {
		return lines
	}
	i := idx[m.rng.Intn(len(idx))]
	p, ok := parseSample(lines[i])
	if !ok {
		return lines
	}
	injected := fmt.Sprintf(`__reserved_%d="%d"`, m.rng.Intn(1000), m.rng.Intn(1000))
	if p.labels == "" {
		p.labels = "{" + injected + "}"
	} else {
		inner := p.labels[1 : len(p.labels)-1]
		if inner == "" {
			p.labels = "{" + injected + "}"
		} else {
			p.labels = "{" + inner + "," + injected + "}"
		}
	}
	lines[i] = renderSample(p)
	return lines
}

func opDropMetaLine(prefix string) mutationOp {
	return func(m *Mutator, lines []string) []string {
		var matches []int
		for i, l := range lines {
			if strings.HasPrefix(strings.TrimSpace(l), prefix) {
				matches = append(matches, i)
			}
		}
		if len(matches) == 0 {
			return lines
		}
		i := matches[m.rng.Intn(len(matches))]
		out := make([]string, 0, len(lines)-1)
		out = append(out, lines[:i]...)
		out = append(out, lines[i+1:]...)
		return out
	}
}

var typeKeywords = []string{"counter", "gauge", "histogram", "summary", "untyped", "info", "stateset", "gaugehistogram"}

func opCorruptTypeLine(m *Mutator, lines []string) []string {
	var matches []int
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "# TYPE") {
			matches = append(matches, i)
		}
	}
	if len(matches) == 0 {
		return lines
	}
	i := matches[m.rng.Intn(len(matches))]
	fields := strings.Fields(lines[i])
	if len(fields) < 4 {
		return lines
	}
	// Replace the keyword (last field) with a different one.
	newKW := typeKeywords[m.rng.Intn(len(typeKeywords))]
	for j := 0; j < 4 && newKW == fields[3]; j++ {
		newKW = typeKeywords[m.rng.Intn(len(typeKeywords))]
	}
	fields[3] = newKW
	lines[i] = strings.Join(fields, " ")
	return lines
}

// opNonMonotonicBucket finds histogram buckets and shuffles their `le` values
// so they're no longer monotonic. Catches parsers that rely on bucket order.
func opNonMonotonicBucket(m *Mutator, lines []string) []string {
	// Find a run of consecutive sample lines whose names end in "_bucket"
	// and share the same base name.
	type run struct{ start, end int }
	var runs []run
	var cur run
	cur.start, cur.end = -1, -1
	var lastBase string
	flush := func() {
		if cur.start >= 0 && cur.end-cur.start >= 1 { // at least 2 buckets
			runs = append(runs, cur)
		}
		cur = run{-1, -1}
		lastBase = ""
	}
	for i, l := range lines {
		if classifyLine(l) != "sample" {
			flush()
			continue
		}
		p, ok := parseSample(l)
		if !ok || !strings.HasSuffix(p.name, "_bucket") {
			flush()
			continue
		}
		base := strings.TrimSuffix(p.name, "_bucket")
		if cur.start < 0 {
			cur.start = i
			cur.end = i
			lastBase = base
			continue
		}
		if base != lastBase {
			flush()
			cur.start = i
			cur.end = i
			lastBase = base
			continue
		}
		cur.end = i
	}
	flush()
	if len(runs) == 0 {
		return lines
	}
	r := runs[m.rng.Intn(len(runs))]
	// Reverse the run — cheap way to break monotonicity.
	i, j := r.start, r.end
	for i < j {
		lines[i], lines[j] = lines[j], lines[i]
		i++
		j--
	}
	return lines
}

func opInjectBlankLine(m *Mutator, lines []string) []string {
	if len(lines) == 0 {
		return lines
	}
	i := m.rng.Intn(len(lines))
	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:i]...)
	out = append(out, "")
	out = append(out, lines[i:]...)
	return out
}

func opInjectComment(m *Mutator, lines []string) []string {
	if len(lines) == 0 {
		return lines
	}
	i := m.rng.Intn(len(lines))
	comment := fmt.Sprintf("# fuzz-comment %d", m.rng.Int())
	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:i]...)
	out = append(out, comment)
	out = append(out, lines[i:]...)
	return out
}

// opTruncateValue cuts a sample line off mid-value. Almost always rejected by
// both parsers — the interesting case is when one is more permissive.
func opTruncateValue(m *Mutator, lines []string) []string {
	idx := sampleIndices(lines)
	if len(idx) == 0 {
		return lines
	}
	i := idx[m.rng.Intn(len(idx))]
	line := lines[i]
	if len(line) < 2 {
		return lines
	}
	cut := m.rng.Intn(len(line)-1) + 1
	lines[i] = line[:cut]
	return lines
}

// splitLabelPairs splits a label-set inner string (without the surrounding
// braces) on top-level commas, respecting quoted values.
func splitLabelPairs(s string) []string {
	var out []string
	var cur bytes.Buffer
	inString := false
	escape := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if escape {
			cur.WriteByte(c)
			escape = false
			continue
		}
		switch c {
		case '\\':
			escape = true
			cur.WriteByte(c)
		case '"':
			inString = !inString
			cur.WriteByte(c)
		case ',':
			if inString {
				cur.WriteByte(c)
			} else {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteByte(c)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}
