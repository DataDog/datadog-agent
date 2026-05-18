// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build openmetrics_differential

package differential

import (
	"fmt"
	"testing"
)

// Stateful tests for the OpenMetrics differential harness. These exercise
// the scrape-to-scrape state machine on both sides (most notably the
// flush_first_value flag, but also share-label caching and sample
// disappearance handling).
//
// Background on flush_first_value:
//
//   Both implementations maintain a per-scraper flag. The flag governs
//   whether the *current* scrape's monotonic_count submissions are emitted
//   as a seed value (flush=true) or held back for delta computation
//   (flush=false). The state machine is:
//
//     constructor:          flag = false (Go) / None (Python; bool(None)=false)
//     scrape 1 runtime:     flush = false
//     after scrape 1 OK:    flag = true
//     scrape 2 runtime:     flush = true
//     scrape N OK:          flag = true (stays)
//     scrape N error:       flag downgraded to false IF it was true
//
//   So the first successful scrape uses flush=false; all subsequent
//   successful scrapes use flush=true. The `use_process_start_time`
//   integration can override this on the very first scrape (see test #2/3).

// TestStatefulFlushFirstValueDoubleScrape is the foundation: scrape the
// same payload twice against a session and verify both sides agree on the
// flush_first_value flag at each scrape.
func TestStatefulFlushFirstValueDoubleScrape(t *testing.T) {
	t.Parallel()
	sess, cleanup := setupSession(t, statefulInstance(nil))
	defer cleanup()

	payload := []byte("# TYPE foo counter\nfoo 5\n")

	// Scrape 1: both sides should emit `diff.foo.count` as monotonic_count
	// with flush_first_value=false (the initial-seed semantic).
	out1 := sess.Scrape(payload)
	requireNoErr(t, "scrape 1", out1)
	assertMonotonicCountFlush(t, "scrape 1 go", out1.GoSubs, "diff.foo.count", false)
	assertMonotonicCountFlush(t, "scrape 1 py", out1.PySubs, "diff.foo.count", false)
	requireAgree(t, "scrape 1", out1)

	// Scrape 2: flag flipped to true after scrape 1 succeeded; both sides
	// should now emit flush_first_value=true.
	out2 := sess.Scrape(payload)
	requireNoErr(t, "scrape 2", out2)
	assertMonotonicCountFlush(t, "scrape 2 go", out2.GoSubs, "diff.foo.count", true)
	assertMonotonicCountFlush(t, "scrape 2 py", out2.PySubs, "diff.foo.count", true)
	requireAgree(t, "scrape 2", out2)
}

// TestStatefulUseProcessStartTimeNoMarker exercises the case where
// `use_process_start_time: true` is set but the payload contains NO
// `process_start_time_seconds` metric. The Go side's `shouldFlushFirstValue`
// heuristic returns false in this case; the Python side calls into
// `datadog_agent.get_process_start_time()`, which in the sidecar context
// is likely to return None or fail.
//
// Expected outcome: divergence likely. This is a documented limitation of
// the sidecar (no real agent runtime), not a Go bug. The test asserts
// nothing strict beyond "both sides produce some output"; divergences are
// logged for visibility.
func TestStatefulUseProcessStartTimeNoMarker(t *testing.T) {
	t.Parallel()
	sess, cleanup := setupSession(t, statefulInstance(map[string]interface{}{
		"use_process_start_time": true,
	}))
	defer cleanup()

	payload := []byte("# TYPE foo counter\nfoo 5\n")

	out1 := sess.Scrape(payload)
	t.Logf("scrape 1 (no process_start_time marker): verdict=%s go=%d py=%d diffs=%d",
		out1.Verdict(), len(out1.GoSubs), len(out1.PySubs), len(out1.Diffs))
	if out1.GoErr != nil {
		t.Logf("  go err: %v", out1.GoErr)
	}
	if out1.PyErr != "" {
		t.Logf("  py err first-line: %s", firstLine(out1.PyErr))
	}

	out2 := sess.Scrape(payload)
	t.Logf("scrape 2: verdict=%s go=%d py=%d diffs=%d",
		out2.Verdict(), len(out2.GoSubs), len(out2.PySubs), len(out2.Diffs))

	// Information-only assertions: if scrape 2 diverges, that's a *new*
	// finding worth attention (scrape 1 divergence is the documented
	// sidecar limitation).
	if len(out2.Diffs) > 0 && out2.GoErr == nil && out2.PyErr == "" {
		t.Errorf("scrape 2 diverged with %d diffs; not expected once both sides have stabilised", len(out2.Diffs))
		summarizeDiffs(t, out2.Diffs)
	}
}

