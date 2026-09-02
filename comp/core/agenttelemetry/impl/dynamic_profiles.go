// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttelemetryimpl

// Dynamic profiles for agent telemetry.
//
// The baseline profile set is embedded at build time (defaultProfiles.yaml), so adding a new metric to be collected
// would normally require an Agent release, and wouldn't retroactively apply to already-deployed versions. Dynamic
// profiles allow periodically fetching a Datadog-controlled configuration which is applied on top of the baseline
// configuration in an additive, best-effort fashion, in order to dynamically augment the metrics collected for
// existing Agent deployments, without restarting.
//
// Highlighting the major tenets:
//
// - **Additive only.** The dynamic configuration cannot be used to change the baseline profile, so there is no chance
//   of regressing behavior by disabling collection of baseline telemetry.
// - **Silent.** Errors encountered during the polling and application of any dynamic configuration are not surfaced to
//   the user and do not impact baseline telemetry collection.
// - **Deduplicated.** A dynamic profile never re-collects something the baseline already collects. Beyond avoiding
//   duplicate points, this is what keeps the counter/histogram delta cache single-consumer: two profiles emitting the
//   same metric would have one consume the delta and the other report zero.
// - **Low frequency and heavily cached.** State is shared by every agent process on the host so a host makes roughly
//   one request per day rather than one per process, and so restart loops do not amplify traffic.
//
// Every long-running agent binary runs its own copy of this (core agent, trace-agent, process-agent, system-probe,
// security-agent, ...), each against its own telemetry registry. So on a typical host several processes would each poll
// the endpoint daily, which is why the cache below is shared across processes rather than kept in memory.
//
// Reading order: the on-disk cache and its concurrency rules, then parsing and deduplication, then the manager that
// ties them to the component's poll job.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	defaultDynamicProfilesURL    = "https://agent-telemetry.datadoghq.com/v1/fetch_dynamic_profiles"
	dynamicProfilesCacheFileName = "agent-telemetry-dynamic-profiles.json"

	// dynamicProfilesCacheVersion guards the on-disk format. A cache file written by an agent with a different version
	// is ignored rather than misread, so the field layout can change without a migration.
	dynamicProfilesCacheVersion = 1

	// Bounds on anything read from the network or from disk. The document is a short list of metric names; a megabyte
	// is already far more than it should ever need.
	dynamicProfilesMaxBodyBytes  = 1 << 20
	dynamicProfilesMaxCacheBytes = dynamicProfilesMaxBodyBytes + 4096
	dynamicProfilesMaxProfiles   = 64
	dynamicProfilesMaxMetrics    = 512

	dynamicProfilesFetchTimeout = 10 * time.Second

	// dynamicProfilesMinPollSeconds floors the poll period. Period 0 would make cron
	// reschedule immediately and spin.
	dynamicProfilesMinPollSeconds = 300

	dynamicProfilesConfigPrefix = "agent_telemetry.dynamic_profiles."

	defaultDynamicProfilesPollIntervalSeconds  = 86400
	defaultDynamicProfilesStartupJitterSeconds = 300
)

// Enumerated failure stages. These are the only values that ever reach the request header, so a formatted error --
// which can embed proxy hostnames, filesystem paths or resolver output -- can never leak off the host. Non-leaking by
// construction, and aggregatable server-side.
const (
	stageNet             = "net"
	stageBody            = "body"
	stageTooLarge        = "too_large"
	stageYAML            = "yaml"
	stageMultiDoc        = "multi_doc"
	stageCompile         = "compile"
	stageEmpty           = "empty"
	stageTooManyProfiles = "too_many_profiles"
	stageTooManyMetrics  = "too_many_metrics"
	stageBadURL          = "bad_url"
	stageCacheWrite      = "cache_write"
	stageUnknown         = "unknown"
)

// profilesDocument is the only shape accepted from the network.
//
// It is deliberately narrower than Config: a remote document may add profiles, but must never be able to flip `enabled`
// or `startup_trace_sampling`. Because parsing is strict, omitting those fields here turns their presence into a hard
// parse failure rather than a silent no-op.
type profilesDocument struct {
	Profiles []*Profile `yaml:"profiles"`
}

// stageError attaches an enumerated stage token to a failure so the poller can report it without ever embedding
// the underlying error text.
type stageError struct {
	stage string
	err   error
}

func (e *stageError) Error() string { return e.stage + ": " + e.err.Error() }
func (e *stageError) Unwrap() error { return e.err }

