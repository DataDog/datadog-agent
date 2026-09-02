// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttelemetryimpl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v2"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

const testDynamicProfilesDoc = `
profiles:
  - name: dynamic-one
    metric:
      exclude:
        zero_metric: true
      metrics:
        - name: mygroup.mymetric
          preserve_tags:
            - mytag
    schedule:
      start_after: 10
      iterations: 0
      period: 60
`

const testDynamicProfilesDocTwoProfiles = testDynamicProfilesDoc + `
  - name: dynamic-two
    metric:
      metrics:
        - name: othergroup.othermetric
          aggregate_total: true
    schedule:
      start_after: 20
      iterations: 0
      period: 120
`

// getTestDynamicProfilesAtel builds an atel whose dynamic profiles manager points at serverURL and can
// actually reach it (createAtel deliberately leaves the fetcher nil).
func getTestDynamicProfilesAtel(t *testing.T, serverURL string) (*atel, *runnerMock) {
	t.Helper()

	yamlCfg := fmt.Sprintf(`
site: datadoghq.com
agent_telemetry:
  enabled: true
  dynamic_profiles:
    enabled: true
    url: %s
    poll_interval_seconds: 300
    startup_jitter_seconds: 0
`, serverURL)

	r := newRunnerMock()
	a := getTestAtel(t, nil, yamlCfg, nil, nil, r)
	require.NotNil(t, a.dynamicProfiles)
	a.dynamicProfiles.httpClient = &http.Client{Timeout: 5 * time.Second}

	return a, r.(*runnerMock)
}

// -------------------------------------------------------------------------
// parseProfilesDocument
// -------------------------------------------------------------------------

func TestParseDynamicProfilesDocument_Valid(t *testing.T) {
	cfg, err := parseProfilesDocument([]byte(testDynamicProfilesDocTwoProfiles))
	require.NoError(t, err)

	require.Len(t, cfg.Profiles, 2)
	assert.Equal(t, "dynamic-one", cfg.Profiles[0].Name)
	assert.True(t, cfg.Profiles[0].excludeZeroMetric)
	require.Contains(t, cfg.Profiles[0].metricsMap, "mygroup_mymetric")
	assert.True(t, cfg.Profiles[0].metricsMap["mygroup_mymetric"].preserveTagsExists)

	// One compiled schedule per distinct schedule block.
	assert.Len(t, cfg.schedule, 2)
	assert.Contains(t, cfg.schedule, Schedule{StartAfter: 10, Iterations: 0, Period: 60})
	assert.Contains(t, cfg.schedule, Schedule{StartAfter: 20, Iterations: 0, Period: 120})
}

func TestParseDynamicProfilesDocument_EventsAreRegistered(t *testing.T) {
	cfg, err := parseProfilesDocument([]byte(`
profiles:
  - name: dynamic-events
    events:
      - name: myevent
        request_type: my-event
        payload_key: my_event
        message: 'My Event'
`))
	require.NoError(t, err)
	require.Contains(t, cfg.events, "myevent")
	assert.Equal(t, "my-event", cfg.events["myevent"].RequestType)
}

func TestParseDynamicProfilesDocument_ScheduleDefaultsApplied(t *testing.T) {
	cfg, err := parseProfilesDocument([]byte(`
profiles:
  - name: dynamic-nosched
    metric:
      metrics:
        - name: mygroup.mymetric
`))
	require.NoError(t, err)
	assert.Contains(t, cfg.schedule, Schedule{
		StartAfter: defaultSheduleStartAfter,
		Iterations: 0,
		Period:     defaultShedulePeriod,
	})
}

func TestParseDynamicProfilesDocument_Rejects(t *testing.T) {
	tests := []struct {
		name  string
		doc   string
		stage string
	}{
		{
			// A remote document must never be able to turn agent telemetry off.
			name:  "enabled key",
			doc:   "enabled: false\nprofiles:\n  - name: p\n",
			stage: stageYAML,
		},
		{
			// ... nor change trace sampling.
			name:  "startup_trace_sampling key",
			doc:   "startup_trace_sampling: 1.0\nprofiles:\n  - name: p\n",
			stage: stageYAML,
		},
		{
			name:  "unknown top-level key",
			doc:   "surprise: 1\nprofiles:\n  - name: p\n",
			stage: stageYAML,
		},
		{
			name:  "unknown profile key",
			doc:   "profiles:\n  - name: p\n    surprise: 1\n",
			stage: stageYAML,
		},
		{
			name:  "unknown metric key",
			doc:   "profiles:\n  - name: p\n    metric:\n      metrics:\n        - name: a.b\n          surprise: 1\n",
			stage: stageYAML,
		},
		{
			name:  "duplicate key",
			doc:   "profiles:\n  - name: p\n    name: q\n",
			stage: stageYAML,
		},
		{
			name:  "not yaml",
			doc:   "\tthis: [is: not\n",
			stage: stageYAML,
		},
		{
			name:  "two documents",
			doc:   "profiles:\n  - name: p\n    metric:\n      metrics:\n        - name: a.b\n---\nprofiles: []\n",
			stage: stageMultiDoc,
		},
		{
			name:  "empty body",
			doc:   "",
			stage: stageEmpty,
		},
		{
			name:  "empty profiles list",
			doc:   "profiles: []\n",
			stage: stageEmpty,
		},
		{
			name:  "null profiles",
			doc:   "profiles:\n",
			stage: stageEmpty,
		},
		{
			name:  "null profile entry",
			doc:   "profiles:\n  -\n",
			stage: stageCompile,
		},
		{
			name:  "profile without name",
			doc:   "profiles:\n  - metric:\n      metrics:\n        - name: a.b\n",
			stage: stageCompile,
		},
		{
			name:  "metric name without group",
			doc:   "profiles:\n  - name: p\n    metric:\n      metrics:\n        - name: nogroup\n",
			stage: stageCompile,
		},
		{
			name:  "empty exclude block",
			doc:   "profiles:\n  - name: p\n    metric:\n      exclude: {}\n      metrics:\n        - name: a.b\n",
			stage: stageCompile,
		},
		{
			name: "aggregate_total with reserved total tag",
			doc: "profiles:\n  - name: p\n    metric:\n      metrics:\n        - name: a.b\n" +
				"          aggregate_total: true\n          preserve_tags:\n            - total\n",
			stage: stageCompile,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := parseProfilesDocument([]byte(tc.doc))
			require.Error(t, err)
			assert.Nil(t, cfg)
			assert.Equal(t, tc.stage, stageOf(err), "error was: %v", err)
		})
	}
}

