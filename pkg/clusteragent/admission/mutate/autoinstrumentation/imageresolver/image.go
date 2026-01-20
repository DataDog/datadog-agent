// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package imageresolver

// ImageInfo represents information about an image from remote configuration.
type ImageInfo struct {
	Tag              string `json:"tag"`
	Digest           string `json:"digest"`
	CanonicalVersion string `json:"canonical_version"`
}

// RepositoryConfig represents a repository configuration from remote config.
type RepositoryConfig struct {
	RepositoryName string      `json:"repository_name"`
	Images         []ImageInfo `json:"images"`
}

// ResolvedImage represents a fully resolved image with digest and metadata.
type ResolvedImage struct {
	FullImageRef     string // e.g., "gcr.io/datadoghq/dd-lib-python-init@sha256:abc123..."
	CanonicalVersion string // e.g., "3.1.0"
}

func newResolvedImage(registry string, repositoryName string, imageInfo ImageInfo) *ResolvedImage {
	return &ResolvedImage{
		FullImageRef:     registry + "/" + repositoryName + "@" + imageInfo.Digest,
		CanonicalVersion: imageInfo.CanonicalVersion,
	}
}
