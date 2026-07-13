// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || darwin || windows

package com_datadoghq_remoteaction_rshell

import (
	"path"
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
)

const (
	pathAccessReadOnly  = ":ro"
	pathAccessReadWrite = ":rw"
)

// onlyRshellPrefixedCommands returns the commands that are prefixed with the rshell namespace.
//
// Assumptions:
//
//  1. The list comes from the backend, and should only contain commands that "make sense" to be run by rshell.
func onlyRshellPrefixedCommands(commands []string) []string {
	prefixedCommands := make([]string, 0, len(commands))
	for _, c := range commands {
		if strings.HasPrefix(c, setup.RShellCommandNamespacePrefix) &&
			c != setup.RShellCommandAllowAllWildcard && // this is the wildcard token itself, it should never be admitted
			c != setup.RShellCommandNamespacePrefix { // this is the empty name after the prefix, it should never be admitted
			prefixedCommands = append(prefixedCommands, c)
		}
	}
	return prefixedCommands
}

// selectBackendPathsFromEnv returns the legacy input path list for the current environment.
// Falls back to the default non-containerized paths.
func selectBackendPathsFromEnv(m map[string][]string) []string {
	if env.IsContainerized() {
		return m[setup.RShellPathAllowMapContainerizedKey]
	}
	return m[setup.RShellPathAllowMapDefaultKey]
}

