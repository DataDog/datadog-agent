// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package imageresolver

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
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

// NewNoOpResolver creates a new noOpImageResolver.
// This is useful for testing or when image resolution is not needed.
func NewNoOpResolver() Resolver {
	return &noOpResolver{}
}

// ResolveImage returns the original image reference.
func (r *noOpResolver) Resolve(registry string, repository string, tag string) (*ResolvedImage, bool) {
	log.Debugf("Cannot resolve %s/%s:%s without remote config", registry, repository, tag)
	metrics.ImageResolutionAttempts.Inc(repository, tag, tag)
	return nil, false
}

// remoteConfigImageResolver resolves image references using remote configuration data.
// It maintains a cache of image mappings received from the remote config service.
type rcResolver struct {
	rcClient RemoteConfigClient

	mu                  sync.RWMutex
	imageMappings       map[string]map[string]ImageInfo // repository name -> tag -> resolved image
	datadoghqRegistries map[string]struct{}

	// Retry configuration for initial cache loading
	maxRetries int
	retryDelay time.Duration
}

func newRcResolver(cfg Config) Resolver {
	resolver := &rcResolver{
		rcClient:            cfg.RCClient,
		imageMappings:       make(map[string]map[string]ImageInfo),
		maxRetries:          cfg.MaxInitRetries,
		retryDelay:          cfg.InitRetryDelay,
		datadoghqRegistries: cfg.DDRegistries,
	}

	resolver.rcClient.Subscribe(state.ProductGradualRollout, resolver.processUpdate)
	log.Debugf("Subscribed to %s", state.ProductGradualRollout)

	go func() {
		if err := resolver.waitForInitialConfig(); err != nil {
			log.Warnf("Failed to load initial image resolution config: %v. Image resolution will remain disabled.", err)
		} else {
			log.Infof("Image resolution cache initialized successfully")
		}
	}()

	return resolver
}

func (r *rcResolver) waitForInitialConfig() error {
	if currentConfigs := r.rcClient.GetConfigs(state.ProductGradualRollout); len(currentConfigs) > 0 {
		log.Debugf("Initial configs available immediately: %d configurations", len(currentConfigs))
		r.updateCache(currentConfigs)
		return nil
	}

	for attempt := 1; attempt <= r.maxRetries; attempt++ {
		time.Sleep(r.retryDelay)

		currentConfigs := r.rcClient.GetConfigs(state.ProductGradualRollout)
		if len(currentConfigs) > 0 {
			log.Infof("Loaded initial image resolution config after %d attempts: %d configurations", attempt, len(currentConfigs))
			r.updateCache(currentConfigs)
			return nil
		}

		log.Debugf("Attempt %d/%d: Still waiting for initial remote config, retrying in %v", attempt, r.maxRetries, r.retryDelay)
	}

	return fmt.Errorf("failed to load initial remote config after %d attempts (%v total wait)", r.maxRetries, time.Duration(r.maxRetries)*r.retryDelay)
}

// Resolve resolves a registry, repository, and tag to a digest-based reference.
// Input: "gcr.io/datadoghq", "dd-lib-python-init", "v3"
// Output: "dd-lib-python-init@sha256:abc123...", true
// If resolution fails or is not available, it returns nil.
// Output: nil, false
func (r *rcResolver) Resolve(registry string, repository string, tag string) (*ResolvedImage, bool) {
	if !isDatadoghqRegistry(registry, r.datadoghqRegistries) {
		log.Debugf("Not a Datadoghq registry, not resolving")
		return nil, false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.imageMappings) == 0 {
		log.Debugf("Cache empty, no resolution available")
		metrics.ImageResolutionAttempts.Inc(repository, tag, tag)
		return nil, false
	}

	repoCache, exists := r.imageMappings[repository]
	if !exists {
		log.Debugf("No mapping found for repository %s", repository)
		metrics.ImageResolutionAttempts.Inc(repository, tag, tag)
		return nil, false
	}

	normalizedTag := strings.TrimPrefix(tag, "v")

	resolved, exists := repoCache[normalizedTag]
	if !exists {
		log.Debugf("No mapping found for %s:%s", repository, normalizedTag)
		metrics.ImageResolutionAttempts.Inc(repository, tag, tag)
		return nil, false
	}
	resolvedImage := newResolvedImage(registry, repository, resolved)
	log.Debugf("Resolved %s/%s:%s -> %s", registry, repository, tag, resolvedImage.FullImageRef)
	metrics.ImageResolutionAttempts.Inc(repository, tag, resolved.Digest)
	return resolvedImage, true
}