func newStageError(stage string, format string, args ...any) *stageError {
	return &stageError{stage: stage, err: fmt.Errorf(format, args...)}
}

// stageOf extracts the enumerated stage from an error, defaulting to "unknown".
func stageOf(err error) string {
	var se *stageError
	if errors.As(err, &se) {
		return se.stage
	}
	return stageUnknown
}

// causeOf strips the stage prefix so a log line that already reports the stage does not
// repeat it.
func causeOf(err error) error {
	var se *stageError
	if errors.As(err, &se) {
		return se.err
	}
	return err
}

// ---------------------------------------------------------------------------
// on-disk state
// ---------------------------------------------------------------------------

// failureRecord is what the endpoint is told about the previous poll's failure. Stage is an enumerated token, never
// formatted error text.
type failureRecord struct {
	Stage string `json:"stage"`
	// At is Unix *seconds*, unlike the cache's nanosecond timestamps: this one is only ever reported, never compared,
	// so second resolution is what the receiving end wants.
	At     int64  `json:"at"`
	Flavor string `json:"flavor"`
}

// The on-disk cache is split across two files, and the split is load-bearing.
//
// The document and the attempt record are produced by different code paths -- only a successful fetch has a document,
// every attempt has a record -- and peer agent processes write them concurrently without a lock. Holding both in one
// file required a read-modify-write, which let a failing process serialize its stale snapshot of the document over a
// peer's freshly fetched one:
//
//	A: fetch succeeds                  B: fetch fails (transient 500)
//	                                   B: reads state (config = old/empty)
//	A: writes config + attempted_at
//	                                   B: writes attempted_at + last_error, and with them
//	                                      its stale empty config -- erasing A's document
//
// attempted_at survives that interleaving, so every process on the host then skips the request for a full poll
// interval while collecting nothing, and adoptCached actively clears peers that had already applied the document.
// Silent, and up to a day long by default.
//
// Two files remove the interleaving by construction: a failed attempt can no longer touch the document. Each file has
// exactly one writer role and is written as a whole, so no read-modify-write is needed at all. What remains is benign:
// two successes race to write the same document, and two failures race to record one of two failure stages.

// cachedDocument holds the fetched document. Written only by a successful fetch.
type cachedDocument struct {
	Version int `json:"version"`
	// FetchedAtNS is Unix nanoseconds. Nanoseconds rather than seconds because this field orders concurrent fetches
	// (see writeCachedDocument), and two polls colliding inside the same second is the normal case for that
	// race rather than the exceptional one.
	FetchedAtNS int64  `json:"fetched_at_ns"`
	ETag        string `json:"etag,omitempty"`
	Config      string `json:"config,omitempty"`
}

// attemptRecord records the most recent poll attempt by any process on this host, and is what gates the HTTP
// request. Tracked separately from the document's FetchedAtNS: gating on success alone would mean a permanently failing
// endpoint gets retried by every restart of every process -- amplification on exactly the path where the server is
// already unhealthy.
type attemptRecord struct {
	Version int `json:"version"`
	// AttemptedAtNS is Unix nanoseconds, matching the document's FetchedAtNS so the two files can never be compared in
	// mismatched units.
	AttemptedAtNS int64          `json:"attempted_at_ns"`
	LastError     *failureRecord `json:"last_error,omitempty"`
}

// attemptPathFor places the attempt record beside the document, honouring a configured cache_path.
func attemptPathFor(documentPath string) string {
	if documentPath == "" {
		return ""
	}
	ext := filepath.Ext(documentPath)
	return strings.TrimSuffix(documentPath, ext) + "-attempt" + ext
}

// writeJSONAtomic serializes v to path by staging a temp file and renaming it into place. Every step below is load
// bearing for the concurrent, cross-user access this cache sees:
//
//   - rename is atomic, so a reader (a peer process, or the flare) sees either the whole previous file or the whole new
//     one, never a partial write.
//   - os.CreateTemp gives a name unique per call, so concurrent writers cannot corrupt each other's staging file, and a
//     process killed mid-write leaks nothing that a later writer would trip over.
//   - Chmod 0644 keeps the file readable across users, because these processes do not share one: the core agent runs as
//     dd-agent while system-probe and security-agent run as root. CreateTemp makes 0600, and unlike os.WriteFile's perm
//     argument, chmod is not filtered through the process umask.
//   - Sync before the rename so a hard crash cannot commit the directory entry without its data.
//
// beforeRename, when non-nil, runs once the temp file is fully staged and immediately before the rename. A caller that
// needs to compare against whatever is on disk must do it there rather than up front, because staging includes an fsync
// -- milliseconds under load -- and a peer's rename landing in that gap would be silently overwritten. Returning an
// error abandons the write; the deferred cleanup discards the temp file.
func writeJSONAtomic(path string, v any, beforeRename func() error) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		// Both are no-ops on the success path: the file is already closed and renamed away.
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.Write(raw); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Chmod(0644); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	// Last-moment check: everything slow is already done, so only a read and the rename remain.
	if beforeRename != nil {
		if err := beforeRename(); err != nil {
			return err
		}
	}

	return os.Rename(tmpName, path)
}

