// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"encoding/json"
	"strings"
	"sync"

	rcclient "github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ImageResolver resolves container image references from tag-based to digest-based.
type ImageResolver interface {
	// Resolve takes a full image reference (e.g., "gcr.io/datadoghq/dd-lib-python-init:latest")
	// and returns a resolved image reference (e.g., "gcr.io/datadoghq/dd-lib-python-init@sha256:abc123")
	// If resolution fails or is not available, it returns ("", false).
	Resolve(imageRef string) (string, bool)
}

// NoOpImageResolver is a simple implementation that returns the original image unchanged.
// This is used when no remote config client is available.
type NoOpImageResolver struct{}

// NewNoOpImageResolver creates a new NoOpImageResolver.
func NewNoOpImageResolver() ImageResolver {
	return &NoOpImageResolver{}
}

// ResolveImage returns the original image reference.
func (r *NoOpImageResolver) Resolve(imageRef string) (string, bool) {
	log.Debugf("No resolution available for %s", imageRef)
	return "", false
}

// RemoteConfigImageResolver resolves image references using remote configuration data.
// It maintains a cache of image mappings received from the remote config service.
type RemoteConfigImageResolver struct {
	rcClient *rcclient.Client

	mu            sync.RWMutex
	imageMappings map[string]map[string]ResolvedImage // repository -> tag -> resolved image
}

// NewRemoteConfigImageResolver creates a new RemoteConfigImageResolver.
func NewRemoteConfigImageResolver(rcClient *rcclient.Client) ImageResolver {
	if rcClient == nil {
		log.Debugf("No remote config client available to resolve images")
		return NewNoOpImageResolver()
	}

	resolver := &RemoteConfigImageResolver{
		rcClient:      rcClient,
		imageMappings: make(map[string]map[string]ResolvedImage),
	}

	rcClient.Subscribe(state.ProductGradualRollout, resolver.processUpdate)
	log.Debugf("Subscribed to remote configuration")

	return resolver
}

// ResolveImage resolves a full image reference to a digest-based reference.
// If resolution fails or is not available, it returns the original image reference.
// Input: "gcr.io/datadoghq/dd-lib-python-init:latest"
// Output: "gcr.io/datadoghq/dd-lib-python-init@sha256:abc123..."
func (r *RemoteConfigImageResolver) Resolve(imageRef string) (string, bool) {
	repository, tag, err := parseImageReference(imageRef)
	if err != nil {
		log.Debugf("Failed to parse image reference %s: %v", imageRef, err)
		return "", false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.imageMappings) == 0 {
		log.Debugf("Cache empty, no resolution available for %s", imageRef)
		return "", false
	}

	repoCache, exists := r.imageMappings[repository]
	if !exists {
		log.Debugf("No mapping found for repository %s", repository)
		return "", false
	}

	resolved, exists := repoCache[tag]
	if !exists {
		log.Debugf("No mapping found for %s:%s", repository, tag)
		return "", false
	}

	log.Debugf("Resolved %s -> %s", imageRef, resolved.FullImageRef)
	return resolved.FullImageRef, true
}

// parseImageReference parses a full image reference into repository and tag.
// Examples:
//
//	"gcr.io/datadoghq/dd-lib-python-init:latest" -> ("dd-lib-python-init", "latest", nil)
//	"dd-lib-java-init:v1.0" -> ("dd-lib-java-init", "v1.0", nil)
//	"invalid-image" -> ("", "", error)
func parseImageReference(imageRef string) (repository, tag string, err error) {
	lastColon := strings.LastIndex(imageRef, ":")
	if lastColon == -1 {
		return "", "", &ImageParseError{imageRef: imageRef, reason: "no tag separator found"}
	}

	// Check if the part after ":" contains "/" which would indicate it's part of the repository (port number)
	potentialTag := imageRef[lastColon+1:]
	if strings.Contains(potentialTag, "/") {
		return "", "", &ImageParseError{imageRef: imageRef, reason: "tag contains slash, likely a port number"}
	}

	// Extract repository name from the full repository URL
	// For "gcr.io/datadoghq/dd-lib-python-init:latest", we want just "dd-lib-python-init"
	fullRepo := imageRef[:lastColon]

	// Find the last "/" to get just the repository name
	lastSlash := strings.LastIndex(fullRepo, "/")
	if lastSlash != -1 {
		repository = fullRepo[lastSlash+1:]
	} else {
		repository = fullRepo
	}

	tag = potentialTag
	return repository, tag, nil
}

// ImageParseError represents an error parsing an image reference.
type ImageParseError struct {
	imageRef string
	reason   string
}

// Error returns the image reference and the reason for the error.
func (e *ImageParseError) Error() string {
	return "failed to parse image reference '" + e.imageRef + "': " + e.reason
}

// processUpdate handles remote configuration updates for image resolution.
//
// NOTE:
// - Remote config maintains a complete state of all active configurations
// - When processUpdate is called, it receives the complete current state via GetConfigs()
// - If a repository is not in the update, it means it's no longer active
// - Therefore, replacing the entire cache ensures we stay in sync with remote config
func (r *RemoteConfigImageResolver) processUpdate(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	newCache := make(map[string]map[string]ResolvedImage)

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

		applyStateCallback(configKey, state.ApplyStatus{
			State: state.ApplyStateAcknowledged,
		})
	}

	r.mu.Lock()
	r.imageMappings = newCache
	r.mu.Unlock()
	log.Debugf("Updated cache with %d repositories", len(newCache))
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
		return NewRemoteConfigImageResolver(rcClient)
	}
	return NewNoOpImageResolver()
}