func TestParseDynamicProfilesDocument_Caps(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("profiles:\n")
	for i := 0; i <= dynamicProfilesMaxProfiles; i++ {
		fmt.Fprintf(&sb, "  - name: p%d\n    metric:\n      metrics:\n        - name: g.m%d\n", i, i)
	}
	_, err := parseProfilesDocument([]byte(sb.String()))
	require.Error(t, err)
	assert.Equal(t, stageTooManyProfiles, stageOf(err))

	sb.Reset()
	sb.WriteString("profiles:\n  - name: p\n    metric:\n      metrics:\n")
	for i := 0; i <= dynamicProfilesMaxMetrics; i++ {
		fmt.Fprintf(&sb, "        - name: g.m%d\n", i)
	}
	_, err = parseProfilesDocument([]byte(sb.String()))
	require.Error(t, err)
	assert.Equal(t, stageTooManyMetrics, stageOf(err))

	_, err = parseProfilesDocument(make([]byte, dynamicProfilesMaxBodyBytes+1))
	require.Error(t, err)
	assert.Equal(t, stageTooLarge, stageOf(err))
}

// -------------------------------------------------------------------------
// filterDynamicProfiles
// -------------------------------------------------------------------------

func mustParseDynamicProfiles(t *testing.T, doc string) *Config {
	t.Helper()
	cfg, err := parseProfilesDocument([]byte(doc))
	require.NoError(t, err)
	return cfg
}

func baselineConfig(t *testing.T) *Config {
	t.Helper()
	var cfg Config
	require.NoError(t, yaml.Unmarshal([]byte(defaultProfiles), &cfg))
	require.NoError(t, compileConfig(&cfg))
	return &cfg
}

func mustFilter(t *testing.T, baseline, dynamic *Config) *Config {
	t.Helper()
	filtered, err := filterDynamicProfiles(baseline, dynamic, logmock.New(t))
	require.NoError(t, err)
	return filtered
}

func TestFilterDynamicProfiles_DropsCollidingProfileName(t *testing.T) {
	baseline := baselineConfig(t)
	dynamic := mustParseDynamicProfiles(t, `
profiles:
  - name: checks
    metric:
      metrics:
        - name: brandnew.metric
  - name: dynamic-one
    metric:
      metrics:
        - name: mygroup.mymetric
`)

	filtered := mustFilter(t, baseline, dynamic)
	require.Len(t, filtered.Profiles, 1)
	assert.Equal(t, "dynamic-one", filtered.Profiles[0].Name)
}

func TestFilterDynamicProfiles_DropsCollidingMetricName(t *testing.T) {
	baseline := baselineConfig(t)
	dynamic := mustParseDynamicProfiles(t, `
profiles:
  - name: dynamic-one
    metric:
      metrics:
        - name: checks.execution_time
        - name: mygroup.mymetric
`)

	filtered := mustFilter(t, baseline, dynamic)
	require.Len(t, filtered.Profiles, 1)
	require.Len(t, filtered.Profiles[0].Metric.Metrics, 1)
	assert.Equal(t, "mygroup.mymetric", filtered.Profiles[0].Metric.Metrics[0].Name)
	// Recompiled, so the map matches the surviving metric only.
	assert.Len(t, filtered.Profiles[0].metricsMap, 1)
	assert.Contains(t, filtered.Profiles[0].metricsMap, "mygroup_mymetric")
}

func TestFilterDynamicProfiles_DropsProfileEmptiedByMetricDedup(t *testing.T) {
	baseline := baselineConfig(t)
	dynamic := mustParseDynamicProfiles(t, `
profiles:
  - name: dynamic-one
    metric:
      metrics:
        - name: checks.execution_time
`)

	filtered := mustFilter(t, baseline, dynamic)
	assert.Empty(t, filtered.Profiles)
	assert.Empty(t, filtered.schedule)
}

func TestFilterDynamicProfiles_DropsCollidingEventKeepsProfile(t *testing.T) {
	baseline := baselineConfig(t)
	require.Contains(t, baseline.events, "agentbsod")

	dynamic := mustParseDynamicProfiles(t, `
profiles:
  - name: dynamic-events
    metric:
      metrics:
        - name: mygroup.mymetric
    events:
      - name: agentbsod
        request_type: hijacked
        payload_key: hijacked
        message: 'nope'
      - name: brandnewevent
        request_type: brand-new
        payload_key: brand_new
        message: 'Brand New'
`)

	filtered := mustFilter(t, baseline, dynamic)
	require.Len(t, filtered.Profiles, 1)
	assert.NotContains(t, filtered.events, "agentbsod")
	assert.Contains(t, filtered.events, "brandnewevent")
}

func TestFilterDynamicProfiles_DedupsWithinDocument(t *testing.T) {
	baseline := baselineConfig(t)
	dynamic := mustParseDynamicProfiles(t, `
profiles:
  - name: dup
    metric:
      metrics:
        - name: mygroup.mymetric
  - name: dup
    metric:
      metrics:
        - name: othergroup.othermetric
`)

	filtered := mustFilter(t, baseline, dynamic)
	require.Len(t, filtered.Profiles, 1)
	assert.Equal(t, "mygroup.mymetric", filtered.Profiles[0].Metric.Metrics[0].Name)
}

// -------------------------------------------------------------------------
// disk cache
// -------------------------------------------------------------------------

// seedCache stands in for a prior successful fetch by this process or a peer.
func seedCache(t *testing.T, dp *dynamicProfilesManager, config, etag string, at time.Time) {
	t.Helper()
	require.NoError(t, writeCachedDocument(dp.cachePath, &cachedDocument{
		FetchedAtNS: at.UnixNano(),
		ETag:        etag,
		Config:      config,
	}))
	require.NoError(t, writeAttemptRecord(dp.attemptPath, &attemptRecord{
		AttemptedAtNS: at.UnixNano(),
	}))
}