// readJSONFile reports whether the file existed. A missing file is not an error: it just means no agent process on this
// host has got that far yet.
func readJSONFile(path string, v any, maxBytes int) (bool, error) {
	if path == "" {
		return false, nil
	}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	defer f.Close()

	// Bound the read rather than checking the size afterwards: anything else with write access to run_path could
	// otherwise make us pull an arbitrarily large file into memory. One byte past the limit distinguishes "at the
	// limit" from "over it".
	raw, err := io.ReadAll(io.LimitReader(f, int64(maxBytes)+1))
	if err != nil {
		return false, err
	}
	if len(raw) > maxBytes {
		return false, fmt.Errorf("%s exceeds %d bytes", filepath.Base(path), maxBytes)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return false, err
	}

	return true, nil
}

func readCachedDocument(path string) (*cachedDocument, error) {
	var doc cachedDocument
	ok, err := readJSONFile(path, &doc, dynamicProfilesMaxCacheBytes)
	if err != nil || !ok {
		return nil, err
	}
	if doc.Version != dynamicProfilesCacheVersion {
		return nil, fmt.Errorf("unsupported document version %d", doc.Version)
	}
	return &doc, nil
}

// errDocumentSuperseded reports that the document already on disk was fetched more recently than the one we were about
// to write, so ours was not written.
var errDocumentSuperseded = errors.New("cached document superseded by a newer fetch")

// writeCachedDocument replaces the cached document, unless the one already on disk was fetched more recently.
//
// Two processes on this host can both pass the advisory poll gate and fetch. If the endpoint's document changed between
// their two responses, one of them holds a document that is already stale -- and because poll parses, filters and
// applies before persisting, the stale fetcher can easily be the one that renames last. A blind overwrite would then
// leave the older document on disk, and every process that adopts from disk or restarts would collect it until the next
// poll: a day, by default.
//
// Comparing fetch timestamps is sound here specifically because every writer is a process on the same host, sharing one
// clock. The same reasoning would not carry across hosts.
//
// The comparison runs as writeJSONAtomic's beforeRename hook, not up front, so that only a read and the rename sit
// between observing the file and replacing it. Checking before staging would put an fsync inside the window.
//
// This narrows the window rather than closing it: a peer's rename can still land between our read and ours. Closing it
// would need a lock shared by agent processes running as different users, which is not a trade worth making for a
// best-effort additive cache.
//
// Ties are allowed through: equal timestamps are indistinguishable, and the documents are then equivalent anyway.
func writeCachedDocument(path string, doc *cachedDocument) error {
	if path == "" {
		return nil
	}

	doc.Version = dynamicProfilesCacheVersion
	return writeJSONAtomic(path, doc, func() error {
		// A corrupt or foreign-version file reads as an error and is overwritten, which is what we want.
		existing, err := readCachedDocument(path)
		if err != nil || existing == nil {
			return nil
		}
		if existing.FetchedAtNS > doc.FetchedAtNS {
			return errDocumentSuperseded
		}
		return nil
	})
}

func readAttemptRecord(path string) (*attemptRecord, error) {
	var at attemptRecord
	ok, err := readJSONFile(path, &at, dynamicProfilesMaxCacheBytes)
	if err != nil || !ok {
		return nil, err
	}
	if at.Version != dynamicProfilesCacheVersion {
		return nil, fmt.Errorf("unsupported attempt version %d", at.Version)
	}
	return &at, nil
}

func writeAttemptRecord(path string, at *attemptRecord) error {
	if path == "" {
		return nil
	}
	at.Version = dynamicProfilesCacheVersion
	// No precondition: the attempt record is advisory. A stale writer landing an older attempted_at only makes the
	// gate open marginally sooner, i.e. one extra request, which the design already tolerates.
	return writeJSONAtomic(path, at, nil)
}

// ---------------------------------------------------------------------------
// network
// ---------------------------------------------------------------------------

