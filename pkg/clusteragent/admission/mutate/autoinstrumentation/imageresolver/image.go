// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package imageresolver

// ResolvedImage represents a fully resolved image with digest and metadata.
type ResolvedImage struct {
	FullImageRef     string // e.g., "gcr.io/datadoghq/dd-lib-python-init@sha256:abc123..."
	CanonicalVersion string // e.g., "3.1.0"
}
