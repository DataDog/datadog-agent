// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"fmt"
	"strconv"
	"strings"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

// braceSequenceLimit mirrors the element cap of mvdan.cc/sh/v3's BracesSeq.
const braceSequenceLimit = 16 << 10

const maxInt64 = 1<<63 - 1

// fieldsSeq is expand.FieldsSeq, except that a numeric brace sequence whose
// next step past its last element would leave the int64 range ends at that
// element, as in bash. mvdan.cc/sh/v3 lets the step wrap around, so
// {9223372036854775807..9223372036854775807} would otherwise iterate over
// wrapped values until the element cap fails the word.
func fieldsSeq(cfg *expand.Config, word *syntax.Word) func(yield func(string, error) bool) {
	return func(yield func(string, error) bool) {
		split := *word // SplitBraces replaces the Parts slice, so work on a copy
		if !syntax.SplitBraces(&split) || !clampBraceSequences(split.Parts) {
			for field, err := range expand.FieldsSeq(cfg, word) {
				if !yield(field, err) {
					return
				}
			}
			return
		}
		for w, err := range expand.BracesSeq(cfg, &split) {
			if err != nil {
				yield("", err)
				return
			}
			for field, err := range expand.FieldsSeq(cfg, escapeBraceLits(w)) {
				if !yield(field, err) || err != nil {
					return
				}
			}
		}
	}
}

// clampBraceSequences replaces, in place, every numeric sequence in parts
// that int64 overflow would derail with the equivalent list expansion, and
// reports whether it replaced any.
func clampBraceSequences(parts []syntax.WordPart) bool {
	changed := false
	for i, part := range parts {
		br, ok := part.(*syntax.BraceExp)
		if !ok {
			continue
		}
		if !br.Sequence {
			for _, elem := range br.Elems {
				changed = clampBraceSequences(elem.Parts) || changed
			}
			continue
		}
		if list := clampedSequence(br); list != nil {
			parts[i] = list
			changed = true
		}
	}
	return changed
}

// clampedSequence returns the list expansion equivalent to the numeric
// sequence br if mvdan.cc/sh/v3 would step past its last element with int64
// overflow, or nil if mvdan expands it correctly.
func clampedSequence(br *syntax.BraceExp) *syntax.BraceExp {
	fromLit, toLit := br.Elems[0].Lit(), br.Elems[1].Lit()
	from, err1 := strconv.ParseInt(fromLit, 10, 64)
	to, err2 := strconv.ParseInt(toLit, 10, 64)
	if err1 != nil || err2 != nil {
		return nil // a character sequence stays within one byte
	}
	step := uint64(1)
	if len(br.Elems) > 2 {
		// SplitBraces only keeps sequences whose step parses.
		if n, _ := strconv.ParseInt(br.Elems[2].Lit(), 10, 64); n < 0 {
			step = -uint64(n)
		} else if n > 0 {
			step = uint64(n)
		}
	}
	// Unsigned arithmetic is exact here: every quantity fits in uint64, and
	// conversions back to int64 only happen for values within [from, to].
	upward := from <= to
	span := uint64(to) - uint64(from)
	if !upward {
		span = uint64(from) - uint64(to)
	}
	steps := span / step
	if steps >= braceSequenceLimit {
		return nil // BracesSeq's cap fails the word before any overflow
	}
	var last, headroom uint64
	if upward {
		last = uint64(from) + steps*step
		headroom = maxInt64 - last
	} else {
		last = uint64(from) - steps*step
		headroom = last + maxInt64 + 1
	}
	if step <= headroom {
		return nil
	}
	width := 0
	if hasLeadingZeros(fromLit) || hasLeadingZeros(toLit) {
		width = max(len(fromLit), len(toLit))
	}
	elems := make([]*syntax.Word, 0, steps+1)
	n := uint64(from)
	for range steps + 1 {
		lit := &syntax.Lit{Value: fmt.Sprintf("%0*d", width, int64(n))}
		elems = append(elems, &syntax.Word{Parts: []syntax.WordPart{lit}})
		if upward {
			n += step
		} else {
			n -= step
		}
	}
	return &syntax.BraceExp{Elems: elems}
}

// hasLeadingZeros mirrors mvdan.cc/sh/v3's zero-padding rule for sequence
// endpoints.
func hasLeadingZeros(s string) bool {
	if s != "" && s[0] == '-' {
		s = s[1:]
	}
	return len(s) > 1 && s[0] == '0'
}

// escapeBraceLits returns a brace-expanded word whose unescaped "{" in
// literal parts are backslash-escaped, so expand.FieldsSeq cannot brace
// expand it a second time.
func escapeBraceLits(word *syntax.Word) *syntax.Word {
	escaped := &syntax.Word{Parts: make([]syntax.WordPart, len(word.Parts))}
	for i, part := range word.Parts {
		escaped.Parts[i] = part
		lit, ok := part.(*syntax.Lit)
		if !ok || !strings.Contains(lit.Value, "{") {
			continue
		}
		var sb strings.Builder
		for j := 0; j < len(lit.Value); j++ {
			switch lit.Value[j] {
			case '\\':
				sb.WriteByte('\\')
				if j+1 < len(lit.Value) {
					j++
					sb.WriteByte(lit.Value[j])
				}
				continue
			case '{':
				sb.WriteByte('\\')
			}
			sb.WriteByte(lit.Value[j])
		}
		copied := *lit // shared Lits such as SplitBraces' "{" must not change
		copied.Value = sb.String()
		escaped.Parts[i] = &copied
	}
	return escaped
}
