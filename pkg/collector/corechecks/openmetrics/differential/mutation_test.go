// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build openmetrics_differential

package differential

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
)

var (
	mutationIterations = flag.Int("mutation.iters", 50, "number of mutated payloads to test per seed fixture")
	mutationOpsPerIter = flag.Int("mutation.ops", 3, "number of mutation ops to apply per iteration")
	mutationSeed       = flag.Int64("mutation.seed", 0, "RNG seed (0 = derive from time)")
	mutationFailFast   = flag.Bool("mutation.failfast", false, "stop at the first divergent iteration instead of running the full budget")
)

// TestOpenMetricsMutation feeds each seed fixture through the mutator and runs
// the Go-vs-Python diff on every mutated payload. Counts outcomes; on any
// divergence, dumps the offending payload to testdata/regressions/ and fails.
//
// Run with (e.g.):
//
//	go test -tags openmetrics_differential -v -run TestOpenMetricsMutation \
//	        -mutation.iters=500 -mutation.ops=4 -mutation.seed=1 \
//	        -timeout 30m \
//	        ./pkg/collector/corechecks/openmetrics/differential/
//
// The default budget (200 iters × N fixtures) finishes in a few seconds and is
// suitable for a smoke check. Crank -mutation.iters up for longer sessions.
func TestOpenMetricsMutation(t *testing.T) {
	t.Parallel()

	sidecar, err := startPythonSidecar(t)
	if err != nil {
		t.Skipf("python sidecar unavailable, skipping mutation differential: %v", err)
	}
	t.Cleanup(sidecar.Close)

	ps := newPayloadServer()
	t.Cleanup(ps.Close)

	seed := *mutationSeed
	if seed == 0 {
		seed = int64(os.Getpid()) // good enough; just need *some* variability when unspecified
	}
	t.Logf("seed=%d iters=%d ops_per_iter=%d failfast=%v", seed, *mutationIterations, *mutationOpsPerIter, *mutationFailFast)

	total := outcomeTally{}
	divergent := 0

	for _, fx := range fixtureCases {
		base, err := loadGzipped(fx.payloadPath)
		if err != nil {
			t.Fatalf("load %s: %v", fx.payloadPath, err)
		}

		m := NewMutator(seed)
		for i := 0; i < *mutationIterations; i++ {
			mutated := m.Mutate(base, *mutationOpsPerIter)
			out := runIteration(ps, sidecar, mutated, fx.instance)
			verdict := out.Verdict()
			total.inc(verdict)

			switch verdict {
			case "divergent", "go_rejected_py_accepted", "go_accepted_py_rejected":
				divergent++
				path, err := dumpRegression(t, fx.name, i, mutated, out)
				if err != nil {
					t.Errorf("dump regression: %v", err)
				}
				t.Errorf("[%s iter %d] %s  go=%d py=%d diffs=%d  saved=%s",
					fx.name, i, verdict, len(out.GoSubs), len(out.PySubs), len(out.Diffs), path)
				if verdict == "divergent" {
					summarizeDiffs(t, out.Diffs)
				}
				if *mutationFailFast {
					t.Logf("--failfast: stopping after first interesting result")
					t.Log(total.format())
					return
				}
			}
		}
	}

	t.Logf("mutation run complete: %s", total.format())
	if divergent > 0 {
		t.Logf("%d divergent iterations; see testdata/regressions/ for repros", divergent)
	}
}

type outcomeTally struct {
	counts map[string]int
}

func (t *outcomeTally) inc(verdict string) {
	if t.counts == nil {
		t.counts = map[string]int{}
	}
	t.counts[verdict]++
}

func (t outcomeTally) format() string {
	if len(t.counts) == 0 {
		return "no iterations run"
	}
	keys := make([]string, 0, len(t.counts))
	for k := range t.counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	s := ""
	for _, k := range keys {
		s += k + "=" + strconv.Itoa(t.counts[k]) + " "
	}
	return s[:len(s)-1]
}

func summarizeDiffs(t *testing.T, diffs []Diff) {
	t.Helper()
	byKind := map[string]int{}
	for _, d := range diffs {
		byKind[d.Kind]++
	}
	t.Logf("  divergence breakdown: %s", summarizeKinds(byKind))
	const sample = 10
	for i, d := range diffs {
		if i >= sample {
			t.Logf("  ... (%d more)", len(diffs)-sample)
			break
		}
		t.Logf("  %s", FormatDiff(d))
	}
}

// dumpRegression writes an offending mutated payload to testdata/regressions/
// alongside a sidecar metadata file. Filename includes a hash so duplicate
// inputs across runs collapse to a single regression seed.
func dumpRegression(t *testing.T, fixtureName string, iter int, payload []byte, out iterationOutcome) (string, error) {
	t.Helper()
	sum := sha256.Sum256(payload)
	digest := hex.EncodeToString(sum[:])[:16]
	dir := filepath.Join("testdata", "regressions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := digest + ".prom"
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return "", err
	}
	meta := "# fixture: " + fixtureName + "\n" +
		"# iter: " + strconv.Itoa(iter) + "\n" +
		"# verdict: " + out.Verdict() + "\n" +
		"# go_err: " + errString(out.GoErr) + "\n" +
		"# py_err: " + out.PyErr + "\n" +
		"# go_subs: " + strconv.Itoa(len(out.GoSubs)) + "\n" +
		"# py_subs: " + strconv.Itoa(len(out.PySubs)) + "\n" +
		"# diffs: " + strconv.Itoa(len(out.Diffs)) + "\n"
	if err := os.WriteFile(path+".meta", []byte(meta), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func errString(e error) string {
	if e == nil {
		return ""
	}
	return e.Error()
}
