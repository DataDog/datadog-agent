// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"encoding/json"
	"slices"
	"sync"

	rcclient "github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

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
func (r *noOpImageResolver) Resolve(_ string, repository string, tag string) (*ResolvedImage, bool) {
	log.Debugf("No resolution available, returning original image reference %s:%s", repository, tag)
	return nil, false
}

// remoteConfigImageResolver resolves image references using remote configuration data.
// It maintains a cache of image mappings received from the remote config service.
type remoteConfigImageResolver struct {
	rcClient *rcclient.Client

	mu            sync.RWMutex
	imageMappings map[string]map[string]ResolvedImage // repository -> tag -> resolved image
}

// newRemoteConfigImageResolver creates a new remoteConfigImageResolver.
// Assumes rcClient is non-nil.
func newRemoteConfigImageResolver(rcClient *rcclient.Client) ImageResolver {
	resolver := &remoteConfigImageResolver{
		rcClient:      rcClient,
		imageMappings: make(map[string]map[string]ResolvedImage),
	}

	// Load initial configurations
	currentConfigs := rcClient.GetConfigs(state.ProductGradualRollout)
	if len(currentConfigs) > 0 {
		log.Debugf("Loading initial state: %d configurations", len(currentConfigs))
		resolver.updateCache(currentConfigs)
	} else {
		log.Debugf("No initial configurations found for %s", state.ProductGradualRollout)
	}

	// Subscribe to future updates
	rcClient.Subscribe(state.ProductGradualRollout, resolver.processUpdate)
	log.Debugf("Subscribed to %s", state.ProductGradualRollout)

	return resolver
}

// Resolve resolves a registry, repository, and tag to a digest-based reference.
// Input: "gcr.io/datadoghq", "dd-lib-python-init", "v3"
// Output: "gcr.io/datadoghq/dd-lib-python-init@sha256:abc123...", true
// If resolution fails or is not available, it returns the original image reference.
// Output: "gcr.io/datadoghq/dd-lib-python-init:v3", false
func (r *remoteConfigImageResolver) Resolve(registry string, repository string, tag string) (*ResolvedImage, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if !isDatadoghqRegistry(registry) {
		log.Debugf("Not a Datadoghq registry, not resolving")
		return nil, false
	}

	if len(r.imageMappings) == 0 {
		log.Debugf("Cache empty, no resolution available")
		return nil, false
	}

	log.Debugf("TEST CACHE: %v", r.imageMappings)

	repoCache, exists := r.imageMappings[repository]
	if !exists {
		log.Debugf("No mapping found for repository %s", repository)
		return nil, false
	}

	resolved, exists := repoCache[tag]
	if !exists {
		log.Debugf("No mapping found for %s:%s", repository, tag)
		return nil, false
	}

	log.Debugf("Resolved %s:%s -> %s", repository, tag, resolved)
	return &resolved, true
}

func isDatadoghqRegistry(registry string) bool {
	var datadoghqRegistries = []string{
		"gcr.io/datadoghq",
	}
	return slices.Contains(datadoghqRegistries, registry)
}

// updateCache processes configuration data and updates the image mappings cache.
// This is the core logic shared by both initialization and remote config updates.
func (r *remoteConfigImageResolver) updateCache(configs map[string]state.RawConfig) {
	newCache := make(map[string]map[string]ResolvedImage)

	for configKey, rawConfig := range configs {
		var repo RepositoryConfig
		if err := json.Unmarshal(rawConfig.Config, &repo); err != nil {
			log.Errorf("Failed to unmarshal repository config for %s: %v", configKey, err)
			continue
		}

		if repo.RepositoryName == "" || repo.RepositoryURL == "" {
			log.Errorf("Missing repository_name or repository_url in config %s", configKey)
			continue
		}

		tagMap := make(map[string]ResolvedImage)
		for _, imageInfo := range repo.Images {
			if imageInfo.Tag == "" || imageInfo.Digest == "" {
				log.Warnf("Skipping invalid image entry (missing tag or digest) in %s", repo.RepositoryName)
				continue
			}

			fullImageRef := repo.RepositoryURL + "@" + imageInfo.Digest
			tagMap[imageInfo.Tag] = ResolvedImage{
				FullImageRef:     fullImageRef,
				Digest:           imageInfo.Digest,
				CanonicalVersion: imageInfo.CanonicalVersion,
			}
		}
		newCache[repo.RepositoryName] = tagMap
		log.Debugf("Processed config for repository %s with %d images", repo.RepositoryName, len(tagMap))
	}

	r.mu.Lock()
	r.imageMappings = newCache
	r.mu.Unlock()
	log.Debugf("Updated cache with %d repositories", len(newCache))
}

// processUpdate handles remote configuration updates for image resolution.
//
// NOTE:
// - Remote config maintains a complete state of all active configurations
// - When processUpdate is called, it receives the complete current state via GetConfigs()
// - If a repository is not in the update, it means it's no longer active
// - Therefore, replacing the entire cache ensures we stay in sync with remote config
func (r *remoteConfigImageResolver) processUpdate(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	for configKey, rawConfig := range update {
		var repo RepositoryConfig
		if err := json.Unmarshal(rawConfig.Config, &repo); err != nil {
			log.Errorf("Failed to unmarshal repository config for %s: %v", configKey, err)
			applyStateCallback(configKey, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
			continue
		}

		if repo.RepositoryName == "" || repo.RepositoryURL == "" {
			log.Errorf("Missing repository_name or repository_url in config %s", configKey)
			applyStateCallback(configKey, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: "Missing repository_name or repository_url",
			})
			continue
		}

		applyStateCallback(configKey, state.ApplyStatus{
			State: state.ApplyStateAcknowledged,
		})
	}

	r.updateCache(update)
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
func NewImageResolver(rcClient *rcclient.Client) ImageResolver {
	if rcClient != nil {
		return newRemoteConfigImageResolver(rcClient)
	}
	log.Debugf("No remote config client available")
	return newNoOpImageResolver()
}
