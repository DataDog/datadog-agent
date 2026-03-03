// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package imageresolver

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Resolver resolves container image references from tag-based to digest-based.
type Resolver interface {
	// Resolve takes a registry, repository, and tag string (e.g., "gcr.io/datadoghq", "dd-lib-python-init", "v3")
	// and returns a resolved image reference (e.g., "gcr.io/datadoghq/dd-lib-python-init@sha256:abc123")
	// If resolution fails or is not available, it returns the original image reference ("gcr.io/datadoghq/dd-lib-python-init:v3", false).
	Resolve(registry string, repository string, tag string) (*ResolvedImage, bool)
}

// noOpImageResolver is a simple implementation that returns the original image unchanged.
// This is used when no remote config client is available.
type noOpResolver struct{}

var _ Resolver = (*noOpResolver)(nil)

// NewNoOpResolver creates a new noOpImageResolver.
// This is useful for testing or when image resolution is not needed.
func NewNoOpResolver() Resolver {
	return &noOpResolver{}
}

// ResolveImage returns the original image reference.
func (r *noOpResolver) Resolve(_ string, repository string, tag string) (*ResolvedImage, bool) {
	metrics.ImageResolutionAttempts.Inc(repository, tag, "-", tag)
	return nil, false
}

// bucketTagResolver resolves image references using bucket tags.
// It maintains a cache of image digests and resolves bucket tags to digests if possible.
// Defaults to returning the original tag if resolution fails for any reason.
type bucketTagResolver struct {
	cache               *httpDigestCache
	bucketID            string
	datadoghqRegistries map[string]struct{}
}

var _ Resolver = (*bucketTagResolver)(nil)

func (r *bucketTagResolver) createBucketTag(tag string) string {
	normalizedTag := strings.TrimPrefix(tag, "v")

	// DEV: Only create bucket tag for major versions (single number like "1" or "v1")
	if !strings.Contains(normalizedTag, ".") {
		return fmt.Sprintf("%s-gr%s", normalizedTag, r.bucketID)
	}
	return normalizedTag
}

func (r *bucketTagResolver) Resolve(registry string, repository string, tag string) (*ResolvedImage, bool) {
	normalizedTag := strings.TrimPrefix(tag, "v")
	var result = normalizedTag
	defer func() {
		metrics.ImageResolutionAttempts.Inc(repository, normalizedTag, r.bucketID, result)
	}()
	if !isDatadoghqRegistry(registry, r.datadoghqRegistries) {
		log.Debugf("%s is not a Datadoghq registry, not opting into gradual rollout", registry)
		return nil, false
	}

	bucketTag := r.createBucketTag(normalizedTag)

	digest, err := r.cache.get(registry, repository, bucketTag)
	if err != nil {
		log.Debugf("Failed to resolve %s/%s:%s for gradual rollout - %v", registry, repository, bucketTag, err)
		return nil, false
	}
	result = digest
	return &ResolvedImage{
		FullImageRef:     registry + "/" + repository + "@" + digest,
		CanonicalVersion: tag, // DEV: This is the customer provided tag
	}, true
}

func newBucketTagResolver(cfg Config) *bucketTagResolver {
	rt := http.DefaultTransport.(*http.Transport).Clone()
	return &bucketTagResolver{
		cache:               newHTTPDigestCache(cfg.DigestCacheTTL, cfg.DDRegistries, rt),
		bucketID:            cfg.BucketID,
		datadoghqRegistries: cfg.DDRegistries,
	}
}

// New creates the appropriate Resolver based on whether
// gradual rollout is enabled.
func New(cfg Config) Resolver {
	if !cfg.Enabled {
		return NewNoOpResolver()
	}
	return newBucketTagResolver(cfg)
}