// TestStatefulUseProcessStartTimeWithMarker exercises the same flag but
// with a `process_start_time_seconds` gauge present in the payload. Go's
// heuristic inspects the payload directly; Python relies on the agent
// runtime callback. With the marker present and old (long before scrape
// time), Go's flush should activate. Python's behaviour in sidecar context
// is uncertain; this test exists to surface and document the divergence.
func TestStatefulUseProcessStartTimeWithMarker(t *testing.T) {
	t.Parallel()
	sess, cleanup := setupSession(t, statefulInstance(map[string]interface{}{
		"use_process_start_time": true,
	}))
	defer cleanup()

	// process_start_time_seconds reports unix seconds when the process
	// started. 1577836800 = 2020-01-01 UTC; very old relative to the test.
	payload := []byte(
		"# TYPE process_start_time_seconds gauge\n" +
			"process_start_time_seconds 1577836800\n" +
			"# TYPE foo counter\n" +
			"foo 5\n",
	)

	out1 := sess.Scrape(payload)
	t.Logf("scrape 1 (with process_start_time_seconds=2020-01-01): verdict=%s go=%d py=%d diffs=%d",
		out1.Verdict(), len(out1.GoSubs), len(out1.PySubs), len(out1.Diffs))
	logStatefulErrors(t, out1)
	if len(out1.Diffs) > 0 {
		summarizeDiffs(t, out1.Diffs)
	}

	out2 := sess.Scrape(payload)
	t.Logf("scrape 2: verdict=%s go=%d py=%d diffs=%d",
		out2.Verdict(), len(out2.GoSubs), len(out2.PySubs), len(out2.Diffs))

	if len(out2.Diffs) > 0 && out2.GoErr == nil && out2.PyErr == "" {
		t.Errorf("scrape 2 diverged with %d diffs", len(out2.Diffs))
		summarizeDiffs(t, out2.Diffs)
	}
}

// TestStatefulShareLabelsAcrossScrapes runs two consecutive scrapes with
// share_labels configured. The single-scrape harness has already shown
// share_labels diverges (worktickets/07007). This test asks the question:
// is the divergence stable across scrapes (cache-of-bugs) or do additional
// scrapes produce *new* divergent shapes (state corruption)?
//
// Both questions matter for the share_labels fix. If the cache amplifies
// the bug, the fix needs to address invalidation; if not, the fix is
// localized to first-scrape behaviour.
func TestStatefulShareLabelsAcrossScrapes(t *testing.T) {
	t.Parallel()

	// Minimal share_labels-shaped payload: one target metric and one
	// match-source metric. The share_labels config asks for `pod` to be
	// joined from kube_pod_info into samples of kube_pod_status_phase.
	payload := []byte(
		"# TYPE kube_pod_info gauge\n" +
			`kube_pod_info{namespace="default",pod="web-1"} 1` + "\n" +
			`kube_pod_info{namespace="default",pod="web-2"} 1` + "\n" +
			"# TYPE kube_pod_status_phase gauge\n" +
			`kube_pod_status_phase{namespace="default",phase="Running"} 1` + "\n" +
			`kube_pod_status_phase{namespace="default",phase="Pending"} 0` + "\n",
	)

	sess, cleanup := setupSession(t, statefulInstance(map[string]interface{}{
		"share_labels": map[string]interface{}{
			"kube_pod_status_phase": map[string]interface{}{
				"labels": []interface{}{"pod"},
				"match":  []interface{}{"kube_pod_info"},
			},
		},
	}))
	defer cleanup()

	out1 := sess.Scrape(payload)
	t.Logf("scrape 1: verdict=%s go=%d py=%d diffs=%d", out1.Verdict(), len(out1.GoSubs), len(out1.PySubs), len(out1.Diffs))
	logStatefulErrors(t, out1)

	out2 := sess.Scrape(payload)
	t.Logf("scrape 2: verdict=%s go=%d py=%d diffs=%d", out2.Verdict(), len(out2.GoSubs), len(out2.PySubs), len(out2.Diffs))
	logStatefulErrors(t, out2)

	// The interesting question: did scrape 2's diff count differ from
	// scrape 1's? If equal, the bug is stable across scrapes. If scrape 2
	// is worse, caching amplifies the bug.
	if len(out1.Diffs) != len(out2.Diffs) {
		t.Errorf("share_labels divergence shape changed across scrapes: scrape1=%d scrape2=%d diffs",
			len(out1.Diffs), len(out2.Diffs))
	}

	// If either scrape has diffs, log a sample for triage. Don't fail the
	// test on the existence of diffs alone — worktickets/07007 already
	// captures the share_labels bug. This test's value is the
	// scrape1-vs-scrape2 comparison above.
	if len(out1.Diffs) > 0 || len(out2.Diffs) > 0 {
		t.Logf("share_labels divergence observed (expected; see 07007): scrape1=%d scrape2=%d diffs",
			len(out1.Diffs), len(out2.Diffs))
	}
}

