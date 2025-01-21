// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package meta provides a way to access embedded remote config TUF metadata
package meta

import (
	_ "embed"
	"encoding/json"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/go-tuf/data"
)

var (
	//go:embed prod.director.json
	prodRootDirector []byte
	//go:embed prod.config.json
	prodRootConfig []byte

	//go:embed staging.director.json
	stagingRootDirector []byte
	//go:embed staging.config.json
	stagingRootConfig []byte
)

// EmbeddedRoot is an embedded root
type EmbeddedRoot []byte

// EmbeddedRoots is a map of version => EmbeddedRoot
type EmbeddedRoots struct {
	latest uint64
	root   EmbeddedRoot
}

func newEmbeddedRoots(embeddedRoot []byte) EmbeddedRoots {
	version := parseRootVersion(embeddedRoot)
	return EmbeddedRoots{
		latest: version,
		root:   embeddedRoot,
	}
}

// RootsDirector returns all the roots of the director repo
func RootsDirector(site string, directorRootOverride string) EmbeddedRoots {
	if directorRootOverride != "" {
		return newEmbeddedRoots([]byte(directorRootOverride))
	}
	switch site {
	case "datad0g.com":
		return newEmbeddedRoots(stagingRootDirector)
	default:
		return newEmbeddedRoots(prodRootDirector)
	}
}

// RootsConfig returns all the roots of the director repo
func RootsConfig(site string, configRootOverride string) EmbeddedRoots {
	if configRootOverride != "" {
		return newEmbeddedRoots([]byte(configRootOverride))
	}

	switch site {
	case "datad0g.com":
		return newEmbeddedRoots(stagingRootConfig)
	default:
		return newEmbeddedRoots(prodRootConfig)
	}
}

// Last returns the last root the EmbeddedRoots
func (roots EmbeddedRoots) Last() EmbeddedRoot {
	return roots.root
}

// LastVersion returns the last version of the EmbeddedRoots
func (roots EmbeddedRoots) LastVersion() uint64 {
	return roots.latest
}

// parseRootVersion from the embedded roots for easy update
func parseRootVersion(rootBytes []byte) uint64 {
	var signedRoot data.Signed
	err := json.Unmarshal(rootBytes, &signedRoot)
	if err != nil {
		log.Errorf("Corrupted root metadata: %v", err)
		panic(err)
	}

	var root data.Root
	err = json.Unmarshal(signedRoot.Signed, &root)
	if err != nil {
		log.Errorf("Corrupted root metadata: %v", err)
		panic(err)
	}

	return uint64(root.Version)
}
