// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package coredump

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const gib = 1 << 30

var linuxSubs = map[byte]string{'e': "agent", 'E': "!opt!datadog-agent!bin!agent!agent"}

// fakeEnv returns a sysEnv that records the RLIMIT_CORE values it receives.
func fakeEnv(free uint64, limits *[][2]uint64) sysEnv {
	now := time.Now()
	return sysEnv{
		readCorePattern: func() (string, error) { return "", nil },
		freeBytes:       func(string) (uint64, error) { return free, nil },
		procName:        func() string { return "agent" },
		exePath:         func() (string, error) { return "/opt/datadog-agent/bin/agent/agent", nil },
		getwd:           func() (string, error) { return "/", nil },
		now:             func() time.Time { return now },
		setCoreLimit: func(cur, maxLimit uint64) error {
			*limits = append(*limits, [2]uint64{cur, maxLimit})
			return nil
		},
		getCoreLimit: func() (uint64, uint64, error) { return 0, 0, errors.New("unused") },
	}
}

func writeFile(t *testing.T, path string, age time.Duration) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte("core"), 0o600))
	mtime := time.Now().Add(-age)
	require.NoError(t, os.Chtimes(path, mtime, mtime))
}

func testConfig(dir string) boundedConfig {
	return boundedConfig{dir: dir, maxSize: 3 * gib, minFreeDisk: 10 * gib, maxAge: 72 * time.Hour}
}

func TestParseCorePattern(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		cwd        string
		pipe       bool
		inDir      bool
		perBinary  bool
		dir        string
		matches    []string
		nonMatches []string
	}{
		{
			name:       "plain",
			raw:        "/var/crash/core",
			inDir:      true,
			dir:        "/var/crash",
			matches:    []string{"core", "core.1234"},
			nonMatches: []string{"core.agent", "xcore", "core.12a"},
		},
		{
			name:       "with %e and %p",
			raw:        "/var/crash/core.%e.%p",
			inDir:      true,
			perBinary:  true,
			dir:        "/var/crash",
			matches:    []string{"core.agent.1234"},
			nonMatches: []string{"core.process-agent.1234", "core.trace-agent.1", "core.agent"},
		},
		{
			name:       "doc example %e-%p-%t",
			raw:        "/var/crash/core-%e-%p-%t\n",
			inDir:      true,
			perBinary:  true,
			dir:        "/var/crash",
			matches:    []string{"core-agent-12-1700000000"},
			nonMatches: []string{"core-process-agent-12-1700000000"},
		},
		{
			name:      "with %E",
			raw:       "/var/crash/%E.core",
			inDir:     true,
			perBinary: true,
			dir:       "/var/crash",
			matches:   []string{"!opt!datadog-agent!bin!agent!agent.core"},
			nonMatches: []string{
				"!opt!datadog-agent!embedded!bin!trace-agent.core",
			},
		},
		{
			name:       "literal percent and trailing percent",
			raw:        "/var/crash/core%%%e%",
			inDir:      true,
			perBinary:  true,
			dir:        "/var/crash",
			matches:    []string{"core%agent"},
			nonMatches: []string{"coreagent"},
		},
		{
			name: "pipe",
			raw:  "|/usr/lib/systemd/systemd-coredump %P %u %g %s %t %c %h",
			pipe: true,
		},
		{
			name:      "other directory",
			raw:       "/tmp/cores/core.%e",
			inDir:     false,
			perBinary: true,
			dir:       "/tmp/cores",
			matches:   []string{"core.agent"},
		},
		{
			name:    "relative pattern resolved against cwd",
			raw:     "core",
			cwd:     "/var/crash",
			inDir:   true,
			dir:     "/var/crash",
			matches: []string{"core"},
		},
		{
			name:  "relative pattern in other cwd",
			raw:   "core",
			cwd:   "/",
			inDir: false,
			dir:   "/",
		},
		{
			name:  "specifier in directory",
			raw:   "/var/crash/%e/core",
			inDir: false,
			dir:   "/var/crash/%e",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cwd := tt.cwd
			if cwd == "" {
				cwd = "/"
			}
			p := parseCorePattern(tt.raw, "/var/crash/", cwd, linuxSubs)
			assert.Equal(t, tt.pipe, p.pipe, "pipe")
			assert.Equal(t, tt.inDir, p.inDir, "inDir")
			assert.Equal(t, tt.perBinary, p.perBinary, "perBinary")
			assert.Equal(t, tt.dir, p.dir, "dir")
			require.NotNil(t, p.matcher)
			for _, m := range tt.matches {
				assert.True(t, p.matcher.MatchString(m), "%q should match %s", m, p.matcher)
			}
			for _, m := range tt.nonMatches {
				assert.False(t, p.matcher.MatchString(m), "%q should not match %s", m, p.matcher)
			}
		})
	}
}