func TestDynamicProfilesCache_MissingFileIsNotAnError(t *testing.T) {
	doc, err := readCachedDocument(t.TempDir() + "/absent.json")
	require.NoError(t, err)
	assert.Nil(t, doc)

	doc, err = readCachedDocument("")
	require.NoError(t, err)
	assert.Nil(t, doc)

	at, err := readAttemptRecord(t.TempDir() + "/absent-attempt.json")
	require.NoError(t, err)
	assert.Nil(t, at)
}

func TestDynamicProfilesCache_CorruptFileErrorsWithoutPanic(t *testing.T) {
	path := t.TempDir() + "/doc.json"
	require.NoError(t, os.WriteFile(path, []byte("{not json"), 0644))

	assert.NotPanics(t, func() {
		_, err := readCachedDocument(path)
		assert.Error(t, err)
	})
}

func TestDynamicProfilesCache_WrongVersionIsRejected(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(dir+"/doc.json", []byte(`{"version":999}`), 0644))
	require.NoError(t, os.WriteFile(dir+"/at.json", []byte(`{"version":999}`), 0644))

	_, err := readCachedDocument(dir + "/doc.json")
	assert.Error(t, err)
	_, err = readAttemptRecord(dir + "/at.json")
	assert.Error(t, err)
}

// The reason the document and the attempt record live in separate files: a failed
// attempt must not be able to rewrite the document. With both in one file this required
// a read-modify-write, and a failing process could serialize its stale snapshot of the
// document over a peer's freshly fetched one.
func TestDynamicProfilesCache_FailureCannotEraseDocument(t *testing.T) {
	a, _ := getTestDynamicProfilesAtel(t, "https://example.com/p.yaml")
	dp := a.dynamicProfiles

	// A peer fetched a document.
	require.NoError(t, writeCachedDocument(dp.cachePath, &cachedDocument{
		FetchedAtNS: 1000,
		ETag:        "etag-1",
		Config:      testDynamicProfilesDoc,
	}))

	// This process then records a failure.
	dp.recordFailure(stageNet, errors.New("boom"))

	doc, err := readCachedDocument(dp.cachePath)
	require.NoError(t, err)
	require.NotNil(t, doc, "the document file must still exist")
	assert.Equal(t, testDynamicProfilesDoc, doc.Config, "a failed attempt must not erase the document")
	assert.Equal(t, "etag-1", doc.ETag)
	assert.Equal(t, int64(1000), doc.FetchedAtNS)

	at, err := readAttemptRecord(dp.attemptPath)
	require.NoError(t, err)
	require.NotNil(t, at.LastError)
	assert.Equal(t, stageNet, at.LastError.Stage)
	assert.NotZero(t, at.AttemptedAtNS)
}

// Two processes can both pass the advisory poll gate and fetch. If the document changed between their two responses,
// the stale fetcher must not be able to leave its older document on disk just because it renamed last -- every process
// that adopts from disk or restarts would then collect the older document until the next poll.
func TestDynamicProfilesCache_StaleFetchDoesNotReplaceNewer(t *testing.T) {
	path := t.TempDir() + "/doc.json"

	fresh := time.Now()
	stale := fresh.Add(-5 * time.Millisecond) // same second, earlier fetch

	// The peer that fetched the newer document wins the race to disk.
	require.NoError(t, writeCachedDocument(path, &cachedDocument{
		FetchedAtNS: fresh.UnixNano(),
		ETag:        "etag-v2",
		Config:      testDynamicProfilesDocTwoProfiles,
	}))

	// The stale fetcher renames afterwards and must be refused.
	err := writeCachedDocument(path, &cachedDocument{
		FetchedAtNS: stale.UnixNano(),
		ETag:        "etag-v1",
		Config:      testDynamicProfilesDoc,
	})
	require.ErrorIs(t, err, errDocumentSuperseded)

	doc, err := readCachedDocument(path)
	require.NoError(t, err)
	assert.Equal(t, testDynamicProfilesDocTwoProfiles, doc.Config, "the newer document must survive")
	assert.Equal(t, "etag-v2", doc.ETag)
}

// The supersede check must run immediately before the rename, not before staging: staging includes an fsync, and a
// peer's rename landing in that gap would be silently overwritten. This drives a write that lands *during* staging.
func TestDynamicProfilesCache_PeerWinningDuringStagingIsRespected(t *testing.T) {
	path := t.TempDir() + "/doc.json"

	stale := time.Now()
	fresh := stale.Add(10 * time.Millisecond)

	// A peer lands the newer document while our write is staging its temp file. writeJSONAtomic's hook runs after
	// staging, so it must observe this.
	staged := make(chan struct{})
	go func() {
		<-staged
		_ = writeCachedDocument(path, &cachedDocument{
			FetchedAtNS: fresh.UnixNano(),
			ETag:        "etag-v2",
			Config:      testDynamicProfilesDocTwoProfiles,
		})
	}()

	err := writeJSONAtomic(path, &cachedDocument{
		Version:     dynamicProfilesCacheVersion,
		FetchedAtNS: stale.UnixNano(),
		ETag:        "etag-v1",
		Config:      testDynamicProfilesDoc,
	}, func() error {
		// Let the peer commit, then apply the same precondition the real writer uses.
		close(staged)
		for i := 0; i < 200; i++ {
			if existing, rerr := readCachedDocument(path); rerr == nil && existing != nil {
				if existing.FetchedAtNS > stale.UnixNano() {
					return errDocumentSuperseded
				}
			}
			time.Sleep(time.Millisecond)
		}
		return nil
	})
	require.ErrorIs(t, err, errDocumentSuperseded)

	doc, err := readCachedDocument(path)
	require.NoError(t, err)
	assert.Equal(t, testDynamicProfilesDocTwoProfiles, doc.Config, "the peer's newer document must survive")

	// The abandoned write leaves no temp file behind.
	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotContains(t, e.Name(), ".tmp")
	}
}

