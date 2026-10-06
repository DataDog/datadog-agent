// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package coredump

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Bounded core dumps
//
// When `core_dump.dir` is set, the core dump size limit (RLIMIT_CORE) is not
// unlimited. It is computed as follows:
//
//   - 0 if a setting is invalid.
//   - 0 if the free space in `core_dump.dir` minus `core_dump.max_size` is less
//     than `core_dump.min_free_disk`.
//   - 0 if `core_dump.max_total_size` is set and the files in `core_dump.dir`
//     plus `core_dump.max_size` are larger than it.
//   - 0 if a core of this binary that is younger than `core_dump.max_age` is
//     already in `core_dump.dir`. If the kernel core pattern has no binary name
//     (%e, %E or %f), any core in the directory counts.
//   - `core_dump.max_size` otherwise.
//
// Cores in `core_dump.dir` that are older than `core_dump.max_age` are deleted
// at start and then every refreshInterval. Only regular files directly in the
// directory whose name matches the kernel core pattern are deleted, and only
// when the pattern writes into `core_dump.dir`. Symlinks and directories are
// never followed nor deleted.
//
// The kernel core pattern is node-wide and the Agent does not change it. The
// Agent only reads it and logs a warning when cores will not land in
// `core_dump.dir`.

const (
	refreshInterval = time.Hour
	// minMaxAge protects against a unitless max_age (for example `72`, read as 72ns).
	minMaxAge = time.Minute
)

// fallbackMatcher is used to find existing cores when the core pattern does not
// tell how core files are named in core_dump.dir (pipe pattern, other
// directory, unreadable pattern). It is never used to delete files.
var fallbackMatcher = regexp.MustCompile(`^core([._-].*)?$`)

type boundedConfig struct {
	dir          string
	maxSize      uint64
	minFreeDisk  uint64
	maxTotalSize uint64 // 0 means no total cap
	maxAge       time.Duration
	// invalid is not empty when a setting is invalid: cores are then disabled.
	invalid string
}

// sysEnv holds the system calls used by the bounded mode, so that tests can replace them.
type sysEnv struct {
	readCorePattern func() (string, error)
	freeBytes       func(dir string) (uint64, error)
	procName        func() string
	exePath         func() (string, error)
	getwd           func() (string, error)
	now             func() time.Time
	setCoreLimit    func(cur, maxLimit uint64) error
	getCoreLimit    func() (cur, maxLimit uint64, err error)
}

func defaultEnv() sysEnv {
	return sysEnv{
		readCorePattern: readCorePattern,
		freeBytes:       freeBytes,
		procName:        procName,
		exePath:         os.Executable,
		getwd:           os.Getwd,
		now:             time.Now,
		setCoreLimit:    setCoreLimit,
		getCoreLimit:    getCoreLimit,
	}
}

// corePattern is the parsed kernel core pattern.
type corePattern struct {
	raw string
	// pipe is true when the kernel sends cores to a program (`|...`).
	pipe bool
	// dir is the absolute directory where the kernel writes cores. Empty for pipes.
	dir string
	// inDir is true when cores land in core_dump.dir.
	inDir bool
	// perBinary is true when the file name contains the binary name.
	perBinary bool
	// matcher matches the base name of core files of this binary in core_dump.dir.
	matcher *regexp.Regexp
	// canDelete is true when matcher is specific enough to delete files with it.
	canDelete bool
}

// numericSpecifiers are the Linux core_pattern specifiers that expand to a number:
// pid, global pid, tid, global tid, uid, gid, signal, time, core limit, dump mode.
const numericSpecifiers = "pPiIugstcd"