func TestComputeLimitSizeCap(t *testing.T) {
	dir := t.TempDir()
	var limits [][2]uint64
	env := fakeEnv(100*gib, &limits)
	p := parseCorePattern(filepath.Join(dir, "core.%e.%p"), dir, "/", linuxSubs)

	limit, _ := computeLimit(testConfig(dir), p, env)
	assert.EqualValues(t, 3*gib, limit)

	applied, err := applyLimit(limit, 3*gib, env)
	require.NoError(t, err)
	assert.EqualValues(t, 3*gib, applied)
	assert.Equal(t, [][2]uint64{{3 * gib, 3 * gib}}, limits)
}

func TestComputeLimitLowDisk(t *testing.T) {
	dir := t.TempDir()
	var limits [][2]uint64
	env := fakeEnv(9*gib, &limits)
	p := parseCorePattern(filepath.Join(dir, "core.%e.%p"), dir, "/", linuxSubs)

	limit, reason := computeLimit(testConfig(dir), p, env)
	assert.EqualValues(t, 0, limit)
	assert.Contains(t, reason, "min_free_disk")
}

func TestComputeLimitStatfsErrorKeepsSizeCap(t *testing.T) {
	dir := t.TempDir()
	var limits [][2]uint64
	env := fakeEnv(0, &limits)
	env.freeBytes = func(string) (uint64, error) { return 0, errors.New("boom") }
	p := parseCorePattern(filepath.Join(dir, "core.%e.%p"), dir, "/", linuxSubs)

	limit, _ := computeLimit(testConfig(dir), p, env)
	assert.EqualValues(t, 3*gib, limit)
}

func TestComputeLimitExistingCore(t *testing.T) {
	dir := t.TempDir()
	var limits [][2]uint64
	env := fakeEnv(100*gib, &limits)
	p := parseCorePattern(filepath.Join(dir, "core.%e.%p"), dir, "/", linuxSubs)

	// A core of another binary does not count.
	writeFile(t, filepath.Join(dir, "core.process-agent.12"), time.Hour)
	limit, _ := computeLimit(testConfig(dir), p, env)
	assert.EqualValues(t, 3*gib, limit)

	// An expired core of this binary does not count.
	writeFile(t, filepath.Join(dir, "core.agent.11"), 100*time.Hour)
	limit, _ = computeLimit(testConfig(dir), p, env)
	assert.EqualValues(t, 3*gib, limit)

	// A recent core of this binary disables core dumps.
	writeFile(t, filepath.Join(dir, "core.agent.13"), time.Hour)
	limit, reason := computeLimit(testConfig(dir), p, env)
	assert.EqualValues(t, 0, limit)
	assert.Contains(t, reason, "core.agent.13")
	assert.Contains(t, reason, "this binary")
}

func TestComputeLimitExistingCorePerDir(t *testing.T) {
	dir := t.TempDir()
	var limits [][2]uint64
	env := fakeEnv(100*gib, &limits)
	// No %e: the file name does not tell which binary crashed.
	p := parseCorePattern(filepath.Join(dir, "core"), dir, "/", linuxSubs)
	require.False(t, p.perBinary)

	writeFile(t, filepath.Join(dir, "core.4242"), time.Hour)
	limit, reason := computeLimit(testConfig(dir), p, env)
	assert.EqualValues(t, 0, limit)
	assert.Contains(t, reason, "any binary")
}

func TestComputeLimitIgnoresSymlinksAndDirs(t *testing.T) {
	dir := t.TempDir()
	var limits [][2]uint64
	env := fakeEnv(100*gib, &limits)
	p := parseCorePattern(filepath.Join(dir, "core.%e.%p"), dir, "/", linuxSubs)

	outside := filepath.Join(t.TempDir(), "target")
	writeFile(t, outside, time.Hour)
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "core.agent.1")))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "core.agent.2"), 0o700))

	limit, _ := computeLimit(testConfig(dir), p, env)
	assert.EqualValues(t, 3*gib, limit)
}

