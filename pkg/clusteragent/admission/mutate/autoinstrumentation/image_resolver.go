// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// RemoteConfigClient defines the interface we need for remote config operations
type RemoteConfigClient interface {
	GetConfigs(product string) map[string]state.RawConfig
	Subscribe(product string, callback func(map[string]state.RawConfig, func(string, state.ApplyStatus)))
}

// ImageResolver resolves container image references from tag-based to digest-based.
type ImageResolver interface {
	// Resolve takes a registry, repository, and tag string (e.g., "gcr.io/datadoghq", "dd-lib-python-init", "v3")
	// and returns a resolved image reference (e.g., "gcr.io/datadoghq/dd-lib-python-init@sha256:abc123")
	// If resolution fails or is not available, it returns the original image reference ("gcr.io/datadoghq/dd-lib-python-init:v3", false).
	Resolve(registry string, repository string, tag string) (*ResolvedImage, bool)
}

// noOpImageResolver is a simple implementation that returns the original image unchanged.
// This is used when no remote config client is available.
type noOpImageResolver struct{}

// newNoOpImageResolver creates a new noOpImageResolver.
func newNoOpImageResolver() ImageResolver {
	return &noOpImageResolver{}
}

// ResolveImage returns the original image reference.
func (r *noOpImageResolver) Resolve(registry string, repository string, tag string) (*ResolvedImage, bool) {
	log.Debugf("Cannot resolve %s/%s:%s without remote config", registry, repository, tag)
	return nil, false
}

// remoteConfigImageResolver resolves image references using remote configuration data.
// It maintains a cache of image mappings received from the remote config service.
type remoteConfigImageResolver struct {
	rcClient RemoteConfigClient

	mu            sync.RWMutex
	imageMappings map[string]map[string]ResolvedImage // repository URL -> tag -> resolved image

	// Retry configuration for initial cache loading
	maxRetries int
	retryDelay time.Duration
}

// newRemoteConfigImageResolver creates a new remoteConfigImageResolver.
// Assumes rcClient is non-nil.
func newRemoteConfigImageResolver(rcClient RemoteConfigClient) ImageResolver {
	return newRemoteConfigImageResolverWithRetryConfig(rcClient, 5, 1*time.Second)
}

// newRemoteConfigImageResolverWithRetryConfig creates a resolver with configurable retry behavior.
// Useful for testing with faster retry settings.
func newRemoteConfigImageResolverWithRetryConfig(rcClient RemoteConfigClient, maxRetries int, retryDelay time.Duration) ImageResolver {
	resolver := &remoteConfigImageResolver{
		rcClient:      rcClient,
		imageMappings: make(map[string]map[string]ResolvedImage),
		maxRetries:    maxRetries,
		retryDelay:    retryDelay,
	}

	rcClient.Subscribe(state.ProductGradualRollout, resolver.processUpdate)
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

func (r *remoteConfigImageResolver) waitForInitialConfig() error {
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
// Output: "gcr.io/datadoghq/dd-lib-python-init@sha256:abc123...", true
// If resolution fails or is not available, it returns nil.
// Output: nil, false
func (r *remoteConfigImageResolver) Resolve(registry string, repository string, tag string) (*ResolvedImage, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.imageMappings) == 0 {
		// log.Debugf("Cache empty, no resolution available")
		return nil, false
	}

	requestedURL := registry + "/" + repository

	repoCache, exists := r.imageMappings[requestedURL]
	if !exists {
		log.Debugf("No mapping found for repository URL %s", requestedURL)
		return nil, false
	}

	normalizedTag := strings.TrimPrefix(tag, "v")

	resolved, exists := repoCache[normalizedTag]
	if !exists {
		log.Debugf("No mapping found for %s:%s", requestedURL, normalizedTag)
		return nil, false
	}

	log.Debugf("Resolved %s:%s -> %s", requestedURL, tag, resolved.FullImageRef)
	return &resolved, true
}

// updateCache processes configuration data and updates the image mappings cache.
// This is the core logic shared by both initialization and remote config updates.
func (r *remoteConfigImageResolver) updateCache(configs map[string]state.RawConfig) {
	validConfigs, errors := parseAndValidateConfigs(configs)

	for configKey, err := range errors {
		log.Errorf("Failed to process config %s during initialization: %v", configKey, err)
	}

	r.updateCacheFromParsedConfigs(validConfigs)
}

func parseAndValidateConfigs(configs map[string]state.RawConfig) (map[string]RepositoryConfig, map[string]error) {
	validConfigs := make(map[string]RepositoryConfig)
	errors := make(map[string]error)
	for configKey, rawConfig := range configs {
		var repo RepositoryConfig
		if err := json.Unmarshal(rawConfig.Config, &repo); err != nil {
			errors[configKey] = fmt.Errorf("failed to unmarshal: %w", err)
			continue
		}

		if repo.RepositoryName == "" || repo.RepositoryURL == "" {
			errors[configKey] = fmt.Errorf("missing repository_name or repository_url in config %s", configKey)
			continue
		}
		validConfigs[configKey] = repo
	}
	return validConfigs, errors
}

func (r *remoteConfigImageResolver) updateCacheFromParsedConfigs(validConfigs map[string]RepositoryConfig) {
	newCache := make(map[string]map[string]ResolvedImage)

	for _, repo := range validConfigs {
		tagMap := make(map[string]ResolvedImage)
		for _, imageInfo := range repo.Images {
			if imageInfo.Tag == "" || imageInfo.Digest == "" {
				log.Warnf("Skipping invalid image entry (missing tag or digest) in %s", repo.RepositoryURL)
				continue
			}

			fullImageRef := repo.RepositoryURL + "@" + imageInfo.Digest
			tagMap[imageInfo.Tag] = ResolvedImage{
				FullImageRef:     fullImageRef,
				Digest:           imageInfo.Digest,
				CanonicalVersion: imageInfo.CanonicalVersion,
			}
		}

		newCache[repo.RepositoryURL] = tagMap
		log.Debugf("Processed config for repository %s with %d images", repo.RepositoryURL, len(tagMap))
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
func (r *remoteConfigImageResolver) processUpdate(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
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

// ImageInfo represents information about an image from remote configuration.
type ImageInfo struct {
	Tag              string `json:"tag"`
	Digest           string `json:"digest"`
	CanonicalVersion string `json:"canonical_version"`
}

// RepositoryConfig represents a repository configuration from remote config.
type RepositoryConfig struct {
	RepositoryURL  string      `json:"repository_url"`
	RepositoryName string      `json:"repository_name"`
	Images         []ImageInfo `json:"images"`
}

// ResolvedImage represents a resolved image with digest and metadata.
type ResolvedImage struct {
	FullImageRef     string // e.g., "gcr.io/project/image@sha256:abc123..."
	Digest           string
	CanonicalVersion string
}

// NewImageResolver creates the appropriate ImageResolver based on whether
// a remote config client is available.
func NewImageResolver(rcClient RemoteConfigClient) ImageResolver {
	if rcClient != nil {
		return newRemoteConfigImageResolver(rcClient)
	}
	log.Debugf("No remote config client available")
	return newNoOpImageResolver()
}