func intersectAllowedCommands(backendAllowed []string, operatorAllowed []string) []string {
	operatorAllowedSet := make(map[string]struct{}, len(operatorAllowed))
	for _, c := range operatorAllowed {
		switch {
		case c == setup.RShellCommandAllowAllWildcard:
			return slices.Clone(backendAllowed)
		case strings.HasPrefix(c, setup.RShellCommandNamespacePrefix):
			operatorAllowedSet[c] = struct{}{}
		}
	}

	filtered := make([]string, 0, len(backendAllowed))
	for _, c := range backendAllowed {
		if _, ok := operatorAllowedSet[c]; ok {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

// cleanPathList applies path.Clean to each element of the list of paths
// and ensures that each path ends with a separator:
// so that "/var/log" is a prefix of "/var/log/nginx" but not of "/var/logger".
// It preserves rshell access suffixes (:ro / :rw) at the end of the path spec.
func cleanPathList(paths []string) []string {
	cleaned := make([]string, len(paths))
	for i, p := range paths {
		pathPart, accessSuffix := splitPathAccessSuffix(p)
		cleanedPath := path.Clean(pathPart)

		if !strings.HasSuffix(cleanedPath, "/") {
			cleanedPath += "/"
		}
		cleaned[i] = cleanedPath + accessSuffix
	}
	return cleaned
}

// intersectAllowedPathsByAccess keeps narrower paths shared by the operator and backend allowlists,
// then reduces the result to the broadest non-redundant paths.
// Paths only match within the same access group, except unsuffixed operator root
// which admits backend paths with their original access suffix.
func intersectAllowedPathsByAccess(operatorAllowed []string, backendAllowed []string) []string {
	filtered := make([]string, 0, len(operatorAllowed))
	seen := make(map[string]struct{}, len(operatorAllowed))

	for _, agentPath := range operatorAllowed {
		for _, backendPath := range backendAllowed {
			pathToKeep, ok := narrowerPathWithSameAccess(agentPath, backendPath)
			if !ok {
				continue
			}
			if _, ok := seen[pathToKeep]; ok {
				continue
			}
			filtered = append(filtered, pathToKeep)
			seen[pathToKeep] = struct{}{}
		}
	}
	return reducePathListToBroadest(filtered)
}

func splitPathAccessSuffix(pathSpec string) (pathPart string, accessSuffix string) {
	switch {
	case strings.HasSuffix(pathSpec, pathAccessReadWrite):
		return strings.TrimSuffix(pathSpec, pathAccessReadWrite), pathAccessReadWrite
	case strings.HasSuffix(pathSpec, pathAccessReadOnly):
		return strings.TrimSuffix(pathSpec, pathAccessReadOnly), pathAccessReadOnly
	default:
		return pathSpec, ""
	}
}

func pathAccessGroup(pathSpec string) string {
	_, accessSuffix := splitPathAccessSuffix(pathSpec)
	if accessSuffix == pathAccessReadWrite {
		return pathAccessReadWrite
	}
	return pathAccessReadOnly
}

func pathSpecPath(pathSpec string) string {
	pathPart, _ := splitPathAccessSuffix(pathSpec)
	return pathPart
}

func narrowerPathWithSameAccess(a, b string) (pathToKeep string, ok bool) {
	aPath := pathSpecPath(a)
	bPath := pathSpecPath(b)
	if isUnsuffixedRootPath(a) {
		if isAbsolutePathSpecPath(bPath) {
			return b, true
		}
		return "", false
	}

	if pathAccessGroup(a) != pathAccessGroup(b) {
		return "", false
	}
	switch {
	case aPath == bPath || strings.HasPrefix(aPath, bPath):
		return a, true
	case strings.HasPrefix(bPath, aPath):
		return b, true
	default:
		return "", false
	}
}

func isUnsuffixedRootPath(pathSpec string) bool {
	pathPart, accessSuffix := splitPathAccessSuffix(pathSpec)
	return pathPart == setup.RShellPathAllowAll && accessSuffix == ""
}

func isAbsolutePathSpecPath(pathPart string) bool {
	return isPOSIXAbsolutePathSpecPath(pathPart) || isWindowsAbsolutePathSpecPath(pathPart)
}

func isPOSIXAbsolutePathSpecPath(pathPart string) bool {
	return strings.HasPrefix(pathPart, "/")
}

func isWindowsAbsolutePathSpecPath(pathPart string) bool {
	// Windows absolute paths are always in the format `C:/` where the drive letter
	// can be any valid uppercase or lowercase letter. We check if the path follows
	// this format.
	if len(pathPart) < 3 {
		return false
	}
	driveLetter := pathPart[0]
	return ((driveLetter >= 'A' && driveLetter <= 'Z') || (driveLetter >= 'a' && driveLetter <= 'z')) &&
		pathPart[1] == ':' &&
		pathPart[2] == '/'
}

// reducePathListToBroadest reduces the list of paths by removing duplicates and
// keeping the broadest path for each common prefix.
// Read-only and read-write paths are reduced independently so write access does
// not collapse into a broader read-only path with the same prefix.
// When the same path exists in both groups, the read-write path is kept because
// it already permits read access.
//
// Assumptions:
//
//  1. All paths have been cleaned (path.Clean)
//
//  2. All paths have been normalized (end with a separator).
func reducePathListToBroadest(paths []string) []string {
	if len(paths) == 0 {
		return []string{}
	}

	readOnlyPaths, readWritePaths := splitPathListByAccess(paths)
	reducedReadOnly := reducePathSpecsToBroadest(readOnlyPaths)
	reducedReadWrite := reducePathSpecsToBroadest(readWritePaths)

	reduced := dedupeSamePathPreferReadWrite(slices.Concat(reducedReadOnly, reducedReadWrite))

	slices.Sort(reduced)
	return slices.Compact(reduced)
}

// dedupeSamePathPreferReadWrite drops read-only entries when the same path also has read-write access.
func dedupeSamePathPreferReadWrite(paths []string) []string {
	readWritePaths := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		if pathAccessGroup(p) == pathAccessReadWrite {
			readWritePaths[pathSpecPath(p)] = struct{}{}
		}
	}

	deduped := make([]string, 0, len(paths))
	for _, p := range paths {
		if pathAccessGroup(p) == pathAccessReadOnly {
			if _, ok := readWritePaths[pathSpecPath(p)]; ok {
				continue
			}
		}
		deduped = append(deduped, p)
	}
	return deduped
}

func splitPathListByAccess(paths []string) (readOnly []string, readWrite []string) {
	for _, p := range paths {
		_, accessSuffix := splitPathAccessSuffix(p)
		if accessSuffix == pathAccessReadWrite {
			readWrite = append(readWrite, p)
			continue
		}
		readOnly = append(readOnly, p)
	}
	return readOnly, readWrite
}

func reducePathSpecsToBroadest(paths []string) []string {
	reduced := make([]string, 0, len(paths))
	for _, p := range paths {
		added := false
		for j := range reduced {
			if broadest, ok := broadestPathSpec(p, reduced[j]); ok {
				// The path p has a common prefix with the already present path reduced[j],
				// so we replace the already present path with the broader common prefix.
				reduced[j] = broadest
				added = true
			}
		}

		// The path p has nothing in common with existing reduced paths,
		// so it is a new path to add to the list of reduced paths.
		if !added {
			reduced = append(reduced, p)
		}
	}

	// Remove duplicates.
	slices.Sort(reduced)
	return slices.Compact(reduced)
}

func broadestPathSpec(a, b string) (string, bool) {
	aPath := pathSpecPath(a)
	bPath := pathSpecPath(b)

	switch {
	case aPath == bPath:
		if strings.HasSuffix(a, pathAccessReadOnly) {
			return a, true
		}
		if strings.HasSuffix(b, pathAccessReadOnly) {
			return b, true
		}
		return a, true
	case strings.HasPrefix(aPath, bPath):
		return b, true
	case strings.HasPrefix(bPath, aPath):
		return a, true
	default:
		return "", false
	}
}
