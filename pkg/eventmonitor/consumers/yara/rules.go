// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package yara

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
)

var (
	// ErrRulesDirUnset is returned when no rules directory is configured
	ErrRulesDirUnset = errors.New("yara: rules_dir is not set")
	// ErrNoRules is returned when the rules directory holds no rule file
	ErrNoRules = errors.New("yara: no rule file found")
	// ErrUnsafeRules is returned when the rules directory or a rule file is not owned by root,
	// or is group or world writable
	ErrUnsafeRules = errors.New("yara: refusing to load rules: unsafe ownership or permissions")
)

// ruleFileExtensions are the file extensions loaded from the rules directory
var ruleFileExtensions = []string{".yar", ".yara"}

// RuleSource is the content of one rule file
type RuleSource struct {
	// Name is the file name, relative to the rules directory
	Name string
	Data []byte
}

// Compiler compiles rule sources into a Scanner. It is the seam where the YARA engine plugs in.
// sources are sorted by name. The returned Scanner's RulesVersion is ignored: LoadScanner
// replaces it with a hash of the sources.
type Compiler func(sources []RuleSource) (Scanner, error)

// trustedRuleOwnerUID is the only owner allowed for the rules directory and the rule files. Rules
// are code run against every executed binary, as root: whoever can write them controls the
// scanner. It is a variable only so that tests, which can't create root-owned files, can trust
// their own uid.
var trustedRuleOwnerUID uint32

// checkRulePermissions returns an ErrUnsafeRules error when path, described by info, is not
// owned by trustedRuleOwnerUID or is group or world writable.
//
// Only the rules directory and the rule files are checked, not their parent directories: a
// writable parent would let its owner swap the whole directory. For the PoC, rules_dir is
// expected under /etc/datadog-agent, which the agent packages keep root-owned.
func checkRulePermissions(path string, info os.FileInfo) error {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("%w: %s: no owner information", ErrUnsafeRules, path)
	}
	if st.Uid != trustedRuleOwnerUID {
		return fmt.Errorf("%w: %s is owned by uid %d, it must be owned by uid %d (root)", ErrUnsafeRules, path, st.Uid, trustedRuleOwnerUID)
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("%w: %s is group or world writable (mode %s)", ErrUnsafeRules, path, info.Mode().Perm())
	}
	return nil
}

// LoadRuleSources reads every rule file (*.yar, *.yara) directly in dir, sorted by name, and
// returns them with their version. Subdirectories are not read.
//
// dir and every rule file must be owned by root and not be group or world writable, or no rule
// is loaded (ErrUnsafeRules). Symlinks are followed, and the permissions of their target are
// checked. Each file is checked on the opened descriptor, so the checked file is the read one.
func LoadRuleSources(dir string) ([]RuleSource, string, error) {
	if dir == "" {
		return nil, "", ErrRulesDirUnset
	}

	d, err := os.Open(dir)
	if err != nil {
		return nil, "", fmt.Errorf("yara: failed to open rules directory: %w", err)
	}
	defer d.Close()
	dirInfo, err := d.Stat()
	if err != nil {
		return nil, "", fmt.Errorf("yara: failed to stat rules directory: %w", err)
	}
	if !dirInfo.IsDir() {
		return nil, "", fmt.Errorf("yara: rules directory %s is not a directory", dir)
	}
	if err := checkRulePermissions(dir, dirInfo); err != nil {
		return nil, "", err
	}
	entries, err := d.ReadDir(-1)
	if err != nil {
		return nil, "", fmt.Errorf("yara: failed to list rules directory: %w", err)
	}

	var sources []RuleSource
	for _, entry := range entries {
		if !isRuleFileName(entry.Name()) {
			continue
		}
		data, ok, err := readRuleFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, "", err
		}
		if ok {
			sources = append(sources, RuleSource{Name: entry.Name(), Data: data})
		}
	}
	if len(sources) == 0 {
		return nil, "", fmt.Errorf("%w in %s", ErrNoRules, dir)
	}

	sort.Slice(sources, func(i, j int) bool { return sources[i].Name < sources[j].Name })
	return sources, RulesVersion(sources), nil
}

