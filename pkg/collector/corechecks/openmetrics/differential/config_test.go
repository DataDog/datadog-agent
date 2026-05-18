// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build openmetrics_differential

package differential

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

var (
	configIters     = flag.Int("config.iters", 50, "number of (config x payload) iterations per seed fixture")
	configKnobs     = flag.Int("config.knobs", 2, "number of config knobs to apply per generated config (0 = baseline only)")
	configSeed      = flag.Int64("config.seed", 0, "config-mutator RNG seed (0 = derive from PID)")
	configFailFast  = flag.Bool("config.failfast", false, "stop at the first divergent iteration")
	configMutPayload = flag.Bool("config.mutate.payload", false, "also apply payload mutations on top of config variation")
)

// TestOpenMetricsConfigDifferential runs the diff harness over a
// Cartesian-ish product of (config knob combos) x (corpus payloads). Unlike
// the payload-mutation test, this targets the transformer / matcher / label
// pipeline rather than the parser. Bugs found here are usually subtle
// per-submission divergences (wrong tags, wrong metric name, dropped
// submission) rather than total scrape failure.
//
// Run with:
//
//	go test -tags openmetrics_differential -v -run TestOpenMetricsConfigDifferential \
//	        ./pkg/collector/corechecks/openmetrics/differential/ \
//	        -config.iters=50 -config.knobs=2 -config.seed=1
//
// Each iteration logs the applied knob names so divergences attribute
// directly to the responsible config change.
func TestOpenMetricsConfigDifferential(t *testing.T) {
	t.Parallel()

	sidecar, err := startPythonSidecar(t)
	if err != nil {
		t.Skipf("python sidecar unavailable, skipping config differential: %v", err)
	}
	t.Cleanup(sidecar.Close)

	ps := newPayloadServer()
	t.Cleanup(ps.Close)

	seed := *configSeed
	if seed == 0 {
		seed = int64(os.Getpid())
	}
	t.Logf("seed=%d iters=%d knobs_per_config=%d failfast=%v mutate_payload=%v",
		seed, *configIters, *configKnobs, *configFailFast, *configMutPayload)

	total := outcomeTally{}
	knobTally := map[string]map[string]int{} // verdict -> knob -> count
	var interestingCount int

	for _, fx := range fixtureCases {
		payload, err := loadGzipped(fx.payloadPath)
		if err != nil {
			t.Fatalf("load %s: %v", fx.payloadPath, err)
		}

		cm := NewConfigMutator(seed)
		var pm *Mutator
		if *configMutPayload {
			pm = NewMutator(seed ^ 0xDEADBEEF) // independent RNG stream
		}

		for i := 0; i < *configIters; i++ {
			gc := cm.NewConfig(*configKnobs)

			currentPayload := payload
			if pm != nil {
				currentPayload = pm.Mutate(payload, 2)
			}

			out := runIteration(ps, sidecar, currentPayload, gc.Config)
			verdict := out.Verdict()
			total.inc(verdict)

			// Attribute non-agreement outcomes to each applied knob.
			if verdict != "agree" {
				for _, knob := range gc.AppliedKnobs {
					if knobTally[verdict] == nil {
						knobTally[verdict] = map[string]int{}
					}
					knobTally[verdict][knob]++
				}
			}

			switch verdict {
			case "agree", "both_rejected":
				continue
			}

			// Suppress known divergences so signal stays clean. Same list the
			// fuzz target uses.
			if known, name := IsKnownDivergence(out); known {
				total.inc("known/" + name)
				continue
			}

			interestingCount++
			path, _ := dumpConfigRegression(fx.name, i, gc, currentPayload, out)
			t.Errorf("[%s iter %d] %s  knobs=%v  go=%d py=%d diffs=%d  saved=%s",
				fx.name, i, verdict, gc.AppliedKnobs,
				len(out.GoSubs), len(out.PySubs), len(out.Diffs), path)
			if verdict == "divergent" {
				// Only show the breakdown by kind; full per-diff dump is too
				// noisy for log scanning. Repro file has full payload + config.
				byKind := map[string]int{}
				for _, d := range out.Diffs {
					byKind[d.Kind]++
				}
				t.Logf("  divergence breakdown: %s", summarizeKinds(byKind))
			}
			if out.GoErr != nil {
				t.Logf("  go err: %v", out.GoErr)
			}
			if out.PyErr != "" {
				t.Logf("  py err first-line: %s", firstLine(out.PyErr))
			}

			if *configFailFast {
				t.Logf("--failfast: stopping after first interesting result")
				return
			}
		}
	}

	t.Logf("config differential complete: %s", total.format())
	if interestingCount > 0 {
		t.Logf("%d non-suppressed interesting iterations; repros in testdata/regressions/", interestingCount)
	}

	// Knob attribution — which knobs correlate with which verdicts.
	if len(knobTally) > 0 {
		t.Logf("knob attribution (knob -> verdict -> count):")
		knobVerdicts := map[string]map[string]int{}
		for v, knobs := range knobTally {
			for k, c := range knobs {
				if knobVerdicts[k] == nil {
					knobVerdicts[k] = map[string]int{}
				}
				knobVerdicts[k][v] += c
			}
		}
		knobs := make([]string, 0, len(knobVerdicts))
		for k := range knobVerdicts {
			knobs = append(knobs, k)
		}
		sort.Strings(knobs)
		for _, k := range knobs {
			vs := knobVerdicts[k]
			parts := make([]string, 0, len(vs))
			verdicts := make([]string, 0, len(vs))
			for v := range vs {
				verdicts = append(verdicts, v)
			}
			sort.Strings(verdicts)
			for _, v := range verdicts {
				parts = append(parts, fmt.Sprintf("%s=%d", v, vs[v]))
			}
			t.Logf("  %-50s %s", k, strings.Join(parts, " "))
		}
	}
}

// dumpConfigRegression writes a divergent (config, payload) pair to disk.
// Filename is keyed by config-and-payload sha so identical configs across
// runs collapse.
func dumpConfigRegression(fixtureName string, iter int, gc GeneratedConfig, payload []byte, out iterationOutcome) (string, error) {
	dir := filepath.Join("testdata", "regressions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	configJSON, _ := json.Marshal(gc.Config)
	sum := sha256.Sum256(append(configJSON, payload...))
	digest := hex.EncodeToString(sum[:])[:16]
	name := "config_" + digest

	if err := os.WriteFile(filepath.Join(dir, name+".prom"), payload, 0o644); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, name+".instance.json"), configJSON, 0o644); err != nil {
		return "", err
	}
	meta := "# fixture: " + fixtureName + "\n" +
		"# iter: " + strconv.Itoa(iter) + "\n" +
		"# verdict: " + out.Verdict() + "\n" +
		"# applied_knobs: " + strings.Join(gc.AppliedKnobs, " ") + "\n" +
		"# go_err: " + errString(out.GoErr) + "\n" +
		"# py_err_first_line: " + firstLine(out.PyErr) + "\n" +
		"# go_subs: " + strconv.Itoa(len(out.GoSubs)) + "\n" +
		"# py_subs: " + strconv.Itoa(len(out.PySubs)) + "\n" +
		"# diffs: " + strconv.Itoa(len(out.Diffs)) + "\n"
	if err := os.WriteFile(filepath.Join(dir, name+".meta"), []byte(meta), 0o644); err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}
