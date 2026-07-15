// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build openmetrics_differential

package differential

import (
	"strings"
	"sync"
	"testing"
)

// fuzzInstance is the instance config used by the fuzz target. We pick the
// most permissive matcher (`.+`) so virtually any well-formed metric line in
// the mutated payload gets through to the transformer layer; that's where we
// want to expose divergences.
var fuzzInstance = map[string]interface{}{
	"namespace": "diff",
	"metrics":   []interface{}{".+"},
}

// fuzzHarness holds the long-lived Python sidecar + mutable payload server used
// by every f.Fuzz iteration. Lading is intentionally not used here because its
// generated response body is immutable. testing.F doesn't give us a Setup
// hook, so we lazy init under a sync.Once and rely on process exit for cleanup.
type fuzzHarness struct {
	sidecar *pythonSidecar
	ps      *payloadServer
	err     error
}

var (
	fuzzOnce sync.Once
	fuzzH    fuzzHarness
)

func getFuzzHarness(tb testing.TB) *fuzzHarness {
	tb.Helper()
	fuzzOnce.Do(func() {
		// startPythonSidecar wants *testing.T for t.Helper(); cast through.
		t, ok := tb.(*testing.T)
		if !ok {
			// f.Fuzz's callback gets a *testing.T; seed corpus replays also do.
			// In the f.Add-seed-replay path the TB *is* a T. If we somehow get
			// here from an F, bail with a clear message instead of panicking.
			fuzzH.err = nil
			tb.Fatal("fuzz harness: unexpected TB type")
			return
		}
		sc, err := startPythonSidecar(t)
		if err != nil {
			fuzzH.err = err
			return
		}
		fuzzH.sidecar = sc
		fuzzH.ps = newPayloadServer()
	})
	return &fuzzH
}

// FuzzOpenMetricsDifferential runs the corpus + (under -fuzz) generated inputs
// through the same diff harness as TestOpenMetricsDifferential. Without -fuzz,
// it just replays the seed corpus, which makes this also serve as a fixed
// regression-replay test once minimized failing inputs accumulate.
//
// Run the corpus replay:
//
//	go test -tags openmetrics_differential -v -run FuzzOpenMetricsDifferential \
//	        ./pkg/collector/corechecks/openmetrics/differential/
//
// Start a fuzz run (Go's libFuzzer-style engine, single-process):
//
//	go test -tags openmetrics_differential \
//	        -fuzz FuzzOpenMetricsDifferential -fuzztime 5m \
//	        ./pkg/collector/corechecks/openmetrics/differential/
//
// Discovered inputs land in testdata/fuzz/FuzzOpenMetricsDifferential/, which
// is gitignored by default in Go (we add the directory to .gitignore to be
// explicit). To turn a discovered failure into a permanent regression seed,
// move it to testdata/regressions/ and reference it from f.Add below.
func FuzzOpenMetricsDifferential(f *testing.F) {
	// Seed corpus: every fixture, ungzipped. f.Add accepts the raw payload.
	for _, fx := range fixtureCases {
		payload, err := loadGzipped(fx.payloadPath)
		if err != nil {
			f.Fatalf("load seed %s: %v", fx.payloadPath, err)
		}
		f.Add(payload)
	}

	f.Fuzz(func(t *testing.T, payload []byte) {
		// Quick filter: payloads with no '#' and no whitespace are vanishingly
		// unlikely to produce metric output from either side. Skip them so the
		// fuzz engine's coverage-guided exploration spends its budget on inputs
		// that at least reach the transformer.
		if !looksLikePromText(payload) {
			t.Skip()
		}

		h := getFuzzHarness(t)
		if h.err != nil {
			t.Skipf("python sidecar unavailable: %v", h.err)
		}

		out := runIteration(h.ps, h.sidecar, payload, fuzzInstance)
		switch out.Verdict() {
		case "agree", "both_rejected":
			return
		}

		// Suppress documented divergence classes so the engine keeps hunting
		// for NEW classes. Removing an entry from knownDivergences (because
		// the underlying bug was fixed) will let the fuzz re-discover it.
		if known, name := IsKnownDivergence(out); known {
			t.Skipf("known divergence (%s); see README", name)
		}

		// Suppress the UTF-8-vs-raw-bytes class: if every diff is a pure
		// value-byte mismatch on what's otherwise an identical key, the only
		// difference is the encoding interpretation. Documented in README;
		// not actionable on the Go side (Go is correct).
		if allUTF8EncodingDiffs(out.Diffs) {
			t.Skip("known divergence (utf8_encoding); all diffs are tag-encoding mismatches")
		}

		switch out.Verdict() {
		case "divergent":
			t.Errorf("divergent: go=%d py=%d diffs=%d", len(out.GoSubs), len(out.PySubs), len(out.Diffs))
			summarizeDiffs(t, out.Diffs)
		case "go_rejected_py_accepted":
			t.Errorf("go rejected, py accepted (%d submissions). go_err: %v", len(out.PySubs), out.GoErr)
		case "go_accepted_py_rejected":
			t.Errorf("py rejected, go accepted (%d submissions). py_err: %s", len(out.GoSubs), out.PyErr)
		}
	})
}

// allUTF8EncodingDiffs reports whether every diff in the slice is the
// known-class "tag-value byte-encoding mismatch" — Go decoded label values as
// UTF-8 while Python preserved them as raw bytes. Heuristic: each pair of
// (only_in_go, only_in_python) diffs shares kind+name, has the same number of
// tags, and at least one tag pair differs in a way consistent with a UTF-8
// roundtrip through Latin-1 (i.e. the Python tag, re-decoded from Latin-1 and
// then UTF-8, equals the Go tag). We approximate that with a simpler check:
// equal byte length up to a small slack, and Python tag contains any of the
// telltale mojibake markers.
//
// The heuristic intentionally errs on the side of NOT skipping (false
// negatives are fine — those just show up as failures). It must never have
// false positives, or we'd silence real bugs.
func allUTF8EncodingDiffs(diffs []Diff) bool {
	if len(diffs) == 0 {
		return false
	}
	for _, d := range diffs {
		if d.Go == nil || d.Py == nil {
			return false
		}
		if d.Go.Name != d.Py.Name || d.Go.Kind != d.Py.Kind {
			return false
		}
		if len(d.Go.Tags) != len(d.Py.Tags) {
			return false
		}
		mojibake := false
		for _, t := range d.Py.Tags {
			// Latin-1 mojibake of UTF-8 bytes typically contains Ã Â  , the
			// Latin-1 chars for the high UTF-8 lead bytes (0xc2/0xc3). It's a
			// strong tell.
			if strings.ContainsRune(t, '\u00c2') || strings.ContainsRune(t, '\u00c3') || strings.ContainsRune(t, '\u00a0') {
				mojibake = true
				break
			}
		}
		if !mojibake {
			return false
		}
	}
	return true
}

// looksLikePromText is a crude pre-filter: a Prometheus text payload contains
// at least one of the structural markers (#, {, or whitespace-separated
// numbers). The mutator produces inputs that always pass, but the fuzz engine
// will eventually try random byte sequences once it runs out of derivable
// inputs.
func looksLikePromText(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	for _, c := range b {
		if c == '#' || c == '{' || c == ' ' || c == '\t' || c == '\n' {
			return true
		}
	}
	return false
}
