// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build openmetrics_differential

package differential

import (
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"

	yaml "go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/openmetrics"
)

// payloadServer is a single httptest.Server whose response body is swappable
// atomically. Reused across many mutation/fuzz iterations so we don't churn
// ephemeral listeners (and so the URL stays stable, which keeps the
// endpoint:<url> tag stable across iterations — unrelated to correctness, but
// nicer for log triage).
type payloadServer struct {
	srv *httptest.Server
	buf atomic.Pointer[[]byte]
}

func newPayloadServer() *payloadServer {
	ps := &payloadServer{}
	ps.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		b := ps.buf.Load()
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.WriteHeader(http.StatusOK)
		if b != nil {
			_, _ = w.Write(*b)
		}
	}))
	return ps
}

func (ps *payloadServer) set(payload []byte) {
	// Defensive copy so callers can mutate their buffer without disturbing
	// in-flight requests.
	cp := make([]byte, len(payload))
	copy(cp, payload)
	ps.buf.Store(&cp)
}

func (ps *payloadServer) endpoint() string { return ps.srv.URL + "/metrics" }
func (ps *payloadServer) Close()            { ps.srv.Close() }

// iterationOutcome captures one Go-vs-Python parity check. It's the unit of
// work for both TestOpenMetricsMutation and FuzzOpenMetricsDifferential.
type iterationOutcome struct {
	GoSubs []Submission
	PySubs []Submission
	GoErr  error  // Go scraper returned an error
	PyErr  string // Python sidecar reported an error (string because tracebacks travel as text)
	Diffs  []Diff // empty if the two impls agree (or if either side errored — see Verdict)
}

// Verdict bucketizes an iteration into one of a handful of qualitative outcomes
// so callers can tally without re-deriving the rules.
func (o iterationOutcome) Verdict() string {
	goFailed := o.GoErr != nil
	pyFailed := o.PyErr != ""
	switch {
	case goFailed && pyFailed:
		return "both_rejected"
	case goFailed && !pyFailed:
		return "go_rejected_py_accepted"
	case !goFailed && pyFailed:
		return "go_accepted_py_rejected"
	case len(o.Diffs) > 0:
		return "divergent"
	default:
		return "agree"
	}
}

// runIteration drives one Go scrape + one Python scrape against `payload`
// served from `ps`, then diffs. Errors on either side are returned in the
// outcome, not via Go's error — the caller decides whether they're fatal or
// signal.
func runIteration(ps *payloadServer, sidecar *pythonSidecar, payload []byte, instance map[string]interface{}) iterationOutcome {
	ps.set(payload)

	instanceWithEndpoint := map[string]interface{}{}
	for k, v := range instance {
		instanceWithEndpoint[k] = v
	}
	instanceWithEndpoint["openmetrics_endpoint"] = ps.endpoint()

	goSubs, goErr := runGoScrape(instanceWithEndpoint)

	// The sidecar fills in openmetrics_endpoint itself — pass the unmodified
	// instance config so we don't end up with the field set twice.
	pyResp, err := sidecar.run(ps.endpoint(), instance)
	if err != nil {
		// Sidecar protocol failure is fatal: it means the Python process is
		// wedged, not that the *check* rejected the payload. Surface as a
		// hard error by populating GoErr (caller will treat as agreement
		// only if Py also errored; here we have no Py info, so mark go-fail).
		return iterationOutcome{GoErr: fmt.Errorf("sidecar protocol: %w", err)}
	}

	out := iterationOutcome{
		GoSubs: goSubs,
		PySubs: pyResp.Submissions,
		GoErr:  goErr,
		PyErr:  pyResp.Error,
	}

	// Only diff when both sides actually produced output. If either side
	// rejected the payload, the Verdict already captures the asymmetry; a
	// formal multiset diff in that case would be noise (one side trivially
	// has everything "only_in_X").
	if goErr == nil && pyResp.Error == "" {
		out.Diffs = CompareSubmissions(goSubs, pyResp.Submissions)
	}
	return out
}

