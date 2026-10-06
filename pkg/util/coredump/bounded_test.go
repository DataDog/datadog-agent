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

const gib uint64 = 1 << 30

var linuxSubs = map[byte]string{'e': "agent", 'E': "!opt!datadog-agent!bin!agent!agent", 'f': "agent"}

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
		canDelete  bool
		dir        string
		matches    []string
		nonMatches []string
	}{
		{
			name:       "plain",
			canDelete:  true,
			raw:        "/var/crash/core",
			inDir:      true,
			dir:        "/var/crash",
			matches:    []string{"core", "core.1234"},
			nonMatches: []string{"core.agent", "xcore", "core.12a"},
		},
		{
			name:       "with %e and %p",
			canDelete:  true,
			raw:        "/var/crash/core.%e.%p",
			inDir:      true,
			perBinary:  true,
			dir:        "/var/crash",
			matches:    []string{"core.agent.1234"},
			nonMatches: []string{"core.process-agent.1234", "core.trace-agent.1", "core.agent"},
		},
		{
			name:       "doc example %e-%p-%t",
			canDelete:  true,
			raw:        "/var/crash/core-%e-%p-%t\n",
			inDir:      true,
			perBinary:  true,
			dir:        "/var/crash",
			matches:    []string{"core-agent-12-1700000000"},
			nonMatches: []string{"core-process-agent-12-1700000000"},
		},
		{
			name:      "with %E",
			canDelete: true,
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
			canDelete:  true,
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
			// The pattern says nothing about files in core_dump.dir: use the
			// fallback matcher, and never delete.
			name:       "other directory",
			raw:        "/tmp/cores/core.%e",
			dir:        "/tmp/cores",
			matches:    []string{"core", "core.agent"},
			nonMatches: []string{"agent.crash"},
		},
		{
			name:       "with %f",
			raw:        "/var/crash/%f.%p.core",
			inDir:      true,
			perBinary:  true,
			canDelete:  true,
			dir:        "/var/crash",
			matches:    []string{"agent.12.core"},
			nonMatches: []string{"trace-agent.12.core"},
		},
		{
			// Numeric specifiers only match digits.
			name:       "numeric specifiers",
			raw:        "/var/crash/core-%e-%p-%s-%t",
			inDir:      true,
			perBinary:  true,
			canDelete:  true,
			dir:        "/var/crash",
			matches:    []string{"core-agent-12-11-1700000000"},
			nonMatches: []string{"core-agent-x-11-1700000000", "core-agent-12-11-"},
		},
		{
			// %p alone matches any file with a numeric name: never delete.
			name:       "only %p",
			raw:        "/var/crash/%p",
			inDir:      true,
			dir:        "/var/crash",
			matches:    []string{"1234"},
			nonMatches: []string{"foo.crash"},
		},
		{
			// %h alone matches any file: never delete.
			name:    "only %h",
			raw:     "/var/crash/%h",
			inDir:   true,
			dir:     "/var/crash",
			matches: []string{"_usr_bin_foo.0.crash"},
		},
		{
			name:      "relative pattern resolved against cwd",
			canDelete: true,
			raw:       "core",
			cwd:       "/var/crash",
			inDir:     true,
			dir:       "/var/crash",
			matches:   []string{"core"},
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
			assert.Equal(t, tt.canDelete, p.canDelete, "canDelete")
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

	// 12 GiB free is more than min_free_disk, but a 3 GiB core would leave
	// less than min_free_disk.
	env = fakeEnv(12*gib, &limits)
	limit, _ = computeLimit(testConfig(dir), p, env)
	assert.EqualValues(t, 0, limit)

	env = fakeEnv(13*gib, &limits)
	limit, _ = computeLimit(testConfig(dir), p, env)
	assert.EqualValues(t, 3*gib, limit)
}

func TestComputeLimitTotalSize(t *testing.T) {
	dir := t.TempDir()
	var limits [][2]uint64
	env := fakeEnv(100*gib, &limits)
	p := parseCorePattern(filepath.Join(dir, "core.%e.%p"), dir, "/", linuxSubs)
	bc := testConfig(dir)
	bc.maxSize = 1000
	bc.maxTotalSize = 1500

	limit, _ := computeLimit(bc, p, env)
	assert.EqualValues(t, 1000, limit)

	// Files of other binaries count in the total size.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "core.trace-agent.1"), make([]byte, 600), 0o600))
	limit, reason := computeLimit(bc, p, env)
	assert.EqualValues(t, 0, limit)
	assert.Contains(t, reason, "max_total_size")
}

func TestComputeLimitInvalidConfig(t *testing.T) {
	dir := t.TempDir()
	var limits [][2]uint64
	env := fakeEnv(100*gib, &limits)
	p := parseCorePattern(filepath.Join(dir, "core.%e.%p"), dir, "/", linuxSubs)
	bc := testConfig(dir)
	bc.invalid = "invalid settings: x"

	limit, reason := computeLimit(bc, p, env)
	assert.EqualValues(t, 0, limit)
	assert.Equal(t, "invalid settings: x", reason)
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

func TestRefreshDoesNotDeleteWithUnsafePattern(t *testing.T) {
	dir := t.TempDir()
	var limits [][2]uint64
	env := fakeEnv(100*gib, &limits)
	env.readCorePattern = func() (string, error) { return filepath.Join(dir, "%p"), nil }
	writeFile(t, filepath.Join(dir, "1234"), 1000*time.Hour)

	s := newBoundedState(testConfig(dir), env)
	require.False(t, s.pattern.canDelete)
	s.refresh()
	assert.FileExists(t, filepath.Join(dir, "1234"))
}

func TestRefreshDoesNotDeleteWithOtherDir(t *testing.T) {
	dir := t.TempDir()
	var limits [][2]uint64
	env := fakeEnv(100*gib, &limits)
	env.readCorePattern = func() (string, error) { return "/somewhere/else/core", nil }
	writeFile(t, filepath.Join(dir, "core"), 1000*time.Hour)

	s := newBoundedState(testConfig(dir), env)
	require.False(t, s.pattern.canDelete)
	s.refresh()
	assert.FileExists(t, filepath.Join(dir, "core"))
}

func TestParseSize(t *testing.T) {
	valid := map[string]uint64{
		"0":         0,
		"1048576":   1 << 20,
		"512B":      512,
		"3GB":       3 * gib,
		"3G":        3 * gib,
		"3Gi":       3 * gib,
		"3GiB":      3 * gib,
		"3gb":       3 * gib,
		" 10 GB ":   10 * gib,
		"500MB":     500 << 20,
		"500Mi":     500 << 20,
		"64K":       64 << 10,
		"2TB":       2 << 40,
		"16777215T": 16777215 << 40,
	}
	for in, want := range valid {
		got, err := parseSize(in)
		if assert.NoError(t, err, in) {
			assert.Equal(t, want, got, in)
		}
	}
	for _, in := range []string{"", "GB", "3.5GB", "-1", "3PB", "3 G B", "16777216T", "99999999999999999999"} {
		_, err := parseSize(in)
		assert.Error(t, err, in)
	}
}

func TestParseBoundedConfig(t *testing.T) {
	get := func(values map[string]string) func(string) string {
		return func(key string) string { return values[key] }
	}
	defaults := map[string]string{
		"core_dump.dir":           "/var/crash",
		"core_dump.max_size":      "2GB",
		"core_dump.min_free_disk": "10GB",
		"core_dump.max_age":       "72h",
	}

	bc := parseBoundedConfig(get(defaults))
	assert.Equal(t, boundedConfig{dir: "/var/crash", maxSize: 2 * gib, minFreeDisk: 10 * gib, maxAge: 72 * time.Hour}, bc)

	with := func(key, value string) map[string]string {
		m := map[string]string{}
		for k, v := range defaults {
			m[k] = v
		}
		m[key] = value
		return m
	}

	bc = parseBoundedConfig(get(with("core_dump.max_total_size", "4Gi")))
	assert.Empty(t, bc.invalid)
	assert.EqualValues(t, 4*gib, bc.maxTotalSize)

	bc = parseBoundedConfig(get(with("core_dump.max_age", "0")))
	assert.Empty(t, bc.invalid)
	assert.Zero(t, bc.maxAge)

	for key, value := range map[string]string{
		"core_dump.max_size":       "3.5GB",
		"core_dump.min_free_disk":  "lots",
		"core_dump.max_total_size": "1GB", // smaller than max_size
		"core_dump.max_age":        "72",  // no unit: 72ns
	} {
		bc = parseBoundedConfig(get(with(key, value)))
		assert.Contains(t, bc.invalid, key, "%s=%s", key, value)
	}
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
