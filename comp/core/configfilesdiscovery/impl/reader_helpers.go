// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build docker || (cri && containerd)

package configfilesdiscoveryimpl

import (
	"errors"
	"io"
	"path"
	"slices"
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

// filePatternSearchRoot returns the literal path or the longest directory
// prefix before the first component containing pattern metacharacters.
func filePatternSearchRoot(pattern VerifiedConfigFilePattern) VerifiedConfigFilePath {
	patternValue := pattern.String()
	if !hasFilePatternMeta(patternValue) {
		return VerifiedConfigFilePath{value: patternValue}
	}

	parts := strings.Split(strings.TrimPrefix(patternValue, "/"), "/")
	fixedParts := make([]string, 0, len(parts))
	for _, part := range parts {
		if hasFilePatternMeta(part) {
			break
		}
		fixedParts = append(fixedParts, part)
	}
	if len(fixedParts) == 0 {
		return VerifiedConfigFilePath{value: "/"}
	}
	return VerifiedConfigFilePath{value: "/" + path.Join(fixedParts...)}
}

// hasFilePatternMeta returns whether pattern contains a supported file-pattern
// metacharacter.
func hasFilePatternMeta(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[")
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