type fetchResult struct {
	body []byte
	etag string

	// notModified is set for a 304: the cached document is still current.
	notModified bool
	// absent is set for 204/404: the server has withdrawn the document, which is how a previously applied document gets
	// cleared remotely.
	absent bool
}

// fetch performs the conditional GET.
//
// dp.httpClient is nil until NewComponent wires one, which is what keeps createAtel -- and therefore every unit test
// that goes through it -- network-inert by construction. Reusing the sender's one-method `client` interface avoids
// introducing a second seam purely for tests.
func (dp *dynamicProfilesManager) fetch(ctx context.Context, headers map[string]string) (fetchResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dp.url, nil)
	if err != nil {
		return fetchResult{}, &stageError{stage: stageBadURL, err: err}
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := dp.httpClient.Do(req)
	if err != nil {
		return fetchResult{}, &stageError{stage: stageNet, err: err}
	}
	defer func() {
		// Drain (bounded) so the connection can be reused, then close.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, dynamicProfilesMaxBodyBytes))
		_ = resp.Body.Close()
	}()

	switch {
	case resp.StatusCode == http.StatusNotModified:
		return fetchResult{notModified: true}, nil
	case resp.StatusCode == http.StatusNoContent, resp.StatusCode == http.StatusNotFound:
		return fetchResult{absent: true}, nil
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		return fetchResult{}, newStageError(fmt.Sprintf("http_%d", resp.StatusCode), "unexpected status %d", resp.StatusCode)
	}

	// Read one byte past the limit so an oversized body is detected rather than silently truncated into a document that
	// happens to still parse.
	body, err := io.ReadAll(io.LimitReader(resp.Body, dynamicProfilesMaxBodyBytes+1))
	if err != nil {
		return fetchResult{}, &stageError{stage: stageBody, err: err}
	}
	if len(body) > dynamicProfilesMaxBodyBytes {
		return fetchResult{}, newStageError(stageTooLarge, "body exceeds %d bytes", dynamicProfilesMaxBodyBytes)
	}

	return fetchResult{body: body, etag: resp.Header.Get("ETag")}, nil
}

// validateProfilesURL requires HTTPS, except against a loopback host so that tests and local manual verification can
// use a plain HTTP server.
func validateProfilesURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Host == "" {
		return "", errors.New("missing host")
	}

	switch u.Scheme {
	case "https":
	case "http":
		if !isLoopbackHost(u.Hostname()) {
			return "", fmt.Errorf("plain http is only allowed for loopback hosts, got %q", u.Host)
		}
	default:
		return "", fmt.Errorf("unsupported scheme %q", u.Scheme)
	}

	return u.String(), nil
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// parseProfilesDocument strictly parses and fully validates a document. Errors carry an enumerated stage.
func parseProfilesDocument(raw []byte) (*Config, error) {
	if len(raw) > dynamicProfilesMaxBodyBytes {
		return nil, newStageError(stageTooLarge, "document is %d bytes, limit is %d", len(raw), dynamicProfilesMaxBodyBytes)
	}

	// A decoder rather than UnmarshalStrict, so a second document in the stream is rejected instead of silently
	// ignored.
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.SetStrict(true)

	var doc profilesDocument
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, newStageError(stageEmpty, "document is empty")
		}
		return nil, &stageError{stage: stageYAML, err: err}
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, newStageError(stageMultiDoc, "expected exactly one YAML document")
	}

	if len(doc.Profiles) == 0 {
		return nil, newStageError(stageEmpty, "document declares no profiles")
	}
	if len(doc.Profiles) > dynamicProfilesMaxProfiles {
		return nil, newStageError(stageTooManyProfiles, "%d profiles, limit is %d",
			len(doc.Profiles), dynamicProfilesMaxProfiles)
	}

	metrics := 0
	for _, p := range doc.Profiles {
		if p == nil {
			return nil, newStageError(stageCompile, "document contains a null profile")
		}
		if p.Metric != nil {
			metrics += len(p.Metric.Metrics)
		}
	}
	if metrics > dynamicProfilesMaxMetrics {
		return nil, newStageError(stageTooManyMetrics, "%d metrics, limit is %d", metrics, dynamicProfilesMaxMetrics)
	}

	// Same validation and compilation the baseline goes through: profile names, metric name format, exclude blocks, the
	// reserved "total" tag rule, schedule defaulting and event registration.
	cfg := &Config{Profiles: doc.Profiles}
	if err := compileConfig(cfg); err != nil {
		return nil, &stageError{stage: stageCompile, err: err}
	}

	return cfg, nil
}

