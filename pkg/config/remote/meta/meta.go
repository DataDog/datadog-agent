// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package meta

import (
	_ "embed"

	"github.com/DataDog/datadog-agent/pkg/config"
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
func RootsDirector() EmbeddedRoots {
	if directorRoot := config.Datadog.GetString("remote_configuration.director_root"); directorRoot != "" {
		return EmbeddedRoots{
			1: EmbeddedRoot(directorRoot),
		}
	}
	return rootsDirector
}

// RootsConfig returns all the roots of the director repo
func RootsConfig() EmbeddedRoots {
	if configRoot := config.Datadog.GetString("remote_configuration.config_root"); configRoot != "" {
		return EmbeddedRoots{
			1: EmbeddedRoot(configRoot),
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
