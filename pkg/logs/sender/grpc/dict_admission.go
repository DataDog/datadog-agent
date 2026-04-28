// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

const (
	// defaultDictAdmissionThreshold is the number of times a longer string
	// must be seen before it is promoted into the dictionary.
	defaultDictAdmissionThreshold uint16 = 3

	// defaultDictMaxShortLen is the byte-length at or below which a string
	// is promoted immediately (heuristic path). Covers log levels (INFO,
	// ERROR), HTTP methods (GET, POST), short status words (none, success).
	defaultDictMaxShortLen = 8

	// defaultDictMaxTracked caps the number of candidate strings tracked
	// for count-based promotion to bound memory usage.
	defaultDictMaxTracked = 4096
)

// dictAdmission decides whether a JSON-context string value should be
// promoted into the stream dictionary. Two complementary strategies:
//
//  1. Heuristic – strings ≤ maxShortLen bytes are admitted immediately.
//     Short strings are almost always categorical (log levels, HTTP
//     methods, enum labels) and pay for their dictionary define on the
//     second occurrence.
//
//  2. Frequency – longer strings are counted; once a string has been
//     seen threshold times it is promoted. This catches things like
//     route names, source identifiers, and URL paths that repeat often
//     but are too long for the heuristic.
type dictAdmission struct {
	counts      map[string]uint16
	threshold   uint16
	maxShortLen int
	maxTracked  int
}

func newDictAdmission() *dictAdmission {
	return &dictAdmission{
		counts:      make(map[string]uint16),
		threshold:   defaultDictAdmissionThreshold,
		maxShortLen: defaultDictMaxShortLen,
		maxTracked:  defaultDictMaxTracked,
	}
}

// shouldAdmit returns true when s should be added to the dictionary.
func (da *dictAdmission) shouldAdmit(s string) bool {
	// Heuristic: short strings are almost always categorical.
	if len(s) <= da.maxShortLen {
		return true
	}

	// Frequency: promote after threshold occurrences.
	n := da.counts[s] + 1
	if n >= da.threshold {
		delete(da.counts, s) // no longer need to track
		return true
	}

	// Only start tracking if we have capacity.
	if len(da.counts) < da.maxTracked {
		da.counts[s] = n
	}
	return false
}

// reset clears accumulated counts. Called on stream rotation so that
// stale candidates don't persist across streams.
func (da *dictAdmission) reset() {
	// Re-use the map when it's small to reduce GC pressure.
	if len(da.counts) <= 256 {
		for k := range da.counts {
			delete(da.counts, k)
		}
	} else {
		da.counts = make(map[string]uint16)
	}
}