// filterDynamicProfiles removes everything the baseline already covers and recompiles what is left. The returned
// Config is independent of the baseline and safe to publish.
//
// Three deduplication passes are used:
//
//  1. Profile name: if a dynamic profile matches a baseline profile by name, the dynamic profile is ignored.
//
//  2. Metric name: when a metric is already collected by an existing profile, whether baseline or dynamic, subsequent
//     references to the metric are ignored. Since we track the delta of various counters/histograms to ensure proper
//     emission between interations, having two profiles try to emit the same metric would lead to one "consuming" the
//     delta and the other getting zero... on top of try to report the same point twice.
//
//  3. Event name: the way events are registered means they can't be registered for collection multiple times.
func filterDynamicProfiles(baseline, dynamic *Config, logComp log.Component) (*Config, error) {
	profileNames := make(map[string]struct{}, len(baseline.Profiles))
	metricNames := make(map[string]struct{})
	for _, p := range baseline.Profiles {
		profileNames[p.Name] = struct{}{}
		if p.Metric == nil {
			continue
		}
		for _, m := range p.Metric.Metrics {
			metricNames[m.Name] = struct{}{}
		}
	}

	kept := make([]*Profile, 0, len(dynamic.Profiles))
	for _, p := range dynamic.Profiles {
		if _, dup := profileNames[p.Name]; dup {
			logComp.Debugf("agent telemetry dynamic profiles: dropping profile %q, name already in use", p.Name)
			continue
		}

		if p.Metric != nil && len(p.Metric.Metrics) > 0 {
			metrics := make([]MetricConfig, 0, len(p.Metric.Metrics))
			for _, m := range p.Metric.Metrics {
				if _, dup := metricNames[m.Name]; dup {
					logComp.Debugf("agent telemetry dynamic profiles: dropping metric %q from profile %q, already collected",
						m.Name, p.Name)
					continue
				}
				metricNames[m.Name] = struct{}{}
				metrics = append(metrics, m)
			}
			p.Metric.Metrics = metrics
		}

		if len(p.Events) > 0 {
			events := make([]*Event, 0, len(p.Events))
			for _, e := range p.Events {
				if _, dup := baseline.events[e.Name]; dup {
					logComp.Debugf("agent telemetry dynamic profiles: dropping event %q from profile %q, already registered",
						e.Name, p.Name)
					continue
				}
				events = append(events, e)
			}
			p.Events = events
		}

		hasMetrics := p.Metric != nil && len(p.Metric.Metrics) > 0
		if !hasMetrics && len(p.Events) == 0 {
			logComp.Debugf("agent telemetry dynamic profiles: dropping profile %q, nothing left after deduplication", p.Name)
			continue
		}

		// Also dedup within the document itself.
		profileNames[p.Name] = struct{}{}
		kept = append(kept, p)
	}

	// Recompile so schedule and events describe the surviving set: compileMetric stores pointers into the metrics
	// slice, which the filtering above rebuilt.
	filtered := &Config{Profiles: kept}
	if err := compileConfig(filtered); err != nil {
		return nil, &stageError{stage: stageCompile, err: err}
	}

	return filtered, nil
}

// profilesSnapshot is the published dynamic-profiles state. Immutable once stored.
type profilesSnapshot struct {
	profiles []*Profile
	events   map[string]*Event

	// raw is the document these profiles came from, kept so a poll that returns an unchanged document can be a no-op
	// instead of churning cron entries.
	raw string
}

// dynamicProfilesManager owns the feature: it holds the poll configuration, performs the poll, and publishes the
// resulting profiles for atel to read.
//
// Published state is a single immutable snapshot swapped atomically. Nothing here ever mutates atel.atelCfg, which is
// written once at construction -- that invariant is what lets atel's readers stay lock-free.
type dynamicProfilesManager struct {
	a       *atel
	logComp log.Component

	url          string
	cachePath    string
	attemptPath  string
	pollInterval time.Duration
	jitter       time.Duration

	// httpClient is nil until NewComponent wires one; poll is a no-op while it is.
	httpClient client
	nowFn      func() time.Time

	snapshot atomic.Pointer[profilesSnapshot]

	// lastError mirrors the persisted failure so this process still reports it in the next poll's header when the disk
	// write itself failed.
	lastError atomic.Pointer[failureRecord]

	// mu serializes apply: the removeJob loop, the addJob loop and the snapshot store must not interleave with a second
	// applier or job IDs would leak.
	mu     sync.Mutex
	jobIDs []jobID
}

