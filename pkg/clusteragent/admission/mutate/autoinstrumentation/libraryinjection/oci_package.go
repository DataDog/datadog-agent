// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package libraryinjection

import "strings"

// OCIPackage contains an OCI package reference for APM library injection.
type OCIPackage struct {
	// Name is the package name (e.g., "dd-lib-java-init").
	Name string
	// Registry is the OCI registry (e.g., "gcr.io/datadoghq").
	Registry string
	// Version is the package version: either a tag (e.g., "1.2.3") or a digest (e.g., "sha256:abc123").
	Version string
	// CanonicalVersion is the human-readable version (e.g., "1.2.3"), empty if not resolved.
	// Used for annotations/telemetry.
	CanonicalVersion string
}

// NewOCIPackageFromFullRef parses a full image reference into an OCIPackage.
// Supports formats like:
//   - "gcr.io/datadoghq/dd-lib-java-init:1.2.3"
//   - "gcr.io/datadoghq/dd-lib-java-init@sha256:abc123"
//   - "foo/bar:1.2.3" (registry is "foo")
//   - "dd-lib-java-init:1.2.3" (no registry)
func NewOCIPackageFromFullRef(fullRef string, canonicalVersion string) OCIPackage {
	pkg := OCIPackage{CanonicalVersion: canonicalVersion}

	// First, split registry from name+version on the last "/"
	nameWithVersion := fullRef
	if idx := strings.LastIndex(fullRef, "/"); idx != -1 {
		pkg.Registry = fullRef[:idx]
		nameWithVersion = fullRef[idx+1:]
	}

	// Then extract version (tag or digest) from name
	if idx := strings.LastIndex(nameWithVersion, "@"); idx != -1 {
		pkg.Name = nameWithVersion[:idx]
		pkg.Version = nameWithVersion[idx+1:]
	} else if idx := strings.LastIndex(nameWithVersion, ":"); idx != -1 {
		pkg.Name = nameWithVersion[:idx]
		pkg.Version = nameWithVersion[idx+1:]
	} else {
		pkg.Name = nameWithVersion
	}

	return pkg
}

// FullRef returns the full OCI image reference.
// If Registry and Version are set, it builds "Registry/Name:Version" or "Registry/Name@digest".
func (p OCIPackage) FullRef() string {
	var result string

	if p.Registry != "" {
		result = p.Registry + "/"
	}
	result += p.Name
	if p.Version != "" {
		// Use @ for digests (sha256:, sha512:, etc.), : for tags
		if strings.Contains(p.Version, ":") {
			result += "@" + p.Version
		} else {
			result += ":" + p.Version
		}
	}
	return result
}
