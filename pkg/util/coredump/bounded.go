// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package coredump

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
//   - 0 if the free space in `core_dump.dir` is less than `core_dump.min_free_disk`.
//   - 0 if a core of this binary that is younger than `core_dump.max_age` is
//     already in `core_dump.dir`. If the kernel core pattern has no %e (or %E),
//     the kernel does not put the binary name in the file name, so any core in
//     the directory counts.
//   - `core_dump.max_size` otherwise.
//
// Core files in `core_dump.dir` that are older than `core_dump.max_age` are
// deleted at start and then every refreshInterval. Only regular files directly
// in the directory that match the core naming are deleted. Symlinks and
// directories are never followed nor deleted.
//
// The kernel core pattern is node-wide and the Agent does not change it. The
// Agent only reads it and logs a warning when cores will not land in
// `core_dump.dir`.

const refreshInterval = time.Hour

// fallbackMatcher is used when the core pattern cannot tell how core files are
// named in core_dump.dir (pipe pattern, other directory, unreadable pattern).
var fallbackMatcher = regexp.MustCompile(`^core([._-].*)?$`)

type boundedConfig struct {
	dir         string
	maxSize     uint64
	minFreeDisk uint64
	maxAge      time.Duration
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
	// perBinary is true when the file name contains the binary name (%e or %E).
	perBinary bool
	// matcher matches the base name of core files of this binary in core_dump.dir.
	matcher *regexp.Regexp
}

// parseCorePattern parses a kernel core pattern.
//
// subs maps a pattern specifier (for example 'e' on Linux) to the binary name
// that the kernel puts in place of it. Other specifiers match any text.
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

	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(base); i++ {
		c := base[i]
		if c != '%' {
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
			b.WriteString("%")
		case subs[spec] != "":
			p.perBinary = true
			b.WriteString(regexp.QuoteMeta(subs[spec]))
		default:
			b.WriteString(".*")
		}
	}
	// With kernel.core_uses_pid=1 and no %p, the kernel appends ".<pid>".
	b.WriteString(`(\.[0-9]+)?$`)

	if m, err := regexp.Compile(b.String()); err == nil {
		p.matcher = m
	} else {
		p.perBinary = false
	}
	return p
}

// coreFile is a core file found in core_dump.dir.
type coreFile struct {
	name    string
	modTime time.Time
}

// listCores returns the regular files directly in dir whose name matches the
// matcher. Symlinks, directories and other file types are ignored.
func listCores(dir string, matcher *regexp.Regexp) ([]coreFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var cores []coreFile
	for _, e := range entries {
		// ReadDir does not follow symlinks: a symlink has the ModeSymlink type.
		if !e.Type().IsRegular() || !matcher.MatchString(e.Name()) {
			continue
		}
		info, err := os.Lstat(filepath.Join(dir, e.Name()))
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		cores = append(cores, coreFile{name: e.Name(), modTime: info.ModTime()})
	}
	return cores, nil
}

// expired returns true when the core is older than maxAge. A maxAge of 0 means
// that cores never expire.
func expired(c coreFile, maxAge time.Duration, now time.Time) bool {
	return maxAge > 0 && now.Sub(c.modTime) > maxAge
}

// cleanupExpired deletes the expired cores of this binary in dir. It returns
// the names of the deleted files.
func cleanupExpired(dir string, matcher *regexp.Regexp, maxAge time.Duration, now time.Time) ([]string, error) {
	if maxAge <= 0 {
		return nil, nil
	}
	cores, err := listCores(dir, matcher)
	if err != nil {
		return nil, err
	}
	var removed []string
	var errs []string
	for _, c := range cores {
		if !expired(c, maxAge, now) {
			continue
		}
		// c.name comes from ReadDir: it has no path separator, so the path
		// stays in dir. os.Remove does not follow symlinks.
		if err := os.Remove(filepath.Join(dir, c.name)); err != nil {
			errs = append(errs, err.Error())
			continue
		}
		removed = append(removed, c.name)
	}
	if len(errs) > 0 {
		return removed, fmt.Errorf("cannot delete expired cores: %s", strings.Join(errs, "; "))
	}
	return removed, nil
}