func newDynamicProfilesManager(a *atel, cfgComp config.Component, logComp log.Component) *dynamicProfilesManager {
	dp := &dynamicProfilesManager{
		a:       a,
		logComp: logComp,
		nowFn:   time.Now,
	}

	rawURL := cfgComp.GetString(dynamicProfilesConfigPrefix + "url")
	if rawURL == "" {
		rawURL = defaultDynamicProfilesURL
	}
	if validated, err := validateProfilesURL(rawURL); err != nil {
		// Leave url empty: poll records bad_url and does nothing, but a previously cached document is still applied at
		// startup.
		logComp.Debugf("agent telemetry dynamic profiles: unusable url: %v", err)
	} else {
		dp.url = validated
	}

	dp.cachePath = cfgComp.GetString(dynamicProfilesConfigPrefix + "cache_path")
	if dp.cachePath == "" {
		if runPath := cfgComp.GetString("run_path"); runPath != "" {
			dp.cachePath = filepath.Join(runPath, dynamicProfilesCacheFileName)
		}
	}
	if dp.cachePath == "" {
		logComp.Debugf("agent telemetry dynamic profiles: no writable cache path, running without a disk cache")
	}
	dp.attemptPath = attemptPathFor(dp.cachePath)

	pollSeconds := getNonNegativeIntSetting(cfgComp, logComp,
		dynamicProfilesConfigPrefix+"poll_interval_seconds", defaultDynamicProfilesPollIntervalSeconds)
	if pollSeconds < dynamicProfilesMinPollSeconds {
		logComp.Debugf("agent telemetry dynamic profiles: poll interval %ds is below the %ds floor, clamping",
			pollSeconds, dynamicProfilesMinPollSeconds)
		pollSeconds = dynamicProfilesMinPollSeconds
	}
	dp.pollInterval = time.Duration(pollSeconds) * time.Second

	jitterSeconds := getNonNegativeIntSetting(cfgComp, logComp,
		dynamicProfilesConfigPrefix+"startup_jitter_seconds", defaultDynamicProfilesStartupJitterSeconds)
	dp.jitter = time.Duration(jitterSeconds) * time.Second

	return dp
}

// pollSchedule describes the poll job: one run after a random startup delay, then once per interval, forever
// (Iterations 0).
func (dp *dynamicProfilesManager) pollSchedule() Schedule {
	var startAfter uint
	if dp.jitter > 0 {
		// The configured value is the maximum; the actual delay is uniform in [0, max). rand.Int63n panics on n <= 0,
		// which the guard covers.
		startAfter = uint(time.Duration(rand.Int63n(int64(dp.jitter))) / time.Second)
	}

	return Schedule{
		StartAfter: startAfter,
		Iterations: 0,
		Period:     uint(dp.pollInterval / time.Second),
	}
}

// load returns the published snapshot, or nil when the feature is off or nothing has been applied yet. Nil-receiver
// safe: atel calls this unconditionally.
func (dp *dynamicProfilesManager) load() *profilesSnapshot {
	if dp == nil {
		return nil
	}
	return dp.snapshot.Load()
}

// applyCachedAtStartup publishes whatever is already on disk, so dynamic-profile collection begins immediately
// instead of waiting out the first (jittered) poll -- and so a short-lived process contributes no traffic at all.
//
// A cached document is applied regardless of age. Continuing to collect stale dynamic profiles while the endpoint is
// unreachable is the desired best-effort behaviour; the server withdraws a document by answering 204 or 404, which
// clears the cache.
func (dp *dynamicProfilesManager) applyCachedAtStartup() {
	doc, err := readCachedDocument(dp.cachePath)
	if err != nil {
		dp.logComp.Debugf("agent telemetry dynamic profiles: unusable cache: %v", err)
		return
	}
	if doc == nil || doc.Config == "" {
		return
	}

	if err := dp.applyRaw(doc.Config); err != nil {
		// The cached document does not parse for this agent build. Record it so the next request reports the stage.
		dp.recordFailure(stageOf(err), err)
	}
}

