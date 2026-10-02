// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package yara

import (
	"crypto/sha256"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// all returns a copy of the reports recorded so far (recordingReporter is in standin_test.go)
func (r *recordingReporter) all() []report {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]report(nil), r.reports...)
}

// testPipelineConfig returns a valid configuration loading the rules of rulesDir
func testPipelineConfig(rulesDir string) *Config {
	return &Config{
		Enabled:           true,
		RulesDir:          rulesDir,
		ChanSize:          10,
		Workers:           2,
		QueueSize:         16,
		MaxFileSize:       1 << 20,
		ScanTimeout:       5 * time.Second,
		IdentityCacheSize: 100,
		RecheckTTL:        time.Hour,
	}
}

// markerRulesDir returns a rules directory holding testRule, which matches EVIL_MARKER
func markerRulesDir(t *testing.T) string {
	t.Helper()
	trustCurrentUser(t)
	dir := t.TempDir()
	writeRuleFiles(t, dir, map[string]string{"marker.yar": testRule})
	return dir
}

// feed sends f through the consumer, as the event monitor would after Copy()
func feed(c *ExecConsumer, f ExecFile) {
	c.HandleEvent(&execFiles{main: f})
}

func TestPipelineEndToEnd(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	client := newRecordingClient()
	recorder := &recordingReporter{}
	p, err := NewPipeline(testPipelineConfig(markerRulesDir(t)), client, PipelineOpts{
		Compiler:        StandInCompiler,
		MetricsInterval: time.Hour, // only the last flush, on Stop
		ExtraReporter:   recorder,
	})
	require.NoError(t, err)
	assert.Len(t, p.RulesVersion(), 64)

	c := p.Consumer()
	require.NoError(t, c.Start())

	dir := t.TempDir()
	evil := []byte("\x7fELF padding EVIL_MARKER padding")
	evilPath := writeTempFile(t, dir, "evil", evil)
	cleanPath := writeTempFile(t, dir, "clean", []byte("\x7fELF nothing to see"))
	copyPath := writeTempFile(t, dir, "evil-copy", evil)

	// the same file executed 50 times, a clean file, and a copy of the first file at another path
	const execs = 50
	for i := 0; i < execs; i++ {
		feed(c, scriptExec(t, evilPath))
	}
	feed(c, scriptExec(t, cleanPath))
	feed(c, scriptExec(t, copyPath))

	require.Eventually(t, func() bool { return len(recorder.all()) == 2 }, 10*time.Second, 5*time.Millisecond)
	c.Stop()

	var matched, clean []report
	for _, r := range recorder.all() {
		require.NoError(t, r.err)
		if len(r.matches) > 0 {
			matched = append(matched, r)
		} else {
			clean = append(clean, r)
		}
	}
	require.Len(t, matched, 1, "one report with a match")
	assert.Equal(t, evilPath, matched[0].file.Path)
	assert.Equal(t, sha256.Sum256(evil), matched[0].sum)
	assert.Equal(t, []Match{{Rule: "evil_marker", Namespace: "standin"}}, matched[0].matches)
	require.Len(t, clean, 1)
	assert.Equal(t, cleanPath, clean[0].file.Path)

	stats := p.Stats()
	assert.EqualValues(t, execs+2, stats.ExecsReceived.Load())
	assert.EqualValues(t, execs-1, stats.IdentityHits.Load(), "the same file is read once")
	assert.EqualValues(t, 3, stats.Reads.Load())
	assert.EqualValues(t, 1, stats.ShaHits.Load(), "the same content is scanned once")
	assert.EqualValues(t, 2, stats.Scans.Load())
	assert.EqualValues(t, 1, stats.Matches.Load())

	// the last flush on Stop sent every counter
	counts, _ := client.takeCounts()
	assert.EqualValues(t, execs+2, counts[metricExecsReceived])
	assert.EqualValues(t, execs-1, counts[metricIdentityHits])
	assert.EqualValues(t, 3, counts[metricReads])
	assert.EqualValues(t, 1, counts[metricShaHits])
	assert.EqualValues(t, 2, counts[metricScans])
	assert.EqualValues(t, 1, counts[metricMatches])
	identities, ok := client.gauge(metricIdentityCacheSize)
	require.True(t, ok)
	assert.EqualValues(t, 3, identities)
	hashes, ok := client.gauge(metricShaSetSize)
	require.True(t, ok)
	assert.EqualValues(t, 2, hashes)
	_, ok = client.gauge(metricQueueDepth)
	assert.True(t, ok)

	client.mu.Lock()
	assert.Len(t, client.distributions[metricScanDuration], 2, "the scan duration hook is installed")
	client.mu.Unlock()
}

func TestPipelineMainBinaryThroughProcExe(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// no size limit: the test binary is large
	cfg := testPipelineConfig(markerRulesDir(t))
	cfg.MaxFileSize = 0
	recorder := &recordingReporter{}
	p, err := NewPipeline(cfg, nil, PipelineOpts{
		Compiler:      StandInCompiler,
		ExtraReporter: recorder,
	})
	require.NoError(t, err)
	c := p.Consumer()
	require.NoError(t, c.Start())
	defer c.Stop()

	// the test binary itself, opened through /proc/<pid>/exe as for a real exec. Its identity
	// is not checked there, so a fake one is fine.
	exe, err := os.Executable()
	require.NoError(t, err)
	content, err := os.ReadFile(exe)
	require.NoError(t, err)
	feed(c, ExecFile{PID: uint32(os.Getpid()), Path: exe, Inode: 1, CTime: 1})

	require.Eventually(t, func() bool { return len(recorder.all()) == 1 }, 10*time.Second, 5*time.Millisecond)
	assert.Equal(t, sha256.Sum256(content), recorder.all()[0].sum)
	assert.EqualValues(t, 1, p.Stats().Reads.Load())
}

