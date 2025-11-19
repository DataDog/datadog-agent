// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package imageresolver

import (
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/opencontainers/go-digest"
)

// ImageResolver resolves container image references from tag-based to digest-based.
type ImageResolver interface {
	// Resolve takes a registry, repository, and tag string (e.g., "gcr.io/datadoghq", "dd-lib-python-init", "v3")
	// and returns a resolved image reference (e.g., "gcr.io/datadoghq/dd-lib-python-init@sha256:abc123")
	// If resolution fails or is not available, it returns the original image reference ("gcr.io/datadoghq/dd-lib-python-init:v3", false).
	Resolve(registry string, repository string, tag string) (*ResolvedImage, bool)
}

func NewImageResolver(cfg ImageResolverConfig) ImageResolver {
	if cfg.RCClient != nil && !reflect.ValueOf(cfg.RCClient).IsNil() {
		log.Debugf("Remote config client available")
		return newRcResolver(cfg)
	}
	return newNoOpImageResolver()
}

func isDatadoghqRegistry(registry string, datadoghqRegistries map[string]any) bool {
	_, exists := datadoghqRegistries[registry]
	return exists
}

// isValidDigest validates that a digest string follows the OCI image specification format
func isValidDigest(digestStr string) bool {
	_, err := digest.Parse(digestStr)
	return err == nil
}