func TestDynamicProfilesCache_NewerFetchReplacesOlder(t *testing.T) {
	path := t.TempDir() + "/doc.json"
	older := time.Now()

	require.NoError(t, writeCachedDocument(path, &cachedDocument{
		FetchedAtNS: older.UnixNano(),
		Config:      testDynamicProfilesDoc,
	}))
	require.NoError(t, writeCachedDocument(path, &cachedDocument{
		FetchedAtNS: older.Add(time.Millisecond).UnixNano(),
		Config:      testDynamicProfilesDocTwoProfiles,
	}))

	doc, err := readCachedDocument(path)
	require.NoError(t, err)
	assert.Equal(t, testDynamicProfilesDocTwoProfiles, doc.Config)
}

// Equal timestamps are indistinguishable, so the write goes through rather than wedging.
func TestDynamicProfilesCache_EqualTimestampWriteIsAllowed(t *testing.T) {
	path := t.TempDir() + "/doc.json"
	at := time.Now().UnixNano()

	require.NoError(t, writeCachedDocument(path, &cachedDocument{
		FetchedAtNS: at, Config: testDynamicProfilesDoc,
	}))
	require.NoError(t, writeCachedDocument(path, &cachedDocument{
		FetchedAtNS: at, Config: testDynamicProfilesDocTwoProfiles,
	}))

	doc, err := readCachedDocument(path)
	require.NoError(t, err)
	assert.Equal(t, testDynamicProfilesDocTwoProfiles, doc.Config)
}

// A corrupt or foreign-version file must not block the write.
func TestDynamicProfilesCache_UnreadableExistingIsOverwritten(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"corrupt.json": "{not json",
		"oldver.json":  `{"version":1,"fetched_at":9999999999}`,
	} {
		path := dir + "/" + name
		require.NoError(t, os.WriteFile(path, []byte(content), 0644))
		require.NoError(t, writeCachedDocument(path, &cachedDocument{
			FetchedAtNS: time.Now().UnixNano(),
			Config:      testDynamicProfilesDoc,
		}), name)

		doc, err := readCachedDocument(path)
		require.NoError(t, err, name)
		assert.Equal(t, testDynamicProfilesDoc, doc.Config, name)
	}
}

// When a peer wins the race, the superseded process adopts the peer's document instead of collecting its own stale one
// until the next poll.
func TestDynamicProfiles_SupersededProcessAdoptsPeerDocument(t *testing.T) {
	a, r := getTestDynamicProfilesAtel(t, "https://example.com/p.yaml")
	dp := a.dynamicProfiles

	// This process fetched and applied the one-profile document.
	stale := time.Now()
	require.NoError(t, dp.applyRaw(testDynamicProfilesDoc))
	require.Len(t, r.liveJobs(), 1)

	// A peer then lands the newer two-profile document.
	require.NoError(t, writeCachedDocument(dp.cachePath, &cachedDocument{
		FetchedAtNS: stale.Add(10 * time.Millisecond).UnixNano(),
		ETag:        "etag-v2",
		Config:      testDynamicProfilesDocTwoProfiles,
	}))

	// Persisting our stale fetch is refused, and we converge on the peer's document.
	dp.recordFetched(testDynamicProfilesDoc, "etag-v1", stale)

	snapshot := dp.load()
	require.NotNil(t, snapshot)
	assert.Len(t, snapshot.profiles, 2, "must adopt the peer's newer document")
	assert.Len(t, r.liveJobs(), 2)

	// The document on disk is still the peer's...
	doc, err := readCachedDocument(dp.cachePath)
	require.NoError(t, err)
	assert.Equal(t, testDynamicProfilesDocTwoProfiles, doc.Config)

	// ... and the attempt record was still written, without being treated as a failure.
	at, err := readAttemptRecord(dp.attemptPath)
	require.NoError(t, err)
	require.NotNil(t, at)
	assert.NotZero(t, at.AttemptedAtNS)
	assert.Nil(t, at.LastError)
	assert.Nil(t, dp.lastError.Load(), "a supersede is not a cache-write failure")
}

func TestDynamicProfilesCache_AtomicWriteLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/doc.json"

	for i := 0; i < 3; i++ {
		require.NoError(t, writeCachedDocument(path, &cachedDocument{
			FetchedAtNS: int64(i),
			Config:      testDynamicProfilesDoc,
		}))
	}

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.Equal(t, []string{"doc.json"}, names, "temp files must not accumulate")
}

// Concurrent writers must never leave a reader looking at a torn or partial file: the
// rename is atomic, so a reader sees either the whole old file or the whole new one.
func TestDynamicProfilesCache_ConcurrentWritersNeverTear(t *testing.T) {
	path := t.TempDir() + "/doc.json"
	require.NoError(t, writeCachedDocument(path, &cachedDocument{Config: testDynamicProfilesDoc}))

	var writers sync.WaitGroup
	for w := 0; w < 4; w++ {
		writers.Add(1)
		go func(w int) {
			defer writers.Done()
			for i := 0; i < 50; i++ {
				_ = writeCachedDocument(path, &cachedDocument{
					FetchedAtNS: int64(w*1000 + i),
					Config:      testDynamicProfilesDoc,
				})
			}
		}(w)
	}

	stop := make(chan struct{})
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		for {
			select {
			case <-stop:
				return
			default:
			}
			doc, err := readCachedDocument(path)
			assert.NoError(t, err, "reader observed a torn file")
			if doc != nil {
				assert.Equal(t, testDynamicProfilesDoc, doc.Config)
			}
		}
	}()

	writers.Wait()
	close(stop)
	<-readerDone

	// And no temp files survive the storm.
	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotContains(t, e.Name(), ".tmp")
	}
}

func TestDynamicProfilesCache_EmptyPathIsANoop(t *testing.T) {
	assert.NoError(t, writeCachedDocument("", &cachedDocument{}))
	assert.NoError(t, writeAttemptRecord("", &attemptRecord{}))
}

// -------------------------------------------------------------------------
// headers and url validation
// -------------------------------------------------------------------------

