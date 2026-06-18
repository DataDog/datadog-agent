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

func pathSpecPath(pathSpec string) string {
	pathPart, _ := splitPathAccessSuffix(pathSpec)
	return pathPart
}

func pathSpecPaths(pathSpecs []string) []string {
	paths := make([]string, len(pathSpecs))
	for i, pathSpec := range pathSpecs {
		paths[i] = pathSpecPath(pathSpec)
	}
	return paths
}
