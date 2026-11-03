// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package boundedreplace sizes the result of a string replacement before any
// of it is allocated.
//
// A short template that repeats a marker, combined with a long replacement,
// can describe a result many orders of magnitude larger than either input
// (xargs -I, find -exec {}, sed s///g). Checking the size limit only after
// strings.ReplaceAll or regexp.ReplaceAllString returns is too late: the
// allocation has already happened, and a Go out-of-memory error is fatal to
// the whole process rather than a recoverable panic. Builtins therefore
// charge the predicted size to a Budget first, and only build the result
// once the Budget confirms it fits.
package boundedreplace

import "strings"

// MaxCommandLineBytes caps a command line that a builtin assembles by
// substituting a value into a script-supplied template (xargs -I,
// find -exec/-execdir {}). It matches the hard ceiling xargs already applies
// to -s.
const MaxCommandLineBytes = 1 << 20 // 1 MiB

// Budget tracks the size of a result assembled from pieces against a fixed
// limit. All arithmetic is overflow-safe: once the limit is exceeded the
// Budget stays exceeded and further charges are ignored.
type Budget struct {
	remaining int
	exceeded  bool
}

// NewBudget returns a Budget that allows up to limit bytes.
func NewBudget(limit int) Budget {
	if limit < 0 {
		return Budget{exceeded: true}
	}
	return Budget{remaining: limit}
}

// Exceeded reports whether the charges so far exceed the limit.
func (b *Budget) Exceeded() bool { return b.exceeded }

// Add charges n bytes and reports whether the result still fits.
// Negative n is treated as zero.
func (b *Budget) Add(n int) bool {
	if b.exceeded || n <= 0 {
		return !b.exceeded
	}
	if n > b.remaining {
		b.exceeded = true
		return false
	}
	b.remaining -= n
	return true
}

// AddMul charges count*n bytes without overflowing and reports whether the
// result still fits. Negative operands are treated as zero.
func (b *Budget) AddMul(count, n int) bool {
	if b.exceeded || count <= 0 || n <= 0 {
		return !b.exceeded
	}
	if count > b.remaining/n {
		b.exceeded = true
		return false
	}
	return b.Add(count * n)
}

// AddReplaceAll charges the length of strings.ReplaceAll(s, old, new)
// without building it, and reports whether the result still fits.
func (b *Budget) AddReplaceAll(s, old, new string) bool {
	if b.exceeded {
		return false
	}
	count := strings.Count(s, old)
	if len(new) <= len(old) {
		// The result can only shrink, so it is at most len(s).
		return b.Add(len(s) - count*(len(old)-len(new)))
	}
	return b.Add(len(s)) && b.AddMul(count, len(new)-len(old))
}