// parseCorePattern parses a kernel core pattern.
//
// subs maps a pattern specifier (for example 'e' on Linux) to the binary name
// that the kernel puts in place of it.
func parseCorePattern(raw, coreDir, cwd string, subs map[byte]string) corePattern {
	raw = strings.TrimRight(raw, "\n")
	p := corePattern{raw: raw, matcher: fallbackMatcher}

	if strings.HasPrefix(raw, "|") {
		p.pipe = true
		return p
	}
	if strings.TrimSpace(raw) == "" {
		return p
	}

	dir, base := filepath.Split(raw)
	if dir == "" {
		dir = "."
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(cwd, dir)
	}
	p.dir = filepath.Clean(dir)

	// A specifier in the directory part makes the directory dynamic: we cannot
	// tell where cores land.
	if strings.Contains(p.dir, "%") || base == "" {
		return p
	}
	p.inDir = p.dir == filepath.Clean(coreDir)
	if !p.inDir {
		// Cores do not land in core_dump.dir: the pattern says nothing about
		// the files there.
		return p
	}

	var b strings.Builder
	hasLiteral := false
	perBinary := false
	b.WriteString("^")
	for i := 0; i < len(base); i++ {
		c := base[i]
		if c != '%' {
			hasLiteral = true
			b.WriteString(regexp.QuoteMeta(string(c)))
			continue
		}
		if i+1 >= len(base) {
			// The kernel drops a trailing lone '%'.
			break
		}
		i++
		spec := base[i]
		switch {
		case spec == '%':
			hasLiteral = true
			b.WriteString("%")
		case subs[spec] != "":
			perBinary = true
			b.WriteString(regexp.QuoteMeta(subs[spec]))
		case strings.IndexByte(numericSpecifiers, spec) >= 0:
			b.WriteString("[0-9]+")
		default:
			// %h (hostname), %e without a known name, unknown specifiers.
			b.WriteString(".+")
		}
	}
	// With kernel.core_uses_pid=1 and no %p, the kernel appends ".<pid>".
	b.WriteString(`(\.[0-9]+)?$`)

	m, err := regexp.Compile(b.String())
	if err != nil {
		return p
	}
	p.matcher = m
	p.perBinary = perBinary
	// A pattern made only of specifiers (for example `%p` or `%h`) matches
	// files that are not cores: use it to find cores, never to delete files.
	p.canDelete = hasLiteral || perBinary
	return p
}

// dirFile is a regular file found directly in core_dump.dir.
type dirFile struct {
	name    string
	size    uint64
	modTime time.Time
}

// listFiles returns the regular files directly in dir. Symlinks, directories
// and other file types are ignored.
func listFiles(dir string) ([]dirFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []dirFile
	for _, e := range entries {
		// ReadDir does not follow symlinks: a symlink has the ModeSymlink type.
		if !e.Type().IsRegular() {
			continue
		}
		info, err := os.Lstat(filepath.Join(dir, e.Name()))
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		files = append(files, dirFile{name: e.Name(), size: uint64(max(info.Size(), 0)), modTime: info.ModTime()})
	}
	return files, nil
}

// expired returns true when the file is older than maxAge. A maxAge of 0 means
// that files never expire.
func expired(f dirFile, maxAge time.Duration, now time.Time) bool {
	return maxAge > 0 && now.Sub(f.modTime) > maxAge
}

// cleanupExpired deletes the expired cores of this binary in dir. It returns
// the names of the deleted files.
func cleanupExpired(dir string, matcher *regexp.Regexp, maxAge time.Duration, now time.Time) ([]string, error) {
	if maxAge <= 0 {
		return nil, nil
	}
	files, err := listFiles(dir)
	if err != nil {
		return nil, err
	}
	var removed []string
	var errs []string
	for _, f := range files {
		if !matcher.MatchString(f.name) || !expired(f, maxAge, now) {
			continue
		}
		// f.name comes from ReadDir: it has no path separator, so the path
		// stays in dir. os.Remove does not follow symlinks.
		if err := os.Remove(filepath.Join(dir, f.name)); err != nil {
			errs = append(errs, err.Error())
			continue
		}
		removed = append(removed, f.name)
	}
	if len(errs) > 0 {
		return removed, fmt.Errorf("cannot delete expired cores: %s", strings.Join(errs, "; "))
	}
	return removed, nil
}

func saturatingAdd(a, b uint64) uint64 {
	if a > math.MaxUint64-b {
		return math.MaxUint64
	}
	return a + b
}

// computeLimit returns the RLIMIT_CORE value to use and the reason.
func computeLimit(bc boundedConfig, p corePattern, env sysEnv) (uint64, string) {
	if bc.invalid != "" {
		return 0, bc.invalid
	}

	// After a core of max_size, at least min_free_disk must stay free.
	if free, err := env.freeBytes(bc.dir); err != nil {
		log.Warnf("Core dumps: cannot read free disk space of %q, skipping the free disk check: %v", bc.dir, err)
	} else if need := saturatingAdd(bc.minFreeDisk, bc.maxSize); free < need {
		return 0, fmt.Sprintf("free disk space in %s is %d bytes, less than core_dump.min_free_disk + core_dump.max_size (%d bytes)", bc.dir, free, need)
	}

	files, err := listFiles(bc.dir)
	if err != nil {
		log.Warnf("Core dumps: cannot list %q, skipping the existing core and total size checks: %v", bc.dir, err)
	}

	if bc.maxTotalSize > 0 {
		var used uint64
		for _, f := range files {
			used = saturatingAdd(used, f.size)
		}
		if saturatingAdd(used, bc.maxSize) > bc.maxTotalSize {
			return 0, fmt.Sprintf("files in %s use %d bytes; with core_dump.max_size (%d bytes) this is more than core_dump.max_total_size (%d bytes)",
				bc.dir, used, bc.maxSize, bc.maxTotalSize)
		}
	}

	now := env.now()
	for _, f := range files {
		if p.matcher.MatchString(f.name) && !expired(f, bc.maxAge, now) {
			scope := "this binary"
			if !p.perBinary {
				scope = "any binary (the core pattern has no binary name)"
			}
			return 0, fmt.Sprintf("a core of %s already exists: %s", scope, filepath.Join(bc.dir, f.name))
		}
	}

	return bc.maxSize, "core_dump.max_size"
}

