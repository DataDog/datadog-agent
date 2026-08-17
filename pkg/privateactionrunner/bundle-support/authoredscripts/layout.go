// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package authoredscripts

import (
	"fmt"
	"path/filepath"
	"regexp"
	"runtime"
)

// The artifact cache holds every authored-script package that has been downloaded and
// extracted, laid out as:
//
//	<cacheRoot>/packages/<fqn>/sha256/<digest>/<platform>/
//	├── script/
//	│   ├── package.yaml
//	│   └── run.sh
//	├── tools/
//	│   └── helm/
//	│       └── helm
//	└── complete
//
// A package is keyed by the digest of the artifact it was extracted from rather than by
// its version, so that published directories are immutable: a new version of a script
// adds a sibling directory instead of rewriting one that executions may be reading, and
// a version that is republished with different contents cannot be mistaken for the one
// already cached. The action's fully qualified name is part of the path because an
// execution only knows the name of the action it is running, and can then find the
// packages belonging to that action without reading every package in the cache.
//
// The completion marker is written last, once a package has been fully extracted and
// verified, and is what makes a directory eligible to be read.
const (
	packagesDirectory = "packages"
	digestAlgorithm   = "sha256"
	scriptDirectory   = "script"
	toolsDirectory    = "tools"
	completionMarker  = "complete"
)

// fqnPattern matches the fully qualified name of an authored-script action, mirroring
// the fqn pattern of the authored-script package schema. Matching it also makes a name
// safe to use as a single path element.
var fqnPattern = regexp.MustCompile(`^[a-z][a-zA-Z0-9]*(\.[a-zA-Z][a-zA-Z0-9]*)+$`)

// digestPattern matches the hex encoding of a sha256 digest, without its algorithm
// prefix. Matching it also makes a digest safe to use as a single path element.
var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func validateFQN(fqn string) error {
	if !fqnPattern.MatchString(fqn) {
		return fmt.Errorf("%q is not a valid authored-script action name", fqn)
	}
	return nil
}

func validateDigest(digest string) error {
	if !digestPattern.MatchString(digest) {
		return fmt.Errorf("%q is not a valid authored-script artifact digest", digest)
	}
	return nil
}

// currentPlatform returns the platform directory name of the running runner. Packages
// are platform specific because they bundle the tools their script depends on.
func currentPlatform() string {
	return runtime.GOOS + "-" + runtime.GOARCH
}

// packageDigestsDirectory returns the directory holding every artifact digest cached
// for an action.
func packageDigestsDirectory(cacheRoot, fqn string) string {
	return filepath.Join(cacheRoot, packagesDirectory, fqn, digestAlgorithm)
}

// packageDirectory returns the directory of a single cached package.
func packageDirectory(cacheRoot, fqn, digest, platform string) string {
	return filepath.Join(packageDigestsDirectory(cacheRoot, fqn), digest, platform)
}
