// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"encoding/json"
	"sync"

	rcclient "github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TagResolver is a component that resolves image tags from a remote configuration.
type TagResolver struct {
	rcClient *rcclient.Client

	mu          sync.RWMutex
	tagMappings map[string]map[string]ResolvedImage
}

// NewTagResolver creates a new TagResolver instance.
func NewTagResolver(rcClient *rcclient.Client) *TagResolver {
	tr := &TagResolver{
		rcClient:    rcClient,
		tagMappings: make(map[string]map[string]ResolvedImage),
	}
	if rcClient != nil {
		rcClient.Subscribe(state.ProductGradualRollout, tr.processUpdate)
		log.Debugf("TagResolver: Subscribed to K8S_INJECTION_DD remote configuration")
	}
	return tr
}

// ImageInfo represents information about an image.
type ImageInfo struct {
	Tag              string `json:"tag"`
	Digest           string `json:"digest"`
	CanonicalVersion string `json:"canonical_version"`
}

// RepositoryConfig represents a repository configuration.
type RepositoryConfig struct {
	RepositoryURL  string      `json:"repository_url"`
	RepositoryName string      `json:"repository_name"`
	Images         []ImageInfo `json:"images"`
}

// ResolvedImage represents a resolved image.
type ResolvedImage struct {
	FullImageRef     string // e.g., "gcr.io/project/image@sha256:abc123..."
	Digest           string
	CanonicalVersion string
}

func (tr *TagResolver) processUpdate(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	newCache := make(map[string]map[string]ResolvedImage)

	for configKey, rawConfig := range update {
		var repositories []RepositoryConfig
		if err := json.Unmarshal(rawConfig.Config, &repositories); err != nil {
			log.Errorf("TagResolver: Failed to unmarshal repository config for %s: %v", configKey, err)
			applyStateCallback(configKey, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
			continue
		}

		for _, repo := range repositories {
			if repo.RepositoryName == "" || repo.RepositoryURL == "" {
				log.Errorf("TagResolver: Missing repository_name or repository_url in config %s", configKey)
				applyStateCallback(configKey, state.ApplyStatus{
					State: state.ApplyStateError,
					Error: "Missing repository_name or repository_url",
				})
				continue
			}

			tagMap := make(map[string]ResolvedImage)
			for _, imageInfo := range repo.Images {
				if imageInfo.Tag == "" || imageInfo.Digest == "" {
					log.Warnf("TagResolver: Skipping invalid image entry (missing tag or digest) in %s", repo.RepositoryName)
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
		}
		applyStateCallback(configKey, state.ApplyStatus{
			State: state.ApplyStateAcknowledged,
		})
	}
	tr.tagMappings = newCache
}

// ResolveImageTag resolves an image tag from a repository name and a tag.
func (tr *TagResolver) ResolveImageTag(repositoryName, tag string) string {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	if len(tr.tagMappings) == 0 {
		log.Debugf("TagResolver: Cache empty, using original tag %s for %s", tag, repositoryName)
		return ""
	}

	repoCache, exists := tr.tagMappings[repositoryName]
	if !exists {
		log.Debugf("TagResolver: No mapping found for repository %s", repositoryName)
		return ""
	}

	resolved, exists := repoCache[tag]
	if !exists {
		log.Debugf("TagResolver: No mapping found for %s:%s", repositoryName, tag)
		return ""
	}

	log.Debugf("TagResolver: Resolved %s:%s -> %s", repositoryName, tag, resolved.FullImageRef)
	return resolved.FullImageRef
}