func TestBuildHeaders(t *testing.T) {
	a, _ := getTestDynamicProfilesAtel(t, "https://example.com/o.yaml")
	dp := a.dynamicProfiles

	h := dp.buildHeaders(nil, nil)
	assert.NotEmpty(t, h["User-Agent"])
	assert.NotEmpty(t, h["DD-Agent-Version"])
	assert.NotEmpty(t, h["DD-Agent-OS"])
	assert.NotEmpty(t, h["DD-Agent-Arch"])
	assert.Equal(t, "application/yaml, text/yaml", h["Accept"])
	assert.NotEmpty(t, h["DD-Agent-Flavor"])
	assert.NotContains(t, h, "If-None-Match")
	assert.NotContains(t, h, "DD-Agent-Telemetry-Dynamic-Profiles-Last-Error")

	h = dp.buildHeaders(
		&cachedDocument{ETag: "etag-1"},
		&attemptRecord{LastError: &failureRecord{Stage: stageYAML, At: 1234, Flavor: "agent"}},
	)
	assert.Equal(t, "etag-1", h["If-None-Match"])
	assert.Equal(t, "stage=yaml;flavor=agent;at=1234", h["DD-Agent-Telemetry-Dynamic-Profiles-Last-Error"])
}

// The error header must never carry a formatted error: those can embed proxy
// hostnames, filesystem paths or resolver output.
// The reported flavor must use the same spelling as the mandatory "emitter" metric tag,
// so the endpoint sees one spelling per flavor rather than both "trace-agent" (emitter)
// and "trace_agent" (raw flavor.GetFlavor).
func TestBuildHeaders_FlavorMatchesEmitterConvention(t *testing.T) {
	flavor.SetTestFlavor(t, "trace_agent")

	a, _ := getTestDynamicProfilesAtel(t, "https://example.com/p.yaml")
	h := a.dynamicProfiles.buildHeaders(nil, nil)

	assert.Equal(t, "trace-agent", h["DD-Agent-Flavor"])
	assert.Equal(t, normalizedFlavor(), h["DD-Agent-Flavor"])
}

// The persisted failure record carries the same normalized flavor.
func TestRecordFailure_FlavorMatchesEmitterConvention(t *testing.T) {
	flavor.SetTestFlavor(t, "process_agent")

	a, _ := getTestDynamicProfilesAtel(t, "https://example.com/p.yaml")
	dp := a.dynamicProfiles
	dp.recordFailure(stageNet, errors.New("boom"))

	at, err := readAttemptRecord(dp.attemptPath)
	require.NoError(t, err)
	require.NotNil(t, at.LastError)
	assert.Equal(t, "process-agent", at.LastError.Flavor)
}

func TestBuildHeaders_ErrorHeaderNeverLeaksErrorText(t *testing.T) {
	a, _ := getTestDynamicProfilesAtel(t, "https://example.com/o.yaml")
	dp := a.dynamicProfiles

	dp.recordFailure(stageNet, errors.New(
		`Get "https://internal.corp/o.yaml": proxyconnect tcp: dial tcp 10.1.2.3:3128: connect: refused (/etc/dd/secret)`))

	at, err := readAttemptRecord(dp.attemptPath)
	require.NoError(t, err)
	value := dp.buildHeaders(nil, at)["DD-Agent-Telemetry-Dynamic-Profiles-Last-Error"]
	require.NotEmpty(t, value)

	for _, leak := range []string{"/", "dial", "internal.corp", "10.1.2.3", "proxyconnect", "secret"} {
		assert.NotContains(t, value, leak, "header %q leaked %q", value, leak)
	}
}

func TestValidateDynamicProfilesURL(t *testing.T) {
	for _, ok := range []string{
		"https://example.com/o.yaml",
		"https://example.com:8443/o.yaml",
		"http://127.0.0.1:8099/o.yaml",
		"http://localhost:8099/o.yaml",
		"http://[::1]:8099/o.yaml",
	} {
		_, err := validateProfilesURL(ok)
		assert.NoError(t, err, ok)
	}

	for _, bad := range []string{
		"http://example.com/o.yaml",
		"http://10.0.0.1/o.yaml",
		"ftp://example.com/o.yaml",
		"file:///etc/passwd",
		"not a url at all",
		"",
	} {
		_, err := validateProfilesURL(bad)
		assert.Error(t, err, bad)
	}
}

func TestDefaultDynamicProfilesURLIsValid(t *testing.T) {
	_, err := validateProfilesURL(defaultDynamicProfilesURL)
	assert.NoError(t, err)
}

// -------------------------------------------------------------------------
// poll / apply
// -------------------------------------------------------------------------

func newDynamicProfilesServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestDynamicProfiles_FetchAppliesJobs(t *testing.T) {
	srv, hits := newDynamicProfilesServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", "etag-1")
		fmt.Fprint(w, testDynamicProfilesDocTwoProfiles)
	})

	a, r := getTestDynamicProfilesAtel(t, srv.URL)
	a.dynamicProfiles.poll(context.Background())

	assert.Equal(t, int64(1), hits.Load())

	// One job per distinct dynamic-profiles schedule, all metric jobs.
	live := r.liveJobs()
	require.Len(t, live, 2)
	schedules := map[Schedule]bool{}
	for _, j := range live {
		assert.Equal(t, jobKindMetrics, j.kind)
		schedules[j.schedule] = true
	}
	assert.True(t, schedules[Schedule{StartAfter: 10, Iterations: 0, Period: 60}])
	assert.True(t, schedules[Schedule{StartAfter: 20, Iterations: 0, Period: 120}])

	snapshot := a.dynamicProfiles.load()
	require.NotNil(t, snapshot)
	assert.Len(t, snapshot.profiles, 2)

	// Persisted for the next process / next boot.
	doc, err := readCachedDocument(a.dynamicProfiles.cachePath)
	require.NoError(t, err)
	require.NotNil(t, doc)
	assert.Equal(t, testDynamicProfilesDocTwoProfiles, doc.Config)
	assert.Equal(t, "etag-1", doc.ETag)
	assert.NotZero(t, doc.FetchedAtNS)

	at, err := readAttemptRecord(a.dynamicProfiles.attemptPath)
	require.NoError(t, err)
	require.NotNil(t, at)
	assert.NotZero(t, at.AttemptedAtNS)
	assert.Nil(t, at.LastError)
}

