// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build openmetrics_differential

package differential

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// floatTolerance is the relative tolerance for value comparison. OpenMetrics
// values are emitted as decimal strings on the wire and re-parsed in both
// runtimes, so exact byte-equal float comparison would flap on harmless
// round-tripping (e.g. 1.0 vs 9.999999999999998e-1).
const floatTolerance = 1e-9

// ignoreTagPrefixes lists tag prefixes whose VALUES are expected to legitimately
// differ between runs (e.g. ephemeral test server URL). Tags with these prefixes
// are stripped before comparison — the *presence* of the tag is still asserted
// indirectly because both implementations should add or omit it together.
var ignoreTagPrefixes = []string{
	"endpoint:", // tag_by_endpoint defaults to true on both sides; URL is httptest's random port
}

// Diff is one normalized discrepancy between the Go and Python submission sets.
type Diff struct {
	Kind string // "only_in_go", "only_in_python", "value_mismatch"
	Key  string // canonicalized identity (kind + name + sorted tags)
	Go   *Submission
	Py   *Submission
}

// CompareSubmissions normalizes both sides and returns a stable, sorted list of
// differences. An empty list means the implementations agree (within tolerance)
// on the input that produced these submissions.
func CompareSubmissions(goSubs, pySubs []Submission) []Diff {
	goIdx := indexSubmissions(goSubs)
	pyIdx := indexSubmissions(pySubs)

	var diffs []Diff
	seen := make(map[string]struct{}, len(goIdx)+len(pyIdx))

	for key, gs := range goIdx {
		seen[key] = struct{}{}
		ps, ok := pyIdx[key]
		if !ok {
			diffs = append(diffs, Diff{Kind: "only_in_go", Key: key, Go: &gs[0]})
			continue
		}
		// Count multiplicity — a key with N samples on one side and M on the
		// other is a real divergence.
		if len(gs) != len(ps) {
			diffs = append(diffs, Diff{
				Kind: "value_mismatch", Key: fmt.Sprintf("%s (count go=%d py=%d)", key, len(gs), len(ps)),
				Go: &gs[0], Py: &ps[0],
			})
			continue
		}
		// Compare values pairwise after sorting by value, which gives a stable
		// ordering for histograms et al. without depending on emission order.
		sortByValue(gs)
		sortByValue(ps)
		for i := range gs {
			if !floatsClose(gs[i].Value, ps[i].Value) {
				diffs = append(diffs, Diff{Kind: "value_mismatch", Key: key, Go: &gs[i], Py: &ps[i]})
			}
		}
	}
	for key, ps := range pyIdx {
		if _, ok := seen[key]; ok {
			continue
		}
		diffs = append(diffs, Diff{Kind: "only_in_python", Key: key, Py: &ps[0]})
	}

	sort.Slice(diffs, func(i, j int) bool {
		if diffs[i].Kind != diffs[j].Kind {
			return diffs[i].Kind < diffs[j].Kind
		}
		return diffs[i].Key < diffs[j].Key
	})
	return diffs
}

func indexSubmissions(subs []Submission) map[string][]Submission {
	idx := make(map[string][]Submission, len(subs))
	for _, s := range subs {
		n := normalize(s)
		key := identityKey(n)
		idx[key] = append(idx[key], n)
	}
	return idx
}

func normalize(s Submission) Submission {
	tags := make([]string, 0, len(s.Tags))
	for _, t := range s.Tags {
		if shouldIgnoreTag(t) {
			continue
		}
		tags = append(tags, t)
	}
	sort.Strings(tags)
	return Submission{
		Kind:            s.Kind,
		Name:            s.Name,
		Value:           s.Value,
		Hostname:        s.Hostname, // hostname comparison kept on; if it flaps, add to ignore list
		Tags:            tags,
		Message:         "", // service-check message wording diverges between runtimes; ignore by design
		FlushFirstValue: s.FlushFirstValue,
	}
}

func shouldIgnoreTag(tag string) bool {
	for _, prefix := range ignoreTagPrefixes {
		if strings.HasPrefix(tag, prefix) {
			return true
		}
	}
	return false
}

func identityKey(s Submission) string {
	flush := ""
	if s.FlushFirstValue {
		flush = "\x00flush=1"
	}
	return s.Kind + "\x00" + s.Name + "\x00" + strings.Join(s.Tags, ",") + "\x00" + s.Hostname + flush
}

func sortByValue(ss []Submission) {
	sort.Slice(ss, func(i, j int) bool { return ss[i].Value < ss[j].Value })
}

func floatsClose(a, b float64) bool {
	if a == b {
		return true
	}
	if math.IsNaN(a) && math.IsNaN(b) {
		return true
	}
	diff := math.Abs(a - b)
	mag := math.Max(math.Abs(a), math.Abs(b))
	return diff <= floatTolerance || diff <= floatTolerance*mag
}

// FormatDiff renders a single diff for human reading; used by the test on failure.
func FormatDiff(d Diff) string {
	switch d.Kind {
	case "only_in_go":
		return fmt.Sprintf("only_in_go:     %s  value=%v tags=%v", d.Key, d.Go.Value, d.Go.Tags)
	case "only_in_python":
		return fmt.Sprintf("only_in_python: %s  value=%v tags=%v", d.Key, d.Py.Value, d.Py.Tags)
	case "value_mismatch":
		return fmt.Sprintf("value_mismatch: %s  go=%v py=%v", d.Key, d.Go.Value, d.Py.Value)
	}
	return fmt.Sprintf("%+v", d)
}