// updateCache processes configuration data and updates the image mappings cache.
// This is the core logic shared by both initialization and remote config updates.
func (r *rcResolver) updateCache(configs map[string]state.RawConfig) {
	validConfigs, errors := parseAndValidateConfigs(configs)

	for configKey, err := range errors {
		log.Errorf("Failed to process config %s during initialization: %v", configKey, err)
	}

	r.updateCacheFromParsedConfigs(validConfigs)
}

func (r *rcResolver) updateCacheFromParsedConfigs(validConfigs map[string]RepositoryConfig) {
	newCache := make(map[string]map[string]ImageInfo)

	for _, repo := range validConfigs {
		tagMap := make(map[string]ImageInfo)
		for _, imageInfo := range repo.Images {
			if imageInfo.Tag == "" || imageInfo.Digest == "" {
				log.Warnf("Skipping invalid image entry (missing tag or digest) in %s", repo.RepositoryName)
				continue
			}

			if !isValidDigest(imageInfo.Digest) {
				log.Warnf("Skipping invalid image entry (invalid digest format: %s) in %s", imageInfo.Digest, repo.RepositoryName)
				continue
			}

			tagMap[imageInfo.Tag] = imageInfo
		}

		newCache[repo.RepositoryName] = tagMap
		log.Debugf("Processed config for repository %s with %d images", repo.RepositoryName, len(tagMap))
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.imageMappings = newCache
}

// processUpdate handles remote configuration updates for image resolution.
//
// NOTE:
// - Remote config maintains a complete state of all active configurations
// - When processUpdate is called, it receives the complete current state via GetConfigs()
// - If a repository is not in the update, it means it's no longer active
// - Therefore, replacing the entire cache ensures we stay in sync with remote config
func (r *rcResolver) processUpdate(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	validConfigs, errors := parseAndValidateConfigs(update)

	r.updateCacheFromParsedConfigs(validConfigs)

	for configKey := range update {
		if err, hasError := errors[configKey]; hasError {
			applyStateCallback(configKey, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
		} else {
			applyStateCallback(configKey, state.ApplyStatus{
				State: state.ApplyStateAcknowledged,
			})
		}
	}
}

type bucketTagResolver struct {
	cache               *httpDigestCache
	bucketID            string
	datadoghqRegistries map[string]struct{}
}

func (r *bucketTagResolver) createBucketTag(tag string) string {
	normalizedTag := strings.TrimPrefix(tag, "v")

	// DEV: Only create bucket tag for major versions (single number like "1" or "v1")
	if !strings.Contains(normalizedTag, ".") {
		return fmt.Sprintf("%s-gr%s", normalizedTag, r.bucketID)
	}
	return normalizedTag
}

func (r *bucketTagResolver) Resolve(registry string, repository string, tag string) (*ResolvedImage, bool) {
	if !isDatadoghqRegistry(registry, r.datadoghqRegistries) {
		log.Debugf("%s is not a Datadoghq registry, not resolving", registry)
		metrics.ImageResolutionAttempts.Inc(repository, tag, tag)
		return nil, false
	}

	bucketTag := r.createBucketTag(tag)

	digest, err := r.cache.get(registry, repository, bucketTag)
	if err != nil {
		log.Debugf("cache miss for %s/%s:%s - %v", registry, repository, bucketTag, err)
		metrics.ImageResolutionAttempts.Inc(repository, bucketTag, tag)
		return nil, false
	}
	log.Debugf("cache hit for %s/%s:%s", registry, repository, bucketTag)
	metrics.ImageResolutionAttempts.Inc(repository, bucketTag, digest)
	return &ResolvedImage{
		FullImageRef:     registry + "/" + repository + "@" + digest,
		CanonicalVersion: tag, // DEV: This is the customer provided tag
	}, true
}

func newBucketTagResolver(cfg Config) *bucketTagResolver {
	return &bucketTagResolver{
		cache:               newHTTPDigestCache(cfg.DigestCacheTTL, cfg.DDRegistries),
		bucketID:            cfg.BucketID,
		datadoghqRegistries: cfg.DDRegistries,
	}
}

// New creates the appropriate Resolver based on whether
// a remote config client is available.
func New(cfg Config) Resolver {
	if cfg.RCClient == nil || reflect.ValueOf(cfg.RCClient).IsNil() {
		log.Debugf("No remote config client available")
		return NewNoOpResolver()
	}
	return newRcResolver(cfg)
}