// readRuleFile reads a rule file. It returns false, and no error, for an entry that is not a
// regular file (e.g. a directory named *.yar), which is skipped.
func readRuleFile(path string) ([]byte, bool, error) {
	// symlinks are followed. O_NONBLOCK so that a FIFO named *.yar doesn't block the open.
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, false, fmt.Errorf("yara: failed to open rule file %s: %w", path, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, false, fmt.Errorf("yara: failed to stat rule file %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, false, nil
	}
	if err := checkRulePermissions(path, info); err != nil {
		return nil, false, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, false, fmt.Errorf("yara: failed to read rule file %s: %w", path, err)
	}
	return data, true, nil
}

// RulesVersion returns a stable hash of the rule sources, over their names and contents. It
// doesn't depend on the order of sources.
func RulesVersion(sources []RuleSource) string {
	sorted := make([]RuleSource, len(sources))
	copy(sorted, sources)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	h := sha256.New()
	var length [8]byte
	writeField := func(b []byte) {
		// length-prefixed, so that moving bytes between a name and a content changes the hash
		binary.LittleEndian.PutUint64(length[:], uint64(len(b)))
		h.Write(length[:])
		h.Write(b)
	}
	for _, s := range sorted {
		writeField([]byte(s.Name))
		writeField(s.Data)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// LoadScanner loads the rule files in dir and compiles them with compile. The returned
// Scanner's RulesVersion is the hash of the rule files, also returned as version.
//
// On error, the caller must log it and keep scanning disabled; it must never be fatal.
func LoadScanner(dir string, compile Compiler) (Scanner, string, error) {
	if compile == nil {
		return nil, "", errors.New("yara: no rule compiler")
	}
	sources, version, err := LoadRuleSources(dir)
	if err != nil {
		return nil, "", err
	}
	scanner, err := compile(sources)
	if err != nil {
		return nil, "", fmt.Errorf("yara: failed to compile rules in %s: %w", dir, err)
	}
	if scanner == nil {
		return nil, "", fmt.Errorf("yara: failed to compile rules in %s: compiler returned no scanner", dir)
	}
	return &versionedScanner{Scanner: scanner, version: version}, version, nil
}

func isRuleFileName(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	for _, e := range ruleFileExtensions {
		if ext == e {
			return true
		}
	}
	return false
}

// versionedScanner overrides the RulesVersion of a compiled Scanner
type versionedScanner struct {
	Scanner
	version string
}

// RulesVersion implements Scanner
func (s *versionedScanner) RulesVersion() string {
	return s.version
}

// Close implements io.Closer. A Scanner holding native resources (e.g. compiled libyara rules)
// can implement io.Closer to free them; Close forwards to it, and is a no-op otherwise. It must
// only be called once no Scan is running.
func (s *versionedScanner) Close() error {
	if c, ok := s.Scanner.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// STAND-IN, NOT A YARA ENGINE. StandInCompiler is a Compiler for tests and the cgo-free dry
// run. For each `rule <name>` in the sources, it takes the first double-quoted string that
// follows as a literal marker, with no escape handling, and reports a match when the data
// contains it. Conditions, modifiers, hex strings and regexes are ignored. It fails when a
// source has no rule, or a rule has no quoted string.
func StandInCompiler(sources []RuleSource) (Scanner, error) {
	var scanners []*MarkerScanner
	for _, src := range sources {
		text := string(src.Data)
		headers := standInRuleHeader.FindAllStringSubmatchIndex(text, -1)
		if len(headers) == 0 {
			return nil, fmt.Errorf("%s: no rule found", src.Name)
		}
		for i, h := range headers {
			name := text[h[2]:h[3]]
			end := len(text)
			if i+1 < len(headers) {
				end = headers[i+1][0]
			}
			marker := standInQuotedString.FindStringSubmatch(text[h[1]:end])
			if marker == nil || marker[1] == "" {
				return nil, fmt.Errorf("%s: rule %s has no quoted string", src.Name, name)
			}
			scanners = append(scanners, NewMarkerScanner(marker[1], name))
		}
	}
	return standInMultiScanner(scanners), nil
}

var (
	standInRuleHeader   = regexp.MustCompile(`(?m)^\s*(?:(?:private|global)\s+)*rule\s+([A-Za-z_][A-Za-z0-9_]*)`)
	standInQuotedString = regexp.MustCompile(`"([^"\n]*)"`)
)

// standInMultiScanner runs each MarkerScanner in turn
type standInMultiScanner []*MarkerScanner

// Scan implements Scanner
func (s standInMultiScanner) Scan(ctx context.Context, data []byte) ([]Match, error) {
	var matches []Match
	for _, m := range s {
		found, err := m.Scan(ctx, data)
		if err != nil {
			return nil, err
		}
		matches = append(matches, found...)
	}
	return matches, nil
}

// RulesVersion implements Scanner. LoadScanner replaces it.
func (s standInMultiScanner) RulesVersion() string {
	return "standin"
}
