// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package catalog defines the shared representation and selection rules for
// downloadable Fleet package catalogs.
package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
)

// Package identifies one downloadable package variant.
type Package struct {
	Name     string `json:"package"`
	Version  string `json:"version"`
	SHA256   string `json:"sha256"`
	URL      string `json:"url"`
	Size     int64  `json:"size"`
	Platform string `json:"platform"`
	Arch     string `json:"arch"`
}

// MatchesTarget reports whether the package can be used on platform and arch.
// Empty constraints are portable and match every target.
func (p Package) MatchesTarget(platform, arch string) bool {
	return (p.Platform == "" || p.Platform == platform) &&
		(p.Arch == "" || p.Arch == arch)
}

// Catalog contains downloadable package variants.
type Catalog struct {
	Packages []Package `json:"packages"`
}

// Parse decodes one catalog document. Validation remains explicit so callers
// can preserve source-specific error reporting and apply-state behavior.
func Parse(data []byte) (Catalog, error) {
	var catalog Catalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		return Catalog{}, err
	}
	return catalog, nil
}

// Merge combines catalog documents without changing package order or resolving
// duplicate entries. Package selection therefore retains the existing
// first-match behavior.
func Merge(catalogs ...Catalog) Catalog {
	var merged Catalog
	for _, catalog := range catalogs {
		merged.Packages = append(merged.Packages, catalog.Packages...)
	}
	return merged
}

// GetPackage returns the first package variant matching name, version, and the
// requested platform and architecture. Empty package constraints match every
// platform or architecture.
func (c Catalog) GetPackage(packageName, version, platform, arch string) (Package, bool) {
	for _, pkg := range c.Packages {
		if pkg.Name == packageName && pkg.Version == version && pkg.MatchesTarget(platform, arch) {
			return pkg, true
		}
	}
	return Package{}, false
}

// Validate validates every package in the catalog.
func (c Catalog) Validate() error {
	for _, pkg := range c.Packages {
		if err := ValidatePackage(pkg); err != nil {
			return err
		}
	}
	return nil
}

// ValidatePackage validates package coordinates accepted from a catalog.
func ValidatePackage(pkg Package) error {
	if pkg.Name == "" {
		return errors.New("package name is empty")
	}
	if pkg.Version == "" {
		return errors.New("package version is empty")
	}
	if pkg.URL == "" {
		return errors.New("package URL is empty")
	}
	parsedURL, err := url.Parse(pkg.URL)
	if err != nil {
		return fmt.Errorf("could not parse package URL: %w", err)
	}
	if parsedURL.Scheme == "oci" {
		ociURL := strings.TrimPrefix(pkg.URL, "oci://")
		// Packages received through a catalog must use immutable OCI references.
		if _, err := name.NewDigest(ociURL); err != nil {
			return fmt.Errorf("could not parse oci digest URL: %w", err)
		}
	}
	return nil
}
