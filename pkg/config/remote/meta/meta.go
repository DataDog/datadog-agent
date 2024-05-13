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
	//go:embed prod.1.director.json
	prodRootDirector1 []byte
	//go:embed prod.1.config.json
	prodRootConfig1 []byte

	//go:embed staging.1.director.json
	stagingRootDirector1 []byte
	//go:embed staging.1.config.json
	stagingRootConfig1 []byte
)

// EmbeddedRoot is an embedded root
type EmbeddedRoot []byte

// EmbeddedRoots is a map of version => EmbeddedRoot
type EmbeddedRoots map[uint64]EmbeddedRoot

var (
	prodRootsDirector = EmbeddedRoots{1: prodRootDirector1}
	prodRootsConfig   = EmbeddedRoots{1: prodRootConfig1}

	stagingRootsDirector = EmbeddedRoots{1: stagingRootDirector1}
	stagingRootsConfig   = EmbeddedRoots{1: stagingRootConfig1}
)

// RootsDirector returns all the roots of the director repo
func RootsDirector(site string, directorRootOverride string) EmbeddedRoots {
	if directorRootOverride != "" {
		return EmbeddedRoots{
			1: EmbeddedRoot(directorRootOverride),
		}
	}
	switch site {
	case "datad0g.com":
		return stagingRootsDirector
	default:
		return prodRootsDirector
	}
}

// RootsConfig returns all the roots of the director repo
func RootsConfig(site string, configRootOverride string) EmbeddedRoots {
	if configRootOverride != "" {
		return EmbeddedRoots{
			1: EmbeddedRoot(configRootOverride),
		}
	}
	switch site {
	case "datad0g.com":
		return stagingRootsConfig
	default:
		return prodRootsConfig
	}
}

// Last returns the last root the EmbeddedRoots
func (roots EmbeddedRoots) Last() EmbeddedRoot {
	return roots[roots.LastVersion()]
}

// LastVersion returns the last version of the EmbeddedRoots
func (roots EmbeddedRoots) LastVersion() uint64 {
	return uint64(len(roots))
}