// applyLimit sets RLIMIT_CORE. The hard limit is always max_size, so that the
// soft limit can go up again later without privileges.
func applyLimit(limit, maxSize uint64, env sysEnv) (uint64, error) {
	err := env.setCoreLimit(limit, maxSize)
	if err == nil {
		return limit, nil
	}
	// We cannot raise the hard limit without privileges: keep the current
	// hard limit and only set the soft limit.
	_, hard, gerr := env.getCoreLimit()
	if gerr != nil {
		return 0, fmt.Errorf("cannot set RLIMIT_CORE to %d: %v", limit, err)
	}
	soft := min(limit, hard)
	if err2 := env.setCoreLimit(soft, hard); err2 != nil {
		return 0, fmt.Errorf("cannot set RLIMIT_CORE to %d: %v, then %v", limit, err, err2)
	}
	return soft, nil
}

type boundedState struct {
	mu      sync.Mutex
	cfg     boundedConfig
	pattern corePattern
	env     sysEnv
	// last is the last applied limit and reason, to log only changes.
	last       uint64
	lastReason string
}

// refresh deletes expired cores, then computes and applies RLIMIT_CORE.
func (s *boundedState) refresh() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.pattern.canDelete {
		removed, err := cleanupExpired(s.cfg.dir, s.pattern.matcher, s.cfg.maxAge, s.env.now())
		if err != nil {
			log.Warnf("Core dumps: %v", err)
		}
		for _, name := range removed {
			log.Infof("Core dumps: deleted expired core %s", filepath.Join(s.cfg.dir, name))
		}
	}

	limit, reason := computeLimit(s.cfg, s.pattern, s.env)
	applied, err := applyLimit(limit, s.cfg.maxSize, s.env)
	if err != nil {
		log.Warnf("Core dumps: %v", err)
		return
	}
	if applied != limit {
		reason += ", lowered to the current RLIMIT_CORE hard limit"
	}
	if applied == s.last && reason == s.lastReason {
		return
	}
	s.last, s.lastReason = applied, reason
	if applied == 0 {
		log.Warnf("Core dumps: disabled (RLIMIT_CORE=0): %s", reason)
		return
	}
	log.Infof("Core dumps: RLIMIT_CORE set to %d bytes (%s)", applied, reason)
}

func newBoundedState(bc boundedConfig, env sysEnv) *boundedState {
	cwd, err := env.getwd()
	if err != nil {
		cwd = "/"
	}
	subs := nameSubstitutions(env)

	var p corePattern
	raw, err := env.readCorePattern()
	if err != nil {
		log.Warnf("Core dumps: cannot read the kernel core pattern, cannot tell where cores land: %v", err)
		p = corePattern{matcher: fallbackMatcher}
	} else {
		p = parseCorePattern(raw, bc.dir, cwd, subs)
	}

	log.Infof("Core dumps: bounded mode, dir=%s max_size=%d min_free_disk=%d max_total_size=%d max_age=%s core_pattern=%q",
		bc.dir, bc.maxSize, bc.minFreeDisk, bc.maxTotalSize, bc.maxAge, p.raw)
	switch {
	case err != nil:
	case p.pipe:
		log.Warnf("Core dumps: the kernel core pattern %q sends cores to a program: cores will not land in %s. "+
			"The kernel does not apply RLIMIT_CORE to piped cores; the program gets it as %%c and can apply it.", p.raw, bc.dir)
	case strings.TrimSpace(p.raw) == "":
		log.Warnf("Core dumps: the kernel core pattern is empty: the kernel may not write cores")
	case !p.inDir:
		log.Warnf("Core dumps: the kernel core pattern %q writes cores in %s, not in core_dump.dir %s: "+
			"cores will not land in core_dump.dir, expired cores are not deleted, and the checks use core_dump.dir", p.raw, p.dir, bc.dir)
	case !p.canDelete:
		log.Warnf("Core dumps: the kernel core pattern %q matches any file name: expired cores are not deleted", p.raw)
	case !p.perBinary:
		log.Infof("Core dumps: the kernel core pattern %q has no binary name (%%e): at most one core per directory", p.raw)
	}

	return &boundedState{cfg: bc, pattern: p, env: env, last: math.MaxUint64}
}

