// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package libraryinjection

import "strings"

// LibraryImage contains an OCI image reference for APM library injection.
type LibraryImage struct {
	// Name is the image name (e.g., "dd-lib-java-init").
	Name string
	// Registry is the OCI registry (e.g., "gcr.io/datadoghq").
	Registry string
	// Version is the image version: either a tag (e.g., "1.2.3") or a digest (e.g., "sha256:abc123").
	Version string
	// CanonicalVersion is the human-readable version (e.g., "1.2.3"), empty if not resolved.
	// Used for annotations/telemetry.
	CanonicalVersion string
}

// NewLibraryImageFromFullRef parses a full image reference into a LibraryImage.
// Supports formats like:
//   - "gcr.io/datadoghq/dd-lib-java-init:1.2.3"
//   - "gcr.io/datadoghq/dd-lib-java-init@sha256:abc123"
//   - "foo/bar:1.2.3" (registry is "foo")
//   - "dd-lib-java-init:1.2.3" (no registry)
func NewLibraryImageFromFullRef(fullRef string, canonicalVersion string) LibraryImage {
	img := LibraryImage{CanonicalVersion: canonicalVersion}

	// First, split registry from name+version on the last "/"
	nameWithVersion := fullRef
	if idx := strings.LastIndex(fullRef, "/"); idx != -1 {
		img.Registry = fullRef[:idx]
		nameWithVersion = fullRef[idx+1:]
	}

	// Then extract version (tag or digest) from name
	if idx := strings.LastIndex(nameWithVersion, "@"); idx != -1 {
		img.Name = nameWithVersion[:idx]
		img.Version = nameWithVersion[idx+1:]
	} else if idx := strings.LastIndex(nameWithVersion, ":"); idx != -1 {
		img.Name = nameWithVersion[:idx]
		img.Version = nameWithVersion[idx+1:]
	} else {
		img.Name = nameWithVersion
	}

	return img
}

// FullRef returns the full OCI image reference.
// If Registry and Version are set, it builds "Registry/Name:Version" or "Registry/Name@digest".
func (i LibraryImage) FullRef() string {
	var result string

	if i.Registry != "" {
		result = i.Registry + "/"
	}
	result += i.Name
	if i.Version != "" {
		// Use @ for digests (sha256:, sha512:, etc.), : for tags
		if strings.Contains(i.Version, ":") {
			result += "@" + i.Version
		} else {
			result += ":" + i.Version
		}
	}
	return result
}