func runGoScrape(instance map[string]interface{}) ([]Submission, error) {
	raw, err := yaml.Marshal(instance)
	if err != nil {
		return nil, fmt.Errorf("marshal instance: %w", err)
	}
	scraper, err := openmetrics.NewScraperFromYAML(raw, "differential-test")
	if err != nil {
		return nil, fmt.Errorf("NewScraperFromYAML: %w", err)
	}
	rec := &RecordingSender{}
	if err := scraper.Scrape(rec); err != nil {
		return nil, fmt.Errorf("scrape: %w", err)
	}
	return rec.Submissions, nil
}

func loadGzipped(path string) ([]byte, error) {
	abs, _ := filepath.Abs(path)
	f, err := os.Open(abs)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	return io.ReadAll(gz)
}

// ---- stateful session ------------------------------------------------------

// statefulSession runs N consecutive scrapes against the same Go scraper +
// Python check instance, so scrape-to-scrape state (flush_first_value, the
// share-labels cache, etc.) is preserved across calls. This is the only
// shape that exercises behaviour like "first-scrape skip" or stale-metric
// handling.
//
// Construction and teardown are explicit: callers must defer Close to avoid
// leaking sessions in the Python sidecar.
type statefulSession struct {
	ps        *payloadServer
	scraper   *openmetrics.Scraper
	sidecar   *pythonSidecar
	sessionID string
	instance  map[string]interface{}
	scrapeNum int
}

// newStatefulSession constructs the Go-side scraper from `instance`, opens a
// matching session on the Python sidecar pinned to the supplied payload
// server's endpoint, and returns a handle. Returns an error if either side
// fails to initialize.
//
// The payload server is borrowed (not owned); the caller is responsible for
// closing it when finished.
func newStatefulSession(ps *payloadServer, sidecar *pythonSidecar, sessionID string, instance map[string]interface{}) (*statefulSession, error) {
	// Build instance-with-endpoint for both sides.
	instanceCopy := map[string]interface{}{}
	for k, v := range instance {
		instanceCopy[k] = v
	}
	instanceCopy["openmetrics_endpoint"] = ps.endpoint()

	raw, err := yaml.Marshal(instanceCopy)
	if err != nil {
		return nil, fmt.Errorf("marshal instance: %w", err)
	}
	scraper, err := openmetrics.NewScraperFromYAML(raw, "differential-stateful-"+sessionID)
	if err != nil {
		return nil, fmt.Errorf("go scraper init: %w", err)
	}

	// Open the Python session. Pass the *original* instance (no endpoint set
	// twice) and let the sidecar add openmetrics_endpoint itself.
	if err := sidecar.openSession(sessionID, ps.endpoint(), instance); err != nil {
		return nil, fmt.Errorf("py session init: %w", err)
	}

	return &statefulSession{
		ps:        ps,
		scraper:   scraper,
		sidecar:   sidecar,
		sessionID: sessionID,
		instance:  instance,
	}, nil
}

// Scrape runs one scrape cycle: sets the payload, invokes the Go scraper,
// requests a scrape from the Python session, and diffs. The Diffs slice is
// populated only when both sides produce output (matching runIteration's
// semantics).
func (s *statefulSession) Scrape(payload []byte) iterationOutcome {
	s.scrapeNum++
	s.ps.set(payload)

	// Go scrape against the long-lived scraper.
	rec := &RecordingSender{}
	var goErr error
	if err := s.scraper.Scrape(rec); err != nil {
		goErr = fmt.Errorf("scrape: %w", err)
	}

	// Python scrape against the persistent session.
	pyResp, err := s.sidecar.scrapeSession(s.sessionID)
	if err != nil {
		return iterationOutcome{GoErr: fmt.Errorf("sidecar protocol: %w", err)}
	}

	out := iterationOutcome{
		GoSubs: rec.Submissions,
		PySubs: pyResp.Submissions,
		GoErr:  goErr,
		PyErr:  pyResp.Error,
	}
	if goErr == nil && pyResp.Error == "" {
		out.Diffs = CompareSubmissions(rec.Submissions, pyResp.Submissions)
	}
	return out
}

// ScrapeCount returns the number of scrapes performed against this session
// (useful for logging / failure reporting).
func (s *statefulSession) ScrapeCount() int { return s.scrapeNum }

// Close releases the Python-side session. The Go scraper has no
// corresponding teardown step. Safe to call multiple times; subsequent calls
// are no-ops in the sidecar.
func (s *statefulSession) Close() error {
	return s.sidecar.closeSession(s.sessionID)
}