func TestCleanupExpired(t *testing.T) {
	dir := t.TempDir()
	outsideDir := t.TempDir()
	p := parseCorePattern(filepath.Join(dir, "core.%e.%p"), dir, "/", linuxSubs)
	old := 100 * time.Hour

	writeFile(t, filepath.Join(dir, "core.agent.1"), old)          // deleted
	writeFile(t, filepath.Join(dir, "core.agent.2"), time.Hour)    // too recent
	writeFile(t, filepath.Join(dir, "core.trace-agent.3"), old)    // other binary
	writeFile(t, filepath.Join(dir, "unrelated.txt"), old)         // not a core
	writeFile(t, filepath.Join(outsideDir, "core.agent.4"), old)   // outside dir
	writeFile(t, filepath.Join(outsideDir, "symlink-target"), old) // symlink target
	require.NoError(t, os.Symlink(filepath.Join(outsideDir, "symlink-target"), filepath.Join(dir, "core.agent.5")))
	symlinkMtime := time.Now().Add(-old)
	require.NoError(t, os.Mkdir(filepath.Join(dir, "core.agent.6"), 0o700))
	require.NoError(t, os.Chtimes(filepath.Join(dir, "core.agent.6"), symlinkMtime, symlinkMtime))
	writeFile(t, filepath.Join(dir, "core.agent.6", "core.agent.7"), old) // nested, not deleted

	removed, err := cleanupExpired(dir, p.matcher, 72*time.Hour, time.Now())
	require.NoError(t, err)
	assert.Equal(t, []string{"core.agent.1"}, removed)

	assert.NoFileExists(t, filepath.Join(dir, "core.agent.1"))
	for _, path := range []string{
		filepath.Join(dir, "core.agent.2"),
		filepath.Join(dir, "core.trace-agent.3"),
		filepath.Join(dir, "unrelated.txt"),
		filepath.Join(outsideDir, "core.agent.4"),
		filepath.Join(outsideDir, "symlink-target"),
		filepath.Join(dir, "core.agent.6", "core.agent.7"),
	} {
		assert.FileExists(t, path)
	}
	_, err = os.Lstat(filepath.Join(dir, "core.agent.5"))
	assert.NoError(t, err, "symlink must not be deleted")
	assert.DirExists(t, filepath.Join(dir, "core.agent.6"))
}

func TestCleanupExpiredDisabled(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "core"), 1000*time.Hour)
	removed, err := cleanupExpired(dir, fallbackMatcher, 0, time.Now())
	require.NoError(t, err)
	assert.Empty(t, removed)
	assert.FileExists(t, filepath.Join(dir, "core"))
}

func TestApplyLimitFallsBackToCurrentHardLimit(t *testing.T) {
	var calls [][2]uint64
	env := fakeEnv(0, &calls)
	env.setCoreLimit = func(cur, maxLimit uint64) error {
		calls = append(calls, [2]uint64{cur, maxLimit})
		if maxLimit > gib {
			return errors.New("EPERM")
		}
		return nil
	}
	env.getCoreLimit = func() (uint64, uint64, error) { return 0, gib, nil }

	applied, err := applyLimit(3*gib, 3*gib, env)
	require.NoError(t, err)
	assert.EqualValues(t, gib, applied)
	assert.Equal(t, [][2]uint64{{3 * gib, 3 * gib}, {gib, gib}}, calls)
}

func TestRefreshEndToEnd(t *testing.T) {
	dir := t.TempDir()
	var limits [][2]uint64
	env := fakeEnv(100*gib, &limits)
	// Use the name specifier of the current OS (%e on Linux, %N on darwin).
	var spec byte
	for k, kind := range nameSpecifiers {
		if kind == specName {
			spec = k
		}
	}
	if spec == 0 {
		t.Skip("no name specifier on this platform")
	}
	env.readCorePattern = func() (string, error) { return filepath.Join(dir, "core.%"+string(spec)+".%p") + "\n", nil }

	s := newBoundedState(testConfig(dir), env)
	require.True(t, s.pattern.inDir)
	require.True(t, s.pattern.perBinary)

	s.refresh()
	writeFile(t, filepath.Join(dir, "core.agent.1"), time.Hour)
	s.refresh()
	// The core expires: refresh deletes it and enables core dumps again.
	require.NoError(t, os.Chtimes(filepath.Join(dir, "core.agent.1"), time.Now().Add(-100*time.Hour), time.Now().Add(-100*time.Hour)))
	s.refresh()

	assert.Equal(t, [][2]uint64{{3 * gib, 3 * gib}, {0, 3 * gib}, {3 * gib, 3 * gib}}, limits)
	assert.NoFileExists(t, filepath.Join(dir, "core.agent.1"))
}

// TestApplyLimitReal sets the real RLIMIT_CORE of the test process. It keeps
// the current hard limit, so that it does not need privileges.
func TestApplyLimitReal(t *testing.T) {
	env := defaultEnv()
	origCur, origMax, err := env.getCoreLimit()
	if err != nil {
		t.Skipf("getrlimit not supported: %v", err)
	}
	t.Cleanup(func() { _ = env.setCoreLimit(origCur, origMax) })

	want := min(uint64(4096), origMax)
	applied, err := applyLimit(want, origMax, env)
	require.NoError(t, err)
	assert.Equal(t, want, applied)

	cur, maxLimit, err := env.getCoreLimit()
	require.NoError(t, err)
	assert.Equal(t, want, cur)
	assert.Equal(t, origMax, maxLimit)
}
