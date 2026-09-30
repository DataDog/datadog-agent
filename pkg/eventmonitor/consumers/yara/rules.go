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
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	// ErrRulesDirUnset is returned when no rules directory is configured
	ErrRulesDirUnset = errors.New("yara: rules_dir is not set")
	// ErrNoRules is returned when the rules directory holds no rule file
	ErrNoRules = errors.New("yara: no rule file found")
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

// LoadRuleSources reads every rule file (*.yar, *.yara) directly in dir, sorted by name, and
// returns them with their version. Subdirectories are not read.
func LoadRuleSources(dir string) ([]RuleSource, string, error) {
	if dir == "" {
		return nil, "", ErrRulesDirUnset
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, "", fmt.Errorf("yara: failed to list rules directory: %w", err)
	}

	var sources []RuleSource
	for _, entry := range entries {
		if !isRuleFileName(entry.Name()) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		// Stat, not the entry type, so that symlinks to rule files are followed
		info, err := os.Stat(path)
		if err != nil {
			return nil, "", fmt.Errorf("yara: failed to stat rule file %s: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, "", fmt.Errorf("yara: failed to read rule file %s: %w", path, err)
		}
		sources = append(sources, RuleSource{Name: entry.Name(), Data: data})
	}
	if len(sources) == 0 {
		return nil, "", fmt.Errorf("%w in %s", ErrNoRules, dir)
	}

	sort.Slice(sources, func(i, j int) bool { return sources[i].Name < sources[j].Name })
	return sources, RulesVersion(sources), nil
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