// nameSubstitutions returns the core pattern specifiers that the kernel
// replaces with the binary name.
func nameSubstitutions(env sysEnv) map[byte]string {
	subs := map[byte]string{}
	name := env.procName()
	exe, _ := env.exePath()
	for spec, kind := range nameSpecifiers {
		switch kind {
		case specName:
			subs[spec] = name
		case specPath:
			if exe != "" {
				subs[spec] = strings.ReplaceAll(exe, "/", "!")
			}
		case specFile:
			if exe != "" {
				subs[spec] = filepath.Base(exe)
			}
		}
	}
	return subs
}

const (
	specName = iota // comm, for example %e
	specPath        // executable path with '/' replaced by '!', for example %E
	specFile        // executable file name, for example %f
)

// parseSize parses a size such as `3GB`, `3G`, `3Gi`, `3GiB` or `1048576`.
// All suffixes (K, M, G, T, with or without `i` and `B`) are powers of 1024.
func parseSize(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, fmt.Errorf("invalid size %q", s)
	}
	n, err := strconv.ParseUint(s[:i], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q: %v", s, err)
	}
	var shift uint
	switch strings.ToLower(strings.TrimSpace(s[i:])) {
	case "", "b":
		shift = 0
	case "k", "kb", "ki", "kib":
		shift = 10
	case "m", "mb", "mi", "mib":
		shift = 20
	case "g", "gb", "gi", "gib":
		shift = 30
	case "t", "tb", "ti", "tib":
		shift = 40
	default:
		return 0, fmt.Errorf("invalid size %q: want an integer and an optional unit K, M, G or T", s)
	}
	if n > math.MaxUint64>>shift {
		return 0, fmt.Errorf("invalid size %q: too large", s)
	}
	return n << shift, nil
}

func readBoundedConfig(cfg model.Reader) boundedConfig {
	return parseBoundedConfig(cfg.GetString)
}

// parseBoundedConfig reads the core_dump settings with get. Invalid settings
// are reported in boundedConfig.invalid.
func parseBoundedConfig(get func(key string) string) boundedConfig {
	bc := boundedConfig{dir: get("core_dump.dir")}
	var invalid []string

	readSize := func(key string, optional bool) uint64 {
		raw := get(key)
		if optional && strings.TrimSpace(raw) == "" {
			return 0
		}
		v, err := parseSize(raw)
		if err != nil {
			invalid = append(invalid, fmt.Sprintf("%s: %v", key, err))
		}
		return v
	}
	bc.maxSize = readSize("core_dump.max_size", false)
	bc.minFreeDisk = readSize("core_dump.min_free_disk", false)
	bc.maxTotalSize = readSize("core_dump.max_total_size", true)

	if raw := strings.TrimSpace(get("core_dump.max_age")); raw != "" && raw != "0" {
		d, err := time.ParseDuration(raw)
		switch {
		case err != nil:
			invalid = append(invalid, fmt.Sprintf("core_dump.max_age: %v", err))
		case d < minMaxAge:
			invalid = append(invalid, fmt.Sprintf("core_dump.max_age: %s is less than %s", d, minMaxAge))
		default:
			bc.maxAge = d
		}
	}

	if bc.maxTotalSize > 0 && bc.maxSize > bc.maxTotalSize {
		invalid = append(invalid, "core_dump.max_size is larger than core_dump.max_total_size")
	}
	if len(invalid) > 0 {
		bc.invalid = "invalid settings: " + strings.Join(invalid, "; ")
	}
	return bc
}

var (
	stateMu sync.Mutex
	state   *boundedState
)

// BoundedEnabled returns true when core dumps are bounded, that is when
// `core_dump.dir` is set.
func BoundedEnabled(cfg model.Reader) bool {
	return cfg.GetString("core_dump.dir") != ""
}

// ApplyBounded computes and applies the bounded RLIMIT_CORE. The first call
// also starts a background task that deletes expired cores and applies the
// limit again every hour. It is safe to call it more than once: Setup calls it
// for Go crashes, and the Python loader calls it for C crashes (c_core_dump).
//
// All errors are logged as warnings.
func ApplyBounded(cfg model.Reader) {
	stateMu.Lock()
	first := state == nil
	if first {
		state = newBoundedState(readBoundedConfig(cfg), defaultEnv())
	}
	s := state
	stateMu.Unlock()

	s.refresh()

	if first {
		go func() {
			ticker := time.NewTicker(refreshInterval)
			defer ticker.Stop()
			for range ticker.C {
				s.refresh()
			}
		}()
	}
}