func TestDynamicProfiles_ReplaceRemovesPreviousJobs(t *testing.T) {
	var doc atomic.Value
	doc.Store(testDynamicProfilesDocTwoProfiles)
	srv, _ := newDynamicProfilesServer(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, doc.Load().(string))
	})

	a, r := getTestDynamicProfilesAtel(t, srv.URL)
	a.dynamicProfiles.poll(context.Background())
	require.Len(t, r.liveJobs(), 2)

	// Second poll, different document. Force the poll past the cache gate.
	doc.Store(testDynamicProfilesDoc)
	a.dynamicProfiles.nowFn = func() time.Time { return time.Now().Add(2 * a.dynamicProfiles.pollInterval) }
	a.dynamicProfiles.poll(context.Background())

	live := r.liveJobs()
	require.Len(t, live, 1)
	assert.Equal(t, Schedule{StartAfter: 10, Iterations: 0, Period: 60}, live[0].schedule)
	// The two original jobs were explicitly unregistered rather than left to fire.
	assert.Len(t, r.removed, 2)

	snapshot := a.dynamicProfiles.load()
	require.NotNil(t, snapshot)
	require.Len(t, snapshot.profiles, 1)
	assert.Equal(t, "dynamic-one", snapshot.profiles[0].Name)
}

func TestDynamicProfiles_UnchangedDocumentDoesNotChurnJobs(t *testing.T) {
	srv, _ := newDynamicProfilesServer(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, testDynamicProfilesDoc)
	})

	a, r := getTestDynamicProfilesAtel(t, srv.URL)
	a.dynamicProfiles.poll(context.Background())
	require.Len(t, r.jobs, 1)

	a.dynamicProfiles.nowFn = func() time.Time { return time.Now().Add(2 * a.dynamicProfiles.pollInterval) }
	a.dynamicProfiles.poll(context.Background())

	assert.Len(t, r.jobs, 1, "identical document should not re-register jobs")
	assert.Empty(t, r.removed)
}

func TestDynamicProfiles_InvalidDocLeavesPreviousActive(t *testing.T) {
	var mode atomic.Value
	mode.Store("good")
	srv, _ := newDynamicProfilesServer(t, func(w http.ResponseWriter, _ *http.Request) {
		switch mode.Load().(string) {
		case "good":
			fmt.Fprint(w, testDynamicProfilesDoc)
		case "500":
			w.WriteHeader(http.StatusInternalServerError)
		case "garbage":
			fmt.Fprint(w, "profiles:\n  - name: p\n    surprise: 1\n")
		}
	})

	a, r := getTestDynamicProfilesAtel(t, srv.URL)
	dp := a.dynamicProfiles

	dp.poll(context.Background())
	require.Len(t, r.liveJobs(), 1)

	skew := time.Duration(0)
	dp.nowFn = func() time.Time { return time.Now().Add(skew) }

	for _, tc := range []struct{ mode, stage string }{
		{"500", "http_500"},
		{"garbage", stageYAML},
	} {
		mode.Store(tc.mode)
		skew += 2 * dp.pollInterval
		dp.poll(context.Background())

		assert.Len(t, r.liveJobs(), 1, "%s: previous dynamic profiles must keep collecting", tc.mode)
		require.NotNil(t, dp.load())
		assert.Len(t, dp.load().profiles, 1)

		at, err := readAttemptRecord(dp.attemptPath)
		require.NoError(t, err)
		require.NotNil(t, at.LastError)
		assert.Equal(t, tc.stage, at.LastError.Stage)

		// The document is still cached so the next boot can reuse it.
		doc, err := readCachedDocument(dp.cachePath)
		require.NoError(t, err)
		require.NotNil(t, doc)
		assert.Equal(t, testDynamicProfilesDoc, doc.Config)

		// ... and the next request reports the failure.
		assert.Equal(t, fmt.Sprintf("stage=%s;flavor=%s;at=%d", tc.stage, at.LastError.Flavor, at.LastError.At),
			dp.buildHeaders(doc, at)["DD-Agent-Telemetry-Dynamic-Profiles-Last-Error"])
	}
}

func TestDynamicProfiles_FreshCacheSkipsHTTP(t *testing.T) {
	srv, hits := newDynamicProfilesServer(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, testDynamicProfilesDocTwoProfiles)
	})

	a, r := getTestDynamicProfilesAtel(t, srv.URL)
	dp := a.dynamicProfiles

	// Stand in for a peer agent process on this host having just polled.
	now := time.Now()
	seedCache(t, dp, testDynamicProfilesDoc, "", now)

	dp.poll(context.Background())

	assert.Equal(t, int64(0), hits.Load(), "a fresh cache must not trigger a request")
	// The peer's document is adopted rather than ignored.
	require.Len(t, r.liveJobs(), 1)
	require.NotNil(t, dp.load())
	assert.Equal(t, "dynamic-one", dp.load().profiles[0].Name)
}

func TestDynamicProfiles_StaleCacheTriggersHTTP(t *testing.T) {
	srv, hits := newDynamicProfilesServer(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, testDynamicProfilesDocTwoProfiles)
	})

	a, r := getTestDynamicProfilesAtel(t, srv.URL)
	dp := a.dynamicProfiles

	stale := time.Now().Add(-2 * dp.pollInterval)
	seedCache(t, dp, testDynamicProfilesDoc, "", stale)

	dp.poll(context.Background())

	assert.Equal(t, int64(1), hits.Load())
	assert.Len(t, r.liveJobs(), 2)
}

// A failed attempt must also push out the next request, otherwise a permanently
// failing endpoint is retried by every restart of every agent process.
func TestDynamicProfiles_FailedAttemptGatesTheNextRequest(t *testing.T) {
	srv, hits := newDynamicProfilesServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	a, _ := getTestDynamicProfilesAtel(t, srv.URL)
	dp := a.dynamicProfiles

	dp.poll(context.Background())
	require.Equal(t, int64(1), hits.Load())

	dp.poll(context.Background())
	assert.Equal(t, int64(1), hits.Load(), "a recent failed attempt must gate the next request")
}

