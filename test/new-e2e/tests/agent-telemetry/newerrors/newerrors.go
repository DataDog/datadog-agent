// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package newerrors finds errors that appear in a candidate release cohort and
// are absent from a baseline cohort, using stack fingerprints of errortracking
// events. It is pure and depends only on the standard library so end to end
// tests and release tooling can both import it.
package newerrors

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"sort"
	"strings"
)

// Event is one errortracking record reduced to what comparison needs.
type Event struct {
	// Host identifies the emitting host, used to normalize rates per host.
	Host string
	// StackTrace is the stack_trace wire field: "function\n\tfile:line +0xoff" frames.
	StackTrace string
	// Count is the number of occurrences the record stands for; values below 1 count as 1.
	Count int
}

// Cohort is the set of events collected from the agents of one release.
type Cohort struct {
	// Hosts is the number of hosts in the cohort, including hosts that emitted
	// nothing. When zero it is the number of distinct hosts seen in Events.
	Hosts  int
	Events []Event
}

// NewError is a fingerprint seen in the candidate cohort and not in the baseline.
type NewError struct {
	Fingerprint string
	// Stack is the normalized stack the fingerprint was computed from.
	Stack string
	// Occurrences is the summed Count of the candidate events with this fingerprint.
	Occurrences int
	// RatePerHost is Occurrences divided by the candidate cohort host count.
	RatePerHost float64
}

var (
	// callArgsRe matches the argument list some runtimes append to a function name.
	callArgsRe = regexp.MustCompile(`\(((0x[0-9a-fA-F]+|\.\.\.)[, ]*)*\)$`)
	// lineRe matches the trailing ":line" of a file reference.
	lineRe = regexp.MustCompile(`:\d+$`)
)

// Normalize reduces a stack trace to its release-independent shape: one
// "function file" line per frame, where file is the base name. Offsets,
// addresses and call arguments are dropped so builds compare equal, and line
// numbers are dropped because they move between releases. Two call sites in
// one function and file therefore share a fingerprint. Lines that are not
// frames are ignored; a stack with no frames normalizes to "".
func Normalize(stack string) string {
	var frames []string
	var fn string
	for _, line := range strings.Split(stack, "\n") {
		if strings.HasPrefix(line, "\t") {
			if fn == "" {
				continue
			}
			file := strings.TrimSpace(line)
			if i := strings.Index(file, " +0x"); i >= 0 {
				file = file[:i]
			}
			file = lineRe.ReplaceAllString(file, "")
			file = file[strings.LastIndexAny(file, "/\\")+1:]
			frames = append(frames, fn+" "+file)
			fn = ""
			continue
		}
		fn = strings.TrimSpace(callArgsRe.ReplaceAllString(strings.TrimSpace(line), ""))
	}
	return strings.Join(frames, "\n")
}

// Fingerprint returns a stable short hash of the normalized stack, or "" when
// the stack has no frames.
func Fingerprint(stack string) string {
	n := Normalize(stack)
	if n == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(n))
	return hex.EncodeToString(sum[:8])
}

// FindNew returns the fingerprints present in candidate and absent from
// baseline, ordered by descending RatePerHost then fingerprint. Events without
// a usable stack are ignored. Duplicates collapse into one entry with summed
// occurrences.
func FindNew(baseline, candidate Cohort) []NewError {
	seen := make(map[string]struct{})
	for _, e := range baseline.Events {
		if fp := Fingerprint(e.StackTrace); fp != "" {
			seen[fp] = struct{}{}
		}
	}

	byFP := make(map[string]*NewError)
	for _, e := range candidate.Events {
		fp := Fingerprint(e.StackTrace)
		if fp == "" {
			continue
		}
		if _, shared := seen[fp]; shared {
			continue
		}
		n, ok := byFP[fp]
		if !ok {
			n = &NewError{Fingerprint: fp, Stack: Normalize(e.StackTrace)}
			byFP[fp] = n
		}
		n.Occurrences += max(e.Count, 1)
	}

	hosts := float64(cohortHosts(candidate))
	out := make([]NewError, 0, len(byFP))
	for _, n := range byFP {
		n.RatePerHost = float64(n.Occurrences) / hosts
		out = append(out, *n)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].RatePerHost != out[j].RatePerHost {
			return out[i].RatePerHost > out[j].RatePerHost
		}
		return out[i].Fingerprint < out[j].Fingerprint
	})
	return out
}

// cohortHosts returns the host count used for normalization, at least 1.
func cohortHosts(c Cohort) int {
	if c.Hosts > 0 {
		return c.Hosts
	}
	distinct := make(map[string]struct{})
	for _, e := range c.Events {
		distinct[e.Host] = struct{}{}
	}
	return max(len(distinct), 1)
}
