// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package authoredscripts

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/Masterminds/semver/v3"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ErrNotCached reports that no usable package is cached for an action. Callers match it
// with errors.Is to tell a package that has not been downloaded apart from one that is
// cached but unusable.
var ErrNotCached = errors.New("authored-script package is not cached")

// Resolver maps the fully qualified name of an authored-script action to the digest of
// the artifact that should be executed for it.
type Resolver interface {
	Resolve(fqn string) (string, error)
}

// cacheResolver resolves an action against the packages already present in the artifact
// cache, selecting the highest version cached for it.
//
// This stands in for the Remote Config catalog, which is the authority on the single
// artifact authorized for an action. Once that catalog is available it replaces this
// resolver rather than supplementing it, so that a package that happens to be on disk
// can never override an authorization decision.
type cacheResolver struct {
	cacheRoot string
	platform  string
}

// NewCacheResolver returns a Resolver backed by the packages cached under cacheRoot.
func NewCacheResolver(cacheRoot string) Resolver {
	return &cacheResolver{cacheRoot: cacheRoot, platform: currentPlatform()}
}

func (r *cacheResolver) Resolve(fqn string) (string, error) {
	if err := validateFQN(fqn); err != nil {
		return "", err
	}

	entries, err := os.ReadDir(packageDigestsDirectory(r.cacheRoot, fqn))
	if errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("%w: %s", ErrNotCached, fqn)
	}
	if err != nil {
		return "", fmt.Errorf("could not read the authored-script cache: %w", err)
	}

	var (
		selectedDigest  string
		selectedVersion *semver.Version
	)
	for _, entry := range entries {
		digest := entry.Name()
		if !entry.IsDir() || validateDigest(digest) != nil {
			continue
		}
		version, err := r.cachedVersion(fqn, digest)
		if err != nil {
			// One unusable package must not hide the other versions cached alongside it.
			log.Debugf("ignoring cached authored-script package %s at %s: %v", fqn, digest, err)
			continue
		}
		if selectedVersion == nil || isPreferredPackage(version, digest, selectedVersion, selectedDigest) {
			selectedDigest, selectedVersion = digest, version
		}
	}
	if selectedDigest == "" {
		return "", fmt.Errorf("%w: %s", ErrNotCached, fqn)
	}
	return selectedDigest, nil
}

// cachedVersion returns the version of a cached package, or reports why that package
// cannot be used for the requested action.
func (r *cacheResolver) cachedVersion(fqn, digest string) (*semver.Version, error) {
	directory := packageDirectory(r.cacheRoot, fqn, digest, r.platform)
	if err := checkComplete(directory); err != nil {
		return nil, err
	}
	manifest, err := LoadManifest(directory)
	if err != nil {
		return nil, err
	}
	if manifest.FQN != fqn {
		return nil, fmt.Errorf("package declares action %q", manifest.FQN)
	}
	version, err := semver.NewVersion(manifest.Version)
	if err != nil {
		return nil, fmt.Errorf("could not parse package version %q: %w", manifest.Version, err)
	}
	return version, nil
}

// isPreferredPackage reports whether one cached package should be chosen over another.
// Packages are ordered by version, falling back to their digests so that the choice
// stays stable when the same version is cached more than once.
func isPreferredPackage(version *semver.Version, digest string, selectedVersion *semver.Version, selectedDigest string) bool {
	if comparison := version.Compare(selectedVersion); comparison != 0 {
		return comparison > 0
	}
	return strings.Compare(digest, selectedDigest) > 0
}