func TestDynamicProfiles_304KeepsCache(t *testing.T) {
	var sawIfNoneMatch atomic.Value
	sawIfNoneMatch.Store("")
	srv, hits := newDynamicProfilesServer(t, func(w http.ResponseWriter, r *http.Request) {
		sawIfNoneMatch.Store(r.Header.Get("If-None-Match"))
		w.WriteHeader(http.StatusNotModified)
	})

	a, r := getTestDynamicProfilesAtel(t, srv.URL)
	dp := a.dynamicProfiles

	stale := time.Now().Add(-2 * dp.pollInterval)
	seedCache(t, dp, testDynamicProfilesDoc, "etag-1", stale)
	dp.applyCachedAtStartup()
	require.Len(t, r.liveJobs(), 1)

	dp.poll(context.Background())

	assert.Equal(t, int64(1), hits.Load())
	assert.Equal(t, "etag-1", sawIfNoneMatch.Load())
	assert.Len(t, r.liveJobs(), 1, "304 must keep the cached dynamic profiles applied")

	doc, err := readCachedDocument(dp.cachePath)
	require.NoError(t, err)
	assert.Equal(t, testDynamicProfilesDoc, doc.Config, "304 must not clear the stored document")
	assert.Equal(t, "etag-1", doc.ETag)

	at, err := readAttemptRecord(dp.attemptPath)
	require.NoError(t, err)
	assert.Nil(t, at.LastError)
	assert.Greater(t, at.AttemptedAtNS, stale.UnixNano())
}

// 204/404 is how the endpoint withdraws the dynamic profiles.
func TestDynamicProfiles_AbsentClearsProfiles(t *testing.T) {
	for _, status := range []int{http.StatusNoContent, http.StatusNotFound} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			srv, _ := newDynamicProfilesServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
			})

			a, r := getTestDynamicProfilesAtel(t, srv.URL)
			dp := a.dynamicProfiles

			stale := time.Now().Add(-2 * dp.pollInterval)
			seedCache(t, dp, testDynamicProfilesDoc, "", stale)
			dp.applyCachedAtStartup()
			require.Len(t, r.liveJobs(), 1)

			dp.poll(context.Background())

			assert.Empty(t, r.liveJobs(), "withdrawn dynamic profiles must stop collecting")
			assert.Empty(t, dp.load().profiles)

			doc, err := readCachedDocument(dp.cachePath)
			require.NoError(t, err)
			require.NotNil(t, doc)
			assert.Empty(t, doc.Config)
		})
	}
}

func TestDynamicProfiles_ContextCancelDoesNotPersist(t *testing.T) {
	release := make(chan struct{})
	srv, _ := newDynamicProfilesServer(t, func(w http.ResponseWriter, _ *http.Request) {
		<-release
		fmt.Fprint(w, testDynamicProfilesDoc)
	})
	t.Cleanup(func() { close(release) })

	a, _ := getTestDynamicProfilesAtel(t, srv.URL)
	dp := a.dynamicProfiles

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	dp.poll(ctx)

	// A shutdown-cancelled request is self-inflicted; recording it would poison the
	// next boot's error header.
	at, err := readAttemptRecord(dp.attemptPath)
	require.NoError(t, err)
	if at != nil {
		assert.Nil(t, at.LastError)
	}
	assert.Nil(t, dp.lastError.Load())
}

func TestDynamicProfiles_NoHTTPClientIsANoop(t *testing.T) {
	r := newRunnerMock()
	a := getTestAtel(t, nil, `
site: datadoghq.com
agent_telemetry:
  enabled: true
`, nil, nil, r)

	require.NotNil(t, a.dynamicProfiles)
	require.Nil(t, a.dynamicProfiles.httpClient, "createAtel must not wire an http client")

	assert.NotPanics(t, func() { a.dynamicProfiles.poll(context.Background()) })
	assert.Nil(t, a.dynamicProfiles.load())
	doc, err := readCachedDocument(a.dynamicProfiles.cachePath)
	require.NoError(t, err)
	assert.Nil(t, doc, "a client-less poll must not touch the disk")
	at, err := readAttemptRecord(a.dynamicProfiles.attemptPath)
	require.NoError(t, err)
	assert.Nil(t, at, "a client-less poll must not touch the disk")
}

// -------------------------------------------------------------------------
// startup and lifecycle
// -------------------------------------------------------------------------

func TestDynamicProfiles_ApplyCachedAtStartup(t *testing.T) {
	a, r := getTestDynamicProfilesAtel(t, "https://example.com/o.yaml")
	dp := a.dynamicProfiles

	seedCache(t, dp, testDynamicProfilesDocTwoProfiles, "", time.Now())

	dp.applyCachedAtStartup()

	assert.Len(t, r.liveJobs(), 2)
	require.NotNil(t, dp.load())
	assert.Len(t, dp.load().profiles, 2)
}

func TestDynamicProfiles_ApplyCachedAtStartupRecordsUnparseableCache(t *testing.T) {
	a, r := getTestDynamicProfilesAtel(t, "https://example.com/o.yaml")
	dp := a.dynamicProfiles

	require.NoError(t, writeCachedDocument(dp.cachePath, &cachedDocument{
		FetchedAtNS: time.Now().UnixNano(),
		Config:      "profiles:\n  - name: p\n    from_a_newer_agent: true\n",
	}))

	assert.NotPanics(t, dp.applyCachedAtStartup)
	assert.Empty(t, r.liveJobs())
	assert.Nil(t, dp.load())
	require.NotNil(t, dp.lastError.Load())
	assert.Equal(t, stageYAML, dp.lastError.Load().Stage)
}

func TestStart_RegistersDynamicProfilesPollJob(t *testing.T) {
	r := newRunnerMock()
	a := getTestAtel(t, nil, getCommonYAMLConfig(true, "foo.bar"), nil, nil, r)
	require.NoError(t, a.start())

	rm := r.(*runnerMock)
	polls := 0
	for _, j := range rm.jobs {
		if j.kind == jobKindDynamicProfilesPoll {
			polls++
			assert.Equal(t, uint(defaultDynamicProfilesPollIntervalSeconds), j.schedule.Period)
			assert.Equal(t, uint(0), j.schedule.Iterations)
		}
	}
	assert.Equal(t, 1, polls)
}