func (dp *dynamicProfilesManager) poll(ctx context.Context) {
	if dp == nil {
		return
	}
	if dp.httpClient == nil {
		dp.logComp.Debugf("agent telemetry dynamic profiles: no http client configured, skipping poll")
		return
	}
	if dp.url == "" {
		dp.recordFailure(stageBadURL, errors.New("no usable dynamic profiles url"))
		return
	}

	// Re-read rather than trusting what startup saw: a peer agent process on this host may have polled in the meantime.
	attempt, err := readAttemptRecord(dp.attemptPath)
	if err != nil {
		dp.logComp.Debugf("agent telemetry dynamic profiles: unusable attempt record: %v", err)
		attempt = nil
	}
	doc, err := readCachedDocument(dp.cachePath)
	if err != nil {
		dp.logComp.Debugf("agent telemetry dynamic profiles: unusable cache: %v", err)
		doc = nil
	}

	now := dp.nowFn()
	if attempt != nil && attempt.AttemptedAtNS > 0 {
		if age := now.Sub(time.Unix(0, attempt.AttemptedAtNS)); age >= 0 && age < dp.pollInterval {
			dp.logComp.Debugf("agent telemetry dynamic profiles: cache attempted %s ago, skipping request", age)
			dp.adoptCached(doc)
			return
		}
	}

	res, err := dp.fetch(ctx, dp.buildHeaders(doc, attempt))
	if ctx.Err() != nil {
		// Shutdown cancelled the request. Recording this would poison the next boot's error header with a
		// self-inflicted failure.
		return
	}
	if err != nil {
		dp.recordFailure(stageOf(err), err)
		return
	}

	switch {
	case res.notModified:
		dp.logComp.Debugf("agent telemetry dynamic profiles: not modified")
		dp.recordAttempt(now)
		return
	case res.absent:
		dp.logComp.Debugf("agent telemetry dynamic profiles: no dynamic profiles published")
		dp.apply(nil, "")
		dp.recordFetched("", "", now)
		return
	}

	if err := dp.applyRaw(string(res.body)); err != nil {
		dp.recordFailure(stageOf(err), err)
		return
	}
	dp.recordFetched(string(res.body), res.etag, now)
}

// adoptCached publishes a document a peer process already fetched.
func (dp *dynamicProfilesManager) adoptCached(doc *cachedDocument) {
	if doc == nil || doc.Config == "" {
		// Only clear if something is actually published, so a run of empty polls does not churn.
		if cur := dp.load(); cur != nil && len(cur.profiles) > 0 {
			dp.apply(nil, "")
		}
		return
	}
	if err := dp.applyRaw(doc.Config); err != nil {
		dp.logComp.Debugf("agent telemetry dynamic profiles: peer-cached document rejected: %v", err)
	}
}

// adoptNewerFromDisk publishes the document a peer left on disk after winning the write race.
func (dp *dynamicProfilesManager) adoptNewerFromDisk() {
	doc, err := readCachedDocument(dp.cachePath)
	if err != nil || doc == nil {
		dp.logComp.Debugf("agent telemetry dynamic profiles: superseded but peer document unreadable: %v", err)
		return
	}

	dp.logComp.Debugf("agent telemetry dynamic profiles: a peer fetched a newer document, adopting it")
	dp.adoptCached(doc)
}

// applyRaw parses, filters and publishes a raw document. A document identical to the one already published is a no-op,
// so a poll that returns unchanged content does not churn cron entries.
func (dp *dynamicProfilesManager) applyRaw(raw string) error {
	if cur := dp.load(); cur != nil && cur.raw == raw {
		dp.logComp.Debugf("agent telemetry dynamic profiles: document unchanged, keeping %d profiles", len(cur.profiles))
		return nil
	}

	cfg, err := parseProfilesDocument([]byte(raw))
	if err != nil {
		return err
	}

	filtered, err := filterDynamicProfiles(dp.a.atelCfg, cfg, dp.logComp)
	if err != nil {
		return err
	}

	dp.apply(filtered, raw)
	return nil
}

// apply swaps the dynamic-profiles job set and publishes the new snapshot.
//
// A nil cfg clears the dynamic profiles.
//
// An in-flight run cannot observe a partial swap: cron stores the job by value, so a running job holds the profiles
// slice captured when it was added, and removeJob only unregisters future firings. The compiled Profile values
// published here are freshly allocated and never rewritten afterwards, so the atomic store is what makes their
// compiled maps safely visible to reader goroutines.
func (dp *dynamicProfilesManager) apply(cfg *Config, raw string) {
	dp.mu.Lock()
	defer dp.mu.Unlock()

	for _, id := range dp.jobIDs {
		dp.a.runner.removeJob(id)
	}
	dp.jobIDs = nil

	var profiles []*Profile
	var events map[string]*Event
	if cfg != nil {
		profiles = cfg.Profiles
		events = cfg.events
		for sh, pp := range cfg.schedule {
			dp.jobIDs = append(dp.jobIDs, dp.a.runner.addJob(job{
				a:        dp.a,
				kind:     jobKindMetrics,
				profiles: pp,
				schedule: sh,
			}))
		}
	}

	dp.snapshot.Store(&profilesSnapshot{profiles: profiles, events: events, raw: raw})

	dp.logComp.Debugf("agent telemetry dynamic profiles: applied %d profiles across %d jobs",
		len(profiles), len(dp.jobIDs))
}

