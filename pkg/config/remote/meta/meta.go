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

// EmbeddedRoot is an embedded root with its version parsed
type EmbeddedRoot struct {
	latest uint64
	root   []byte
}

// NewEmbeddedRoot creates a new EmbeddedRoot
func NewEmbeddedRoot(embeddedRoot []byte) EmbeddedRoot {
	version, err := parseRootVersion(embeddedRoot)
	if err != nil {
		panic(err)
	}
	return EmbeddedRoot{
		latest: version,
		root:   embeddedRoot,
	}
}

// RootsDirector returns all the roots of the director repo
func RootsDirector(site string, directorRootOverride string) EmbeddedRoot {
	if directorRootOverride != "" {
		return NewEmbeddedRoot([]byte(directorRootOverride))
	}
	switch site {
	case "datad0g.com":
		return NewEmbeddedRoot(stagingRootDirector)
	default:
		return NewEmbeddedRoot(prodRootDirector)
	}
}

// RootsConfig returns all the roots of the director repo
func RootsConfig(site string, configRootOverride string) EmbeddedRoot {
	if configRootOverride != "" {
		return NewEmbeddedRoot([]byte(configRootOverride))
	}

	switch site {
	case "datad0g.com":
		return NewEmbeddedRoot(stagingRootConfig)
	default:
		return NewEmbeddedRoot(prodRootConfig)
	}
}

// Root returns the last root the EmbeddedRoots
func (root EmbeddedRoot) Root() []byte {
	return root.root
}

// Version returns the last version of the EmbeddedRoots
func (root EmbeddedRoot) Version() uint64 {
	return root.latest
}

// parseRootVersion from the embedded roots for easy update
func parseRootVersion(rootBytes []byte) (uint64, error) {
	var signedRoot data.Signed
	err := json.Unmarshal(rootBytes, &signedRoot)
	if err != nil {
		log.Errorf("Corrupted root metadata: %v", err)
		return 0, err
	}

	var root data.Root
	err = json.Unmarshal(signedRoot.Signed, &root)
	if err != nil {
		log.Errorf("Corrupted root metadata: %v", err)
		return 0, err
	}

	return uint64(root.Version), nil
}
