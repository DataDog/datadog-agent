// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build docker || (cri && containerd)

package configfilesdiscoveryimpl

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"slices"
	"strconv"
	"strings"
)

const maxConfigFileSize = 1024 * 1024 // 1MiB

func filterEnvVars(envEntries []string, predicate ConfigEnvVarPredicate) map[string]string {
	env := make(map[string]string)
	for _, entry := range envEntries {
		name, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if IsSecretEnvVarName(name) {
			continue
		}
		if !predicate(name) {
			continue
		}
		env[name] = value
	}

	return env
}

// configFileSearchRoot returns a search root that never requires the runtime
// to traverse a path component below the trusted root. Directory traversal
// can therefore observe and reject symlinks instead of following them.
func configFileSearchRoot(search ConfigFileSearch) VerifiedConfigFilePath {
	root := search.Root().String()
	pattern := search.Pattern().String()
	if pattern == root {
		return search.Root()
	}

	relativePattern := strings.TrimPrefix(pattern, root)
	firstComponent, _, _ := strings.Cut(strings.TrimPrefix(relativePattern, "/"), "/")
	if firstComponent == "" || hasFilePatternMeta(firstComponent) {
		return search.Root()
	}
	return VerifiedConfigFilePath{value: path.Join(root, firstComponent)}
}

// hasFilePatternMeta returns whether pattern contains a supported file-pattern
// metacharacter.
func hasFilePatternMeta(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[")
}

// escapeFindPathPattern returns filePath escaped so find -path treats it as a
// literal path rather than a shell pattern.
func escapeFindPathPattern(filePath VerifiedConfigFilePath) string {
	value := filePath.String()
	var escaped strings.Builder
	escaped.Grow(len(value))
	for i := 0; i < len(value); i++ {
		character := value[i]
		if character == '\\' || character == '*' || character == '?' || character == '[' {
			escaped.WriteByte('\\')
		}
		escaped.WriteByte(character)
	}
	return escaped.String()
}

// sortAndLimitFilePaths sorts and deduplicates paths in place, then returns at
// most maxMatches paths and whether additional paths were omitted.
func sortAndLimitFilePaths(paths []VerifiedConfigFilePath, maxMatches int) ([]VerifiedConfigFilePath, bool, error) {
	if maxMatches <= 0 {
		return nil, false, errors.New("maximum file matches must be positive")
	}
	slices.SortFunc(paths, func(left, right VerifiedConfigFilePath) int {
		return strings.Compare(left.String(), right.String())
	})
	paths = slices.CompactFunc(paths, func(left, right VerifiedConfigFilePath) bool {
		return left == right
	})
	if len(paths) <= maxMatches {
		return paths, false, nil
	}
	return paths[:maxMatches], true, nil
}

// readMatchingConfigFiles validates, matches, and reads NUL-delimited paths
// returned by a bounded runtime search.
func readMatchingConfigFiles(
	stdout []byte,
	discoveryLimited bool,
	search ConfigFileSearch,
	maxMatches int,
	matches ConfigFilePathMatcher,
	readFile func(VerifiedConfigFilePath) (ConfigFile, error),
) ([]ConfigFileReadResult, bool, error) {
	// A bounded search may stop halfway through a path. Keep only complete,
	// NUL-terminated entries and report that discovery was partial.
	if len(stdout) != 0 && stdout[len(stdout)-1] != 0 {
		discoveryLimited = true
		lastSeparator := bytes.LastIndexByte(stdout, 0)
		if lastSeparator < 0 {
			stdout = nil
		} else {
			stdout = stdout[:lastSeparator+1]
		}
	}

	// Treat runtime output as untrusted: accept only verified paths confined to
	// the requested search and approved by the collector's matcher.
	var paths []VerifiedConfigFilePath
	for _, outputPath := range bytes.Split(stdout, []byte{0}) {
		if len(outputPath) == 0 {
			continue
		}
		filePath, err := VerifyConfigFilePath(string(outputPath))
		if err != nil || !search.Contains(filePath) {
			continue
		}
		matched, err := matches(filePath)
		if err != nil {
			return nil, false, fmt.Errorf("match config file %q: %w", filePath.String(), err)
		}
		if matched {
			paths = append(paths, filePath)
		}
	}

	// Runtime discovery order is not stable, so establish lexical order and
	// remove duplicates before applying the match limit.
	paths, pathsLimited, err := sortAndLimitFilePaths(paths, maxMatches)
	if err != nil {
		return nil, false, err
	}

	// Preserve per-file read failures alongside successful reads so callers can
	// skip individual files without discarding the rest of the bounded result.
	var results []ConfigFileReadResult
	for _, filePath := range paths {
		file, err := readFile(filePath)
		if err != nil {
			results = append(results, NewConfigFileReadError(filePath, err))
			continue
		}
		results = append(results, NewConfigFileReadResult(filePath, file))
	}
	return results, discoveryLimited || pathsLimited, nil
}

// buildReadFileWithinSearchCommand returns a command that emits the path
// followed by bounded contents only when find observes a regular file.
func buildReadFileWithinSearchCommand(searchRoot VerifiedConfigFilePath, filePath VerifiedConfigFilePath) []string {
	return []string{
		"find", "-P", searchRoot.String(),
		"-type", "f",
		"-path", escapeFindPathPattern(filePath),
		"-print0",
		"-exec", "head", "-c", strconv.Itoa(maxConfigFileSize + 1), "{}", ";",
	}
}

// decodeReadFileWithinSearchOutput validates and decodes the path-prefixed
// output from buildReadFileWithinSearchCommand.
func decodeReadFileWithinSearchOutput(stdout []byte, stderr []byte, searchRoot VerifiedConfigFilePath, filePath VerifiedConfigFilePath) (ConfigFile, error) {
	if len(stderr) != 0 {
		return ConfigFile{}, fmt.Errorf("read scoped config file: %s", strings.TrimSpace(string(stderr)))
	}

	pathPrefix := append([]byte(filePath.String()), 0)
	if !bytes.HasPrefix(stdout, pathPrefix) {
		return ConfigFile{}, fmt.Errorf("config file %q is not a regular file within search root %q", filePath.String(), searchRoot.String())
	}
	content, truncated, err := readLimitedFileContent(bytes.NewReader(stdout[len(pathPrefix):]), maxConfigFileSize)
	if err != nil {
		return ConfigFile{}, fmt.Errorf("read config file output: %w", err)
	}
	return ConfigFile{Path: filePath.String(), Content: content, Truncated: truncated}, nil
}

func readLimitedFileContent(r io.Reader, limit int) ([]byte, bool, error) {
	// Read one byte past the returned content limit so callers can distinguish
	// a file exactly at the limit from a larger file.
	content, err := io.ReadAll(io.LimitReader(r, int64(limit)+1))
	if err != nil {
		return nil, false, err
	}
	if len(content) <= limit {
		return content, false, nil
	}
	return content[:limit], true, nil
}
