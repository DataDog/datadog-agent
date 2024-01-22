// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package meta provides a way to access embedded remote config TUF metadata
package meta

import (
	_ "embed"
)

var (
	//go:embed 1.director.json
	rootDirector1 []byte

	//go:embed 1.config.json
	rootConfig1 []byte
)

// EmbeddedRoot is an embedded root
type EmbeddedRoot []byte

// EmbeddedRoots is a map of version => EmbeddedRoot
type EmbeddedRoots map[uint64]EmbeddedRoot

var rootsDirector = EmbeddedRoots{
	1: rootDirector1,
}

var rootsConfig = EmbeddedRoots{
	1: rootConfig1,
}

// RootsDirector returns all the roots of the director repo
func RootsDirector(directorRootOverride string) EmbeddedRoots {
	if directorRootOverride != "" {
		return EmbeddedRoots{
			1: EmbeddedRoot(directorRootOverride),
		}
	}
	return rootsDirector
}

// RootsConfig returns all the roots of the director repo
func RootsConfig(configRootOverride string) EmbeddedRoots {
	if configRootOverride != "" {
		return EmbeddedRoots{
			1: EmbeddedRoot(configRootOverride),
		}
	}
	return rootsConfig
}

// Last returns the last root the EmbeddedRoots
func (roots EmbeddedRoots) Last() EmbeddedRoot {
	return roots[roots.LastVersion()]
}

// LastVersion returns the last version of the EmbeddedRoots
func (roots EmbeddedRoots) LastVersion() uint64 {
	return uint64(len(roots))
}