// buildHeaders builds the request headers. OS, architecture, version and flavor exist so the endpoint can target a
// subset of the fleet.
func (dp *dynamicProfilesManager) buildHeaders(doc *cachedDocument, attempt *attemptRecord) map[string]string {
	agentVersion := version.AgentVersion

	headers := map[string]string{
		"User-Agent":       fmt.Sprintf("datadog-agent/%s (%s)", agentVersion, runtime.Version()),
		"Accept":           "application/yaml, text/yaml",
		"DD-Agent-Version": agentVersion,
		"DD-Agent-Flavor":  normalizedFlavor(),
		"DD-Agent-OS":      runtime.GOOS,
		"DD-Agent-Arch":    runtime.GOARCH,
	}

	if doc != nil && doc.ETag != "" {
		headers["If-None-Match"] = doc.ETag
	}

	// Report the previous poll's failure so the endpoint can track breakage of the mechanism itself. Prefer the
	// persisted record (which may come from a peer process or a previous boot) and fall back to this process's own
	// memory when the disk write itself failed.
	last := dp.lastError.Load()
	if attempt != nil && attempt.LastError != nil {
		last = attempt.LastError
	}
	if last != nil {
		headers["DD-Agent-Telemetry-Dynamic-Profiles-Last-Error"] = fmt.Sprintf("stage=%s;flavor=%s;at=%d",
			last.Stage, last.Flavor, last.At)
	}

	return headers
}

// recordFailure notes a failed attempt. Debug only: this mechanism is additive and must never produce a user-visible
// error.
func (dp *dynamicProfilesManager) recordFailure(stage string, cause error) {
	dp.logComp.Debugf("agent telemetry dynamic profiles: poll failed (%s): %v", stage, causeOf(cause))

	now := dp.nowFn()
	record := &failureRecord{Stage: stage, At: now.Unix(), Flavor: normalizedFlavor()}
	dp.lastError.Store(record)

	// Only the attempt record: a failure must never be able to rewrite the document.
	if err := writeAttemptRecord(dp.attemptPath, &attemptRecord{
		AttemptedAtNS: now.UnixNano(),
		LastError:     record,
	}); err != nil {
		dp.logComp.Debugf("agent telemetry dynamic profiles: failed to persist attempt record: %v", err)
	}
}

// recordAttempt notes a successful poll that carried no new document -- a 304. It advances the gate and clears any
// recorded failure, leaving the stored document alone.
func (dp *dynamicProfilesManager) recordAttempt(now time.Time) {
	dp.lastError.Store(nil)
	dp.persist(now, nil, "")
}

// recordFetched notes a successful poll that produced a document, storing it. An empty raw is how a withdrawal
// (204/404) clears the cache.
func (dp *dynamicProfilesManager) recordFetched(raw string, etag string, now time.Time) {
	dp.lastError.Store(nil)
	dp.persist(now, &raw, etag)
}

// persist writes the document (when there is one) and then the attempt record.
//
// That order matters: crashing between the two leaves a good document with a stale attempted_at, costing one extra
// request next boot. The reverse would leave a fresh attempted_at with no document, which gates every process on the
// host into collecting nothing for a full poll interval.
func (dp *dynamicProfilesManager) persist(now time.Time, raw *string, etag string) {
	var err error

	if raw != nil {
		err = writeCachedDocument(dp.cachePath, &cachedDocument{
			FetchedAtNS: now.UnixNano(),
			ETag:        etag,
			Config:      *raw,
		})
		if errors.Is(err, errDocumentSuperseded) {
			// A peer fetched a newer document while we were parsing ours. Theirs is on disk and is what every other
			// process will adopt, so converge on it now instead of collecting our already-stale document until the
			// next poll. Not a failure: the cache holds the right thing.
			err = nil
			dp.adoptNewerFromDisk()
		}
	}
	if err == nil {
		err = writeAttemptRecord(dp.attemptPath, &attemptRecord{AttemptedAtNS: now.UnixNano()})
	}

	if err != nil {
		dp.logComp.Debugf("agent telemetry dynamic profiles: failed to persist cache: %v", err)
		dp.lastError.Store(&failureRecord{Stage: stageCacheWrite, At: now.Unix(), Flavor: normalizedFlavor()})
	}
}
