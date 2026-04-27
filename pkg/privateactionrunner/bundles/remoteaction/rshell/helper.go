// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_rshell

import (
	"path"
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config/setup"
)

// onlyRshellPrefixedCommands returns the commands that are prefixed with the
// rshell namespace, excluding the wildcard token and the bare prefix.
func onlyRshellPrefixedCommands(commands []string) []string {
	prefixedCommands := make([]string, 0, len(commands))
	for _, c := range commands {
		if strings.HasPrefix(c, setup.RShellCommandNamespacePrefix) &&
			c != setup.RShellCommandAllowAllWildcard &&
			c != setup.RShellCommandNamespacePrefix {
			prefixedCommands = append(prefixedCommands, c)
		}
	}
	return prefixedCommands
}

// cleanPathList applies path.Clean to each element and ensures every path
// ends with a separator so "/var/log" is a prefix of "/var/log/nginx" but
// not of "/var/logger".
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

// reducePathListToBroadest removes duplicates and keeps the broadest path
// for each common prefix.
//
// Assumes inputs are already cleaned (path.Clean + trailing "/").
func reducePathListToBroadest(paths []string) []string {
	reduced := make([]string, 0)
	for _, p := range paths {
		added := false
		for j := range reduced {
			if _, broadest := commonPath(p, reduced[j]); broadest != "" {
				reduced[j] = broadest
				added = true
			}
		}
		if !added {
			reduced = append(reduced, p)
		}
	}
	slices.Sort(reduced)
	return slices.Compact(reduced)
}

// intersectPathLists returns the containment intersection of two reduced
// path lists. The narrower side of each matching pair wins.
func intersectPathLists(list1, list2 []string) []string {
	intersection := make([]string, 0)
	for _, p1 := range list1 {
		for _, p2 := range list2 {
			if deepest, _ := commonPath(p1, p2); deepest != "" {
				intersection = append(intersection, deepest)
				if deepest == p1 {
					break
				}
			}
		}
	}
	return intersection
}

// commonPath returns the deepest and broadest common path between two
// cleaned + trailing-slash-normalized paths.
func commonPath(a, b string) (deepest string, broadest string) {
	if a == b {
		return a, a
	}
	if strings.HasPrefix(a, b) {
		return a, b
	}
	if strings.HasPrefix(b, a) {
		return b, a
	}
	return "", ""
}
