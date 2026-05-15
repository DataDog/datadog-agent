// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build openmetrics_differential

package differential

import (
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

// fuzzHarness holds the long-lived Python sidecar + httptest.Server used by
// every f.Fuzz iteration. testing.F doesn't give us a Setup hook, so we lazy
// init under a sync.Once and rely on the test process exiting to clean up.
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
