// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// AutoInstrumentationCatalog represents the Remote Configuration payload containing
// tag-to-digest mappings for APM instrumentation libraries
type AutoInstrumentationCatalog struct {
	Version string                 `json:"version"`
	Images  []InstrumentationImage `json:"images"`
}

// InstrumentationImage represents a single image mapping from the catalog
type InstrumentationImage struct {
	Repository       string `json:"repository"`        // e.g., gcr.io/datadoghq/dd-lib-java-init
	Tag              string `json:"tag"`               // e.g., latest, v1, v1.2
	Digest           string `json:"digest"`            // e.g., sha256:abc123...
	CanonicalVersion string `json:"canonical_version"` // e.g., v1.29.0
	Language         string `json:"language"`          // e.g., java, python
}

// RemoteConfigClient interface for dependency injection and testing
type RemoteConfigClient interface {
	Subscribe(product string, fn func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)))
}

// TagResolver provides deterministic image tag resolution using Remote Configuration
type TagResolver struct {
	catalog  *AutoInstrumentationCatalog
	mappings map[string]string // imageRef -> digest
	mutex    sync.RWMutex
	rcClient RemoteConfigClient
}

// Mutable tag patterns - only resolve tags matching these patterns
var mutableTagPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^latest$`),
	regexp.MustCompile(`^v?\d+$`),           // v1, v2, 1, 2
	regexp.MustCompile(`^v?\d+\.\d+$`),      // v1.2, 1.2  
	regexp.MustCompile(`^main$`),
	regexp.MustCompile(`^master$`),
	regexp.MustCompile(`^dev$`),
	regexp.MustCompile(`^development$`),
}

// Datadog registry patterns - only resolve images from these registries
var datadogRegistries = []*regexp.Regexp{
	regexp.MustCompile(`^gcr\.io/datadoghq/`),
	regexp.MustCompile(`^datadog/`),
	regexp.MustCompile(`^public\.ecr\.aws/datadog/`),
}

// NewTagResolver creates a new TagResolver with Remote Configuration integration
func NewTagResolver(rcClient RemoteConfigClient) *TagResolver {
	resolver := &TagResolver{
		mappings: make(map[string]string),
		rcClient: rcClient,
	}

	// Subscribe to auto-instrumentation catalog updates if RC client is provided
	if rcClient != nil {
		rcClient.Subscribe(state.ProductAutoInstrumentationCatalog, resolver.handleCatalogUpdate)
		log.Info("TagResolver: Subscribed to AUTO_INSTRUMENTATION_CATALOG remote configuration")
	}

	return resolver
}

// ResolveImageReference applies deterministic resolution for Datadog registry images with mutable tags
// Returns the original imageRef if no resolution should be applied
func (tr *TagResolver) ResolveImageReference(imageRef string) string {
	if tr == nil {
		return imageRef
	}

	tr.mutex.RLock()
	defer tr.mutex.RUnlock()

	// Check if this is a Datadog registry image
	if !tr.isDatadogRegistry(imageRef) {
		log.Debugf("TagResolver: Skipping resolution for non-Datadog registry: %s", imageRef)
		return imageRef
	}

	// Extract tag from image reference
	tag := tr.extractTag(imageRef)
	if tag == "" {
		log.Debugf("TagResolver: Skipping resolution, already digest reference: %s", imageRef)
		return imageRef
	}

	// Check if tag is mutable
	if !tr.isMutableTag(tag) {
		log.Debugf("TagResolver: Skipping resolution for specific version tag: %s", imageRef)
		return imageRef
	}

	// Look up digest mapping
	if digest, found := tr.mappings[imageRef]; found {
		// Convert tag reference to digest reference
		baseImage := tr.extractBaseImage(imageRef)
		resolvedRef := fmt.Sprintf("%s@%s", baseImage, digest)

		log.Infof("TagResolver: Resolved mutable tag %s -> %s", imageRef, resolvedRef)
		return resolvedRef
	}

	// No mapping found - log warning but continue with original tag
	log.Warnf("TagResolver: No digest mapping found for mutable tag %s, using original (non-deterministic)", imageRef)
	return imageRef
}

// handleCatalogUpdate processes Remote Configuration updates for the auto-instrumentation catalog
func (tr *TagResolver) handleCatalogUpdate(updates map[string]state.RawConfig, applyCallback func(string, state.ApplyStatus)) {
	tr.mutex.Lock()
	defer tr.mutex.Unlock()

	for configID, rawConfig := range updates {
		var catalog AutoInstrumentationCatalog
		if err := json.Unmarshal(rawConfig.Config, &catalog); err != nil {
			log.Errorf("TagResolver: Failed to unmarshal auto-instrumentation catalog: %v", err)
			applyCallback(configID, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: fmt.Sprintf("JSON unmarshal error: %v", err),
			})
			continue
		}

		// Rebuild mappings from catalog
		tr.rebuildMappings(&catalog)
		tr.catalog = &catalog

		log.Infof("TagResolver: Updated auto-instrumentation catalog with %d image mappings (version: %s)", 
			len(catalog.Images), catalog.Version)
		applyCallback(configID, state.ApplyStatus{State: state.ApplyStateAcknowledged})
	}
}

// rebuildMappings reconstructs the internal mapping table from the catalog
func (tr *TagResolver) rebuildMappings(catalog *AutoInstrumentationCatalog) {
	tr.mappings = make(map[string]string)

	for _, image := range catalog.Images {
		imageRef := fmt.Sprintf("%s:%s", image.Repository, image.Tag)
		tr.mappings[imageRef] = image.Digest

		log.Debugf("TagResolver: Added mapping %s -> %s (canonical: %s)", 
			imageRef, image.Digest, image.CanonicalVersion)
	}
}

// isDatadogRegistry checks if the image reference is from a Datadog-controlled registry
func (tr *TagResolver) isDatadogRegistry(imageRef string) bool {
	for _, pattern := range datadogRegistries {
		if pattern.MatchString(imageRef) {
			return true
		}
	}
	return false
}

// isMutableTag checks if the tag matches patterns for mutable tags
func (tr *TagResolver) isMutableTag(tag string) bool {
	for _, pattern := range mutableTagPatterns {
		if pattern.MatchString(tag) {
			return true
		}
	}
	return false
}

// extractTag extracts the tag portion from an image reference
// Returns empty string if the reference is already a digest reference
func (tr *TagResolver) extractTag(imageRef string) string {
	// Handle digest references (contain @sha256:)
	if strings.Contains(imageRef, "@sha256:") {
		return ""
	}

	// Extract tag after the last colon
	parts := strings.Split(imageRef, ":")
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-1]
}

// extractBaseImage extracts the repository portion from an image reference
func (tr *TagResolver) extractBaseImage(imageRef string) string {
	if strings.Contains(imageRef, "@") {
		return strings.Split(imageRef, "@")[0]
	}
	if strings.Contains(imageRef, ":") {
		return strings.Split(imageRef, ":")[0]
	}
	return imageRef
}

// GetCatalogInfo returns information about the current catalog (for debugging/monitoring)
func (tr *TagResolver) GetCatalogInfo() map[string]interface{} {
	if tr == nil {
		return map[string]interface{}{"status": "disabled"}
	}

	tr.mutex.RLock()
	defer tr.mutex.RUnlock()

	info := map[string]interface{}{
		"status":         "enabled",
		"mapping_count":  len(tr.mappings),
		"catalog_loaded": tr.catalog != nil,
	}

	if tr.catalog != nil {
		info["catalog_version"] = tr.catalog.Version
		info["image_count"] = len(tr.catalog.Images)
	}

	return info
}