// computeLimit returns the RLIMIT_CORE value to use and the reason.
func computeLimit(bc boundedConfig, p corePattern, env sysEnv) (uint64, string) {
	if free, err := env.freeBytes(bc.dir); err != nil {
		log.Warnf("Core dumps: cannot read free disk space of %q, skipping the free disk check: %v", bc.dir, err)
	} else if free < bc.minFreeDisk {
		return 0, fmt.Sprintf("free disk space in %s is %d bytes, less than core_dump.min_free_disk (%d bytes)", bc.dir, free, bc.minFreeDisk)
	}

	cores, err := listCores(bc.dir, p.matcher)
	if err != nil {
		log.Warnf("Core dumps: cannot list %q, skipping the existing core check: %v", bc.dir, err)
	}
	now := env.now()
	for _, c := range cores {
		if !expired(c, bc.maxAge, now) {
			scope := "this binary"
			if !p.perBinary {
				scope = "any binary (the core pattern has no binary name)"
			}
			return 0, fmt.Sprintf("a core of %s already exists: %s", scope, filepath.Join(bc.dir, c.name))
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
}

// refresh deletes expired cores, then computes and applies RLIMIT_CORE.
func (s *boundedState) refresh() {
	s.mu.Lock()
	defer s.mu.Unlock()

	removed, err := cleanupExpired(s.cfg.dir, s.pattern.matcher, s.cfg.maxAge, s.env.now())
	if err != nil {
		log.Warnf("Core dumps: %v", err)
	}
	for _, name := range removed {
		log.Infof("Core dumps: deleted expired core %s", filepath.Join(s.cfg.dir, name))
	}

	limit, reason := computeLimit(s.cfg, s.pattern, s.env)
	applied, err := applyLimit(limit, s.cfg.maxSize, s.env)
	if err != nil {
		log.Warnf("Core dumps: %v", err)
		return
	}
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

	log.Infof("Core dumps: bounded mode, dir=%s max_size=%d min_free_disk=%d max_age=%s core_pattern=%q",
		bc.dir, bc.maxSize, bc.minFreeDisk, bc.maxAge, p.raw)
	switch {
	case err != nil:
	case p.pipe:
		log.Warnf("Core dumps: the kernel core pattern %q sends cores to a program: cores will not land in %s. "+
			"The kernel does not apply RLIMIT_CORE to piped cores; the program gets it as %%c and can apply it.", p.raw, bc.dir)
	case strings.TrimSpace(p.raw) == "":
		log.Warnf("Core dumps: the kernel core pattern is empty: the kernel may not write cores")
	case !p.inDir:
		log.Warnf("Core dumps: the kernel core pattern %q writes cores in %s, not in core_dump.dir %s: "+
			"cores will not land in core_dump.dir, and the free disk and existing core checks use core_dump.dir", p.raw, p.dir, bc.dir)
	case !p.perBinary:
		log.Infof("Core dumps: the kernel core pattern %q has no binary name (%%e): at most one core per directory", p.raw)
	}
	if bc.maxSize == 0 {
		log.Warnf("Core dumps: core_dump.max_size is 0 or invalid: cores are disabled")
	}

	return &boundedState{cfg: bc, pattern: p, env: env}
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
		}
	}
	return subs
}

const (
	specName = iota
	specPath
)

func readBoundedConfig(cfg model.Reader) boundedConfig {
	return boundedConfig{
		dir:         cfg.GetString("core_dump.dir"),
		maxSize:     uint64(cfg.GetSizeInBytes("core_dump.max_size")),
		minFreeDisk: uint64(cfg.GetSizeInBytes("core_dump.min_free_disk")),
		maxAge:      cfg.GetDuration("core_dump.max_age"),
	}
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