func TestStart_NoPollJobWhenDisabled(t *testing.T) {
	r := newRunnerMock()
	a := getTestAtel(t, nil, `
site: foo.bar
agent_telemetry:
  enabled: true
  dynamic_profiles:
    enabled: false
`, nil, nil, r)

	assert.Nil(t, a.dynamicProfiles)
	require.NoError(t, a.start())

	for _, j := range r.(*runnerMock).jobs {
		assert.NotEqual(t, jobKindDynamicProfilesPoll, j.kind)
	}
}

func TestDynamicProfiles_PollIntervalIsFloored(t *testing.T) {
	a, _ := getTestDynamicProfilesAtel(t, "https://example.com/o.yaml")
	assert.Equal(t, dynamicProfilesMinPollSeconds*time.Second, a.dynamicProfiles.pollInterval)

	r := newRunnerMock()
	b := getTestAtel(t, nil, `
site: foo.bar
agent_telemetry:
  enabled: true
  dynamic_profiles:
    poll_interval_seconds: 1
`, nil, nil, r)
	require.NotNil(t, b.dynamicProfiles)
	assert.Equal(t, dynamicProfilesMinPollSeconds*time.Second, b.dynamicProfiles.pollInterval)
	assert.Equal(t, uint(dynamicProfilesMinPollSeconds), b.dynamicProfiles.pollSchedule().Period)
}

// -------------------------------------------------------------------------
// interaction with the rest of the component
// -------------------------------------------------------------------------

func TestDynamicProfiles_SendEventBaselineWins(t *testing.T) {
	a, _ := getTestDynamicProfilesAtel(t, "https://example.com/o.yaml")

	baselineEvent, ok := a.lookupEvent("agentbsod")
	require.True(t, ok)
	require.Equal(t, "agent-bsod", baselineEvent.RequestType)

	// A dynamic profile that tries to re-point an existing event: the colliding
	// event is dropped, and lookupEvent would prefer the baseline regardless.
	require.NoError(t, a.dynamicProfiles.applyRaw(`
profiles:
  - name: dynamic-events
    metric:
      metrics:
        - name: mygroup.mymetric
    events:
      - name: agentbsod
        request_type: hijacked
        payload_key: hijacked
        message: 'nope'
      - name: brandnewevent
        request_type: brand-new
        payload_key: brand_new
        message: 'Brand New'
`))

	got, ok := a.lookupEvent("agentbsod")
	require.True(t, ok)
	assert.Equal(t, "agent-bsod", got.RequestType, "baseline event must not be re-pointed")

	got, ok = a.lookupEvent("brandnewevent")
	require.True(t, ok)
	assert.Equal(t, "brand-new", got.RequestType)

	_, ok = a.lookupEvent("nosuchevent")
	assert.False(t, ok)
}

func TestDynamicProfiles_AllProfilesIncludesDynamic(t *testing.T) {
	a, _ := getTestDynamicProfilesAtel(t, "https://example.com/o.yaml")

	baselineCount := len(a.allProfiles())
	require.Equal(t, len(a.atelCfg.Profiles), baselineCount)

	require.NoError(t, a.dynamicProfiles.applyRaw(testDynamicProfilesDocTwoProfiles))

	all := a.allProfiles()
	assert.Len(t, all, baselineCount+2)
	// Baseline profiles keep their position and identity.
	assert.Equal(t, a.atelCfg.Profiles[0], all[0])
	assert.Equal(t, "dynamic-one", all[baselineCount].Name)
	assert.Equal(t, "dynamic-two", all[baselineCount+1].Name)

	// atelCfg itself must never be mutated.
	assert.Len(t, a.atelCfg.Profiles, baselineCount)
}

func TestDynamicProfiles_AllProfilesAndLookupEventWithoutManager(t *testing.T) {
	r := newRunnerMock()
	a := getTestAtel(t, nil, `
site: foo.bar
agent_telemetry:
  enabled: true
  dynamic_profiles:
    enabled: false
`, nil, nil, r)
	require.Nil(t, a.dynamicProfiles)

	assert.Equal(t, a.atelCfg.Profiles, a.allProfiles())
	_, ok := a.lookupEvent("agentbsod")
	assert.True(t, ok)
}

// Regression guard for the atelCfg write-once invariant: applying dynamic profiles
// concurrently with the readers must not race.
func TestDynamicProfiles_ConcurrentApplyAndReadersAreRaceFree(t *testing.T) {
	a, _ := getTestDynamicProfilesAtel(t, "https://example.com/o.yaml")
	a.runner = newRunnerImpl()
	a.runner.start()
	t.Cleanup(func() { <-a.runner.stop().Done() })
	a.cancelCtx, a.cancel = context.WithCancel(context.Background())
	t.Cleanup(a.cancel)

	docs := []string{testDynamicProfilesDoc, testDynamicProfilesDocTwoProfiles, ""}

	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		for i := 0; i < 30; i++ {
			doc := docs[i%len(docs)]
			if doc == "" {
				a.dynamicProfiles.apply(nil, "")
				continue
			}
			_ = a.dynamicProfiles.applyRaw(doc)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 30; i++ {
			_ = a.allProfiles()
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 30; i++ {
			_, _ = a.lookupEvent("agentbsod")
			_, _ = a.lookupEvent("brandnewevent")
		}
	}()

	wg.Wait()
}

func TestDynamicProfilesCachedDocumentJSONRoundTrip(t *testing.T) {
	inDoc := &cachedDocument{
		Version:     dynamicProfilesCacheVersion,
		FetchedAtNS: 1,
		ETag:        "e",
		Config:      testDynamicProfilesDoc,
	}
	raw, err := json.Marshal(inDoc)
	require.NoError(t, err)
	var outDoc cachedDocument
	require.NoError(t, json.Unmarshal(raw, &outDoc))
	assert.Equal(t, *inDoc, outDoc)

	inAt := &attemptRecord{
		Version:       dynamicProfilesCacheVersion,
		AttemptedAtNS: 2,
		LastError:     &failureRecord{Stage: stageCompile, At: 3, Flavor: "agent"},
	}
	raw, err = json.Marshal(inAt)
	require.NoError(t, err)
	var outAt attemptRecord
	require.NoError(t, json.Unmarshal(raw, &outAt))
	assert.Equal(t, *inAt, outAt)
}
