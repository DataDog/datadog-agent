// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/opencontainers/go-digest"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Following rollout ordering from SRE
var rolloutBucketMapping = map[string]int{
	"datad0g.com":       1,
	"ap1.datadoghq.com": 1,
	"ap2.datadoghq.com": 2,
	"us3.datadoghq.com": 2,
	"us5.datadoghq.com": 3,
	"datadoghq.eu":      4,
	"datadoghq.com":     5,
	"ddog-gov.com":      6,
}

var numRolloutBuckets = 6
var ttlDuration = 24 * time.Hour

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
	metrics.ImageResolutionAttempts.Inc(registry, repository, metrics.DigestResolutionDisabled, tag)
	return nil, false
}

// remoteConfigImageResolver resolves image references using remote configuration data.
// It maintains a cache of image mappings received from the remote config service.
type remoteConfigImageResolver struct {
	rcClient RemoteConfigClient

	mu                  sync.RWMutex
	imageMappings       map[string]map[string]ImageInfo // repository name -> tag -> resolved image
	datadoghqRegistries map[string]struct{}

	// Retry configuration for initial cache loading
	maxRetries int
	retryDelay time.Duration
}

// newRemoteConfigImageResolver creates a new remoteConfigImageResolver.
// Assumes rcClient is non-nil.
func newRemoteConfigImageResolver(rcClient RemoteConfigClient, datadoghqRegistries map[string]struct{}) ImageResolver {
	return newRemoteConfigImageResolverWithRetryConfig(rcClient, 5, 1*time.Second, datadoghqRegistries)
}

