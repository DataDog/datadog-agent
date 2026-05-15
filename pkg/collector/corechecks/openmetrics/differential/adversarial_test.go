// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build openmetrics_differential

package differential

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestOpenMetricsAdversarial walks the hand-crafted AdversarialCatalog and
// runs each spec-corner case through the diff harness. Unlike
// TestOpenMetricsMutation, each case targets a *specific* behavior class —
// so when something diverges, the subtest name tells you which behavior is
// broken without further triage.
//
// Run with:
//
//	go test -tags openmetrics_differential -v -run TestOpenMetricsAdversarial \
//	        ./pkg/collector/corechecks/openmetrics/differential/
//
// Each case becomes a subtest, so `-run` can filter by category, e.g.:
//
//	-run 'TestOpenMetricsAdversarial/histogram'
//	-run 'TestOpenMetricsAdversarial/format/openmetrics_eof'
func TestOpenMetricsAdversarial(t *testing.T) {
	t.Parallel()

	sidecar, err := startPythonSidecar(t)
	if err != nil {
		t.Skipf("python sidecar unavailable, skipping adversarial: %v", err)
	}
	t.Cleanup(sidecar.Close)

	ps := newPayloadServer()
	t.Cleanup(ps.Close)

	catalog := AdversarialCatalog
	t.Logf("catalog: %d cases", len(catalog))

	// Tally outcomes across all cases for an at-a-glance summary at the end
	// of the run. (Subtest failures still fail the parent test.)
	tally := outcomeTally{}
	var interesting []string

	for _, tc := range catalog {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			instance := tc.Instance
			if instance == nil {
				instance = defaultAdversarialInstance
			}
			out := runIteration(ps, sidecar, tc.Payload, instance)
			verdict := out.Verdict()
			tally.inc(verdict)

			t.Logf("%s  go=%d py=%d diffs=%d", verdict, len(out.GoSubs), len(out.PySubs), len(out.Diffs))
			t.Logf("description: %s", tc.Description)
			if out.GoErr != nil {
				t.Logf("go err: %v", out.GoErr)
			}
			if out.PyErr != "" {
				// Python error blobs are huge tracebacks; print first line only.
				t.Logf("py err: %s", firstLine(out.PyErr))
			}

			switch verdict {
			case "agree", "both_rejected":
				// Expected for most cases. Some adversarial cases happen to
				// agree (both impls reject or both impls handle identically);
				// that's a finding too, but a quiet one.
			case "divergent":
				interesting = append(interesting, tc.Name)
				summarizeDiffs(t, out.Diffs)
				path, err := dumpAdversarial(tc, out)
				if err == nil {
					t.Logf("saved: %s", path)
				}
				t.Fail()
			case "go_rejected_py_accepted":
				interesting = append(interesting, tc.Name)
				path, _ := dumpAdversarial(tc, out)
				t.Errorf("go rejected, py accepted %d submissions; go_err=%v; saved=%s", len(out.PySubs), out.GoErr, path)
			case "go_accepted_py_rejected":
				interesting = append(interesting, tc.Name)
				path, _ := dumpAdversarial(tc, out)
				t.Errorf("py rejected, go accepted %d submissions; py_err=%s; saved=%s", len(out.GoSubs), firstLine(out.PyErr), path)
			}
		})
	}

	t.Logf("adversarial run complete: %s", tally.format())
	if len(interesting) > 0 {
		sort.Strings(interesting)
		t.Logf("%d interesting cases (re-run with -run='%s' to focus):", len(interesting), strings.Join(interesting, "|"))
		for _, name := range interesting {
			t.Logf("  - %s", name)
		}
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// dumpAdversarial writes the case payload + outcome metadata to
// testdata/regressions/ for offline inspection. Uses the case name as the
// filename so adversarial regressions are stable across runs (unlike mutation
// regressions which hash payload bytes).
func dumpAdversarial(tc AdversarialCase, out iterationOutcome) (string, error) {
	dir := filepath.Join("testdata", "regressions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	safe := strings.NewReplacer("/", "__", " ", "_").Replace(tc.Name)
	path := filepath.Join(dir, "adversarial_"+safe+".prom")
	if err := os.WriteFile(path, tc.Payload, 0o644); err != nil {
		return "", err
	}
	sum := sha256.Sum256(tc.Payload)
	meta := "# case: " + tc.Name + "\n" +
		"# description: " + tc.Description + "\n" +
		"# verdict: " + out.Verdict() + "\n" +
		"# go_err: " + errString(out.GoErr) + "\n" +
		"# py_err_first_line: " + firstLine(out.PyErr) + "\n" +
		"# go_subs: " + itoa(len(out.GoSubs)) + "\n" +
		"# py_subs: " + itoa(len(out.PySubs)) + "\n" +
		"# diffs: " + itoa(len(out.Diffs)) + "\n" +
		"# sha256: " + hex.EncodeToString(sum[:]) + "\n"
	return path, os.WriteFile(path+".meta", []byte(meta), 0o644)
}

func itoa(i int) string {
	// Tiny helper to avoid pulling in strconv just for this; matches the style
	// of mutation_test.go.
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	n := len(buf)
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	for i > 0 {
		n--
		buf[n] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		n--
		buf[n] = '-'
	}
	return string(buf[n:])
}
