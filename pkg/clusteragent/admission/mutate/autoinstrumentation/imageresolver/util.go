// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package imageresolver provides configuration and utilities for resolving
// container image references from mutable tags to digests.
package imageresolver

import (
	"strings"

	"github.com/opencontainers/go-digest"
)

func newDatadoghqRegistries(datadogRegistriesList []string) map[string]struct{} {
	datadoghqRegistries := make(map[string]struct{})
	for _, registry := range datadogRegistriesList {
		datadoghqRegistries[registry] = struct{}{}
	}
	return datadoghqRegistries
}

func isDatadoghqRegistry(registry string, datadoghqRegistries map[string]struct{}) bool {
	_, exists := datadoghqRegistries[registry]
	return exists
}

// isValidDigest validates that a digest string follows the OCI image specification format
// and is a sha256 digest.
func isValidDigest(digestStr string) bool {
	if _, err := digest.Parse(digestStr); err != nil {
		return false
	}
	return strings.HasPrefix(digestStr, "sha256:")
}