// newRemoteConfigImageResolverWithRetryConfig creates a resolver with configurable retry behavior.
// Useful for testing with faster retry settings.
func newRemoteConfigImageResolverWithRetryConfig(rcClient RemoteConfigClient, maxRetries int, retryDelay time.Duration, datadoghqRegistries map[string]struct{}) ImageResolver {
	resolver := &remoteConfigImageResolver{
		rcClient:            rcClient,
		imageMappings:       make(map[string]map[string]ImageInfo),
		maxRetries:          maxRetries,
		retryDelay:          retryDelay,
		datadoghqRegistries: datadoghqRegistries,
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
// Output: "dd-lib-python-init@sha256:abc123...", true
// If resolution fails or is not available, it returns nil.
// Output: nil, false
func (r *remoteConfigImageResolver) Resolve(registry string, repository string, tag string) (*ResolvedImage, bool) {
	if !isDatadoghqRegistry(registry, r.datadoghqRegistries) {
		log.Debugf("Not a Datadoghq registry, not resolving")
		return nil, false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.imageMappings) == 0 {
		log.Debugf("Cache empty, no resolution available")
		metrics.ImageResolutionAttempts.Inc(registry, repository, metrics.DigestResolutionEnabled, tag)
		return nil, false
	}

	repoCache, exists := r.imageMappings[repository]
	if !exists {
		log.Debugf("No mapping found for repository %s", repository)
		metrics.ImageResolutionAttempts.Inc(registry, repository, metrics.DigestResolutionEnabled, tag)
		return nil, false
	}

	normalizedTag := strings.TrimPrefix(tag, "v")

	resolved, exists := repoCache[normalizedTag]
	if !exists {
		log.Debugf("No mapping found for %s:%s", repository, normalizedTag)
		metrics.ImageResolutionAttempts.Inc(registry, repository, metrics.DigestResolutionEnabled, tag)
		return nil, false
	}
	resolvedImage := newResolvedImage(registry, repository, resolved)
	log.Debugf("Resolved %s/%s:%s -> %s", registry, repository, tag, resolvedImage.FullImageRef)
	metrics.ImageResolutionAttempts.Inc(registry, repository, metrics.DigestResolutionEnabled, resolved.Digest)
	return resolvedImage, true
}

func isDatadoghqRegistry(registry string, datadoghqRegistries map[string]struct{}) bool {
	_, exists := datadoghqRegistries[registry]
	return exists
}

// isValidDigest validates that a digest string follows the OCI image specification format
func isValidDigest(digestStr string) bool {
	_, err := digest.Parse(digestStr)
	return err == nil
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

		if repo.RepositoryName == "" {
			errors[configKey] = fmt.Errorf("missing repository_name in config %s", configKey)
			continue
		}
		validConfigs[configKey] = repo
	}
	return validConfigs, errors
}

func (r *remoteConfigImageResolver) updateCacheFromParsedConfigs(validConfigs map[string]RepositoryConfig) {
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
	RepositoryName string      `json:"repository_name"`
	Images         []ImageInfo `json:"images"`
}

// ResolvedImage represents a fully resolved image.
type ResolvedImage struct {
	FullImageRef string // e.g., "gcr.io/datadoghq/dd-lib-python-init@v3-rollout1"
}

func newResolvedImage(registry string, repositoryName string, imageInfo ImageInfo) *ResolvedImage {
	return &ResolvedImage{
		FullImageRef: registry + "/" + repositoryName + "@" + imageInfo.Digest,
	}
}

func newTagBasedImage(registry string, repository string, rolloutTag string) *ResolvedImage {
	return &ResolvedImage{
		FullImageRef: registry + "/" + repository + ":" + rolloutTag,
	}
}

// digestCacheEntry represents a cached digest with its last update time
type digestCacheEntry struct {
	digest      string
	lastUpdated time.Time
}

type tagBasedImageResolver struct {
	datadoghqRegistries map[string]any
	datacenter          string
	rolloutBucket       string
	digestCache         map[string]digestCacheEntry
	mu                  sync.RWMutex
}

func (r *tagBasedImageResolver) Resolve(registry string, repository string, tag string) (*ResolvedImage, bool) {
	if !isDatadoghqRegistry(registry, r.datadoghqRegistries) {
		log.Debugf("Not a Datadoghq registry, not resolving")
		return nil, false
	}

	normalizedTag := strings.TrimPrefix(tag, "v")
	rolloutBucketTag := normalizedTag + "-rollout" + r.rolloutBucket
	// Check cache first
	cacheKey := registry + "/" + repository + ":" + rolloutBucketTag
	if cachedDigest, found := r.getCachedDigest(cacheKey); found {
		if entryAge, _ := r.getCacheEntryAge(cacheKey); entryAge > ttlDuration {
			log.Debugf("Cached digest for %s is too old, fetching from registry", cacheKey)
		} else {
			log.Debugf("Using cached digest for %s", cacheKey)
			return newResolvedImage(registry, repository, ImageInfo{Digest: cachedDigest}), true
		}
	}

	// Fetch from registry
	digest, err := crane.Digest(registry + "/" + repository + ":" + rolloutBucketTag)
	if err != nil {
		log.Errorf("Failed to get digest of image %s/%s:%s: %v", registry, repository, rolloutBucketTag, err)
		return nil, false
	}
	r.setCachedDigest(cacheKey, digest)

	return newResolvedImage(registry, repository, ImageInfo{Digest: digest}), true
}

// getCachedDigest retrieves a digest from the cache if it exists
func (r *tagBasedImageResolver) getCachedDigest(cacheKey string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if entry, exists := r.digestCache[cacheKey]; exists {
		return entry.digest, true
	}
	return "", false
}

// setCachedDigest stores a digest in the cache with the current timestamp
func (r *tagBasedImageResolver) setCachedDigest(cacheKey string, digest string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.digestCache[cacheKey] = digestCacheEntry{
		digest:      digest,
		lastUpdated: time.Now(),
	}
}

// getCacheEntryAge returns how long ago a cache entry was last updated
func (r *tagBasedImageResolver) getCacheEntryAge(cacheKey string) (time.Duration, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if entry, exists := r.digestCache[cacheKey]; exists {
		return time.Since(entry.lastUpdated), true
	}
	return 0, false
}

func newTagBasedImageResolver(datadoghqRegistries map[string]any, datacenter string) ImageResolver {
	rolloutBucket, exists := rolloutBucketMapping[datacenter]
	if !exists {
		// Fallback to the last bucket if the datacenter is not found
		rolloutBucket = numRolloutBuckets
	}
	return &tagBasedImageResolver{
		datadoghqRegistries: datadoghqRegistries,
		datacenter:          datacenter,
		rolloutBucket:       strconv.Itoa(rolloutBucket),
		digestCache:         make(map[string]digestCacheEntry),
	}
}

// NewImageResolver creates the appropriate ImageResolver based on whether
// a remote config client is available.
// TODO(erika): ImageResolveConfig should be passed in here
func NewImageResolver(rcClient RemoteConfigClient, cfg config.Component) ImageResolver {
	datadogRegistriesSet := cfg.GetStringMap("admission_controller.auto_instrumentation.default_dd_registries")

	if rcClient == nil || reflect.ValueOf(rcClient).IsNil() {
		log.Debugf("No remote config client available")
		datacenter := cfg.GetString("site")
		return newTagBasedImageResolver(datadogRegistriesSet, datacenter)
	}

	datadogRegistriesList := cfg.GetStringSlice("admission_controller.auto_instrumentation.default_dd_registries")
	datadogRegistries := newDatadoghqRegistries(datadogRegistriesList)

	return newRemoteConfigImageResolverWithDefaultDatadoghqRegistries(rcClient, datadogRegistries)
}

func newRemoteConfigImageResolverWithDefaultDatadoghqRegistries(rcClient RemoteConfigClient, datadoghqRegistries map[string]struct{}) ImageResolver {
	resolver := newRemoteConfigImageResolver(rcClient, datadoghqRegistries)
	resolver.(*remoteConfigImageResolver).datadoghqRegistries = datadoghqRegistries
	return resolver
}

func newDatadoghqRegistries(datadogRegistriesList []string) map[string]struct{} {
	datadoghqRegistries := make(map[string]struct{})
	for _, registry := range datadogRegistriesList {
		datadoghqRegistries[registry] = struct{}{}
	}
	return datadoghqRegistries
}
