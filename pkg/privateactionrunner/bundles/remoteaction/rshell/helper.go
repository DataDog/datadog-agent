// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_rshell

import (
	"path"
	"strings"

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

func intersectAllowedCommands(backendAllowed []string, agentAllowed []string) []string {
	agentAllowedSet := make(map[string]struct{}, len(agentAllowed))
	for _, c := range agentAllowed {
		switch {
		case c == setup.RShellCommandAllowAllWildcard:
			return append([]string(nil), backendAllowed...)
		case c == setup.RShellCommandNamespacePrefix || c == "":
			continue
		case strings.HasPrefix(c, setup.RShellCommandNamespacePrefix):
			agentAllowedSet[c] = struct{}{}
		}
	}

	filtered := make([]string, 0, len(backendAllowed))
	for _, c := range backendAllowed {
		if _, ok := agentAllowedSet[c]; ok {
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

func intersectAllowedPathsByAccess(agentAllowed []string, backendAllowed []string) []string {
	filtered := make([]string, 0, len(agentAllowed))
	seen := make(map[string]struct{}, len(agentAllowed))

	for _, agentPath := range agentAllowed {
		for _, backendPath := range backendAllowed {
			if pathAccessGroup(agentPath) != pathAccessGroup(backendPath) {
				continue
			}
			if !pathIsDescendantOrSame(agentPath, backendPath) {
				continue
			}
			if _, ok := seen[agentPath]; ok {
				break
			}
			filtered = append(filtered, agentPath)
			seen[agentPath] = struct{}{}
			break
		}
	}
	return filtered
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

func pathIsDescendantOrSame(pathSpec, ancestorSpec string) bool {
	pathPart := pathSpecPath(pathSpec)
	ancestorPart := pathSpecPath(ancestorSpec)
	return pathPart == ancestorPart || strings.HasPrefix(pathPart, ancestorPart)
}