func TestPipelineRulesErrors(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	trustCurrentUser(t)

	_, err := NewPipeline(testPipelineConfig(""), nil, PipelineOpts{Compiler: StandInCompiler})
	assert.ErrorIs(t, err, ErrRulesDirUnset)

	_, err = NewPipeline(testPipelineConfig(t.TempDir()), nil, PipelineOpts{Compiler: StandInCompiler})
	assert.ErrorIs(t, err, ErrNoRules)

	bad := t.TempDir()
	writeRuleFiles(t, bad, map[string]string{"bad.yar": "rule no_string { condition: true }"})
	_, err = NewPipeline(testPipelineConfig(bad), nil, PipelineOpts{Compiler: StandInCompiler})
	assert.ErrorContains(t, err, "failed to compile")

	unsafe := markerRulesDir(t)
	setTrustedRuleOwner(t, uint32(os.Getuid())+1)
	_, err = NewPipeline(testPipelineConfig(unsafe), nil, PipelineOpts{Compiler: StandInCompiler})
	assert.ErrorIs(t, err, ErrUnsafeRules)
}

func TestPipelineDefaultCompiler(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	p, err := NewPipeline(testPipelineConfig(markerRulesDir(t)), nil, PipelineOpts{})
	require.NoError(t, err, "the default compiler of this build compiles the test rule")
	p.Stop()
}

func TestPipelineInvalidConfigClosesScanner(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	var scanner *closingScanner
	compile := func([]RuleSource) (Scanner, error) {
		scanner = &closingScanner{MarkerScanner: NewMarkerScanner("m", "r")}
		return scanner, nil
	}

	_, err := NewPipeline(nil, nil, PipelineOpts{Compiler: compile})
	assert.Error(t, err)

	for name, mutate := range map[string]func(*Config){
		"workers":             func(c *Config) { c.Workers = 0 },
		"queue size":          func(c *Config) { c.QueueSize = -1 },
		"identity cache size": func(c *Config) { c.IdentityCacheSize = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			cfg := testPipelineConfig(markerRulesDir(t))
			mutate(cfg)
			_, err := NewPipeline(cfg, nil, PipelineOpts{Compiler: compile})
			assert.Error(t, err)
			require.NotNil(t, scanner)
			assert.True(t, scanner.closed, "the compiled rules are freed")
		})
	}
}

func TestPipelineStartStop(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	var scanner *closingScanner
	p, err := NewPipeline(testPipelineConfig(markerRulesDir(t)), nil, PipelineOpts{
		Compiler: func([]RuleSource) (Scanner, error) {
			scanner = &closingScanner{MarkerScanner: NewMarkerScanner("m", "r")}
			return scanner, nil
		},
	})
	require.NoError(t, err)

	c := p.Consumer()
	require.NoError(t, c.Start())
	require.NoError(t, c.Start(), "a second Start is a no-op")
	c.Stop()
	assert.True(t, scanner.closed, "Stop frees the compiled rules")
	c.Stop()
	require.NoError(t, c.Start(), "Start after Stop is a no-op")

	// Stop without Start
	p, err = NewPipeline(testPipelineConfig(markerRulesDir(t)), nil, PipelineOpts{Compiler: StandInCompiler})
	require.NoError(t, err)
	p.Stop()
}

func TestPipelineStopDropsQueuedScans(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	recorder := &recordingReporter{}
	p, err := NewPipeline(testPipelineConfig(markerRulesDir(t)), nil, PipelineOpts{
		Compiler:      StandInCompiler,
		ExtraReporter: recorder,
	})
	require.NoError(t, err)

	// never started: the scans wait in the queue, and Stop drops them
	dir := t.TempDir()
	for i := 0; i < 3; i++ {
		path := writeTempFile(t, dir, "bin"+string(rune('0'+i)), []byte{byte(i)})
		feed(p.Consumer(), scriptExec(t, path))
	}
	assert.Equal(t, 3, p.pool.QueueDepth())
	p.Stop()
	assert.Empty(t, recorder.all())
	_, hashes := p.deduper.Sizes()
	assert.Equal(t, 0, hashes, "dropped scans release their hash")
}

func TestTeeReporter(t *testing.T) {
	a, b := &recordingReporter{}, &recordingReporter{}
	scanErr := errors.New("boom")
	teeReporter{a, b}.Report(ExecFile{Path: "/x"}, [32]byte{1}, nil, scanErr)
	for _, r := range []*recordingReporter{a, b} {
		require.Len(t, r.all(), 1)
		assert.Equal(t, "/x", r.all()[0].file.Path)
		assert.Equal(t, scanErr, r.all()[0].err)
	}
}

func TestContainerPIDsFromEventMonitorWithoutProbe(t *testing.T) {
	assert.Nil(t, containerPIDsFromEventMonitor(nil))
}
