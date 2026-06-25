// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_rshell

import (
	"path"
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
)

// onlyRshellPrefixedCommands returns the commands that are prefixed with the rshell namespace.
//
// Assumptions:
//
//  1. The list comes from the backend, and should only contain commmands that "make sense" to be run by rshell.
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

// selectBackendPathsFromEnv returns the list of allowed paths for the current environment.
// Falls back to the default (non-containerized) paths.
func selectBackendPathsFromEnv(m map[string][]string) []string {
	if env.IsContainerized() {
		return m[setup.RShellPathAllowMapContainerizedKey]
	}
	return m[setup.RShellPathAllowMapDefaultKey]
}

// cleanPathList applies path.Clean to each element of the list of paths
// and ensures that each path ends with a separator:
// so that "/var/log" is a prefix of "/var/log/nginx" but not of "/var/logger".
func cleanPathList(paths []string) []string {
	cleaned := make([]string, len(paths))
	for i, p := range paths {
		cleaned[i] = path.Clean(p)

		if !strings.HasSuffix(cleaned[i], "/") {
			cleaned[i] += "/"
		}
	}
	return cleaned
}

// reducePathListToBroadest reduces the list of paths by removing duplicates and
// keeping the broadest path for each common prefix.
//
// Assumptions:
//
//  1. All paths have been cleaned (path.Clean)
//
//  2. All paths have been normalized (end with a separator).
func reducePathListToBroadest(paths []string) []string {
	reduced := make([]string, 0)
	for _, p := range paths {
		added := false
		for j := range reduced {
			if _, broadest := commonPath(p, reduced[j]); broadest != "" {
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

// intersectPathLists returns the intersection of two lists of paths.
// Meaning that the returned list contains only the paths that are present in both lists.
// If one list contains a sub-path of the other, only the sub-path is included in the intersection:
// the narrower side wins.
//
// Assumptions:
//
//  1. Both lists have been reduced to the broadest possible paths (reducePathListToBroadest).
func intersectPathLists(list1, list2 []string) []string {
	intersection := make([]string, 0)
	for _, p1 := range list1 {
		for _, p2 := range list2 {
			if deepest, _ := commonPath(p1, p2); deepest != "" {
				intersection = append(intersection, deepest)

				// If the common path is exactly the list1 path,
				// then we already added the biggest possible path,
				// so we can ignore the other path from list2.
				if deepest == p1 {
					break
				}
			}
		}
	}
	return intersection
}

// commonPath returns the deepest and the broadest common path between two paths.
//
// Assumptions:
//
//  1. Both a and b have been cleaned (path.Clean)
//
//  2. Both a and b have been normalized (end with a separator).
func commonPath(a, b string) (deepest string, broadest string) {
	if a == b {
		return a, a
	}

	// a is "deeper" than b.
	if strings.HasPrefix(a, b) {
		return a, b
	}

	// b is "deeper" than a.
	if strings.HasPrefix(b, a) {
		return b, a
	}

	// a and b are not related, there is no common path.
	return "", ""
}