// TestStatefulSampleDisappearsThenReturns covers a common production
// scenario: a metric is present in one scrape, absent in the next (pod
// died), then reappears (new pod with same labels). Both implementations
// should:
//
//   - Not emit anything for the missing metric in the absent scrape.
//   - Treat the reappearance as a continuation (flush_first_value=true,
//     not a new seed), because the SCRAPER's flag is already true after
//     scrape 1.
func TestStatefulSampleDisappearsThenReturns(t *testing.T) {
	t.Parallel()
	sess, cleanup := setupSession(t, statefulInstance(nil))
	defer cleanup()

	payloadWith := []byte("# TYPE foo counter\nfoo 5\n")
	payloadWithout := []byte("# TYPE bar counter\nbar 1\n") // foo absent, an unrelated counter present

	// Scrape 1: foo present. Expect flush=false (initial seed).
	out1 := sess.Scrape(payloadWith)
	requireNoErr(t, "scrape 1", out1)
	assertMonotonicCountFlush(t, "scrape 1 go", out1.GoSubs, "diff.foo.count", false)
	assertMonotonicCountFlush(t, "scrape 1 py", out1.PySubs, "diff.foo.count", false)
	requireAgree(t, "scrape 1", out1)

	// Scrape 2: foo absent. Both sides should emit ONLY bar, not foo.
	out2 := sess.Scrape(payloadWithout)
	requireNoErr(t, "scrape 2", out2)
	assertNoSubmission(t, "scrape 2 go", out2.GoSubs, "diff.foo.count")
	assertNoSubmission(t, "scrape 2 py", out2.PySubs, "diff.foo.count")
	requireAgree(t, "scrape 2", out2)

	// Scrape 3: foo back. The scraper's flag is already true (set after
	// scrape 1 / 2's success), so foo should re-emerge with flush=true —
	// NOT flush=false (which would imply a fresh seed).
	out3 := sess.Scrape(payloadWith)
	requireNoErr(t, "scrape 3", out3)
	assertMonotonicCountFlush(t, "scrape 3 go", out3.GoSubs, "diff.foo.count", true)
	assertMonotonicCountFlush(t, "scrape 3 py", out3.PySubs, "diff.foo.count", true)
	requireAgree(t, "scrape 3", out3)
}

// ---- helpers ---------------------------------------------------------------

// setupSession is shared session bootstrap: starts the sidecar (if it isn't
// already running for this test), creates a payload server, and opens a
// stateful session bound to the given instance config. The returned cleanup
// closes the session, sidecar, and server in order.
func setupSession(t *testing.T, instance map[string]interface{}) (*statefulSession, func()) {
	t.Helper()
	sidecar, err := startPythonSidecar(t)
	if err != nil {
		t.Skipf("python sidecar unavailable: %v", err)
	}
	ps := newPayloadServer()
	sess, err := newStatefulSession(ps, sidecar, fmt.Sprintf("%s-1", t.Name()), instance)
	if err != nil {
		ps.Close()
		sidecar.Close()
		t.Fatalf("newStatefulSession: %v", err)
	}
	return sess, func() {
		_ = sess.Close()
		ps.Close()
		sidecar.Close()
	}
}

// statefulInstance merges per-test config over the standard baseline.
func statefulInstance(overrides map[string]interface{}) map[string]interface{} {
	cfg := map[string]interface{}{
		"namespace": "diff",
		"metrics":   []interface{}{".+"},
	}
	for k, v := range overrides {
		cfg[k] = v
	}
	return cfg
}

func requireNoErr(t *testing.T, label string, out iterationOutcome) {
	t.Helper()
	if out.GoErr != nil {
		t.Fatalf("%s: go err: %v", label, out.GoErr)
	}
	if out.PyErr != "" {
		t.Fatalf("%s: py err first-line: %s", label, firstLine(out.PyErr))
	}
}

func requireAgree(t *testing.T, label string, out iterationOutcome) {
	t.Helper()
	if len(out.Diffs) == 0 {
		return
	}
	t.Errorf("%s: expected go and py to agree, got %d divergences", label, len(out.Diffs))
	summarizeDiffs(t, out.Diffs)
}

func logStatefulErrors(t *testing.T, out iterationOutcome) {
	t.Helper()
	if out.GoErr != nil {
		t.Logf("  go err: %v", out.GoErr)
	}
	if out.PyErr != "" {
		t.Logf("  py err first-line: %s", firstLine(out.PyErr))
	}
}

// assertMonotonicCountFlush checks that `subs` contains exactly one
// monotonic_count submission for `name`, with the expected flush_first_value.
func assertMonotonicCountFlush(t *testing.T, label string, subs []Submission, name string, wantFlush bool) {
	t.Helper()
	var matches []Submission
	for _, s := range subs {
		if s.Kind == "monotonic_count" && s.Name == name {
			matches = append(matches, s)
		}
	}
	if len(matches) != 1 {
		t.Errorf("%s: expected exactly 1 monotonic_count for %q, found %d (all=%v)", label, name, len(matches), subs)
		return
	}
	if matches[0].FlushFirstValue != wantFlush {
		t.Errorf("%s: %q flush_first_value=%v, want %v", label, name, matches[0].FlushFirstValue, wantFlush)
	}
}

// assertNoSubmission checks that `subs` does NOT contain any submission
// with the given name.
func assertNoSubmission(t *testing.T, label string, subs []Submission, name string) {
	t.Helper()
	for _, s := range subs {
		if s.Name == name {
			t.Errorf("%s: unexpectedly found submission for %q: %+v", label, name, s)
		}
	}
}
